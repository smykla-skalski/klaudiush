package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	guardedConfig = "[protection]\nenabled = true\npaths = [\"guarded/**\"]\n"
	unguarded     = "[protection]\nenabled = false\n"
	commitWarning = "\n[validators.git.commit]\nseverity = \"warning\"\n"
	evidenceGate  = "\n[evidence]\nenabled = true\n\n[[evidence.checks]]\nname = \"tests\"\n" +
		"commands = [\"./check.sh\"]\npaths = [\"src/**\"]\n"
	brokenConfig   = "[validators.git.commit\nbroken\n"
	blockOnFailure = "[failure_policy]\nmode = \"block\"\n"
	brokenMarkdown = "# T\n### Skip\ntext\n"
	warnedCommand  = `touch warned.txt && git commit --allow-empty -m "feat(api): add smoke"`
	unrelatedLog   = "unrelated-hook.log"
	globalConfig   = "{{HOME}}/.config/klaudiush/config.toml"
)

// Record names a hook capture a scenario keeps as a fixture: the first
// capture of the canonical event with the outcome.
type Record struct {
	Event   hook.CanonicalEvent
	Outcome Outcome
}

// Scenario is one live check. Config is appended to the project klaudiush
// configuration, Global is the sandbox global configuration, and Setup runs
// after hooks are installed and before the harness runs. Build returns the
// model script and run options; Check returns the problems it finds in the
// result. Records are captures kept as fixtures, with Workspace naming the
// files a replay needs and ReplaySkip saying why a fixture cannot replay on
// its own.
type Scenario struct {
	Name       string
	Summary    string
	Features   []Feature
	Enforces   bool
	Config     string
	Global     string
	Setup      func(sb *Sandbox) error
	Build      func(d Driver, sb *Sandbox) (Script, RunOptions)
	Check      func(r *Result) []string
	Records    []Record
	Workspace  map[string]string
	ReplaySkip string
}

// ProjectConfig is the scenario's project klaudiush configuration.
func (s Scenario) ProjectConfig(d Driver, sb *Sandbox) string {
	base := guardedConfig
	if strings.Contains(s.Config, "[protection]") {
		base = ""
	}

	return base + s.Config + "\n" + d.ProviderConfig(sb)
}

func single(calls ...Call) Script {
	return Script{Turns: [][]Call{calls}, Final: "harness script finished"}
}

func defaultRun(script Script) (Script, RunOptions) {
	return script, RunOptions{}
}

// Scenarios lists every live check.
func Scenarios() []Scenario {
	return []Scenario{
		controlScenario(),
		denyShellScenario(),
		denyWriteScenario(),
		parallelScenario(),
		warnPromptsScenario(),
		warnAllowedScenario(),
		afterToolScenario(),
		completionScenario(),
		failurePolicyScenario(),
		subagentScenario(),
	}
}

func controlScenario() Scenario {
	return Scenario{
		Name:    "control_unguarded",
		Summary: "with protection off the same calls change the files, so the deny checks are not vacuous",
		Config:  unguarded,
		Build: func(d Driver, sb *Sandbox) (Script, RunOptions) {
			script := Script{
				Turns: [][]Call{{d.ShellCall("touch guarded/shell.txt")}},
				Final: "harness script finished",
			}
			if d.Supports(FeatureWriteTool) {
				script.Turns = append(
					script.Turns,
					[]Call{d.WriteCall(sb, "guarded/write.txt", "written\n")},
				)
			}

			return defaultRun(script)
		},
		Check: func(r *Result) []string {
			problems := r.expectFiles("guarded/shell.txt")
			if r.Driver.Supports(FeatureWriteTool) {
				problems = append(problems, r.expectFiles("guarded/write.txt")...)
			}

			return append(problems, r.checkUnrelatedHook()...)
		},
		Records: []Record{{hook.CanonicalEventBeforeTool, OutcomePass}},
	}
}

