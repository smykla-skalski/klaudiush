package git

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/validator"
)

// RuleResult contains the result of a rule validation including reference.
type RuleResult struct {
	// Errors contains the error messages from this rule.
	Errors []string

	// Reference is the URL that uniquely identifies this type of validation failure.
	Reference validator.Reference
}

// CommitRule represents a validation rule for commit messages.
type CommitRule interface {
	// Name returns the rule name.
	Name() string

	// Validate checks the commit against the rule and returns a RuleResult.
	Validate(commit *ParsedCommit, message string) *RuleResult
}

// TitleLengthRule validates the commit title length.
type TitleLengthRule struct {
	MaxLength                 int
	AllowUnlimitedRevertTitle bool
}

func (*TitleLengthRule) Name() string {
	return "title-length"
}

func (r *TitleLengthRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	// Skip length validation for revert commits if configured
	if r.AllowUnlimitedRevertTitle && isRevertCommit(commit.Title) {
		return nil
	}

	if len(commit.Title) <= r.MaxLength {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitBadTitle,
		Errors: []string{
			fmt.Sprintf(
				"❌ Title exceeds %d characters (%d chars): '%s'",
				r.MaxLength,
				len(commit.Title),
				commit.Title,
			),
			"   Note: Revert commits (Revert \"...\") are exempt from this limit",
		},
	}
}

// ConventionalFormatRule validates conventional commit format.
type ConventionalFormatRule struct {
	ValidTypes   []string
	RequireScope bool
}

func (*ConventionalFormatRule) Name() string {
	return "conventional-format"
}

func (r *ConventionalFormatRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	// Skip validation for revert commits
	if isRevertCommit(commit.Title) {
		return nil
	}

	if !commit.Valid || commit.ParseError != "" {
		errors := []string{
			"❌ Title doesn't follow conventional commits format: type(scope): description",
		}

		if r.RequireScope {
			errors = append(errors, "   Scope is mandatory and must be in parentheses")
		}

		errors = append(errors, "   Valid types: "+strings.Join(r.ValidTypes, ", "))
		errors = append(errors, "   Alternative: Revert \"original commit title\"")
		errors = append(errors, fmt.Sprintf("   Current title: '%s'", commit.Title))

		return &RuleResult{
			Reference: validator.RefGitConventionalCommit,
			Errors:    errors,
		}
	}

	// Check scope requirement
	if r.RequireScope && commit.Scope == "" {
		return &RuleResult{
			Reference: validator.RefGitConventionalCommit,
			Errors: []string{
				"❌ Title doesn't follow conventional commits format: type(scope): description",
				"   Scope is mandatory and must be in parentheses",
				"   Valid types: " + strings.Join(r.ValidTypes, ", "),
				"   Alternative: Revert \"original commit title\"",
				fmt.Sprintf("   Current title: '%s'", commit.Title),
			},
		}
	}

	return nil
}

// InfraScopeMisuseRule blocks feat/fix with infrastructure scopes.
type InfraScopeMisuseRule struct {
	infraScopeMisuseRegex *regexp.Regexp
}

func NewInfraScopeMisuseRule() *InfraScopeMisuseRule {
	return &InfraScopeMisuseRule{
		infraScopeMisuseRegex: regexp.MustCompile(`^(feat|fix)\((ci|test|docs|build)\):`),
	}
}

func (*InfraScopeMisuseRule) Name() string {
	return "infra-scope-misuse"
}

func (r *InfraScopeMisuseRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	if !r.infraScopeMisuseRegex.MatchString(commit.Title) {
		return nil
	}

	matches := r.infraScopeMisuseRegex.FindStringSubmatch(commit.Title)

	const minMatchGroups = 3 // Full match + type + scope groups

	if len(matches) < minMatchGroups {
		return nil
	}

	typeMatch := matches[1]  // feat or fix
	scopeMatch := matches[2] // ci, test, docs, or build

	return &RuleResult{
		Reference: validator.RefGitFeatCI,
		Errors: []string{
			fmt.Sprintf(
				"❌ Use '%s(...)' not '%s(%s)' for infrastructure changes",
				scopeMatch,
				typeMatch,
				scopeMatch,
			),
			"   feat/fix should only be used for user-facing changes",
		},
	}
}

