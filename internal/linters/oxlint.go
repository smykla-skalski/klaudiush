package linters

//go:generate mockgen -source=oxlint.go -destination=oxlint_mock.go -package=linters

import (
	"context"
	"encoding/json"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
)

// oxlintFinding represents a single file's findings from oxlint JSON output
type oxlintFinding struct {
	FilePath string          `json:"filePath"`
	Messages []oxlintMessage `json:"messages"`
}

// oxlintMessage represents a single finding message
type oxlintMessage struct {
	RuleID   string `json:"ruleId"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity int    `json:"severity"` // 0=off, 1=warn, 2=error
}

// OxlintCheckOptions configures oxlint behavior
type OxlintCheckOptions struct {
	// ExcludeRules are oxlint rules to exclude (e.g., []string{"no-unused-vars", "no-console"})
	ExcludeRules []string
	// ConfigPath is the path to an oxlint configuration file (.oxlintrc.json)
	ConfigPath string
}

// OxlintChecker validates JavaScript/TypeScript code using oxlint
type OxlintChecker interface {
	Check(ctx context.Context, content string) *LintResult
	CheckWithOptions(ctx context.Context, content string, opts *OxlintCheckOptions) *LintResult
}

// RealOxlintChecker implements OxlintChecker using the oxlint CLI tool
type RealOxlintChecker struct {
	linter *ContentLinter
}

// NewOxlintChecker creates a new RealOxlintChecker
func NewOxlintChecker(runner execpkg.CommandRunner) *RealOxlintChecker {
	return &RealOxlintChecker{
		linter: NewContentLinter(runner),
	}
}

// NewOxlintCheckerWithDeps creates a RealOxlintChecker with a custom ContentLinter (for testing).
func NewOxlintCheckerWithDeps(linter *ContentLinter) *RealOxlintChecker {
	return &RealOxlintChecker{
		linter: linter,
	}
}

// Check validates JavaScript/TypeScript code using oxlint
func (o *RealOxlintChecker) Check(ctx context.Context, content string) *LintResult {
	return o.CheckWithOptions(ctx, content, nil)
}

// CheckWithOptions validates JavaScript/TypeScript code with custom options
func (o *RealOxlintChecker) CheckWithOptions(
	ctx context.Context,
	content string,
	opts *OxlintCheckOptions,
) *LintResult {
	args := []string{"--format=json"}

	// Add config path if specified
	if opts != nil && opts.ConfigPath != "" {
		args = append(args, "-c", opts.ConfigPath)
	}

	// Add exclude rules if specified
	if opts != nil {
		for _, rule := range opts.ExcludeRules {
			args = append(args, "--disable", rule)
		}
	}

	return o.linter.LintContent(
		ctx,
		"oxlint",
		"script-*.js",
		content,
		parseOxlintOutput,
		args...,
	)
}

// parseOxlintOutput parses oxlint JSON output into LintFindings
func parseOxlintOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	var oxlintFindings []oxlintFinding
	if err := json.Unmarshal([]byte(output), &oxlintFindings); err != nil {
		return []LintFinding{}
	}

	var findings []LintFinding

	for _, f := range oxlintFindings {
		for _, msg := range f.Messages {
			severity := SeverityError
			if msg.Severity == 1 {
				severity = SeverityWarning
			}

			findings = append(findings, LintFinding{
				File:     f.FilePath,
				Line:     msg.Line,
				Column:   msg.Column,
				Severity: severity,
				Message:  msg.Message,
				Rule:     msg.RuleID,
			})
		}
	}

	return findings
}
