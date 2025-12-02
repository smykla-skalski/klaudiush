package git

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	motivationHeader       = "## Motivation"
	implementationHeader   = "## Implementation information"
	supportingDocsHeader   = "## Supporting documentation"
	changelogLineThreshold = 40
	shortLineThreshold     = 3
	totalLineThreshold     = 5
)

var (
	changelogSkipRegex   = regexp.MustCompile(`(?m)^>\s*Changelog:\s*skip`)
	changelogCustomRegex = regexp.MustCompile(`(?m)^>\s*Changelog:\s*(.+)`)
	formalWordsRegex     = regexp.MustCompile(`(?i)\b(utilize|leverage|facilitate|implement)\b`)
	htmlCommentRegex     = regexp.MustCompile(`<!--[\s\S]*?-->`)

	// textPlaceholders matches text-based placeholders at the start of a line,
	// even if followed by additional content (e.g., "N/A - explanation")
	textPlaceholders = regexp.MustCompile(
		`(?i)^\s*(n/?a|none|nothing|empty|tbd|todo)\b`,
	)

	// symbolPlaceholders matches symbol-based placeholders that must be standalone
	// (prevents matching Markdown list items like "- Link")
	symbolPlaceholders = regexp.MustCompile(
		`^\s*(-|—|–|\.{2,})\s*$`,
	)
)

// PRBodyValidationResult contains the result of PR body validation
type PRBodyValidationResult struct {
	Errors   []string
	Warnings []string
}

// validatePRBody validates PR body structure, changelog rules, and language
func validatePRBody(body, prType string, requireChangelog bool) PRBodyValidationResult {
	result := PRBodyValidationResult{
		Errors:   []string{},
		Warnings: []string{},
	}

	if body == "" {
		result.Warnings = append(
			result.Warnings,
			"Could not extract PR body - ensure you're using --body flag",
		)

		return result
	}

	// Check for required sections
	checkRequiredSections(body, &result)

	// Check changelog placement (must not be before ## Motivation)
	checkChangelogPlacement(body, &result)

	// Validate changelog handling
	validateChangelog(body, prType, requireChangelog, &result)

	// Check for simple, personal language
	if formalWordsRegex.MatchString(body) {
		result.Warnings = append(result.Warnings,
			"PR description uses formal language - consider simpler, more personal tone",
			"Examples: 'use' instead of 'utilize', 'add' instead of 'implement'",
		)
	}

	// Check for line breaks in paragraphs
	checkLineBreaks(body, &result)

	// Check if Supporting documentation section is empty
	checkSupportingDocs(body, &result)

	return result
}

// checkRequiredSections validates that all required sections are present
func checkRequiredSections(body string, result *PRBodyValidationResult) {
	if !strings.Contains(body, motivationHeader) {
		result.Errors = append(result.Errors, "PR body missing '## Motivation' section")
	}

	if !strings.Contains(body, implementationHeader) {
		result.Errors = append(
			result.Errors,
			"PR body missing '## Implementation information' section",
		)
	}

	if !strings.Contains(body, supportingDocsHeader) {
		result.Warnings = append(
			result.Warnings,
			"PR body missing '## Supporting documentation' section",
			"This section can be omitted only when it would result in N/A",
		)
	}
}

// checkChangelogPlacement validates that > Changelog: is not placed before ## Motivation
func checkChangelogPlacement(body string, result *PRBodyValidationResult) {
	motivationIdx := strings.Index(body, motivationHeader)
	changelogMatch := changelogCustomRegex.FindStringIndex(body)

	if changelogMatch == nil {
		return
	}

	// If motivation header doesn't exist, we already report that as an error
	if motivationIdx == -1 {
		return
	}

	// Changelog line should not appear before Motivation
	if changelogMatch[0] < motivationIdx {
		result.Errors = append(result.Errors,
			"'> Changelog:' line must not appear before '## Motivation' section",
			"Move the changelog line to the end of the PR body",
		)
	}
}

