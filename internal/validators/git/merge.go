package git

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

const (
	ghAPITimeout       = 30 * time.Second
	minPRBodyLineCount = 2
)

var (
	// ErrNoBranch is returned when current branch cannot be determined.
	ErrNoBranch = errors.New("could not determine current branch")

	// ErrGHCommandFailed is returned when gh command fails.
	ErrGHCommandFailed = errors.New("gh command failed")

	// ErrParsePRDetails is returned when PR details cannot be parsed.
	ErrParsePRDetails = errors.New("failed to parse PR details")
)

// PRDetails contains the details fetched from GitHub API.
type PRDetails struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Head   struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// MergeValidator validates gh pr merge commands and the resulting commit message.
type MergeValidator struct {
	validator.BaseValidator
	config      *config.MergeValidatorConfig
	gitRunner   GitRunner
	cmdRunner   exec.CommandRunner
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewMergeValidator creates a new MergeValidator instance.
func NewMergeValidator(
	log logger.Logger,
	gitRunner GitRunner,
	cfg *config.MergeValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *MergeValidator {
	if gitRunner == nil {
		gitRunner = NewGitRunner()
	}

	return &MergeValidator{
		BaseValidator: *validator.NewBaseValidator("validate-merge", log),
		config:        cfg,
		gitRunner:     gitRunner,
		cmdRunner:     exec.NewCommandRunner(ghAPITimeout),
		ruleAdapter:   ruleAdapter,
	}
}

// Validate checks gh pr merge command and validates the merge commit message.
func (v *MergeValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running merge validation")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Parse the command
	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(hookCtx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)

		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Find gh pr merge commands
	for _, cmd := range result.Commands {
		if !parser.IsGHPRMerge(&cmd) {
			continue
		}

		// Parse the merge command
		mergeCmd, err := parser.ParseGHMergeCommand(cmd)
		if err != nil {
			log.Debug("Failed to parse merge command", "error", err)

			continue
		}

		// Validate the merge command
		return v.validateMerge(ctx, mergeCmd)
	}

	log.Debug("No gh pr merge commands found")

	return validator.Pass()
}

// validateMerge validates a gh pr merge command.
func (v *MergeValidator) validateMerge(
	ctx context.Context,
	mergeCmd *parser.GHMergeCommand,
) *validator.Result {
	log := v.Logger()

	// Check if this is auto-merge and we should validate it
	if mergeCmd.IsAutoMerge() && !v.shouldValidateAutomerge() {
		log.Debug("Skipping auto-merge validation (disabled)")

		return validator.Pass()
	}

	// Only validate squash merges (they use PR title+body as commit message)
	if !mergeCmd.IsSquashMerge() {
		log.Debug("Skipping validation for non-squash merge")

		return validator.Pass()
	}

	// Fetch PR details
	prDetails, err := v.fetchPRDetails(ctx, mergeCmd)
	if err != nil {
		log.Error("Failed to fetch PR details", "error", err)

		return validator.Warn(fmt.Sprintf("Failed to fetch PR details: %v", err))
	}

	log.Debug("Fetched PR details",
		"number", prDetails.Number,
		"title", prDetails.Title,
	)

	// Validate the merge message (PR title + body)
	result := v.validateMergeMessage(prDetails)
	if !result.Passed {
		return result
	}

	// Validate signoff in merge command body (not PR body)
	return v.validateMergeCommandSignoff(mergeCmd)
}

// fetchPRDetails fetches PR details from GitHub API.
func (v *MergeValidator) fetchPRDetails(
	ctx context.Context,
	mergeCmd *parser.GHMergeCommand,
) (*PRDetails, error) {
	args, err := v.buildGHArgs(mergeCmd)
	if err != nil {
		return nil, err
	}

	result := v.cmdRunner.Run(ctx, "gh", args...)
	if result.Failed() {
		return nil, errors.Wrapf(ErrGHCommandFailed, "%s", result.Stderr)
	}

	var prDetails PRDetails
	if err := json.Unmarshal([]byte(result.Stdout), &prDetails); err != nil {
		return nil, errors.Wrap(ErrParsePRDetails, err.Error())
	}

	return &prDetails, nil
}

// buildGHArgs builds the gh command arguments for fetching PR details.
func (v *MergeValidator) buildGHArgs(mergeCmd *parser.GHMergeCommand) ([]string, error) {
	// If PR number is specified, use gh api
	if mergeCmd.PRNumber > 0 {
		return v.buildAPIArgs(mergeCmd), nil
	}

	// Otherwise, use gh pr view for current branch
	return v.buildPRViewArgs(mergeCmd)
}

// buildAPIArgs builds gh api command arguments.
func (*MergeValidator) buildAPIArgs(mergeCmd *parser.GHMergeCommand) []string {
	args := []string{"api"}

	if mergeCmd.Repo != "" {
		args = append(args, fmt.Sprintf("repos/%s/pulls/%d", mergeCmd.Repo, mergeCmd.PRNumber))
	} else {
		args = append(args, fmt.Sprintf("repos/{owner}/{repo}/pulls/%d", mergeCmd.PRNumber))
	}

	args = append(args, "--jq", ".")

	return args
}

// buildPRViewArgs builds gh pr view command arguments.
func (v *MergeValidator) buildPRViewArgs(mergeCmd *parser.GHMergeCommand) ([]string, error) {
	currentBranch := v.getCurrentBranch()
	if currentBranch == "" {
		return nil, ErrNoBranch
	}

	args := []string{"pr", "view", "--json", "number,title,body,state,head,base"}
	if mergeCmd.Repo != "" {
		args = append(args, "--repo", mergeCmd.Repo)
	}

	return args, nil
}

// getCurrentBranch returns the current git branch name.
func (v *MergeValidator) getCurrentBranch() string {
	if v.gitRunner == nil {
		return ""
	}

	branch, err := v.gitRunner.GetCurrentBranch()
	if err != nil {
		return ""
	}

	return branch
}

// validateMergeMessage validates the PR title + body as a commit message.
func (v *MergeValidator) validateMergeMessage(pr *PRDetails) *validator.Result {
	log := v.Logger()

	if !v.isMessageValidationEnabled() {
		log.Debug("Merge message validation is disabled")

		return validator.Pass()
	}

	var allErrors []string

	// 1. Validate PR title (commit message title)
	titleErrors := v.validateTitle(pr.Title)
	allErrors = append(allErrors, titleErrors...)

	// 2. Validate PR body (commit message body)
	bodyErrors := v.validateBody(pr.Body)
	allErrors = append(allErrors, bodyErrors...)

	// Build result
	if len(allErrors) > 0 {
		message := "PR merge message validation failed\n\n" + strings.Join(allErrors, "\n")
		message += fmt.Sprintf("\n\n📝 PR #%d: %s", pr.Number, pr.Title)

		return validator.FailWithRef(
			validator.RefGitMergeMessage,
			"PR merge message validation failed",
		).AddDetail("errors", message)
	}

	log.Debug("Merge message validation passed")

	return validator.Pass()
}

// validateMergeCommandSignoff validates that the merge command includes a signoff.
// The signoff should be in the --body or --body-file flag, not the PR body.
func (v *MergeValidator) validateMergeCommandSignoff(
	mergeCmd *parser.GHMergeCommand,
) *validator.Result {
	if !v.shouldRequireSignoff() {
		return validator.Pass()
	}

	// Check if --body flag contains signoff
	if mergeCmd.Body != "" {
		signoffErrors := v.validateSignoffInText(mergeCmd.Body)
		if len(signoffErrors) == 0 {
			return validator.Pass()
		}

		// Body provided but signoff is wrong
		if len(signoffErrors) > 1 {
			// Wrong signoff identity
			return validator.FailWithRef(
				validator.RefGitSignoffMismatch,
				"Wrong signoff identity in merge command body",
			).AddDetail("errors", strings.Join(signoffErrors, "\n"))
		}
	}

	// If --body-file is used, we can't validate the content here
	// Just warn that signoff should be included
	if mergeCmd.BodyFile != "" {
		return validator.Pass() // Assume the file contains signoff
	}

	// No body provided or body doesn't contain signoff
	expectedSignoff := v.getExpectedSignoff()
	signoffExample := "Signed-off-by: Your Name <your.email@klaudiu.sh>"

	if expectedSignoff != "" {
		signoffExample = "Signed-off-by: " + expectedSignoff
	}

	return validator.FailWithRef(
		validator.RefGitMergeSignoff,
		"Merge command missing Signed-off-by in commit body",
	).AddDetail("errors", fmt.Sprintf(
		"❌ Merge commit body missing Signed-off-by trailer\n"+
			"   Add --body flag with signoff:\n\n"+
			"   gh pr merge --body \"$(cat <<'EOF'\n"+
			"Your commit body here\n\n"+
			"%s\n"+
			"EOF\n"+
			")\"",
		signoffExample,
	))
}

// validateTitle validates the PR title as a commit message title.
func (v *MergeValidator) validateTitle(title string) []string {
	if title == "" {
		return []string{"❌ PR title is empty"}
	}

	var errs []string

	// Check title length
	errs = v.checkTitleLength(title, errs)

	// Check conventional commit format (skip for reverts)
	errs = v.checkTitleConventionalFormat(title, errs)

	return errs
}

// checkTitleLength validates the title length.
func (v *MergeValidator) checkTitleLength(title string, errs []string) []string {
	maxLength := v.getTitleMaxLength()
	isRevert := isRevertCommit(title)
	allowUnlimited := v.shouldAllowUnlimitedRevertTitle()

	// Skip length check for reverts if allowed
	if allowUnlimited && isRevert {
		return errs
	}

	if len(title) <= maxLength {
		return errs
	}

	errs = append(errs,
		fmt.Sprintf("❌ PR title exceeds %d characters (%d chars)", maxLength, len(title)),
		fmt.Sprintf("   Title: '%s'", title),
	)

	if allowUnlimited {
		errs = append(errs, "   Note: Revert titles (Revert \"...\") are exempt from this limit")
	}

	return errs
}

// checkTitleConventionalFormat validates the title follows conventional commit format.
func (v *MergeValidator) checkTitleConventionalFormat(title string, errs []string) []string {
	if !v.shouldCheckConventionalCommits() {
		return errs
	}

	if isRevertCommit(title) {
		return errs
	}

	parserOpts := []CommitParserOption{
		WithValidTypes(v.getValidTypes()),
	}
	commitParser := NewCommitParser(parserOpts...)
	parsed := commitParser.Parse(title)

	// Check format validity
	if !parsed.Valid || parsed.ParseError != "" {
		errs = append(errs,
			"❌ PR title doesn't follow conventional commits format: type(scope): description",
			"   Valid types: "+strings.Join(v.getValidTypes(), ", "),
			fmt.Sprintf("   Current title: '%s'", title),
		)

		return errs
	}

	// Check scope requirement
	if v.shouldRequireScope() && parsed.Scope == "" {
		errs = append(errs,
			"❌ PR title requires a scope: type(scope): description",
			fmt.Sprintf("   Current title: '%s'", title),
		)
	}

	// Check for infra scope misuse
	if v.shouldBlockInfraScopeMisuse() {
		rule := NewInfraScopeMisuseRule()
		result := rule.Validate(parsed, title)

		if result != nil && len(result.Errors) > 0 {
			errs = append(errs, result.Errors...)
		}
	}

	return errs
}

// validateBody validates the PR body as a commit message body.
func (v *MergeValidator) validateBody(body string) []string {
	if body == "" {
		return nil // Empty body is allowed
	}

	var validationErrors []string

	// Check body line lengths
	validationErrors = v.checkBodyLineLength(body, validationErrors)

	// Check for PR references
	validationErrors = v.checkPRReferences(body, validationErrors)

	// Check for AI attribution
	validationErrors = v.checkAIAttribution(body, validationErrors)

	// Check for forbidden patterns
	validationErrors = v.checkForbiddenPatterns(body, validationErrors)

	// Check list formatting
	validationErrors = v.checkListFormatting(body, validationErrors)

	return validationErrors
}

// checkBodyLineLength checks body line lengths.
func (v *MergeValidator) checkBodyLineLength(body string, errs []string) []string {
	maxLen := v.getBodyMaxLineLength()
	tolerance := v.getBodyLineTolerance()
	rule := NewBodyLineLengthRule(maxLen, tolerance)
	result := rule.Validate(nil, body)

	if result != nil && len(result.Errors) > 0 {
		errs = append(errs, result.Errors...)
	}

	return errs
}

// checkPRReferences checks for PR references in body.
func (v *MergeValidator) checkPRReferences(body string, errs []string) []string {
	if !v.shouldBlockPRReferences() {
		return errs
	}

	prRule := NewPRReferenceRule()
	result := prRule.Validate(nil, body)

	if result != nil && len(result.Errors) > 0 {
		errs = append(errs, result.Errors...)
	}

	return errs
}

// checkAIAttribution checks for AI attribution in body.
func (v *MergeValidator) checkAIAttribution(body string, errs []string) []string {
	if !v.shouldBlockAIAttribution() {
		return errs
	}

	aiRule := NewAIAttributionRule()
	result := aiRule.Validate(nil, body)

	if result != nil && len(result.Errors) > 0 {
		errs = append(errs, result.Errors...)
	}

	return errs
}

// checkForbiddenPatterns checks for forbidden patterns in body.
func (v *MergeValidator) checkForbiddenPatterns(body string, errs []string) []string {
	forbiddenRule := &ForbiddenPatternRule{
		Patterns: v.getForbiddenPatterns(),
	}
	result := forbiddenRule.Validate(nil, body)

	if result != nil && len(result.Errors) > 0 {
		errs = append(errs, result.Errors...)
	}

	return errs
}

// checkListFormatting checks list formatting in body.
func (*MergeValidator) checkListFormatting(body string, errs []string) []string {
	lines := strings.Split(body, "\n")
	if len(lines) <= minPRBodyLineCount {
		return errs
	}

	listRule := NewListFormattingRule()
	result := listRule.Validate(nil, body)

	if result != nil && len(result.Errors) > 0 {
		errs = append(errs, result.Errors...)
	}

	return errs
}

// validateSignoffInText validates that text contains a valid Signed-off-by trailer.
// Used to validate the merge command's --body flag content.
func (v *MergeValidator) validateSignoffInText(text string) []string {
	// Check if Signed-off-by is present
	if !strings.Contains(text, "Signed-off-by:") {
		return []string{"missing Signed-off-by trailer"}
	}

	// If expected signoff is set, validate it matches
	expectedSignoff := v.getExpectedSignoff()
	if expectedSignoff != "" {
		expectedLine := "Signed-off-by: " + expectedSignoff

		if !strings.Contains(text, expectedLine) {
			// Find the actual signoff line
			lines := strings.Split(text, "\n")
			actualSignoff := ""

			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "Signed-off-by:") {
					actualSignoff = strings.TrimSpace(line)

					break
				}
			}

			return []string{
				"wrong signoff identity",
				"found: " + actualSignoff,
				"expected: " + expectedLine,
			}
		}
	}

	return nil
}

