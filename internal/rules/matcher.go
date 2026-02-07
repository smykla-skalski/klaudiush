package rules

import (
	"strings"

	"github.com/smykla-labs/klaudiush/pkg/hook"
)

// RepoPatternMatcher matches against the repository root path.
type RepoPatternMatcher struct {
	pattern Pattern
}

// NewRepoPatternMatcher creates a matcher for repository path patterns.
func NewRepoPatternMatcher(patternStr string) (*RepoPatternMatcher, error) {
	pattern, err := GetCachedPattern(patternStr)
	if err != nil {
		return nil, err
	}

	return &RepoPatternMatcher{pattern: pattern}, nil
}

// NewRepoPatternMatcherWithOpts creates a matcher with pattern options.
func NewRepoPatternMatcherWithOpts(
	patternStr string,
	opts PatternOptions,
) (*RepoPatternMatcher, error) {
	pattern, err := CompilePatternWithOptions(patternStr, opts)
	if err != nil {
		return nil, err
	}

	return &RepoPatternMatcher{pattern: pattern}, nil
}

// NewRepoMultiPatternMatcher creates a matcher for multiple repository path patterns.
func NewRepoMultiPatternMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (*RepoPatternMatcher, error) {
	pattern, err := CompileMultiPattern(patterns, mode, opts)
	if err != nil {
		return nil, err
	}

	if pattern == nil {
		return nil, nil //nolint:nilnil // no patterns is valid
	}

	return &RepoPatternMatcher{pattern: pattern}, nil
}

// Match returns true if the repo root matches the pattern.
func (m *RepoPatternMatcher) Match(ctx *MatchContext) bool {
	if ctx.GitContext == nil || ctx.GitContext.RepoRoot == "" {
		return false
	}

	return m.pattern.Match(ctx.GitContext.RepoRoot)
}

// Name returns the matcher name.
func (m *RepoPatternMatcher) Name() string {
	return "repo_pattern:" + m.pattern.String()
}

// RemoteMatcher matches against the git remote name.
type RemoteMatcher struct {
	remote string
}

// NewRemoteMatcher creates a matcher for exact remote name matching.
func NewRemoteMatcher(remote string) *RemoteMatcher {
	return &RemoteMatcher{remote: remote}
}

// Match returns true if the remote matches exactly.
func (m *RemoteMatcher) Match(ctx *MatchContext) bool {
	if ctx.GitContext == nil {
		return false
	}

	return ctx.GitContext.Remote == m.remote
}

// Name returns the matcher name.
func (m *RemoteMatcher) Name() string {
	return "remote:" + m.remote
}

// BranchPatternMatcher matches against branch names.
type BranchPatternMatcher struct {
	pattern Pattern
}

// NewBranchPatternMatcher creates a matcher for branch name patterns.
func NewBranchPatternMatcher(patternStr string) (*BranchPatternMatcher, error) {
	pattern, err := GetCachedPattern(patternStr)
	if err != nil {
		return nil, err
	}

	return &BranchPatternMatcher{pattern: pattern}, nil
}

// NewBranchPatternMatcherWithOpts creates a matcher with pattern options.
func NewBranchPatternMatcherWithOpts(
	patternStr string,
	opts PatternOptions,
) (*BranchPatternMatcher, error) {
	pattern, err := CompilePatternWithOptions(patternStr, opts)
	if err != nil {
		return nil, err
	}

	return &BranchPatternMatcher{pattern: pattern}, nil
}

// NewBranchMultiPatternMatcher creates a matcher for multiple branch patterns.
func NewBranchMultiPatternMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (*BranchPatternMatcher, error) {
	pattern, err := CompileMultiPattern(patterns, mode, opts)
	if err != nil {
		return nil, err
	}

	if pattern == nil {
		return nil, nil //nolint:nilnil // no patterns is valid
	}

	return &BranchPatternMatcher{pattern: pattern}, nil
}

