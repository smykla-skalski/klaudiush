// Package config provides configuration schema types for klaudiush validators.
package config

// FileConfig groups all file-related validator configurations.
type FileConfig struct {
	// Markdown validator configuration
	Markdown *MarkdownValidatorConfig `json:"markdown,omitempty" koanf:"markdown" toml:"markdown"`

	// ShellScript validator configuration
	ShellScript *ShellScriptValidatorConfig `json:"shellscript,omitempty" koanf:"shellscript" toml:"shellscript"`

	// Terraform validator configuration
	Terraform *TerraformValidatorConfig `json:"terraform,omitempty" koanf:"terraform" toml:"terraform"`

	// Workflow validator configuration (GitHub Actions)
	Workflow *WorkflowValidatorConfig `json:"workflow,omitempty" koanf:"workflow" toml:"workflow"`

	// Gofumpt validator configuration (Go formatting)
	Gofumpt *GofumptValidatorConfig `json:"gofumpt,omitempty" koanf:"gofumpt" toml:"gofumpt"`

	// Python validator configuration
	Python *PythonValidatorConfig `json:"python,omitempty" koanf:"python" toml:"python"`

	// JavaScript validator configuration
	JavaScript *JavaScriptValidatorConfig `json:"javascript,omitempty" koanf:"javascript" toml:"javascript"`

	// Rust validator configuration
	Rust *RustValidatorConfig `json:"rust,omitempty" koanf:"rust" toml:"rust"`

	// LinterIgnore validator configuration
	LinterIgnore *LinterIgnoreValidatorConfig `json:"linter_ignore,omitempty" koanf:"linter_ignore" toml:"linter_ignore"`
}

// MarkdownValidatorConfig configures the Markdown file validator.
type MarkdownValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for markdown linting operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// ContextLines is the number of lines before/after an edit to include for validation.
	// This allows validating edited fragments without forcing fixes for all existing issues.
	// Default: 2
	ContextLines *int `json:"context_lines,omitempty" koanf:"context_lines" toml:"context_lines"`

	// HeadingSpacing enforces blank lines around headings (custom rule).
	// Default: true
	HeadingSpacing *bool `json:"heading_spacing,omitempty" koanf:"heading_spacing" toml:"heading_spacing"`

	// CodeBlockFormatting enforces proper code block formatting (custom rule).
	// Default: true
	CodeBlockFormatting *bool `json:"code_block_formatting,omitempty" koanf:"code_block_formatting" toml:"code_block_formatting"`

	// ListFormatting enforces proper list item formatting and spacing (custom rule).
	// Default: true
	ListFormatting *bool `json:"list_formatting,omitempty" koanf:"list_formatting" toml:"list_formatting"`

	// UseMarkdownlint enables markdownlint-cli integration if available.
	// Default: true
	UseMarkdownlint *bool `json:"use_markdownlint,omitempty" koanf:"use_markdownlint" toml:"use_markdownlint"`

	// SkipPlanDocuments skips markdown validation for Claude Code plan files
	// written to ~/.claude/plans/. These are ephemeral working documents where
	// linting is counterproductive.
	// Superseded by SkipClaudeCodeFiles which covers all internal directories.
	// Default: true
	SkipPlanDocuments *bool `json:"skip_plan_documents,omitempty" koanf:"skip_plan_documents" toml:"skip_plan_documents"`

	// SkipClaudeCodeFiles skips markdown validation for all Claude Code
	// internal files (plans, memory, projects). These are auto-generated
	// working documents where linting is counterproductive.
	// Default: true
	SkipClaudeCodeFiles *bool `json:"skip_claude_code_files,omitempty" koanf:"skip_claude_code_files" toml:"skip_claude_code_files"`

	// MarkdownlintPath is the path to the markdownlint binary.
	// Default: "" (use PATH)
	MarkdownlintPath string `json:"markdownlint_path,omitempty" koanf:"markdownlint_path" toml:"markdownlint_path"`

	// MarkdownlintRules configures specific markdownlint-cli rules.
	// Map of rule name (e.g., "MD022") to enabled status.
	// When not specified, all markdownlint default rules are enabled.
	// Example: {"MD022": true, "MD041": false}
	MarkdownlintRules map[string]bool `json:"markdownlint_rules,omitempty" koanf:"markdownlint_rules" toml:"markdownlint_rules"`

	// MarkdownlintConfig is the path to a markdownlint configuration file.
	// If specified, this file takes precedence over MarkdownlintRules.
	// Default: "" (use MarkdownlintRules or markdownlint defaults)
	MarkdownlintConfig string `json:"markdownlint_config,omitempty" koanf:"markdownlint_config" toml:"markdownlint_config"`

	// TableFormatting enables validation and formatting suggestions for Markdown tables.
	// When enabled, malformed tables will be detected and properly formatted alternatives
	// will be suggested in error messages.
	// Default: true
	TableFormatting *bool `json:"table_formatting,omitempty" koanf:"table_formatting" toml:"table_formatting"`

	// TableFormattingMode controls how table column widths are calculated.
	// Options:
	//   - "display_width": Uses proper display width for Unicode characters (CJK, emoji).
	//     Tables will be visually aligned but may fail markdownlint MD060.
	//   - "byte_width": Uses byte length for width calculations.
	//     Tables will pass markdownlint MD060 but may not be visually aligned for Unicode.
	// Default: "display_width"
	TableFormattingMode string `json:"table_formatting_mode,omitempty" jsonschema:"enum=display_width,enum=byte_width" koanf:"table_formatting_mode" toml:"table_formatting_mode"`

	// TableFormattingSeverity controls how cosmetic table formatting issues
	// (column width padding differences) are treated.
	// Options:
	//   - "warning": Non-blocking warning (default). The agent sees a suggestion but
	//     the command is not blocked.
	//   - "error": Blocking error (old behavior). Column width mismatches block the operation.
	// Structural issues (column count mismatch, missing cell padding) always block
	// regardless of this setting.
	// Default: "warning"
	TableFormattingSeverity string `json:"table_formatting_severity,omitempty" jsonschema:"enum=warning,enum=error" koanf:"table_formatting_severity" toml:"table_formatting_severity"`
}

