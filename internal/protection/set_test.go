package protection_test

import (
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Set", func() {
	var e *env

	BeforeEach(func() {
		e = newEnv(GinkgoT().TempDir(), "linux", nil)
	})

	DescribeTable(
		"protects policy files",
		func(rel, reason string) {
			m, ok := e.set().Check(filepath.Join(e.root, rel))
			Expect(ok).To(BeTrue(), rel)
			Expect(m.Reason).To(Equal(reason))
		},
		Entry("project config", "project/.klaudiush/config.toml", protection.ReasonKlaudiushConfig),
		Entry(
			"new file in project config dir",
			"project/.klaudiush/new.toml",
			protection.ReasonKlaudiushConfig,
		),
		Entry(
			"nested config",
			"project/sub/dir/.klaudiush/config.toml",
			protection.ReasonKlaudiushConfig,
		),
		Entry(
			"alternate config anywhere",
			"project/sub/klaudiush.toml",
			protection.ReasonKlaudiushConfig,
		),
		Entry(
			"global config",
			"home/.config/klaudiush/config.toml",
			protection.ReasonKlaudiushConfig,
		),
		Entry("legacy config", "home/.klaudiush/config.toml", protection.ReasonKlaudiushConfig),
		Entry(
			"session state",
			"home/.local/state/klaudiush/hook_sessions/state.json",
			protection.ReasonKlaudiushState,
		),
		Entry("data", "home/.local/share/klaudiush/backups/x", protection.ReasonKlaudiushState),
		Entry("binary", "home/bin/klaudiush", protection.ReasonKlaudiushBinary),
		Entry(
			"claude project settings",
			"project/.claude/settings.json",
			protection.ReasonClaudeSettings,
		),
		Entry(
			"claude local settings",
			"project/.claude/settings.local.json",
			protection.ReasonClaudeSettings,
		),
		Entry(
			"claude user settings",
			"home/.claude/settings.json",
			protection.ReasonClaudeSettings,
		),
		Entry("claude hook scripts", "project/.claude/hooks/guard.sh", protection.ReasonHookScript),
		Entry("project mcp servers", "project/.mcp.json", protection.ReasonMCPConfig),
		Entry("user mcp servers", "home/.claude.json", protection.ReasonMCPConfig),
		Entry("codex user hooks", "home/.codex/hooks.json", protection.ReasonCodexHooks),
		Entry("codex user config", "home/.codex/config.toml", protection.ReasonCodexHooks),
		Entry("codex project hooks", "project/.codex/hooks.json", protection.ReasonCodexHooks),
		Entry("gemini settings", "project/.gemini/settings.json", protection.ReasonGeminiSettings),
		Entry(
			"opencode plugin",
			"home/.config/opencode/plugin/klaudiush.ts",
			protection.ReasonOpenCodePlugin,
		),
	)

	DescribeTable("protects managed configuration",
		func(path string) {
			_, ok := e.set().Check(path)
			Expect(ok).To(BeTrue())
		},
		Entry("claude managed settings", "/etc/claude-code/managed-settings.json"),
		Entry("claude managed drop-in", "/etc/claude-code/managed-settings.d/10-hooks.json"),
		Entry("codex requirements", "/etc/codex/requirements.toml"),
		Entry("gemini system settings", "/etc/gemini-cli/settings.json"),
	)

	DescribeTable("leaves other files alone",
		func(rel string) {
			_, ok := e.set().Check(filepath.Join(e.root, rel))
			Expect(ok).To(BeFalse(), rel)
		},
		Entry("source file", "project/main.go"),
		Entry("other dotdir", "project/.github/workflows/ci.yml"),
		Entry("similar name", "project/.klaudiush-notes.md"),
		Entry("settings elsewhere", "project/app/settings.json"),
	)

	It("resolves symlinks to protected files", func() {
		link := filepath.Join(e.project, "innocent.json")
		Expect(os.Symlink(filepath.Join(e.project, ".claude", "settings.json"), link)).To(Succeed())

		m, ok := e.set().Check(link)
		Expect(ok).To(BeTrue())
		Expect(m.Reason).To(Equal(protection.ReasonClaudeSettings))
	})

	It("resolves symlinked directories on the way to a new file", func() {
		dir := filepath.Join(e.project, "conf")
		Expect(os.Symlink(filepath.Join(e.project, ".klaudiush"), dir)).To(Succeed())

		_, ok := e.set().Check(filepath.Join(dir, "fresh.toml"))
		Expect(ok).To(BeTrue())
	})

	It("protects the file behind a symlinked protected directory", func() {
		target := filepath.Join(e.root, "dotfiles", "claude")
		Expect(os.MkdirAll(target, 0o755)).To(Succeed())
		Expect(os.Symlink(target, filepath.Join(e.home, ".claude"))).To(Succeed())

		_, ok := e.set().Check(filepath.Join(target, "settings.json"))
		Expect(ok).To(BeTrue())
	})

	It("protects the target of a symlinked protected file", func() {
		target := e.write("dotfiles/claude-settings.json", "{}")
		Expect(os.MkdirAll(filepath.Join(e.home, ".claude"), 0o755)).To(Succeed())
		Expect(os.Symlink(target, filepath.Join(e.home, ".claude", "settings.json"))).To(Succeed())

		_, ok := e.set().Check(target)
		Expect(ok).To(BeTrue())
	})

	It("catches a hard link to a protected file", func() {
		if runtime.GOOS == "windows" {
			Skip("hard links are not inspected on Windows")
		}

		link := filepath.Join(e.project, "copy.toml")
		Expect(os.Link(filepath.Join(e.project, ".klaudiush", "config.toml"), link)).To(Succeed())

		m, ok := e.set().Check(link)
		Expect(ok).To(BeTrue())
		Expect(m.Reason).To(Equal(protection.ReasonKlaudiushConfig))
	})

	It("compares case-insensitively where the file system does", func() {
		folding := newEnv(GinkgoT().TempDir(), "darwin", nil)
		path := filepath.Join(folding.project, ".CLAUDE", "Settings.JSON")

		_, ok := folding.set().Check(path)
		Expect(ok).To(BeTrue())

		_, ok = e.set().Check(filepath.Join(e.project, ".CLAUDE", "Settings.JSON"))
		Expect(ok).To(BeFalse())
	})

	It("folds Unicode case variants that fold to the same letter", func() {
		folding := newEnv(GinkgoT().TempDir(), "darwin", nil)

		_, ok := folding.set().Check(filepath.Join(folding.project, ".claude", "ſettings.json"))
		Expect(ok).To(BeTrue())
	})

	It("matches directories holding protected files with CheckTree", func() {
		set := e.set()

		_, ok := set.Check(filepath.Join(e.project, ".claude"))
		Expect(ok).To(BeFalse())

		m, ok := set.CheckTree(filepath.Join(e.project, ".claude"))
		Expect(ok).To(BeTrue())
		Expect(m.Path).To(HaveSuffix("settings.json"))

		_, ok = set.CheckTree(e.project)
		Expect(ok).To(BeTrue())

		_, ok = set.CheckTree(filepath.Join(e.project, "src"))
		Expect(ok).To(BeFalse())
	})

	It("finds protected names inside an unlisted directory", func() {
		e.write("other/.klaudiush/config.toml", "")

		m, ok := e.set().CheckTree(filepath.Join(e.root, "other"))
		Expect(ok).To(BeTrue())
		Expect(m.Path).To(HaveSuffix(".klaudiush"))
	})

	Context("with configured paths", func() {
		It("protects extra paths, names and patterns", func() {
			e.opts.Config = &config.ProtectionConfig{
				Paths: []string{"scripts/ci/**", "Makefile", "~/policy", "tools/*.sh"},
			}
			set := e.set()

			for _, rel := range []string{
				"project/scripts/ci/run/test.sh",
				"project/sub/Makefile",
				"home/policy/rules.toml",
				"project/tools/lint.sh",
			} {
				m, ok := set.Check(filepath.Join(e.root, rel))
				Expect(ok).To(BeTrue(), rel)
				Expect(m.Reason).To(Equal(protection.ReasonConfigured))
			}

			_, ok := set.Check(filepath.Join(e.project, "tools", "sub", "x.txt"))
			Expect(ok).To(BeFalse())
		})

		It("lets allowed paths through", func() {
			e.opts.Config = &config.ProtectionConfig{
				Allow: []string{".claude/settings.local.json", "home-notes"},
			}
			set := e.set()

			_, ok := set.Check(filepath.Join(e.project, ".claude", "settings.local.json"))
			Expect(ok).To(BeFalse())
			Expect(set.Allowed(".claude/settings.local.json")).To(BeTrue())

			_, ok = set.Check(filepath.Join(e.project, ".claude", "settings.json"))
			Expect(ok).To(BeTrue())
		})

		It("rejects invalid patterns", func() {
			e.opts.Config = &config.ProtectionConfig{Paths: []string{"  "}}

			_, err := protection.NewSet(e.opts)
			Expect(err).To(MatchError(protection.ErrBadPattern))

			e.opts.Config = &config.ProtectionConfig{Allow: []string{""}}

			_, err = protection.NewSet(e.opts)
			Expect(err).To(MatchError(protection.ErrBadPattern))
		})
	})

	It("protects scripts that evidence checks and hooks run", func() {
		script := e.write("project/scripts/test.sh", "#!/bin/sh\n")
		hook := e.write("project/tools/guard.sh", "#!/bin/sh\n")
		e.write(
			"project/.claude/settings.json",
			`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"$CLAUDE_PROJECT_DIR/tools/guard.sh --strict"}]}]}}`,
		)
		e.opts.EvidenceCommands = []string{"bash ./scripts/test.sh -v", "mise run test"}

		set := e.set()

		m, ok := set.Check(script)
		Expect(ok).To(BeTrue())
		Expect(m.Reason).To(Equal(protection.ReasonEvidenceScript))

		m, ok = set.Check(hook)
		Expect(ok).To(BeTrue())
		Expect(m.Reason).To(Equal(protection.ReasonHookScript))
	})

	It("protects hook scripts named with quotes, variables and in TOML", func() {
		quoted := e.write("project/scripts/hook.sh", "")
		inHome := e.write("home/bin/guard.sh", "")
		inCodex := e.write("project/tools/codex-hook.sh", "")
		e.write(
			"project/.claude/settings.json",
			`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"\"$CLAUDE_PROJECT_DIR\"/scripts/hook.sh"},`+
				`{"type":"command","command":"${HOME}/bin/guard.sh --x"}]}]}}`,
		)
		e.write(
			"project/.codex/config.toml",
			"[[hooks.PreToolUse]]\n[[hooks.PreToolUse.hooks]]\ntype = \"command\"\ncommand = \"./tools/codex-hook.sh\"\n",
		)

		set := e.set()

		for _, path := range []string{quoted, inHome, inCodex} {
			m, ok := set.Check(path)
			Expect(ok).To(BeTrue(), path)
			Expect(m.Reason).To(Equal(protection.ReasonHookScript))
		}
	})

	It("protects configured hook files, plugins and CODEX_HOME", func() {
		codexHome := filepath.Join(e.root, "codex")
		e.opts.LookupEnv = func(name string) (string, bool) {
			if name == "CODEX_HOME" {
				return codexHome, true
			}

			return "", false
		}
		e.opts.HookFiles = []string{"custom/hooks.json"}
		e.opts.PluginPaths = []string{"plugins/check.sh"}
		e.opts.OpenCodePlugin = filepath.Join(e.root, "oc", "klaudiush.ts")
		set := e.set()

		for _, path := range []string{
			filepath.Join(codexHome, "hooks.json"),
			filepath.Join(e.project, "custom", "hooks.json"),
			filepath.Join(e.project, "plugins", "check.sh"),
			e.opts.OpenCodePlugin,
		} {
			_, ok := set.Check(path)
			Expect(ok).To(BeTrue(), path)
		}
	})
})

