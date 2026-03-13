package git

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	defaultValidTypesPattern  = "build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test"
	nonUserFacingTypesPattern = "ci|test|chore|build|docs|style|refactor"
)

var userFacingInfraRegex = regexp.MustCompile(`^(feat|fix)\((ci|test|docs|build)\):`)

// PRTitleValidationResult contains the result of PR title validation
type PRTitleValidationResult struct {
	Valid        bool
	ErrorMessage string
	Details      []string
}

// validatePRTitle validates that a PR title follows semantic commit format (if enabled)
// and doesn't misuse feat/fix with infrastructure scopes
func validatePRTitle(
	title string,
	maxLength int,
	checkConventionalCommits bool,
	allowUnlimitedRevertTitle bool,
	validTypes []string,
) PRTitleValidationResult {
	if title == "" {
		return PRTitleValidationResult{
			Valid:        false,
			ErrorMessage: "PR title is empty",
		}
	}

	// Check title length (skip for revert titles if configured)
	isRevert := isRevertCommit(title)

	if len(title) > maxLength && (!allowUnlimitedRevertTitle || !isRevert) {
		return PRTitleValidationResult{
			Valid: false,
			ErrorMessage: fmt.Sprintf(
				"PR title exceeds maximum length of %d characters",
				maxLength,
			),
			Details: []string{
				fmt.Sprintf("Current length: %d", len(title)),
				fmt.Sprintf("Title: '%s'", title),
				"Note: Revert titles (Revert \"...\") are exempt from this limit",
			},
		}
	}

	// Check semantic commit format (if enabled)
	// Skip for revert titles since they use a different format
	if checkConventionalCommits && !isRevert {
		validTypesPattern := strings.Join(validTypes, "|")
		semanticCommitRegex := regexp.MustCompile(
			fmt.Sprintf(`^(%s)(\([a-zA-Z0-9_\/-]+\))?!?: .+`, validTypesPattern),
		)

		if !semanticCommitRegex.MatchString(title) {
			return PRTitleValidationResult{
				Valid:        false,
				ErrorMessage: "PR title doesn't follow semantic commit format",
				Details: []string{
					fmt.Sprintf("Current: '%s'", title),
					"Expected: type(scope): description",
					"Valid types: " + strings.Join(validTypes, ", "),
					"Alternative: Revert \"original PR title\"",
				},
			}
		}

		// Check for feat/fix misuse with infrastructure scopes
		if matches := userFacingInfraRegex.FindStringSubmatch(title); matches != nil {
			typeMatch := matches[1]  // feat or fix
			scopeMatch := matches[2] // ci, test, docs, or build

			return PRTitleValidationResult{
				Valid: false,
				ErrorMessage: fmt.Sprintf(
					"Use '%s(...)' not '%s(%s)' for infrastructure changes",
					scopeMatch,
					typeMatch,
					scopeMatch,
				),
				Details: []string{
					"feat/fix should only be used for user-facing changes",
				},
			}
		}
	}

	return PRTitleValidationResult{Valid: true}
}

// extractPRType extracts the type from a semantic commit title (e.g., "feat", "fix", "ci")
func extractPRType(title string, validTypes []string) string {
	if len(validTypes) == 0 {
		return ""
	}

	validTypesPattern := strings.Join(validTypes, "|")
	typeRegex := regexp.MustCompile(fmt.Sprintf(`^(%s)`, validTypesPattern))

	matches := typeRegex.FindStringSubmatch(title)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// ValidatePRTitle validates a PR title with default configuration (exported for testing)
//
//nolint:revive // Exported for testing, intentionally similar to internal function
func ValidatePRTitle(title string) PRTitleValidationResult {
	return validatePRTitle(
		title,
		config.DefaultTitleMaxLength,
		true,
		true,
		config.DefaultValidTypes,
	)
}

// ExtractPRType extracts the type with default valid types (exported for testing)
//
//nolint:revive // Exported for testing, intentionally similar to internal function
func ExtractPRType(title string) string {
	return extractPRType(title, config.DefaultValidTypes)
}

// IsNonUserFacingType returns true if the type is non-user-facing
// (ci, test, chore, build, docs, style, refactor)
func IsNonUserFacingType(prType string) bool {
	nonUserFacingTypes := strings.Split(nonUserFacingTypesPattern, "|")
	return slices.Contains(nonUserFacingTypes, prType)
}
