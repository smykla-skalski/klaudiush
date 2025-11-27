package git

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

const (
	ghCommand         = "gh"
	prSubcommand      = "pr"
	createOperation   = "create"
	minGHPRCreateArgs = 2
)

var (
	// Regex patterns for extracting PR metadata from gh command
	prTitleRegex       = regexp.MustCompile(`--title\s+"([^"]+)"`)
	prTitleSingleRegex = regexp.MustCompile(`--title\s+'([^']+)'`)
	baseRegex          = regexp.MustCompile(`--base\s+"([^"]+)"`)
	baseSingleRegex    = regexp.MustCompile(`--base\s+'([^']+)'`)
	labelRegex         = regexp.MustCompile(`--label\s+"([^"]+)"`)
	labelSingleRegex   = regexp.MustCompile(`--label\s+'([^']+)'`)
	heredocRegex       = regexp.MustCompile(`<<'?EOF'?\s*\n((?s:.+?))\nEOF`)
	bodyRegex          = regexp.MustCompile(`--body\s+"([^"]+)"`)
	bodySingleRegex    = regexp.MustCompile(`--body\s+'([^']+)'`)

	// defaultPRForbiddenPatterns blocks mentions of tmp directory
	defaultPRForbiddenPatterns = []string{
		`\btmp/`,  // tmp/ path references
		`\btmp\b`, // standalone tmp word
	}
)

const (
	defaultPRTitleMaxLength = 50
)

// PRValidator validates gh pr create commands
type PRValidator struct {
	validator.BaseValidator
	config *config.PRValidatorConfig
}

// NewPRValidator creates a new PRValidator instance
func NewPRValidator(cfg *config.PRValidatorConfig, log logger.Logger) *PRValidator {
	return &PRValidator{
		BaseValidator: *validator.NewBaseValidator("validate-pr", log),
		config:        cfg,
	}
}

// getTitleMaxLength returns the maximum allowed length for PR titles
func (v *PRValidator) getTitleMaxLength() int {
	if v.config != nil && v.config.TitleMaxLength != nil {
		return *v.config.TitleMaxLength
	}

	return defaultPRTitleMaxLength
}

// isTitleConventionalCommitsEnabled returns whether conventional commit format is required for titles
func (v *PRValidator) isTitleConventionalCommitsEnabled() bool {
	if v.config != nil && v.config.TitleConventionalCommits != nil {
		return *v.config.TitleConventionalCommits
	}

	return true // default: enabled
}

// shouldAllowUnlimitedRevertTitle returns whether revert PRs are exempt from title length limits
func (v *PRValidator) shouldAllowUnlimitedRevertTitle() bool {
	if v.config != nil && v.config.AllowUnlimitedRevertTitle != nil {
		return *v.config.AllowUnlimitedRevertTitle
	}

	return true // default: allow unlimited revert title length
}

// getValidTypes returns the list of valid commit types for PR titles
func (v *PRValidator) getValidTypes() []string {
	if v.config != nil && len(v.config.ValidTypes) > 0 {
		return v.config.ValidTypes
	}

	// Default: same as commit message valid types
	return defaultValidTypes
}

// isRequireChangelog returns whether a changelog line is required in PR body
func (v *PRValidator) isRequireChangelog() bool {
	if v.config != nil && v.config.RequireChangelog != nil {
		return *v.config.RequireChangelog
	}

	return false // default: not required (PR title used if omitted)
}

// isCheckCILabelsEnabled returns whether CI label suggestions are enabled
func (v *PRValidator) isCheckCILabelsEnabled() bool {
	if v.config != nil && v.config.CheckCILabels != nil {
		return *v.config.CheckCILabels
	}

	return true // default: enabled
}

// isRequireBody returns whether PR body is required
func (v *PRValidator) isRequireBody() bool {
	if v.config != nil && v.config.RequireBody != nil {
		return *v.config.RequireBody
	}

	return true // default: required
}

// getMarkdownDisabledRules returns the list of markdownlint rules to disable for PR body validation
func (v *PRValidator) getMarkdownDisabledRules() []string {
	if v.config != nil && len(v.config.MarkdownDisabledRules) > 0 {
		return v.config.MarkdownDisabledRules
	}

	// Default: disable line length, bare URLs, and first line heading requirement
	return []string{"MD013", "MD034", "MD041"}
}

