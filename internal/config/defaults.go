// Package config provides internal configuration loading and processing.
package config

import (
	"time"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

const (
	// DefaultTimeout is the default timeout for operations.
	DefaultTimeout = 10 * time.Second

	// DefaultGHAPITimeout is the default timeout for GitHub API calls.
	DefaultGHAPITimeout = 5 * time.Second
)

// DefaultConfig returns a Config with all default values populated.
func DefaultConfig() *config.Config {
	return &config.Config{
		Global:     DefaultGlobalConfig(),
		Validators: DefaultValidatorsConfig(),
		Rules:      DefaultRulesConfig(),
	}
}

// DefaultRulesConfig returns the default rules configuration.
// Rules are enabled by default but no rules are pre-defined.
// Users can add rules in project or global configuration.
func DefaultRulesConfig() *config.RulesConfig {
	enabled := true
	stopOnFirstMatch := true

	return &config.RulesConfig{
		Enabled:          &enabled,
		StopOnFirstMatch: &stopOnFirstMatch,
		Rules:            []config.RuleConfig{},
	}
}

// DefaultGlobalConfig returns the default global configuration.
func DefaultGlobalConfig() *config.GlobalConfig {
	useSDKGit := true

	return &config.GlobalConfig{
		UseSDKGit:      &useSDKGit,
		DefaultTimeout: config.Duration(DefaultTimeout),
	}
}

// DefaultValidatorsConfig returns the default validators configuration.
func DefaultValidatorsConfig() *config.ValidatorsConfig {
	return &config.ValidatorsConfig{
		Git:          DefaultGitConfig(),
		GitHub:       DefaultGitHubConfig(),
		File:         DefaultFileConfig(),
		Notification: DefaultNotificationConfig(),
	}
}

// DefaultGitConfig returns the default git validators configuration.
func DefaultGitConfig() *config.GitConfig {
	return &config.GitConfig{
		Commit:   DefaultCommitValidatorConfig(),
		Push:     DefaultPushValidatorConfig(),
		Fetch:    DefaultFetchValidatorConfig(),
		Add:      DefaultAddValidatorConfig(),
		PR:       DefaultPRValidatorConfig(),
		Branch:   DefaultBranchValidatorConfig(),
		NoVerify: DefaultNoVerifyValidatorConfig(),
	}
}

// DefaultGitHubConfig returns the default GitHub CLI validators configuration.
func DefaultGitHubConfig() *config.GitHubConfig {
	return &config.GitHubConfig{
		Issue: DefaultIssueValidatorConfig(),
	}
}

// DefaultIssueValidatorConfig returns the default issue validator configuration.
func DefaultIssueValidatorConfig() *config.IssueValidatorConfig {
	enabled := true
	requireBody := false

	return &config.IssueValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityWarning, // Default: warning only, don't block
		},
		RequireBody: &requireBody,
		MarkdownDisabledRules: []string{
			"MD013", // Line length
			"MD034", // Bare URLs
			"MD041", // First line heading
			"MD047", // Trailing newline
		},
		Timeout: config.Duration(DefaultTimeout),
	}
}

// DefaultFileConfig returns the default file validators configuration.
func DefaultFileConfig() *config.FileConfig {
	return &config.FileConfig{
		Markdown:    DefaultMarkdownValidatorConfig(),
		ShellScript: DefaultShellScriptValidatorConfig(),
		Terraform:   DefaultTerraformValidatorConfig(),
		Workflow:    DefaultWorkflowValidatorConfig(),
		Python:      DefaultPythonValidatorConfig(),
	}
}

// DefaultNotificationConfig returns the default notification validators configuration.
func DefaultNotificationConfig() *config.NotificationConfig {
	return &config.NotificationConfig{
		Bell: DefaultBellValidatorConfig(),
	}
}

// DefaultCommitValidatorConfig returns the default commit validator configuration.
func DefaultCommitValidatorConfig() *config.CommitValidatorConfig {
	enabled := true
	checkStagingArea := true

	return &config.CommitValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		RequiredFlags:    []string{"-s", "-S"},
		CheckStagingArea: &checkStagingArea,
		Message:          DefaultCommitMessageConfig(),
	}
}

// DefaultCommitMessageConfig returns the default commit message configuration.
func DefaultCommitMessageConfig() *config.CommitMessageConfig {
	enabled := true
	titleMaxLength := 50
	bodyMaxLineLength := 72
	bodyLineTolerance := 5
	conventionalCommits := true
	requireScope := true
	blockInfraScopeMisuse := true
	blockPRReferences := true
	blockAIAttribution := true

	return &config.CommitMessageConfig{
		Enabled:               &enabled,
		TitleMaxLength:        &titleMaxLength,
		BodyMaxLineLength:     &bodyMaxLineLength,
		BodyLineTolerance:     &bodyLineTolerance,
		ConventionalCommits:   &conventionalCommits,
		RequireScope:          &requireScope,
		BlockInfraScopeMisuse: &blockInfraScopeMisuse,
		BlockPRReferences:     &blockPRReferences,
		BlockAIAttribution:    &blockAIAttribution,
		ValidTypes: []string{
			"build",
			"chore",
			"ci",
			"docs",
			"feat",
			"fix",
			"perf",
			"refactor",
			"revert",
			"style",
			"test",
		},
		ExpectedSignoff: "",
	}
}

// DefaultPushValidatorConfig returns the default push validator configuration.
func DefaultPushValidatorConfig() *config.PushValidatorConfig {
	enabled := true
	requireTracking := true

	return &config.PushValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		BlockedRemotes:  []string{},
		RequireTracking: &requireTracking,
	}
}

