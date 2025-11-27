package file

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/smykla-labs/klaudiush/internal/github"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	// defaultWorkflowTimeout is the timeout for actionlint commands
	defaultWorkflowTimeout = 10 * time.Second
	// defaultGHAPITimeout is the timeout for GitHub API calls
	defaultGHAPITimeout = 5 * time.Second
	// digestPreviewLength is the length of digest to show in error messages
	digestPreviewLength = 8
	// actionRefParts is the number of parts when splitting action@version
	actionRefParts = 2
	// outputParts is the number of parts when splitting actionlint output
	outputParts = 2
	// ownerRepoParts is the number of parts in owner/repo format
	ownerRepoParts = 2
)

var (
	// usesRegex matches "uses:" lines in workflows (with optional YAML list dash)
	usesRegex = regexp.MustCompile(`^[\s-]*uses:\s*(.+)$`)
	// yamlCommentRegex matches YAML comments
	yamlCommentRegex = regexp.MustCompile(`^\s*#`)
	// versionCommentRegex extracts version from comment (e.g., "# v1.2.3" or "# 1.2.3")
	versionCommentRegex = regexp.MustCompile(
		`#\s*(v?[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:[.-][a-zA-Z0-9]+)?)`,
	)
	// sha1Regex matches 40-character hex SHA-1
	sha1Regex = regexp.MustCompile(`^[a-f0-9]{40}$`)
	// sha256Regex matches 64-character hex SHA-256
	sha256Regex = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// actionUse represents a parsed GitHub Actions "uses" directive
type actionUse struct {
	LineNum       int
	ActionName    string
	Version       string
	IsDigest      bool
	InlineComment string
	PreviousLine  string
	FullLine      string
}

// WorkflowValidator validates GitHub Actions workflow and composable action files
type WorkflowValidator struct {
	validator.BaseValidator
	linter       linters.ActionLinter
	githubClient github.Client
	config       *config.WorkflowValidatorConfig
}

// NewWorkflowValidator creates a new WorkflowValidator
func NewWorkflowValidator(
	linter linters.ActionLinter,
	githubClient github.Client,
	log logger.Logger,
	cfg *config.WorkflowValidatorConfig,
) *WorkflowValidator {
	return &WorkflowValidator{
		BaseValidator: *validator.NewBaseValidator("validate-github-workflow", log),
		linter:        linter,
		githubClient:  githubClient,
		config:        cfg,
	}
}

// Validate checks GitHub Actions workflow and composable action files for digest pinning and runs actionlint
func (v *WorkflowValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()

	// Check if this is a workflow or composable action file
	filePath := hookCtx.GetFilePath()
	if !v.isWorkflowFile(filePath) {
		log.Debug("not a workflow or action file, skipping", "path", filePath)
		return validator.Pass()
	}

	content, err := v.getContent(hookCtx)
	if err != nil {
		log.Debug("skipping workflow/action validation", "error", err)
		return validator.Pass()
	}

	if content == "" {
		return validator.Pass()
	}

	var allErrors []string

	var allWarnings []string

	// Parse workflow and validate digest pinning if enabled
	if v.isEnforceDigestPinning() {
		actions := v.parseWorkflow(content)
		for _, action := range actions {
			errs, warns := v.validateAction(ctx, action)
			allErrors = append(allErrors, errs...)
			allWarnings = append(allWarnings, warns...)
		}
	}

	// Run actionlint if enabled and available
	if v.isUseActionlint() {
		actionlintWarnings := v.runActionlint(ctx, content, filePath)
		if len(actionlintWarnings) > 0 {
			allWarnings = append(allWarnings, actionlintWarnings...)
		}
	}

	// Report warnings
	if len(allWarnings) > 0 {
		log.Debug("workflow/action validation warnings", "count", len(allWarnings))

		for _, warn := range allWarnings {
			fmt.Fprintf(os.Stderr, "⚠️  %s\n", warn)
		}
	}

	// Report errors (blocking)
	if len(allErrors) > 0 {
		message := "GitHub Actions workflow/action validation failed"
		details := map[string]string{
			"file":   filepath.Base(filePath),
			"errors": strings.Join(allErrors, "\n"),
			"help": `Requirements:
  - Use digest-pinned actions with version comments:
    uses: actions/checkout@abc123... # v4.1.7

  - Or provide explanation when digest pinning not possible:
    # Cannot pin by digest: marketplace action with frequent updates
    uses: vendor/custom-action@v1`,
		}

		return validator.FailWithRef(
			validator.RefActionlint,
			message,
		).AddDetail("file", details["file"]).AddDetail("errors", details["errors"]).AddDetail("help", details["help"])
	}

	return validator.Pass()
}

// isWorkflowFile checks if the file path is a GitHub Actions workflow or composable action
func (*WorkflowValidator) isWorkflowFile(path string) bool {
	ext := filepath.Ext(path)
	if ext != ".yml" && ext != ".yaml" {
		return false
	}

	// Match .github/workflows/*.yml or .github/workflows/*.yaml
	if strings.Contains(path, ".github/workflows/") {
		return true
	}

	// Match .github/actions/*/action.yml or .github/actions/*/action.yaml
	if strings.Contains(path, ".github/actions/") {
		base := filepath.Base(path)
		return base == "action.yml" || base == "action.yaml"
	}

	return false
}

// getContent extracts workflow content from context
func (v *WorkflowValidator) getContent(ctx *hook.Context) (string, error) {
	log := v.Logger()

	// Try to get content from tool input (Write operation)
	if ctx.ToolInput.Content != "" {
		return ctx.ToolInput.Content, nil
	}

	// For Edit operations in PreToolUse, read file and apply edit
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		filePath := ctx.GetFilePath()
		if filePath == "" {
			return "", errNoContent
		}

		// Read original file content
		//nolint:gosec // filePath is from Claude Code tool context, not user input
		originalContent, err := os.ReadFile(filePath)
		if err != nil {
			log.Debug("failed to read file for edit validation", "file", filePath, "error", err)
			return "", err
		}

		// Apply the edit (replace old_string with new_string)
		oldStr := ctx.ToolInput.OldString
		newStr := ctx.ToolInput.NewString

		if oldStr == "" {
			log.Debug("no old_string in edit operation, cannot validate")
			return "", errNoContent
		}

		// Replace first occurrence (Edit tool replaces first match)
		editedContent := strings.Replace(string(originalContent), oldStr, newStr, 1)

		return editedContent, nil
	}

	// Try to get from file path (Edit or PostToolUse)
	filePath := ctx.GetFilePath()
	if filePath != "" && ctx.EventType == hook.EventTypePostToolUse {
		// In PostToolUse, we could read the file
		//nolint:gosec // filePath is from Claude Code tool context, not user input
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("reading file: %w", err)
		}

		return string(content), nil
	}

	return "", errNoContent
}

