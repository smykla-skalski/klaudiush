// Package config provides configuration schema types for klaudiush validators.
package config

// GitHubConfig groups all GitHub CLI-related validator configurations.
type GitHubConfig struct {
	// Issue validator configuration
	Issue *IssueValidatorConfig `json:"issue,omitempty" koanf:"issue" toml:"issue"`
}

// IssueValidatorConfig configures the gh issue create validator.
type IssueValidatorConfig struct {
	ValidatorConfig

	// RequireBody requires issue body to be present.
	// Default: false (body is optional for issues)
	RequireBody *bool `json:"require_body,omitempty" koanf:"require_body" toml:"require_body"`

	// MarkdownDisabledRules is a list of markdownlint rules to disable for issue body validation.
	// Default: ["MD013", "MD034", "MD041", "MD047"]
	// - MD013: Line length (issues often have long lines)
	// - MD034: Bare URLs (commonly used in issues)
	// - MD041: First line heading (issues often start with ### headings)
	// - MD047: Files should end with newline (gh CLI handles this)
	MarkdownDisabledRules []string `json:"markdown_disabled_rules,omitempty" koanf:"markdown_disabled_rules" toml:"markdown_disabled_rules"`

	// Timeout for markdown linting operations.
	// Default: 10s
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`
}
