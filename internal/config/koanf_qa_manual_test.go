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
				// Env: markdown enabled=false (use single-word field to avoid
				// envTransform issue where _ in field names becomes . in koanf path)
				os.Setenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED", "false")
				DeferCleanup(func() { os.Unsetenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED") })

				// Flags: disable shellscript
				flags := map[string]any{
					"disable": []string{"shellscript"},
				}

				cfg, err := loader.Load(flags)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeFalse(), "from env var")
				Expect(md.GetSeverity().String()).To(Equal("warning"), "from global")
				Expect(*md.HeadingSpacing).To(BeFalse(), "from project")
				Expect(*md.UseMarkdownlint).To(BeTrue(), "default preserved")
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
})