// parseWorkflow parses workflow content and extracts all action uses
func (*WorkflowValidator) parseWorkflow(content string) []actionUse {
	var actions []actionUse

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	prevLine := ""

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check for 'uses:' lines
		if matches := usesRegex.FindStringSubmatch(line); len(matches) > 1 {
			actionRef := matches[1]

			// Extract inline comment if present
			inlineComment := ""
			if idx := strings.Index(actionRef, "#"); idx != -1 {
				inlineComment = strings.TrimSpace(actionRef[idx:])
				actionRef = strings.TrimSpace(actionRef[:idx])
			}

			// Skip local actions (start with ./)
			if strings.HasPrefix(actionRef, "./") {
				prevLine = line
				continue
			}

			// Skip Docker actions (contain docker://)
			if strings.HasPrefix(actionRef, "docker://") {
				prevLine = line
				continue
			}

			// Extract action name and version
			if parts := strings.SplitN(actionRef, "@", actionRefParts); len(
				parts,
			) == actionRefParts {
				actionName := parts[0]
				version := parts[1]

				// Check if version is a digest
				isDigest := sha1Regex.MatchString(version) || sha256Regex.MatchString(version)

				actions = append(actions, actionUse{
					LineNum:       lineNum,
					ActionName:    actionName,
					Version:       version,
					IsDigest:      isDigest,
					InlineComment: inlineComment,
					PreviousLine:  prevLine,
					FullLine:      line,
				})
			}
		}

		prevLine = line
	}

	return actions
}