// Match returns true if the branch matches the pattern.
func (m *BranchPatternMatcher) Match(ctx *MatchContext) bool {
	if ctx.GitContext == nil || ctx.GitContext.Branch == "" {
		return false
	}

	return m.pattern.Match(ctx.GitContext.Branch)
}

// Name returns the matcher name.
func (m *BranchPatternMatcher) Name() string {
	return "branch_pattern:" + m.pattern.String()
}

// FilePatternMatcher matches against file paths.
type FilePatternMatcher struct {
	pattern Pattern
}

// NewFilePatternMatcher creates a matcher for file path patterns.
func NewFilePatternMatcher(patternStr string) (*FilePatternMatcher, error) {
	pattern, err := GetCachedPattern(patternStr)
	if err != nil {
		return nil, err
	}

	return &FilePatternMatcher{pattern: pattern}, nil
}

// NewFilePatternMatcherWithOpts creates a matcher with pattern options.
func NewFilePatternMatcherWithOpts(
	patternStr string,
	opts PatternOptions,
) (*FilePatternMatcher, error) {
	pattern, err := CompilePatternWithOptions(patternStr, opts)
	if err != nil {
		return nil, err
	}

	return &FilePatternMatcher{pattern: pattern}, nil
}

// NewFileMultiPatternMatcher creates a matcher for multiple file path patterns.
func NewFileMultiPatternMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (*FilePatternMatcher, error) {
	pattern, err := CompileMultiPattern(patterns, mode, opts)
	if err != nil {
		return nil, err
	}

	if pattern == nil {
		return nil, nil //nolint:nilnil // no patterns is valid
	}

	return &FilePatternMatcher{pattern: pattern}, nil
}

// Match returns true if the file path matches the pattern.
func (m *FilePatternMatcher) Match(ctx *MatchContext) bool {
	if ctx.FileContext == nil || ctx.FileContext.Path == "" {
		// Fall back to hook context file path.
		if ctx.HookContext != nil {
			return m.pattern.Match(ctx.HookContext.GetFilePath())
		}

		return false
	}

	return m.pattern.Match(ctx.FileContext.Path)
}

// Name returns the matcher name.
func (m *FilePatternMatcher) Name() string {
	return "file_pattern:" + m.pattern.String()
}

// ContentPatternMatcher matches against file content using regex.
type ContentPatternMatcher struct {
	pattern Pattern
}

// NewContentPatternMatcher creates a matcher for content patterns.
// Content patterns always use regex.
func NewContentPatternMatcher(patternStr string) (*ContentPatternMatcher, error) {
	pattern, err := NewRegexPattern(patternStr)
	if err != nil {
		return nil, err
	}

	return &ContentPatternMatcher{pattern: pattern}, nil
}

// NewContentPatternMatcherWithOpts creates a matcher with pattern options.
// Content patterns always use regex.
func NewContentPatternMatcherWithOpts(
	patternStr string,
	opts PatternOptions,
) (*ContentPatternMatcher, error) {
	// For content, force regex detection by using CompilePatternWithOptions
	// but handle case-insensitivity here since content is always regex.
	negated := opts.Negate || IsNegated(patternStr)
	if IsNegated(patternStr) {
		patternStr = StripNegation(patternStr)
	}

	// Add case-insensitive flag if needed.
	if opts.CaseInsensitive && !strings.HasPrefix(patternStr, "(?i)") {
		patternStr = "(?i)" + patternStr
	}

	pattern, err := NewRegexPattern(patternStr)
	if err != nil {
		return nil, err
	}

	// Wrap in NegatedPattern if needed.
	if negated {
		return &ContentPatternMatcher{pattern: NewNegatedPattern(pattern)}, nil
	}

	return &ContentPatternMatcher{pattern: pattern}, nil
}