func denyShellScenario() Scenario {
	return Scenario{
		Name:     "deny_shell",
		Summary:  "a denied shell command never runs and the reason reaches the model",
		Enforces: true,
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			return defaultRun(single(d.ShellCall("touch guarded/shell.txt")))
		},
		Check: func(r *Result) []string {
			problems := r.expectAbsent("guarded/shell.txt")
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventBeforeTool, OutcomeDeny, "POL001")...)
			problems = append(problems, r.expectModelSaw("POL001")...)

			return append(problems, r.checkUnrelatedHook()...)
		},
		Records: []Record{{hook.CanonicalEventBeforeTool, OutcomeDeny}},
	}
}

func denyWriteScenario() Scenario {
	return Scenario{
		Name:     "deny_write",
		Summary:  "a denied file write (Write, apply_patch, write) never lands",
		Features: []Feature{FeatureWriteTool},
		Enforces: true,
		Build: func(d Driver, sb *Sandbox) (Script, RunOptions) {
			return defaultRun(single(d.WriteCall(sb, "guarded/write.txt", "denied\n")))
		},
		Check: func(r *Result) []string {
			problems := r.expectAbsent("guarded/write.txt")
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventBeforeTool, OutcomeDeny, "POL001")...)

			return append(problems, r.expectModelSaw("POL001")...)
		},
		Records: []Record{{hook.CanonicalEventBeforeTool, OutcomeDeny}},
	}
}

func parallelScenario() Scenario {
	return Scenario{
		Name:     "parallel",
		Summary:  "two calls in one model turn: the denied one never runs, the allowed one does",
		Enforces: true,
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			return defaultRun(single(
				d.ShellCall("touch guarded/parallel.txt"),
				d.ShellCall("touch parallel-ok.txt"),
			))
		},
		Check: func(r *Result) []string {
			problems := r.expectAbsent("guarded/parallel.txt")
			problems = append(problems, r.expectFiles("parallel-ok.txt")...)
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventBeforeTool, OutcomeDeny, "POL001")...)

			if capture, ok := r.find(hook.CanonicalEventBeforeTool, OutcomeDeny); ok &&
				!strings.Contains(string(capture.Input), "guarded/parallel.txt") {
				problems = append(problems, "the denied call is not the guarded one")
			}

			if n := r.count(hook.CanonicalEventBeforeTool); n < minParallelHooks {
				problems = append(problems, "want at least 2 before-tool hooks, got "+itoa(n))
			}

			return problems
		},
	}
}

func warnPromptsScenario() Scenario {
	return Scenario{
		Name:     "warn_keeps_prompt",
		Summary:  "a warning does not approve the call: an unapproved command still goes through the harness permission flow",
		Features: []Feature{FeaturePermissionFlow},
		Enforces: true,
		Config:   commitWarning,
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			return single(d.ShellCall(warnedCommand)), RunOptions{AllowedTools: []string{toolWrite}}
		},
		Check: func(r *Result) []string {
			problems := r.expectAbsent("warned.txt")
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventBeforeTool, OutcomeAdvise, "GIT010")...)
			problems = append(problems, r.expectNoPermissionDecision()...)

			if !deniedByHarness(r.Output) {
				problems = append(
					problems,
					"the harness did not deny the unapproved command itself",
				)
			}

			return problems
		},
		Records: []Record{{hook.CanonicalEventBeforeTool, OutcomeAdvise}},
	}
}

func warnAllowedScenario() Scenario {
	return Scenario{
		Name:    "warn_allows",
		Summary: "a warning does not block an approved command",
		Config:  commitWarning,
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			return defaultRun(single(d.ShellCall(warnedCommand)))
		},
		Check: func(r *Result) []string {
			problems := r.expectFiles("warned.txt")
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventBeforeTool, OutcomeAdvise, "GIT010")...)

			return append(problems, r.expectNoPermissionDecision()...)
		},
	}
}

