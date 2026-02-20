// Package file provides validators for file operations
package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const (
	// defaultMarkdownTimeout is the default timeout for markdown linting
	defaultMarkdownTimeout = 10 * time.Second

	// defaultContextLines is the default number of lines before/after an edit to include for validation
	defaultContextLines = 2
)

var (
	errFileValidationNotImpl = errors.New("file-based validation not implemented")
	errNoContent             = errors.New("no content found")
)

// MarkdownValidator validates Markdown formatting rules
type MarkdownValidator struct {
	validator.BaseValidator
	config      *config.MarkdownValidatorConfig
	linter      linters.MarkdownLinter
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewMarkdownValidator creates a new MarkdownValidator
func NewMarkdownValidator(
	cfg *config.MarkdownValidatorConfig,
	linter linters.MarkdownLinter,
	log logger.Logger,
	ruleAdapter *rules.RuleValidatorAdapter,
) *MarkdownValidator {
	return &MarkdownValidator{
		BaseValidator: *validator.NewBaseValidator("validate-markdown", log),
		config:        cfg,
		linter:        linter,
		ruleAdapter:   ruleAdapter,
	}
}

// getTimeout returns the timeout for markdown linting operations
func (v *MarkdownValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultMarkdownTimeout
}

// getContextLines returns the number of lines before/after an edit to include
func (v *MarkdownValidator) getContextLines() int {
	if v.config != nil && v.config.ContextLines != nil {
		return *v.config.ContextLines
	}

	return defaultContextLines
}

// isUseMarkdownlint returns whether markdownlint integration is enabled
func (v *MarkdownValidator) isUseMarkdownlint() bool {
	if v.config != nil && v.config.UseMarkdownlint != nil {
		return *v.config.UseMarkdownlint
	}

	return true // default: enabled
}

// isSkipPlanDocuments returns whether plan document skipping is enabled
func (v *MarkdownValidator) isSkipPlanDocuments() bool {
	if v.config != nil && v.config.SkipPlanDocuments != nil {
		return *v.config.SkipPlanDocuments
	}

	return true // default: skip plan documents
}

// Validate checks Markdown formatting rules
func (v *MarkdownValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Skip validation for Claude Code plan documents
	if v.isSkipPlanDocuments() && strings.Contains(hookCtx.GetFilePath(), ".claude/plans/") {
		log.Debug("skipping markdown validation for plan document", "path", hookCtx.GetFilePath())
		return validator.Pass()
	}

	// Skip if markdownlint is disabled
	if !v.isUseMarkdownlint() {
		log.Debug("markdownlint is disabled, skipping validation")
		return validator.Pass()
	}

	content, initialState, err := v.getContentWithState(hookCtx)
	if err != nil {
		log.Debug("skipping markdown validation", "error", err)
		return validator.Pass()
	}

	if content == "" {
		return validator.Pass()
	}

	timeout := v.getTimeout()

	lintCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get the file path for error reporting
	filePath := hookCtx.GetFilePath()
	displayPath := getDisplayPath(filePath)

	result := v.linter.LintWithPath(lintCtx, content, initialState, displayPath)

	if !result.Success {
		return v.buildBlockingResult(result)
	}

	// No blocking errors - check for cosmetic table warnings
	if len(result.CosmeticTableWarnings) > 0 {
		return v.buildCosmeticResult(result)
	}

	return validator.Pass()
}

// getContentWithState extracts markdown content and detects initial state from context
func (v *MarkdownValidator) getContentWithState(
	ctx *hook.Context,
) (string, *validators.MarkdownState, error) {
	log := v.Logger()

	// Try to get content from tool input (Write operation)
	if ctx.ToolInput.Content != "" {
		return ctx.ToolInput.Content, nil, nil
	}

	// For Edit operations in PreToolUse, validate only the changed fragment with context
	// to avoid forcing users to fix all existing linting issues
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		filePath := ctx.GetFilePath()
		if filePath == "" {
			return "", nil, errNoContent
		}

		oldStr := ctx.ToolInput.OldString
		newStr := ctx.ToolInput.NewString

		if oldStr == "" || newStr == "" {
			log.Debug("missing old_string or new_string in edit operation")
			return "", nil, errNoContent
		}

		// Read original file to extract context around the edit
		//nolint:gosec // filePath is from Claude Code tool context, not user input
		originalContent, err := os.ReadFile(filePath)
		if err != nil {
			log.Debug("failed to read file for edit validation", "file", filePath, "error", err)
			return "", nil, err
		}

		// Extract fragment with context lines around the edit
		contextLines := v.getContextLines()

		fragment := ExtractEditFragment(
			string(originalContent),
			oldStr,
			newStr,
			contextLines,
			log,
		)
		if fragment == "" {
			log.Debug("could not extract edit fragment, skipping validation")
			return "", nil, errNoContent
		}

		// Detect markdown state at fragment start
		fragmentStartLine := getFragmentStartLine(string(originalContent), oldStr, contextLines)
		state := validators.DetectMarkdownState(string(originalContent), fragmentStartLine)
		state.StartLine = fragmentStartLine

		// Determine if the NEW content (new_string) reaches end of file.
		// We check if old_string is at or near EOF in the original content.
		// If old_string ends at EOF, then new_string (which replaces it) also ends at EOF.
		state.EndsAtEOF = EditReachesEOF(string(originalContent), oldStr)

		log.Debug("fragment initial state",
			"start_line", fragmentStartLine,
			"in_code_block", state.InCodeBlock,
			"ends_at_eof", state.EndsAtEOF,
			"in_list", state.InList,
			"list_depth", state.ListItemDepth,
			"list_stack_len", len(state.ListStack),
			"last_heading_level", state.LastHeadingLevel,
		)

		log.Debug("validating edit fragment with context")

		return fragment, &state, nil
	}

	// Try to get from file path (Edit or PostToolUse)
	filePath := ctx.GetFilePath()
	if filePath != "" {
		// In PostToolUse, we could read the file, but for now skip
		// as the Bash version doesn't handle this case well either
		return "", nil, errFileValidationNotImpl
	}

	return "", nil, errNoContent
}