// NewContentMultiPatternMatcher creates a matcher for multiple content patterns.
func NewContentMultiPatternMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (*ContentPatternMatcher, error) {
	if len(patterns) == 0 {
		return nil, nil //nolint:nilnil // no patterns is valid
	}

	// Single pattern.
	if len(patterns) == 1 {
		return NewContentPatternMatcherWithOpts(patterns[0], opts)
	}

	// Multiple patterns.
	compiled := make([]Pattern, 0, len(patterns))

	for _, p := range patterns {
		negated := opts.Negate || IsNegated(p)
		patternStr := p

		if IsNegated(p) {
			patternStr = StripNegation(p)
		}

		if opts.CaseInsensitive && !strings.HasPrefix(patternStr, "(?i)") {
			patternStr = "(?i)" + patternStr
		}

		pattern, err := NewRegexPattern(patternStr)
		if err != nil {
			return nil, err
		}

		if negated {
			compiled = append(compiled, NewNegatedPattern(pattern))
		} else {
			compiled = append(compiled, pattern)
		}
	}

	// Build string representation.
	modeStr := PatternModeAny
	if mode == MultiPatternAll {
		modeStr = PatternModeAll
	}

	repr := modeStr + "(" + strings.Join(patterns, ", ") + ")"

	return &ContentPatternMatcher{pattern: NewMultiPattern(compiled, mode, repr)}, nil
}

// Match returns true if the file content matches the pattern.
func (m *ContentPatternMatcher) Match(ctx *MatchContext) bool {
	if ctx.FileContext == nil || ctx.FileContext.Content == "" {
		// Fall back to hook context content.
		if ctx.HookContext != nil {
			return m.pattern.Match(ctx.HookContext.GetContent())
		}

		return false
	}

	return m.pattern.Match(ctx.FileContext.Content)
}

// Name returns the matcher name.
func (m *ContentPatternMatcher) Name() string {
	return "content_pattern:" + m.pattern.String()
}

// CommandPatternMatcher matches against bash commands.
type CommandPatternMatcher struct {
	pattern Pattern
}

// NewCommandPatternMatcher creates a matcher for command patterns.
func NewCommandPatternMatcher(patternStr string) (*CommandPatternMatcher, error) {
	pattern, err := GetCachedPattern(patternStr)
	if err != nil {
		return nil, err
	}

	return &CommandPatternMatcher{pattern: pattern}, nil
}

// NewCommandPatternMatcherWithOpts creates a matcher with pattern options.
func NewCommandPatternMatcherWithOpts(
	patternStr string,
	opts PatternOptions,
) (*CommandPatternMatcher, error) {
	pattern, err := CompilePatternWithOptions(patternStr, opts)
	if err != nil {
		return nil, err
	}

	return &CommandPatternMatcher{pattern: pattern}, nil
}

// NewCommandMultiPatternMatcher creates a matcher for multiple command patterns.
func NewCommandMultiPatternMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (*CommandPatternMatcher, error) {
	pattern, err := CompileMultiPattern(patterns, mode, opts)
	if err != nil {
		return nil, err
	}

	if pattern == nil {
		return nil, nil //nolint:nilnil // no patterns is valid
	}

	return &CommandPatternMatcher{pattern: pattern}, nil
}

// Match returns true if the command matches the pattern.
func (m *CommandPatternMatcher) Match(ctx *MatchContext) bool {
	if ctx.Command != "" {
		return m.pattern.Match(ctx.Command)
	}

	if ctx.HookContext != nil {
		return m.pattern.Match(ctx.HookContext.GetCommand())
	}

	return false
}

// Name returns the matcher name.
func (m *CommandPatternMatcher) Name() string {
	return "command_pattern:" + m.pattern.String()
}

// ValidatorTypeMatcher matches against validator type.
type ValidatorTypeMatcher struct {
	validatorType ValidatorType
}

