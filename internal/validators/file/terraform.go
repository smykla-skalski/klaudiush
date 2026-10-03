package file

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const (
	// defaultTerraformTimeout is the timeout for terraform/tofu commands
	defaultTerraformTimeout = 10 * time.Second

	// defaultTfContextLines is the number of lines before/after an edit to include for validation
	defaultTfContextLines = 2
)

// TerraformValidator validates Terraform/OpenTofu file formatting
type TerraformValidator struct {
	validator.BaseValidator
	formatter   linters.TerraformFormatter
	linter      linters.TfLinter
	tempManager execpkg.TempFileManager
	config      *config.TerraformValidatorConfig
}

// NewTerraformValidator creates a new TerraformValidator
func NewTerraformValidator(
	formatter linters.TerraformFormatter,
	linter linters.TfLinter,
	log logger.Logger,
	cfg *config.TerraformValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *TerraformValidator {
	return &TerraformValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules("validate-terraform", log, ruleAdapter),
		formatter:     formatter,
		linter:        linter,
		tempManager:   execpkg.NewTempFileManager(),
		config:        cfg,
	}
}

// Validate checks Terraform formatting and optionally runs tflint
func (v *TerraformValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()

	// Check rules first
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	content, err := v.getContent(hookCtx)
	if err != nil {
		log.Debug("skipping terraform validation", "error", err)
		return validator.Pass()
	}

	// After the tool ran, content is the whole file as the tool left it.
	cov := fileCoverage(hookCtx, hookCtx.IsAfterTool() && hookCtx.GetFilePath() != "")

	if content == "" {
		return cov.mark(validator.Pass())
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

	var (
		warnings    []string
		unavailable *validator.Result
	)

	// Run format check if enabled
	if v.isCheckFormat() {
		fmtWarning, notRun := v.checkFormat(ctx, content, tool)
		if fmtWarning != "" {
			warnings = append(warnings, fmtWarning)
		}

		unavailable = mergeUnavailable(unavailable, notRun)
	}

	// Run tflint if enabled and available
	if v.isUseTflint() {
		lintWarnings, notRun := v.runTflint(ctx, tmpFile)
		warnings = append(warnings, lintWarnings...)
		unavailable = mergeUnavailable(unavailable, notRun)
	}

	cov = cov.only(unavailable == nil)

	// A missing optional tool must not hide what the other check found, as
	// missing tools are ignored by default; any other failure is reported
	// with the findings attached.
	if unavailable != nil &&
		(len(warnings) == 0 || unavailable.UnavailableReason != validator.ReasonMissingTool) {
		if len(warnings) > 0 {
			unavailable.Message += "\n" + strings.Join(warnings, "\n")
		}

		return unavailable
	}

	if len(warnings) > 0 {
		message := "Terraform validation warnings"
		details := map[string]string{
			"warnings": strings.Join(warnings, "\n"),
		}

		return cov.mark(validator.WarnWithDetails(message, details))
	}

	return cov.mark(validator.Pass())
}

// mergeUnavailable combines the results of two checks that could not run.
// A missing tool is ignored by default, so any other reason decides; the
// other check's message is kept either way.
func mergeUnavailable(first, second *validator.Result) *validator.Result {
	if first == nil || second == nil {
		return cmp.Or(first, second)
	}

	kept, other := first, second
	if kept.UnavailableReason == validator.ReasonMissingTool &&
		other.UnavailableReason != validator.ReasonMissingTool {
		kept, other = other, kept
	}

	kept.Message += "\n" + other.Message

	return kept
}

// getContent extracts terraform content from context
func (v *TerraformValidator) getContent(ctx *hook.Context) (string, error) {
	log := v.Logger()

	if content, ok, err := readToolResult(ctx, ctx.GetFilePath()); ok {
		return content, err
	}

	// Try to get content from tool input (Write operation)
	if ctx.ToolInput.Content != "" {
		return ctx.ToolInput.Content, nil
	}

	// For Edit operations in PreToolUse, validate only the changed fragment with context
	// to avoid forcing users to fix all existing linting issues
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
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
			v.getContextLines(),
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

// checkFormat runs terraform/tofu fmt -check using TerraformFormatter. The
// second return value is set when the check could not run.
func (v *TerraformValidator) checkFormat(
	ctx context.Context,
	content, tool string,
) (string, *validator.Result) {
	if tool == "" {
		return "", validator.Unavailable(
			validator.ReasonMissingTool,
			"Neither 'tofu' nor 'terraform' found in PATH, so the format check did not run",
		)
	}

	fmtCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	result := v.formatter.CheckFormat(fmtCtx, content)

	if notRun := lintUnavailable(fmtCtx, tool+" fmt", result); notRun != nil {
		return "", notRun
	}

	if result.Success {
		return "", nil
	}

	// Format check failed
	diff := strings.TrimSpace(result.RawOut)
	if diff != "" && len(result.Findings) > 0 {
		return fmt.Sprintf(
			"Terraform formatting issues detected:\n%s\nRun '%s fmt' to fix",
			diff,
			tool,
		), nil
	}

	v.Logger().Debug("fmt command failed", "error", result.Err)

	return "", validator.Unavailable(
		validator.ReasonError,
		fmt.Sprintf("Failed to run '%s fmt -check': %v", tool, result.Err),
	)
}

// runTflint runs tflint on the file if available using TfLinter. The second
// return value is set when tflint could not run.
func (v *TerraformValidator) runTflint(
	ctx context.Context,
	filePath string,
) ([]string, *validator.Result) {
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	result := v.linter.Lint(lintCtx, filePath)

	if notRun := lintUnavailable(lintCtx, "tflint", result); notRun != nil {
		return nil, notRun
	}

	if result.Success {
		return nil, nil
	}

	return []string{"tflint findings:\n" + strings.TrimSpace(result.RawOut)}, nil
}

// getTimeout returns the configured timeout for terraform/tofu operations.
func (v *TerraformValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultTerraformTimeout
}

// getContextLines returns the configured number of context lines for edit validation.
func (v *TerraformValidator) getContextLines() int {
	if v.config != nil && v.config.ContextLines != nil {
		return *v.config.ContextLines
	}

	return defaultTfContextLines
}

// isCheckFormat returns whether format checking is enabled.
func (v *TerraformValidator) isCheckFormat() bool {
	if v.config != nil && v.config.CheckFormat != nil {
		return *v.config.CheckFormat
	}

	return true
}

// isUseTflint returns whether tflint integration is enabled.
func (v *TerraformValidator) isUseTflint() bool {
	if v.config != nil && v.config.UseTflint != nil {
		return *v.config.UseTflint
	}

	return true
}

// Category returns the validator category for parallel execution.
// TerraformValidator uses CategoryIO because it invokes terraform/tofu and tflint.
func (*TerraformValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
