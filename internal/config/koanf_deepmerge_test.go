package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// helper to create a loader with separate home and work dirs.
func newSeparatedLoader() (loader *KoanfLoader, homeDir, workDir string) {
	var err error

	homeDir, err = os.MkdirTemp("", "koanf-deepmerge-home-")
	Expect(err).NotTo(HaveOccurred())

	workDir, err = os.MkdirTemp("", "koanf-deepmerge-work-")
	Expect(err).NotTo(HaveOccurred())

	loader, err = NewKoanfLoaderWithDirs(homeDir, workDir)
	Expect(err).NotTo(HaveOccurred())

	return loader, homeDir, workDir
}

func writeProjectConfig(workDir, content string) {
	dir := filepath.Join(workDir, ProjectConfigDir)
	err := os.MkdirAll(dir, 0o755)
	Expect(err).NotTo(HaveOccurred())

	err = os.WriteFile(filepath.Join(dir, ProjectConfigFile), []byte(content), 0o644)
	Expect(err).NotTo(HaveOccurred())
}

func writeGlobalConfig(homeDir, content string) {
	dir := filepath.Join(homeDir, GlobalConfigDir)
	err := os.MkdirAll(dir, 0o755)
	Expect(err).NotTo(HaveOccurred())

	err = os.WriteFile(filepath.Join(dir, GlobalConfigFile), []byte(content), 0o644)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Deep merge config integration", func() {
	// ===================================================================
	// TASK 1: Single-source partial configs - setting 1 field must not
	// wipe sibling defaults in the same section.
	// ===================================================================
	Describe("single-source partial project config", func() {
		// --- Markdown ---
		Context("markdown: only enabled=true", func() {
			It("preserves all markdown defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.file.markdown]
enabled = true
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeTrue(), "enabled")
				Expect(md.GetSeverity().String()).To(Equal("error"), "severity")
				Expect(md.UseMarkdownlint).NotTo(BeNil(), "use_markdownlint nil")
				Expect(*md.UseMarkdownlint).To(BeTrue(), "use_markdownlint")
				Expect(md.HeadingSpacing).NotTo(BeNil(), "heading_spacing nil")
				Expect(*md.HeadingSpacing).To(BeTrue(), "heading_spacing")
				Expect(md.CodeBlockFormatting).NotTo(BeNil(), "code_block_formatting nil")
				Expect(*md.CodeBlockFormatting).To(BeTrue(), "code_block_formatting")
				Expect(md.ListFormatting).NotTo(BeNil(), "list_formatting nil")
				Expect(*md.ListFormatting).To(BeTrue(), "list_formatting")
				Expect(md.ContextLines).NotTo(BeNil(), "context_lines nil")
				Expect(*md.ContextLines).To(Equal(2), "context_lines")
			})
		})

		Context("markdown: only use_markdownlint=false", func() {
			It("preserves enabled and other booleans", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.file.markdown]
use_markdownlint = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(*md.UseMarkdownlint).To(BeFalse(), "use_markdownlint set to false")
				Expect(*md.HeadingSpacing).To(BeTrue(), "heading_spacing preserved")
				Expect(*md.CodeBlockFormatting).To(BeTrue(), "code_block_formatting preserved")
				Expect(*md.ListFormatting).To(BeTrue(), "list_formatting preserved")
			})
		})

		// --- Shellscript ---
		Context("shellscript: only severity=warning", func() {
			It("preserves all shellscript defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.file.shellscript]
severity = "warning"
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				ss := cfg.Validators.File.ShellScript
				Expect(ss.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(ss.GetSeverity().String()).To(Equal("warning"), "severity set")
				Expect(ss.UseShellcheck).NotTo(BeNil(), "use_shellcheck nil")
				Expect(*ss.UseShellcheck).To(BeTrue(), "use_shellcheck preserved")
				Expect(ss.ShellcheckSeverity).To(Equal("warning"), "shellcheck_severity preserved")
				Expect(ss.ContextLines).NotTo(BeNil(), "context_lines nil")
				Expect(*ss.ContextLines).To(Equal(2), "context_lines preserved")
			})
		})

		// --- Terraform ---
		Context("terraform: only check_format=false", func() {
			It("preserves all terraform defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.file.terraform]
check_format = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				tf := cfg.Validators.File.Terraform
				Expect(tf.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(tf.GetSeverity().String()).To(Equal("error"), "severity preserved")
				Expect(*tf.CheckFormat).To(BeFalse(), "check_format set to false")
				Expect(tf.UseTflint).NotTo(BeNil(), "use_tflint nil")
				Expect(*tf.UseTflint).To(BeTrue(), "use_tflint preserved")
				Expect(tf.ToolPreference).To(Equal("auto"), "tool_preference preserved")
			})
		})

		// --- Workflow ---
		Context("workflow: only enforce_digest_pinning=false", func() {
			It("preserves all workflow defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.file.workflow]
enforce_digest_pinning = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				wf := cfg.Validators.File.Workflow
				Expect(wf.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(*wf.EnforceDigestPinning).To(BeFalse(), "enforce_digest_pinning set")
				Expect(*wf.RequireVersionComment).To(BeTrue(), "require_version_comment preserved")
				Expect(*wf.CheckLatestVersion).To(BeTrue(), "check_latest_version preserved")
				Expect(*wf.UseActionlint).To(BeTrue(), "use_actionlint preserved")
			})
		})

		// --- Commit ---
		Context("commit: only severity=warning", func() {
			It("preserves all commit defaults including nested message config", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.commit]
severity = "warning"
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				commit := cfg.Validators.Git.Commit
				Expect(commit.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(commit.GetSeverity().String()).To(Equal("warning"), "severity set")
				Expect(*commit.CheckStagingArea).To(BeTrue(), "check_staging_area preserved")
				Expect(
					commit.RequiredFlags,
				).To(ContainElements("-s", "-S"), "required_flags preserved")

				// Nested message config should be fully preserved
				msg := commit.Message
				Expect(msg).NotTo(BeNil(), "message config nil")
				Expect(msg.Enabled).NotTo(BeNil(), "message.enabled nil")
				Expect(*msg.Enabled).To(BeTrue(), "message.enabled preserved")
				Expect(*msg.TitleMaxLength).To(Equal(50), "message.title_max_length preserved")
				Expect(
					*msg.BodyMaxLineLength,
				).To(Equal(72), "message.body_max_line_length preserved")
				Expect(
					*msg.ConventionalCommits,
				).To(BeTrue(), "message.conventional_commits preserved")
				Expect(*msg.RequireScope).To(BeTrue(), "message.require_scope preserved")
				Expect(
					*msg.BlockInfraScopeMisuse,
				).To(BeTrue(), "message.block_infra_scope_misuse preserved")
				Expect(*msg.BlockPRReferences).To(BeTrue(), "message.block_pr_references preserved")
				Expect(
					*msg.BlockAIAttribution,
				).To(BeTrue(), "message.block_ai_attribution preserved")
				Expect(
					msg.ValidTypes,
				).To(ContainElements("feat", "fix", "chore"), "message.valid_types preserved")
			})
		})

		// --- Push ---
		Context("push: only require_tracking=false", func() {
			It("preserves all push defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.push]
require_tracking = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				push := cfg.Validators.Git.Push
				Expect(push.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(push.GetSeverity().String()).To(Equal("error"), "severity preserved")
				Expect(*push.RequireTracking).To(BeFalse(), "require_tracking set to false")
			})
		})

		// --- Branch ---
		Context("branch: only allow_uppercase=true", func() {
			It("preserves all branch defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.branch]
allow_uppercase = true
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				branch := cfg.Validators.Git.Branch
				Expect(branch.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(*branch.AllowUppercase).To(BeTrue(), "allow_uppercase set")
				Expect(*branch.RequireType).To(BeTrue(), "require_type preserved")
				Expect(
					branch.ProtectedBranches,
				).To(ContainElements("main", "master"), "protected_branches preserved")
				Expect(
					branch.ValidTypes,
				).To(ContainElements("feat", "fix", "docs"), "valid_types preserved")
			})
		})

		// --- PR ---
		Context("pr: only require_body=false", func() {
			It("preserves all PR defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.pr]
require_body = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				pr := cfg.Validators.Git.PR
				Expect(pr.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(*pr.RequireBody).To(BeFalse(), "require_body set")
				Expect(*pr.TitleMaxLength).To(Equal(50), "title_max_length preserved")
				Expect(
					*pr.TitleConventionalCommits,
				).To(BeTrue(), "title_conventional_commits preserved")
				Expect(*pr.RequireChangelog).To(BeFalse(), "require_changelog preserved")
				Expect(*pr.CheckCILabels).To(BeTrue(), "check_ci_labels preserved")
				Expect(
					pr.MarkdownDisabledRules,
				).To(ContainElements("MD013", "MD034", "MD041"), "markdown_disabled_rules preserved")
				Expect(pr.ValidTypes).To(ContainElements("feat", "fix"), "valid_types preserved")
			})
		})

		// --- Exceptions rate_limit sub-map ---
		Context("exceptions: only token_prefix changed", func() {
			It("preserves rate_limit and audit sub-maps", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[exceptions]
token_prefix = "MYEXC"
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				exc := cfg.Exceptions
				Expect(exc.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(exc.TokenPrefix).To(Equal("MYEXC"), "token_prefix set")

				// rate_limit sub-map
				Expect(exc.RateLimit).NotTo(BeNil(), "rate_limit nil")
				Expect(
					exc.RateLimit.IsRateLimitEnabled(),
				).To(BeTrue(), "rate_limit.enabled preserved")
				Expect(
					exc.RateLimit.GetMaxPerHour(),
				).To(Equal(10), "rate_limit.max_per_hour preserved")
				Expect(
					exc.RateLimit.GetMaxPerDay(),
				).To(Equal(50), "rate_limit.max_per_day preserved")

				// audit sub-map
				Expect(exc.Audit).NotTo(BeNil(), "audit nil")
				Expect(exc.Audit.IsAuditEnabled()).To(BeTrue(), "audit.enabled preserved")
				Expect(exc.Audit.GetMaxSizeMB()).To(Equal(10), "audit.max_size_mb preserved")
				Expect(exc.Audit.GetMaxAgeDays()).To(Equal(30), "audit.max_age_days preserved")
				Expect(exc.Audit.GetMaxBackups()).To(Equal(3), "audit.max_backups preserved")
			})
		})

		// --- Global ---
		Context("global: only use_sdk_git=false", func() {
			It("preserves default_timeout", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[global]
use_sdk_git = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				global := cfg.Global
				Expect(global.UseSDKGit).NotTo(BeNil(), "use_sdk_git nil")
				Expect(*global.UseSDKGit).To(BeFalse(), "use_sdk_git set")
				Expect(global.DefaultTimeout).NotTo(BeZero(), "default_timeout preserved")
			})
		})
	})

	// ===================================================================
	// TASK 2: Multi-source merge precedence
	// defaults < global < project < env < flags
	// ===================================================================
	Describe("multi-source merge precedence", func() {
		Context("global sets enabled=false, project sets severity=warning", func() {
			It("merges both without wiping defaults", func() {
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
				Expect(md.IsEnabled()).To(BeFalse(), "enabled=false from global")
				Expect(
					md.GetSeverity().String(),
				).To(Equal("warning"), "severity=warning from project")
				Expect(*md.UseMarkdownlint).To(BeTrue(), "use_markdownlint from defaults")
				Expect(*md.HeadingSpacing).To(BeTrue(), "heading_spacing from defaults")
			})
		})

		Context("global sets one commit field, project sets different commit field", func() {
			It("merges both overrides with defaults", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[validators.git.commit]
check_staging_area = false
`)
				writeProjectConfig(workDir, `[validators.git.commit]
severity = "warning"
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				commit := cfg.Validators.Git.Commit
				Expect(commit.GetSeverity().String()).To(Equal("warning"), "severity from project")
				Expect(*commit.CheckStagingArea).To(BeFalse(), "check_staging_area from global")
				Expect(commit.IsEnabled()).To(BeTrue(), "enabled from defaults")
				Expect(
					commit.RequiredFlags,
				).To(ContainElements("-s", "-S"), "required_flags from defaults")
			})
		})

		Context("project overrides same field as global", func() {
			It("project wins", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[validators.file.shellscript]
shellcheck_severity = "error"
`)
				writeProjectConfig(workDir, `[validators.file.shellscript]
shellcheck_severity = "info"
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				ss := cfg.Validators.File.ShellScript
				Expect(ss.ShellcheckSeverity).To(Equal("info"), "project overrides global")
				Expect(ss.IsEnabled()).To(BeTrue(), "enabled from defaults")
				Expect(*ss.UseShellcheck).To(BeTrue(), "use_shellcheck from defaults")
			})
		})

		Context("global touches one section, project touches different section", func() {
			It("both merge without interfering", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[validators.file.markdown]
enabled = false
`)
				writeProjectConfig(workDir, `[validators.file.shellscript]
enabled = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(
					cfg.Validators.File.Markdown.IsEnabled(),
				).To(BeFalse(), "markdown disabled by global")
				Expect(
					cfg.Validators.File.ShellScript.IsEnabled(),
				).To(BeFalse(), "shellscript disabled by project")
				Expect(
					cfg.Validators.File.Terraform.IsEnabled(),
				).To(BeTrue(), "terraform unaffected")
				Expect(cfg.Validators.File.Workflow.IsEnabled()).To(BeTrue(), "workflow unaffected")
				Expect(cfg.Validators.Git.Commit.IsEnabled()).To(BeTrue(), "commit unaffected")
			})
		})

		Context("env var overrides specific field", func() {
			It("preserves defaults for unset fields", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })

				// No TOML configs - just env var
				os.Setenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED", "false")
				DeferCleanup(func() { os.Unsetenv("KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED") })

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeFalse(), "enabled=false from env")
				Expect(*md.UseMarkdownlint).To(BeTrue(), "use_markdownlint from defaults")
				Expect(*md.HeadingSpacing).To(BeTrue(), "heading_spacing from defaults")
			})
		})

		Context("--disable flag for one validator", func() {
			It("preserves other validators and defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })

				flags := map[string]any{
					"disable": []string{"markdown"},
				}
				cfg, err := loader.Load(flags)
				Expect(err).NotTo(HaveOccurred())

				// markdown disabled
				Expect(
					cfg.Validators.File.Markdown.IsEnabled(),
				).To(BeFalse(), "markdown disabled by flag")
				// other validators untouched
				Expect(
					cfg.Validators.File.ShellScript.IsEnabled(),
				).To(BeTrue(), "shellscript unaffected")
				Expect(cfg.Validators.Git.Commit.IsEnabled()).To(BeTrue(), "commit unaffected")
				// markdown fields besides enabled should be preserved
				Expect(
					*cfg.Validators.File.Markdown.UseMarkdownlint,
				).To(BeTrue(), "use_markdownlint preserved despite disable")
			})
		})

		Context("four sources: defaults + global + project + flags", func() {
			It("all layers merge correctly", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[validators.file.markdown]
severity = "warning"
`)
				writeProjectConfig(workDir, `[validators.file.markdown]
heading_spacing = false
`)

				flags := map[string]any{
					"disable": []string{"shellscript"},
				}
				cfg, err := loader.Load(flags)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeTrue(), "enabled from defaults")
				Expect(md.GetSeverity().String()).To(Equal("warning"), "severity from global")
				Expect(*md.HeadingSpacing).To(BeFalse(), "heading_spacing from project")
				Expect(*md.UseMarkdownlint).To(BeTrue(), "use_markdownlint from defaults")
				Expect(*md.CodeBlockFormatting).To(BeTrue(), "code_block_formatting from defaults")
				Expect(
					cfg.Validators.File.ShellScript.IsEnabled(),
				).To(BeFalse(), "shellscript disabled by flag")
			})
		})
	})

	// ===================================================================
	// TASK 3: Nested sub-maps and arrays
	// ===================================================================
	Describe("nested sub-maps and arrays", func() {
		Context("commit.message: setting one nested field", func() {
			It("preserves all other message defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.commit.message]
title_max_length = 72
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				msg := cfg.Validators.Git.Commit.Message
				Expect(msg).NotTo(BeNil(), "message not nil")
				Expect(*msg.TitleMaxLength).To(Equal(72), "title_max_length set to 72")
				Expect(
					*msg.BodyMaxLineLength,
				).To(Equal(72), "body_max_line_length default preserved")
				Expect(
					*msg.ConventionalCommits,
				).To(BeTrue(), "conventional_commits default preserved")
				Expect(*msg.RequireScope).To(BeTrue(), "require_scope default preserved")
				Expect(
					*msg.BlockInfraScopeMisuse,
				).To(BeTrue(), "block_infra_scope_misuse default preserved")
				Expect(*msg.BlockPRReferences).To(BeTrue(), "block_pr_references default preserved")
				Expect(
					msg.ValidTypes,
				).To(ContainElements("feat", "fix", "chore"), "valid_types default preserved")

				// Parent commit fields should also be preserved
				commit := cfg.Validators.Git.Commit
				Expect(commit.IsEnabled()).To(BeTrue(), "commit.enabled default preserved")
				Expect(
					*commit.CheckStagingArea,
				).To(BeTrue(), "commit.check_staging_area default preserved")
				Expect(
					commit.RequiredFlags,
				).To(ContainElements("-s", "-S"), "commit.required_flags default preserved")
			})
		})

		Context("exceptions.rate_limit: setting one nested field", func() {
			It("preserves other rate_limit fields and parent fields", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[exceptions.rate_limit]
max_per_hour = 5
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				rl := cfg.Exceptions.RateLimit
				Expect(rl).NotTo(BeNil(), "rate_limit not nil")
				Expect(rl.GetMaxPerHour()).To(Equal(5), "max_per_hour set to 5")
				Expect(rl.GetMaxPerDay()).To(Equal(50), "max_per_day default preserved")
				Expect(rl.IsRateLimitEnabled()).To(BeTrue(), "rate_limit.enabled default preserved")

				// Parent exceptions fields
				Expect(
					cfg.Exceptions.IsEnabled(),
				).To(BeTrue(), "exceptions.enabled default preserved")
				Expect(
					cfg.Exceptions.TokenPrefix,
				).To(Equal("EXC"), "token_prefix default preserved")

				// Sibling audit sub-map
				Expect(cfg.Exceptions.Audit).NotTo(BeNil(), "audit sub-map not nil")
				Expect(
					cfg.Exceptions.Audit.IsAuditEnabled(),
				).To(BeTrue(), "audit.enabled default preserved")
			})
		})

		Context("array fields: project replaces array (not append)", func() {
			It("replaces required_flags completely", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.commit]
required_flags = ["-s"]
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				commit := cfg.Validators.Git.Commit
				// Array should be replaced, not merged with default ["-s", "-S"]
				Expect(commit.RequiredFlags).To(Equal([]string{"-s"}), "required_flags replaced")
				// Other fields preserved
				Expect(commit.IsEnabled()).To(BeTrue(), "enabled preserved")
				Expect(*commit.CheckStagingArea).To(BeTrue(), "check_staging_area preserved")
			})

			It("replaces valid_types completely", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.commit.message]
valid_types = ["feat", "fix"]
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				msg := cfg.Validators.Git.Commit.Message
				Expect(msg.ValidTypes).To(Equal([]string{"feat", "fix"}), "valid_types replaced")
				Expect(*msg.TitleMaxLength).To(Equal(50), "title_max_length preserved")
			})

			It("replaces protected_branches completely", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.branch]
protected_branches = ["main", "develop", "release"]
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				branch := cfg.Validators.Git.Branch
				Expect(
					branch.ProtectedBranches,
				).To(Equal([]string{"main", "develop", "release"}), "protected_branches replaced")
				Expect(*branch.RequireType).To(BeTrue(), "require_type preserved")
			})
		})
	})

	// ===================================================================
	// TASK 4: Edge cases and real-world scenarios
	// ===================================================================
	Describe("edge cases", func() {
		Context("empty project config file", func() {
			It("all defaults are intact", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, "")

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				// Spot-check across all categories
				Expect(cfg.Validators.File.Markdown.IsEnabled()).To(BeTrue())
				Expect(*cfg.Validators.File.Markdown.UseMarkdownlint).To(BeTrue())
				Expect(cfg.Validators.File.ShellScript.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.Commit.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.Push.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.Branch.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.PR.IsEnabled()).To(BeTrue())
				Expect(cfg.Exceptions.IsEnabled()).To(BeTrue())
			})
		})

		Context("no config files at all", func() {
			It("all defaults are intact", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.Validators.File.Markdown.IsEnabled()).To(BeTrue())
				Expect(*cfg.Validators.File.Markdown.UseMarkdownlint).To(BeTrue())
				Expect(*cfg.Validators.File.Markdown.HeadingSpacing).To(BeTrue())
				Expect(*cfg.Validators.File.Markdown.CodeBlockFormatting).To(BeTrue())
				Expect(*cfg.Validators.File.Markdown.ListFormatting).To(BeTrue())
				Expect(cfg.Validators.File.ShellScript.IsEnabled()).To(BeTrue())
				Expect(*cfg.Validators.File.ShellScript.UseShellcheck).To(BeTrue())
				Expect(cfg.Validators.File.Terraform.IsEnabled()).To(BeTrue())
				Expect(*cfg.Validators.File.Terraform.UseTflint).To(BeTrue())
				Expect(cfg.Validators.File.Workflow.IsEnabled()).To(BeTrue())
				Expect(*cfg.Validators.File.Workflow.UseActionlint).To(BeTrue())
				Expect(cfg.Validators.Git.Commit.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.Push.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.Branch.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.PR.IsEnabled()).To(BeTrue())
				Expect(cfg.Exceptions.IsEnabled()).To(BeTrue())
			})
		})

		Context(
			"original bug report scenario: markdown enabled=true wiping use_markdownlint",
			func() {
				It("enabled=true does not wipe use_markdownlint or table_formatting", func() {
					loader, _, workDir := newSeparatedLoader()

					DeferCleanup(
						func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) },
					)
					// Exact config from the bug report
					writeProjectConfig(workDir, `[validators.file.markdown]
enabled = true
`)

					cfg, err := loader.Load(nil)
					Expect(err).NotTo(HaveOccurred())

					md := cfg.Validators.File.Markdown
					Expect(md.IsEnabled()).To(BeTrue())
					Expect(md.UseMarkdownlint).NotTo(BeNil(), "use_markdownlint must not be nil")
					Expect(*md.UseMarkdownlint).To(BeTrue(), "use_markdownlint must be true")
					// table_formatting doesn't have a default in the koanf map, check nil safety
					if md.TableFormatting != nil {
						// If set, should be from defaults
						Expect(
							*md.TableFormatting,
						).NotTo(BeFalse(), "table_formatting should not be false")
					}
				})
			},
		)

		Context("setting the same field in both global and project", func() {
			It("project wins for the field, all other defaults preserved", func() {
				loader, homeDir, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(homeDir); os.RemoveAll(workDir) })

				writeGlobalConfig(homeDir, `[validators.file.markdown]
use_markdownlint = false
`)
				writeProjectConfig(workDir, `[validators.file.markdown]
use_markdownlint = true
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(*md.UseMarkdownlint).To(BeTrue(), "project overrides global")
				Expect(md.IsEnabled()).To(BeTrue(), "enabled from defaults")
				Expect(*md.HeadingSpacing).To(BeTrue(), "heading_spacing from defaults")
			})
		})

		Context("multiple validators in one project config", func() {
			It("each validator preserves its own defaults", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.file.markdown]
enabled = true

[validators.file.shellscript]
enabled = false

[validators.git.commit]
severity = "warning"

[validators.git.push]
require_tracking = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				// markdown
				Expect(cfg.Validators.File.Markdown.IsEnabled()).To(BeTrue())
				Expect(
					*cfg.Validators.File.Markdown.UseMarkdownlint,
				).To(BeTrue(), "md: use_markdownlint preserved")
				Expect(
					*cfg.Validators.File.Markdown.HeadingSpacing,
				).To(BeTrue(), "md: heading_spacing preserved")

				// shellscript
				Expect(cfg.Validators.File.ShellScript.IsEnabled()).To(BeFalse())
				Expect(
					*cfg.Validators.File.ShellScript.UseShellcheck,
				).To(BeTrue(), "ss: use_shellcheck preserved")

				// commit
				Expect(cfg.Validators.Git.Commit.GetSeverity().String()).To(Equal("warning"))
				Expect(
					cfg.Validators.Git.Commit.IsEnabled(),
				).To(BeTrue(), "commit: enabled preserved")
				Expect(
					*cfg.Validators.Git.Commit.CheckStagingArea,
				).To(BeTrue(), "commit: check_staging_area preserved")
				Expect(
					cfg.Validators.Git.Commit.RequiredFlags,
				).To(ContainElements("-s", "-S"), "commit: required_flags preserved")

				// push
				Expect(*cfg.Validators.Git.Push.RequireTracking).To(BeFalse())
				Expect(cfg.Validators.Git.Push.IsEnabled()).To(BeTrue(), "push: enabled preserved")

				// Untouched validators
				Expect(
					cfg.Validators.File.Terraform.IsEnabled(),
				).To(BeTrue(), "terraform unaffected")
				Expect(cfg.Validators.Git.Branch.IsEnabled()).To(BeTrue(), "branch unaffected")
			})
		})

		Context("deeply nested: commit message field + parent field", func() {
			It("both levels merge without interference", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[validators.git.commit]
check_staging_area = false

[validators.git.commit.message]
conventional_commits = false
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				commit := cfg.Validators.Git.Commit
				Expect(*commit.CheckStagingArea).To(BeFalse(), "parent field set")
				Expect(commit.IsEnabled()).To(BeTrue(), "parent enabled preserved")
				Expect(
					commit.RequiredFlags,
				).To(ContainElements("-s", "-S"), "parent flags preserved")

				msg := commit.Message
				Expect(*msg.ConventionalCommits).To(BeFalse(), "nested field set")
				Expect(*msg.TitleMaxLength).To(Equal(50), "nested title_max_length preserved")
				Expect(
					*msg.BodyMaxLineLength,
				).To(Equal(72), "nested body_max_line_length preserved")
				Expect(*msg.RequireScope).To(BeTrue(), "nested require_scope preserved")
			})
		})

		Context("exceptions.audit: setting one field in deeply nested sub-map", func() {
			It("preserves sibling audit fields and parent exception fields", func() {
				loader, _, workDir := newSeparatedLoader()

				DeferCleanup(func() { os.RemoveAll(filepath.Dir(workDir)); os.RemoveAll(workDir) })
				writeProjectConfig(workDir, `[exceptions.audit]
max_size_mb = 50
`)

				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				audit := cfg.Exceptions.Audit
				Expect(audit).NotTo(BeNil())
				Expect(audit.GetMaxSizeMB()).To(Equal(50), "max_size_mb set")
				Expect(audit.IsAuditEnabled()).To(BeTrue(), "audit.enabled preserved")
				Expect(audit.GetMaxAgeDays()).To(Equal(30), "max_age_days preserved")
				Expect(audit.GetMaxBackups()).To(Equal(3), "max_backups preserved")

				// Parent fields
				Expect(cfg.Exceptions.IsEnabled()).To(BeTrue(), "exceptions.enabled preserved")
				Expect(cfg.Exceptions.TokenPrefix).To(Equal("EXC"), "token_prefix preserved")

				// Sibling rate_limit
				Expect(
					cfg.Exceptions.RateLimit.IsRateLimitEnabled(),
				).To(BeTrue(), "rate_limit.enabled preserved")
				Expect(
					cfg.Exceptions.RateLimit.GetMaxPerHour(),
				).To(Equal(10), "rate_limit.max_per_hour preserved")
			})
		})
	})
})