// validateAction validates a single action use
func (v *WorkflowValidator) validateAction(
	ctx context.Context,
	action actionUse,
) ([]string, []string) {
	if action.IsDigest {
		return v.validateDigestAction(ctx, action)
	}

	return v.validateTagAction(action)
}

// validateDigestAction validates digest-pinned actions
func (v *WorkflowValidator) validateDigestAction(
	ctx context.Context,
	action actionUse,
) ([]string, []string) {
	var errs []string

	var warnings []string

	versionComment := v.extractVersionComment(action)

	// Check version comment if required
	if v.isRequireVersionComment() && versionComment == "" {
		digestPreview := action.Version
		if len(digestPreview) > digestPreviewLength {
			digestPreview = digestPreview[:digestPreviewLength] + "..."
		}

		errs = append(errs, fmt.Sprintf(
			"Line %d: Digest-pinned action '%s@%s' missing version comment",
			action.LineNum, action.ActionName, digestPreview,
		))

		return errs, warnings
	}

	// Check if using latest version if enabled
	if v.isCheckLatestVersion() && versionComment != "" {
		latestVersion := v.getLatestVersion(ctx, action.ActionName)
		if latestVersion != "" && !v.isVersionLatest(versionComment, latestVersion) {
			warnings = append(warnings, fmt.Sprintf(
				"Line %d: Action '%s' using %s, latest is %s",
				action.LineNum, action.ActionName, versionComment, latestVersion,
			))
		}
	}

	return errs, warnings
}

// validateTagAction validates tag-pinned actions
func (v *WorkflowValidator) validateTagAction(action actionUse) ([]string, []string) {
	var errs []string

	if !v.hasExplanationComment(action) {
		errs = append(errs, fmt.Sprintf(
			"Line %d: Action '%s@%s' uses tag without digest",
			action.LineNum, action.ActionName, action.Version,
		))
	}

	return errs, nil
}

