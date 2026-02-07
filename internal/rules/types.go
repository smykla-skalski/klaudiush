// Package rules provides a dynamic rule engine for validator configuration.
// Rules allow users to define custom validation behavior without modifying code.
package rules

import (
	"context"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

// ActionType represents the action to take when a rule matches.
type ActionType string

const (
	// ActionBlock blocks the operation with an error.
	ActionBlock ActionType = "block"

	// ActionWarn warns without blocking.
	ActionWarn ActionType = "warn"

	// ActionAllow explicitly allows the operation.
	ActionAllow ActionType = "allow"
)

// ValidatorType identifies a specific validator or group of validators.
// Format: "category.name" (e.g., "git.push", "file.markdown")
// Wildcards: "git.*" (all git validators), "*" (all validators)
type ValidatorType string

// Common validator type constants.
const (
	ValidatorGitPush        ValidatorType = "git.push"
	ValidatorGitFetch       ValidatorType = "git.fetch"
	ValidatorGitCommit      ValidatorType = "git.commit"
	ValidatorGitAdd         ValidatorType = "git.add"
	ValidatorGitPR          ValidatorType = "git.pr"
	ValidatorGitMerge       ValidatorType = "git.merge"
	ValidatorGitBranch      ValidatorType = "git.branch"
	ValidatorGitNoVerify    ValidatorType = "git.no_verify"
	ValidatorGitAll         ValidatorType = "git.*"
	ValidatorGitHubIssue    ValidatorType = "github.issue"
	ValidatorGitHubAll      ValidatorType = "github.*"
	ValidatorFileMarkdown   ValidatorType = "file.markdown"
	ValidatorFileShell      ValidatorType = "file.shell"
	ValidatorFileTerraform  ValidatorType = "file.terraform"
	ValidatorFileWorkflow   ValidatorType = "file.workflow"
	ValidatorFileGofumpt    ValidatorType = "file.gofumpt"
	ValidatorFilePython     ValidatorType = "file.python"
	ValidatorFileJavaScript ValidatorType = "file.javascript"
	ValidatorFileRust       ValidatorType = "file.rust"
	ValidatorFileAll        ValidatorType = "file.*"
	ValidatorSecrets        ValidatorType = "secrets.secrets"
	ValidatorShellBacktick  ValidatorType = "shell.backtick"
	ValidatorNotification   ValidatorType = "notification.bell"
	ValidatorAll            ValidatorType = "*"
)

// Rule represents a single validation rule with match conditions and action.
type Rule struct {
	// Name uniquely identifies this rule. Used for override precedence.
	Name string

	// Description provides human-readable explanation of the rule.
	Description string

	// Enabled controls whether this rule is active.
	Enabled bool

	// Priority determines evaluation order (higher = evaluated first).
	Priority int

	// Match contains the conditions that must be satisfied.
	Match *RuleMatch

	// Action specifies what happens when the rule matches.
	Action *RuleAction
}

// RuleMatch contains all conditions for a rule to match.
// All non-nil conditions must be satisfied (AND logic).
type RuleMatch struct {
	// ValidatorType filters by validator type (supports wildcards).
	ValidatorType ValidatorType

	// RepoPattern matches against the repository root path.
	RepoPattern string

	// RepoPatterns allows multiple repository patterns.
	RepoPatterns []string

	// Remote matches against git remote name (exact match).
	Remote string

	// BranchPattern matches against branch name.
	BranchPattern string

	// BranchPatterns allows multiple branch patterns.
	BranchPatterns []string

	// FilePattern matches against file path.
	FilePattern string

	// FilePatterns allows multiple file patterns.
	FilePatterns []string

	// ContentPattern matches against file content (regex).
	ContentPattern string

	// ContentPatterns allows multiple content patterns.
	ContentPatterns []string

	// CommandPattern matches against bash command.
	CommandPattern string

	// CommandPatterns allows multiple command patterns.
	CommandPatterns []string

	// ToolType matches against the hook tool type.
	ToolType string

	// EventType matches against the hook event type.
	EventType string

	// CaseInsensitive enables case-insensitive pattern matching.
	CaseInsensitive bool

	// PatternMode specifies how multiple patterns are combined ("any" or "all").
	PatternMode string
}

// RuleAction specifies what happens when a rule matches.
type RuleAction struct {
	// Type is the action to take (block, warn, allow).
	Type ActionType

	// Message is the human-readable message to display.
	Message string

	// Reference is an optional error reference code (e.g., "GIT019").
	Reference string
}

// RuleResult represents the outcome of rule evaluation.
type RuleResult struct {
	// Matched indicates whether any rule matched.
	Matched bool

	// Rule is the rule that matched (if any).
	Rule *Rule

	// Action is the action to take.
	Action ActionType

	// Message is the message to display.
	Message string

	// Reference is the error reference code (if any).
	Reference string
}

// GitContext contains git-specific data for rule matching.
type GitContext struct {
	// RepoRoot is the absolute path to the repository root.
	RepoRoot string

	// Remote is the target remote name for push/pull operations.
	Remote string

	// Branch is the current or target branch name.
	Branch string

	// IsInRepo indicates whether we're inside a git repository.
	IsInRepo bool
}

// FileContext contains file-specific data for rule matching.
type FileContext struct {
	// Path is the file path being operated on.
	Path string

	// Content is the file content (if available).
	Content string
}

// MatchContext provides all data needed for rule matching.
type MatchContext struct {
	// HookContext is the original hook context.
	HookContext *hook.Context

	// GitContext contains git-related data (may be nil).
	GitContext *GitContext

	// FileContext contains file-related data (may be nil).
	FileContext *FileContext

	// ValidatorType is the type of validator being run.
	ValidatorType ValidatorType

	// Command is the bash command being executed (if applicable).
	Command string
}

// Engine is the main interface for the rule engine.
type Engine interface {
	// Evaluate evaluates rules against the given context.
	// Returns a RuleResult indicating whether any rule matched and what action to take.
	Evaluate(ctx context.Context, matchCtx *MatchContext) *RuleResult
}

// Pattern represents a compiled pattern for matching strings.
type Pattern interface {
	// Match returns true if the string matches the pattern.
	Match(s string) bool

	// String returns the original pattern string.
	String() string
}

// Matcher evaluates a single condition against a match context.
type Matcher interface {
	// Match returns true if the condition is satisfied.
	Match(ctx *MatchContext) bool

	// Name returns a descriptive name for the matcher.
	Name() string
}

// ValidatorAdapter provides rule checking for validators.
type ValidatorAdapter interface {
	// CheckRules evaluates rules for the given hook context.
	// Returns a validator.Result if a rule matched, or nil to continue with built-in logic.
	CheckRules(ctx context.Context, hookCtx *hook.Context) *validator.Result
}