var _ = Describe("Validate", func() {
	It("accepts valid configuration", func() {
		Expect(protection.Validate(&config.ProtectionConfig{
			Paths: []string{"a/**", "b"},
			Allow: []string{"c"},
			ConfigChangeSources: []string{
				config.ConfigSourceProjectSettings,
				config.ConfigSourceSkills,
			},
		})).To(Succeed())
		Expect(protection.Validate(nil)).To(Succeed())
	})

	It("reports empty patterns and unknown sources", func() {
		err := protection.Validate(&config.ProtectionConfig{
			Paths:               []string{""},
			ConfigChangeSources: []string{"team_settings"},
		})
		Expect(err).To(MatchError(ContainSubstring("team_settings")))
		Expect(err).To(MatchError(ContainSubstring("empty pattern")))
	})
})

var _ = Describe("missing hook scripts", func() {
	It("protects a hook script that does not exist yet", func() {
		e := newEnv(GinkgoT().TempDir(), "linux", nil)
		e.write("project/.claude/settings.json",
			`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./hooks/missing.sh"}]}]}}`)

		_, ok := e.set().Check(filepath.Join(e.project, "hooks", "missing.sh"))
		Expect(ok).To(BeTrue())
	})
})

var _ = Describe("allowed symlinks", func() {
	It("does not allow the protected file an allowed symlink points to", func() {
		e := newEnv(GinkgoT().TempDir(), "linux", &config.ProtectionConfig{
			Allow: []string{".claude/settings.local.json"},
		})
		link := filepath.Join(e.project, ".claude", "settings.local.json")
		Expect(os.Symlink("settings.json", link)).To(Succeed())

		set := e.set()

		_, ok := set.Check(filepath.Join(e.project, ".claude", "settings.json"))
		Expect(ok).To(BeTrue())
		_, ok = set.Check(link)
		Expect(ok).To(BeTrue())
		Expect(set.Allowed(link)).To(BeFalse())
		Expect(set.Allowed(filepath.Join(e.project, ".claude", "settings.json"))).To(BeFalse())
	})
})
