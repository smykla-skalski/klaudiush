// Package config provides configuration schema types for klaudiush validators.
package config

// GitConfig groups all git-related validator configurations.
type GitConfig struct {
	// Commit validator configuration
	Commit *CommitValidatorConfig `json:"commit,omitempty" toml:"commit"`

	// Push validator configuration
	Push *PushValidatorConfig `json:"push,omitempty" toml:"push"`

	// Add validator configuration
	Add *AddValidatorConfig `json:"add,omitempty" toml:"add"`

	// PR validator configuration
	PR *PRValidatorConfig `json:"pr,omitempty" toml:"pr"`

	// Branch validator configuration
	Branch *BranchValidatorConfig `json:"branch,omitempty" toml:"branch"`

	// NoVerify validator configuration
	NoVerify *NoVerifyValidatorConfig `json:"no_verify,omitempty" toml:"no_verify"`
}

// CommitValidatorConfig configures the git commit validator.
type CommitValidatorConfig struct {
	ValidatorConfig

	// RequiredFlags are the flags that must be present in commit commands.
	// Default: ["-s", "-S"]
	RequiredFlags []string `json:"required_flags,omitempty" toml:"required_flags"`

	// CheckStagingArea enables checking that files are staged before commit.
	// Default: true
	CheckStagingArea *bool `json:"check_staging_area,omitempty" toml:"check_staging_area"`

	// Message contains commit message validation settings.
	Message *CommitMessageConfig `json:"message,omitempty" toml:"message"`
}

// CommitMessageConfig configures commit message validation rules.
type CommitMessageConfig struct {
	// Enabled controls whether commit message validation is performed.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" toml:"enabled"`

	// TitleMaxLength is the maximum allowed length for the commit title.
	// Default: 50
	TitleMaxLength *int `json:"title_max_length,omitempty" toml:"title_max_length"`

	// BodyMaxLineLength is the maximum allowed length for body lines.
	// Default: 72
	BodyMaxLineLength *int `json:"body_max_line_length,omitempty" toml:"body_max_line_length"`

	// BodyLineTolerance allows body lines to exceed max length by this amount.
	// Default: 5 (total: 77 characters)
	BodyLineTolerance *int `json:"body_line_tolerance,omitempty" toml:"body_line_tolerance"`

	// ConventionalCommits enforces conventional commit format (type(scope): description).
	// Default: true
	ConventionalCommits *bool `json:"conventional_commits,omitempty" toml:"conventional_commits"`

	// ValidTypes is the list of valid commit types.
	// Default: ["build", "chore", "ci", "docs", "feat", "fix", "perf", "refactor", "revert", "style", "test"]
	ValidTypes []string `json:"valid_types,omitempty" toml:"valid_types"`

	// RequireScope enforces that conventional commits must have a scope.
	// Default: true
	RequireScope *bool `json:"require_scope,omitempty" toml:"require_scope"`

	// BlockInfraScopeMisuse blocks feat/fix with infrastructure scopes (ci, test, docs, build).
	// Default: true
	BlockInfraScopeMisuse *bool `json:"block_infra_scope_misuse,omitempty" toml:"block_infra_scope_misuse"`

	// BlockPRReferences blocks PR references (#123 or GitHub URLs) in commit messages.
	// Default: true
	BlockPRReferences *bool `json:"block_pr_references,omitempty" toml:"block_pr_references"`

	// BlockAIAttribution blocks Claude AI attribution in commit messages.
	// Default: true
	BlockAIAttribution *bool `json:"block_ai_attribution,omitempty" toml:"block_ai_attribution"`

	// ExpectedSignoff is the expected Signed-off-by trailer value.
	// When set, commits with Signed-off-by trailers must match this exactly.
	// Format: "Name <email@example.com>"
	// Default: "" (no signoff validation)
	ExpectedSignoff string `json:"expected_signoff,omitempty" toml:"expected_signoff"`
}

// PushValidatorConfig configures the git push validator.
type PushValidatorConfig struct {
	ValidatorConfig

	// BlockedRemotes is a list of remote names that are not allowed for push operations.
	// Default: []
	BlockedRemotes []string `json:"blocked_remotes,omitempty" toml:"blocked_remotes"`

	// RequireTracking requires branches to have remote tracking configured before push.
	// Default: true
	RequireTracking *bool `json:"require_tracking,omitempty" toml:"require_tracking"`
}

// AddValidatorConfig configures the git add validator.
type AddValidatorConfig struct {
	ValidatorConfig

	// BlockedPatterns is a list of file path patterns that should not be added to git.
	// Patterns use filepath.Match syntax (e.g., "tmp/*", "*.secret").
	// Default: ["tmp/*"]
	BlockedPatterns []string `json:"blocked_patterns,omitempty" toml:"blocked_patterns"`
}

// PRValidatorConfig configures the GitHub PR (gh pr create) validator.
type PRValidatorConfig struct {
	ValidatorConfig

	// TitleMaxLength is the maximum allowed length for PR titles.
	// Default: 50
	TitleMaxLength *int `json:"title_max_length,omitempty" toml:"title_max_length"`

	// TitleConventionalCommits enforces conventional commit format for PR titles.
	// Default: true
	TitleConventionalCommits *bool `json:"title_conventional_commits,omitempty" toml:"title_conventional_commits"`

	// ValidTypes is the list of valid commit types for PR titles.
	// Default: same as commit message valid types
	ValidTypes []string `json:"valid_types,omitempty" toml:"valid_types"`

	// RequireChangelog requires a "> Changelog:" line in the PR body.
	// Default: false (changelog line is optional, PR title is used if omitted)
	RequireChangelog *bool `json:"require_changelog,omitempty" toml:"require_changelog"`

	// CheckCILabels enables checking for ci/ labels and providing suggestions.
	// Default: true
	CheckCILabels *bool `json:"check_ci_labels,omitempty" toml:"check_ci_labels"`

	// RequireBody requires PR body to be present.
	// Default: true
	RequireBody *bool `json:"require_body,omitempty" toml:"require_body"`

	// MarkdownDisabledRules is a list of markdownlint rules to disable for PR body validation.
	// Default: ["MD013", "MD034", "MD041"]
	MarkdownDisabledRules []string `json:"markdown_disabled_rules,omitempty" toml:"markdown_disabled_rules"`
}

// BranchValidatorConfig configures the git branch name validator.
type BranchValidatorConfig struct {
	ValidatorConfig

	// ProtectedBranches is a list of branch names that skip validation.
	// Default: ["main", "master"]
	ProtectedBranches []string `json:"protected_branches,omitempty" toml:"protected_branches"`

	// ValidTypes is the list of valid branch type prefixes.
	// Default: ["feat", "fix", "docs", "style", "refactor", "test", "chore", "ci", "build", "perf"]
	ValidTypes []string `json:"valid_types,omitempty" toml:"valid_types"`

	// RequireType requires branches to follow type/description format.
	// Default: true
	RequireType *bool `json:"require_type,omitempty" toml:"require_type"`

	// AllowUppercase allows uppercase letters in branch names.
	// Default: false
	AllowUppercase *bool `json:"allow_uppercase,omitempty" toml:"allow_uppercase"`
}

// NoVerifyValidatorConfig configures the git commit --no-verify validator.
type NoVerifyValidatorConfig struct {
	ValidatorConfig

	// No additional configuration beyond base ValidatorConfig
	// This validator blocks --no-verify flag on git commit commands
}