func afterToolScenario() Scenario {
	return Scenario{
		Name:     "after_tool_repair",
		Summary:  "a file a shell command broke is reported after the tool ran and the repair request reaches the model",
		Features: []Feature{FeatureAfterToolRepair},
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			return defaultRun(single(d.ShellCall(`printf '# T\n### Skip\ntext\n' > notes.md`)))
		},
		Check: func(r *Result) []string {
			problems := r.expectFiles("notes.md")
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventAfterTool, OutcomeAdvise, "FILE005")...)

			return append(problems, r.expectModelSaw("FILE005")...)
		},
		Records:   []Record{{hook.CanonicalEventAfterTool, OutcomeAdvise}},
		Workspace: map[string]string{"notes.md": brokenMarkdown},
	}
}

func completionScenario() Scenario {
	return Scenario{
		Name:     "completion_gate",
		Summary:  "the completion gate keeps the agent working while a required check is missing",
		Features: []Feature{FeatureCompletionGate},
		Enforces: true,
		Config:   evidenceGate,
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			return defaultRun(single(d.ShellCall("mkdir -p src && touch src/change.txt")))
		},
		Check: func(r *Result) []string {
			problems := r.expectFiles("src/change.txt")
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventTurnStop, OutcomeBlock, "EVID001")...)

			return append(problems, r.expectModelSaw("EVID001")...)
		},
		Records:    []Record{{hook.CanonicalEventTurnStop, OutcomeBlock}},
		ReplaySkip: "the gate compares against a baseline that earlier hooks of the session recorded",
	}
}

func failurePolicyScenario() Scenario {
	return Scenario{
		Name:     "failure_policy",
		Summary:  "with failure_policy block, an unreadable configuration denies the call (HOOK001)",
		Enforces: true,
		Global:   blockOnFailure,
		Setup: func(sb *Sandbox) error {
			return sb.WriteFile(
				filepath.Join(sb.Work, ".klaudiush", "config.toml"),
				brokenConfig,
				filePerm,
			)
		},
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			return defaultRun(single(d.ShellCall("touch failpolicy.txt")))
		},
		Check: func(r *Result) []string {
			problems := r.expectAbsent("failpolicy.txt")

			return append(
				problems,
				r.expectCapture(hook.CanonicalEventBeforeTool, OutcomeDeny, "HOOK001")...)
		},
		Records: []Record{{hook.CanonicalEventBeforeTool, OutcomeDeny}},
		Workspace: map[string]string{
			".klaudiush/config.toml": brokenConfig,
			globalConfig:             blockOnFailure,
		},
	}
}

func subagentScenario() Scenario {
	return Scenario{
		Name:     "subagent",
		Summary:  "a subagent's tool calls go through the same hooks and are denied too",
		Features: []Feature{FeatureSubagent},
		Enforces: true,
		Build: func(d Driver, _ *Sandbox) (Script, RunOptions) {
			nested, err := single(
				d.ShellCall("touch guarded/subagent.txt"),
			).Prompt("Run the nested harness script.")
			if err != nil {
				nested = err.Error()
			}

			return defaultRun(single(d.SubagentCall(nested)))
		},
		Check: func(r *Result) []string {
			problems := r.expectAbsent("guarded/subagent.txt")
			problems = append(
				problems,
				r.expectCapture(hook.CanonicalEventBeforeTool, OutcomeDeny, "POL001")...)

			capture, ok := r.find(hook.CanonicalEventBeforeTool, OutcomeDeny)
			if ok && !strings.Contains(string(capture.Input), `"agent_id"`) {
				problems = append(problems, "the denied subagent call carries no agent_id")
			}

			return problems
		},
		Records: []Record{{hook.CanonicalEventBeforeTool, OutcomeDeny}},
	}
}

// deniedByHarness reports whether Claude's print-mode result lists a
// permission denial of its own.
func deniedByHarness(output []byte) bool {
	var result struct {
		PermissionDenials []json.RawMessage `json:"permission_denials"`
	}

	return json.Unmarshal(output, &result) == nil && len(result.PermissionDenials) > 0
}

// writeGlobal writes the sandbox global klaudiush configuration.
func writeGlobal(sb *Sandbox, content string) error {
	if content == "" {
		return nil
	}

	return sb.WriteFile(
		filepath.Join(sb.ConfigHome(), "klaudiush", "config.toml"),
		content,
		filePerm,
	)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
