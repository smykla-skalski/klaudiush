package git

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

const (
	ghCommand       = "gh"
	prSubcommand    = "pr"
	createOperation = "create"
	editOperation   = "edit"
	minGHPRArgs     = 2

	// maxBodyFileBytes caps how much of a --body-file is read for validation.
	maxBodyFileBytes = 1 << 20
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
	bodyFileRegex      = regexp.MustCompile(
		`(?:--body-file|-F)[=\s]+("[^"]+"|'[^']+'|[^\s)'";|&]+)`,
	)

	// apiBodyFileRegex captures a request body read from a file: gh api
	// --input FILE or -F field=@FILE, and an HTTP client's --data @FILE.
	apiBodyFileRegex = regexp.MustCompile(
		`(?:--input[=\s]+|--data(?:-raw|-binary|-ascii)?[=\s]*@|-d\s*@|-F\s*[^\s=]+=@)` +
			`("[^"]+"|'[^']+'|[^\s)'";|&]+)`,
	)

	// prWriteRegex matches any command line that writes a pull request title or
	// body: a gh pr create/edit however it is wrapped (env, sudo, bash -c, a
	// script), a REST call to a pulls endpoint, or a createPullRequest /
	// updatePullRequest GraphQL mutation. The request text always survives the
	// wrapping, so matching the text covers every wrapper without modelling any
	// of them.
	prWriteRegex = regexp.MustCompile(PRWritePattern)

	// PRWritePattern matches any command line that writes a pull request title
	// or body. It is also the validator's registration predicate, so a command
	// the validator would inspect always reaches it - a substring gate would
	// miss "gh  pr  create". A merge is included because --body becomes the
	// squash commit body.
	PRWritePattern = `\bgh\s+pr\s+(?:create|edit|merge)\b` +
		`|repos/[^/\s"']+/[^/\s"']+/pulls` +
		`|(?i:createPullRequest|updatePullRequest)`

	// MCPPullRequestToolPattern matches the MCP tools that create or update a
	// pull request, e.g. "mcp__github__create_pull_request".
	MCPPullRequestToolPattern = `(?i)(create|update)_pull_request`
)

// PRValidator validates gh pr create commands
type PRValidator struct {
	validator.BaseValidator
	config *config.PRValidatorConfig
}

// NewPRValidator creates a new PRValidator instance
func NewPRValidator(
	cfg *config.PRValidatorConfig,
	log logger.Logger,
	ruleAdapter validator.RuleChecker,
) *PRValidator {
	return &PRValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules(
			"validate-pr", log, ruleAdapter,
		),
		config: cfg,
	}
}

// getTitleMaxLength returns the maximum allowed length for PR titles
func (v *PRValidator) getTitleMaxLength() int {
	if v.config != nil && v.config.TitleMaxLength != nil {
		return *v.config.TitleMaxLength
	}

	return config.DefaultTitleMaxLength
}

// isTitleConventionalCommitsEnabled returns whether conventional commit format is required for titles
func (v *PRValidator) isTitleConventionalCommitsEnabled() bool {
	if v.config != nil && v.config.TitleConventionalCommits != nil {
		return *v.config.TitleConventionalCommits
	}

	return true // default: enabled
}

// getTitleStyle returns the title format style to enforce.
// Supports the same options as commit_style in CommitMessageConfig.
// Falls back to TitleConventionalCommits for backwards compatibility.
func (v *PRValidator) getTitleStyle() string {
	if v.config != nil && v.config.TitleStyle != "" {
		return v.config.TitleStyle
	}

	// Legacy: TitleConventionalCommits = false maps to "none"
	if !v.isTitleConventionalCommitsEnabled() {
		return commitStyleNone
	}

	return commitStyleConventional
}

// getPRTitlePattern returns the compiled title pattern regex, or nil if not configured.
func (v *PRValidator) getPRTitlePattern() *regexp.Regexp {
	if v.config == nil || v.config.TitlePattern == "" {
		return nil
	}

	pattern, err := regexp.Compile(v.config.TitlePattern)
	if err != nil {
		return nil
	}

	return pattern
}

