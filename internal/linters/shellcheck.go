package linters

//go:generate mockgen -source=shellcheck.go -destination=shellcheck_mock.go -package=linters

import (
	"context"
	"encoding/json"
	"strconv"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// shellcheckFinding represents a single finding from shellcheck JSON output
type shellcheckFinding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Level   string `json:"level"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ShellCheckOptions configures shellcheck behavior
type ShellCheckOptions struct {
	// ExcludeCodes are shellcheck codes to exclude (e.g., []int{2034, 2154})
	ExcludeCodes []int
}

// ShellChecker validates shell scripts using shellcheck
type ShellChecker interface {
	Check(ctx context.Context, content string) *LintResult
	CheckWithOptions(ctx context.Context, content string, opts *ShellCheckOptions) *LintResult
}

// RealShellChecker implements ShellChecker using the shellcheck CLI tool
type RealShellChecker struct {
	linter *ContentLinter
}

// NewShellChecker creates a new RealShellChecker
func NewShellChecker(runner execpkg.CommandRunner) *RealShellChecker {
	return &RealShellChecker{
		linter: NewContentLinter(runner),
	}
}

// NewShellCheckerWithDeps creates a RealShellChecker with a custom ContentLinter (for testing).
func NewShellCheckerWithDeps(linter *ContentLinter) *RealShellChecker {
	return &RealShellChecker{
		linter: linter,
	}
}

// Check validates shell script content using shellcheck
func (s *RealShellChecker) Check(ctx context.Context, content string) *LintResult {
	return s.CheckWithOptions(ctx, content, nil)
}

// CheckWithOptions validates shell script content with custom options
func (s *RealShellChecker) CheckWithOptions(
	ctx context.Context,
	content string,
	opts *ShellCheckOptions,
) *LintResult {
	args := []string{"--format=json"}

	// Add exclude codes if specified
	if opts != nil {
		for _, code := range opts.ExcludeCodes {
			args = append(args, "--exclude=SC"+strconv.Itoa(code))
		}
	}

	return s.linter.LintContent(
		ctx,
		"shellcheck",
		"script-*.sh",
		content,
		parseShellcheckOutput,
		args...,
	)
}

// parseShellcheckOutput parses shellcheck JSON output into LintFindings
func parseShellcheckOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	var scFindings []shellcheckFinding
	if err := json.Unmarshal([]byte(output), &scFindings); err != nil {
		return []LintFinding{}
	}

	findings := make([]LintFinding, 0, len(scFindings))

	for _, f := range scFindings {
		findings = append(findings, LintFinding{
			File:     f.File,
			Line:     f.Line,
			Column:   f.Column,
			Severity: shellcheckLevelToSeverity(f.Level),
			Message:  f.Message,
			Rule:     formatShellcheckRule(f.Code),
		})
	}

	return findings
}

// shellcheckLevelToSeverity converts shellcheck level to LintSeverity
func shellcheckLevelToSeverity(level string) LintSeverity {
	switch level {
	case "error":
		return SeverityError
	case "warning":
		return SeverityWarning
	case "info", "style":
		return SeverityInfo
	default:
		return SeverityWarning
	}
}

// formatShellcheckRule formats shellcheck code as SC#### rule
func formatShellcheckRule(code int) string {
	return "SC" + strconv.Itoa(code)
}