// Validate checks gh pr create command for proper PR structure
func (v *PRValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running PR validation")

	// Parse the command
	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(hookCtx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Find gh pr create commands
	for _, cmd := range result.Commands {
		if !v.isGHPRCreate(&cmd) {
			continue
		}

		// Extract PR metadata from the full command
		fullCmd := hookCtx.GetCommand()
		prData := v.extractPRData(fullCmd)

		// Validate PR
		return v.validatePR(ctx, prData)
	}

	log.Debug("No gh pr create commands found")

	return validator.Pass()
}

// isGHPRCreate checks if a command is gh pr create
func (*PRValidator) isGHPRCreate(cmd *parser.Command) bool {
	if cmd.Name != ghCommand {
		return false
	}

	if len(cmd.Args) < minGHPRCreateArgs {
		return false
	}

	return cmd.Args[0] == prSubcommand && cmd.Args[1] == createOperation
}

// PRData holds extracted PR metadata
type PRData struct {
	Title      string
	Body       string
	BaseBranch string
	Labels     []string
	HasLabels  bool
}

// extractPRData extracts PR title, body, base branch, and labels from gh command
func (v *PRValidator) extractPRData(command string) PRData {
	data := PRData{
		Labels: []string{},
	}

	// Extract title (try double quotes first, then single quotes)
	if matches := prTitleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Title = matches[1]
	} else if matches := prTitleSingleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Title = matches[1]
	}

	// Extract base branch (try double quotes first, then single quotes)
	if matches := baseRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.BaseBranch = matches[1]
	} else if matches := baseSingleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.BaseBranch = matches[1]
	}

	// Extract labels (try double quotes first, then single quotes)
	if matches := labelRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.HasLabels = true
		data.Labels = v.parseLabels(matches[1])
	} else if matches := labelSingleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.HasLabels = true
		data.Labels = v.parseLabels(matches[1])
	}

	// Extract body - try heredoc first, then quoted strings
	if matches := heredocRegex.FindStringSubmatch(command); len(matches) > 1 {
		// Add trailing newline for markdownlint MD047 rule
		data.Body = matches[1] + "\n"
	} else if matches := bodyRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Body = matches[1] + "\n"
	} else if matches := bodySingleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Body = matches[1] + "\n"
	}

	return data
}

