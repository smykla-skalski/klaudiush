package policy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/parser"
	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/policy"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func parseHook(provider hook.Provider, event, payload string) *hook.Context {
	ctx, err := parser.NewJSONParser(strings.NewReader(payload)).
		ParseWithOptions(parser.ParseOptions{
			Provider:  provider,
			EventName: event,
		})
	Expect(err).NotTo(HaveOccurred())

	return ctx
}

func testLocator(root string, cfg *config.ProtectionConfig) policy.Locator {
	return func(*hook.Context) protection.Options {
		return protection.Options{
			WorkDir: filepath.Join(root, "project"),
			Home:    filepath.Join(root, "home"),
			Config:  cfg,
			GOOS:    "linux",
			LookupEnv: func(string) (string, bool) {
				return "", false
			},
		}
	}
}

var _ = Describe("ProtectionValidator", func() {
	var (
		root string
		cfg  *config.ProtectionConfig
		v    *policy.ProtectionValidator
	)

	validate := func(provider hook.Provider, event, payload string) *validator.Result {
		return v.Validate(context.Background(), parseHook(provider, event, payload))
	}

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(root, "project", ".klaudiush"), 0o755)).To(Succeed())

		enabled := true
		cfg = &config.ProtectionConfig{
			Enabled: &enabled,
			Allow:   []string{".claude/settings.local.json"},
		}
		v = policy.NewProtectionValidator(logger.NewNoOpLogger(), cfg, testLocator(root, cfg))
	})

	It("is named protection and does I/O", func() {
		Expect(v.Name()).To(Equal(policy.ProtectionValidatorName))
		Expect(v.Category()).To(Equal(validator.CategoryIO))
	})

	It("blocks a write to a protected file with a finding", func() {
		result := validate(
			hook.ProviderClaude,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":".claude/settings.json","content":"{}"}}`,
		)

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefProtectedFile))
		Expect(result.Findings).To(HaveLen(1))
		Expect(result.Findings[0].Message).To(ContainSubstring(protection.ReasonClaudeSettings))
		Expect(result.Findings[0].Actual).To(Equal("Write .claude/settings.json"))
	})

	It("passes other writes and allowed files", func() {
		result := validate(
			hook.ProviderClaude,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"main.go","content":"x"}}`,
		)
		Expect(result.Passed).To(BeTrue())

		result = validate(
			hook.ProviderClaude,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":".claude/settings.local.json"}}`,
		)
		Expect(result.Passed).To(BeTrue())
	})

	It("checks directories for tools of unknown kind", func() {
		result := validate(
			hook.ProviderClaude,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"mcp__fs__delete_tree","tool_input":{"path":".klaudiush"}}`,
		)
		Expect(result.ShouldBlock).To(BeTrue())
	})

	It("blocks Codex apply_patch on protected files", func() {
		result := validate(
			hook.ProviderCodex,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Add File: sub/klaudiush.toml\n+x\n*** End Patch\n"}}`,
		)
		Expect(result.ShouldBlock).To(BeTrue())
	})

	It("blocks shell commands and klaudiush policy commands", func() {
		result := validate(
			hook.ProviderGemini,
			"BeforeTool",
			`{"hook_event_name":"BeforeTool","tool_name":"run_shell_command","tool_input":{"command":"rm -rf .klaudiush && klaudiush disable x"}}`,
		)

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefProtectedFile))
		Expect(result.Findings).To(HaveLen(2))

		result = validate(
			hook.ProviderClaude,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"klaudiush bypass skip"}}`,
		)
		Expect(result.Reference).To(Equal(validator.RefPolicyCommand))

		result = validate(hook.ProviderClaude, "PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`)
		Expect(result.Passed).To(BeTrue())
	})

	It("resolves shell commands in the directory the tool call names", func() {
		Expect(os.MkdirAll(filepath.Join(root, "project", ".gemini"), 0o755)).To(Succeed())

		for _, key := range []string{"dir_path", "directory"} {
			result := validate(
				hook.ProviderGemini,
				"BeforeTool",
				`{"hook_event_name":"BeforeTool","tool_name":"run_shell_command","tool_input":{"command":"rm settings.json","`+
					key+`":".gemini"}}`,
			)
			Expect(result.ShouldBlock).To(BeTrue(), key)
		}

		result := validate(
			hook.ProviderCodex,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"exec_command","tool_input":{"cmd":"x","command":"rm config.toml","workdir":"`+
				filepath.Join(
					root,
					"project",
					".klaudiush",
				)+`"}}`,
		)
		Expect(result.ShouldBlock).To(BeTrue())

		result = validate(
			hook.ProviderGemini,
			"BeforeTool",
			`{"hook_event_name":"BeforeTool","tool_name":"run_shell_command","tool_input":{"command":"rm settings.json","dir_path":"src"}}`,
		)
		Expect(result.Passed).To(BeTrue())
	})

	It("fails closed on commands it cannot inspect", func() {
		result := validate(
			hook.ProviderClaude,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo 'unterminated"}}`,
		)

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Findings[0].Location).To(Equal("command"))
	})

	It("reports protected files a command changed after it ran", func() {
		result := validate(
			hook.ProviderClaude,
			"PostToolUse",
			`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"./x.sh"},`+
				`"tool_response":{"bashEditDiff":{"changedFiles":["`+filepath.Join(
				root,
				"project",
				".klaudiush",
				"config.toml",
			)+`","a.go"]}}}`,
		)

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Findings).To(HaveLen(1))
		Expect(result.Findings[0].Repair).To(ContainSubstring("Restore"))

		result = validate(
			hook.ProviderClaude,
			"PostToolUse",
			`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"./x.sh"},`+
				`"tool_response":{"bashEditDiff":{"changedFiles":["a.go"]}}}`,
		)
		Expect(result.Passed).To(BeTrue())
	})

	Context("on ConfigChange", func() {
		It("blocks protected sources", func() {
			result := validate(
				hook.ProviderClaude,
				"ConfigChange",
				`{"hook_event_name":"ConfigChange","source":"project_settings","file_path":".claude/settings.json"}`,
			)

			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(validator.RefConfigChangeBlocked))
			Expect(result.Message).To(ContainSubstring("project_settings"))
		})

		It("names the source when the file is not reported", func() {
			result := validate(hook.ProviderClaude, "ConfigChange",
				`{"hook_event_name":"ConfigChange","source":"user_settings"}`)

			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Findings[0].Location).To(Equal("user_settings"))
		})

		It("passes policy settings, other sources and allowed files", func() {
			for _, payload := range []string{
				`{"hook_event_name":"ConfigChange","source":"policy_settings"}`,
				`{"hook_event_name":"ConfigChange","source":"skills","file_path":".claude/skills/a/SKILL.md"}`,
				`{"hook_event_name":"ConfigChange","source":"local_settings","file_path":".claude/settings.local.json"}`,
			} {
				Expect(
					validate(hook.ProviderClaude, "ConfigChange", payload).Passed,
				).To(BeTrue(), payload)
			}
		})

		It("blocks nothing with an empty source list", func() {
			cfg.ConfigChangeSources = []string{}

			result := validate(hook.ProviderClaude, "ConfigChange",
				`{"hook_event_name":"ConfigChange","source":"project_settings"}`)
			Expect(result.Passed).To(BeTrue())
		})
	})

	It("blocks when the configuration does not compile", func() {
		cfg.Paths = []string{""}

		result := validate(
			hook.ProviderClaude,
			"PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"a.go"}}`,
		)
		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Message).To(ContainSubstring("invalid"))
	})
})

