package git

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators"
)

const (
	// Default values for commit message validation
	defaultMaxTitleLength    = 50
	defaultMaxBodyLineLength = 72
	defaultBodyLineTolerance = 5  // Additional tolerance beyond max body line length
	truncateErrorLineAt      = 60 // Truncate long lines in error messages for readability

	// Commit style constants for commit_style config field.
	commitStyleConventional = "conventional"
	commitStyleScopeOnly    = "scope-only"
	commitStyleNone         = "none"
	commitStyleCustom       = "custom"
	commitStyleAuto         = "auto"
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

	// defaultForbiddenPatterns blocks mentions of tmp directory
	defaultForbiddenPatterns = []string{
		`\btmp/`,  // tmp/ path references
		`\btmp\b`, // standalone tmp word
	}

	// Git revert commit format: Revert "original commit" or Revert 'original commit'
	revertCommitRegex = regexp.MustCompile(`^Revert ["'].+["']$`)
)

// validateMessage validates the commit message content using the parser and rules.
func (v *CommitValidator) validateMessage(ctx context.Context, message string) *validator.Result {
	log := v.Logger()
	log.Debug("Validating commit message", "length", len(message))

	if message == "" {
		return validator.FailWithRef(validator.RefGitConventionalCommit, "Commit message is empty")
	}

	// Create parser with configured valid types
	parserOpts := []CommitParserOption{
		WithValidTypes(v.getValidTypes()),
	}
	parser := NewCommitParser(parserOpts...)

	// Parse the commit message
	parsed := parser.Parse(message)

	// Build and execute validation rules
	rules := v.buildRules(ctx)
	ruleResults := make([]*RuleResult, 0)

	for _, rule := range rules {
		result := rule.Validate(parsed, message)
		if result != nil && result.Message != "" {
			ruleResults = append(ruleResults, result)
		}
	}

	// Validate markdown formatting in body
	if len(strings.Split(message, "\n")) > 1 {
		markdownErrors := v.validateMarkdownInBody(strings.Split(message, "\n"))
		if len(markdownErrors) > 0 {
			// Markdown errors are body-related warnings
			ruleResults = append(ruleResults, &RuleResult{
				Reference: validator.RefGitBadBody,
				Message:   markdownErrors[0],
				Context:   markdownErrors[1:],
			})
		}
	}

	// Report errors if any
	if len(ruleResults) > 0 {
		return v.buildErrorResult(ruleResults, message)
	}

	log.Debug("Commit message validation passed")

	return validator.Pass()
}

// buildRules creates the validation rules based on configuration.
func (v *CommitValidator) buildRules(ctx context.Context) []CommitRule {
	rules := make([]CommitRule, 0)

	// Title length rule
	rules = append(rules, &TitleLengthRule{
		MaxLength:                 v.getTitleMaxLength(),
		AllowUnlimitedRevertTitle: v.shouldAllowUnlimitedRevertTitle(),
	})

	// Title format rule - determined by commit_style config
	switch v.getCommitStyle() {
	case commitStyleConventional:
		rules = append(rules, &ConventionalFormatRule{
			ValidTypes:   v.getValidTypes(),
			RequireScope: v.shouldRequireScope(),
		})
	case commitStyleScopeOnly:
		rules = append(rules, &ScopeOnlyFormatRule{})
	case commitStyleCustom:
		if pattern := v.getTitlePattern(); pattern != nil {
			rules = append(rules, &CustomPatternRule{Pattern: pattern})
		}
	case commitStyleNone:
		// no title format rule
	case commitStyleAuto:
		detected := NewCommitStyleDetector().Detect(ctx)
		if detected == commitStyleScopeOnly {
			rules = append(rules, &ScopeOnlyFormatRule{})
		} else {
			rules = append(rules, &ConventionalFormatRule{
				ValidTypes:   v.getValidTypes(),
				RequireScope: v.shouldRequireScope(),
			})
		}
	}

	// Infrastructure scope misuse rule
	if v.shouldBlockInfraScopeMisuse() {
		rules = append(rules, NewInfraScopeMisuseRule())
	}

	// Body line length rule
	rules = append(rules, NewBodyLineLengthRule(
		v.getBodyMaxLineLength(),
		v.getBodyLineTolerance(),
	))

	// List formatting rule
	rules = append(rules, NewListFormattingRule())

	// PR reference rule
	if v.shouldBlockPRReferences() {
		rules = append(rules, NewPRReferenceRule())
	}

	// AI attribution rule
	if v.shouldBlockAIAttribution() {
		rules = append(rules, NewAIAttributionRule())
	}

	// Forbidden patterns rule
	rules = append(rules, &ForbiddenPatternRule{
		Patterns: v.getForbiddenPatterns(),
	})

	// Signoff rule
	if expectedSignoff := v.getExpectedSignoff(); expectedSignoff != "" {
		rules = append(rules, &SignoffRule{
			ExpectedSignoff: expectedSignoff,
		})
	}

	return rules
}