// buildPRTitleFormatRules returns the format rules for PR title validation
// based on the configured title_style.
func (v *PRValidator) buildPRTitleFormatRules(ctx context.Context) []CommitRule {
	var formatRules []CommitRule

	switch v.getTitleStyle() {
	case commitStyleConventional:
		// PR titles don't require scope (scope is optional)
		formatRules = append(formatRules, &ConventionalFormatRule{
			ValidTypes:   v.getValidTypes(),
			RequireScope: false,
		})
		formatRules = append(formatRules, NewInfraScopeMisuseRule())

	case commitStyleScopeOnly:
		formatRules = append(formatRules, &ScopeOnlyFormatRule{})

	case commitStyleCustom:
		if pattern := v.getPRTitlePattern(); pattern != nil {
			formatRules = append(formatRules, &CustomPatternRule{Pattern: pattern})
		}

	case commitStyleNone:
		// no format rule

	case commitStyleAuto:
		detected := NewCommitStyleDetector().Detect(ctx)
		if detected == commitStyleScopeOnly {
			formatRules = append(formatRules, &ScopeOnlyFormatRule{})
		} else {
			formatRules = append(formatRules, &ConventionalFormatRule{
				ValidTypes:   v.getValidTypes(),
				RequireScope: false,
			})
			formatRules = append(formatRules, NewInfraScopeMisuseRule())
		}
	}

	return formatRules
}

// parsedCommitForPRTitle builds a ParsedCommit from a PR title string.
// PR titles are single-line commit subjects, so we parse just the title.
func (v *PRValidator) parsedCommitForPRTitle(title string) *ParsedCommit {
	parser := NewCommitParser(WithValidTypes(v.getValidTypes()))
	return parser.Parse(title)
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
	return config.DefaultValidTypes
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

	// Check rules first
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	if !hookCtx.IsBashTool() {
		return v.validateMCPPullRequest(hookCtx)
	}

	// Parse the command
	result, err := hookCtx.ParsedCommand()
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	fullCmd := hookCtx.GetCommand()

	// A body file is named relative to the directory the command runs in, which
	// a preceding "cd" may have changed, so every directory in the chain is a
	// candidate base for resolving it.
	dirs := commandDirs(result, hookCtx.GetWorkingDir())

	op := ""

	for _, cmd := range result.Commands {
		if found := ghPROperation(&cmd); found != "" {
			op = found

			break
		}
	}

	// A pull request written any other way - through the GitHub API, or through
	// a gh pr the bash parser reports under a wrapper - is still a pull request
	// description, so it gets the same attribution check even though none of the
	// gh pr contract applies to it.
	if op == "" && !prWriteRegex.MatchString(fullCmd) {
		log.Debug("No pull request write found")

		return validator.Pass()
	}

	// gh takes the body from the last --body-file it is given, so only that file
	// is the pull request body - an earlier one is never sent.
	bodyFile := v.readBodyFiles(bodyFileRegex, fullCmd, dirs, true)

	// Attribution is checked before anything else and against the whole command,
	// so no flag spelling can carry a footer past it.
	if v.shouldBlockAIAttribution() {
		apiBody := v.readBodyFiles(apiBodyFileRegex, fullCmd, dirs, false)
		if attrResult := v.checkAIAttribution(
			fullCmd + "\n" + bodyFile + "\n" + apiBody,
		); attrResult != nil {
			return attrResult
		}
	}

	// Only gh pr create carries the full title/body/label contract: an edit is a
	// partial update, and an API call is not a gh pr invocation at all.
	if op != createOperation {
		return validator.Pass()
	}

	return v.validatePR(ctx, v.extractPRData(fullCmd, bodyFile))
}

// validateMCPPullRequest checks an MCP tool call that creates or updates a pull
// request. Such a call never reaches a shell, so no command text exists to
// scan - the title and body arrive as tool arguments instead.
func (v *PRValidator) validateMCPPullRequest(hookCtx *hook.Context) *validator.Result {
	if result := v.checkAIAttribution(mcpToolText(hookCtx)); result != nil {
		return result
	}

	return validator.Pass()
}

// mcpToolText joins every string argument of an MCP tool call, so attribution
// is caught whichever field name the server uses for the description.
func mcpToolText(hookCtx *hook.Context) string {
	var text strings.Builder

	text.WriteString(hookCtx.ToolInput.Content)
	text.WriteString("\n")

	for _, raw := range hookCtx.ToolInput.Additional {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}

		text.WriteString(value)
		text.WriteString("\n")
	}

	return text.String()
}

// ghPROperation returns the gh pr subcommand being run - "create" or "edit" -
// or "" when the command is neither.
func ghPROperation(cmd *parser.Command) string {
	if cmd.Name != ghCommand || len(cmd.Args) < minGHPRArgs {
		return ""
	}

	if cmd.Args[0] != prSubcommand {
		return ""
	}

	if cmd.Args[1] == createOperation || cmd.Args[1] == editOperation {
		return cmd.Args[1]
	}

	return ""
}

