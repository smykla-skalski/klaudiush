package linters

//go:generate mockgen -source=ruff.go -destination=ruff_mock.go -package=linters

import (
	"context"
	"encoding/json"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// ruffFinding represents a single finding from ruff JSON output
type ruffFinding struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	Location    ruffLocation `json:"location"`
	EndLocation ruffLocation `json:"end_location"`
	Filename    string       `json:"filename"`
}

// ruffLocation represents a location in ruff output
type ruffLocation struct {
	Row    int `json:"row"`
	Column int `json:"column"`
}

// RuffCheckOptions configures ruff behavior
type RuffCheckOptions struct {
	// ExcludeRules are ruff codes to exclude (e.g., []string{"F401", "E501"})
	ExcludeRules []string
	// ConfigPath is the path to a ruff configuration file (pyproject.toml or ruff.toml)
	ConfigPath string
}

// RuffChecker validates Python code using ruff
type RuffChecker interface {
	Check(ctx context.Context, content string) *LintResult
	CheckWithOptions(ctx context.Context, content string, opts *RuffCheckOptions) *LintResult
}

// RealRuffChecker implements RuffChecker using the ruff CLI tool
type RealRuffChecker struct {
	linter *ContentLinter
}

// NewRuffChecker creates a new RealRuffChecker
func NewRuffChecker(runner execpkg.CommandRunner) *RealRuffChecker {
	return &RealRuffChecker{
		linter: NewContentLinter(runner),
	}
}

// NewRuffCheckerWithDeps creates a RealRuffChecker with a custom ContentLinter (for testing).
func NewRuffCheckerWithDeps(linter *ContentLinter) *RealRuffChecker {
	return &RealRuffChecker{
		linter: linter,
	}
}

// Check validates Python code using ruff
func (r *RealRuffChecker) Check(ctx context.Context, content string) *LintResult {
	return r.CheckWithOptions(ctx, content, nil)
}

// CheckWithOptions validates Python code with custom options
func (r *RealRuffChecker) CheckWithOptions(
	ctx context.Context,
	content string,
	opts *RuffCheckOptions,
) *LintResult {
	args := []string{"check", "--output-format=json"}

	// Add config path if specified
	if opts != nil && opts.ConfigPath != "" {
		args = append(args, "--config="+opts.ConfigPath)
	}

	// Add exclude rules if specified
	if opts != nil {
		for _, code := range opts.ExcludeRules {
			args = append(args, "--ignore="+code)
		}
	}

	return r.linter.LintContent(
		ctx,
		"ruff",
		"script-*.py",
		content,
		parseRuffOutput,
		args...,
	)
}

// parseRuffOutput parses ruff JSON output into LintFindings
func parseRuffOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	var ruffFindings []ruffFinding
	if err := json.Unmarshal([]byte(output), &ruffFindings); err != nil {
		return []LintFinding{}
	}

	findings := make([]LintFinding, 0, len(ruffFindings))

	for _, f := range ruffFindings {
		findings = append(findings, LintFinding{
			File:     f.Filename,
			Line:     f.Location.Row,
			Column:   f.Location.Column,
			Severity: SeverityError, // Ruff treats all violations as errors
			Message:  f.Message,
			Rule:     f.Code,
		})
	}

	return findings
}
