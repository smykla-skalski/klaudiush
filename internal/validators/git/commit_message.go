package git

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/internal/validators"
)

const (
	maxTitleLength       = 50
	maxBodyLineLength    = 72
	maxBodyLineTolerance = 77 // 72 + 5 tolerance
	truncateErrorLineAt  = 60 // Truncate long lines in error messages for readability
)

// ExpectedSignoff can be set at build time using:
// go build -ldflags="-X 'github.com/smykla-labs/claude-hooks/internal/validators/git.ExpectedSignoff=Your Name <your@email.com>'"
var ExpectedSignoff = "" // Default empty, must be set at build time

var (
	// validTypes from commitlint config-conventional
	validTypes = []string{
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

	// Conventional commit format: type(scope): description
	conventionalCommitRegex = regexp.MustCompile(
		`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-zA-Z0-9_\/-]+\))?!?: .+`,
	)

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

	// Check for Claude AI attribution (allow CLAUDE.md file references)
	if v.containsClaudeAIAttribution(message) {
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
	if len(title) > maxTitleLength {
		errors = append(
			errors,
			fmt.Sprintf(
				"❌ Title exceeds %d characters (%d chars): '%s'",
				maxTitleLength,
				len(title),
				title,
			),
		)
	}

	// Check conventional commit format
	if !conventionalCommitRegex.MatchString(title) {
		errors = append(
			errors,
			"❌ Title doesn't follow conventional commits format: type(scope): description",
		)
		errors = append(errors, "   Valid types: "+strings.Join(validTypes, ", "))
		errors = append(errors, fmt.Sprintf("   Current title: '%s'", title))
	}

	// Check for feat/fix misuse with infrastructure scopes
	infraErrors := v.checkInfraScopeMisuse(title)
	errors = append(errors, infraErrors...)

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

	// Check for PR references
	prErrors := v.checkPRReferences(message)
	errors = append(errors, prErrors...)

	// Check Signed-off-by trailer
	if ExpectedSignoff != "" && strings.Contains(message, "Signed-off-by:") {
		signoffErrors := v.validateSignoff(lines)
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
func (*CommitValidator) validateSignoff(lines []string) []string {
	errors := make([]string, 0)
	signoffLine := ""

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Signed-off-by:") {
			signoffLine = strings.TrimSpace(line)
			break
		}
	}

	expectedSignoffLine := "Signed-off-by: " + ExpectedSignoff
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

		// Allow up to 77 chars (72 + 5 tolerance)
		if lineLen > maxBodyLineTolerance {
			errors = append(errors, v.formatLineLengthError(line, lineNum, lineLen)...)
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
func (*CommitValidator) formatLineLengthError(line string, lineNum, lineLen int) []string {
	truncated := truncateLine(line)

	return []string{
		fmt.Sprintf(
			"❌ Line %d exceeds %d characters (%d chars, >5 over limit)",
			lineNum+1,
			maxBodyLineLength,
			lineLen,
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
		"assisted by claude",
		"created by claude",
		"written by claude",
		"with help from claude",
		"powered by claude",
		"claude ai",
	}

	for _, pattern := range aiPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// If "claude" doesn't appear at all, it's fine
	if !strings.Contains(lower, "claude") {
		return false
	}

	// Allow legitimate tool/file references:
	// - CLAUDE.md file references
	// - claude-hooks, claude-code (hyphenated tool names)
	// - `claude` in backticks (code references)
	legitimatePatterns := []string{
		"claude.md",
		"claude-hooks",
		"claude-code",
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
