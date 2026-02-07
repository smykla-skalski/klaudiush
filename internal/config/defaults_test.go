package config

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("Defaults", func() {
	Describe("DefaultConfig", func() {
		It("should return a complete config with all defaults", func() {
			cfg := DefaultConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Global).NotTo(BeNil())
			Expect(cfg.Validators).NotTo(BeNil())
			Expect(cfg.Rules).NotTo(BeNil())
		})
	})

	Describe("DefaultGlobalConfig", func() {
		It("should return global config with correct defaults", func() {
			cfg := DefaultGlobalConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.UseSDKGit).NotTo(BeNil())
			Expect(*cfg.UseSDKGit).To(BeTrue())
			Expect(cfg.DefaultTimeout.ToDuration()).To(Equal(10 * time.Second))
		})
	})

	Describe("DefaultRulesConfig", func() {
		It("should return rules config with correct defaults", func() {
			cfg := DefaultRulesConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.StopOnFirstMatch).NotTo(BeNil())
			Expect(*cfg.StopOnFirstMatch).To(BeTrue())
			Expect(cfg.Rules).To(BeEmpty())
		})
	})

	Describe("DefaultValidatorsConfig", func() {
		It("should return validators config with all sub-configs", func() {
			cfg := DefaultValidatorsConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Git).NotTo(BeNil())
			Expect(cfg.GitHub).NotTo(BeNil())
			Expect(cfg.File).NotTo(BeNil())
			Expect(cfg.Notification).NotTo(BeNil())
		})
	})

	Describe("DefaultGitConfig", func() {
		It("should return git config with all validators", func() {
			cfg := DefaultGitConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Commit).NotTo(BeNil())
			Expect(cfg.Push).NotTo(BeNil())
			Expect(cfg.Add).NotTo(BeNil())
			Expect(cfg.PR).NotTo(BeNil())
			Expect(cfg.Branch).NotTo(BeNil())
			Expect(cfg.NoVerify).NotTo(BeNil())
		})
	})

	Describe("DefaultGitHubConfig", func() {
		It("should return GitHub config with issue validator", func() {
			cfg := DefaultGitHubConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Issue).NotTo(BeNil())
		})
	})

	Describe("DefaultIssueValidatorConfig", func() {
		It("should return issue validator config with correct defaults", func() {
			cfg := DefaultIssueValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityWarning))
			Expect(cfg.RequireBody).NotTo(BeNil())
			Expect(*cfg.RequireBody).To(BeFalse())
			Expect(
				cfg.MarkdownDisabledRules,
			).To(ContainElements("MD013", "MD034", "MD041", "MD047"))
			Expect(cfg.Timeout.ToDuration()).To(Equal(10 * time.Second))
		})
	})

	Describe("DefaultFileConfig", func() {
		It("should return file config with all validators", func() {
			cfg := DefaultFileConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Markdown).NotTo(BeNil())
			Expect(cfg.ShellScript).NotTo(BeNil())
			Expect(cfg.Terraform).NotTo(BeNil())
			Expect(cfg.Workflow).NotTo(BeNil())
		})
	})

	Describe("DefaultNotificationConfig", func() {
		It("should return notification config with bell validator", func() {
			cfg := DefaultNotificationConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Bell).NotTo(BeNil())
		})
	})

	Describe("DefaultCommitValidatorConfig", func() {
		It("should return commit validator config with correct defaults", func() {
			cfg := DefaultCommitValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.RequiredFlags).To(ContainElements("-s", "-S"))
			Expect(cfg.CheckStagingArea).NotTo(BeNil())
			Expect(*cfg.CheckStagingArea).To(BeTrue())
			Expect(cfg.Message).NotTo(BeNil())
		})
	})

	Describe("DefaultCommitMessageConfig", func() {
		It("should return commit message config with correct defaults", func() {
			cfg := DefaultCommitMessageConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(*cfg.Enabled).To(BeTrue())
			Expect(*cfg.TitleMaxLength).To(Equal(50))
			Expect(*cfg.BodyMaxLineLength).To(Equal(72))
			Expect(*cfg.BodyLineTolerance).To(Equal(5))
			Expect(*cfg.ConventionalCommits).To(BeTrue())
			Expect(*cfg.RequireScope).To(BeTrue())
			Expect(*cfg.BlockInfraScopeMisuse).To(BeTrue())
			Expect(*cfg.BlockPRReferences).To(BeTrue())
			Expect(*cfg.BlockAIAttribution).To(BeTrue())
			Expect(cfg.ValidTypes).To(ContainElements(
				"build", "chore", "ci", "docs", "feat", "fix",
				"perf", "refactor", "revert", "style", "test",
			))
		})
	})

	Describe("DefaultPushValidatorConfig", func() {
		It("should return push validator config with correct defaults", func() {
			cfg := DefaultPushValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.BlockedRemotes).To(BeEmpty())
			Expect(*cfg.RequireTracking).To(BeTrue())
		})
	})

	Describe("DefaultAddValidatorConfig", func() {
		It("should return add validator config with correct defaults", func() {
			cfg := DefaultAddValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.BlockedPatterns).To(ContainElement("tmp/*"))
		})
	})

	Describe("DefaultPRValidatorConfig", func() {
		It("should return PR validator config with correct defaults", func() {
			cfg := DefaultPRValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(*cfg.TitleMaxLength).To(Equal(50))
			Expect(*cfg.TitleConventionalCommits).To(BeTrue())
			Expect(*cfg.RequireChangelog).To(BeFalse())
			Expect(*cfg.CheckCILabels).To(BeTrue())
			Expect(*cfg.RequireBody).To(BeTrue())
			Expect(cfg.ValidTypes).To(HaveLen(11))
			Expect(cfg.MarkdownDisabledRules).To(ContainElements("MD013", "MD034", "MD041"))
		})
	})

	Describe("DefaultBranchValidatorConfig", func() {
		It("should return branch validator config with correct defaults", func() {
			cfg := DefaultBranchValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.ProtectedBranches).To(ContainElements("main", "master"))
			Expect(*cfg.RequireType).To(BeTrue())
			Expect(*cfg.AllowUppercase).To(BeFalse())
			Expect(cfg.ValidTypes).To(HaveLen(10))
		})
	})

	Describe("DefaultNoVerifyValidatorConfig", func() {
		It("should return no-verify validator config with correct defaults", func() {
			cfg := DefaultNoVerifyValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
		})
	})

	Describe("DefaultMarkdownValidatorConfig", func() {
		It("should return markdown validator config with correct defaults", func() {
			cfg := DefaultMarkdownValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.Timeout.ToDuration()).To(Equal(10 * time.Second))
			Expect(*cfg.ContextLines).To(Equal(2))
			Expect(*cfg.HeadingSpacing).To(BeTrue())
			Expect(*cfg.CodeBlockFormatting).To(BeTrue())
			Expect(*cfg.ListFormatting).To(BeTrue())
			Expect(*cfg.UseMarkdownlint).To(BeTrue())
			Expect(cfg.MarkdownlintRules).To(HaveKeyWithValue("MD013", false))
			Expect(cfg.MarkdownlintRules).To(HaveKeyWithValue("MD034", false))
		})
	})

	Describe("DefaultShellScriptValidatorConfig", func() {
		It("should return shell script validator config with correct defaults", func() {
			cfg := DefaultShellScriptValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.Timeout.ToDuration()).To(Equal(10 * time.Second))
			Expect(*cfg.ContextLines).To(Equal(2))
			Expect(*cfg.UseShellcheck).To(BeTrue())
			Expect(cfg.ShellcheckSeverity).To(Equal("warning"))
			Expect(cfg.ExcludeRules).To(BeEmpty())
			Expect(cfg.ShellcheckPath).To(BeEmpty())
		})
	})

	Describe("DefaultTerraformValidatorConfig", func() {
		It("should return terraform validator config with correct defaults", func() {
			cfg := DefaultTerraformValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.Timeout.ToDuration()).To(Equal(10 * time.Second))
			Expect(*cfg.ContextLines).To(Equal(2))
			Expect(cfg.ToolPreference).To(Equal("auto"))
			Expect(*cfg.CheckFormat).To(BeTrue())
			Expect(*cfg.UseTflint).To(BeTrue())
			Expect(cfg.TerraformPath).To(BeEmpty())
			Expect(cfg.TofuPath).To(BeEmpty())
			Expect(cfg.TflintPath).To(BeEmpty())
		})
	})

	Describe("DefaultWorkflowValidatorConfig", func() {
		It("should return workflow validator config with correct defaults", func() {
			cfg := DefaultWorkflowValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.Timeout.ToDuration()).To(Equal(10 * time.Second))
			Expect(cfg.GHAPITimeout.ToDuration()).To(Equal(5 * time.Second))
			Expect(*cfg.EnforceDigestPinning).To(BeTrue())
			Expect(*cfg.RequireVersionComment).To(BeTrue())
			Expect(*cfg.CheckLatestVersion).To(BeTrue())
			Expect(*cfg.UseActionlint).To(BeTrue())
			Expect(cfg.ActionlintPath).To(BeEmpty())
		})
	})

	Describe("DefaultBellValidatorConfig", func() {
		It("should return bell validator config with correct defaults", func() {
			cfg := DefaultBellValidatorConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IsEnabled()).To(BeTrue())
			Expect(cfg.Severity).To(Equal(config.SeverityError))
			Expect(cfg.CustomCommand).To(BeEmpty())
		})
	})
})