// validateMarkdownInBody validates markdown formatting in the commit body.
func (*CommitValidator) validateMarkdownInBody(lines []string) []string {
	// Extract body (skip title and empty line after title)
	bodyStartIdx := 1
	if bodyStartIdx < len(lines) && strings.TrimSpace(lines[bodyStartIdx]) == "" {
		bodyStartIdx++
	}

	if bodyStartIdx >= len(lines) {
		return nil
	}

	bodyLines := lines[bodyStartIdx:]

	// Strip git trailer lines before markdown analysis to prevent
	// false positives (e.g., Signed-off-by: triggering list detection)
	footerStart := findFooterStartIndex(bodyLines)
	if footerStart < len(bodyLines) {
		bodyLines = bodyLines[:footerStart]
	}

	if len(bodyLines) == 0 {
		return nil
	}

	body := strings.Join(bodyLines, "\n")
	markdownResult := validators.AnalyzeMarkdown(body, nil)

	return markdownResult.Warnings
}

// buildErrorResult constructs the error result with details.
// It selects the most appropriate reference based on what rules failed.
// Results are sorted by fix priority so Claude sees the most actionable errors first.
func (*CommitValidator) buildErrorResult(results []*RuleResult, message string) *validator.Result {
	// Sort results by fix priority
	sortResultsByFixOrder(results)

	// Collect all errors and determine the primary reference
	ref := selectPrimaryReference(results)

	primaryMsg := results[0].Message

	var details strings.Builder

	if len(results) > 1 {
		// Multiple errors: list all with context
		fmt.Fprintf(&details, "%d issues:\n\n", len(results))

		for _, r := range results {
			details.WriteString("- ")
			details.WriteString(r.Message)
			details.WriteString("\n")

			for _, ctx := range r.Context {
				details.WriteString("  ")
				details.WriteString(ctx)
				details.WriteString("\n")
			}
		}
	} else {
		// Single error: context only (message is already in Message field)
		for _, ctx := range results[0].Context {
			details.WriteString(ctx)
			details.WriteString("\n")
		}
	}

	result := validator.FailWithRef(ref, primaryMsg)

	if details.Len() > 0 {
		result = result.AddDetail("errors", details.String())
	}

	// Commit preview in separate key - formatter skips by default
	result = result.AddDetail("commit_preview", message)

	return result
}

// referenceFixOrder defines the sort priority for error results.
// References earlier in the slice have higher fix priority.
// This reuses the same order as selectPrimaryReference.
var referenceFixOrder = []validator.Reference{
	validator.RefGitConventionalCommit, // GIT013
	validator.RefGitFeatCI,             // GIT006
	validator.RefGitBadTitle,           // GIT004
	validator.RefGitBadBody,            // GIT005
	validator.RefGitListFormat,         // GIT016
	validator.RefGitPRRef,              // GIT011
	validator.RefGitClaudeAttr,         // GIT012
	validator.RefGitForbiddenPattern,   // GIT014
	validator.RefGitSignoffMismatch,    // GIT015
}

// sortResultsByFixOrder sorts rule results by fix priority.
// Errors that should be fixed first appear earlier in the list.
func sortResultsByFixOrder(results []*RuleResult) {
	orderIndex := make(map[validator.Reference]int, len(referenceFixOrder))
	for i, ref := range referenceFixOrder {
		orderIndex[ref] = i
	}

	fallback := len(referenceFixOrder)

	slices.SortStableFunc(results, func(a, b *RuleResult) int {
		orderA, okA := orderIndex[a.Reference]
		if !okA {
			orderA = fallback
		}

		orderB, okB := orderIndex[b.Reference]
		if !okB {
			orderB = fallback
		}

		return orderA - orderB
	})
}

// selectPrimaryReference selects the most appropriate reference from rule results.
// Priority order (highest to lowest):
// 1. Conventional commit format errors (GIT013)
// 2. Infrastructure scope misuse (GIT006)
// 3. Title length errors (GIT004)
// 4. Body errors (GIT005)
// 5. List formatting (GIT016)
// 6. PR references (GIT011)
// 7. AI attribution (GIT012)
// 8. Forbidden patterns (GIT014)
// 9. Signoff mismatch (GIT015)
func selectPrimaryReference(results []*RuleResult) validator.Reference {
	if len(results) == 0 {
		return validator.RefGitConventionalCommit // fallback
	}

	// If only one result, use its reference
	if len(results) == 1 {
		return results[0].Reference
	}

	// Priority-based selection for multiple errors
	// Use a map to track which references are present
	refs := make(map[validator.Reference]bool)
	for _, result := range results {
		refs[result.Reference] = true
	}

	// Check in priority order
	priorityOrder := []validator.Reference{
		validator.RefGitConventionalCommit, // Format issues are fundamental
		validator.RefGitFeatCI,             // Semantic type misuse
		validator.RefGitBadTitle,           // Title issues
		validator.RefGitBadBody,            // Body issues
		validator.RefGitListFormat,         // List formatting
		validator.RefGitPRRef,              // Content issues
		validator.RefGitClaudeAttr,         // Content issues
		validator.RefGitForbiddenPattern,   // Content issues
		validator.RefGitSignoffMismatch,    // Signoff issues
	}

	for _, ref := range priorityOrder {
		if refs[ref] {
			return ref
		}
	}

	// Fallback to first result's reference
	return results[0].Reference
}