// NewValidatorTypeMatcher creates a matcher for validator types.
func NewValidatorTypeMatcher(validatorType ValidatorType) *ValidatorTypeMatcher {
	return &ValidatorTypeMatcher{validatorType: validatorType}
}

// Match returns true if the validator type matches.
// Supports wildcards: "git.*" matches all git validators, "*" matches all.
func (m *ValidatorTypeMatcher) Match(ctx *MatchContext) bool {
	if m.validatorType == ValidatorAll {
		return true
	}

	if ctx.ValidatorType == "" {
		return false
	}

	// Check for exact match.
	if ctx.ValidatorType == m.validatorType {
		return true
	}

	// Check for category wildcard (e.g., "git.*" matches "git.push").
	pattern := string(m.validatorType)
	target := string(ctx.ValidatorType)

	if before, ok := strings.CutSuffix(pattern, ".*"); ok {
		prefix := before
		return strings.HasPrefix(target, prefix+".")
	}

	return false
}

// Name returns the matcher name.
func (m *ValidatorTypeMatcher) Name() string {
	return "validator_type:" + string(m.validatorType)
}

// ToolTypeMatcher matches against the hook tool type.
type ToolTypeMatcher struct {
	toolType string
}

// NewToolTypeMatcher creates a matcher for tool types.
func NewToolTypeMatcher(toolType string) *ToolTypeMatcher {
	return &ToolTypeMatcher{toolType: toolType}
}

// Match returns true if the tool type matches.
func (m *ToolTypeMatcher) Match(ctx *MatchContext) bool {
	if ctx.HookContext == nil {
		return false
	}

	return strings.EqualFold(ctx.HookContext.ToolName.String(), m.toolType)
}

// Name returns the matcher name.
func (m *ToolTypeMatcher) Name() string {
	return "tool_type:" + m.toolType
}

// EventTypeMatcher matches against the hook event type.
type EventTypeMatcher struct {
	eventType string
}

// NewEventTypeMatcher creates a matcher for event types.
func NewEventTypeMatcher(eventType string) *EventTypeMatcher {
	return &EventTypeMatcher{eventType: eventType}
}

// Match returns true if the event type matches.
func (m *EventTypeMatcher) Match(ctx *MatchContext) bool {
	if ctx.HookContext == nil {
		return false
	}

	return strings.EqualFold(ctx.HookContext.EventType.String(), m.eventType)
}

// Name returns the matcher name.
func (m *EventTypeMatcher) Name() string {
	return "event_type:" + m.eventType
}

// CompositeOp represents the operation for composite matchers.
type CompositeOp int

const (
	// CompositeOpAND requires all matchers to match.
	CompositeOpAND CompositeOp = iota

	// CompositeOpOR requires at least one matcher to match.
	CompositeOpOR

	// CompositeOpNOT inverts the result of the first matcher.
	CompositeOpNOT
)

// CompositeMatcher combines multiple matchers with AND/OR/NOT logic.
type CompositeMatcher struct {
	matchers []Matcher
	op       CompositeOp
}

// NewAndMatcher creates a matcher that requires all conditions to match.
func NewAndMatcher(matchers ...Matcher) *CompositeMatcher {
	return &CompositeMatcher{
		matchers: matchers,
		op:       CompositeOpAND,
	}
}

// NewOrMatcher creates a matcher that requires at least one condition to match.
func NewOrMatcher(matchers ...Matcher) *CompositeMatcher {
	return &CompositeMatcher{
		matchers: matchers,
		op:       CompositeOpOR,
	}
}

// NewNotMatcher creates a matcher that inverts the result.
func NewNotMatcher(matcher Matcher) *CompositeMatcher {
	return &CompositeMatcher{
		matchers: []Matcher{matcher},
		op:       CompositeOpNOT,
	}
}