// Configuration accessor methods

func (v *MergeValidator) isMessageValidationEnabled() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.Enabled != nil {
		return *v.config.Message.Enabled
	}

	return true // Default: enabled
}

func (v *MergeValidator) shouldValidateAutomerge() bool {
	if v.config != nil && v.config.ValidateAutomerge != nil {
		return *v.config.ValidateAutomerge
	}

	return true // Default: validate auto-merge
}

func (v *MergeValidator) shouldRequireSignoff() bool {
	if v.config != nil && v.config.RequireSignoff != nil {
		return *v.config.RequireSignoff
	}

	return true // Default: require signoff
}

func (v *MergeValidator) getExpectedSignoff() string {
	if v.config != nil {
		return v.config.ExpectedSignoff
	}

	return ""
}

func (v *MergeValidator) getTitleMaxLength() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.TitleMaxLength != nil {
		return *v.config.Message.TitleMaxLength
	}

	return defaultMaxTitleLength
}

func (v *MergeValidator) shouldAllowUnlimitedRevertTitle() bool {
	if v.config == nil || v.config.Message == nil {
		return true // Default: allow unlimited revert title
	}

	if v.config.Message.AllowUnlimitedRevertTitle != nil {
		return *v.config.Message.AllowUnlimitedRevertTitle
	}

	return true // Default: allow unlimited revert title
}

