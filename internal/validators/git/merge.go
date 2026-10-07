package git

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

const (
	ghAPITimeout       = 30 * time.Second
	minPRBodyLineCount = 2

	prTitleLabel = "PR title"
	subjectLabel = "Squash commit subject (--subject or commit_title)"

	ghMergeSignoffHint = "Add --body flag with signoff:\n\n" +
		"gh pr merge --body \"$(cat <<'EOF'\n" +
		"Your commit body here\n\n" +
		"%s\n" +
		"EOF\n" +
		")\""

	restMergeSignoffHint = "Send the signoff in the commit_message field:\n\n" +
		"gh api -X PUT repos/{owner}/{repo}/pulls/{number}/merge " +
		"-f merge_method=squash -f commit_message=\"$(cat <<'EOF'\n" +
		"Your commit body here\n\n" +
		"%s\n" +
		"EOF\n" +
		")\""
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
	config    *config.MergeValidatorConfig
	gitRunner GitRunner
	cmdRunner exec.CommandRunner
	apiHosts  []string
}

// NewMergeValidator creates a new MergeValidator instance.
func NewMergeValidator(
	log logger.Logger,
	gitRunner GitRunner,
	cfg *config.MergeValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *MergeValidator {
	return &MergeValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules(
			"validate-merge", log, ruleAdapter,
		),
		config:    cfg,
		gitRunner: defaultGitRunner(gitRunner),
		cmdRunner: exec.NewCommandRunner(ghAPITimeout),
	}
}

