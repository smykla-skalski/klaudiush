// Package config provides internal configuration loading and processing.
package config

import (
	"github.com/smykla-labs/klaudiush/pkg/config"
)

// Merger handles deep merging of configurations.
// Merging follows these rules:
// - Non-nil pointer fields override nil fields
// - Non-empty slices override empty slices
// - Non-zero values override zero values
// - String fields: non-empty overrides empty
// - Duration fields: non-zero overrides zero
type Merger struct{}

// NewMerger creates a new Merger.
func NewMerger() *Merger {
	return &Merger{}
}

// Merge merges multiple configurations in order of precedence (last wins).
// Returns a new configuration that is the result of the merge.
func (m *Merger) Merge(configs ...*config.Config) *config.Config {
	if len(configs) == 0 {
		return &config.Config{}
	}

	result := &config.Config{}

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		m.mergeConfig(result, cfg)
	}

	return result
}

// mergeConfig merges src into dst (dst is modified in place).
func (m *Merger) mergeConfig(dst, src *config.Config) {
	// Merge global config
	if src.Global != nil {
		if dst.Global == nil {
			dst.Global = &config.GlobalConfig{}
		}

		m.mergeGlobalConfig(dst.Global, src.Global)
	}

	// Merge validators config
	if src.Validators != nil {
		if dst.Validators == nil {
			dst.Validators = &config.ValidatorsConfig{}
		}

		m.mergeValidatorsConfig(dst.Validators, src.Validators)
	}
}

// mergeGlobalConfig merges global configurations.
func (*Merger) mergeGlobalConfig(dst, src *config.GlobalConfig) {
	if src.UseSDKGit != nil {
		dst.UseSDKGit = src.UseSDKGit
	}

	if src.DefaultTimeout != 0 {
		dst.DefaultTimeout = src.DefaultTimeout
	}
}

// mergeValidatorsConfig merges validator configurations.
func (m *Merger) mergeValidatorsConfig(dst, src *config.ValidatorsConfig) {
	// Merge git validators
	if src.Git != nil {
		if dst.Git == nil {
			dst.Git = &config.GitConfig{}
		}

		m.mergeGitConfig(dst.Git, src.Git)
	}

	// Merge file validators
	if src.File != nil {
		if dst.File == nil {
			dst.File = &config.FileConfig{}
		}

		m.mergeFileConfig(dst.File, src.File)
	}

	// Merge notification validators
	if src.Notification != nil {
		if dst.Notification == nil {
			dst.Notification = &config.NotificationConfig{}
		}

		m.mergeNotificationConfig(dst.Notification, src.Notification)
	}
}

// mergeGitConfig merges git validator configurations.
func (m *Merger) mergeGitConfig(dst, src *config.GitConfig) {
	if src.Commit != nil {
		if dst.Commit == nil {
			dst.Commit = &config.CommitValidatorConfig{}
		}

		m.mergeCommitConfig(dst.Commit, src.Commit)
	}

	if src.Push != nil {
		if dst.Push == nil {
			dst.Push = &config.PushValidatorConfig{}
		}

		m.mergePushConfig(dst.Push, src.Push)
	}

	if src.Add != nil {
		if dst.Add == nil {
			dst.Add = &config.AddValidatorConfig{}
		}

		m.mergeAddConfig(dst.Add, src.Add)
	}

	if src.PR != nil {
		if dst.PR == nil {
			dst.PR = &config.PRValidatorConfig{}
		}

		m.mergePRConfig(dst.PR, src.PR)
	}

	if src.Branch != nil {
		if dst.Branch == nil {
			dst.Branch = &config.BranchValidatorConfig{}
		}

		m.mergeBranchConfig(dst.Branch, src.Branch)
	}

	if src.NoVerify != nil {
		if dst.NoVerify == nil {
			dst.NoVerify = &config.NoVerifyValidatorConfig{}
		}

		m.mergeBaseConfig(&dst.NoVerify.ValidatorConfig, &src.NoVerify.ValidatorConfig)
	}
}