// readBodyFiles reads the files the command takes a request body from, so their
// text is validated like an inline body. With lastOnly set, only the last match
// is read - the way a repeated flag overwrites its earlier value. An unreadable
// file, stdin, or an unresolved expansion contributes nothing - heredoc text
// handed to "--body-file -" is already part of the command string.
func (v *PRValidator) readBodyFiles(
	re *regexp.Regexp,
	command string,
	dirs []string,
	lastOnly bool,
) string {
	matches := re.FindAllStringSubmatch(command, -1)
	if lastOnly && len(matches) > 1 {
		matches = matches[len(matches)-1:]
	}

	var contents strings.Builder

	for _, match := range matches {
		path := strings.Trim(match[1], `"'`)

		// "-" is stdin, a "$" or backtick is an unresolved expansion, and an "="
		// means the token is a gh api field ("-F title=x"), not a path.
		if path == "-" || strings.ContainsAny(path, "$`=") {
			continue
		}

		data, ok := v.readBodyFile(path, dirs)
		if !ok {
			continue
		}

		contents.WriteString(data)
		contents.WriteString("\n")
	}

	return contents.String()
}

// readBodyFile reads a body file named on the command line, resolving a
// relative path against each candidate directory until one holds it.
func (v *PRValidator) readBodyFile(path string, dirs []string) (string, bool) {
	if filepath.IsAbs(path) {
		return v.readCappedFile(filepath.Clean(path))
	}

	for _, dir := range dirs {
		if data, ok := v.readCappedFile(filepath.Clean(filepath.Join(dir, path))); ok {
			return data, true
		}
	}

	return "", false
}

// commandDirs returns every effective working directory in the parsed command
// line, resolved against the hook's own directory, with that directory first.
// Each "cd" target is added as well: the parser does not carry the effective
// directory into a subshell, so "(cd docs && gh pr edit --body-file x.md)"
// would otherwise resolve the file against the wrong directory.
func commandDirs(result *parser.ParseResult, hookDir string) []string {
	dirs := []string{hookDir}

	add := func(dir string) {
		if dir == "" {
			return
		}

		if !filepath.IsAbs(dir) {
			dir = filepath.Join(hookDir, dir)
		}

		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}

	for _, cmd := range result.Commands {
		add(cmd.WorkingDirectory)

		if cmd.Name == "cd" && len(cmd.Args) > 0 {
			add(cmd.Args[0])
		}
	}

	return dirs
}

// readCappedFile reads at most maxBodyFileBytes of a regular file.
func (v *PRValidator) readCappedFile(path string) (string, bool) {
	return validators.ReadCapped(v.Logger(), path, maxBodyFileBytes)
}

// PRData holds extracted PR metadata
type PRData struct {
	Title      string
	Body       string
	BaseBranch string
	Labels     []string
	HasLabels  bool
	// TitleIsExpansion / BodyIsExpansion report that the value came from a
	// double-quoted argument and is a single unresolved shell expansion (e.g.
	// --title "$TITLE"), so its runtime value can't be validated. Single-quoted
	// values are literal in shell and are never marked, so they stay validated.
	TitleIsExpansion bool
	BodyIsExpansion  bool
}

