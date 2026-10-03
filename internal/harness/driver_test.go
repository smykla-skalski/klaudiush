package harness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// fakeBinary writes an executable script into the sandbox root.
func fakeBinary(sb *harness.Sandbox, name, body string) string {
	path := filepath.Join(sb.Root, name)
	Expect(sb.WriteFile(path, "#!/bin/sh\n"+body+"\n", 0o700)).To(Succeed())

	return path
}

func readJSON(path string) map[string]any {
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())

	var out map[string]any
	Expect(json.Unmarshal(data, &out)).To(Succeed())

	return out
}

var _ = Describe("drivers", func() {
	var (
		sb    *harness.Sandbox
		model *harness.ScriptedModel
	)

	BeforeEach(func() {
		sb = newSandbox()
		model = harness.NewScriptedModel()
		DeferCleanup(model.Close)
	})

	It("points Claude Code at the scripted model and registers Stop for the gate", func() {
		d := harness.NewClaudeDriver()
		Expect(d.Name()).To(Equal("claude"))
		Expect(d.Provider()).To(Equal(hook.ProviderClaude))
		Expect(d.KnownGap("2.1.288")).To(BeEmpty())
		Expect(d.Supports(harness.FeatureSubagent)).To(BeTrue())
		Expect(d.Prepare(sb, model)).To(Succeed())

		env := envMap(sb.Env())
		Expect(env).To(HaveKeyWithValue("ANTHROPIC_BASE_URL", model.URL()))
		Expect(env).To(HaveKey("ANTHROPIC_API_KEY"))
		Expect(d.ProviderConfig(sb)).To(ContainSubstring("[providers.claude]"))

		log := filepath.Join(sb.Root, "unrelated.log")
		Expect(d.SeedUnrelatedHook(sb, log)).To(Succeed())
		Expect(d.AfterInstall(context.Background(), sb, nil)).To(Succeed())
		Expect(readJSON(d.HookFile(sb))["hooks"]).NotTo(HaveKey("Stop"))

		Expect(
			d.AfterInstall(
				context.Background(),
				sb,
				[]harness.Feature{harness.FeatureCompletionGate},
			),
		).
			To(Succeed())
		settings := readJSON(d.HookFile(sb))
		Expect(settings["hooks"]).To(HaveKey("Stop"))
		Expect(settings["hooks"]).To(HaveKey("PreToolUse"))

		Expect(d.ShellCall("ls").Tool).To(Equal("Bash"))
		Expect(
			d.WriteCall(sb, "a.txt", "x").Args,
		).To(HaveKeyWithValue("file_path", filepath.Join(sb.Work, "a.txt")))
		Expect(d.SubagentCall("p").Tool).To(Equal("Agent"))
	})

	It("fails the Claude Stop registration without hooks to extend", func() {
		d := harness.NewClaudeDriver()
		gate := []harness.Feature{harness.FeatureCompletionGate}
		Expect(
			d.AfterInstall(context.Background(), sb, gate),
		).To(MatchError(ContainSubstring("reading")))

		Expect(sb.WriteFile(d.HookFile(sb), "{", 0o600)).To(Succeed())
		Expect(
			d.AfterInstall(context.Background(), sb, gate),
		).To(MatchError(ContainSubstring("parsing")))

		Expect(sb.WriteFile(d.HookFile(sb), "{}", 0o600)).To(Succeed())
		Expect(
			d.AfterInstall(context.Background(), sb, gate),
		).To(MatchError(ContainSubstring("no hooks")))
	})

	It("runs claude in print mode with the requested tools", func() {
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CLAUDE", fakeBinary(sb, "claude", `printf '%s\n' "$@"`))

		d := harness.NewClaudeDriver()

		out, err := d.Run(context.Background(), sb, "prompt", harness.RunOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("Bash,Write,Agent"))

		out, err = d.Run(
			context.Background(),
			sb,
			"prompt",
			harness.RunOptions{AllowedTools: []string{}},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).NotTo(ContainSubstring("--allowedTools"))
	})

	It("configures Codex for the scripted model and trusts listed hooks", func() {
		listing := `{"id":1,"result":{}}` + "\n" + `{"id":2,"result":{"data":[{"hooks":[` +
			`{"key":"/h/hooks.json:pre_tool_use:0:0","currentHash":"sha256:abc","trustStatus":"untrusted"}]}]}}`
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CODEX", fakeBinary(
			sb,
			"codex",
			`if [ "$1" = app-server ]; then head -n 3 > /dev/null; echo '`+listing+`'; exit 0; fi; printf '%s\n' "$@"`,
		))

		d := harness.NewCodexDriver()
		Expect(d.Name()).To(Equal("codex"))
		Expect(d.Provider()).To(Equal(hook.ProviderCodex))
		Expect(d.KnownGap("0.160.0")).To(BeEmpty())
		Expect(d.Supports(harness.FeatureAfterToolRepair)).To(BeFalse())
		Expect(d.Prepare(sb, model)).To(Succeed())
		Expect(d.ProviderConfig(sb)).To(ContainSubstring(d.HookFile(sb)))
		Expect(d.SeedUnrelatedHook(sb, filepath.Join(sb.Root, "log"))).To(Succeed())

		Expect(d.AfterInstall(context.Background(), sb, nil)).To(Succeed())

		config, err := os.ReadFile(filepath.Join(sb.CodexHome(), "config.toml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(config)).To(ContainSubstring(model.URL() + "/v1"))
		Expect(
			string(config),
		).To(ContainSubstring(`[hooks.state.'/h/hooks.json:pre_tool_use:0:0']`))
		Expect(string(config)).To(ContainSubstring(`trusted_hash = "sha256:abc"`))

		out, err := d.Run(context.Background(), sb, "prompt", harness.RunOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("workspace-write"))

		patch := d.WriteCall(sb, "guarded/w.txt", "a\nb\n")
		Expect(
			patch.Input,
		).To(Equal("*** Begin Patch\n*** Add File: guarded/w.txt\n+a\n+b\n*** End Patch\n"))
		Expect(d.ShellCall("ls").Args).To(HaveKeyWithValue("cmd", "ls"))
		Expect(d.SubagentCall("p").Tool).To(Equal("spawn_agent"))
	})

	It("fails Codex trust when no hook is listed", func() {
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CODEX", fakeBinary(sb, "codex",
			`head -n 3 > /dev/null; echo '{"id":2,"result":{"data":[]}}'`))

		d := harness.NewCodexDriver()
		Expect(d.Prepare(sb, model)).To(Succeed())
		Expect(
			d.AfterInstall(context.Background(), sb, nil),
		).To(MatchError(ContainSubstring("no hooks")))
	})

	It("reports a Codex hook listing error", func() {
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CODEX", fakeBinary(sb, "codex",
			`head -n 3 > /dev/null; echo 'noise'; echo '{"id":2,"error":{"message":"denied"}}'`))

		d := harness.NewCodexDriver()
		Expect(
			d.AfterInstall(context.Background(), sb, nil),
		).To(MatchError(ContainSubstring("denied")))
	})

	It("reports a Codex app-server that closes without answering", func() {
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CODEX", fakeBinary(sb, "codex", `head -n 3 > /dev/null; exit 0`))
		DeferCleanup(harness.SetCodexTrustLimit(time.Second))

		d := harness.NewCodexDriver()
		Expect(
			d.AfterInstall(context.Background(), sb, nil),
		).To(MatchError(ContainSubstring("app-server")))
	})

	It("reports configuration errors Codex hit while listing hooks", func() {
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CODEX", fakeBinary(sb, "codex", `head -n 3 > /dev/null; `+
			`echo '{"id":2,"result":{"data":[{"hooks":[],"errors":[{"message":"bad catalog"}]}]}}'`))

		d := harness.NewCodexDriver()
		Expect(
			d.AfterInstall(context.Background(), sb, nil),
		).To(MatchError(ContainSubstring("bad catalog")))
	})

	It("picks opencode tool names by major version and reports the 2.x gap", func() {
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_OPENCODE", fakeBinary(sb, "opencode", `printf '%s\n' "$@"`))

		d := harness.NewOpenCodeDriver()
		Expect(d.Name()).To(Equal("opencode"))
		Expect(d.Provider()).To(Equal(hook.ProviderOpenCode))
		Expect(d.Supports(harness.FeatureCompletionGate)).To(BeFalse())
		Expect(d.Prepare(sb, model)).To(Succeed())
		Expect(d.ProviderConfig(sb)).To(ContainSubstring("[providers.opencode]"))
		Expect(d.HookFile(sb)).To(HaveSuffix("klaudiush.ts"))
		Expect(d.SeedUnrelatedHook(sb, "")).To(Succeed())
		Expect(d.AfterInstall(context.Background(), sb, nil)).To(Succeed())
		Expect(d.SubagentCall("p").Tool).To(Equal("task"))

		d.SetVersion("1.14.0")
		Expect(d.KnownGap("1.14.0")).To(BeEmpty())
		Expect(d.ShellCall("ls").Tool).To(Equal("bash"))
		Expect(d.WriteCall(sb, "a", "x").Args).To(HaveKey("filePath"))

		d.SetVersion("2.0.19")
		Expect(d.KnownGap("2.0.19")).To(ContainSubstring("bridge plugin"))
		Expect(d.ShellCall("ls").Tool).To(Equal("shell"))
		Expect(d.WriteCall(sb, "a", "x").Args).To(HaveKey("path"))

		out, err := d.Run(context.Background(), sb, "prompt", harness.RunOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("--standalone"))
	})

	It("reads harness versions and explains a binary that does not run", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		Expect(
			harness.Version(ctx, sb, fakeBinary(sb, "v", "echo 'codex-cli 0.160.0'")),
		).To(Equal("0.160.0"))

		_, err := harness.Version(ctx, sb, fakeBinary(sb, "nov", "echo nothing"))
		Expect(err).To(MatchError(ContainSubstring("no version")))

		_, err = harness.Version(
			ctx,
			sb,
			fakeBinary(sb, "shim", "echo 'mise ERROR not active'; exit 1"),
		)
		Expect(err).To(MatchError(ContainSubstring("version manager shim")))
	})

	It("writes a Codex model catalog that is valid JSON", func() {
		var catalog struct {
			Models []map[string]any `json:"models"`
		}

		Expect(json.Unmarshal([]byte(harness.CodexCatalog), &catalog)).To(Succeed())
		Expect(catalog.Models).To(HaveLen(1))
		Expect(catalog.Models[0]).To(HaveKeyWithValue("apply_patch_tool_type", "freeform"))
		Expect(catalog.Models[0]).To(HaveKey("description"))
	})

	It("parses major versions", func() {
		Expect(harness.MajorVersion("2.0.19")).To(Equal(2))
		Expect(harness.MajorVersion("12.1")).To(Equal(12))
		Expect(harness.MajorVersion("")).To(Equal(0))
	})
})

