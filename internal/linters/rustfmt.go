package linters

//go:generate mockgen -source=rustfmt.go -destination=rustfmt_mock.go -package=linters

import (
	"context"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// RustfmtOptions configures rustfmt behavior
type RustfmtOptions struct {
	// Edition is the Rust edition (2015, 2018, 2021, 2024)
	Edition string
	// ConfigPath is the path to a rustfmt.toml configuration file
	ConfigPath string
}

// RustfmtChecker validates Rust code formatting using rustfmt
type RustfmtChecker interface {
	Check(ctx context.Context, content string) *LintResult
	CheckWithOptions(ctx context.Context, content string, opts *RustfmtOptions) *LintResult
}

// RealRustfmtChecker implements RustfmtChecker using the rustfmt CLI tool
type RealRustfmtChecker struct {
	linter *ContentLinter
}

// NewRustfmtChecker creates a new RealRustfmtChecker
func NewRustfmtChecker(runner execpkg.CommandRunner) *RealRustfmtChecker {
	return &RealRustfmtChecker{
		linter: NewContentLinter(runner),
	}
}

// NewRustfmtCheckerWithDeps creates a RealRustfmtChecker with a custom ContentLinter (for testing).
func NewRustfmtCheckerWithDeps(linter *ContentLinter) *RealRustfmtChecker {
	return &RealRustfmtChecker{
		linter: linter,
	}
}

// Check validates Rust code formatting using rustfmt
func (r *RealRustfmtChecker) Check(ctx context.Context, content string) *LintResult {
	return r.CheckWithOptions(ctx, content, nil)
}

// CheckWithOptions validates Rust code formatting with custom options
func (r *RealRustfmtChecker) CheckWithOptions(
	ctx context.Context,
	content string,
	opts *RustfmtOptions,
) *LintResult {
	args := []string{"--check"}

	// Default to edition 2021 if not specified
	edition := "2021"
	if opts != nil && opts.Edition != "" {
		edition = opts.Edition
	}

	args = append(args, "--edition", edition)

	// Add config path if specified
	if opts != nil && opts.ConfigPath != "" {
		args = append(args, "--config-path", opts.ConfigPath)
	}

	return r.linter.LintContent(
		ctx,
		"rustfmt",
		"code-*.rs",
		content,
		parseRustfmtOutput,
		args...,
	)
}

// parseRustfmtOutput parses rustfmt diff output into LintFindings
func parseRustfmtOutput(output string) []LintFinding {
	// Rustfmt outputs unified diff to stderr when formatting is needed
	// Return empty if no output (code is already formatted)
	if output == "" {
		return []LintFinding{}
	}

	// Return a single finding with the diff as the message
	return []LintFinding{
		{
			File:     "<temp>",
			Line:     0,
			Column:   0,
			Severity: SeverityError,
			Message:  "Rust code formatting issues detected",
			Rule:     "rustfmt",
		},
	}
}