// ShellScriptValidatorConfig configures the shell script validator.
type ShellScriptValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for shellcheck operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// ContextLines is the number of lines before/after an edit to include for validation.
	// Default: 2
	ContextLines *int `json:"context_lines,omitempty" koanf:"context_lines" toml:"context_lines"`

	// UseShellcheck enables shellcheck integration if available.
	// Default: true
	UseShellcheck *bool `json:"use_shellcheck,omitempty" koanf:"use_shellcheck" toml:"use_shellcheck"`

	// ShellcheckSeverity is the minimum severity level for shellcheck findings.
	// Options: "error", "warning", "info", "style"
	// Default: "warning"
	ShellcheckSeverity string `json:"shellcheck_severity,omitempty" jsonschema:"enum=error,enum=warning,enum=info,enum=style" koanf:"shellcheck_severity" toml:"shellcheck_severity"`

	// ExcludeRules is a list of shellcheck rules to exclude (e.g., ["SC2086", "SC2154"]).
	// Default: []
	ExcludeRules []string `json:"exclude_rules,omitempty" koanf:"exclude_rules" toml:"exclude_rules"`

	// ShellcheckPath is the path to the shellcheck binary.
	// Default: "" (use PATH)
	ShellcheckPath string `json:"shellcheck_path,omitempty" koanf:"shellcheck_path" toml:"shellcheck_path"`
}

