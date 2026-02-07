package linters

//go:generate mockgen -source=terraform.go -destination=terraform_mock.go -package=linters

import (
	"context"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

// TerraformFormatter validates and formats Terraform/OpenTofu files
type TerraformFormatter interface {
	CheckFormat(ctx context.Context, content string) *LintResult
	DetectTool() string
}

// RealTerraformFormatter implements TerraformFormatter using terraform/tofu CLI
type RealTerraformFormatter struct {
	runner      execpkg.CommandRunner
	toolChecker execpkg.ToolChecker
	tempManager execpkg.TempFileManager
}

// NewTerraformFormatter creates a new RealTerraformFormatter
func NewTerraformFormatter(runner execpkg.CommandRunner) *RealTerraformFormatter {
	return &RealTerraformFormatter{
		runner:      runner,
		toolChecker: execpkg.NewToolChecker(),
		tempManager: execpkg.NewTempFileManager(),
	}
}

// NewTerraformFormatterWithDeps creates a RealTerraformFormatter with all dependencies injected (for testing).
func NewTerraformFormatterWithDeps(
	runner execpkg.CommandRunner,
	toolChecker execpkg.ToolChecker,
	tempManager execpkg.TempFileManager,
) *RealTerraformFormatter {
	return &RealTerraformFormatter{
		runner:      runner,
		toolChecker: toolChecker,
		tempManager: tempManager,
	}
}

// DetectTool detects whether to use tofu or terraform
func (t *RealTerraformFormatter) DetectTool() string {
	return t.toolChecker.FindTool("tofu", "terraform")
}

// CheckFormat validates Terraform file formatting
func (t *RealTerraformFormatter) CheckFormat(ctx context.Context, content string) *LintResult {
	tool := t.DetectTool()
	if tool == "" {
		return &LintResult{
			Success: true,
			Err:     nil,
		}
	}

	// Create temp file for validation
	tmpFile, cleanup, err := t.tempManager.Create("terraform-*.tf", content)
	if err != nil {
		return &LintResult{
			Success: false,
			Err:     err,
		}
	}
	defer cleanup()

	// Run terraform fmt -check -diff
	result := t.runner.Run(ctx, tool, "fmt", "-check", "-diff", tmpFile)

	findings := t.parseDiffOutput(result.Stdout)

	return &LintResult{
		Success:  result.Err == nil,
		RawOut:   result.Stdout + result.Stderr,
		Findings: findings,
		Err:      result.Err,
	}
}

// parseDiffOutput parses terraform fmt diff output into findings
func (*RealTerraformFormatter) parseDiffOutput(output string) []LintFinding {
	if output == "" {
		return []LintFinding{}
	}

	// For now, create a single finding with the entire diff
	// A more sophisticated parser could extract specific line changes
	return []LintFinding{
		{
			File:     "",
			Line:     0,
			Column:   0,
			Message:  "Terraform formatting issues detected",
			Severity: SeverityError,
		},
	}
}
