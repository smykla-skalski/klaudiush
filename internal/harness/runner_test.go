package harness_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// fakeKlaudiush denies commands that touch guarded/ unless the project turns
// protection off, and passes the rest.
const fakeKlaudiush = `#!/bin/sh
[ "$1" = init ] && exit 0
input=$(cat)
grep -q '^enabled = false' .klaudiush/config.toml 2>/dev/null && exit 0
case "$input" in
*guarded/*) printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"[POL001] protected"}}' ;;
esac
`

// miniHarness asks the scripted model for tool calls over chat completions,
// runs each through the klaudiush hook on the sandbox PATH, and executes the
// calls the hook did not deny, the way a real harness does.
type miniHarness struct {
	model  *harness.ScriptedModel
	hooks  string
	runErr error
}

func (*miniHarness) Name() string            { return "mini" }
func (*miniHarness) Provider() hook.Provider { return hook.ProviderClaude }
func (*miniHarness) Binary() string          { return "/bin/sh" }
func (*miniHarness) BinaryError() error      { return nil }
func (*miniHarness) ProviderConfig(_ *harness.Sandbox) string {
	return "[providers.claude]\nenabled = true\n"
}

func (*miniHarness) HookFile(
	sb *harness.Sandbox,
) string {
	return filepath.Join(sb.Work, "hooks.json")
}
func (*miniHarness) SeedUnrelatedHook(_ *harness.Sandbox, _ string) error { return nil }
func (*miniHarness) Supports(_ harness.Feature) bool                      { return false }
func (*miniHarness) KnownGap(_ string) string                             { return "" }
func (*miniHarness) SubagentCall(_ string) harness.Call                   { return harness.Call{} }

func (*miniHarness) AfterInstall(_ context.Context, _ *harness.Sandbox, _ []harness.Feature) error {
	return nil
}

func (m *miniHarness) Prepare(sb *harness.Sandbox, model *harness.ScriptedModel) error {
	m.model = model

	hooks := m.hooks
	if hooks == "" {
		hooks = `{"hooks":{"PreToolUse":[]}}`
	}

	return sb.WriteFile(m.HookFile(sb), hooks, 0o600)
}

func (*miniHarness) ShellCall(command string) harness.Call {
	return harness.Call{Tool: "Bash", Args: map[string]any{"command": command}}
}

func (*miniHarness) WriteCall(
	_ *harness.Sandbox,
	_, _ string,
) harness.Call {
	return harness.Call{}
}

func (m *miniHarness) Run(
	ctx context.Context,
	sb *harness.Sandbox,
	prompt string,
	_ harness.RunOptions,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	messages := []map[string]any{{"role": "user", "content": prompt}}

	for range 5 {
		reply := m.ask(messages)
		if len(reply.ToolCalls) == 0 {
			return []byte(reply.Content), m.runErr
		}

		messages = append(
			messages,
			map[string]any{"role": "assistant", "tool_calls": reply.ToolCalls},
		)

		for _, call := range reply.ToolCalls {
			var args struct {
				Command string `json:"command"`
			}

			Expect(json.Unmarshal([]byte(call.Function.Arguments), &args)).To(Succeed())

			payload := `{"hook_event_name":"PreToolUse","cwd":"` + sb.Work + `","tool_name":"Bash",` +
				`"tool_input":{"command":"` + args.Command + `"}}`
			hookCmd := exec.CommandContext(
				ctx,
				filepath.Join(sb.Bin, "klaudiush"),
				"--hook-type",
				"PreToolUse",
			)
			hookCmd.Dir = sb.Work
			hookCmd.Env = sb.Env()
			hookCmd.Stdin = strings.NewReader(payload)

			// A hook that exits non-zero does not block, as in Claude and Codex.
			out, _ := hookCmd.Output()

			result := string(out)
			if !strings.Contains(result, `"deny"`) {
				ran, err := harness.RunIn(ctx, sb, sb.Work, "sh", "-c", args.Command)
				Expect(err).NotTo(HaveOccurred(), string(ran))

				result = "ok"
			}

			messages = append(messages, map[string]any{"role": "tool", "content": result})
		}
	}

	return nil, nil
}

