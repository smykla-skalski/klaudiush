package linters

//go:generate mockgen -source=actionlint.go -destination=actionlint_mock.go -package=linters

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// actionlintPattern matches actionlint output: file:line:col: message [rule]
var actionlintPattern = regexp.MustCompile(`^(.+):(\d+):(\d+): (.+) \[([^\]]+)\]$`)

// ActionLinter validates GitHub Actions workflow files using actionlint
type ActionLinter interface {
	Lint(ctx context.Context, content string, filePath string) *LintResult
}

// RealActionLinter implements ActionLinter using the actionlint CLI tool
type RealActionLinter struct {
	linter *ContentLinter
}

// NewActionLinter creates a new RealActionLinter
func NewActionLinter(runner execpkg.CommandRunner) *RealActionLinter {
	return &RealActionLinter{
		linter: NewContentLinter(runner),
	}
}

// NewActionLinterWithDeps creates a RealActionLinter with a custom ContentLinter (for testing).
func NewActionLinterWithDeps(linter *ContentLinter) *RealActionLinter {
	return &RealActionLinter{
		linter: linter,
	}
}

// Lint validates GitHub Actions workflow file content
func (a *RealActionLinter) Lint(ctx context.Context, content string, _ string) *LintResult {
	return a.linter.LintContent(
		ctx,
		"actionlint",
		"workflow-*.yml",
		content,
		parseActionlintOutput,
		"-no-color",
	)
}

// parseActionlintOutput parses actionlint text output into LintFindings
// Format: file:line:col: message [rule]
func parseActionlintOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	findings := make([]LintFinding, 0, strings.Count(output, "\n")+1)

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := actionlintPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		lineNum, _ := strconv.Atoi(matches[2])
		col, _ := strconv.Atoi(matches[3])

		findings = append(findings, LintFinding{
			File:     matches[1],
			Line:     lineNum,
			Column:   col,
			Severity: SeverityError,
			Message:  matches[4],
			Rule:     matches[5],
		})
	}

	return findings
}
