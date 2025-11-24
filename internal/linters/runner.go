package linters

import (
	"context"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// OutputParser is a function that parses command output into LintFindings
type OutputParser func(output string) []LintFinding

// ContentLinter provides common functionality for content-based linters
type ContentLinter struct {
	runner      execpkg.CommandRunner
	toolChecker execpkg.ToolChecker
	tempManager execpkg.TempFileManager
}

// NewContentLinter creates a new ContentLinter
func NewContentLinter(runner execpkg.CommandRunner) *ContentLinter {
	return &ContentLinter{
		runner:      runner,
		toolChecker: execpkg.NewToolChecker(),
		tempManager: execpkg.NewTempFileManager(),
	}
}

// LintContent validates content using a CLI tool
// toolName: the command to run
// tempPattern: pattern for temp file (e.g., "script-*.sh")
// content: the content to validate
// parser: function to parse the output into findings
// args: additional arguments for the tool (temp file path is appended)
func (l *ContentLinter) LintContent(
	ctx context.Context,
	toolName string,
	tempPattern string,
	content string,
	parser OutputParser,
	args ...string,
) *LintResult {
	// Check if tool is available
	if !l.toolChecker.IsAvailable(toolName) {
		return &LintResult{
			Success: true,
			Err:     nil,
		}
	}

	// Create temp file for validation
	tmpFile, cleanup, err := l.tempManager.Create(tempPattern, content)
	if err != nil {
		return &LintResult{
			Success: false,
			Err:     err,
		}
	}
	defer cleanup()

	// Build full args with temp file path
	fullArgs := make([]string, len(args)+1)
	copy(fullArgs, args)
	fullArgs[len(args)] = tmpFile

	// Run tool
	result := l.runner.Run(ctx, toolName, fullArgs...)

	rawOut := result.Stdout + result.Stderr
	findings := parser(result.Stdout)

	return &LintResult{
		Success:  result.Err == nil,
		RawOut:   rawOut,
		Findings: findings,
		Err:      result.Err,
	}
}
