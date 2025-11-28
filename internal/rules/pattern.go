package rules

import (
	"regexp"
	"strings"
	"sync"

	"github.com/gobwas/glob"
)

// PatternType indicates whether a pattern is a glob or regex.
type PatternType int

const (
	// PatternTypeGlob indicates a glob pattern (e.g., "*/kong/*").
	PatternTypeGlob PatternType = iota

	// PatternTypeRegex indicates a regex pattern (e.g., "^.*/kong/.*$").
	PatternTypeRegex
)

// regexIndicators are strings that indicate a pattern is regex rather than glob.
var regexIndicators = []string{
	"^",   // Start anchor
	"$",   // End anchor
	"(?",  // Non-capturing group or flags
	"\\d", // Digit class
	"\\w", // Word class
	"\\s", // Whitespace class
	"\\b", // Word boundary
	"[",   // Character class start
	"]",   // Character class end
	"(",   // Capturing group start
	")",   // Capturing group end
	"|",   // Alternation
	"+",   // One or more quantifier
	".*",  // Wildcard in regex
	".+",  // One or more any
	"\\.", // Escaped dot
}

// DetectPatternType determines whether a pattern is a glob or regex.
// Returns PatternTypeRegex if the pattern contains regex-specific syntax,
// otherwise returns PatternTypeGlob.
func DetectPatternType(pattern string) PatternType {
	for _, indicator := range regexIndicators {
		if strings.Contains(pattern, indicator) {
			return PatternTypeRegex
		}
	}

	return PatternTypeGlob
}

// GlobPattern wraps a compiled glob pattern.
type GlobPattern struct {
	pattern  string
	compiled glob.Glob
}

// NewGlobPattern creates a new GlobPattern from the given pattern string.
func NewGlobPattern(pattern string) (*GlobPattern, error) {
	compiled, err := glob.Compile(pattern, '/')
	if err != nil {
		return nil, err
	}

	return &GlobPattern{
		pattern:  pattern,
		compiled: compiled,
	}, nil
}

// Match returns true if the string matches the glob pattern.
func (p *GlobPattern) Match(s string) bool {
	return p.compiled.Match(s)
}

// String returns the original pattern string.
func (p *GlobPattern) String() string {
	return p.pattern
}

// RegexPattern wraps a compiled regular expression.
type RegexPattern struct {
	pattern  string
	compiled *regexp.Regexp
}

// NewRegexPattern creates a new RegexPattern from the given pattern string.
func NewRegexPattern(pattern string) (*RegexPattern, error) {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	return &RegexPattern{
		pattern:  pattern,
		compiled: compiled,
	}, nil
}

// Match returns true if the string matches the regex pattern.
func (p *RegexPattern) Match(s string) bool {
	return p.compiled.MatchString(s)
}

// String returns the original pattern string.
func (p *RegexPattern) String() string {
	return p.pattern
}

// CompilePattern compiles a pattern string, auto-detecting the pattern type.
// Supports negation via ! prefix (e.g., "!*.tmp" matches anything except *.tmp).
// Returns the compiled Pattern or an error if compilation fails.
//
//nolint:ireturn // interface for polymorphism
func CompilePattern(pattern string) (Pattern, error) {
	// Handle negated patterns.
	negated := IsNegated(pattern)
	if negated {
		pattern = StripNegation(pattern)
	}

	patternType := DetectPatternType(pattern)

	var compiled Pattern

	var err error

	switch patternType {
	case PatternTypeRegex:
		compiled, err = NewRegexPattern(pattern)
	default:
		compiled, err = NewGlobPattern(pattern)
	}

	if err != nil {
		return nil, err
	}

	// Wrap in NegatedPattern if needed.
	if negated {
		return NewNegatedPattern(compiled), nil
	}

	return compiled, nil
}

// PatternCache provides thread-safe caching of compiled patterns.
type PatternCache struct {
	mu       sync.RWMutex
	patterns map[string]Pattern
	errors   map[string]error
}