// truncateLine truncates a line for display in error messages.
func truncateLine(line string) string {
	if len(line) > truncateErrorLineAt {
		return line[:truncateErrorLineAt] + "..."
	}

	return line
}

// getTitleMaxLength returns the max title length from config, or default.
func (v *CommitValidator) getTitleMaxLength() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.TitleMaxLength != nil {
		return *v.config.Message.TitleMaxLength
	}

	return defaultMaxTitleLength
}

// shouldAllowUnlimitedRevertTitle returns whether revert commits are exempt from title length limits.
func (v *CommitValidator) shouldAllowUnlimitedRevertTitle() bool {
	if v.config != nil && v.config.Message != nil &&
		v.config.Message.AllowUnlimitedRevertTitle != nil {
		return *v.config.Message.AllowUnlimitedRevertTitle
	}

	return true // Default: allow unlimited revert title length
}

// getBodyMaxLineLength returns the max body line length from config, or default.
func (v *CommitValidator) getBodyMaxLineLength() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.BodyMaxLineLength != nil {
		return *v.config.Message.BodyMaxLineLength
	}

	return defaultMaxBodyLineLength
}

// getBodyLineTolerance returns the body line tolerance from config, or default.
func (v *CommitValidator) getBodyLineTolerance() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.BodyLineTolerance != nil {
		return *v.config.Message.BodyLineTolerance
	}

	return defaultBodyLineTolerance
}

// shouldCheckConventionalCommits returns whether conventional commits validation is enabled.
func (v *CommitValidator) shouldCheckConventionalCommits() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.ConventionalCommits != nil {
		return *v.config.Message.ConventionalCommits
	}

	return true // Default: enabled
}

// getCommitStyle returns the effective commit style.
// When CommitStyle is empty or unset, falls back to the legacy ConventionalCommits flag.
func (v *CommitValidator) getCommitStyle() string {
	if v.config != nil && v.config.Message != nil && v.config.Message.CommitStyle != "" {
		return v.config.Message.CommitStyle
	}

	// Legacy: ConventionalCommits=false → "none"
	if !v.shouldCheckConventionalCommits() {
		return commitStyleNone
	}

	return commitStyleConventional // default
}

// getTitlePattern compiles and returns the custom title regex, or nil if not set.
func (v *CommitValidator) getTitlePattern() *regexp.Regexp {
	if v.config == nil || v.config.Message == nil || v.config.Message.TitlePattern == "" {
		return nil
	}

	pattern, err := regexp.Compile(v.config.Message.TitlePattern)
	if err != nil {
		return nil // already validated by config validation
	}

	return pattern
}

// getValidTypes returns the valid commit types from config, or defaults.
func (v *CommitValidator) getValidTypes() []string {
	if v.config != nil && v.config.Message != nil && len(v.config.Message.ValidTypes) > 0 {
		return v.config.Message.ValidTypes
	}

	return defaultValidTypes
}

// shouldRequireScope returns whether scope is required in conventional commits.
func (v *CommitValidator) shouldRequireScope() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.RequireScope != nil {
		return *v.config.Message.RequireScope
	}

	return true // Default: require scope
}

// shouldBlockInfraScopeMisuse returns whether to block feat/fix with infrastructure scopes.
func (v *CommitValidator) shouldBlockInfraScopeMisuse() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockInfraScopeMisuse != nil {
		return *v.config.Message.BlockInfraScopeMisuse
	}

	return true // Default: block infra scope misuse
}

// shouldBlockPRReferences returns whether to block PR references in commits.
func (v *CommitValidator) shouldBlockPRReferences() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockPRReferences != nil {
		return *v.config.Message.BlockPRReferences
	}

	return true // Default: block PR references
}

// shouldBlockAIAttribution returns whether to block AI attribution in commits.
func (v *CommitValidator) shouldBlockAIAttribution() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockAIAttribution != nil {
		return *v.config.Message.BlockAIAttribution
	}

	return true // Default: block AI attribution
}

// getExpectedSignoff returns the expected signoff from config.
func (v *CommitValidator) getExpectedSignoff() string {
	if v.config != nil && v.config.Message != nil {
		return v.config.Message.ExpectedSignoff
	}

	return ""
}

// getForbiddenPatterns returns the list of forbidden patterns from config, or defaults.
func (v *CommitValidator) getForbiddenPatterns() []string {
	if v.config != nil && v.config.Message != nil && len(v.config.Message.ForbiddenPatterns) > 0 {
		return v.config.Message.ForbiddenPatterns
	}

	return defaultForbiddenPatterns
}

// isRevertCommit checks if the title follows git's revert commit format.
// Git generates revert commits with the format: Revert "original commit title"
func isRevertCommit(title string) bool {
	return revertCommitRegex.MatchString(title)
}
