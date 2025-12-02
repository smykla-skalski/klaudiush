// Package config provides configuration schema types for klaudiush validators.
package config

// GitConfig groups all git-related validator configurations.
type GitConfig struct {
	// Commit validator configuration
	Commit *CommitValidatorConfig `json:"commit,omitempty" koanf:"commit" toml:"commit"`

	// Push validator configuration
	Push *PushValidatorConfig `json:"push,omitempty" koanf:"push" toml:"push"`

	// Fetch validator configuration
	Fetch *FetchValidatorConfig `json:"fetch,omitempty" koanf:"fetch" toml:"fetch"`

	// Add validator configuration
	Add *AddValidatorConfig `json:"add,omitempty" koanf:"add" toml:"add"`

	// PR validator configuration
	PR *PRValidatorConfig `json:"pr,omitempty" koanf:"pr" toml:"pr"`

	// Merge validator configuration
	Merge *MergeValidatorConfig `json:"merge,omitempty" koanf:"merge" toml:"merge"`

	// Branch validator configuration
	Branch *BranchValidatorConfig `json:"branch,omitempty" koanf:"branch" toml:"branch"`

	// NoVerify validator configuration
	NoVerify *NoVerifyValidatorConfig `json:"no_verify,omitempty" koanf:"no_verify" toml:"no_verify"`
}

// CommitValidatorConfig configures the git commit validator.
type CommitValidatorConfig struct {
	ValidatorConfig

	// RequiredFlags are the flags that must be present in commit commands.
	// Default: ["-s", "-S"]
	RequiredFlags []string `json:"required_flags,omitempty" koanf:"required_flags" toml:"required_flags"`

	// CheckStagingArea enables checking that files are staged before commit.
	// Default: true
	CheckStagingArea *bool `json:"check_staging_area,omitempty" koanf:"check_staging_area" toml:"check_staging_area"`

	// Message contains commit message validation settings.
	Message *CommitMessageConfig `json:"message,omitempty" koanf:"message" toml:"message"`
}

// CommitMessageConfig configures commit message validation rules.
type CommitMessageConfig struct {
	// Enabled controls whether commit message validation is performed.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// TitleMaxLength is the maximum allowed length for the commit title.
	// Default: 50
	TitleMaxLength *int `json:"title_max_length,omitempty" koanf:"title_max_length" toml:"title_max_length"`

	// AllowUnlimitedRevertTitle skips title length validation for revert commits.
	// Revert commits use the format: Revert "original commit title"
	// Since the original title may already be at max length, adding the "Revert" prefix
	// would cause the revert commit to exceed the limit.
	// Default: true
	AllowUnlimitedRevertTitle *bool `json:"allow_unlimited_revert_title,omitempty" koanf:"allow_unlimited_revert_title" toml:"allow_unlimited_revert_title"`

	// BodyMaxLineLength is the maximum allowed length for body lines.
	// Default: 72
	BodyMaxLineLength *int `json:"body_max_line_length,omitempty" koanf:"body_max_line_length" toml:"body_max_line_length"`

	// BodyLineTolerance allows body lines to exceed max length by this amount.
	// Default: 5 (total: 77 characters)
	BodyLineTolerance *int `json:"body_line_tolerance,omitempty" koanf:"body_line_tolerance" toml:"body_line_tolerance"`

	// ConventionalCommits enforces conventional commit format (type(scope): description).
	// Default: true
	ConventionalCommits *bool `json:"conventional_commits,omitempty" koanf:"conventional_commits" toml:"conventional_commits"`

	// ValidTypes is the list of valid commit types.
	// Default: ["build", "chore", "ci", "docs", "feat", "fix", "perf", "refactor", "revert", "style", "test"]
	ValidTypes []string `json:"valid_types,omitempty" koanf:"valid_types" toml:"valid_types"`

	// RequireScope enforces that conventional commits must have a scope.
	// Default: true
	RequireScope *bool `json:"require_scope,omitempty" koanf:"require_scope" toml:"require_scope"`

	// BlockInfraScopeMisuse blocks feat/fix with infrastructure scopes (ci, test, docs, build).
	// Default: true
	BlockInfraScopeMisuse *bool `json:"block_infra_scope_misuse,omitempty" koanf:"block_infra_scope_misuse" toml:"block_infra_scope_misuse"`

	// BlockPRReferences blocks PR references (#123 or GitHub URLs) in commit messages.
	// Default: true
	BlockPRReferences *bool `json:"block_pr_references,omitempty" koanf:"block_pr_references" toml:"block_pr_references"`

	// BlockAIAttribution blocks Claude AI attribution in commit messages.
	// Default: true
	BlockAIAttribution *bool `json:"block_ai_attribution,omitempty" koanf:"block_ai_attribution" toml:"block_ai_attribution"`

	// ForbiddenPatterns is a list of regex patterns that are forbidden in commit messages.
	// Each pattern is a regular expression that will be checked against the entire commit message.
	// Default: ["\\btmp/", "\\btmp\\b"] (blocks mentions of tmp directory)
	ForbiddenPatterns []string `json:"forbidden_patterns,omitempty" koanf:"forbidden_patterns" toml:"forbidden_patterns"`

	// ExpectedSignoff is the expected Signed-off-by trailer value.
	// When set, commits with Signed-off-by trailers must match this exactly.
	// Format: "Name <email@klaudiu.sh>"
	// Default: "" (no signoff validation)
	ExpectedSignoff string `json:"expected_signoff,omitempty" koanf:"expected_signoff" toml:"expected_signoff"`
}

