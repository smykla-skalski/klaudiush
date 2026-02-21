package file

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const (
	defaultOxlintTimeout = 10 * time.Second

	// defaultJavaScriptContextLines is the number of lines before/after an edit to include for validation
	defaultJavaScriptContextLines = 2
)

// jsFragmentExcludes are oxlint rules to exclude when validating fragments.
// These are false positives due to limited context:
// - no-unused-vars: variable may be used elsewhere in file
// - no-undef: variable may be defined elsewhere
// - import/no-unresolved: imports may be valid in full context
var jsFragmentExcludes = []string{"no-unused-vars", "no-undef", "import/no-unresolved"}

// JavaScriptValidator validates JavaScript/TypeScript scripts using oxlint.
type JavaScriptValidator struct {
	validator.BaseValidator
	checker     linters.OxlintChecker
	config      *config.JavaScriptValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewJavaScriptValidator creates a new JavaScriptValidator.
func NewJavaScriptValidator(
	log logger.Logger,
	checker linters.OxlintChecker,
	cfg *config.JavaScriptValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *JavaScriptValidator {
	return &JavaScriptValidator{
		BaseValidator: *validator.NewBaseValidator("validate-javascript", log),
		checker:       checker,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate validates JavaScript/TypeScript scripts using oxlint.
func (v *JavaScriptValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating JavaScript/TypeScript script")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Check if oxlint is enabled
	if !v.isUseOxlint() {
		log.Debug("oxlint is disabled, skipping validation")
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

	// Run oxlint using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	// Build exclude codes from config and fragment-specific excludes
	opts := v.buildOxlintOptions(ci.IsFragment)
	result := v.checker.CheckWithOptions(lintCtx, ci.Content, opts)

	if result.Success {
		log.Debug("oxlint passed")
		return validator.Pass()
	}

	log.Debug("oxlint failed", "output", result.RawOut)

	return validator.FailWithRef(validator.RefOxlintCheck, v.formatOxlintOutput(result))
}

// extractContent creates a ContentExtractor and extracts content from the hook context.
func (v *JavaScriptValidator) extractContent(
	ctx *hook.Context,
	filePath string,
) (*ContentInfo, error) {
	return NewContentExtractor(v.Logger(), v.getContextLines()).Extract(ctx, filePath)
}

// formatOxlintOutput formats oxlint findings into human-readable text.
func (*JavaScriptValidator) formatOxlintOutput(result *linters.LintResult) string {
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

// buildOxlintOptions creates OxlintCheckOptions with excludes from config and fragment-specific rules.
func (v *JavaScriptValidator) buildOxlintOptions(isFragment bool) *linters.OxlintCheckOptions {
	var (
		excludes   []string
		configPath string
	)

	// Add config excludes and config path

	if v.config != nil {
		excludes = append(excludes, v.config.ExcludeRules...)
		configPath = v.config.OxlintConfig
	}

	// Add fragment-specific excludes for Edit operations
	if isFragment {
		excludes = append(excludes, jsFragmentExcludes...)
	}

	if len(excludes) == 0 && configPath == "" {
		return nil
	}

	return &linters.OxlintCheckOptions{
		ExcludeRules: excludes,
		ConfigPath:   configPath,
	}
}

// getTimeout returns the configured timeout for oxlint operations.
func (v *JavaScriptValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultOxlintTimeout
}

// getContextLines returns the configured number of context lines for edit validation.
func (v *JavaScriptValidator) getContextLines() int {
	if v.config != nil && v.config.ContextLines != nil {
		return *v.config.ContextLines
	}

	return defaultJavaScriptContextLines
}

// Category returns the validator category for parallel execution.
// JavaScriptValidator uses CategoryIO because it invokes oxlint.
func (*JavaScriptValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}

// isUseOxlint returns whether oxlint integration is enabled.
func (v *JavaScriptValidator) isUseOxlint() bool {
	if v.config != nil && v.config.UseOxlint != nil {
		return *v.config.UseOxlint
	}

	return true
}
