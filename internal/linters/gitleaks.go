package linters

//go:generate mockgen -source=gitleaks.go -destination=gitleaks_mock.go -package=linters

import (
	"context"
	"encoding/json"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// gitleaksFinding represents a single finding from gitleaks JSON output.
type gitleaksFinding struct {
	Description string `json:"Description"`
	File        string `json:"File"`
	StartLine   int    `json:"StartLine"`
	EndLine     int    `json:"EndLine"`
	StartColumn int    `json:"StartColumn"`
	EndColumn   int    `json:"EndColumn"`
	Match       string `json:"Match"`
	Secret      string `json:"Secret"`
	RuleID      string `json:"RuleID"`
}

// GitleaksChecker validates content for secrets using gitleaks.
type GitleaksChecker interface {
	// IsAvailable returns true if gitleaks is installed.
	IsAvailable() bool

	// Check validates content for secrets.
	Check(ctx context.Context, content string) *LintResult
}

// RealGitleaksChecker implements GitleaksChecker using the gitleaks CLI tool.
type RealGitleaksChecker struct {
	linter      *ContentLinter
	toolChecker execpkg.ToolChecker
}

// NewGitleaksChecker creates a new RealGitleaksChecker.
func NewGitleaksChecker(runner execpkg.CommandRunner) *RealGitleaksChecker {
	return &RealGitleaksChecker{
		linter:      NewContentLinter(runner),
		toolChecker: execpkg.NewToolChecker(),
	}
}

// NewGitleaksCheckerWithDeps creates a RealGitleaksChecker with custom dependencies (for testing).
func NewGitleaksCheckerWithDeps(
	linter *ContentLinter,
	toolChecker execpkg.ToolChecker,
) *RealGitleaksChecker {
	return &RealGitleaksChecker{
		linter:      linter,
		toolChecker: toolChecker,
	}
}

// IsAvailable returns true if gitleaks is installed.
func (g *RealGitleaksChecker) IsAvailable() bool {
	return g.toolChecker.IsAvailable("gitleaks")
}

// Check validates content for secrets using gitleaks.
func (g *RealGitleaksChecker) Check(ctx context.Context, content string) *LintResult {
	return g.linter.LintContent(
		ctx,
		"gitleaks",
		"content-*.txt",
		content,
		parseGitleaksOutput,
		"detect",
		"--no-git",
		"--report-format=json",
		"--source",
	)
}

// parseGitleaksOutput parses gitleaks JSON output into LintFindings.
func parseGitleaksOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	var glFindings []gitleaksFinding
	if err := json.Unmarshal([]byte(output), &glFindings); err != nil {
		return []LintFinding{}
	}

	findings := make([]LintFinding, 0, len(glFindings))

	for _, f := range glFindings {
		findings = append(findings, LintFinding{
			File:     f.File,
			Line:     f.StartLine,
			Column:   f.StartColumn,
			Severity: SeverityError,
			Message:  f.Description + ": " + f.RuleID,
			Rule:     f.RuleID,
		})
	}

	return findings
}