var _ = Describe("Locator", func() {
	It("finds the repository root", func() {
		root := GinkgoT().TempDir()
		sub := filepath.Join(root, "a", "b")
		Expect(os.MkdirAll(sub, 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(root, ".git"), 0o755)).To(Succeed())

		Expect(policy.ProjectRoot(sub)).To(Equal(root))

		bare := GinkgoT().TempDir()
		Expect(policy.ProjectRoot(bare)).To(Equal(bare))
	})

	It("collects configured files", func() {
		enabled := true
		cfg := &config.Config{
			Protection: &config.ProtectionConfig{Enabled: &enabled},
			Providers: &config.ProvidersConfig{
				Codex:    &config.CodexProviderConfig{HooksConfigPath: "/x/hooks.json"},
				Gemini:   &config.GeminiProviderConfig{SettingsPath: "/x/settings.json"},
				OpenCode: &config.OpenCodeProviderConfig{PluginPath: "/x/plugin.ts"},
			},
			Evidence: &config.EvidenceConfig{Checks: []*config.EvidenceCheckConfig{
				nil, {Name: "t", Commands: []string{"./t.sh"}},
			}},
			Plugins: &config.PluginConfig{Plugins: []*config.PluginInstanceConfig{
				nil, {Path: "/x/plugin"},
			}},
		}

		opts := policy.NewLocator(cfg)(&hook.Context{WorkingDir: "/work"})

		Expect(opts.WorkDir).To(Equal("/work"))
		Expect(opts.HookFiles).To(Equal([]string{"/x/hooks.json", "/x/settings.json"}))
		Expect(opts.OpenCodePlugin).To(Equal("/x/plugin.ts"))
		Expect(opts.EvidenceCommands).To(Equal([]string{"./t.sh"}))
		Expect(opts.PluginPaths).To(Equal([]string{"/x/plugin"}))
		Expect(opts.Config).To(BeIdenticalTo(cfg.Protection))
		Expect(opts.Executables).NotTo(BeEmpty())

		empty := policy.NewLocator(nil)(&hook.Context{})
		Expect(empty.WorkDir).NotTo(BeEmpty())
		Expect(empty.HookFiles).To(BeEmpty())
		Expect(empty.Config).To(BeNil())
	})

	It("keeps the files of inherited policy sources", func() {
		enabled := true
		source := &config.Config{
			Protection: &config.ProtectionConfig{Enabled: &enabled},
			Providers: &config.ProvidersConfig{
				Codex:    &config.CodexProviderConfig{HooksConfigPath: "/src/hooks.json"},
				OpenCode: &config.OpenCodeProviderConfig{PluginPath: "/src/plugin.ts"},
			},
			Evidence: &config.EvidenceConfig{Checks: []*config.EvidenceCheckConfig{
				{Name: "t", Commands: []string{"./t.sh", "./shared.sh"}},
			}},
			Plugins: &config.PluginConfig{Plugins: []*config.PluginInstanceConfig{
				{Path: "/src/plugin"},
			}},
		}
		cfg := &config.Config{
			Protection: source.Protection,
			Evidence: &config.EvidenceConfig{Checks: []*config.EvidenceCheckConfig{
				{Name: "s", Commands: []string{"./shared.sh"}},
			}},
			PolicySources: []*config.Config{source},
		}

		opts := policy.NewLocator(cfg)(&hook.Context{WorkingDir: "/work"})

		Expect(opts.HookFiles).To(Equal([]string{"/src/plugin.ts", "/src/hooks.json"}))
		Expect(opts.EvidenceCommands).To(Equal([]string{"./shared.sh", "./t.sh"}))
		Expect(opts.PluginPaths).To(Equal([]string{"/src/plugin"}))
	})
})