// extractVersionComment extracts version comment from inline or previous line
func (*WorkflowValidator) extractVersionComment(action actionUse) string {
	// Check inline comment first
	if matches := versionCommentRegex.FindStringSubmatch(action.InlineComment); len(matches) > 1 {
		return matches[1]
	}

	// Check previous line
	if matches := versionCommentRegex.FindStringSubmatch(action.PreviousLine); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// hasExplanationComment checks if action has an explanation comment
func (*WorkflowValidator) hasExplanationComment(action actionUse) bool {
	// Check previous line for comment
	if yamlCommentRegex.MatchString(action.PreviousLine) {
		// It's a comment, but is it a version comment?
		if matches := versionCommentRegex.FindStringSubmatch(action.PreviousLine); len(
			matches,
		) == 0 {
			// Not a version comment, so it's an explanation
			return true
		}
	}

	// Check inline comment
	if action.InlineComment != "" {
		// Check if it's not a version comment
		if matches := versionCommentRegex.FindStringSubmatch(action.InlineComment); len(
			matches,
		) == 0 {
			// Has alphabetic characters (not just version)
			if strings.ContainsAny(
				action.InlineComment,
				"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
			) {
				return true
			}
		}
	}

	return false
}

// getLatestVersion queries GitHub API for the latest version of an action
func (v *WorkflowValidator) getLatestVersion(ctx context.Context, actionName string) string {
	// Parse action name (format: owner/repo)
	parts := strings.SplitN(actionName, "/", ownerRepoParts)
	if len(parts) != ownerRepoParts {
		v.Logger().Debug("invalid action name format", "action", actionName)
		return ""
	}

	owner, repo := parts[0], parts[1]

	apiCtx, cancel := context.WithTimeout(ctx, v.getGHAPITimeout())
	defer cancel()

	// Try releases first
	release, err := v.githubClient.GetLatestRelease(apiCtx, owner, repo)
	if err == nil && release.TagName != "" {
		return release.TagName
	}

	if err != nil &&
		!errors.Is(err, github.ErrNoReleases) &&
		!errors.Is(err, github.ErrRepositoryNotFound) {
		v.Logger().Debug("failed to get latest release", "action", actionName, "error", err)
	}

	// Fallback to tags
	apiCtx, cancel = context.WithTimeout(ctx, v.getGHAPITimeout())
	defer cancel()

	tags, err := v.githubClient.GetTags(apiCtx, owner, repo)
	if err != nil {
		if !errors.Is(err, github.ErrNoTags) && !errors.Is(err, github.ErrRepositoryNotFound) {
			v.Logger().Debug("failed to get tags", "action", actionName, "error", err)
		}

		return ""
	}

	if len(tags) > 0 {
		return tags[0].Name
	}

	return ""
}

// isVersionLatest checks if current version is >= latest version
func (*WorkflowValidator) isVersionLatest(current, latest string) bool {
	currentVer, err := semver.NewVersion(current)
	if err != nil {
		// Fall back to string comparison if not valid semver
		return strings.TrimPrefix(current, "v") >= strings.TrimPrefix(latest, "v")
	}

	latestVer, err := semver.NewVersion(latest)
	if err != nil {
		// Fall back to string comparison if not valid semver
		return strings.TrimPrefix(current, "v") >= strings.TrimPrefix(latest, "v")
	}

	return currentVer.Compare(latestVer) >= 0
}

// runActionlint runs actionlint on the workflow content using ActionLinter
func (v *WorkflowValidator) runActionlint(
	ctx context.Context,
	content, originalPath string,
) []string {
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	result := v.linter.Lint(lintCtx, content, originalPath)

	if result.Success {
		return nil
	}

	output := strings.TrimSpace(result.RawOut)
	if output != "" {
		return v.parseActionlintOutput(output)
	}

	if result.Err != nil {
		v.Logger().Debug("actionlint failed", "error", result.Err)
	}

	return nil
}

// parseActionlintOutput parses actionlint output into individual warnings
func (*WorkflowValidator) parseActionlintOutput(output string) []string {
	lines := strings.Split(output, "\n")
	warnings := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: file:line:col: message
		// Remove the temp file path and just show the message
		if parts := strings.SplitN(line, ": ", outputParts); len(parts) == outputParts {
			warnings = append(warnings, parts[1])
		} else {
			warnings = append(warnings, line)
		}
	}

	return warnings
}

// getTimeout returns the configured timeout for actionlint operations.
func (v *WorkflowValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultWorkflowTimeout
}

// getGHAPITimeout returns the configured timeout for GitHub API calls.
func (v *WorkflowValidator) getGHAPITimeout() time.Duration {
	if v.config != nil && v.config.GHAPITimeout.ToDuration() > 0 {
		return v.config.GHAPITimeout.ToDuration()
	}

	return defaultGHAPITimeout
}

// isEnforceDigestPinning returns whether digest pinning enforcement is enabled.
func (v *WorkflowValidator) isEnforceDigestPinning() bool {
	if v.config != nil && v.config.EnforceDigestPinning != nil {
		return *v.config.EnforceDigestPinning
	}

	return true
}

// isRequireVersionComment returns whether version comments are required for digest-pinned actions.
func (v *WorkflowValidator) isRequireVersionComment() bool {
	if v.config != nil && v.config.RequireVersionComment != nil {
		return *v.config.RequireVersionComment
	}

	return true
}

// isCheckLatestVersion returns whether latest version checking is enabled.
func (v *WorkflowValidator) isCheckLatestVersion() bool {
	if v.config != nil && v.config.CheckLatestVersion != nil {
		return *v.config.CheckLatestVersion
	}

	return true
}

// isUseActionlint returns whether actionlint integration is enabled.
func (v *WorkflowValidator) isUseActionlint() bool {
	if v.config != nil && v.config.UseActionlint != nil {
		return *v.config.UseActionlint
	}

	return true
}

// Category returns the validator category for parallel execution.
// WorkflowValidator uses CategoryIO because it invokes actionlint and GitHub API.
func (*WorkflowValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