// Validate checks gh pr merge command and validates the merge commit message.
func (v *MergeValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running merge validation")

	// Check rules first
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	// Parse the command
	result, err := hookCtx.ParsedCommand()
	if err != nil {
		log.Error("Failed to parse command", "error", err)

		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	targets := v.findMerges(result)
	if len(targets) == 0 {
		log.Debug("No gh pr merge commands found")

		return validator.Pass()
	}

	// Every merge on the line is checked, so a harmless first merge cannot
	// carry an unchecked second one past the validator.
	var warning *validator.Result

	for _, target := range targets {
		res := v.validateTarget(ctx, target)

		switch {
		case res.ShouldBlock:
			return res
		case !res.Passed && warning == nil:
			warning = res
		}
	}

	if warning != nil {
		return warning
	}

	return validator.Pass()
}

// mergeTarget is one pull request merge on the command line, from gh pr merge
// or from the REST API, with what is needed to check it.
type mergeTarget struct {
	cmd         *parser.GHMergeCommand
	signoffHint string

	// unlistedHost is a REST host outside the configured GitHub API hosts.
	// Its pull request is not fetched, so gh never sends a token there.
	unlistedHost string
}

// findMerges returns every gh pr merge and REST pull request merge, in order.
func (v *MergeValidator) findMerges(result *parser.ParseResult) []mergeTarget {
	var targets []mergeTarget

	for _, cmd := range result.Commands {
		if parser.IsGHPRMerge(&cmd) {
			mergeCmd, err := parser.ParseGHMergeCommand(cmd)
			if err != nil {
				v.Logger().Debug("Failed to parse merge command", "error", err)

				continue
			}

			targets = append(targets, mergeTarget{cmd: mergeCmd, signoffHint: ghMergeSignoffHint})

			continue
		}

		targets = append(targets, v.restMerges(result, cmd)...)
	}

	return targets
}

// validateTarget checks one merge.
func (v *MergeValidator) validateTarget(ctx context.Context, target mergeTarget) *validator.Result {
	if target.unlistedHost == "" {
		return v.validateMerge(ctx, target.cmd, target.signoffHint)
	}

	if !target.cmd.IsSquashMerge() {
		return validator.Pass()
	}

	if res := v.validateSubjects(target.cmd); !res.Passed {
		return res
	}

	if res := v.validateMergeCommandSignoff(target.cmd, target.signoffHint); !res.Passed {
		return res
	}

	return validator.Warn(fmt.Sprintf(
		"Pull request title and body not checked: %s is not a configured GitHub API host "+
			"(validators.github.api.hosts), so its pull request is not fetched",
		target.unlistedHost,
	))
}

// validateMerge validates a gh pr merge command.
func (v *MergeValidator) validateMerge(
	ctx context.Context,
	mergeCmd *parser.GHMergeCommand,
	signoffHint string,
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

	// The subject needs no fetch, so a failed fetch cannot let it through.
	if res := v.validateSubjects(mergeCmd); !res.Passed {
		return res
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
	result := v.validateMergeMessage(prDetails, mergeSubjects(mergeCmd))
	if !result.Passed {
		return result
	}

	// Validate signoff in merge command body (not PR body)
	return v.validateMergeCommandSignoff(mergeCmd, signoffHint)
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
	if mergeCmd.PRNumber > 0 || mergeCmd.APIPath != "" {
		return v.buildAPIArgs(mergeCmd), nil
	}

	// Otherwise, use gh pr view for current branch
	return v.buildPRViewArgs(mergeCmd)
}

// buildAPIArgs builds gh api command arguments.
func (*MergeValidator) buildAPIArgs(mergeCmd *parser.GHMergeCommand) []string {
	args := []string{"api"}

	if mergeCmd.Hostname != "" {
		args = append(args, "--hostname="+mergeCmd.Hostname)
	}

	switch {
	case mergeCmd.APIPath != "":
		args = append(args, mergeCmd.APIPath)
	case mergeCmd.Repo != "":
		args = append(args, fmt.Sprintf("repos/%s/pulls/%d", mergeCmd.Repo, mergeCmd.PRNumber))
	default:
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

// validateMergeMessage validates the PR title + body as a commit message. The
// subjects, already checked, only name the commit in the preview.
func (v *MergeValidator) validateMergeMessage(pr *PRDetails, subjects []string) *validator.Result {
	log := v.Logger()

	if !v.isMessageValidationEnabled() {
		log.Debug("Merge message validation is disabled")

		return validator.Pass()
	}

	const typicalErrorCount = 5 // Typical number of title + body errors

	allErrors := make([]string, 0, typicalErrorCount)

	// 1. Validate PR title (commit message title)
	titleErrors := v.validateTitle(pr.Title)
	allErrors = append(allErrors, titleErrors...)

	// 2. Validate PR body (commit message body)
	bodyErrors := v.validateBody(pr.Body)
	allErrors = append(allErrors, bodyErrors...)

	preview := pr.Title
	if len(subjects) > 0 {
		preview = subjects[0]
	}

	commitPreview := fmt.Sprintf("PR #%d: %s", pr.Number, preview)
	if res := mergeMessageResult(allErrors, commitPreview); res != nil {
		return res
	}

	log.Debug("Merge message validation passed")

	return validator.Pass()
}

// validateSubjects validates the subjects a merge sets in place of the PR
// title. It needs no pull request fetch.
func (v *MergeValidator) validateSubjects(mergeCmd *parser.GHMergeCommand) *validator.Result {
	if !v.isMessageValidationEnabled() {
		return validator.Pass()
	}

	subjects := mergeSubjects(mergeCmd)
	if len(subjects) == 0 {
		return validator.Pass()
	}

	if res := mergeMessageResult(v.subjectErrors(subjects), subjects[0]); res != nil {
		return res
	}

	return validator.Pass()
}

// mergeSubjects returns the squash commit subjects a merge sets in place of
// the PR title: gh pr merge --subject, or each REST commit_title value. An
// empty subject leaves the PR title in place, so it is skipped.
func mergeSubjects(mergeCmd *parser.GHMergeCommand) []string {
	var subjects []string

	for _, subject := range append([]string{mergeCmd.Subject}, mergeCmd.AltSubjects...) {
		if subject != "" && !slices.Contains(subjects, subject) {
			subjects = append(subjects, subject)
		}
	}

	return subjects
}

// subjectErrors applies the PR title rules to each subject.
func (v *MergeValidator) subjectErrors(subjects []string) []string {
	errs := make([]string, 0, len(subjects))

	for _, subject := range subjects {
		errs = append(errs, v.validateTitleAs(subjectLabel, subject)...)
	}

	return errs
}

// mergeMessageResult builds the GIT017 failure for the given errors, or nil
// when there are none.
func mergeMessageResult(allErrors []string, preview string) *validator.Result {
	if len(allErrors) == 0 {
		return nil
	}

	var details strings.Builder

	// Skip first error in details (already in Message)
	for _, e := range allErrors[1:] {
		details.WriteString(e)
		details.WriteString("\n")
	}

	result := validator.FailWithRef(validator.RefGitMergeMessage, allErrors[0])

	if details.Len() > 0 {
		result = result.AddDetail("errors", details.String())
	}

	return result.AddDetail("commit_preview", preview)
}

// validateMergeCommandSignoff validates that the merge command includes a signoff.
// The signoff should be in the --body or --body-file flag, not the PR body.
func (v *MergeValidator) validateMergeCommandSignoff(
	mergeCmd *parser.GHMergeCommand,
	signoffHint string,
) *validator.Result {
	if !v.shouldRequireSignoff() {
		return validator.Pass()
	}

	if res := v.checkBodySignoff(mergeCmd.Body, mergeCmd.BodyFile, signoffHint); !res.Passed {
		return res
	}

	// Every other commit_message a REST merge sends must carry it too.
	for _, body := range mergeCmd.AltBodies {
		if res := v.checkBodySignoff(body, "", signoffHint); !res.Passed {
			return res
		}
	}

	return validator.Pass()
}

// checkBodySignoff checks one merge commit body for the signoff.
func (v *MergeValidator) checkBodySignoff(body, bodyFile, signoffHint string) *validator.Result {
	// Check if --body flag contains signoff
	if body != "" {
		signoffErrors := v.validateSignoffInText(body)
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
	if bodyFile != "" {
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
	).AddDetail("errors", fmt.Sprintf(signoffHint, signoffExample))
}

// validateTitle validates the PR title as a commit message title.
func (v *MergeValidator) validateTitle(title string) []string {
	return v.validateTitleAs(prTitleLabel, title)
}

// validateTitleAs validates a commit message title, naming it label in errors.
func (v *MergeValidator) validateTitleAs(label, title string) []string {
	if title == "" {
		return []string{label + " is empty"}
	}

	var errs []string

	// Check title length
	errs = v.checkTitleLength(label, title, errs)

	// Check conventional commit format (skip for reverts)
	errs = v.checkTitleConventionalFormat(label, title, errs)

	return errs
}

// checkTitleLength validates the title length.
func (v *MergeValidator) checkTitleLength(label, title string, errs []string) []string {
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
		fmt.Sprintf("%s exceeds %d characters (%d chars)", label, maxLength, len(title)),
		fmt.Sprintf("Title: '%s'", title),
	)

	if allowUnlimited {
		errs = append(errs, "Revert titles (Revert \"...\") are exempt from this limit")
	}

	return errs
}

// checkTitleConventionalFormat validates the title follows conventional commit format.
func (v *MergeValidator) checkTitleConventionalFormat(
	label, title string,
	errs []string,
) []string {
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
			label+" doesn't follow conventional commits format: type(scope): description",
			"Valid types: "+strings.Join(v.getValidTypes(), ", "),
			fmt.Sprintf("Current title: '%s'", title),
		)

		return errs
	}

	// Check scope requirement
	if v.shouldRequireScope() && parsed.Scope == "" {
		errs = append(errs,
			label+" requires a scope: type(scope): description",
			fmt.Sprintf("Current title: '%s'", title),
		)
	}

	// Check for infra scope misuse
	if v.shouldBlockInfraScopeMisuse() {
		rule := NewInfraScopeMisuseRule()
		result := rule.Validate(parsed, title)

		if result != nil && result.Message != "" {
			errs = append(errs, result.Message)
			errs = append(errs, result.Context...)
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

	if result != nil && result.Message != "" {
		errs = append(errs, result.Message)
		errs = append(errs, result.Context...)
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

	if result != nil && result.Message != "" {
		errs = append(errs, result.Message)
		errs = append(errs, result.Context...)
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

	if result != nil && result.Message != "" {
		errs = append(errs, result.Message)
		errs = append(errs, result.Context...)
	}

	return errs
}

// checkForbiddenPatterns checks for forbidden patterns in body.
func (v *MergeValidator) checkForbiddenPatterns(body string, errs []string) []string {
	forbiddenRule := &ForbiddenPatternRule{
		Patterns: v.getForbiddenPatterns(),
	}
	result := forbiddenRule.Validate(nil, body)

	if result != nil && result.Message != "" {
		errs = append(errs, result.Message)
		errs = append(errs, result.Context...)
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

	if result != nil && result.Message != "" {
		errs = append(errs, result.Message)
		errs = append(errs, result.Context...)
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

	return config.DefaultTitleMaxLength
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

	return config.DefaultBodyMaxLineLength
}

func (v *MergeValidator) getBodyLineTolerance() int {
	if v.config != nil && v.config.Message != nil && v.config.Message.BodyLineTolerance != nil {
		return *v.config.Message.BodyLineTolerance
	}

	return config.DefaultBodyLineTolerance
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

	return config.DefaultValidTypes
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

	return config.DefaultForbiddenPatterns
}

// Category returns the validator category for parallel execution.
func (*MergeValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO // Uses gh CLI (external process)
}

// Ensure MergeValidator implements validator.Validator
var _ validator.Validator = (*MergeValidator)(nil)