// TerraformValidatorConfig configures the Terraform/OpenTofu validator.
type TerraformValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for terraform/tofu operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// ContextLines is the number of lines before/after an edit to include for validation.
	// Default: 2
	ContextLines *int `json:"context_lines,omitempty" koanf:"context_lines" toml:"context_lines"`

	// ToolPreference specifies which tool to use when both are available.
	// Options: "tofu", "terraform", "auto" (prefers tofu)
	// Default: "auto"
	ToolPreference string `json:"tool_preference,omitempty" jsonschema:"enum=tofu,enum=terraform,enum=auto" koanf:"tool_preference" toml:"tool_preference"`

	// CheckFormat enables terraform/tofu format checking.
	// Default: true
	CheckFormat *bool `json:"check_format,omitempty" koanf:"check_format" toml:"check_format"`

	// UseTflint enables tflint integration if available.
	// Default: true
	UseTflint *bool `json:"use_tflint,omitempty" koanf:"use_tflint" toml:"use_tflint"`

	// TerraformPath is the path to the terraform binary.
	// Default: "" (use PATH)
	TerraformPath string `json:"terraform_path,omitempty" koanf:"terraform_path" toml:"terraform_path"`

	// TofuPath is the path to the tofu binary.
	// Default: "" (use PATH)
	TofuPath string `json:"tofu_path,omitempty" koanf:"tofu_path" toml:"tofu_path"`

	// TflintPath is the path to the tflint binary.
	// Default: "" (use PATH)
	TflintPath string `json:"tflint_path,omitempty" koanf:"tflint_path" toml:"tflint_path"`
}

// WorkflowValidatorConfig configures the GitHub Actions workflow validator.
type WorkflowValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for actionlint operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// GHAPITimeout is the maximum time allowed for GitHub API calls.
	// Default: "5s"
	GHAPITimeout Duration `json:"gh_api_timeout,omitempty" koanf:"gh_api_timeout" toml:"gh_api_timeout"`

	// EnforceDigestPinning requires actions to be pinned by digest.
	// Default: true
	EnforceDigestPinning *bool `json:"enforce_digest_pinning,omitempty" koanf:"enforce_digest_pinning" toml:"enforce_digest_pinning"`

	// RequireVersionComment requires a version comment when using digest pinning.
	// Format: uses: actions/checkout@sha256... # v4.1.7
	// Default: true
	RequireVersionComment *bool `json:"require_version_comment,omitempty" koanf:"require_version_comment" toml:"require_version_comment"`

	// CheckLatestVersion checks if the version comment matches the latest release.
	// Default: true
	CheckLatestVersion *bool `json:"check_latest_version,omitempty" koanf:"check_latest_version" toml:"check_latest_version"`

	// UseActionlint enables actionlint integration if available.
	// Default: true
	UseActionlint *bool `json:"use_actionlint,omitempty" koanf:"use_actionlint" toml:"use_actionlint"`

	// ActionlintPath is the path to the actionlint binary.
	// Default: "" (use PATH)
	ActionlintPath string `json:"actionlint_path,omitempty" koanf:"actionlint_path" toml:"actionlint_path"`
}

// GofumptValidatorConfig configures the Go code formatter validator.
type GofumptValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for gofumpt operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// ExtraRules enables gofumpt's -extra flag for stricter formatting rules.
	// Default: false
	ExtraRules *bool `json:"extra_rules,omitempty" koanf:"extra_rules" toml:"extra_rules"`

	// Lang specifies the Go language version (e.g., "go1.21").
	// If not specified, auto-detected from go.mod if available.
	// Default: "" (auto-detect)
	Lang string `json:"lang,omitempty" koanf:"lang" toml:"lang"`

	// ModPath specifies the module path for gofumpt.
	// If not specified, auto-detected from go.mod if available.
	// Default: "" (auto-detect)
	ModPath string `json:"modpath,omitempty" koanf:"modpath" toml:"modpath"`

	// GofumptPath is the path to the gofumpt binary.
	// Default: "" (use PATH)
	GofumptPath string `json:"gofumpt_path,omitempty" koanf:"gofumpt_path" toml:"gofumpt_path"`
}