func (v *MergeValidator) getBodyMaxLineLength() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.BodyMaxLineLength != nil {
		return *v.config.Message.BodyMaxLineLength
	}

	return defaultMaxBodyLineLength
}

func (v *MergeValidator) getBodyLineTolerance() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.BodyLineTolerance != nil {
		return *v.config.Message.BodyLineTolerance
	}

	return defaultBodyLineTolerance
}

func (v *MergeValidator) shouldCheckConventionalCommits() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.ConventionalCommits != nil {
		return *v.config.Message.ConventionalCommits
	}

	return true // Default: enabled
}

func (v *MergeValidator) getValidTypes() []string {
	if v.config != nil && v.config.Message != nil && len(v.config.Message.ValidTypes) > 0 {
		return v.config.Message.ValidTypes
	}

	return defaultValidTypes
}

func (v *MergeValidator) shouldRequireScope() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.RequireScope != nil {
		return *v.config.Message.RequireScope
	}

	return true // Default: require scope
}

func (v *MergeValidator) shouldBlockInfraScopeMisuse() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockInfraScopeMisuse != nil {
		return *v.config.Message.BlockInfraScopeMisuse
	}

	return true // Default: block infra scope misuse
}

func (v *MergeValidator) shouldBlockPRReferences() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockPRReferences != nil {
		return *v.config.Message.BlockPRReferences
	}

	return true // Default: block PR references
}

func (v *MergeValidator) shouldBlockAIAttribution() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.BlockAIAttribution != nil {
		return *v.config.Message.BlockAIAttribution
	}

	return true // Default: block AI attribution
}

func (v *MergeValidator) getForbiddenPatterns() []string {
	if v.config != nil && v.config.Message != nil && len(v.config.Message.ForbiddenPatterns) > 0 {
		return v.config.Message.ForbiddenPatterns
	}

	return defaultForbiddenPatterns
}

// Category returns the validator category for parallel execution.
func (*MergeValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO // Uses gh CLI (external process)
}

// Ensure MergeValidator implements validator.Validator
var _ validator.Validator = (*MergeValidator)(nil)