// Match evaluates all matchers according to the composite operation.
func (m *CompositeMatcher) Match(ctx *MatchContext) bool {
	if len(m.matchers) == 0 {
		return true
	}

	switch m.op {
	case CompositeOpAND:
		for _, matcher := range m.matchers {
			if !matcher.Match(ctx) {
				return false
			}
		}

		return true

	case CompositeOpOR:
		for _, matcher := range m.matchers {
			if matcher.Match(ctx) {
				return true
			}
		}

		return false

	case CompositeOpNOT:
		return !m.matchers[0].Match(ctx)

	default:
		return false
	}
}

// Name returns a descriptive name for the composite matcher.
func (m *CompositeMatcher) Name() string {
	switch m.op {
	case CompositeOpAND:
		return "AND"
	case CompositeOpOR:
		return "OR"
	case CompositeOpNOT:
		return "NOT"
	default:
		return "UNKNOWN"
	}
}

// matcherBuilder is a helper for building matchers with error handling.
type matcherBuilder struct {
	matchers []Matcher
	err      error
	opts     PatternOptions
	mode     MultiPatternMode
}

// addSimple adds a matcher that doesn't require compilation.
func (b *matcherBuilder) addSimple(m Matcher) {
	if b.err != nil {
		return
	}

	b.matchers = append(b.matchers, m)
}

// addPatternMatcher adds a pattern matcher if pattern is non-empty.
func (b *matcherBuilder) addPatternMatcher(
	pattern string,
	factory func(string) (Matcher, error),
) {
	if b.err != nil || pattern == "" {
		return
	}

	m, err := factory(pattern)
	if err != nil {
		b.err = err
		return
	}

	b.matchers = append(b.matchers, m)
}

// advancedPatternFactory is a function that creates a matcher with pattern options.
type advancedPatternFactory func(string, PatternOptions) (Matcher, error)

// multiPatternFactory is a function that creates a multi-pattern matcher.
type multiPatternFactory func([]string, MultiPatternMode, PatternOptions) (Matcher, error)

// addAdvancedPatternMatcher adds a pattern matcher with options support.
// Uses multi-patterns if provided, otherwise falls back to single pattern.
func (b *matcherBuilder) addAdvancedPatternMatcher(
	pattern string,
	patterns []string,
	singleFactory advancedPatternFactory,
	multiFactory multiPatternFactory,
) {
	if b.err != nil {
		return
	}

	// Prefer multi-patterns if provided.
	if len(patterns) > 0 {
		m, err := multiFactory(patterns, b.mode, b.opts)
		if err != nil {
			b.err = err
			return
		}

		if m != nil {
			b.matchers = append(b.matchers, m)
		}

		return
	}

	// Fall back to single pattern.
	if pattern == "" {
		return
	}

	m, err := singleFactory(pattern, b.opts)
	if err != nil {
		b.err = err
		return
	}

	b.matchers = append(b.matchers, m)
}

// result returns the final matcher or error.
//
//nolint:nilnil,ireturn // returning nil, nil is intentional; interface for polymorphism
func (b *matcherBuilder) result() (Matcher, error) {
	if b.err != nil {
		return nil, b.err
	}

	switch len(b.matchers) {
	case 0:
		return nil, nil
	case 1:
		return b.matchers[0], nil
	default:
		return NewAndMatcher(b.matchers...), nil
	}
}

// Pattern matcher factory wrappers.
//
//nolint:ireturn // interface for polymorphism
func wrapRepoMatcher(p string) (Matcher, error) { return NewRepoPatternMatcher(p) }

//nolint:ireturn // interface for polymorphism
func wrapBranchMatcher(p string) (Matcher, error) { return NewBranchPatternMatcher(p) }

//nolint:ireturn // interface for polymorphism
func wrapFileMatcher(p string) (Matcher, error) { return NewFilePatternMatcher(p) }

//nolint:ireturn // interface for polymorphism
func wrapContentMatcher(p string) (Matcher, error) { return NewContentPatternMatcher(p) }

