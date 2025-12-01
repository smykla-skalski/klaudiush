package file

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
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
	checker     linters.RuffChecker
	config      *config.PythonValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewPythonValidator creates a new PythonValidator.
func NewPythonValidator(
	log logger.Logger,
	checker linters.RuffChecker,
	cfg *config.PythonValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *PythonValidator {
	return &PythonValidator{
		BaseValidator: *validator.NewBaseValidator("validate-python", log),
		checker:       checker,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate validates Python scripts using ruff.
func (v *PythonValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating Python script")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
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
	pc, err := v.getContent(hookCtx, filePath)
	if err != nil {
		log.Debug("failed to get content", "error", err)
		return validator.Pass()
	}

	// Run ruff using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	// Build exclude codes from config and fragment-specific excludes
	opts := v.buildRuffOptions(pc.isFragment)
	result := v.checker.CheckWithOptions(lintCtx, pc.content, opts)

	if result.Success {
		log.Debug("ruff passed")
		return validator.Pass()
	}

	log.Debug("ruff failed", "output", result.RawOut)

	return validator.FailWithRef(validator.RefRuffCheck, v.formatRuffOutput(result))
}

// pythonContent holds Python script content and metadata for validation
type pythonContent struct {
	content    string
	isFragment bool
}

// getContent extracts Python script content from context
//
//nolint:dupl // Similar pattern to ShellScriptValidator.getContent, acceptable duplication
func (v *PythonValidator) getContent(
	ctx *hook.Context,
	filePath string,
) (*pythonContent, error) {
	log := v.Logger()

	// For Edit operations, validate only the changed fragment with context
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		content, err := v.getEditContent(ctx, filePath)
		if err != nil {
			return nil, err
		}

		return &pythonContent{content: content, isFragment: true}, nil
	}

	// Get content from context or read from file (Write operation)
	content := ctx.ToolInput.Content
	if content != "" {
		return &pythonContent{content: content, isFragment: false}, nil
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		log.Debug("file does not exist, skipping", "file", filePath)
		return nil, err
	}

	// Read file content
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from Claude Code context
	if err != nil {
		log.Debug("failed to read file", "file", filePath, "error", err)
		return nil, err
	}

	return &pythonContent{content: string(data), isFragment: false}, nil
}

// getEditContent extracts content for Edit operations with context
func (v *PythonValidator) getEditContent(
	ctx *hook.Context,
	filePath string,
) (string, error) {
	log := v.Logger()

	oldStr := ctx.ToolInput.OldString
	newStr := ctx.ToolInput.NewString

	if oldStr == "" || newStr == "" {
		log.Debug("missing old_string or new_string in edit operation")
		return "", os.ErrNotExist
	}

	// Read original file to extract context around the edit
	//nolint:gosec // filePath is from Claude Code tool context, not user input
	originalContent, err := os.ReadFile(filePath)
	if err != nil {
		log.Debug("failed to read file for edit validation", "file", filePath, "error", err)
		return "", err
	}

	originalStr := string(originalContent)

	// Extract fragment with context lines around the edit
	fragment := ExtractEditFragment(
		originalStr,
		oldStr,
		newStr,
		v.getContextLines(),
		log,
	)
	if fragment == "" {
		log.Debug("could not extract edit fragment, skipping validation")
		return "", os.ErrNotExist
	}

	fragmentLineCount := len(strings.Split(fragment, "\n"))
	log.Debug("validating edit fragment with context",
		"fragment_lines", fragmentLineCount,
	)

	return fragment, nil
}

// formatRuffOutput formats ruff findings into human-readable text.
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

		return "Ruff validation failed\n\n" + strings.Join(cleanLines, "\n") +
			"\n\nFix these issues before committing."
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

	return "Ruff validation failed\n\n" + strings.Join(lines, "\n") +
		"\n\nFix these issues before committing."
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
