package linters

import (
	"context"
	"errors"
	"strings"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
	"github.com/smykla-labs/claude-hooks/internal/validators"
)

// ErrMarkdownCustomRules indicates custom markdown rules found issues
var ErrMarkdownCustomRules = errors.New("custom markdown rules validation failed")

// MarkdownLinter validates Markdown files using markdownlint
type MarkdownLinter interface {
	Lint(ctx context.Context, content string, initialState *validators.MarkdownState) *LintResult
}

// RealMarkdownLinter implements MarkdownLinter using the markdownlint CLI tool
type RealMarkdownLinter struct {
	runner      execpkg.CommandRunner
	toolChecker execpkg.ToolChecker
}

// NewMarkdownLinter creates a new RealMarkdownLinter
func NewMarkdownLinter(runner execpkg.CommandRunner) *RealMarkdownLinter {
	return &RealMarkdownLinter{
		runner:      runner,
		toolChecker: execpkg.NewToolChecker(),
	}
}

// Lint validates Markdown content using custom rules only
// Note: markdownlint CLI integration is disabled for backward compatibility
func (*RealMarkdownLinter) Lint(
	_ context.Context,
	content string,
	initialState *validators.MarkdownState,
) *LintResult {
	// Run custom markdown analysis
	analysisResult := validators.AnalyzeMarkdown(content, initialState)

	if len(analysisResult.Warnings) > 0 {
		output := strings.Join(analysisResult.Warnings, "\n")

		return &LintResult{
			Success:  false,
			RawOut:   output,
			Findings: []LintFinding{},
			Err:      ErrMarkdownCustomRules,
		}
	}

	return &LintResult{
		Success:  true,
		RawOut:   "",
		Findings: []LintFinding{},
		Err:      nil,
	}
}
