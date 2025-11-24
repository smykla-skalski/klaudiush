package git

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators"
)

const (
	// Default values for commit message validation
	defaultMaxTitleLength    = 50
	defaultMaxBodyLineLength = 72
	defaultBodyLineTolerance = 5  // Additional tolerance beyond max body line length
	truncateErrorLineAt      = 60 // Truncate long lines in error messages for readability
)

var (
	// defaultValidTypes from commitlint config-conventional
	defaultValidTypes = []string{
		"build",
		"chore",
		"ci",
		"docs",
		"feat",
		"fix",
		"perf",
		"refactor",
		"revert",
		"style",
		"test",
	}

	// Infrastructure scope misuse: feat(ci), fix(test), etc.
	infraScopeMisuseRegex = regexp.MustCompile(`^(feat|fix)\((ci|test|docs|build)\):`)

	// PR references: #123 or GitHub URLs
	prReferenceRegex = regexp.MustCompile(`#[0-9]+|github\.com/.+/pull/[0-9]+`)

	// List item patterns
	listItemRegex = regexp.MustCompile(`^\s*[-*]\s+|\s*[0-9]+\.\s+`)

	// URL pattern (to allow long lines with URLs)
	urlRegex = regexp.MustCompile(`https?://`)
)

// validateMessage validates the commit message content
func (v *CommitValidator) validateMessage(message string) *validator.Result {
	log := v.Logger()
	log.Debug("Validating commit message", "length", len(message))

	errors := make([]string, 0)

	// Check for Claude AI attribution (if enabled)
	if v.shouldBlockAIAttribution() && v.containsClaudeAIAttribution(message) {
		errors = append(
			errors,
			"❌ Commit message contains AI attribution - remove any AI generation attribution",
		)
	}

	// Split message into lines
	lines := strings.Split(message, "\n")

	// Validate title
	title, titleErrors := v.validateTitle(lines)
	errors = append(errors, titleErrors...)

	if title == "" {
		return validator.Fail("Commit message is empty")
	}

	// Validate body and additional checks
	bodyErrors := v.validateBodyAndChecks(lines, message)
	errors = append(errors, bodyErrors...)

	// Report errors if any
	if len(errors) > 0 {
		return v.buildErrorResult(errors, message)
	}

	log.Debug("Commit message validation passed")

	return validator.Pass()
}

// validateTitle extracts and validates the commit title
func (v *CommitValidator) validateTitle(lines []string) (string, []string) {
	errors := make([]string, 0)

	// Get title (first non-empty line)
	title := ""

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			title = line
			break
		}
	}

	if title == "" {
		return "", errors
	}

	// Check title length
	maxLen := v.getTitleMaxLength()
	if len(title) > maxLen {
		errors = append(
			errors,
			fmt.Sprintf(
				"❌ Title exceeds %d characters (%d chars): '%s'",
				maxLen,
				len(title),
				title,
			),
		)
	}

	// Check conventional commit format (if enabled)
	if v.shouldCheckConventionalCommits() {
		validTypes := v.getValidTypes()
		requireScope := v.shouldRequireScope()

		// Build regex pattern based on configuration
		pattern := v.buildConventionalCommitPattern(validTypes, requireScope)
		conventionalCommitRegex := regexp.MustCompile(pattern)

		if !conventionalCommitRegex.MatchString(title) {
			errors = append(
				errors,
				"❌ Title doesn't follow conventional commits format: type(scope): description",
			)
			if requireScope {
				errors = append(errors, "   Scope is mandatory and must be in parentheses")
			}

			errors = append(errors, "   Valid types: "+strings.Join(validTypes, ", "))
			errors = append(errors, fmt.Sprintf("   Current title: '%s'", title))
		}
	}

	// Check for feat/fix misuse with infrastructure scopes (if enabled)
	if v.shouldBlockInfraScopeMisuse() {
		infraErrors := v.checkInfraScopeMisuse(title)
		errors = append(errors, infraErrors...)
	}

	return title, errors
}

// validateBodyAndChecks validates body lines, markdown, PR references, and signoff
func (v *CommitValidator) validateBodyAndChecks(lines []string, message string) []string {
	errors := make([]string, 0)

	// Validate body lines
	bodyErrors := v.validateBodyLines(lines)
	errors = append(errors, bodyErrors...)

	// Validate markdown formatting in body
	if len(lines) > 1 {
		markdownErrors := v.validateMarkdownInBody(lines)
		errors = append(errors, markdownErrors...)
	}

	// Check for PR references (if enabled)
	if v.shouldBlockPRReferences() {
		prErrors := v.checkPRReferences(message)
		errors = append(errors, prErrors...)
	}

	// Check Signed-off-by trailer (if configured)
	expectedSignoff := v.getExpectedSignoff()
	if expectedSignoff != "" && strings.Contains(message, "Signed-off-by:") {
		signoffErrors := v.validateSignoff(lines, expectedSignoff)
		errors = append(errors, signoffErrors...)
	}

	return errors
}