// PushValidatorConfig configures the git push validator.
type PushValidatorConfig struct {
	ValidatorConfig

	// BlockedRemotes is a list of remote names that are not allowed for push operations.
	// When Claude Code tries to push to a blocked remote, the operation will be rejected
	// with a clear error message showing available alternatives from AllowedRemotePriority.
	// Default: []
	BlockedRemotes []string `json:"blocked_remotes,omitempty" koanf:"blocked_remotes" toml:"blocked_remotes"`

	// AllowedRemotePriority defines the priority order of remotes to suggest when a blocked
	// remote is used or when "origin" doesn't exist. The validator will suggest the first
	// remote from this list that exists in the repository.
	// Example: ["origin", "upstream", "fork"] means suggest "origin" first, then "upstream", etc.
	// Default: ["origin", "upstream"]
	AllowedRemotePriority []string `json:"allowed_remote_priority,omitempty" koanf:"allowed_remote_priority" toml:"allowed_remote_priority"`

	// RequireTracking requires branches to have remote tracking configured before push.
	// Default: true
	RequireTracking *bool `json:"require_tracking,omitempty" koanf:"require_tracking" toml:"require_tracking"`
}

// AddValidatorConfig configures the git add validator.
type AddValidatorConfig struct {
	ValidatorConfig

	// BlockedPatterns is a list of file path patterns that should not be added to git.
	// Patterns use filepath.Match syntax (e.g., "tmp/*", "*.secret").
	// Default: ["tmp/*"]
	BlockedPatterns []string `json:"blocked_patterns,omitempty" koanf:"blocked_patterns" toml:"blocked_patterns"`
}