type chatReply struct {
	Content   string `json:"content"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func (m *miniHarness) ask(messages []map[string]any) chatReply {
	body, err := json.Marshal(map[string]any{"messages": messages})
	Expect(err).NotTo(HaveOccurred())

	resp, err := http.Post(
		m.model.URL()+"/v1/chat/completions",
		"application/json",
		strings.NewReader(string(body)),
	)
	Expect(err).NotTo(HaveOccurred())

	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	var completion struct {
		Choices []struct {
			Message chatReply `json:"message"`
		} `json:"choices"`
	}

	Expect(json.Unmarshal(data, &completion)).To(Succeed())

	return completion.Choices[0].Message
}

func scenarioNamed(name string) harness.Scenario {
	for _, scenario := range harness.Scenarios() {
		if scenario.Name == name {
			return scenario
		}
	}

	Fail("no scenario " + name)

	return harness.Scenario{}
}

var _ = Describe("Runner", func() {
	var runner harness.Runner

	BeforeEach(func() {
		dir := GinkgoT().TempDir()
		binary := filepath.Join(dir, "klaudiush-under-test")
		Expect(writeExecutable(binary, fakeKlaudiush)).To(Succeed())

		runner = harness.Runner{Binary: binary, Base: dir}
	})

	run := func(name string) *harness.Result {
		result, cleanup, err := runner.Run(
			context.Background(),
			&miniHarness{},
			"1.0.0",
			scenarioNamed(name),
		)
		DeferCleanup(cleanup)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	It("proves a denied call left no side effect and keeps its capture as a fixture", func() {
		result := run("deny_shell")
		Expect(result.Problems()).To(BeEmpty())

		fixtures, err := result.Fixtures()
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures).To(HaveLen(1))
		Expect(fixtures[0].Expect).To(Equal(harness.OutcomeDeny))
		Expect(string(fixtures[0].Payload)).To(ContainSubstring(`"cwd":"{{WORK}}"`))
		Expect(fixtures[0].Validate()).To(Succeed())
	})

	It("lets the unguarded control change the file", func() {
		result := run("control_unguarded")
		Expect(result.Problems()).To(BeEmpty())
	})

	It(
		"reports a missing denial, a side effect and no feedback when the hook does not deny",
		func() {
			Expect(writeExecutable(runner.Binary, "#!/bin/sh\ncat > /dev/null\n")).To(Succeed())

			result := run("deny_shell")
			problems := result.Problems()
			Expect(problems).To(ContainElement(ContainSubstring("side effect happened")))
			Expect(
				problems,
			).To(ContainElement(ContainSubstring("no before_tool hook answered deny")))
			Expect(problems).To(ContainElement(ContainSubstring("never received POL001")))

			_, err := result.Fixtures()
			Expect(err).To(MatchError(ContainSubstring("no before_tool capture")))
		},
	)

	It("flags crashing hooks, unsupported response fields and stale installed events", func() {
		Expect(writeExecutable(
			runner.Binary,
			"#!/bin/sh\n[ \"$1\" = init ] && exit 0\ncat > /dev/null\necho '{\"verdict\":\"x\"}'\nexit 3\n",
		)).
			To(Succeed())

		result, cleanup, err := runner.Run(
			context.Background(),
			&miniHarness{
				hooks: `{"hooks":{"AfterToolUse":[]}}`,
			},
			"1.0.0",
			scenarioNamed("control_unguarded"),
		)
		DeferCleanup(cleanup)
		Expect(err).NotTo(HaveOccurred())

		problems := result.Problems()
		Expect(problems).To(ContainElement(ContainSubstring("hook exited 3")))
		Expect(problems).To(ContainElement(ContainSubstring("unsupported field verdict")))
		Expect(problems).To(ContainElement(ContainSubstring("installed hook")))
	})

	It("confirms a known gap only when the denied call ran unchecked", func() {
		Expect(run("deny_shell").GapConfirmed()).To(BeFalse())

		Expect(writeExecutable(runner.Binary, "#!/bin/sh\ncat > /dev/null\n")).To(Succeed())
		Expect(run("deny_shell").GapConfirmed()).To(BeTrue())
	})

	It("reports a harness that exited with an error after making the expected calls", func() {
		result, cleanup, err := runner.Run(
			context.Background(),
			&miniHarness{runErr: errors.New("exit status 2")},
			"1.0.0",
			scenarioNamed("deny_shell"),
		)
		DeferCleanup(cleanup)
		Expect(err).NotTo(HaveOccurred())

		Expect(result.HarnessProblems()).To(ConsistOf(ContainSubstring("exit status 2")))
		Expect(result.Problems()).To(ContainElement(ContainSubstring("exited with an error")))
	})

	It("keeps harness failures apart from the findings a known gap expects", func() {
		Expect(writeExecutable(
			runner.Binary,
			"#!/bin/sh\n[ \"$1\" = init ] && exit 0\ncat > /dev/null\nexit 3\n",
		)).To(Succeed())

		result := run("deny_shell")
		Expect(result.GapConfirmed()).To(BeTrue())
		Expect(result.HarnessProblems()).To(ContainElement(ContainSubstring("hook exited 3")))
		Expect(result.HarnessProblems()).NotTo(ContainElement(ContainSubstring("side effect")))
		Expect(result.Problems()).To(ContainElement(ContainSubstring("side effect")))
	})

	It("reports a harness that ran past the timeout", func() {
		runner.Timeout = time.Nanosecond

		result := run("deny_shell")
		Expect(result.TimedOut).To(BeTrue())
		Expect(result.Problems()).To(ContainElement(ContainSubstring("did not finish")))
		Expect(result.Problems()).NotTo(ContainElement(ContainSubstring("exited with an error")))
		Expect(result.GapConfirmed()).To(BeFalse())
	})

	It("fails setup when klaudiush init fails", func() {
		Expect(writeExecutable(runner.Binary, "#!/bin/sh\necho broken; exit 1\n")).To(Succeed())

		_, cleanup, err := runner.Run(
			context.Background(),
			&miniHarness{},
			"1.0.0",
			scenarioNamed("deny_shell"),
		)
		DeferCleanup(cleanup)
		Expect(err).To(MatchError(ContainSubstring("klaudiush init")))
	})

	It("keeps the sandbox when asked", func() {
		runner.Keep = true
		result, cleanup, err := runner.Run(
			context.Background(),
			&miniHarness{},
			"1.0.0",
			scenarioNamed("deny_shell"),
		)
		Expect(err).NotTo(HaveOccurred())

		cleanup()
		Expect(result.Sandbox.Root).To(BeADirectory())
		Expect(result.Sandbox.Close()).To(Succeed())
	})

	It("lists every scenario with a check and a summary", func() {
		for _, scenario := range harness.Scenarios() {
			Expect(scenario.Summary).NotTo(BeEmpty(), scenario.Name)
			Expect(scenario.Check).NotTo(BeNil(), scenario.Name)
			Expect(scenario.Build).NotTo(BeNil(), scenario.Name)
		}
	})
})

func writeExecutable(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o700)
}