// validateMarkdownInBody validates markdown formatting in the commit body
func (*CommitValidator) validateMarkdownInBody(lines []string) []string {
	// Extract body (skip title and empty line after title)
	bodyStartIdx := 1
	if bodyStartIdx < len(lines) && strings.TrimSpace(lines[bodyStartIdx]) == "" {
		bodyStartIdx++
	}

	if bodyStartIdx >= len(lines) {
		return nil
	}

	body := strings.Join(lines[bodyStartIdx:], "\n")
	markdownResult := validators.AnalyzeMarkdown(body, nil)

	return markdownResult.Warnings
}

// validateSignoff checks the Signed-off-by trailer
func (*CommitValidator) validateSignoff(lines []string, expectedSignoff string) []string {
	errors := make([]string, 0)
	signoffLine := ""

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Signed-off-by:") {
			signoffLine = strings.TrimSpace(line)
			break
		}
	}

	expectedSignoffLine := "Signed-off-by: " + expectedSignoff
	if signoffLine != expectedSignoffLine {
		errors = append(errors, "❌ Wrong signoff identity")
		errors = append(errors, "   Found: "+signoffLine)
		errors = append(errors, "   Expected: "+expectedSignoffLine)
	}

	return errors
}

// buildErrorResult constructs the error result with details
func (*CommitValidator) buildErrorResult(errors []string, message string) *validator.Result {
	var details strings.Builder
	for _, err := range errors {
		details.WriteString(err)
		details.WriteString("\n")
	}

	details.WriteString("\n📝 Commit message:\n")
	details.WriteString("---\n")
	details.WriteString(message)
	details.WriteString("\n---")

	return validator.Fail("Commit message validation failed").AddDetail("errors", details.String())
}

// validateBodyLines validates the body lines of the commit message
func (v *CommitValidator) validateBodyLines(lines []string) []string {
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

		// Process body lines (all non-empty lines after title)

		// Check for list items
		if listItemRegex.MatchString(line) {
			// Check if this is the first list item and there was no empty line before it
			if !foundFirstList && !prevLineEmpty {
				errors = append(errors, v.formatListItemError(line, lineNum)...)
			}

			foundFirstList = true
		}

		lineLen := len(line)

		// Allow URLs to break the rule
		if urlRegex.MatchString(line) {
			prevLineEmpty = false
			continue
		}

		// Check line length with tolerance
		maxLen := v.getBodyMaxLineLength()
		tolerance := v.getBodyLineTolerance()
		maxLenWithTolerance := maxLen + tolerance

		if lineLen > maxLenWithTolerance {
			errors = append(errors, v.formatLineLengthError(line, lineNum, lineLen, maxLen)...)
		}

		prevLineEmpty = false
	}

	return errors
}

// formatListItemError formats error messages for list items missing empty line before
func (*CommitValidator) formatListItemError(line string, lineNum int) []string {
	truncated := truncateLine(line)

	return []string{
		fmt.Sprintf("❌ Missing empty line before first list item at line %d", lineNum+1),
		"   List items must be preceded by an empty line",
		fmt.Sprintf("   Line: '%s'", truncated),
	}
}

// formatLineLengthError formats error messages for lines exceeding length limit
func (v *CommitValidator) formatLineLengthError(
	line string,
	lineNum, lineLen, maxLen int,
) []string {
	truncated := truncateLine(line)
	tolerance := v.getBodyLineTolerance()

	return []string{
		fmt.Sprintf(
			"❌ Line %d exceeds %d characters (%d chars, >%d over limit)",
			lineNum+1,
			maxLen,
			lineLen,
			tolerance,
		),
		fmt.Sprintf("   Line: '%s'", truncated),
	}
}

// truncateLine truncates a line for display in error messages
func truncateLine(line string) string {
	if len(line) > truncateErrorLineAt {
		return line[:truncateErrorLineAt] + "..."
	}

	return line
}

// checkInfraScopeMisuse checks for feat/fix misuse with infrastructure scopes
func (*CommitValidator) checkInfraScopeMisuse(title string) []string {
	if !infraScopeMisuseRegex.MatchString(title) {
		return nil
	}

	matches := infraScopeMisuseRegex.FindStringSubmatch(title)

	const minMatchGroups = 3 // Full match + type + scope groups

	if len(matches) < minMatchGroups {
		return nil
	}

	typeMatch := matches[1]  // feat or fix
	scopeMatch := matches[2] // ci, test, docs, or build

	return []string{
		fmt.Sprintf(
			"❌ Use '%s(...)' not '%s(%s)' for infrastructure changes",
			scopeMatch,
			typeMatch,
			scopeMatch,
		),
		"   feat/fix should only be used for user-facing changes",
	}
}