// mergeFileConfig merges file validator configurations.
func (m *Merger) mergeFileConfig(dst, src *config.FileConfig) {
	if src.Markdown != nil {
		if dst.Markdown == nil {
			dst.Markdown = &config.MarkdownValidatorConfig{}
		}

		m.mergeMarkdownConfig(dst.Markdown, src.Markdown)
	}

	if src.ShellScript != nil {
		if dst.ShellScript == nil {
			dst.ShellScript = &config.ShellScriptValidatorConfig{}
		}

		m.mergeShellScriptConfig(dst.ShellScript, src.ShellScript)
	}

	if src.Terraform != nil {
		if dst.Terraform == nil {
			dst.Terraform = &config.TerraformValidatorConfig{}
		}

		m.mergeTerraformConfig(dst.Terraform, src.Terraform)
	}

	if src.Workflow != nil {
		if dst.Workflow == nil {
			dst.Workflow = &config.WorkflowValidatorConfig{}
		}

		m.mergeWorkflowConfig(dst.Workflow, src.Workflow)
	}
}

// mergeNotificationConfig merges notification validator configurations.
func (m *Merger) mergeNotificationConfig(dst, src *config.NotificationConfig) {
	if src.Bell != nil {
		if dst.Bell == nil {
			dst.Bell = &config.BellValidatorConfig{}
		}

		m.mergeBellConfig(dst.Bell, src.Bell)
	}
}

// mergeCommitConfig merges commit validator configurations.
func (m *Merger) mergeCommitConfig(dst, src *config.CommitValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if len(src.RequiredFlags) > 0 {
		dst.RequiredFlags = src.RequiredFlags
	}

	if src.CheckStagingArea != nil {
		dst.CheckStagingArea = src.CheckStagingArea
	}

	if src.Message != nil {
		if dst.Message == nil {
			dst.Message = &config.CommitMessageConfig{}
		}

		m.mergeCommitMessageConfig(dst.Message, src.Message)
	}
}

// mergeCommitMessageConfig merges commit message configurations.
func (*Merger) mergeCommitMessageConfig(dst, src *config.CommitMessageConfig) {
	if src.Enabled != nil {
		dst.Enabled = src.Enabled
	}

	if src.TitleMaxLength != nil {
		dst.TitleMaxLength = src.TitleMaxLength
	}

	if src.BodyMaxLineLength != nil {
		dst.BodyMaxLineLength = src.BodyMaxLineLength
	}

	if src.BodyLineTolerance != nil {
		dst.BodyLineTolerance = src.BodyLineTolerance
	}

	if src.ConventionalCommits != nil {
		dst.ConventionalCommits = src.ConventionalCommits
	}

	if len(src.ValidTypes) > 0 {
		dst.ValidTypes = src.ValidTypes
	}

	if src.RequireScope != nil {
		dst.RequireScope = src.RequireScope
	}

	if src.BlockInfraScopeMisuse != nil {
		dst.BlockInfraScopeMisuse = src.BlockInfraScopeMisuse
	}

	if src.BlockPRReferences != nil {
		dst.BlockPRReferences = src.BlockPRReferences
	}

	if src.BlockAIAttribution != nil {
		dst.BlockAIAttribution = src.BlockAIAttribution
	}

	if src.ExpectedSignoff != "" {
		dst.ExpectedSignoff = src.ExpectedSignoff
	}
}

// mergePushConfig merges push validator configurations.
func (m *Merger) mergePushConfig(dst, src *config.PushValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if len(src.BlockedRemotes) > 0 {
		dst.BlockedRemotes = src.BlockedRemotes
	}

	if src.RequireTracking != nil {
		dst.RequireTracking = src.RequireTracking
	}
}

// mergeAddConfig merges add validator configurations.
func (m *Merger) mergeAddConfig(dst, src *config.AddValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if len(src.BlockedPatterns) > 0 {
		dst.BlockedPatterns = src.BlockedPatterns
	}
}

// mergePRConfig merges PR validator configurations.
func (m *Merger) mergePRConfig(dst, src *config.PRValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if src.TitleMaxLength != nil {
		dst.TitleMaxLength = src.TitleMaxLength
	}

	if src.TitleConventionalCommits != nil {
		dst.TitleConventionalCommits = src.TitleConventionalCommits
	}

	if len(src.ValidTypes) > 0 {
		dst.ValidTypes = src.ValidTypes
	}

	if src.RequireChangelog != nil {
		dst.RequireChangelog = src.RequireChangelog
	}

	if src.CheckCILabels != nil {
		dst.CheckCILabels = src.CheckCILabels
	}

	if src.RequireBody != nil {
		dst.RequireBody = src.RequireBody
	}

	if len(src.MarkdownDisabledRules) > 0 {
		dst.MarkdownDisabledRules = src.MarkdownDisabledRules
	}
}

