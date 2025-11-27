package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	defaultShellCheckTimeout = 10 * time.Second

	// defaultShellContextLines is the number of lines before/after an edit to include for validation
	defaultShellContextLines = 2
)

// ShellScriptValidator validates shell scripts using shellcheck.
type ShellScriptValidator struct {
	validator.BaseValidator
	checker linters.ShellChecker
	config  *config.ShellScriptValidatorConfig
}

// NewShellScriptValidator creates a new ShellScriptValidator.
func NewShellScriptValidator(
	log logger.Logger,
	checker linters.ShellChecker,
	cfg *config.ShellScriptValidatorConfig,
) *ShellScriptValidator {
	return &ShellScriptValidator{
		BaseValidator: *validator.NewBaseValidator("validate-shellscript", log),
		checker:       checker,
		config:        cfg,
	}
}

// Validate validates shell scripts using shellcheck.
func (v *ShellScriptValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating shell script")

	// Get the file path
	filePath := hookCtx.GetFilePath()
	if filePath == "" {
		log.Debug("no file path provided")
		return validator.Pass()
	}

	// Get content based on operation type
	content, err := v.getContent(hookCtx, filePath)
	if err != nil {
		log.Debug("failed to get content", "error", err)
		return validator.Pass()
	}

	// Skip Fish scripts
	if v.isFishScript(filePath, content) {
		log.Debug("skipping Fish script", "file", filePath)
		return validator.Pass()
	}

	// Run shellcheck using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	result := v.checker.Check(lintCtx, content)
	if result.Success {
		log.Debug("shellcheck passed")
		return validator.Pass()
	}

	log.Debug("shellcheck failed", "output", result.RawOut)

	return validator.FailWithRef(validator.RefShellcheck, v.formatShellCheckOutput(result.RawOut))
}

// getContent extracts shell script content from context
func (v *ShellScriptValidator) getContent(ctx *hook.Context, filePath string) (string, error) {
	log := v.Logger()

	// For Edit operations, validate only the changed fragment with context
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		return v.getEditContent(ctx, filePath)
	}

	// Get content from context or read from file (Write operation)
	content := ctx.ToolInput.Content
	if content != "" {
		return content, nil
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		log.Debug("file does not exist, skipping", "file", filePath)
		return "", err
	}

	// Read file content
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from Claude Code context
	if err != nil {
		log.Debug("failed to read file", "file", filePath, "error", err)
		return "", err
	}

	return string(data), nil
}

// getEditContent extracts content for Edit operations with context
func (v *ShellScriptValidator) getEditContent(
	ctx *hook.Context,
	filePath string,
) (string, error) {
	log := v.Logger()

	oldStr := ctx.ToolInput.OldString
	newStr := ctx.ToolInput.NewString

	if oldStr == "" || newStr == "" {
		log.Debug("missing old_string or new_string in edit operation")
		return "", os.ErrNotExist
	}

	// Read original file to extract context around the edit
	//nolint:gosec // filePath is from Claude Code tool context, not user input
	originalContent, err := os.ReadFile(filePath)
	if err != nil {
		log.Debug("failed to read file for edit validation", "file", filePath, "error", err)
		return "", err
	}

	// Extract fragment with context lines around the edit
	fragment := ExtractEditFragment(
		string(originalContent),
		oldStr,
		newStr,
		v.getContextLines(),
		log,
	)
	if fragment == "" {
		log.Debug("could not extract edit fragment, skipping validation")
		return "", os.ErrNotExist
	}

	fragmentLineCount := len(strings.Split(fragment, "\n"))
	log.Debug("validating edit fragment with context", "fragment_lines", fragmentLineCount)

	return fragment, nil
}

// isFishScript checks if the script is a Fish shell script.
func (*ShellScriptValidator) isFishScript(filePath, content string) bool {
	// Check file extension
	if filepath.Ext(filePath) == ".fish" {
		return true
	}

	// Check shebang
	if strings.HasPrefix(content, "#!/usr/bin/env fish") ||
		strings.HasPrefix(content, "#!/usr/bin/fish") ||
		strings.HasPrefix(content, "#!/bin/fish") {
		return true
	}

	return false
}

// formatShellCheckOutput formats shellcheck output for display.
func (*ShellScriptValidator) formatShellCheckOutput(output string) string {
	// Clean up the output - remove empty lines
	lines := strings.Split(output, "\n")

	var cleanLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return "Shellcheck validation failed\n\n" + strings.Join(
		cleanLines,
		"\n",
	) + "\n\nFix these issues before committing."
}

// getTimeout returns the configured timeout for shellcheck operations.
func (v *ShellScriptValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultShellCheckTimeout
}

// getContextLines returns the configured number of context lines for edit validation.
func (v *ShellScriptValidator) getContextLines() int {
	if v.config != nil && v.config.ContextLines != nil {
		return *v.config.ContextLines
	}

	return defaultShellContextLines
}

// Category returns the validator category for parallel execution.
// ShellScriptValidator uses CategoryIO because it invokes shellcheck.
func (*ShellScriptValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
