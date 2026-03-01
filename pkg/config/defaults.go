package config

// Default values for git commit message validation.
const (
	DefaultTitleMaxLength    = 50
	DefaultBodyMaxLineLength = 72
	DefaultBodyLineTolerance = 5
	DefaultCommitStyle       = "conventional"
)

// DefaultValidTypes are the valid commit types from commitlint config-conventional.
var DefaultValidTypes = []string{
	"build", "chore", "ci", "docs", "feat", "fix",
	"perf", "refactor", "revert", "style", "test",
}

// DefaultValidBranchTypes are the valid branch type prefixes.
var DefaultValidBranchTypes = []string{
	"feat", "fix", "docs", "style", "refactor",
	"test", "chore", "ci", "build", "perf",
}

// DefaultProtectedBranches are branches that skip validation.
var DefaultProtectedBranches = []string{"main", "master"}
