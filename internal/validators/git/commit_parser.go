package git

import (
	"regexp"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// footerPattern matches git trailer format: "Token: value"
// Supports "BREAKING CHANGE" (with space) per conventional commits spec.
// Pattern ensures no trailing spaces/hyphens in token names.
// Compiled once at package initialization for efficiency.
var footerPattern = regexp.MustCompile(`^([A-Za-z0-9]+(?:[ -][A-Za-z0-9]+)*):\s*(.*)$`)

// titleRegex matches conventional commit title format: type(scope)!: description
// Capture groups: 1=type, 2=(scope) with parens, 3=scope only, 4=!, 5=description
var titleRegex = regexp.MustCompile(
	`^(\w+)(\(([a-zA-Z0-9_]+(?:[/-][a-zA-Z0-9_]+)*)\))?(!)?:\s+(.+)$`,
)

// titleParseResult holds the parsed components of a conventional commit title.
type titleParseResult struct {
	Type        string
	Scope       string
	Exclamation bool
	Description string
	Valid       bool
}

// parseTitle parses a conventional commit title into its components.
func parseTitle(title string) titleParseResult {
	matches := titleRegex.FindStringSubmatch(title)
	if matches == nil {
		return titleParseResult{Valid: false}
	}

	return titleParseResult{
		Type:        matches[1],
		Scope:       matches[3], // Group 3 is inside the optional group 2
		Exclamation: matches[4] == "!",
		Description: matches[5],
		Valid:       true,
	}
}

// ParsedCommit represents a parsed conventional commit message.
type ParsedCommit struct {
	// Type is the commit type (e.g., "feat", "fix", "chore").
	Type string

	// Scope is the optional scope (e.g., "api", "auth").
	Scope string

	// Description is the commit description.
	Description string

	// Body is the optional commit body.
	Body string

	// Footers contains any footer tokens/values.
	Footers map[string][]string

	// IsBreakingChange indicates if this is a breaking change.
	IsBreakingChange bool

	// Title is the full first line (type(scope): description).
	Title string

	// Raw is the original commit message.
	Raw string

	// Valid indicates whether the commit follows conventional commit format.
	Valid bool

	// ParseError contains the error message if parsing failed.
	ParseError string
}

// CommitParser parses conventional commit messages.
type CommitParser struct {
	validTypes map[string]bool
}

// CommitParserOption configures the CommitParser.
type CommitParserOption func(*CommitParser)

// WithValidTypes sets the allowed commit types.
func WithValidTypes(types []string) CommitParserOption {
	return func(p *CommitParser) {
		p.validTypes = make(map[string]bool, len(types))
		for _, t := range types {
			p.validTypes[t] = true
		}
	}
}

// NewCommitParser creates a new CommitParser with the given options.
func NewCommitParser(opts ...CommitParserOption) *CommitParser {
	p := &CommitParser{
		validTypes: make(map[string]bool),
	}

	// Set default valid types
	for _, t := range config.DefaultValidTypes {
		p.validTypes[t] = true
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Parse parses a commit message into a structured ParsedCommit.
func (p *CommitParser) Parse(message string) *ParsedCommit {
	result := &ParsedCommit{
		Raw: message,
	}

	if message == "" {
		return result
	}

	// Extract title (first line)
	title := extractTitle(message)
	result.Title = title

	// Check for git revert format first
	if isRevertCommit(title) {
		result.Valid = true
		result.Type = "revert"

		return result
	}

	// Parse the title line
	titleResult := parseTitle(title)
	if !titleResult.Valid {
		result.ParseError = "failed to parse as conventional commit"

		return result
	}

	// Extract parsed fields
	result.Type = titleResult.Type
	result.Scope = titleResult.Scope
	result.Description = titleResult.Description
	result.IsBreakingChange = titleResult.Exclamation

	// Extract body and footers (also sets IsBreakingChange if footer found)
	p.extractBodyAndFooters(message, result)

	// Validate type against allowed types
	if len(p.validTypes) > 0 && !p.validTypes[result.Type] {
		result.ParseError = "invalid commit type: " + result.Type
		result.Valid = false

		return result
	}

	result.Valid = true

	return result
}

// extractBodyAndFooters extracts body and footers from the full message.
// It populates the Footers map and detects BREAKING CHANGE footers.
func (*CommitParser) extractBodyAndFooters(message string, result *ParsedCommit) {
	lines := strings.Split(message, "\n")
	if len(lines) <= 1 {
		return
	}

	// Skip title and any blank lines after it
	bodyStartIdx := 1
	for bodyStartIdx < len(lines) && strings.TrimSpace(lines[bodyStartIdx]) == "" {
		bodyStartIdx++
	}

	if bodyStartIdx >= len(lines) {
		return
	}

	bodyLines := lines[bodyStartIdx:]

	// Find where footers start (scanning backwards from the end)
	footerStartIdx := findFooterStartIndex(bodyLines)

	// Extract footers if found
	if footerStartIdx < len(bodyLines) {
		extractFootersFromLines(bodyLines[footerStartIdx:], result)
		result.Body = strings.TrimRight(strings.Join(bodyLines[:footerStartIdx], "\n"), "\n")
	} else {
		result.Body = strings.Join(bodyLines, "\n")
	}
}

// findFooterStartIndex scans backwards to find where git trailers start in the body.
//
// Git trailers typically appear at the end, separated from the body by a blank line.
// However, if all body lines match the footer pattern (no blank line separator), we
// treat the entire body as footers to handle edge cases where commits contain only
// trailers without body text.
func findFooterStartIndex(bodyLines []string) int {
	footerStartIdx := len(bodyLines)
	foundNonFooter := false

	for i := len(bodyLines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(bodyLines[i])
		if line == "" {
			// Blank line marks the boundary
			footerStartIdx = i + 1
			break
		}

		if !footerPattern.MatchString(line) {
			// Non-footer line found
			footerStartIdx = i + 1
			foundNonFooter = true

			break
		}
	}

	// If we scanned all lines and they all matched the footer pattern,
	// treat the entire body as footers (footerStartIdx remains 0 from the loop)
	if !foundNonFooter && footerStartIdx == len(bodyLines) {
		footerStartIdx = 0
	}

	return footerStartIdx
}

// extractFootersFromLines parses footer lines and populates the result's Footers map.
func extractFootersFromLines(footerLines []string, result *ParsedCommit) {
	if result.Footers == nil {
		result.Footers = make(map[string][]string)
	}

	const expectedFooterMatches = 3 // full match + 2 capture groups

	for _, line := range footerLines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		matches := footerPattern.FindStringSubmatch(trimmedLine)
		if len(matches) != expectedFooterMatches {
			continue
		}

		token := matches[1]
		value := matches[2]
		result.Footers[token] = append(result.Footers[token], value)

		// Check for breaking change markers
		if token == "BREAKING CHANGE" || token == "BREAKING-CHANGE" {
			result.IsBreakingChange = true
		}
	}
}

// IsValidType checks if a type is in the valid types list.
func (p *CommitParser) IsValidType(commitType string) bool {
	if len(p.validTypes) == 0 {
		return true
	}

	return p.validTypes[commitType]
}

// GetValidTypes returns the list of valid types.
func (p *CommitParser) GetValidTypes() []string {
	types := make([]string, 0, len(p.validTypes))
	for t := range p.validTypes {
		types = append(types, t)
	}

	return types
}

// extractTitle extracts the first non-empty line from a message.
func extractTitle(message string) string {
	// Find the first newline or end of string
	for i, c := range message {
		if c == '\n' {
			return message[:i]
		}
	}

	return message
}

// conventionalTitleRegex matches conventional commit title format.
var conventionalTitleRegex = regexp.MustCompile(`^(\w+)(\([a-zA-Z0-9_\/-]+\))?!?: .+`)

// HasValidFormat checks if a title matches the conventional commit format.
func HasValidFormat(title string) bool {
	return conventionalTitleRegex.MatchString(title)
}
