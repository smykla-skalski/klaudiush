package file

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const (
	defaultRuffTimeout = 10 * time.Second

	// defaultPythonContextLines is the number of lines before/after an edit to include for validation
	defaultPythonContextLines = 2
)

// pythonFragmentExcludes are ruff codes to exclude when validating fragments.
// These are false positives due to limited context:
// - F401: unused imports (may be imported for use elsewhere in file)
// - F841: local variable assigned but never used (may be used elsewhere)
var pythonFragmentExcludes = []string{"F401", "F841"}

// PythonValidator validates Python scripts using ruff.
type PythonValidator struct {
	validator.BaseValidator
	checker linters.RuffChecker
	config  *config.PythonValidatorConfig
}

// NewPythonValidator creates a new PythonValidator.
func NewPythonValidator(
	log logger.Logger,
	checker linters.RuffChecker,
	cfg *config.PythonValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *PythonValidator {
	return &PythonValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules("validate-python", log, ruleAdapter),
		checker:       checker,
		config:        cfg,
	}
}

// Validate validates Python scripts using ruff.
func (v *PythonValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating Python script")

	// Check rules first
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	// Check if ruff is enabled
	if !v.isUseRuff() {
		log.Debug("ruff is disabled, skipping validation")
		return validator.Pass()
	}

	// Get the file path
	filePath := hookCtx.GetFilePath()
	if filePath == "" {
		log.Debug("no file path provided")
		return validator.Pass()
	}

	// Get content based on operation type
	ci, err := v.extractContent(hookCtx, filePath)
	if err != nil {
		log.Debug("failed to get content", "error", err)
		return validator.Pass()
	}

	// Run ruff using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	// Build exclude codes from config and fragment-specific excludes
	opts := v.buildRuffOptions(ci.IsFragment)
	result := v.checker.CheckWithOptions(lintCtx, ci.Content, opts)

	if result.Success {
		log.Debug("ruff passed")
		return validator.Pass()
	}

	log.Debug("ruff failed", "output", result.RawOut)

	return validator.FailWithRef(validator.RefRuffCheck, v.formatRuffOutput(result))
}

// extractContent creates a ContentExtractor and extracts content from the hook context.
func (v *PythonValidator) extractContent(
	ctx *hook.Context,
	filePath string,
) (*ContentInfo, error) {
	return NewContentExtractor(v.Logger(), v.getContextLines()).Extract(ctx, filePath)
}

// formatRuffOutput formats ruff findings into human-readable text.
//
//nolint:dupl // Same display logic as formatOxlintOutput, not worth abstracting
func (*PythonValidator) formatRuffOutput(result *linters.LintResult) string {
	if len(result.Findings) == 0 {
		// Fallback to raw output if no findings parsed
		lines := strings.Split(result.RawOut, "\n")

		var cleanLines []string

		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				cleanLines = append(cleanLines, line)
			}
		}

		return strings.Join(cleanLines, "\n")
	}

	lines := make([]string, 0, len(result.Findings))

	for _, f := range result.Findings {
		// Format: file:line:col: message (rule)
		line := fmt.Sprintf("%s:%d:%d: %s", f.File, f.Line, f.Column, f.Message)
		if f.Rule != "" {
			line += " (" + f.Rule + ")"
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// buildRuffOptions creates RuffCheckOptions with excludes from config and fragment-specific rules.
func (v *PythonValidator) buildRuffOptions(isFragment bool) *linters.RuffCheckOptions {
	var (
		excludes   []string
		configPath string
	)

	// Add config excludes and config path

	if v.config != nil {
		excludes = append(excludes, v.config.ExcludeRules...)
		configPath = v.config.RuffConfig
	}

	// Add fragment-specific excludes for Edit operations
	if isFragment {
		excludes = append(excludes, pythonFragmentExcludes...)
	}

	if len(excludes) == 0 && configPath == "" {
		return nil
	}

	return &linters.RuffCheckOptions{
		ExcludeRules: excludes,
		ConfigPath:   configPath,
	}
}

// getTimeout returns the configured timeout for ruff operations.
func (v *PythonValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultRuffTimeout
}

// getContextLines returns the configured number of context lines for edit validation.
func (v *PythonValidator) getContextLines() int {
	if v.config != nil && v.config.ContextLines != nil {
		return *v.config.ContextLines
	}

	return defaultPythonContextLines
}

// Category returns the validator category for parallel execution.
// PythonValidator uses CategoryIO because it invokes ruff.
func (*PythonValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}

// isUseRuff returns whether ruff integration is enabled.
func (v *PythonValidator) isUseRuff() bool {
	if v.config != nil && v.config.UseRuff != nil {
		return *v.config.UseRuff
	}

	return true
}
