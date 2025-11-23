package linters

import (
	"context"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
)

// ActionLinter validates GitHub Actions workflow files using actionlint
type ActionLinter interface {
	Lint(ctx context.Context, content string, filePath string) *LintResult
}

// RealActionLinter implements ActionLinter using the actionlint CLI tool
type RealActionLinter struct {
	runner      execpkg.CommandRunner
	toolChecker execpkg.ToolChecker
	tempManager execpkg.TempFileManager
}

// NewActionLinter creates a new RealActionLinter
func NewActionLinter(runner execpkg.CommandRunner) *RealActionLinter {
	return &RealActionLinter{
		runner:      runner,
		toolChecker: execpkg.NewToolChecker(),
		tempManager: execpkg.NewTempFileManager(),
	}
}

// Lint validates GitHub Actions workflow file content
func (a *RealActionLinter) Lint(ctx context.Context, content string, _ string) *LintResult {
	// Check if actionlint is available
	if !a.toolChecker.IsAvailable("actionlint") {
		return &LintResult{
			Success: true,
			Err:     nil,
		}
	}

	// Create temp file for validation
	tmpFile, cleanup, err := a.tempManager.Create("workflow-*.yml", content)
	if err != nil {
		return &LintResult{
			Success: false,
			Err:     err,
		}
	}
	defer cleanup()

	// Run actionlint
	result := a.runner.Run(ctx, "actionlint", "-no-color", tmpFile)

	return &LintResult{
		Success:  result.Err == nil,
		RawOut:   result.Stdout + result.Stderr,
		Findings: []LintFinding{}, // TODO: Parse actionlint output
		Err:      result.Err,
	}
}