// DefaultFetchValidatorConfig returns the default fetch validator configuration.
func DefaultFetchValidatorConfig() *config.FetchValidatorConfig {
	enabled := true

	return &config.FetchValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
	}
}

// DefaultAddValidatorConfig returns the default add validator configuration.
func DefaultAddValidatorConfig() *config.AddValidatorConfig {
	enabled := true

	return &config.AddValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		BlockedPatterns: []string{"tmp/*"},
	}
}

// DefaultPRValidatorConfig returns the default PR validator configuration.
func DefaultPRValidatorConfig() *config.PRValidatorConfig {
	enabled := true
	titleMaxLength := 50
	titleConventionalCommits := true
	requireChangelog := false
	checkCILabels := true
	requireBody := true

	return &config.PRValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		TitleMaxLength:           &titleMaxLength,
		TitleConventionalCommits: &titleConventionalCommits,
		RequireChangelog:         &requireChangelog,
		CheckCILabels:            &checkCILabels,
		RequireBody:              &requireBody,
		ValidTypes: []string{
			"build",
			"chore",
			"ci",
			"docs",
			"feat",
			"fix",
			"perf",
			"refactor",
			"revert",
			"style",
			"test",
		},
		MarkdownDisabledRules: []string{"MD013", "MD034", "MD041"},
	}
}

// DefaultBranchValidatorConfig returns the default branch validator configuration.
func DefaultBranchValidatorConfig() *config.BranchValidatorConfig {
	enabled := true
	requireType := true
	allowUppercase := false

	return &config.BranchValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		ProtectedBranches: []string{"main", "master"},
		RequireType:       &requireType,
		AllowUppercase:    &allowUppercase,
		ValidTypes: []string{
			"build",
			"chore",
			"ci",
			"docs",
			"feat",
			"fix",
			"perf",
			"refactor",
			"style",
			"test",
		},
	}
}

// DefaultNoVerifyValidatorConfig returns the default no-verify validator configuration.
func DefaultNoVerifyValidatorConfig() *config.NoVerifyValidatorConfig {
	enabled := true

	return &config.NoVerifyValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
	}
}

// DefaultMarkdownValidatorConfig returns the default markdown validator configuration.
func DefaultMarkdownValidatorConfig() *config.MarkdownValidatorConfig {
	enabled := true
	contextLines := 2
	headingSpacing := true
	codeBlockFormatting := true
	listFormatting := true
	useMarkdownlint := true

	return &config.MarkdownValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		Timeout:             config.Duration(DefaultTimeout),
		ContextLines:        &contextLines,
		HeadingSpacing:      &headingSpacing,
		CodeBlockFormatting: &codeBlockFormatting,
		ListFormatting:      &listFormatting,
		UseMarkdownlint:     &useMarkdownlint,
		MarkdownlintRules: map[string]bool{
			"MD013": false, // line-length disabled by default
			"MD034": false, // bare URLs disabled by default (common in code blocks)
		},
	}
}

// DefaultShellScriptValidatorConfig returns the default shell script validator configuration.
func DefaultShellScriptValidatorConfig() *config.ShellScriptValidatorConfig {
	enabled := true
	contextLines := 2
	useShellcheck := true

	return &config.ShellScriptValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		Timeout:            config.Duration(DefaultTimeout),
		ContextLines:       &contextLines,
		UseShellcheck:      &useShellcheck,
		ShellcheckSeverity: "warning",
		ExcludeRules:       []string{},
		ShellcheckPath:     "",
	}
}

// DefaultTerraformValidatorConfig returns the default terraform validator configuration.
func DefaultTerraformValidatorConfig() *config.TerraformValidatorConfig {
	enabled := true
	contextLines := 2
	checkFormat := true
	useTflint := true

	return &config.TerraformValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		Timeout:        config.Duration(DefaultTimeout),
		ContextLines:   &contextLines,
		ToolPreference: "auto",
		CheckFormat:    &checkFormat,
		UseTflint:      &useTflint,
		TerraformPath:  "",
		TofuPath:       "",
		TflintPath:     "",
	}
}

// DefaultWorkflowValidatorConfig returns the default workflow validator configuration.
func DefaultWorkflowValidatorConfig() *config.WorkflowValidatorConfig {
	enabled := true
	enforceDigestPinning := true
	requireVersionComment := true
	checkLatestVersion := true
	useActionlint := true

	return &config.WorkflowValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		Timeout:               config.Duration(DefaultTimeout),
		GHAPITimeout:          config.Duration(DefaultGHAPITimeout),
		EnforceDigestPinning:  &enforceDigestPinning,
		RequireVersionComment: &requireVersionComment,
		CheckLatestVersion:    &checkLatestVersion,
		UseActionlint:         &useActionlint,
		ActionlintPath:        "",
	}
}

// DefaultPythonValidatorConfig returns the default Python validator configuration.
func DefaultPythonValidatorConfig() *config.PythonValidatorConfig {
	enabled := true
	contextLines := 2
	useRuff := true

	return &config.PythonValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		Timeout:      config.Duration(DefaultTimeout),
		ContextLines: &contextLines,
		UseRuff:      &useRuff,
		RuffPath:     "",
		ExcludeRules: []string{},
		RuffConfig:   "",
	}
}

// DefaultBellValidatorConfig returns the default bell validator configuration.
func DefaultBellValidatorConfig() *config.BellValidatorConfig {
	enabled := true

	return &config.BellValidatorConfig{
		ValidatorConfig: config.ValidatorConfig{
			Enabled:  &enabled,
			Severity: config.SeverityError,
		},
		CustomCommand: "",
	}
}
