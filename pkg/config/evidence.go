package config

import "time"

// Evidence check kinds. A test result covers the content of the files the
// check covers; a review result covers the exact diff of those files
// against a base revision.
const (
	EvidenceKindTest   = "test"
	EvidenceKindReview = "review"
)

// DefaultEvidenceTimeout bounds one verifier run of a check, and how long a
// result reported as running counts as running.
const DefaultEvidenceTimeout = 30 * time.Minute

// EvidenceConfig gates completion on fresh results of required checks. When
// a session changed files a check covers, the completion gate (Claude Stop,
// Codex Stop, Gemini AfterAgent) blocks until the check passed against the
// current content. Disabled by default.
//
// Example configuration:
//
//	[evidence]
//	enabled = true
//
//	[[evidence.checks]]
//	name = "tests"
//	commands = ["mise run test"]
//	paths = ["**/*.go", "go.mod", "go.sum"]
type EvidenceConfig struct {
	// Enabled turns the evidence gate on. Default: false.
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// Checks lists the required checks.
	Checks []*EvidenceCheckConfig `json:"checks,omitempty" koanf:"checks" toml:"checks,omitempty"`
}

// EvidenceCheckConfig describes one required check.
type EvidenceCheckConfig struct {
	// Name identifies the check in messages and in "klaudiush evidence run".
	Name string `json:"name" koanf:"name" toml:"name"`

	// Kind is "test" (default) or "review". A test result covers the content
	// of the files matching Paths; a review result covers the exact diff of
	// those files against Base.
	Kind string `json:"kind,omitempty" jsonschema:"enum=test,enum=review" koanf:"kind" toml:"kind,omitempty"`

	// Commands lists the exact commands that run the check. A shell command
	// counts only when it is one of these, word for word, run on its own
	// from the repository root. The first one is what the verifier runs.
	Commands []string `json:"commands" koanf:"commands" toml:"commands"`

	// Paths are glob patterns, relative to the repository root, of the
	// files the check covers. Only changes to these files require the check
	// and invalidate its results. Default: every file.
	Paths []string `json:"paths,omitempty" koanf:"paths" toml:"paths,omitempty"`

	// Exclude are glob patterns of files the check ignores.
	Exclude []string `json:"exclude,omitempty" koanf:"exclude" toml:"exclude,omitempty"`

	// Base is the branch a review diffs against, such as "origin/main".
	// The diff starts at the merge base of Base and HEAD, so committing the
	// reviewed changes keeps the review valid. Required for reviews.
	Base string `json:"base,omitempty" koanf:"base" toml:"base,omitempty"`

	// Timeout bounds one verifier run and how long a running result stays
	// running. Default: "30m".
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout,omitempty"`
}

// IsEnabled reports whether the evidence gate is on. Defaults to false.
func (e *EvidenceConfig) IsEnabled() bool {
	if e == nil || e.Enabled == nil {
		return false
	}

	return *e.Enabled
}

// GetKind returns the check kind, defaulting to EvidenceKindTest.
func (c *EvidenceCheckConfig) GetKind() string {
	if c == nil || c.Kind == "" {
		return EvidenceKindTest
	}

	return c.Kind
}

// GetTimeout returns the verifier timeout, defaulting to DefaultEvidenceTimeout.
func (c *EvidenceCheckConfig) GetTimeout() time.Duration {
	if c == nil || c.Timeout <= 0 {
		return DefaultEvidenceTimeout
	}

	return time.Duration(c.Timeout)
}
