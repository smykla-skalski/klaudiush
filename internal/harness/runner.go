package harness

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	gitConfig         = "config"
	defaultRunTimeout = 3 * time.Minute
	minParallelHooks  = 2
)

// Result is what one scenario run left behind.
type Result struct {
	Driver   Driver
	Version  string
	Scenario Scenario
	Sandbox  *Sandbox
	Model    *ScriptedModel
	Config   string
	Output   []byte
	RunErr   error
	Captures []Capture
}

// Runner sets up a sandbox per scenario, installs klaudiush hooks with
// `klaudiush init --install-hooks`, runs the harness and collects what the
// hooks saw. Binary is the klaudiush build under test; Base is where
// sandboxes are created; Keep leaves them on disk for debugging.
type Runner struct {
	Binary  string
	Base    string
	Timeout time.Duration
	Keep    bool
}

// Run executes one scenario. The returned cleanup removes the sandbox and
// stops the model; call it even when Run fails.
func (r Runner) Run(
	ctx context.Context,
	d Driver,
	version string,
	sc Scenario,
) (*Result, func(), error) {
	sb, err := NewSandbox(r.Base)
	if err != nil {
		return nil, func() {}, err
	}

	model := NewScriptedModel()
	cleanup := func() {
		model.Close()

		if !r.Keep {
			_ = sb.Close()
		}
	}

	result := &Result{Driver: d, Version: version, Scenario: sc, Sandbox: sb, Model: model}

	if setupErr := r.setup(ctx, d, sc, result); setupErr != nil {
		return result, cleanup, setupErr
	}

	script, opts := sc.Build(d, sb)

	prompt, err := script.Prompt("Follow the klaudiush harness script.")
	if err != nil {
		return result, cleanup, err
	}

	timeout := r.Timeout
	if timeout == 0 {
		timeout = defaultRunTimeout
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result.Output, result.RunErr = d.Run(runCtx, sb, prompt, opts)

	captures, err := sb.ReadCaptures()
	result.Captures = captures

	return result, cleanup, err
}

func (r Runner) setup(ctx context.Context, d Driver, sc Scenario, result *Result) error {
	sb := result.Sandbox

	if err := sb.InstallShim(r.Binary); err != nil {
		return err
	}

	sb.AddRedaction(r.Binary, "klaudiush")

	if resolved, err := filepath.EvalSymlinks(r.Binary); err == nil {
		sb.AddRedaction(resolved, "klaudiush")
	}

	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{gitConfig, "user.email", "harness@example.invalid"},
		{gitConfig, "user.name", "klaudiush harness"},
		{gitConfig, "commit.gpgsign", "false"},
	} {
		if out, err := RunIn(ctx, sb, sb.Work, "git", args...); err != nil {
			return errors.Wrapf(err, "git: %s", out)
		}
	}

	for _, dir := range []string{"guarded"} {
		if err := sb.WriteFile(filepath.Join(sb.Work, dir, ".keep"), "", filePerm); err != nil {
			return err
		}
	}

	result.Config = sc.ProjectConfig(d, sb)

	steps := []func() error{
		func() error {
			return sb.WriteFile(
				filepath.Join(sb.Work, ".klaudiush", "config.toml"),
				result.Config,
				filePerm,
			)
		},
		func() error { return writeGlobal(sb, sc.Global) },
		func() error { return d.Prepare(sb, result.Model) },
		func() error {
			if !d.Supports(FeatureUnrelatedHook) {
				return nil
			}

			return d.SeedUnrelatedHook(sb, filepath.Join(sb.Root, unrelatedLog))
		},
		func() error {
			out, err := RunIn(ctx, sb, sb.Work, r.Binary, "init", "--install-hooks")

			return errors.Wrapf(err, "klaudiush init: %s", out)
		},
		func() error { return d.AfterInstall(ctx, sb, sc.Features) },
		func() error {
			if sc.Setup == nil {
				return nil
			}

			return sc.Setup(sb)
		},
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

// Fixtures turns the scenario's recorded captures into redacted fixtures.
func (res *Result) Fixtures() ([]Fixture, error) {
	fixtures := make([]Fixture, 0, len(res.Scenario.Records))

	for _, record := range res.Scenario.Records {
		capture, ok := res.find(record.Event, record.Outcome)
		if !ok {
			return nil, errors.Newf("%s/%s: no %s capture with outcome %s",
				res.Driver.Name(), res.Scenario.Name, record.Event, record.Outcome)
		}

		fixture := Fixture{
			Provider:       res.Driver.Provider(),
			Harness:        res.Driver.Name(),
			HarnessVersion: res.Version,
			Source:         SourceLive,
			Scenario:       res.Scenario.Name,
			Event:          capture.Event(),
			Args:           capture.Args,
			Expect:         record.Outcome,
			Config:         res.Sandbox.Redact(res.Config),
			Workspace:      res.Scenario.Workspace,
			ReplaySkip:     res.Scenario.ReplaySkip,
			Payload:        json.RawMessage(res.Sandbox.Redact(string(capture.Input))),
			Response:       json.RawMessage("null"),
		}

		if output := strings.TrimSpace(string(capture.Output)); output != "" {
			fixture.Response = json.RawMessage(res.Sandbox.Redact(output))
		}

		fixtures = append(fixtures, fixture)
	}

	return fixtures, nil
}

func (res *Result) find(event hook.CanonicalEvent, outcome Outcome) (Capture, bool) {
	for _, capture := range res.Captures {
		if hook.NormalizeEventName(capture.Event()) != event {
			continue
		}

		if got, err := ClassifyResponse(capture.Output); err == nil && got == outcome {
			return capture, true
		}
	}

	return Capture{}, false
}

func (res *Result) count(event hook.CanonicalEvent) int {
	n := 0

	for _, capture := range res.Captures {
		if hook.NormalizeEventName(capture.Event()) == event {
			n++
		}
	}

	return n
}

func (res *Result) expectFiles(rels ...string) []string {
	var problems []string

	for _, rel := range rels {
		if !fileExists(filepath.Join(res.Sandbox.Work, rel)) {
			problems = append(problems, rel+" was not created, so the harness did not run the call")
		}
	}

	return problems
}

func (res *Result) expectAbsent(rel string) []string {
	if fileExists(filepath.Join(res.Sandbox.Work, rel)) {
		return []string{rel + " exists: the side effect happened"}
	}

	return nil
}

func (res *Result) expectCapture(event hook.CanonicalEvent, outcome Outcome, text string) []string {
	capture, ok := res.find(event, outcome)
	if !ok {
		return []string{"no " + string(event) + " hook answered " + string(outcome) +
			" (hooks seen: " + res.describeCaptures() + ")"}
	}

	if text != "" && !strings.Contains(string(capture.Output), text) {
		return []string{string(event) + " " + string(outcome) + " response lacks " + text}
	}

	if err := CheckResponse(res.Driver.Provider(), capture.Event(), capture.Output); err != nil {
		return []string{err.Error()}
	}

	return nil
}

func (res *Result) expectModelSaw(text string) []string {
	if res.Model.Saw(text) {
		return nil
	}

	return []string{"the model never received " + text + ", so the agent got no feedback"}
}

func (res *Result) expectNoPermissionDecision() []string {
	for _, capture := range res.Captures {
		if strings.Contains(string(capture.Output), "permissionDecision") {
			return []string{"a hook set permissionDecision on a warning: " + string(capture.Output)}
		}
	}

	return nil
}

// checkUnrelatedHook verifies klaudiush kept and ran the user's own hook.
func (res *Result) checkUnrelatedHook() []string {
	if !res.Driver.Supports(FeatureUnrelatedHook) {
		return nil
	}

	var problems []string

	if !fileExists(filepath.Join(res.Sandbox.Root, unrelatedLog)) {
		problems = append(problems, "the unrelated user hook did not run next to klaudiush")
	}

	data, err := readFile(res.Driver.HookFile(res.Sandbox))
	if err != nil || !strings.Contains(string(data), "unrelated hook ran") {
		problems = append(problems, "klaudiush init dropped the unrelated user hook")
	}

	return problems
}

func (res *Result) describeCaptures() string {
	parts := make([]string, 0, len(res.Captures))

	for _, capture := range res.Captures {
		outcome, _ := ClassifyResponse(capture.Output)
		parts = append(parts, capture.Event()+"="+string(outcome))
	}

	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, ", ")
}

func itoa(n int) string { return strconv.Itoa(n) }

var slugPattern = regexp.MustCompile(`[^A-Za-z0-9]`)

func slug(path string) string { return slugPattern.ReplaceAllString(path, "-") }