// NewPatternCache creates a new PatternCache.
func NewPatternCache() *PatternCache {
	return &PatternCache{
		patterns: make(map[string]Pattern),
		errors:   make(map[string]error),
	}
}

// Get returns a compiled pattern, compiling and caching it if necessary.
// Returns the cached error if the pattern previously failed to compile.
//
//nolint:ireturn // interface for polymorphism
func (c *PatternCache) Get(patternStr string) (Pattern, error) {
	// Fast path: check if already cached.
	c.mu.RLock()

	if p, ok := c.patterns[patternStr]; ok {
		c.mu.RUnlock()
		return p, nil
	}

	if err, ok := c.errors[patternStr]; ok {
		c.mu.RUnlock()
		return nil, err
	}

	c.mu.RUnlock()

	// Slow path: compile and cache.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	if p, ok := c.patterns[patternStr]; ok {
		return p, nil
	}

	if err, ok := c.errors[patternStr]; ok {
		return nil, err
	}

	pattern, err := CompilePattern(patternStr)
	if err != nil {
		c.errors[patternStr] = err
		return nil, err
	}

	c.patterns[patternStr] = pattern

	return pattern, nil
}

// Clear removes all cached patterns.
func (c *PatternCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.patterns = make(map[string]Pattern)
	c.errors = make(map[string]error)
}

// Size returns the number of cached patterns.
func (c *PatternCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.patterns)
}

// PatternOptions configures pattern compilation behavior.
type PatternOptions struct {
	// CaseInsensitive enables case-insensitive matching.
	CaseInsensitive bool

	// Negate inverts the match result.
	Negate bool
}

// NegatedPattern wraps a pattern and inverts its match result.
type NegatedPattern struct {
	inner Pattern
}

// NewNegatedPattern creates a pattern that matches when the inner pattern does not.
func NewNegatedPattern(inner Pattern) *NegatedPattern {
	return &NegatedPattern{inner: inner}
}

// Match returns true if the inner pattern does NOT match.
func (p *NegatedPattern) Match(s string) bool {
	return !p.inner.Match(s)
}

// String returns the original pattern string with ! prefix.
func (p *NegatedPattern) String() string {
	return "!" + p.inner.String()
}

// IsNegated returns true if the pattern string starts with !.
func IsNegated(pattern string) bool {
	return strings.HasPrefix(pattern, "!")
}

// StripNegation removes the ! prefix from a pattern string.
func StripNegation(pattern string) string {
	return strings.TrimPrefix(pattern, "!")
}

// CaseInsensitivePattern wraps a pattern and performs case-insensitive matching.
type CaseInsensitivePattern struct {
	inner   Pattern
	pattern string
}

// NewCaseInsensitivePattern creates a pattern that matches case-insensitively.
func NewCaseInsensitivePattern(inner Pattern, originalPattern string) *CaseInsensitivePattern {
	return &CaseInsensitivePattern{inner: inner, pattern: originalPattern}
}

// Match returns true if the lowercased string matches the lowercased pattern.
func (p *CaseInsensitivePattern) Match(s string) bool {
	return p.inner.Match(strings.ToLower(s))
}

// String returns the original pattern string.
func (p *CaseInsensitivePattern) String() string {
	return p.pattern
}

// NewCaseInsensitiveGlobPattern creates a case-insensitive glob pattern.
func NewCaseInsensitiveGlobPattern(pattern string) (*CaseInsensitivePattern, error) {
	// Compile the lowercased pattern for case-insensitive matching.
	lowerPattern := strings.ToLower(pattern)

	compiled, err := glob.Compile(lowerPattern, '/')
	if err != nil {
		return nil, err
	}

	inner := &GlobPattern{pattern: lowerPattern, compiled: compiled}

	return NewCaseInsensitivePattern(inner, pattern), nil
}

// MultiPatternMode specifies how multiple patterns are combined.
type MultiPatternMode int

const (
	// MultiPatternAny requires at least one pattern to match (OR logic).
	MultiPatternAny MultiPatternMode = iota

	// MultiPatternAll requires all patterns to match (AND logic).
	MultiPatternAll
)

