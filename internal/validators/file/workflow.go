package file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

const (
	// workflowTimeout is the timeout for actionlint commands
	workflowTimeout = 10 * time.Second
	// ghAPITimeout is the timeout for GitHub API calls
	ghAPITimeout = 5 * time.Second
	// digestPreviewLength is the length of digest to show in error messages
	digestPreviewLength = 8
	// actionRefParts is the number of parts when splitting action@version
	actionRefParts = 2
	// outputParts is the number of parts when splitting actionlint output
	outputParts = 2
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

// WorkflowValidator validates GitHub Actions workflow files
type WorkflowValidator struct {
	validator.BaseValidator
	toolChecker execpkg.ToolChecker
	runner      execpkg.CommandRunner
	tempManager execpkg.TempFileManager
}

// NewWorkflowValidator creates a new WorkflowValidator
func NewWorkflowValidator(log logger.Logger) *WorkflowValidator {
	return &WorkflowValidator{
		BaseValidator: *validator.NewBaseValidator("validate-github-workflow", log),
		toolChecker:   execpkg.NewToolChecker(),
		runner:        execpkg.NewCommandRunner(workflowTimeout),
		tempManager:   execpkg.NewTempFileManager(),
	}
}

// Validate checks GitHub Actions workflow file for digest pinning and runs actionlint
func (v *WorkflowValidator) Validate(ctx *hook.Context) *validator.Result {
	log := v.Logger()

	// Check if this is a workflow file
	filePath := ctx.GetFilePath()
	if !v.isWorkflowFile(filePath) {
		log.Debug("not a workflow file, skipping", "path", filePath)
		return validator.Pass()
	}

	content, err := v.getContent(ctx)
	if err != nil {
		log.Debug("skipping workflow validation", "error", err)
		return validator.Pass()
	}

	if content == "" {
		return validator.Pass()
	}

	var allErrors []string

	var allWarnings []string

	// Parse workflow and validate digest pinning
	actions := v.parseWorkflow(content)
	for _, action := range actions {
		errs, warns := v.validateAction(action)
		allErrors = append(allErrors, errs...)
		allWarnings = append(allWarnings, warns...)
	}

	// Run actionlint if available
	if actionlintWarnings := v.runActionlint(content, filePath); len(actionlintWarnings) > 0 {
		allWarnings = append(allWarnings, actionlintWarnings...)
	}

	// Report warnings
	if len(allWarnings) > 0 {
		log.Debug("workflow validation warnings", "count", len(allWarnings))

		for _, warn := range allWarnings {
			fmt.Fprintf(os.Stderr, "⚠️  %s\n", warn)
		}
	}

	// Report errors (blocking)
	if len(allErrors) > 0 {
		message := "GitHub Actions workflow validation failed"
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

		return validator.FailWithDetails(message, details)
	}

	return validator.Pass()
}

// isWorkflowFile checks if the file path is a GitHub Actions workflow
func (v *WorkflowValidator) isWorkflowFile(path string) bool {
	// Match .github/workflows/*.yml or .github/workflows/*.yaml
	if !strings.Contains(path, ".github/workflows/") {
		return false
	}

	ext := filepath.Ext(path)

	return ext == ".yml" || ext == ".yaml"
}

// getContent extracts workflow content from context
//
//nolint:dupl // Same pattern used across validators, extraction would add complexity
func (v *WorkflowValidator) getContent(ctx *hook.Context) (string, error) {
	log := v.Logger()

	// Try to get content from tool input (Write operation)
	if ctx.ToolInput.Content != "" {
		return ctx.ToolInput.Content, nil
	}

	// For Edit operations in PreToolUse, read file and apply edit
	if ctx.EventType == hook.PreToolUse && ctx.ToolName == hook.Edit {
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
	if filePath != "" && ctx.EventType == hook.PostToolUse {
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
func (v *WorkflowValidator) parseWorkflow(content string) []actionUse {
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
func (v *WorkflowValidator) validateAction(action actionUse) ([]string, []string) {
	if action.IsDigest {
		return v.validateDigestAction(action)
	}

	return v.validateTagAction(action)
}

// validateDigestAction validates digest-pinned actions
func (v *WorkflowValidator) validateDigestAction(action actionUse) ([]string, []string) {
	var errs []string

	var warnings []string

	versionComment := v.extractVersionComment(action)
	if versionComment == "" {
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

	// Check if using latest version
	latestVersion := v.getLatestVersion(action.ActionName)
	if latestVersion != "" && !v.isVersionLatest(versionComment, latestVersion) {
		warnings = append(warnings, fmt.Sprintf(
			"Line %d: Action '%s' using %s, latest is %s",
			action.LineNum, action.ActionName, versionComment, latestVersion,
		))
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
func (v *WorkflowValidator) extractVersionComment(action actionUse) string {
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
func (v *WorkflowValidator) hasExplanationComment(action actionUse) bool {
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
func (v *WorkflowValidator) getLatestVersion(actionName string) string {
	// Check if gh CLI is available
	if !v.toolChecker.IsAvailable("gh") {
		v.Logger().Debug("gh CLI not found, skipping version check")
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), ghAPITimeout)
	defer cancel()

	// Try releases first
	result := v.runner.Run(
		ctx,
		"gh",
		"api",
		fmt.Sprintf("repos/%s/releases/latest", actionName),
		"--jq",
		".tag_name",
	)
	if result.Err == nil {
		version := strings.TrimSpace(result.Stdout)
		if version != "" {
			return version
		}
	}

	// Fallback to tags
	ctx, cancel = context.WithTimeout(context.Background(), ghAPITimeout)
	defer cancel()

	result = v.runner.Run(ctx, "gh", "api", fmt.Sprintf("repos/%s/tags", actionName))
	if result.Err != nil {
		return ""
	}

	// Parse JSON response
	var tags []struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &tags); err != nil {
		return ""
	}

	if len(tags) > 0 {
		return tags[0].Name
	}

	return ""
}

// isVersionLatest checks if current version is >= latest version
func (v *WorkflowValidator) isVersionLatest(current, latest string) bool {
	// Normalize versions (strip leading 'v')
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Simple string comparison for now
	// TODO: Could use proper semver comparison library
	return current >= latest
}

// runActionlint runs actionlint on the workflow content
func (v *WorkflowValidator) runActionlint(content, originalPath string) []string {
	// Check if actionlint is available
	if !v.toolChecker.IsAvailable("actionlint") {
		v.Logger().Debug("actionlint not found in PATH, skipping")
		return nil
	}

	// Create temp file for validation
	ext := filepath.Ext(originalPath)
	if ext == "" {
		ext = ".yml"
	}

	tmpFile, cleanup, err := v.tempManager.Create("workflow-*"+ext, content)
	if err != nil {
		v.Logger().Debug("failed to create temp file for actionlint", "error", err)
		return nil
	}

	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), workflowTimeout)
	defer cancel()

	result := v.runner.Run(ctx, "actionlint", "-no-color", tmpFile)
	output := strings.TrimSpace(result.Stdout)

	if result.Err != nil {
		// actionlint returns non-zero on findings
		if output != "" {
			// Parse output into individual warnings
			return v.parseActionlintOutput(output)
		}

		// Check if it's a real error (not just findings)
		errOutput := strings.TrimSpace(result.Stderr)
		if errOutput != "" {
			v.Logger().Debug("actionlint failed", "error", result.Err, "stderr", errOutput)
			return nil
		}

		return nil
	}

	// No findings
	return nil
}

// parseActionlintOutput parses actionlint output into individual warnings
func (v *WorkflowValidator) parseActionlintOutput(output string) []string {
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