// PRValidatorConfig configures the GitHub PR (gh pr create) validator.
type PRValidatorConfig struct {
	ValidatorConfig

	// TitleMaxLength is the maximum allowed length for PR titles.
	// Default: 50
	TitleMaxLength *int `json:"title_max_length,omitempty" koanf:"title_max_length" toml:"title_max_length"`

	// AllowUnlimitedRevertTitle skips title length validation for revert PRs.
	// Revert PRs use the format: Revert "original PR title"
	// Since the original title may already be at max length, adding the "Revert" prefix
	// would cause the revert PR title to exceed the limit.
	// Default: true
	AllowUnlimitedRevertTitle *bool `json:"allow_unlimited_revert_title,omitempty" koanf:"allow_unlimited_revert_title" toml:"allow_unlimited_revert_title"`

	// TitleConventionalCommits enforces conventional commit format for PR titles.
	// Default: true
	TitleConventionalCommits *bool `json:"title_conventional_commits,omitempty" koanf:"title_conventional_commits" toml:"title_conventional_commits"`

	// ValidTypes is the list of valid commit types for PR titles.
	// Default: same as commit message valid types
	ValidTypes []string `json:"valid_types,omitempty" koanf:"valid_types" toml:"valid_types"`

	// RequireChangelog requires a "> Changelog:" line in the PR body.
	// Default: false (changelog line is optional, PR title is used if omitted)
	RequireChangelog *bool `json:"require_changelog,omitempty" koanf:"require_changelog" toml:"require_changelog"`

	// CheckCILabels enables checking for ci/ labels and providing suggestions.
	// Default: true
	CheckCILabels *bool `json:"check_ci_labels,omitempty" koanf:"check_ci_labels" toml:"check_ci_labels"`

	// RequireBody requires PR body to be present.
	// Default: true
	RequireBody *bool `json:"require_body,omitempty" koanf:"require_body" toml:"require_body"`

	// MarkdownDisabledRules is a list of markdownlint rules to disable for PR body validation.
	// Default: ["MD013", "MD034", "MD041"]
	MarkdownDisabledRules []string `json:"markdown_disabled_rules,omitempty" koanf:"markdown_disabled_rules" toml:"markdown_disabled_rules"`

	// ForbiddenPatterns is a list of regex patterns that are forbidden in PR title and body.
	// Each pattern is a regular expression that will be checked against the PR title and body.
	// Default: ["\\btmp/", "\\btmp\\b"] (blocks mentions of tmp directory)
	ForbiddenPatterns []string `json:"forbidden_patterns,omitempty" koanf:"forbidden_patterns" toml:"forbidden_patterns"`
}

// MergeValidatorConfig configures the gh pr merge validator.
type MergeValidatorConfig struct {
	ValidatorConfig

	// Message contains merge message validation settings.
	// Validates PR title + body format before merge.
	Message *MergeMessageConfig `json:"message,omitempty" koanf:"message" toml:"message"`

	// ValidateAutomerge validates PR before enabling auto-merge.
	// Default: true
	ValidateAutomerge *bool `json:"validate_automerge,omitempty" koanf:"validate_automerge" toml:"validate_automerge"`

	// RequireSignoff requires Signed-off-by trailer in merge commit body.
	// The signoff must be provided via --body flag in gh pr merge command.
	// Default: true
	RequireSignoff *bool `json:"require_signoff,omitempty" koanf:"require_signoff" toml:"require_signoff"`

	// ExpectedSignoff is the expected Signed-off-by trailer value.
	// When set, the merge commit body must contain this exact signoff.
	// Format: "Name <email@klaudiu.sh>"
	// Default: "" (any signoff accepted if RequireSignoff is true)
	ExpectedSignoff string `json:"expected_signoff,omitempty" koanf:"expected_signoff" toml:"expected_signoff"`
}