// extractPRData extracts PR title, body, base branch, and labels from gh command.
// bodyFile carries the already-read contents of a --body-file, used when no
// inline body is present.
func (v *PRValidator) extractPRData(command, bodyFile string) PRData {
	data := PRData{
		Labels: []string{},
	}

	// Extract title (try double quotes first, then single quotes). Only a
	// double-quoted value undergoes shell expansion, so only it can be a bare
	// expansion to skip; a single-quoted value is a literal and stays validated.
	if matches := prTitleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Title = matches[1]
		data.TitleIsExpansion = isBarePRExpansion(matches[1])
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

	// Extract body - try heredoc first, then quoted strings. As with the title,
	// only a double-quoted body can be a bare expansion to skip; heredoc bodies
	// and single-quoted bodies are literal and stay validated.
	if matches := heredocRegex.FindStringSubmatch(command); len(matches) > 1 {
		// Add trailing newline for markdownlint MD047 rule
		data.Body = matches[1] + "\n"
	} else if matches := bodyRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Body = matches[1] + "\n"
		data.BodyIsExpansion = isBarePRExpansion(matches[1])
	} else if matches := bodySingleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Body = matches[1] + "\n"
	} else if bodyFile != "" {
		data.Body = bodyFile
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
	const (
		typicalErrorCount   = 10 // Typical number of PR validation errors
		typicalWarningCount = 5  // Typical number of PR validation warnings
	)

	allErrors := make([]string, 0, typicalErrorCount)

	allWarnings := make([]string, 0, typicalWarningCount)

	// 1. Validate PR title
	v.validatePRTitleData(ctx, data.Title, data.TitleIsExpansion, &allErrors, &allWarnings)

	// An unresolved expansion (e.g. --title "$TITLE", --body "$(...)") can't be
	// inspected, so treat it as empty for every content-based check below. This
	// keeps the raw token from being pattern-matched, linted, used for type
	// detection, or surfaced in the result.
	titleForChecks := data.Title
	if data.TitleIsExpansion {
		titleForChecks = ""
	}

	bodyForChecks := data.Body
	if data.BodyIsExpansion {
		bodyForChecks = ""
	}

	// 2. Check for forbidden patterns in title and body
	forbiddenErrors := v.checkForbiddenPatterns(titleForChecks, bodyForChecks)
	allErrors = append(allErrors, forbiddenErrors...)

	// 3. Extract PR type for body validation
	validTypes := v.getValidTypes()
	prType := extractPRType(titleForChecks, validTypes)

	// 4. Validate PR body
	v.validatePRBodyData(data.Body, prType, data.BodyIsExpansion, &allErrors, &allWarnings)

	// 5. Validate markdown formatting
	if bodyForChecks != "" {
		// External markdownlint validation
		disabledRules := v.getMarkdownDisabledRules()
		mdResult := ValidatePRMarkdown(ctx, bodyForChecks, disabledRules)
		allWarnings = append(allWarnings, mdResult.Errors...)

		// Internal markdown validation (code block indentation, empty lines, etc.)
		internalMdResult := validators.AnalyzeMarkdown(bodyForChecks, nil)
		allWarnings = append(allWarnings, internalMdResult.Warnings...)
	}

	// 6. Validate base branch labels
	validateBaseBranchLabels(data, &allErrors)

	// 7. Validate CI label heuristics (if enabled)
	if v.isCheckCILabelsEnabled() && titleForChecks != "" && bodyForChecks != "" {
		ciWarnings := v.checkCILabelHeuristics(data, prType)
		allWarnings = append(allWarnings, ciWarnings...)
	}

	// Redact an unresolved-expansion title from the preview so a "$(...)" form is
	// not echoed back in the hook output.
	previewTitle := data.Title
	if data.TitleIsExpansion {
		previewTitle = "(unresolved expansion)"
	}

	return v.buildResult(allErrors, allWarnings, previewTitle)
}

// validatePRTitleData validates the PR title using commit rules.
func (v *PRValidator) validatePRTitleData(
	ctx context.Context,
	title string,
	titleIsExpansion bool,
	allErrors, allWarnings *[]string,
) {
	if title == "" {
		*allWarnings = append(
			*allWarnings,
			"Could not extract PR title - ensure you're using --title flag",
		)

		return
	}

	// A bare variable title (e.g. gh pr create --title "$TITLE") is an unresolved
	// expansion whose runtime value the hook cannot see, so its format can't be
	// checked. Skip rather than reject a valid title. The value is omitted from
	// the log since a "$(...)" form can carry sensitive command content.
	if titleIsExpansion {
		v.Logger().Debug("PR title is an unresolved expansion; skipping validation")

		return
	}

	// Check title length
	lengthRule := &TitleLengthRule{
		MaxLength:                 v.getTitleMaxLength(),
		AllowUnlimitedRevertTitle: v.shouldAllowUnlimitedRevertTitle(),
	}

	commit := v.parsedCommitForPRTitle(title)

	if result := lengthRule.Validate(commit, title); result != nil {
		*allErrors = append(*allErrors, result.Message)
		*allErrors = append(*allErrors, result.Context...)

		return
	}

	// Check title format based on style (skip for revert titles)
	if !isRevertCommit(title) {
		for _, rule := range v.buildPRTitleFormatRules(ctx) {
			if result := rule.Validate(commit, title); result != nil {
				*allErrors = append(*allErrors, result.Message)
				*allErrors = append(*allErrors, result.Context...)

				return
			}
		}
	}
}

// validatePRBodyData validates the PR body
func (v *PRValidator) validatePRBodyData(
	body, prType string,
	bodyIsExpansion bool,
	allErrors, allWarnings *[]string,
) {
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

	// A bare variable body (e.g. gh pr create --body "$BODY") is an unresolved
	// expansion the hook cannot inspect, so skip rather than report it as missing
	// required sections. The value is omitted from the log since a "$(...)" form
	// can carry sensitive command content.
	if bodyIsExpansion {
		v.Logger().Debug("PR body is an unresolved expansion; skipping validation")

		return
	}

	requireChangelog := v.isRequireChangelog()
	bodyResult := validatePRBody(body, prType, requireChangelog)
	*allErrors = append(*allErrors, bodyResult.Errors...)
	*allWarnings = append(*allWarnings, bodyResult.Warnings...)
}

