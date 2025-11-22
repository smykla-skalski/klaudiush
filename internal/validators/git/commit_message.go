package git

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/smykla-labs/claude-hooks/internal/validator"
)

const (
	maxTitleLength       = 50
	maxBodyLineLength    = 72
	maxBodyLineTolerance = 77 // 72 + 5 tolerance
)

// ExpectedSignoff can be set at build time using:
// go build -ldflags="-X 'github.com/smykla-labs/claude-hooks/internal/validators/git.ExpectedSignoff=Your Name <your@email.com>'"
var ExpectedSignoff = "" // Default empty, must be set at build time

var (
	// validTypes from commitlint config-conventional
	validTypes = []string{"build", "chore", "ci", "docs", "feat", "fix", "perf", "refactor", "revert", "style", "test"}

	// Conventional commit format: type(scope): description
	conventionalCommitRegex = regexp.MustCompile(`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-zA-Z0-9_\/-]+\))?!?: .+`)

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

	// Check for Claude AI references
	if strings.Contains(strings.ToLower(message), "claude") {
		errors = append(errors, "❌ Commit message contains Claude AI reference - remove any AI generation attribution")
	}

	// Split message into lines
	lines := strings.Split(message, "\n")

	// Get title (first non-empty line)
	title := ""
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			title = line
			break
		}
	}

	if title == "" {
		return validator.Fail("Commit message is empty")
	}

	// Check title length
	if len(title) > maxTitleLength {
		errors = append(errors, fmt.Sprintf("❌ Title exceeds %d characters (%d chars): '%s'", maxTitleLength, len(title), title))
	}

	// Check conventional commit format
	if !conventionalCommitRegex.MatchString(title) {
		errors = append(errors, "❌ Title doesn't follow conventional commits format: type(scope): description")
		errors = append(errors, fmt.Sprintf("   Valid types: %s", strings.Join(validTypes, ", ")))
		errors = append(errors, fmt.Sprintf("   Current title: '%s'", title))
	}

	// Check for feat/fix misuse with infrastructure scopes
	if infraScopeMisuseRegex.MatchString(title) {
		matches := infraScopeMisuseRegex.FindStringSubmatch(title)
		if len(matches) >= 3 {
			typeMatch := matches[1]   // feat or fix
			scopeMatch := matches[2]  // ci, test, docs, or build
			errors = append(errors, fmt.Sprintf("❌ Use '%s(...)' not '%s(%s)' for infrastructure changes", scopeMatch, typeMatch, scopeMatch))
			errors = append(errors, "   feat/fix should only be used for user-facing changes")
		}
	}

	// Validate body lines
	bodyErrors := v.validateBodyLines(lines)
	errors = append(errors, bodyErrors...)

	// Check for PR references
	if prReferenceRegex.MatchString(message) {
		errors = append(errors, "❌ PR references found - remove '#' prefix or convert URLs to plain numbers")

		// Show examples
		if hashMatch := regexp.MustCompile(`#[0-9]+`).FindString(message); hashMatch != "" {
			fix := strings.TrimPrefix(hashMatch, "#")
			errors = append(errors, fmt.Sprintf("   Found: '%s' → Should be: '%s'", hashMatch, fix))
		}

		if urlMatch := regexp.MustCompile(`github\.com/.+/pull/[0-9]+`).FindString(message); urlMatch != "" {
			prNum := regexp.MustCompile(`[0-9]+$`).FindString(urlMatch)
			errors = append(errors, fmt.Sprintf("   Found: 'https://%s' → Should be: '%s'", urlMatch, prNum))
		}
	}

	// Check Signed-off-by trailer if present (only if ExpectedSignoff is set)
	if ExpectedSignoff != "" && strings.Contains(message, "Signed-off-by:") {
		signoffLine := ""
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "Signed-off-by:") {
				signoffLine = strings.TrimSpace(line)
				break
			}
		}

		expectedSignoffLine := fmt.Sprintf("Signed-off-by: %s", ExpectedSignoff)
		if signoffLine != expectedSignoffLine {
			errors = append(errors, "❌ Wrong signoff identity")
			errors = append(errors, fmt.Sprintf("   Found: %s", signoffLine))
			errors = append(errors, fmt.Sprintf("   Expected: %s", expectedSignoffLine))
		}
	}

	// Report errors
	if len(errors) > 0 {
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

	log.Debug("Commit message validation passed")
	return validator.Pass()
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
				truncated := line
				if len(line) > 60 {
					truncated = line[:60] + "..."
				}
				errors = append(errors, fmt.Sprintf("❌ Missing empty line before first list item at line %d", lineNum+1))
				errors = append(errors, "   List items must be preceded by an empty line")
				errors = append(errors, fmt.Sprintf("   Line: '%s'", truncated))
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
			truncated := line
			if len(line) > 60 {
				truncated = line[:60] + "..."
			}
			errors = append(errors, fmt.Sprintf("❌ Line %d exceeds %d characters (%d chars, >5 over limit)", lineNum+1, maxBodyLineLength, lineLen))
			errors = append(errors, fmt.Sprintf("   Line: '%s'", truncated))
		}

		prevLineEmpty = false
	}

	return errors
}
