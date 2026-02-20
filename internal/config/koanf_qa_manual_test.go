package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manual QA: end-to-end deep merge scenarios", func() {
	Describe("real-world config combinations", func() {
		Context("global disables markdown, project overrides severity only", func() {
			It("markdown stays disabled, severity changes, all other defaults survive", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[validators.file.markdown]
enabled = false
`)
				writeProjectConfig(workDir, `[validators.file.markdown]
severity = "warning"
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeFalse(), "global disabled it")
				Expect(md.GetSeverity().String()).To(Equal("warning"), "project set warning")
				Expect(*md.UseMarkdownlint).To(BeTrue(), "default preserved")
				Expect(*md.HeadingSpacing).To(BeTrue(), "default preserved")
				Expect(*md.CodeBlockFormatting).To(BeTrue(), "default preserved")
				Expect(*md.ListFormatting).To(BeTrue(), "default preserved")
				Expect(*md.ContextLines).To(Equal(2), "default preserved")
			})
		})

		Context("global changes commit title length, project changes commit scope", func() {
			It("both changes apply, all other commit and message defaults survive", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[validators.git.commit.message]
title_max_length = 72
`)
				writeProjectConfig(workDir, `[validators.git.commit.message]
require_scope = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				msg := cfg.Validators.Git.Commit.Message
				Expect(*msg.TitleMaxLength).To(Equal(72), "global set 72")
				Expect(*msg.RequireScope).To(BeFalse(), "project set false")
				Expect(*msg.BodyMaxLineLength).To(Equal(72), "default body_max_line_length")
				Expect(*msg.ConventionalCommits).To(BeTrue(), "default conventional_commits")
				Expect(*msg.BlockInfraScopeMisuse).To(BeTrue(), "default block_infra_scope_misuse")
				Expect(*msg.BlockPRReferences).To(BeTrue(), "default block_pr_references")
				Expect(*msg.BlockAIAttribution).To(BeTrue(), "default block_ai_attribution")
				Expect(
					msg.ValidTypes,
				).To(ContainElements("feat", "fix", "chore", "docs"), "default valid_types")

				// Parent commit fields
				commit := cfg.Validators.Git.Commit
				Expect(commit.IsEnabled()).To(BeTrue(), "parent enabled")
				Expect(*commit.CheckStagingArea).To(BeTrue(), "parent check_staging_area")
				Expect(
					commit.RequiredFlags,
				).To(ContainElements("-s", "-S"), "parent required_flags")
			})
		})

		Context("env var overrides one field while TOML sets another", func() {
			It("both sources merge correctly with defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })

				writeProjectConfig(workDir, `[validators.file.markdown]
heading_spacing = false
`)

				os.Setenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY", "warning")
				DeferCleanup(func() { os.Unsetenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY") })

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeTrue(), "default enabled")
				Expect(*md.HeadingSpacing).To(BeFalse(), "TOML set false")
				Expect(md.GetSeverity().String()).To(Equal("warning"), "env var set warning")
				Expect(*md.UseMarkdownlint).To(BeTrue(), "default preserved")
				Expect(*md.CodeBlockFormatting).To(BeTrue(), "default preserved")
			})
		})

		Context("all five sources active on different fields", func() {
			It("each source's field wins at its priority level", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				// Global: severity=warning
				writeGlobalConfig(homeDir, `[validators.file.markdown]
severity = "warning"
`)
				// Project: heading_spacing=false
				writeProjectConfig(workDir, `[validators.file.markdown]
heading_spacing = false
`)
				// Env: use_markdownlint=false (underscore in field name)
				os.Setenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_USE_MARKDOWNLINT", "false")
				DeferCleanup(
					func() { os.Unsetenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_USE_MARKDOWNLINT") },
				)

				// Flags: disable shellscript
				flags := map[string]any{
					"disable": []string{"shellscript"},
				}

				cfg, err := loader.Load(flags)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeTrue(), "default (no source disabled it)")
				Expect(md.GetSeverity().String()).To(Equal("warning"), "from global")
				Expect(*md.HeadingSpacing).To(BeFalse(), "from project")
				Expect(*md.UseMarkdownlint).To(BeFalse(), "from env var (underscored field)")
				Expect(*md.CodeBlockFormatting).To(BeTrue(), "default preserved")
				Expect(*md.ListFormatting).To(BeTrue(), "default preserved")
				Expect(*md.ContextLines).To(Equal(2), "default preserved")

				// shellscript disabled by flag, but its sub-fields survive
				ss := cfg.Validators.File.ShellScript
				Expect(ss.IsEnabled()).To(BeFalse(), "disabled by flag")
				Expect(*ss.UseShellcheck).To(BeTrue(), "default preserved despite disable")
			})
		})

		Context(
			"project overrides env var (env has lower priority than... wait, env is higher)",
			func() {
				It("env var wins over project TOML for the same field", func() {
					loader, _, workDir := newSeparatedLoader()

					DeferCleanup(
						func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) },
					)

					writeProjectConfig(workDir, `[validators.file.markdown]
severity = "warning"
`)
					os.Setenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY", "error")
					DeferCleanup(
						func() { os.Unsetenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY") },
					)

					cfg, err := loader.Load(nil)
					Expect(err).NotTo(HaveOccurred())

					// Env var has higher priority than project TOML
					Expect(
						cfg.Validators.File.Markdown.GetSeverity().String(),
					).To(Equal("error"), "env var wins")
				})
			},
		)

		Context("notification bell: only severity set", func() {
			It("preserves bell defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })

				writeProjectConfig(workDir, `[validators.notification.bell]
severity = "warning"
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				bell := cfg.Validators.Notification.Bell
				Expect(bell.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(bell.GetSeverity().String()).To(Equal("warning"), "severity set")
			})
		})

		// crash_dump and backup don't have entries in defaultsToMap(),
		// so their sub-fields come from struct defaults, not the koanf layer.
		// Deep merge still applies if both global and project set different
		// crash_dump fields - verify that works.
		Context("crash_dump: global sets enabled, project sets max_dumps", func() {
			It("both fields merge correctly", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[crash_dump]
enabled = false
`)
				writeProjectConfig(workDir, `[crash_dump]
max_dumps = 5
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				cd := cfg.CrashDump
				Expect(cd.IsEnabled()).To(BeFalse(), "enabled from global")
				Expect(cd.MaxDumps).NotTo(BeNil(), "max_dumps not nil")
				Expect(*cd.MaxDumps).To(Equal(5), "max_dumps from project")
			})
		})

		Context("backup: global sets auto_backup, project sets max_backups", func() {
			It("both fields merge correctly", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[backup]
auto_backup = false
`)
				writeProjectConfig(workDir, `[backup]
max_backups = 20
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				bk := cfg.Backup
				Expect(bk.AutoBackup).NotTo(BeNil(), "auto_backup not nil")
				Expect(*bk.AutoBackup).To(BeFalse(), "auto_backup from global")
				Expect(bk.MaxBackups).NotTo(BeNil(), "max_backups not nil")
				Expect(*bk.MaxBackups).To(Equal(20), "max_backups from project")
			})
		})
	})

	Describe("envMatchHierarchy", func() {
		DescribeTable("maps env var segments to koanf paths",
			func(envVar, expected string) {
				loader := &KoanfLoader{}
				path, _ := loader.envTransform(envVar, "test")
				Expect(path).To(Equal(expected))
			},
			// Simple single-word fields
			Entry("simple field",
				"KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED",
				"validators.git.commit.enabled"),
			Entry("simple severity",
				"KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY",
				"validators.file.markdown.severity"),

			// Underscored field names
			Entry("use_markdownlint",
				"KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_USE_MARKDOWNLINT",
				"validators.file.markdown.use_markdownlint"),
			Entry("check_staging_area",
				"KLAUDIUSH_VALIDATORS_GIT_COMMIT_CHECK_STAGING_AREA",
				"validators.git.commit.check_staging_area"),
			Entry("title_max_length in message",
				"KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH",
				"validators.git.commit.message.title_max_length"),
			Entry("heading_spacing",
				"KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_HEADING_SPACING",
				"validators.file.markdown.heading_spacing"),
			Entry("code_block_formatting",
				"KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_CODE_BLOCK_FORMATTING",
				"validators.file.markdown.code_block_formatting"),
			Entry("use_shellcheck",
				"KLAUDIUSH_VALIDATORS_FILE_SHELLSCRIPT_USE_SHELLCHECK",
				"validators.file.shellscript.use_shellcheck"),
			Entry("shellcheck_severity",
				"KLAUDIUSH_VALIDATORS_FILE_SHELLSCRIPT_SHELLCHECK_SEVERITY",
				"validators.file.shellscript.shellcheck_severity"),
			Entry("require_tracking",
				"KLAUDIUSH_VALIDATORS_GIT_PUSH_REQUIRE_TRACKING",
				"validators.git.push.require_tracking"),
			Entry("protected_branches",
				"KLAUDIUSH_VALIDATORS_GIT_BRANCH_PROTECTED_BRANCHES",
				"validators.git.branch.protected_branches"),
			Entry("block_infra_scope_misuse",
				"KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_BLOCK_INFRA_SCOPE_MISUSE",
				"validators.git.commit.message.block_infra_scope_misuse"),

			// Multi-word hierarchy segments
			Entry("crash_dump field",
				"KLAUDIUSH_CRASH_DUMP_MAX_DUMPS",
				"crash_dump.max_dumps"),
			Entry("crash_dump enabled",
				"KLAUDIUSH_CRASH_DUMP_ENABLED",
				"crash_dump.enabled"),
			Entry("no_verify enabled",
				"KLAUDIUSH_VALIDATORS_GIT_NO_VERIFY_ENABLED",
				"validators.git.no_verify.enabled"),
			Entry("rate_limit field",
				"KLAUDIUSH_EXCEPTIONS_RATE_LIMIT_MAX_PER_HOUR",
				"exceptions.rate_limit.max_per_hour"),

			// Global and session
			Entry("use_sdk_git",
				"KLAUDIUSH_GLOBAL_USE_SDK_GIT",
				"global.use_sdk_git"),
			Entry("default_timeout",
				"KLAUDIUSH_GLOBAL_DEFAULT_TIMEOUT",
				"global.default_timeout"),
			Entry("session state_file",
				"KLAUDIUSH_SESSION_STATE_FILE",
				"session.state_file"),
			Entry("session max_session_age",
				"KLAUDIUSH_SESSION_MAX_SESSION_AGE",
				"session.max_session_age"),

			// Secrets and shell validators
			Entry("secrets use_gitleaks",
				"KLAUDIUSH_VALIDATORS_SECRETS_SECRETS_USE_GITLEAKS",
				"validators.secrets.secrets.use_gitleaks"),
			Entry("backtick check_all_commands",
				"KLAUDIUSH_VALIDATORS_SHELL_BACKTICK_CHECK_ALL_COMMANDS",
				"validators.shell.backtick.check_all_commands"),

			// PR fields
			Entry("pr title_conventional_commits",
				"KLAUDIUSH_VALIDATORS_GIT_PR_TITLE_CONVENTIONAL_COMMITS",
				"validators.git.pr.title_conventional_commits"),
			Entry("pr markdown_disabled_rules",
				"KLAUDIUSH_VALIDATORS_GIT_PR_MARKDOWN_DISABLED_RULES",
				"validators.git.pr.markdown_disabled_rules"),
		)
	})
})