//nolint:ireturn // interface for polymorphism
func wrapCommandMatcher(p string) (Matcher, error) { return NewCommandPatternMatcher(p) }

// Advanced pattern matcher factory wrappers.
//
//nolint:ireturn // interface for polymorphism
func wrapRepoMatcherWithOpts(p string, opts PatternOptions) (Matcher, error) {
	return NewRepoPatternMatcherWithOpts(p, opts)
}

//
//nolint:ireturn // interface for polymorphism
func wrapRepoMultiMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (Matcher, error) {
	return NewRepoMultiPatternMatcher(patterns, mode, opts)
}

//nolint:ireturn // interface for polymorphism
func wrapBranchMatcherWithOpts(p string, opts PatternOptions) (Matcher, error) {
	return NewBranchPatternMatcherWithOpts(p, opts)
}

//
//nolint:ireturn // interface for polymorphism
func wrapBranchMultiMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (Matcher, error) {
	return NewBranchMultiPatternMatcher(patterns, mode, opts)
}

//nolint:ireturn // interface for polymorphism
func wrapFileMatcherWithOpts(p string, opts PatternOptions) (Matcher, error) {
	return NewFilePatternMatcherWithOpts(p, opts)
}

//
//nolint:ireturn // interface for polymorphism
func wrapFileMultiMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (Matcher, error) {
	return NewFileMultiPatternMatcher(patterns, mode, opts)
}

//nolint:ireturn // interface for polymorphism
func wrapContentMatcherWithOpts(p string, opts PatternOptions) (Matcher, error) {
	return NewContentPatternMatcherWithOpts(p, opts)
}

//
//nolint:ireturn // interface for polymorphism
func wrapContentMultiMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (Matcher, error) {
	return NewContentMultiPatternMatcher(patterns, mode, opts)
}

//nolint:ireturn // interface for polymorphism
func wrapCommandMatcherWithOpts(p string, opts PatternOptions) (Matcher, error) {
	return NewCommandPatternMatcherWithOpts(p, opts)
}

