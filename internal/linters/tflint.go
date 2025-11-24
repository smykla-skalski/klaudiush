package linters

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// tflintPattern matches tflint compact format: file:line:col: severity - message (rule)
var tflintPattern = regexp.MustCompile(`^(.+):(\d+):(\d+): (\w+) - (.+) \(([^)]+)\)$`)

// TfLinter validates Terraform files using tflint
type TfLinter interface {
	Lint(ctx context.Context, filePath string) *LintResult
}

// RealTfLinter implements TfLinter using the tflint CLI tool
type RealTfLinter struct {
	runner      execpkg.CommandRunner
	toolChecker execpkg.ToolChecker
}

// NewTfLinter creates a new RealTfLinter
func NewTfLinter(runner execpkg.CommandRunner) *RealTfLinter {
	return &RealTfLinter{
		runner:      runner,
		toolChecker: execpkg.NewToolChecker(),
	}
}

// Lint validates Terraform file using tflint
func (t *RealTfLinter) Lint(ctx context.Context, filePath string) *LintResult {
	// Check if tflint is available
	if !t.toolChecker.IsAvailable("tflint") {
		return &LintResult{
			Success: true,
			Err:     nil,
		}
	}

	// Run tflint with compact format
	result := t.runner.Run(ctx, "tflint", "--format=compact", filePath)

	// tflint returns non-zero when findings are detected
	if result.Err != nil {
		// If there's output, it means there are findings (not an error)
		output := result.Stdout
		if output == "" {
			output = result.Stderr
		}

		if output != "" {
			return &LintResult{
				Success:  false,
				RawOut:   output,
				Findings: parseTflintOutput(output),
				Err:      result.Err,
			}
		}

		// Real error
		return &LintResult{
			Success: false,
			Err:     result.Err,
		}
	}

	// No findings
	return &LintResult{
		Success:  true,
		RawOut:   result.Stdout,
		Findings: []LintFinding{},
		Err:      nil,
	}
}

// parseTflintOutput parses tflint compact output into LintFindings
// Format: file:line:col: severity - message (rule)
func parseTflintOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	findings := make([]LintFinding, 0, strings.Count(output, "\n")+1)

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := tflintPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		lineNum, _ := strconv.Atoi(matches[2])
		col, _ := strconv.Atoi(matches[3])

		findings = append(findings, LintFinding{
			File:     matches[1],
			Line:     lineNum,
			Column:   col,
			Severity: tflintSeverityToLintSeverity(matches[4]),
			Message:  matches[5],
			Rule:     matches[6],
		})
	}

	return findings
}

// tflintSeverityToLintSeverity converts tflint severity to LintSeverity
func tflintSeverityToLintSeverity(severity string) LintSeverity {
	switch strings.ToLower(severity) {
	case "error":
		return SeverityError
	case "warning":
		return SeverityWarning
	case "notice", "info":
		return SeverityInfo
	default:
		return SeverityWarning
	}
}