// BodyLineLengthRule validates body line lengths.
type BodyLineLengthRule struct {
	MaxLength int
	Tolerance int
	urlRegex  *regexp.Regexp
}

func NewBodyLineLengthRule(maxLength, tolerance int) *BodyLineLengthRule {
	return &BodyLineLengthRule{
		MaxLength: maxLength,
		Tolerance: tolerance,
		urlRegex:  regexp.MustCompile(`https?://`),
	}
}

func (*BodyLineLengthRule) Name() string {
	return "body-line-length"
}

func (r *BodyLineLengthRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	lines := strings.Split(message, "\n")
	errors := make([]string, 0)
	maxLenWithTolerance := r.MaxLength + r.Tolerance

	for lineNum, line := range lines {
		// Skip title (first line)
		if lineNum == 0 {
			continue
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Allow URLs to break the rule
		if r.urlRegex.MatchString(line) {
			continue
		}

		lineLen := len(line)
		if lineLen > maxLenWithTolerance {
			truncated := truncateLine(line)
			errors = append(errors,
				fmt.Sprintf(
					"❌ Line %d exceeds %d characters (%d chars, >%d over limit)",
					lineNum+1,
					r.MaxLength,
					lineLen,
					r.Tolerance,
				),
				fmt.Sprintf("   Line: '%s'", truncated),
			)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitBadBody,
		Errors:    errors,
	}
}

// ListFormattingRule validates list item formatting.
type ListFormattingRule struct {
	listItemRegex *regexp.Regexp
}

func NewListFormattingRule() *ListFormattingRule {
	return &ListFormattingRule{
		listItemRegex: regexp.MustCompile(`^\s*[-*]\s+|\s*[0-9]+\.\s+`),
	}
}

func (*ListFormattingRule) Name() string {
	return "list-formatting"
}

func (r *ListFormattingRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	lines := strings.Split(message, "\n")
	errors := make([]string, 0)
	prevLineEmpty := false
	foundFirstList := false

	for lineNum, line := range lines {
		// Skip title (first line)
		if lineNum == 0 {
			continue
		}

		// Check if blank line
		if strings.TrimSpace(line) == "" {
			prevLineEmpty = true

			continue
		}

		// Check for list items
		if r.listItemRegex.MatchString(line) {
			// Check if this is the first list item and there was no empty line before it
			if !foundFirstList && !prevLineEmpty {
				truncated := truncateLine(line)
				errors = append(
					errors,
					fmt.Sprintf(
						"❌ Missing empty line before first list item at line %d",
						lineNum+1,
					),
					"   List items must be preceded by an empty line",
					fmt.Sprintf("   Line: '%s'", truncated),
				)
			}

			foundFirstList = true
		}

		prevLineEmpty = false
	}

	if len(errors) == 0 {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitListFormat,
		Errors:    errors,
	}
}

// PRReferenceRule blocks PR references in commit messages.
type PRReferenceRule struct {
	prReferenceRegex *regexp.Regexp
	hashRefRegex     *regexp.Regexp
	urlRefRegex      *regexp.Regexp
}

func NewPRReferenceRule() *PRReferenceRule {
	return &PRReferenceRule{
		prReferenceRegex: regexp.MustCompile(
			`#[0-9]{1,10}\b|(?:^|://|[^/a-zA-Z0-9])github\.com/[^/]+/[^/]+/pull/[0-9]{1,10}\b`,
		),
		hashRefRegex: regexp.MustCompile(`#[0-9]{1,10}\b`),
		urlRefRegex: regexp.MustCompile(
			`(?:^|://|[^/a-zA-Z0-9])github\.com/[^/]+/[^/]+/pull/[0-9]{1,10}\b`,
		),
	}
}

func (*PRReferenceRule) Name() string {
	return "pr-reference"
}

func (r *PRReferenceRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if !r.prReferenceRegex.MatchString(message) {
		return nil
	}

	errors := []string{"❌ PR references found - remove '#' prefix or convert URLs to plain numbers"}

	// Show examples for hash references
	if hashMatch := r.hashRefRegex.FindString(message); hashMatch != "" {
		fix := strings.TrimPrefix(hashMatch, "#")
		errors = append(errors, fmt.Sprintf("   Found: '%s' → Should be: '%s'", hashMatch, fix))
	}

	// Show examples for URL references
	if urlMatch := r.urlRefRegex.FindString(message); urlMatch != "" {
		prNumRegex := regexp.MustCompile(`[0-9]{1,10}$`)
		prNum := prNumRegex.FindString(urlMatch)

		// Strip any prefix captured by the anchor pattern (e.g., "://", space, etc.)
		cleanURL := urlMatch
		if idx := strings.Index(urlMatch, "github.com"); idx > 0 {
			cleanURL = urlMatch[idx:]
		}

		errors = append(
			errors,
			fmt.Sprintf("   Found: 'https://%s' → Should be: '%s'", cleanURL, prNum),
		)
	}

	return &RuleResult{
		Reference: validator.RefGitPRRef,
		Errors:    errors,
	}
}

// AIAttributionRule blocks Claude AI attribution patterns.
type AIAttributionRule struct{}

func NewAIAttributionRule() *AIAttributionRule {
	return &AIAttributionRule{}
}

func (*AIAttributionRule) Name() string {
	return "ai-attribution"
}

func (*AIAttributionRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if !containsClaudeAIAttribution(message) {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitClaudeAttr,
		Errors: []string{
			"❌ Commit message contains AI attribution - remove any AI generation attribution",
		},
	}
}

// ForbiddenPatternRule blocks forbidden patterns in commit messages.
type ForbiddenPatternRule struct {
	Patterns []string
}

func (*ForbiddenPatternRule) Name() string {
	return "forbidden-pattern"
}

func (r *ForbiddenPatternRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if len(r.Patterns) == 0 {
		return nil
	}

	errors := make([]string, 0)

	for _, pattern := range r.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}

		if re.MatchString(message) {
			match := re.FindString(message)
			errors = append(errors,
				fmt.Sprintf("❌ Forbidden pattern found: '%s'", match),
				"   Pattern: "+pattern,
				"   Remove this content from your commit message",
			)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitForbiddenPattern,
		Errors:    errors,
	}
}

// SignoffRule validates the Signed-off-by trailer.
type SignoffRule struct {
	ExpectedSignoff string
}

func (*SignoffRule) Name() string {
	return "signoff"
}

func (r *SignoffRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if r.ExpectedSignoff == "" {
		return nil
	}

	if !strings.Contains(message, "Signed-off-by:") {
		return nil
	}

	lines := strings.Split(message, "\n")
	signoffLine := ""

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Signed-off-by:") {
			signoffLine = strings.TrimSpace(line)

			break
		}
	}

	expectedSignoffLine := "Signed-off-by: " + r.ExpectedSignoff
	if signoffLine != expectedSignoffLine {
		return &RuleResult{
			Reference: validator.RefGitSignoffMismatch,
			Errors: []string{
				"❌ Wrong signoff identity",
				"   Found: " + signoffLine,
				"   Expected: " + expectedSignoffLine,
			},
		}
	}

	return nil
}

// containsClaudeAIAttribution checks for AI attribution patterns.
func containsClaudeAIAttribution(message string) bool {
	lower := strings.ToLower(message)

	// Explicit AI attribution patterns to block
	aiPatterns := []string{
		"generated by claude",
		"generated with claude",
		"assisted by claude",
		"created by claude",
		"written by claude",
		"with help from claude",
		"powered by claude",
		"claude ai",
		"co-authored-by: claude",
	}

	for _, pattern := range aiPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// Check for markdown links with Claude attribution
	if regexp.MustCompile(`\[claude[^\]]*\]\([^)]*claude[^)]*\)`).MatchString(lower) {
		return true
	}

	// If "claude" doesn't appear at all, it's fine
	if !strings.Contains(lower, "claude") {
		return false
	}

	// Allow legitimate tool/file references
	legitimatePatterns := []string{
		"claude.md",
		"claude-hooks",
		"klaudiush",
		"`claude",
		"claude`",
	}

	for _, pattern := range legitimatePatterns {
		if strings.Contains(lower, pattern) {
			return false
		}
	}

	// Check for CLAUDE (all caps) - this is usually the file name
	if strings.Contains(message, "CLAUDE") {
		return false
	}

	return false
}