// PythonValidatorConfig configures the Python file validator.
type PythonValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for ruff operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// ContextLines is the number of lines before/after an edit to include for validation.
	// Default: 2
	ContextLines *int `json:"context_lines,omitempty" koanf:"context_lines" toml:"context_lines"`

	// UseRuff enables ruff integration if available.
	// Default: true
	UseRuff *bool `json:"use_ruff,omitempty" koanf:"use_ruff" toml:"use_ruff"`

	// RuffPath is the path to the ruff binary.
	// Default: "" (use PATH)
	RuffPath string `json:"ruff_path,omitempty" koanf:"ruff_path" toml:"ruff_path"`

	// ExcludeRules is a list of ruff rules to exclude (e.g., ["F401", "E501"]).
	// Default: []
	ExcludeRules []string `json:"exclude_rules,omitempty" koanf:"exclude_rules" toml:"exclude_rules"`

	// RuffConfig is the path to a ruff configuration file (pyproject.toml or ruff.toml).
	// Default: "" (use ruff defaults)
	RuffConfig string `json:"ruff_config,omitempty" koanf:"ruff_config" toml:"ruff_config"`
}

// JavaScriptValidatorConfig configures the JavaScript/TypeScript file validator.
type JavaScriptValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for oxlint operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// ContextLines is the number of lines before/after an edit to include for validation.
	// Default: 2
	ContextLines *int `json:"context_lines,omitempty" koanf:"context_lines" toml:"context_lines"`

	// UseOxlint enables oxlint integration if available.
	// Default: true
	UseOxlint *bool `json:"use_oxlint,omitempty" koanf:"use_oxlint" toml:"use_oxlint"`

	// OxlintPath is the path to the oxlint binary.
	// Default: "" (use PATH)
	OxlintPath string `json:"oxlint_path,omitempty" koanf:"oxlint_path" toml:"oxlint_path"`

	// ExcludeRules is a list of oxlint rules to exclude (e.g., ["no-unused-vars", "no-console"]).
	// Default: []
	ExcludeRules []string `json:"exclude_rules,omitempty" koanf:"exclude_rules" toml:"exclude_rules"`

	// OxlintConfig is the path to an oxlint configuration file (.oxlintrc.json).
	// Default: "" (use oxlint defaults)
	OxlintConfig string `json:"oxlint_config,omitempty" koanf:"oxlint_config" toml:"oxlint_config"`
}

// RustValidatorConfig configures the Rust file validator.
type RustValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Timeout is the maximum time allowed for rustfmt operations.
	// Default: "10s"
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// ContextLines is the number of lines before/after an edit to include for validation.
	// Default: 2
	ContextLines *int `json:"context_lines,omitempty" koanf:"context_lines" toml:"context_lines"`

	// UseRustfmt enables rustfmt integration if available.
	// Default: true
	UseRustfmt *bool `json:"use_rustfmt,omitempty" koanf:"use_rustfmt" toml:"use_rustfmt"`

	// Edition is the Rust edition (2015, 2018, 2021, 2024).
	// If not specified, auto-detected from Cargo.toml if available.
	// Default: "2021"
	Edition string `json:"edition,omitempty" jsonschema:"enum=2015,enum=2018,enum=2021,enum=2024" koanf:"edition" toml:"edition"`

	// RustfmtPath is the path to the rustfmt binary.
	// Default: "" (use PATH)
	RustfmtPath string `json:"rustfmt_path,omitempty" koanf:"rustfmt_path" toml:"rustfmt_path"`

	// RustfmtConfig is the path to a rustfmt configuration file (rustfmt.toml).
	// Default: "" (use rustfmt defaults)
	RustfmtConfig string `json:"rustfmt_config,omitempty" koanf:"rustfmt_config" toml:"rustfmt_config"`
}

// LinterIgnoreValidatorConfig configures the linter ignore directive validator.
type LinterIgnoreValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// Patterns is a list of regex patterns to detect linter ignore directives.
	// Default: built-in patterns for common languages (noqa, eslint-disable, nolint, etc.)
	Patterns []string `json:"patterns,omitempty" koanf:"patterns" toml:"patterns"`
}