// mergeBranchConfig merges branch validator configurations.
func (m *Merger) mergeBranchConfig(dst, src *config.BranchValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if len(src.ProtectedBranches) > 0 {
		dst.ProtectedBranches = src.ProtectedBranches
	}

	if len(src.ValidTypes) > 0 {
		dst.ValidTypes = src.ValidTypes
	}

	if src.RequireType != nil {
		dst.RequireType = src.RequireType
	}

	if src.AllowUppercase != nil {
		dst.AllowUppercase = src.AllowUppercase
	}
}

// mergeMarkdownConfig merges markdown validator configurations.
func (m *Merger) mergeMarkdownConfig(dst, src *config.MarkdownValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if src.Timeout != 0 {
		dst.Timeout = src.Timeout
	}

	if src.ContextLines != nil {
		dst.ContextLines = src.ContextLines
	}

	if src.HeadingSpacing != nil {
		dst.HeadingSpacing = src.HeadingSpacing
	}

	if src.CodeBlockFormatting != nil {
		dst.CodeBlockFormatting = src.CodeBlockFormatting
	}

	if src.ListFormatting != nil {
		dst.ListFormatting = src.ListFormatting
	}

	if src.UseMarkdownlint != nil {
		dst.UseMarkdownlint = src.UseMarkdownlint
	}
}

// mergeShellScriptConfig merges shell script validator configurations.
func (m *Merger) mergeShellScriptConfig(dst, src *config.ShellScriptValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if src.Timeout != 0 {
		dst.Timeout = src.Timeout
	}

	if src.ContextLines != nil {
		dst.ContextLines = src.ContextLines
	}

	if src.UseShellcheck != nil {
		dst.UseShellcheck = src.UseShellcheck
	}

	if src.ShellcheckSeverity != "" {
		dst.ShellcheckSeverity = src.ShellcheckSeverity
	}

	if len(src.ExcludeRules) > 0 {
		dst.ExcludeRules = src.ExcludeRules
	}

	if src.ShellcheckPath != "" {
		dst.ShellcheckPath = src.ShellcheckPath
	}
}

// mergeTerraformConfig merges terraform validator configurations.
func (m *Merger) mergeTerraformConfig(dst, src *config.TerraformValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if src.Timeout != 0 {
		dst.Timeout = src.Timeout
	}

	if src.ContextLines != nil {
		dst.ContextLines = src.ContextLines
	}

	if src.ToolPreference != "" {
		dst.ToolPreference = src.ToolPreference
	}

	if src.CheckFormat != nil {
		dst.CheckFormat = src.CheckFormat
	}

	if src.UseTflint != nil {
		dst.UseTflint = src.UseTflint
	}

	if src.TerraformPath != "" {
		dst.TerraformPath = src.TerraformPath
	}

	if src.TofuPath != "" {
		dst.TofuPath = src.TofuPath
	}

	if src.TflintPath != "" {
		dst.TflintPath = src.TflintPath
	}
}

// mergeWorkflowConfig merges workflow validator configurations.
func (m *Merger) mergeWorkflowConfig(dst, src *config.WorkflowValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if src.Timeout != 0 {
		dst.Timeout = src.Timeout
	}

	if src.GHAPITimeout != 0 {
		dst.GHAPITimeout = src.GHAPITimeout
	}

	if src.EnforceDigestPinning != nil {
		dst.EnforceDigestPinning = src.EnforceDigestPinning
	}

	if src.RequireVersionComment != nil {
		dst.RequireVersionComment = src.RequireVersionComment
	}

	if src.CheckLatestVersion != nil {
		dst.CheckLatestVersion = src.CheckLatestVersion
	}

	if src.UseActionlint != nil {
		dst.UseActionlint = src.UseActionlint
	}

	if src.ActionlintPath != "" {
		dst.ActionlintPath = src.ActionlintPath
	}
}

// mergeBellConfig merges bell validator configurations.
func (m *Merger) mergeBellConfig(dst, src *config.BellValidatorConfig) {
	m.mergeBaseConfig(&dst.ValidatorConfig, &src.ValidatorConfig)

	if src.CustomCommand != "" {
		dst.CustomCommand = src.CustomCommand
	}
}

// mergeBaseConfig merges base validator configurations.
func (*Merger) mergeBaseConfig(dst, src *config.ValidatorConfig) {
	if src.Enabled != nil {
		dst.Enabled = src.Enabled
	}

	if src.Severity != "" {
		dst.Severity = src.Severity
	}
}
