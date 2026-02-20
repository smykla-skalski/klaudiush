// Package tools provides checkers for optional tool dependencies.
package tools

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/exec"
)

// ToolChecker checks for optional tool dependencies
type ToolChecker struct {
	toolName        string
	alternatives    []string
	description     string
	severity        doctor.Severity
	installHint     string
	toolCheckerImpl exec.ToolChecker
}

// NewShellcheckChecker creates a checker for shellcheck
func NewShellcheckChecker() *ToolChecker {
	return &ToolChecker{
		toolName:        "shellcheck",
		alternatives:    []string{"shellcheck"},
		description:     "Shell script linting",
		severity:        doctor.SeverityWarning,
		installHint:     "Install with: brew install shellcheck (macOS) or apt-get install shellcheck (Linux)",
		toolCheckerImpl: exec.NewToolChecker(),
	}
}

// NewTerraformChecker creates a checker for terraform/tofu
func NewTerraformChecker() *ToolChecker {
	return &ToolChecker{
		toolName:        "terraform",
		alternatives:    []string{"tofu", "terraform"},
		description:     "Terraform/OpenTofu",
		severity:        doctor.SeverityWarning,
		installHint:     "Install with: brew install opentofu (macOS) or brew install terraform",
		toolCheckerImpl: exec.NewToolChecker(),
	}
}

// NewTflintChecker creates a checker for tflint
func NewTflintChecker() *ToolChecker {
	return &ToolChecker{
		toolName:        "tflint",
		alternatives:    []string{"tflint"},
		description:     "Terraform linting",
		severity:        doctor.SeverityInfo,
		installHint:     "Install with: brew install tflint (macOS)",
		toolCheckerImpl: exec.NewToolChecker(),
	}
}

// NewActionlintChecker creates a checker for actionlint
func NewActionlintChecker() *ToolChecker {
	return &ToolChecker{
		toolName:        "actionlint",
		alternatives:    []string{"actionlint"},
		description:     "GitHub Actions workflow linting",
		severity:        doctor.SeverityInfo,
		installHint:     "Install with: brew install actionlint (macOS)",
		toolCheckerImpl: exec.NewToolChecker(),
	}
}

// NewMarkdownlintChecker creates a checker for markdownlint
func NewMarkdownlintChecker() *ToolChecker {
	return &ToolChecker{
		toolName:        "markdownlint",
		alternatives:    []string{"markdownlint-cli2", "markdownlint", "markdownlint-cli"},
		description:     "Markdown linting",
		severity:        doctor.SeverityWarning,
		installHint:     "Install with: npm install -g markdownlint-cli2 (or markdownlint-cli), or use mise",
		toolCheckerImpl: exec.NewToolChecker(),
	}
}

// Name returns the name of the check
func (c *ToolChecker) Name() string {
	return c.toolName + " available"
}

// Category returns the category of the check
func (*ToolChecker) Category() doctor.Category {
	return doctor.CategoryTools
}

// Check performs the tool availability check
func (c *ToolChecker) Check(_ context.Context) doctor.CheckResult {
	// Try to find any of the alternative tools
	foundTool := c.toolCheckerImpl.FindTool(c.alternatives...)

	if foundTool == "" {
		message := c.toolName + " not found"
		details := []string{
			c.description + " may be limited",
		}

		if c.installHint != "" {
			details = append(details, c.installHint)
		}

		result := doctor.CheckResult{
			Name:     c.Name(),
			Severity: c.severity,
			Status:   doctor.StatusFail,
			Message:  message,
			Details:  details,
		}

		return result
	}

	message := "Found " + foundTool
	if foundTool != c.toolName {
		message = fmt.Sprintf("Found %s (alternative to %s)", foundTool, c.toolName)
	}

	return doctor.Pass(c.Name(), message)
}