// bareShortVarPattern matches a bare short-form shell variable like "$TITLE".
var bareShortVarPattern = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*$`)

// isBarePRExpansion reports whether a regex-extracted PR field is a single
// unresolved shell expansion - "$NAME", "${NAME}", or a "$(...)" command
// substitution. The PR validator reads the title and body from the raw command
// text, so an expansion appears verbatim; its runtime value can't be inspected,
// and validating the literal token would wrongly reject a valid PR, so skip it.
func isBarePRExpansion(s string) bool {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "$(") && strings.HasSuffix(s, ")") {
		return true
	}

	// ${NAME} braced form (shared with the commit/branch validators).
	if isBareExpansion(s) {
		return true
	}

	// $NAME short form.
	return bareShortVarPattern.MatchString(s)
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
		primaryMsg := allErrors[0]

		var details strings.Builder

		// Skip first error in details (already in Message field)
		if len(allErrors) > 1 {
			for _, err := range allErrors[1:] {
				details.WriteString(err)
				details.WriteString("\n")
			}
		}

		if len(allWarnings) > 0 {
			details.WriteString("\nWarnings:\n")

			for _, warn := range allWarnings {
				details.WriteString(warn)
				details.WriteString("\n")
			}
		}

		result := validator.FailWithRef(
			validator.RefGitPRValidation,
			primaryMsg,
		)

		if details.Len() > 0 {
			result = result.AddDetail("errors", details.String())
		}

		result = result.AddDetail("commit_preview", "PR title: "+title)

		return result
	}

	if len(allWarnings) > 0 {
		var message strings.Builder

		message.WriteString("PR validation passed with warnings:\n\n")

		for _, warn := range allWarnings {
			message.WriteString(warn)
			message.WriteString("\n")
		}

		return validator.WarnWithRef(validator.RefGitPRValidation, message.String())
	}

	return validator.Pass()
}

// checkCILabelHeuristics suggests ci/ labels based on PR type and content
func (*PRValidator) checkCILabelHeuristics(data PRData, prType string) []string {
	warnings := []string{}

	shouldSkipTests := false
	shouldSkipE2E := false

	// Check PR type for non-logic changes
	if prType == commitTypeCI || prType == commitTypeDocs ||
		prType == commitTypeChore || prType == commitTypeStyle {
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
		warnings = append(
			warnings,
			"This appears to be a non-logic change - consider adding --label \"ci/skip-test\"",
			"Important: ci/* labels MUST be added during creation (--label flag)",
		)
	} else if shouldSkipE2E && !hasCILabel {
		warnings = append(
			warnings,
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
				fmt.Sprintf("Forbidden pattern found in PR title: '%s'", match),
				"Pattern: "+pattern,
			)
		}

		// Check body
		if body != "" && re.MatchString(body) {
			match := re.FindString(body)
			errors = append(errors,
				fmt.Sprintf("Forbidden pattern found in PR body: '%s'", match),
				"Pattern: "+pattern,
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

	return config.DefaultForbiddenPatterns
}

// checkAIAttribution rejects AI generation attribution anywhere in a gh pr
// create/edit command - the title, an inline body, a heredoc, or a --body-file.
// The whole command text is scanned rather than a parsed body so no flag
// spelling (--body, -b, --body-file, heredoc) can carry a footer past the check.
// It catches both plain phrasing ("Generated with Claude Code") and markdown
// footer links, while allow-listing legitimate references (CLAUDE.md, klaudiush,
// claude-hooks). Detection reuses containsAIAttribution so the PR and commit
// paths stay in sync, and it reports under GIT012 - the same code as
// commit-message attribution - so disabling the general PR-validation code
// (GIT023) does not silently disable it too.
func (v *PRValidator) checkAIAttribution(text string) *validator.Result {
	if !v.shouldBlockAIAttribution() {
		return nil
	}

	return aiAttributionResult(text, "PR")
}

// shouldBlockAIAttribution returns whether AI attribution should be blocked in
// the PR title and body.
func (v *PRValidator) shouldBlockAIAttribution() bool {
	if v.config != nil && v.config.BlockAIAttribution != nil {
		return *v.config.BlockAIAttribution
	}

	return true // default: enabled
}