var _ = Describe("Report", func() {
	It("records versions, contracts, results and writes JSON", func() {
		report := harness.NewReport("v1.2.3")
		entry := report.Harness("codex", hook.ProviderCodex)
		Expect(report.Harness("codex", hook.ProviderCodex)).To(BeIdenticalTo(entry))

		entry.Version = "0.160.0"
		entry.Status = harness.StatusRan
		report.Add(
			"codex",
			harness.ScenarioResult{Scenario: "deny_shell", Status: harness.StatusPassed},
		)
		report.Add(
			"codex",
			harness.ScenarioResult{Scenario: "after", Status: harness.StatusSkipped, Detail: "why"},
		)
		report.Add("missing", harness.ScenarioResult{Scenario: "x"})
		report.Harness("gemini", hook.ProviderGemini).Reason = "not installed"

		Expect(entry.Registered).To(ContainElement("PreToolUse"))
		Expect(entry.Contracts).To(ContainElement("Stop"))
		Expect(entry.Unsupported).To(ContainElement(ContainSubstring("write_stdin")))

		var summary bytes.Buffer
		report.Summary(&summary)
		Expect(summary.String()).To(ContainSubstring("codex     0.160.0"))
		Expect(summary.String()).To(ContainSubstring("after                skipped (why)"))
		Expect(summary.String()).To(ContainSubstring("gemini    -          skipped: not installed"))

		path := filepath.Join(GinkgoT().TempDir(), "out", "report.json")
		Expect(report.Write(path)).To(Succeed())
		Expect(readJSON(path)["klaudiush"]).To(Equal("v1.2.3"))
	})

	It("lists the events klaudiush registers per provider", func() {
		Expect(
			harness.RegisteredEvents(hook.ProviderClaude),
		).To(ContainElement("PostToolUseFailure"))
		Expect(harness.RegisteredEvents(hook.ProviderGemini)).To(ContainElement("AfterAgent"))
		Expect(
			harness.RegisteredEvents(hook.ProviderOpenCode),
		).To(ContainElement("tool.execute.before"))
		Expect(harness.RegisteredEvents(hook.ProviderUnknown)).To(BeNil())
		Expect(harness.RegisteredEvents(hook.Provider("bogus"))).To(BeNil())
		Expect(
			strings.Join(harness.RegisteredEvents(hook.ProviderCodex), ","),
		).To(Equal("SessionStart,PreToolUse,Stop"))
	})
})
