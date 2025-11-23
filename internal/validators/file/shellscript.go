package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

const (
	shellCheckTimeout = 10 * time.Second
)

// ShellScriptValidator validates shell scripts using shellcheck.
type ShellScriptValidator struct {
	validator.BaseValidator
	toolChecker execpkg.ToolChecker
	runner      execpkg.CommandRunner
}

// NewShellScriptValidator creates a new ShellScriptValidator.
func NewShellScriptValidator(log logger.Logger) *ShellScriptValidator {
	return &ShellScriptValidator{
		BaseValidator: *validator.NewBaseValidator("validate-shellscript", log),
		toolChecker:   execpkg.NewToolChecker(),
		runner:        execpkg.NewCommandRunner(shellCheckTimeout),
	}
}

// Validate validates shell scripts using shellcheck.
func (v *ShellScriptValidator) Validate(ctx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("validating shell script")

	// Check if shellcheck is available
	if !v.toolChecker.IsAvailable("shellcheck") {
		log.Debug("shellcheck not available, skipping validation")
		return validator.Pass()
	}

	// Get the file path
	filePath := ctx.GetFilePath()
	if filePath == "" {
		log.Debug("no file path provided")
		return validator.Pass()
	}

	// Skip Fish scripts
	if v.isFishScript(filePath, ctx.ToolInput.Content) {
		log.Debug("skipping Fish script", "file", filePath)
		return validator.Pass()
	}

	// Run shellcheck
	return v.runShellCheck(filePath, ctx.ToolInput.Content)
}

// isFishScript checks if the script is a Fish shell script.
func (v *ShellScriptValidator) isFishScript(filePath, content string) bool {
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

// runShellCheck runs shellcheck on the script.
func (v *ShellScriptValidator) runShellCheck(filePath, content string) *validator.Result {
	log := v.Logger()

	// If content is provided, create a temp file
	if content != "" {
		return v.runShellCheckOnContent(content)
	}

	// Otherwise, check if file exists
	if _, err := os.Stat(filePath); err != nil {
		log.Debug("file does not exist, skipping", "file", filePath)
		return validator.Pass()
	}

	return v.runShellCheckOnFile(filePath)
}

// runShellCheckOnContent runs shellcheck on content via stdin.
func (v *ShellScriptValidator) runShellCheckOnContent(content string) *validator.Result {
	log := v.Logger()

	ctx, cancel := context.WithTimeout(context.Background(), shellCheckTimeout)
	defer cancel()

	result, err := v.runner.RunWithStdin(ctx, strings.NewReader(content), "shellcheck", "-")
	if err == nil {
		log.Debug("shellcheck passed")
		return validator.Pass()
	}

	// Parse shellcheck output
	output := result.Stdout
	if output == "" {
		output = result.Stderr
	}

	log.Debug("shellcheck failed", "output", output)

	return validator.Fail(v.formatShellCheckOutput(output))
}

// runShellCheckOnFile runs shellcheck on a file.
func (v *ShellScriptValidator) runShellCheckOnFile(filePath string) *validator.Result {
	log := v.Logger()

	ctx, cancel := context.WithTimeout(context.Background(), shellCheckTimeout)
	defer cancel()

	result, err := v.runner.Run(ctx, "shellcheck", filePath)
	if err == nil {
		log.Debug("shellcheck passed", "file", filePath)
		return validator.Pass()
	}

	// Parse shellcheck output
	output := result.Stdout
	if output == "" {
		output = result.Stderr
	}

	log.Debug("shellcheck failed", "file", filePath, "output", output)

	return validator.Fail(v.formatShellCheckOutput(output))
}

// formatShellCheckOutput formats shellcheck output for display.
func (v *ShellScriptValidator) formatShellCheckOutput(output string) string {
	// Clean up the output - remove empty lines
	lines := strings.Split(output, "\n")
	var cleanLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return "Shellcheck validation failed\n\n" + strings.Join(cleanLines, "\n") + "\n\nFix these issues before committing."
}
