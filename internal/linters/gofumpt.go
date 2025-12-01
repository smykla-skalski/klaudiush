package linters

//go:generate mockgen -source=gofumpt.go -destination=gofumpt_mock.go -package=linters

import (
	"context"
	"strings"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// GofumptOptions configures gofumpt behavior
type GofumptOptions struct {
	// ExtraRules enables gofumpt's -extra flag for stricter rules
	ExtraRules bool
	// Lang specifies Go version (e.g., "go1.21")
	Lang string
	// ModPath specifies module path
	ModPath string
}

// GofumptChecker validates Go code formatting using gofumpt
type GofumptChecker interface {
	Check(ctx context.Context, content string) *LintResult
	CheckWithOptions(ctx context.Context, content string, opts *GofumptOptions) *LintResult
}

// RealGofumptChecker implements GofumptChecker using the gofumpt CLI tool
type RealGofumptChecker struct {
	linter *ContentLinter
}

// NewGofumptChecker creates a new RealGofumptChecker
func NewGofumptChecker(runner execpkg.CommandRunner) *RealGofumptChecker {
	return &RealGofumptChecker{
		linter: NewContentLinter(runner),
	}
}

// NewGofumptCheckerWithDeps creates a RealGofumptChecker with a custom ContentLinter (for testing).
func NewGofumptCheckerWithDeps(linter *ContentLinter) *RealGofumptChecker {
	return &RealGofumptChecker{
		linter: linter,
	}
}

// Check validates Go code formatting using gofumpt
func (g *RealGofumptChecker) Check(ctx context.Context, content string) *LintResult {
	return g.CheckWithOptions(ctx, content, nil)
}

// CheckWithOptions validates Go code formatting with custom options
func (g *RealGofumptChecker) CheckWithOptions(
	ctx context.Context,
	content string,
	opts *GofumptOptions,
) *LintResult {
	// gofumpt flags:
	// -l: list files with formatting differences
	// -d: show diff of formatting changes
	args := []string{"-l", "-d"}

	// Add optional flags
	if opts != nil {
		if opts.ExtraRules {
			args = append(args, "-extra")
		}

		if opts.Lang != "" {
			args = append(args, "-lang", opts.Lang)
		}

		if opts.ModPath != "" {
			args = append(args, "-modpath", opts.ModPath)
		}
	}

	return g.linter.LintContent(
		ctx,
		"gofumpt",
		"code-*.go",
		content,
		parseGofumptOutput,
		args...,
	)
}

// parseGofumptOutput parses gofumpt diff output into LintFindings
func parseGofumptOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	// gofumpt outputs a unified diff when there are formatting issues
	// We'll create a single finding with the diff as the message
	lines := strings.Split(output, "\n")

	var diffLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			diffLines = append(diffLines, line)
		}
	}

	if len(diffLines) == 0 {
		return []LintFinding{}
	}

	// Return a single finding with the full diff
	return []LintFinding{
		{
			File:     "<content>",
			Line:     0,
			Column:   0,
			Severity: SeverityError,
			Message:  "Go code formatting issues detected",
			Rule:     "gofumpt",
		},
	}
}
