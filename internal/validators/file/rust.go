package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	defaultRustfmtTimeout = 10 * time.Second

	// defaultRustContextLines is the number of lines before/after an edit to include for validation
	defaultRustContextLines = 2

	// defaultRustEdition is the default Rust edition
	defaultRustEdition = "2021"
)

var cargoTomlEditionPattern = regexp.MustCompile(`^\s*edition\s*=\s*"(\d{4})"`)

// RustValidator validates Rust code formatting using rustfmt.
type RustValidator struct {
	validator.BaseValidator
	checker     linters.RustfmtChecker
	config      *config.RustValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewRustValidator creates a new RustValidator.
func NewRustValidator(
	log logger.Logger,
	checker linters.RustfmtChecker,
	cfg *config.RustValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *RustValidator {
	return &RustValidator{
		BaseValidator: *validator.NewBaseValidator("validate-rust", log),
		checker:       checker,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate validates Rust code formatting using rustfmt.
func (v *RustValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating Rust code formatting")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Check if rustfmt is enabled
	if !v.isUseRustfmt() {
		log.Debug("rustfmt is disabled, skipping validation")
		return validator.Pass()
	}

	// Get the file path
	filePath := hookCtx.GetFilePath()
	if filePath == "" {
		log.Debug("no file path provided")
		return validator.Pass()
	}

	// Get content based on operation type
	rustc, err := v.getContent(hookCtx, filePath)
	if err != nil {
		log.Debug("failed to get content", "error", err)
		return validator.Pass()
	}

	// Run rustfmt using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	// Build options with edition detection
	opts := v.buildRustfmtOptions(filePath)
	result := v.checker.CheckWithOptions(lintCtx, rustc.content, opts)

	if result.Success {
		log.Debug("rustfmt passed")
		return validator.Pass()
	}

	log.Debug("rustfmt failed", "output", result.RawOut)

	return validator.FailWithRef(validator.RefRustfmtCheck, v.formatRustfmtOutput(result))
}

// rustContent holds Rust code content and metadata for validation
type rustContent struct {
	content    string
	isFragment bool
}

// getContent extracts Rust code content from context
//
//nolint:dupl // Similar pattern to JavaScriptValidator.getContent, acceptable duplication
func (v *RustValidator) getContent(
	ctx *hook.Context,
	filePath string,
) (*rustContent, error) {
	log := v.Logger()

	// For Edit operations, validate only the changed fragment with context
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		content, err := v.getEditContent(ctx, filePath)
		if err != nil {
			return nil, err
		}

		return &rustContent{content: content, isFragment: true}, nil
	}

	// Get content from context or read from file (Write operation)
	content := ctx.ToolInput.Content
	if content != "" {
		return &rustContent{content: content, isFragment: false}, nil
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

	return &rustContent{content: string(data), isFragment: false}, nil
}

// getEditContent extracts content for Edit operations with context
func (v *RustValidator) getEditContent(
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

// buildRustfmtOptions creates RustfmtOptions with edition detection
func (v *RustValidator) buildRustfmtOptions(filePath string) *linters.RustfmtOptions {
	opts := &linters.RustfmtOptions{}

	// Get edition from config or auto-detect
	edition := ""
	if v.config != nil && v.config.Edition != "" {
		edition = v.config.Edition
	}

	// Auto-detect from Cargo.toml if not configured
	if edition == "" {
		edition = v.autoDetectEdition(filePath)
	}

	// Default to 2021 if still empty
	if edition == "" {
		edition = defaultRustEdition
	}

	opts.Edition = edition

	// Get config path from config
	if v.config != nil && v.config.RustfmtConfig != "" {
		opts.ConfigPath = v.config.RustfmtConfig
	}

	return opts
}

// autoDetectEdition attempts to auto-detect Rust edition from Cargo.toml
func (v *RustValidator) autoDetectEdition(filePath string) string {
	cargoTomlPath := v.findCargoToml(filePath)
	if cargoTomlPath == "" {
		return ""
	}

	edition, err := v.parseCargoToml(cargoTomlPath)
	if err != nil {
		return ""
	}

	if edition != "" {
		v.Logger().Debug(
			"auto-detected Rust edition",
			"edition", edition,
			"cargo_toml", cargoTomlPath,
		)
	}

	return edition
}

// findCargoToml walks up the directory tree to find Cargo.toml
func (*RustValidator) findCargoToml(startPath string) string {
	dir := filepath.Dir(startPath)

	// Walk up to 10 levels max
	for range 10 {
		cargoTomlPath := filepath.Join(dir, "Cargo.toml")
		if _, err := os.Stat(cargoTomlPath); err == nil {
			return cargoTomlPath
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}

		dir = parent
	}

	return ""
}

// parseCargoToml extracts Rust edition from Cargo.toml
func (*RustValidator) parseCargoToml(cargoTomlPath string) (string, error) {
	data, err := os.ReadFile(cargoTomlPath) //nolint:gosec // cargoTomlPath is from findCargoToml
	if err != nil {
		return "", err
	}

	content := string(data)

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Parse edition directive
		if matches := cargoTomlEditionPattern.FindStringSubmatch(line); len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", nil
}

// formatRustfmtOutput formats rustfmt output into human-readable text.
func (*RustValidator) formatRustfmtOutput(result *linters.LintResult) string {
	// Clean up the output - remove empty lines
	lines := strings.Split(result.RawOut, "\n")

	var cleanLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	if len(cleanLines) == 0 {
		return "Rust code formatting issues detected"
	}

	return fmt.Sprintf(
		"Rust code formatting issues detected\n\n%s\n\nRun 'rustfmt <file>' to auto-fix.",
		strings.Join(cleanLines, "\n"),
	)
}

// getTimeout returns the configured timeout for rustfmt operations.
func (v *RustValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultRustfmtTimeout
}

// getContextLines returns the configured number of context lines for edit validation.
func (v *RustValidator) getContextLines() int {
	if v.config != nil && v.config.ContextLines != nil {
		return *v.config.ContextLines
	}

	return defaultRustContextLines
}

// Category returns the validator category for parallel execution.
// RustValidator uses CategoryIO because it invokes rustfmt.
func (*RustValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}

// isUseRustfmt returns whether rustfmt integration is enabled.
func (v *RustValidator) isUseRustfmt() bool {
	if v.config != nil && v.config.UseRustfmt != nil {
		return *v.config.UseRustfmt
	}

	return true
}
