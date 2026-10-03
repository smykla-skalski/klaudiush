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

	// ToolPhase withholds mutation tools from Gemini until prerequisite
	// checks pass on the current content.
	ToolPhase *EvidenceToolPhaseConfig `json:"tool_phase,omitempty" koanf:"tool_phase" toml:"tool_phase,omitempty"`
}

// DefaultToolPhaseReadOnlyTools are the Gemini built-in tools offered while
// a tool phase withholds mutation tools.
var DefaultToolPhaseReadOnlyTools = []string{
	"ask_user",
	"glob",
	"google_web_search",
	"grep_search",
	"list_directory",
	"read_file",
	"read_many_files",
	"web_fetch",
	"write_todos",
}

// EvidenceToolPhaseConfig restricts the tools Gemini offers the model until
// the prerequisite checks have a passing result on the current content.
// Until then Gemini BeforeToolSelection offers only read-only tools, the
// shell (for "klaudiush evidence run") and, when WritablePaths is set, the
// file tools; BeforeTool denies any call outside that set. A later change
// that invalidates a prerequisite withholds the tools again. Only Gemini has
// a tool-selection event, so other providers are not restricted.
//
// Example configuration:
//
//	[evidence.tool_phase]
//	enabled = true
//	requires = ["plan"]
//	writable_paths = ["docs/plans/**"]
type EvidenceToolPhaseConfig struct {
	// Enabled turns the tool phase on. Requires the evidence gate. Default: false.
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// Requires names the evidence checks that must pass before mutation
	// tools are offered.
	Requires []string `json:"requires,omitempty" koanf:"requires" toml:"requires,omitempty"`

	// ReadOnlyTools are the Gemini tool names offered while the phase is
	// restricted. Default: DefaultToolPhaseReadOnlyTools.
	ReadOnlyTools []string `json:"read_only_tools,omitempty" koanf:"read_only_tools" toml:"read_only_tools,omitempty"`

	// WritablePaths are glob patterns, relative to the repository root, of
	// the files write_file and replace may change while the phase is
	// restricted, such as the plan a prerequisite checks. Default: none.
	WritablePaths []string `json:"writable_paths,omitempty" koanf:"writable_paths" toml:"writable_paths,omitempty"`

	// FilterTools answers Gemini BeforeToolSelection with the phase's tools.
	// Gemini sends them as allowedFunctionNames with mode AUTO, which the
	// Gemini API documents only for modes ANY and VALIDATED; set false if
	// the model API rejects it, and BeforeTool still denies withheld calls.
	// Default: true.
	FilterTools *bool `json:"filter_tools,omitempty" koanf:"filter_tools" toml:"filter_tools,omitempty"`
}

// IsEnabled reports whether the tool phase is on. Defaults to false.
func (p *EvidenceToolPhaseConfig) IsEnabled() bool {
	if p == nil || p.Enabled == nil {
		return false
	}

	return *p.Enabled
}

// FiltersTools reports whether Gemini tool selection is narrowed. Defaults to true.
func (p *EvidenceToolPhaseConfig) FiltersTools() bool {
	if p == nil || p.FilterTools == nil {
		return true
	}

	return *p.FilterTools
}

// SelectsTools reports whether the phase answers Gemini
// BeforeToolSelection: it is on and narrows the tools offered. Only then
// does the selection hook need registering.
func (p *EvidenceToolPhaseConfig) SelectsTools() bool {
	return p.IsEnabled() && p.FiltersTools()
}

// GetReadOnlyTools returns the tools offered while restricted.
func (p *EvidenceToolPhaseConfig) GetReadOnlyTools() []string {
	if p == nil || p.ReadOnlyTools == nil {
		return DefaultToolPhaseReadOnlyTools
	}

	return p.ReadOnlyTools
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

// GetToolPhase returns the tool phase configuration, or nil.
func (e *EvidenceConfig) GetToolPhase() *EvidenceToolPhaseConfig {
	if e == nil {
		return nil
	}

	return e.ToolPhase
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