// Pattern mode string constants.
const (
	PatternModeAny = "any"
	PatternModeAll = "all"
)

// MultiPattern combines multiple patterns with any/all logic.
type MultiPattern struct {
	patterns []Pattern
	mode     MultiPatternMode
	repr     string
}

// NewMultiPattern creates a pattern that matches against multiple sub-patterns.
func NewMultiPattern(patterns []Pattern, mode MultiPatternMode, repr string) *MultiPattern {
	return &MultiPattern{patterns: patterns, mode: mode, repr: repr}
}

// Match returns true based on the mode:
// - MultiPatternAny: true if at least one pattern matches
// - MultiPatternAll: true if all patterns match
func (p *MultiPattern) Match(s string) bool {
	if len(p.patterns) == 0 {
		return true
	}

	switch p.mode {
	case MultiPatternAny:
		for _, pattern := range p.patterns {
			if pattern.Match(s) {
				return true
			}
		}

		return false

	case MultiPatternAll:
		for _, pattern := range p.patterns {
			if !pattern.Match(s) {
				return false
			}
		}

		return true

	default:
		return false
	}
}

// String returns a representation of all patterns.
func (p *MultiPattern) String() string {
	return p.repr
}

// CompileMultiPattern compiles multiple pattern strings into a single MultiPattern.
//
//nolint:ireturn // interface for polymorphism
func CompileMultiPattern(
	patterns []string,
	mode MultiPatternMode,
	opts PatternOptions,
) (Pattern, error) {
	if len(patterns) == 0 {
		return nil, nil //nolint:nilnil // no patterns is valid
	}

	// Single pattern doesn't need MultiPattern wrapper.
	if len(patterns) == 1 {
		return CompilePatternWithOptions(patterns[0], opts)
	}

	compiled := make([]Pattern, 0, len(patterns))

	for _, p := range patterns {
		pattern, err := CompilePatternWithOptions(p, opts)
		if err != nil {
			return nil, err
		}

		compiled = append(compiled, pattern)
	}

	// Build string representation.
	modeStr := PatternModeAny
	if mode == MultiPatternAll {
		modeStr = PatternModeAll
	}

	repr := modeStr + "(" + strings.Join(patterns, ", ") + ")"

	return NewMultiPattern(compiled, mode, repr), nil
}

// CompilePatternWithOptions compiles a pattern with additional options.
// Supports negation via ! prefix and case-insensitive matching via options.
//
//nolint:ireturn // interface for polymorphism
func CompilePatternWithOptions(pattern string, opts PatternOptions) (Pattern, error) {
	// Handle negated patterns (both from prefix and options).
	negated := opts.Negate || IsNegated(pattern)
	if IsNegated(pattern) {
		pattern = StripNegation(pattern)
	}

	patternType := DetectPatternType(pattern)

	var compiled Pattern

	var err error

	switch patternType {
	case PatternTypeRegex:
		// For regex, add (?i) flag if case-insensitive.
		if opts.CaseInsensitive && !strings.HasPrefix(pattern, "(?i)") {
			pattern = "(?i)" + pattern
		}

		compiled, err = NewRegexPattern(pattern)

	default:
		// For glob, use case-insensitive wrapper.
		if opts.CaseInsensitive {
			compiled, err = NewCaseInsensitiveGlobPattern(pattern)
		} else {
			compiled, err = NewGlobPattern(pattern)
		}
	}

	if err != nil {
		return nil, err
	}

	// Wrap in NegatedPattern if needed.
	if negated {
		return NewNegatedPattern(compiled), nil
	}

	return compiled, nil
}

// defaultCache is the global pattern cache.
var defaultCache = NewPatternCache()

// GetCachedPattern returns a compiled pattern from the default cache.
//
//nolint:ireturn // interface for polymorphism
func GetCachedPattern(pattern string) (Pattern, error) {
	return defaultCache.Get(pattern)
}

// ClearPatternCache clears the default pattern cache.
func ClearPatternCache() {
	defaultCache.Clear()
}