// checkPRReferences checks for PR references in the message
func (*CommitValidator) checkPRReferences(message string) []string {
	if !prReferenceRegex.MatchString(message) {
		return nil
	}

	errors := []string{"❌ PR references found - remove '#' prefix or convert URLs to plain numbers"}

	// Show examples for hash references
	if hashMatch := regexp.MustCompile(`#[0-9]+`).FindString(message); hashMatch != "" {
		fix := strings.TrimPrefix(hashMatch, "#")
		errors = append(errors, fmt.Sprintf("   Found: '%s' → Should be: '%s'", hashMatch, fix))
	}

	// Show examples for URL references
	if urlMatch := regexp.MustCompile(`github\.com/.+/pull/[0-9]+`).FindString(message); urlMatch != "" {
		prNum := regexp.MustCompile(`[0-9]+$`).FindString(urlMatch)
		errors = append(
			errors,
			fmt.Sprintf("   Found: 'https://%s' → Should be: '%s'", urlMatch, prNum),
		)
	}

	return errors
}

// containsClaudeAIAttribution checks for AI attribution patterns while allowing legitimate tool references
func (*CommitValidator) containsClaudeAIAttribution(message string) bool {
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
	// Pattern: [Claude Code](...) or [Claude](...)
	if regexp.MustCompile(`\[claude[^\]]*\]\([^)]*claude[^)]*\)`).MatchString(lower) {
		return true
	}

	// If "claude" doesn't appear at all, it's fine
	if !strings.Contains(lower, "claude") {
		return false
	}

	// Allow legitimate tool/file references:
	// - CLAUDE.md file references
	// - claude-hooks, klaudiush (hyphenated tool names)
	// - `claude` in backticks (code references)
	// Note: We need word boundaries to avoid matching URLs like https://claude.com/claude-code
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

	// If we get here, only block explicit attribution patterns
	// Allow general usage like "claude integration" or "claude features"
	return false
}

// getTitleMaxLength returns the max title length from config, or default
func (v *CommitValidator) getTitleMaxLength() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.TitleMaxLength != nil {
		return *v.config.Message.TitleMaxLength
	}

	return defaultMaxTitleLength
}

// getBodyMaxLineLength returns the max body line length from config, or default
func (v *CommitValidator) getBodyMaxLineLength() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.BodyMaxLineLength != nil {
		return *v.config.Message.BodyMaxLineLength
	}

	return defaultMaxBodyLineLength
}

// getBodyLineTolerance returns the body line tolerance from config, or default
func (v *CommitValidator) getBodyLineTolerance() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.BodyLineTolerance != nil {
		return *v.config.Message.BodyLineTolerance
	}

	return defaultBodyLineTolerance
}

// shouldCheckConventionalCommits returns whether conventional commits validation is enabled
func (v *CommitValidator) shouldCheckConventionalCommits() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.ConventionalCommits != nil {
		return *v.config.Message.ConventionalCommits
	}

	return true // Default: enabled
}

// getValidTypes returns the valid commit types from config, or defaults
func (v *CommitValidator) getValidTypes() []string {
	if v.config != nil && v.config.Message != nil && len(v.config.Message.ValidTypes) > 0 {
		return v.config.Message.ValidTypes
	}

	return defaultValidTypes
}

// shouldRequireScope returns whether scope is required in conventional commits
func (v *CommitValidator) shouldRequireScope() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.RequireScope != nil {
		return *v.config.Message.RequireScope
	}

	return true // Default: require scope
}

// shouldBlockInfraScopeMisuse returns whether to block feat/fix with infrastructure scopes
func (v *CommitValidator) shouldBlockInfraScopeMisuse() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockInfraScopeMisuse != nil {
		return *v.config.Message.BlockInfraScopeMisuse
	}

	return true // Default: block infra scope misuse
}

// shouldBlockPRReferences returns whether to block PR references in commits
func (v *CommitValidator) shouldBlockPRReferences() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockPRReferences != nil {
		return *v.config.Message.BlockPRReferences
	}

	return true // Default: block PR references
}

// shouldBlockAIAttribution returns whether to block AI attribution in commits
func (v *CommitValidator) shouldBlockAIAttribution() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockAIAttribution != nil {
		return *v.config.Message.BlockAIAttribution
	}

	return true // Default: block AI attribution
}

// getExpectedSignoff returns the expected signoff from config
func (v *CommitValidator) getExpectedSignoff() string {
	if v.config != nil && v.config.Message != nil {
		return v.config.Message.ExpectedSignoff
	}

	return ""
}

// buildConventionalCommitPattern builds a regex pattern for conventional commits
func (*CommitValidator) buildConventionalCommitPattern(
	validTypes []string,
	requireScope bool,
) string {
	typesStr := strings.Join(validTypes, "|")

	if requireScope {
		// Scope is mandatory
		return fmt.Sprintf(`^(%s)\([a-zA-Z0-9_\/-]+\)!?: .+`, typesStr)
	}

	// Scope is optional
	return fmt.Sprintf(`^(%s)(\([a-zA-Z0-9_\/-]+\))?!?: .+`, typesStr)
}