//
//nolint:ireturn // interface for polymorphism
func wrapCommandMultiMatcher(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (Matcher, error) {
	return NewCommandMultiPatternMatcher(patterns, mode, opts)
}

// parsePatternMode converts a string pattern mode to MultiPatternMode.
func parsePatternMode(mode string) MultiPatternMode {
	switch strings.ToLower(mode) {
	case PatternModeAll:
		return MultiPatternAll
	default:
		return MultiPatternAny
	}
}

// BuildMatcher creates a composite matcher from RuleMatch conditions.
// Returns nil if no conditions are specified.
//
//nolint:nilnil,ireturn // returning nil, nil is intentional; interface for polymorphism
func BuildMatcher(match *RuleMatch) (Matcher, error) {
	if match == nil {
		return nil, nil
	}

	// Check if advanced pattern features are used.
	useAdvanced := match.CaseInsensitive ||
		len(match.RepoPatterns) > 0 ||
		len(match.BranchPatterns) > 0 ||
		len(match.FilePatterns) > 0 ||
		len(match.ContentPatterns) > 0 ||
		len(match.CommandPatterns) > 0

	// Use legacy builder for simple cases (backward compatibility).
	if !useAdvanced {
		return buildMatcherLegacy(match)
	}

	// Use advanced builder.
	return buildMatcherAdvanced(match)
}

// buildMatcherLegacy builds a matcher using the legacy (simple) approach.
//
//nolint:ireturn // interface for polymorphism
func buildMatcherLegacy(match *RuleMatch) (Matcher, error) {
	b := &matcherBuilder{}

	// Add simple matchers.
	if match.ValidatorType != "" {
		b.addSimple(NewValidatorTypeMatcher(match.ValidatorType))
	}

	if match.Remote != "" {
		b.addSimple(NewRemoteMatcher(match.Remote))
	}

	if match.ToolType != "" {
		b.addSimple(NewToolTypeMatcher(match.ToolType))
	}

	if match.EventType != "" {
		b.addSimple(NewEventTypeMatcher(match.EventType))
	}

	// Add pattern matchers.
	b.addPatternMatcher(match.RepoPattern, wrapRepoMatcher)
	b.addPatternMatcher(match.BranchPattern, wrapBranchMatcher)
	b.addPatternMatcher(match.FilePattern, wrapFileMatcher)
	b.addPatternMatcher(match.ContentPattern, wrapContentMatcher)
	b.addPatternMatcher(match.CommandPattern, wrapCommandMatcher)

	return b.result()
}

// buildMatcherAdvanced builds a matcher using advanced pattern features.
//
//nolint:ireturn // interface for polymorphism
func buildMatcherAdvanced(match *RuleMatch) (Matcher, error) {
	opts := PatternOptions{
		CaseInsensitive: match.CaseInsensitive,
	}
	mode := parsePatternMode(match.PatternMode)

	b := &matcherBuilder{opts: opts, mode: mode}

	// Add simple matchers.
	if match.ValidatorType != "" {
		b.addSimple(NewValidatorTypeMatcher(match.ValidatorType))
	}

	if match.Remote != "" {
		b.addSimple(NewRemoteMatcher(match.Remote))
	}

	if match.ToolType != "" {
		b.addSimple(NewToolTypeMatcher(match.ToolType))
	}

	if match.EventType != "" {
		b.addSimple(NewEventTypeMatcher(match.EventType))
	}

	// Add pattern matchers with advanced options.
	b.addAdvancedPatternMatcher(match.RepoPattern, match.RepoPatterns,
		wrapRepoMatcherWithOpts, wrapRepoMultiMatcher)
	b.addAdvancedPatternMatcher(match.BranchPattern, match.BranchPatterns,
		wrapBranchMatcherWithOpts, wrapBranchMultiMatcher)
	b.addAdvancedPatternMatcher(match.FilePattern, match.FilePatterns,
		wrapFileMatcherWithOpts, wrapFileMultiMatcher)
	b.addAdvancedPatternMatcher(match.ContentPattern, match.ContentPatterns,
		wrapContentMatcherWithOpts, wrapContentMultiMatcher)
	b.addAdvancedPatternMatcher(match.CommandPattern, match.CommandPatterns,
		wrapCommandMatcherWithOpts, wrapCommandMultiMatcher)

	return b.result()
}

// AlwaysMatcher always returns true.
type AlwaysMatcher struct{}

// Match always returns true.
func (*AlwaysMatcher) Match(*MatchContext) bool {
	return true
}

// Name returns the matcher name.
func (*AlwaysMatcher) Name() string {
	return "always"
}

// NeverMatcher always returns false.
type NeverMatcher struct{}

// Match always returns false.
func (*NeverMatcher) Match(*MatchContext) bool {
	return false
}

// Name returns the matcher name.
func (*NeverMatcher) Name() string {
	return "never"
}

// Verify interface compliance.
var (
	_ Matcher = (*RepoPatternMatcher)(nil)
	_ Matcher = (*RemoteMatcher)(nil)
	_ Matcher = (*BranchPatternMatcher)(nil)
	_ Matcher = (*FilePatternMatcher)(nil)
	_ Matcher = (*ContentPatternMatcher)(nil)
	_ Matcher = (*CommandPatternMatcher)(nil)
	_ Matcher = (*ValidatorTypeMatcher)(nil)
	_ Matcher = (*ToolTypeMatcher)(nil)
	_ Matcher = (*EventTypeMatcher)(nil)
	_ Matcher = (*CompositeMatcher)(nil)
	_ Matcher = (*AlwaysMatcher)(nil)
	_ Matcher = (*NeverMatcher)(nil)
)

// Ensure hook package is used.
var _ hook.EventType = hook.EventTypeUnknown
