package file

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
	"github.com/smykla-labs/claude-hooks/internal/linters"
	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

const (
	// terraformTimeout is the timeout for terraform/tofu commands
	terraformTimeout = 10 * time.Second

	// tfContextLines is the number of lines before/after an edit to include for validation
	tfContextLines = 2
)

// TerraformValidator validates Terraform/OpenTofu file formatting
type TerraformValidator struct {
	validator.BaseValidator
	formatter   linters.TerraformFormatter
	linter      linters.TfLinter
	tempManager execpkg.TempFileManager
}

// NewTerraformValidator creates a new TerraformValidator
func NewTerraformValidator(
	formatter linters.TerraformFormatter,
	linter linters.TfLinter,
	log logger.Logger,
) *TerraformValidator {
	return &TerraformValidator{
		BaseValidator: *validator.NewBaseValidator("validate-terraform", log),
		formatter:     formatter,
		linter:        linter,
		tempManager:   execpkg.NewTempFileManager(),
	}
}

// Validate checks Terraform formatting and optionally runs tflint
func (v *TerraformValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()

	content, err := v.getContent(hookCtx)
	if err != nil {
		log.Debug("skipping terraform validation", "error", err)
		return validator.Pass()
	}

	if content == "" {
		return validator.Pass()
	}

	// Detect which tool to use
	tool := v.formatter.DetectTool()
	log.Debug("detected terraform tool", "tool", tool)

	// Create temp file for tflint
	tmpFile, cleanup, err := v.tempManager.Create("terraform-*.tf", content)
	if err != nil {
		log.Debug("failed to create temp file", "error", err)
		return validator.Pass()
	}
	defer cleanup()

	var warnings []string

	// Run format check
	if fmtWarning := v.checkFormat(ctx, content, tool); fmtWarning != "" {
		warnings = append(warnings, fmtWarning)
	}

	// Run tflint if available
	if lintWarnings := v.runTflint(ctx, tmpFile); len(lintWarnings) > 0 {
		warnings = append(warnings, lintWarnings...)
	}

	if len(warnings) > 0 {
		message := "Terraform validation warnings"
		details := map[string]string{
			"warnings": strings.Join(warnings, "\n"),
		}

		return validator.WarnWithDetails(message, details)
	}

	return validator.Pass()
}

// getContent extracts terraform content from context
func (v *TerraformValidator) getContent(ctx *hook.Context) (string, error) {
	log := v.Logger()

	// Try to get content from tool input (Write operation)
	if ctx.ToolInput.Content != "" {
		return ctx.ToolInput.Content, nil
	}

	// For Edit operations in PreToolUse, validate only the changed fragment with context
	// to avoid forcing users to fix all existing linting issues
	if ctx.EventType == hook.PreToolUse && ctx.ToolName == hook.Edit {
		filePath := ctx.GetFilePath()
		if filePath == "" {
			return "", errNoContent
		}

		oldStr := ctx.ToolInput.OldString
		newStr := ctx.ToolInput.NewString

		if oldStr == "" || newStr == "" {
			log.Debug("missing old_string or new_string in edit operation")
			return "", errNoContent
		}

		// Read original file to extract context around the edit
		//nolint:gosec // filePath is from Claude Code tool context, not user input
		originalContent, err := os.ReadFile(filePath)
		if err != nil {
			log.Debug("failed to read file for edit validation", "file", filePath, "error", err)
			return "", err
		}

		// Extract fragment with context lines around the edit
		fragment := ExtractEditFragment(
			string(originalContent),
			oldStr,
			newStr,
			tfContextLines,
			log,
		)
		if fragment == "" {
			log.Debug("could not extract edit fragment, skipping validation")
			return "", errNoContent
		}

		fragmentLineCount := len(strings.Split(fragment, "\n"))
		log.Debug("validating edit fragment with context", "fragment_lines", fragmentLineCount)

		return fragment, nil
	}

	// Try to get from file path (Edit or PostToolUse)
	filePath := ctx.GetFilePath()
	if filePath != "" {
		// In PostToolUse, we could read the file, but for now skip
		// as the Bash version doesn't handle this case well either
		return "", errFileValidationNotImpl
	}

	return "", errNoContent
}

// checkFormat runs terraform/tofu fmt -check using TerraformFormatter
func (v *TerraformValidator) checkFormat(ctx context.Context, content, tool string) string {
	if tool == "" {
		return "⚠️  Neither 'tofu' nor 'terraform' found in PATH - skipping format check"
	}

	fmtCtx, cancel := context.WithTimeout(ctx, terraformTimeout)
	defer cancel()

	result := v.formatter.CheckFormat(fmtCtx, content)

	if result.Success {
		return ""
	}

	// Format check failed
	diff := strings.TrimSpace(result.RawOut)
	if diff != "" && len(result.Findings) > 0 {
		return fmt.Sprintf(
			"⚠️  Terraform formatting issues detected:\n%s\n   Run '%s fmt' to fix",
			diff,
			tool,
		)
	}

	if result.Err != nil {
		v.Logger().Debug("fmt command failed", "error", result.Err)
		return fmt.Sprintf("⚠️  Failed to run '%s fmt -check': %v", tool, result.Err)
	}

	return ""
}

// runTflint runs tflint on the file if available using TfLinter
func (v *TerraformValidator) runTflint(ctx context.Context, filePath string) []string {
	lintCtx, cancel := context.WithTimeout(ctx, terraformTimeout)
	defer cancel()

	result := v.linter.Lint(lintCtx, filePath)

	if result.Success {
		return nil
	}

	output := strings.TrimSpace(result.RawOut)
	if output != "" {
		return []string{"⚠️  tflint findings:\n" + output}
	}

	if result.Err != nil {
		v.Logger().Debug("tflint failed", "error", result.Err)
	}

	return nil
}
