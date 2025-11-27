// Package file provides validators for file operations
package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
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
	config *config.MarkdownValidatorConfig
	linter linters.MarkdownLinter
}

// NewMarkdownValidator creates a new MarkdownValidator
func NewMarkdownValidator(
	cfg *config.MarkdownValidatorConfig,
	linter linters.MarkdownLinter,
	log logger.Logger,
) *MarkdownValidator {
	return &MarkdownValidator{
		BaseValidator: *validator.NewBaseValidator("validate-markdown", log),
		config:        cfg,
		linter:        linter,
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

// Validate checks Markdown formatting rules
func (v *MarkdownValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()

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

	result := v.linter.Lint(lintCtx, content, initialState)

	if !result.Success {
		message := "Markdown formatting errors"

		r := validator.FailWithRef(validator.RefMarkdownLint, message).
			AddDetail("errors", strings.TrimSpace(result.RawOut))

		// Include table suggestions if available
		if len(result.TableSuggested) > 0 {
			for lineNum, suggestion := range result.TableSuggested {
				r = r.AddDetail("suggested_table", formatTableSuggestion(lineNum, suggestion))

				break // Only include first suggestion in details for now
			}
		}

		return r
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

		// Determine if fragment reaches end of file
		fragmentLineCount := len(strings.Split(fragment, "\n"))
		totalLines := len(strings.Split(string(originalContent), "\n"))
		fragmentEndLine := fragmentStartLine + fragmentLineCount
		state.EndsAtEOF = fragmentEndLine >= totalLines

		log.Debug("fragment initial state",
			"start_line", fragmentStartLine,
			"in_code_block", state.InCodeBlock,
			"ends_at_eof", state.EndsAtEOF,
			"in_list", state.InList,
			"list_depth", state.ListItemDepth,
			"list_stack_len", len(state.ListStack),
			"last_heading_level", state.LastHeadingLevel,
		)

		log.Debug("validating edit fragment with context", "fragment_lines", fragmentLineCount)

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

// formatTableSuggestion formats a table suggestion for display in error details.
func formatTableSuggestion(lineNum int, suggestion string) string {
	return fmt.Sprintf("Line %d - Use this properly formatted table:\n\n%s", lineNum, suggestion)
}

// Category returns the validator category for parallel execution.
// MarkdownValidator uses CategoryIO because it invokes markdownlint.
func (*MarkdownValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