// MergeMessageConfig configures merge commit message validation rules.
// These rules are applied to the PR title + body which becomes the commit message on squash merge.
type MergeMessageConfig struct {
	// Enabled controls whether merge message validation is performed.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// TitleMaxLength is the maximum allowed length for the PR title (commit title).
	// Default: 50
	TitleMaxLength *int `json:"title_max_length,omitempty" koanf:"title_max_length" toml:"title_max_length"`

	// AllowUnlimitedRevertTitle skips title length validation for revert commits.
	// Default: true
	AllowUnlimitedRevertTitle *bool `json:"allow_unlimited_revert_title,omitempty" koanf:"allow_unlimited_revert_title" toml:"allow_unlimited_revert_title"`

	// BodyMaxLineLength is the maximum allowed length for body lines.
	// Default: 72
	BodyMaxLineLength *int `json:"body_max_line_length,omitempty" koanf:"body_max_line_length" toml:"body_max_line_length"`

	// BodyLineTolerance allows body lines to exceed max length by this amount.
	// Default: 5 (total: 77 characters)
	BodyLineTolerance *int `json:"body_line_tolerance,omitempty" koanf:"body_line_tolerance" toml:"body_line_tolerance"`

	// ConventionalCommits enforces conventional commit format for PR title.
	// Default: true
	ConventionalCommits *bool `json:"conventional_commits,omitempty" koanf:"conventional_commits" toml:"conventional_commits"`

	// ValidTypes is the list of valid commit types for PR titles.
	// Default: ["build", "chore", "ci", "docs", "feat", "fix", "perf", "refactor", "revert", "style", "test"]
	ValidTypes []string `json:"valid_types,omitempty" koanf:"valid_types" toml:"valid_types"`

	// RequireScope enforces that conventional commits must have a scope.
	// Default: true
	RequireScope *bool `json:"require_scope,omitempty" koanf:"require_scope" toml:"require_scope"`

	// BlockInfraScopeMisuse blocks feat/fix with infrastructure scopes (ci, test, docs, build).
	// Default: true
	BlockInfraScopeMisuse *bool `json:"block_infra_scope_misuse,omitempty" koanf:"block_infra_scope_misuse" toml:"block_infra_scope_misuse"`

	// BlockPRReferences blocks PR references (#123 or GitHub URLs) in PR body.
	// Default: true
	BlockPRReferences *bool `json:"block_pr_references,omitempty" koanf:"block_pr_references" toml:"block_pr_references"`

	// BlockAIAttribution blocks Claude AI attribution in PR body.
	// Default: true
	BlockAIAttribution *bool `json:"block_ai_attribution,omitempty" koanf:"block_ai_attribution" toml:"block_ai_attribution"`

	// ForbiddenPatterns is a list of regex patterns that are forbidden in PR body.
	// Default: ["\\btmp/", "\\btmp\\b"]
	ForbiddenPatterns []string `json:"forbidden_patterns,omitempty" koanf:"forbidden_patterns" toml:"forbidden_patterns"`
}

// BranchValidatorConfig configures the git branch name validator.
type BranchValidatorConfig struct {
	ValidatorConfig

	// ProtectedBranches is a list of branch names that skip validation.
	// Default: ["main", "master"]
	ProtectedBranches []string `json:"protected_branches,omitempty" koanf:"protected_branches" toml:"protected_branches"`

	// ValidTypes is the list of valid branch type prefixes.
	// Default: ["feat", "fix", "docs", "style", "refactor", "test", "chore", "ci", "build", "perf"]
	ValidTypes []string `json:"valid_types,omitempty" koanf:"valid_types" toml:"valid_types"`

	// RequireType requires branches to follow type/description format.
	// Default: true
	RequireType *bool `json:"require_type,omitempty" koanf:"require_type" toml:"require_type"`

	// AllowUppercase allows uppercase letters in branch names.
	// Default: false
	AllowUppercase *bool `json:"allow_uppercase,omitempty" koanf:"allow_uppercase" toml:"allow_uppercase"`
}

// NoVerifyValidatorConfig configures the git commit --no-verify validator.
type NoVerifyValidatorConfig struct {
	ValidatorConfig

	// No additional configuration beyond base ValidatorConfig
	// This validator blocks --no-verify flag on git commit commands
}

// FetchValidatorConfig configures the git fetch validator.
type FetchValidatorConfig struct {
	ValidatorConfig

	// No additional configuration beyond base ValidatorConfig
	// This validator checks that the remote exists before fetch
}