// buildBlockingResult creates a blocking (FailWithRef) result from lint output.
func (*MarkdownValidator) buildBlockingResult(result *linters.LintResult) *validator.Result {
	message := buildSpecificMessage(result.RawOut)

	r := validator.FailWithRef(validator.RefMarkdownLint, message).
		AddDetail("errors", strings.TrimSpace(result.RawOut))

	// Include table suggestions from structural issues
	attachFirstSuggestion(r, result.TableSuggested)

	// Also attach cosmetic suggestions if present
	attachFirstSuggestion(r, result.CosmeticTableSuggested)

	return r
}

// buildCosmeticResult creates a result for cosmetic-only table warnings.
// Returns a blocking or warning result depending on config.
func (v *MarkdownValidator) buildCosmeticResult(result *linters.LintResult) *validator.Result {
	errText := strings.Join(result.CosmeticTableWarnings, "\n")
	message := buildSpecificMessage(errText)

	var r *validator.Result

	if v.isTableFormattingSeverityError() {
		r = validator.FailWithRef(validator.RefMarkdownLint, message)
	} else {
		r = validator.WarnWithRef(validator.RefMarkdownLint, message)
	}

	r = r.AddDetail("errors", errText)
	attachFirstSuggestion(r, result.CosmeticTableSuggested)

	return r
}

// attachFirstSuggestion adds the first table suggestion from the map to the result.
func attachFirstSuggestion(r *validator.Result, suggestions map[int]string) {
	for lineNum, suggestion := range suggestions {
		r.AddDetail("suggested_table", formatTableSuggestion(lineNum, suggestion))

		break // Only include first suggestion
	}
}

// isTableFormattingSeverityError returns true if cosmetic table issues should block.
func (v *MarkdownValidator) isTableFormattingSeverityError() bool {
	if v.config != nil && v.config.TableFormattingSeverity == "error" {
		return true
	}

	return false // default: "warning" (non-blocking)
}

// buildSpecificMessage extracts a specific, actionable message from lint output.
// Instead of generic "Markdown formatting errors", returns the first warning.
func buildSpecificMessage(rawOut string) string {
	if rawOut == "" {
		return "Markdown formatting errors"
	}

	// Use the first non-empty line as the message
	for line := range strings.SplitSeq(rawOut, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	return "Markdown formatting errors"
}

// formatTableSuggestion formats a table suggestion for display in error details.
func formatTableSuggestion(lineNum int, suggestion string) string {
	return fmt.Sprintf("Line %d - Use this properly formatted table:\n\n%s", lineNum, suggestion)
}

// getDisplayPath converts an absolute file path to a relative path for display.
// Returns "<content>" if the path is empty, or the relative path if it can be computed,
// otherwise returns the original path.
func getDisplayPath(filePath string) string {
	if filePath == "" {
		return "<content>"
	}

	cwd, err := os.Getwd()
	if err != nil {
		return filePath
	}

	relPath, err := filepath.Rel(cwd, filePath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return filePath
	}

	return relPath
}

// Category returns the validator category for parallel execution.
// MarkdownValidator uses CategoryIO because it invokes markdownlint.
func (*MarkdownValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