// parseLabels splits a comma-separated label string
func (*PRValidator) parseLabels(labelStr string) []string {
	if labelStr == "" {
		return []string{}
	}

	labels := strings.Split(labelStr, ",")
	result := make([]string, 0, len(labels))

	for _, label := range labels {
		trimmed := strings.TrimSpace(label)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// validatePR performs comprehensive PR validation
func (v *PRValidator) validatePR(ctx context.Context, data PRData) *validator.Result {
	var allErrors []string

	var allWarnings []string

	// 1. Validate PR title
	v.validatePRTitleData(data.Title, &allErrors, &allWarnings)

	// 2. Check for forbidden patterns in title and body
	forbiddenErrors := v.checkForbiddenPatterns(data.Title, data.Body)
	allErrors = append(allErrors, forbiddenErrors...)

	// 3. Extract PR type for body validation
	validTypes := v.getValidTypes()
	prType := extractPRType(data.Title, validTypes)

	// 4. Validate PR body
	v.validatePRBodyData(data.Body, prType, &allErrors, &allWarnings)

	// 4. Validate markdown formatting
	if data.Body != "" {
		// External markdownlint validation
		disabledRules := v.getMarkdownDisabledRules()
		mdResult := ValidatePRMarkdown(ctx, data.Body, disabledRules)
		allErrors = append(allErrors, mdResult.Errors...)

		// Internal markdown validation (code block indentation, empty lines, etc.)
		internalMdResult := validators.AnalyzeMarkdown(data.Body, nil)
		allErrors = append(allErrors, internalMdResult.Warnings...)
	}

	// 5. Validate base branch labels
	validateBaseBranchLabels(data, &allErrors)

	// 6. Validate CI label heuristics (if enabled)
	if v.isCheckCILabelsEnabled() && data.Title != "" && data.Body != "" {
		ciWarnings := v.checkCILabelHeuristics(data, prType)
		allWarnings = append(allWarnings, ciWarnings...)
	}

	return v.buildResult(allErrors, allWarnings, data.Title)
}

// validatePRTitleData validates the PR title
func (v *PRValidator) validatePRTitleData(title string, allErrors, allWarnings *[]string) {
	if title == "" {
		*allWarnings = append(
			*allWarnings,
			"Could not extract PR title - ensure you're using --title flag",
		)

		return
	}

	validTypes := v.getValidTypes()
	titleMaxLength := v.getTitleMaxLength()
	checkConventionalCommits := v.isTitleConventionalCommitsEnabled()
	allowUnlimitedRevertTitle := v.shouldAllowUnlimitedRevertTitle()

	titleResult := validatePRTitle(
		title,
		titleMaxLength,
		checkConventionalCommits,
		allowUnlimitedRevertTitle,
		validTypes,
	)

	if !titleResult.Valid {
		*allErrors = append(*allErrors, titleResult.ErrorMessage)
		*allErrors = append(*allErrors, titleResult.Details...)
	}
}

// validatePRBodyData validates the PR body
func (v *PRValidator) validatePRBodyData(body, prType string, allErrors, allWarnings *[]string) {
	requireBody := v.isRequireBody()

	if body == "" {
		if requireBody {
			*allErrors = append(
				*allErrors,
				"PR body is required - ensure you're using --body flag",
			)
		} else {
			*allWarnings = append(
				*allWarnings,
				"Could not extract PR body - ensure you're using --body flag",
			)
		}

		return
	}

	requireChangelog := v.isRequireChangelog()
	bodyResult := validatePRBody(body, prType, requireChangelog)
	*allErrors = append(*allErrors, bodyResult.Errors...)
	*allWarnings = append(*allWarnings, bodyResult.Warnings...)
}

// validateBaseBranchLabels validates base branch labels
func validateBaseBranchLabels(data PRData, allErrors *[]string) {
	if data.BaseBranch == "" || data.BaseBranch == "master" || data.BaseBranch == "main" {
		return
	}

	// Release branch - should have matching label
	hasMatchingLabel := slices.Contains(data.Labels, data.BaseBranch)

	if !hasMatchingLabel {
		*allErrors = append(*allErrors,
			fmt.Sprintf("PR targets '%s' but missing label with base branch name", data.BaseBranch),
			fmt.Sprintf("Add: --label \"%s\"", data.BaseBranch),
			"Note: ci/* labels MUST be added during PR creation (not after)",
		)
	}
}

// buildResult builds the final validation result
func (*PRValidator) buildResult(allErrors, allWarnings []string, title string) *validator.Result {
	if len(allErrors) > 0 {
		message := "PR validation failed\n\n" + strings.Join(allErrors, "\n")
		if len(allWarnings) > 0 {
			message += "\n\nWarnings:\n" + strings.Join(allWarnings, "\n")
		}

		message += "\n\nPR title: " + title

		return validator.FailWithRef(validator.RefGitPRValidation, message)
	}

	if len(allWarnings) > 0 {
		message := "PR validation passed with warnings:\n\n" + strings.Join(allWarnings, "\n")
		return validator.WarnWithRef(validator.RefGitPRValidation, message)
	}

	return validator.Pass()
}

// checkCILabelHeuristics suggests ci/ labels based on PR type and content
func (*PRValidator) checkCILabelHeuristics(data PRData, prType string) []string {
	warnings := []string{}

	shouldSkipTests := false
	shouldSkipE2E := false

	// Check PR type for non-logic changes
	if prType == "ci" || prType == "docs" || prType == "chore" || prType == "style" {
		shouldSkipTests = true
		shouldSkipE2E = true
	}

	// Check for specific keywords in body
	bodyLower := strings.ToLower(data.Body)
	if strings.Contains(bodyLower, "only documentation") ||
		strings.Contains(bodyLower, "just comments") ||
		strings.Contains(bodyLower, "only ci") ||
		strings.Contains(bodyLower, "workflow changes") {
		shouldSkipTests = true
		shouldSkipE2E = true
	}

	if strings.Contains(bodyLower, "only unit tests") ||
		strings.Contains(bodyLower, "unit test changes") {
		shouldSkipE2E = true
	}

	// Check if ci/ labels are already present
	hasCILabel := false

	for _, label := range data.Labels {
		if strings.HasPrefix(label, "ci/skip") {
			hasCILabel = true
			break
		}
	}

	// Suggest labels if appropriate
	if shouldSkipTests && !data.HasLabels {
		warnings = append(warnings,
			"This appears to be a non-logic change - consider adding --label \"ci/skip-test\"",
			"Important: ci/* labels MUST be added during creation (--label flag)",
		)
	} else if shouldSkipE2E && !hasCILabel {
		warnings = append(warnings,
			"This appears to be a unit-test-only change - consider adding --label \"ci/skip-e2e-test\"",
			"Important: ci/* labels MUST be added during creation (--label flag)",
		)
	}

	return warnings
}

// checkForbiddenPatterns checks for forbidden patterns in PR title and body
func (v *PRValidator) checkForbiddenPatterns(title, body string) []string {
	patterns := v.getForbiddenPatterns()
	if len(patterns) == 0 {
		return nil
	}

	errors := make([]string, 0)

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			v.Logger().Debug("Invalid forbidden pattern", "pattern", pattern, "error", err)
			continue
		}

		// Check title
		if title != "" && re.MatchString(title) {
			match := re.FindString(title)
			errors = append(errors,
				fmt.Sprintf("❌ Forbidden pattern found in PR title: '%s'", match),
				"   Pattern: "+pattern,
				"   Remove this content from your PR title",
			)
		}

		// Check body
		if body != "" && re.MatchString(body) {
			match := re.FindString(body)
			errors = append(errors,
				fmt.Sprintf("❌ Forbidden pattern found in PR body: '%s'", match),
				"   Pattern: "+pattern,
				"   Remove this content from your PR body",
			)
		}
	}

	return errors
}

// getForbiddenPatterns returns the list of forbidden patterns from config, or defaults
func (v *PRValidator) getForbiddenPatterns() []string {
	if v.config != nil && len(v.config.ForbiddenPatterns) > 0 {
		return v.config.ForbiddenPatterns
	}

	return defaultPRForbiddenPatterns
}