// validateChangelog validates changelog rules based on PR type
func validateChangelog(body, prType string, requireChangelog bool, result *PRBodyValidationResult) {
	hasChangelogSkip := changelogSkipRegex.MatchString(body)
	changelogMatches := changelogCustomRegex.FindStringSubmatch(body)
	hasCustomChangelog := len(changelogMatches) > 1 && changelogMatches[1] != "skip"
	hasAnyChangelog := hasChangelogSkip || hasCustomChangelog

	// If changelog is required and not present, error
	if requireChangelog && !hasAnyChangelog {
		result.Errors = append(result.Errors,
			"PR body missing required '> Changelog:' line",
			"Add either '> Changelog: skip' or '> Changelog: type(scope): description'",
		)

		return
	}

	if prType != "" {
		isNonUserFacing := IsNonUserFacingType(prType)

		// Non-user-facing changes should have changelog: skip
		if isNonUserFacing && !hasChangelogSkip && !hasCustomChangelog {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("PR type '%s' should typically have '> Changelog: skip'", prType),
				"Infrastructure changes don't need changelog entries",
			)
		}

		// User-facing changes should NOT skip changelog
		if !isNonUserFacing && hasChangelogSkip {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("PR type '%s' is user-facing but has 'Changelog: skip'", prType),
				"Consider removing 'skip' or using custom changelog entry",
			)
		}
	}

	// Validate custom changelog format if present
	if hasCustomChangelog {
		changelogEntry := changelogMatches[1]

		// Build a regex from valid types (use default if prType is empty)
		validTypesPattern := defaultValidTypesPattern
		semanticCommitRegex := regexp.MustCompile(
			fmt.Sprintf(`^(%s)(\([a-zA-Z0-9_\/-]+\))?!?: .+`, validTypesPattern),
		)

		if !semanticCommitRegex.MatchString(changelogEntry) {
			result.Errors = append(result.Errors,
				"Custom changelog entry doesn't follow semantic commit format",
				fmt.Sprintf("Found: '%s'", changelogEntry),
				"Note: Changelog format is flexible on length but should be semantic",
			)
		}
	}
}

// checkLineBreaks checks for unnecessary line breaks in paragraphs
func checkLineBreaks(body string, result *PRBodyValidationResult) {
	shortLines := 0
	totalLines := 0
	lines := strings.SplitSeq(body, "\n")

	for line := range lines {
		// Skip headers, blank lines, blockquotes, and list items
		if strings.HasPrefix(line, "##") ||
			strings.TrimSpace(line) == "" ||
			strings.HasPrefix(line, ">") ||
			strings.HasPrefix(line, "-") ||
			strings.HasPrefix(line, "*") {
			continue
		}

		totalLines++

		if len(line) < changelogLineThreshold {
			shortLines++
		}
	}

	if totalLines > totalLineThreshold && shortLines > shortLineThreshold {
		result.Warnings = append(result.Warnings,
			"PR description may have unnecessary line breaks within paragraphs",
			"Don't break long lines in body paragraphs - let them flow naturally",
		)
	}
}

// checkSupportingDocs checks if Supporting documentation section has N/A or empty placeholder values
func checkSupportingDocs(body string, result *PRBodyValidationResult) {
	idx := strings.Index(body, supportingDocsHeader)
	if idx == -1 {
		return
	}

	// Extract the section content
	sectionContent := extractSectionContent(body[idx+len(supportingDocsHeader):])

	// Strip HTML comments from the content
	sectionContent = htmlCommentRegex.ReplaceAllString(sectionContent, "")

	// Check each non-empty line for N/A or empty placeholder patterns
	for line := range strings.SplitSeq(sectionContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check both text placeholders (can have trailing content) and symbol placeholders (must be standalone)
		if textPlaceholders.MatchString(trimmed) || symbolPlaceholders.MatchString(trimmed) {
			result.Errors = append(
				result.Errors,
				"Supporting documentation section contains placeholder value: "+trimmed,
				"Remove the entire '## Supporting documentation' section if there's no supporting documentation",
			)

			return
		}
	}
}

// extractSectionContent extracts the content of a section until the next section boundary
// Boundaries are: next ## header, ---, or > Changelog: line
func extractSectionContent(content string) string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Stop at next header
		if strings.HasPrefix(trimmed, "##") {
			break
		}

		// Stop at horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			break
		}

		// Stop at changelog line
		if strings.HasPrefix(trimmed, "> Changelog:") {
			break
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// ValidatePRBody validates PR body with default configuration (exported for testing)
//
//nolint:revive // Exported for testing, intentionally similar to internal function
func ValidatePRBody(body, prType string) PRBodyValidationResult {
	return validatePRBody(body, prType, false)
}
