package file

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	defaultShellCheckTimeout = 10 * time.Second

	// defaultShellContextLines is the number of lines before/after an edit to include for validation
	defaultShellContextLines = 2
)

// ShellScriptValidator validates shell scripts using shellcheck.
type ShellScriptValidator struct {
	validator.BaseValidator
	checker     linters.ShellChecker
	config      *config.ShellScriptValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewShellScriptValidator creates a new ShellScriptValidator.
func NewShellScriptValidator(
	log logger.Logger,
	checker linters.ShellChecker,
	cfg *config.ShellScriptValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *ShellScriptValidator {
	return &ShellScriptValidator{
		BaseValidator: *validator.NewBaseValidator("validate-shellscript", log),
		checker:       checker,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// fragmentExcludes are shellcheck codes to exclude when validating fragments.
// These are false positives due to limited context:
// - SC1009: The mentioned syntax error was in... (follow-up to parsing errors)
// - SC1072: Unexpected token (fragment may start mid-statement)
// - SC1073: Couldn't parse this (incomplete syntax in fragment)
// - SC1089: Parsing stopped - keywords not matched (fragment contains partial control structure)
// - SC2034: variable appears unused (may be used elsewhere in the file)
// - SC2154: variable is referenced but not assigned (may be assigned elsewhere)
var fragmentExcludes = []int{1009, 1072, 1073, 1089, 2034, 2154}

// Validate validates shell scripts using shellcheck.
func (v *ShellScriptValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating shell script")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
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

	// Skip Fish scripts
	if v.isFishScript(filePath, ci.Content) {
		log.Debug("skipping Fish script", "file", filePath)
		return validator.Pass()
	}

	// Run shellcheck using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	// Build exclude codes from config and fragment-specific excludes
	opts := v.buildShellCheckOptions(ci.IsFragment)
	result := v.checker.CheckWithOptions(lintCtx, ci.Content, opts)

	if result.Success {
		log.Debug("shellcheck passed")
		return validator.Pass()
	}

	log.Debug("shellcheck failed", "output", result.RawOut)

	return validator.FailWithRef(validator.RefShellcheck, v.formatShellCheckOutput(result.RawOut))
}

// extractContent extracts shell script content from the hook context, with
// shell-specific post-processing for edit fragments (prepending shell directive).
func (v *ShellScriptValidator) extractContent(
	hookCtx *hook.Context,
	filePath string,
) (*ContentInfo, error) {
	log := v.Logger()
	ci, err := NewContentExtractor(log, v.getContextLines()).Extract(hookCtx, filePath)

	if err != nil {
		return nil, err
	}

	// Shell-specific: for edit fragments, prepend a shellcheck directive if the
	// fragment doesn't start at line 0 (to avoid SC2148 unknown shell errors).
	if ci.IsFragment && hookCtx.ToolName == hook.ToolTypeEdit {
		//nolint:gosec // filePath is from Claude Code tool context, not user input
		original, readErr := os.ReadFile(filePath)
		if readErr == nil {
			originalStr := string(original)
			oldStr := hookCtx.ToolInput.OldString
			startLine := getFragmentStartLine(originalStr, oldStr, v.getContextLines())

			if startLine > 0 {
				ci.Content = v.prependShellDirective(originalStr, ci.Content, log)
			}
		}
	}

	return ci, nil
}

// prependShellDirective prepends a shellcheck shell directive to the fragment
// based on the shebang from the original file. This ensures shellcheck knows
// which shell to use when validating fragments that don't include line 1.
func (*ShellScriptValidator) prependShellDirective(
	originalContent string,
	fragment string,
	log logger.Logger,
) string {
	shell := detectShellFromShebang(originalContent)
	if shell == "" {
		log.Debug("no shell detected from shebang, fragment may fail validation")
		return fragment
	}

	// Use shellcheck directive instead of shebang to avoid confusing line numbers
	// https://www.shellcheck.net/wiki/Directive
	directive := "# shellcheck shell=" + shell + "\n"
	log.Debug("prepending shell directive for fragment", "shell", shell)

	return directive + fragment
}

// detectShellFromShebang extracts the shell name from a shebang line.
// Returns empty string if no shebang is found or shell cannot be determined.
func detectShellFromShebang(content string) string {
	if !strings.HasPrefix(content, "#!") {
		return ""
	}

	// Get first line
	firstLine, _, _ := strings.Cut(content, "\n")

	// Common shebang patterns
	switch {
	case strings.Contains(firstLine, "/bash"):
		return "bash"
	case strings.Contains(firstLine, "/sh"):
		return "sh"
	case strings.Contains(firstLine, "/zsh"):
		return "zsh"
	case strings.Contains(firstLine, "/ksh"):
		return "ksh"
	case strings.Contains(firstLine, "/dash"):
		return "dash"
	case strings.HasSuffix(firstLine, "bash"):
		return "bash"
	case strings.HasSuffix(firstLine, "sh"):
		return "sh"
	}

	return ""
}

// isFishScript checks if the script is a Fish shell script.
func (*ShellScriptValidator) isFishScript(filePath, content string) bool {
	// Check file extension
	if filepath.Ext(filePath) == ".fish" {
		return true
	}

	// Check shebang
	if strings.HasPrefix(content, "#!/usr/bin/env fish") ||
		strings.HasPrefix(content, "#!/usr/bin/fish") ||
		strings.HasPrefix(content, "#!/bin/fish") {
		return true
	}

	return false
}

// formatShellCheckOutput formats shellcheck output for display.
func (*ShellScriptValidator) formatShellCheckOutput(output string) string {
	lines := strings.Split(output, "\n")

	var cleanLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	if len(cleanLines) == 0 {
		return "Shellcheck validation failed"
	}

	return strings.Join(cleanLines, "\n")
}

// buildShellCheckOptions creates ShellCheckOptions with excludes from config and fragment-specific rules.
func (v *ShellScriptValidator) buildShellCheckOptions(isFragment bool) *linters.ShellCheckOptions {
	var excludes []int

	// Add config excludes
	if v.config != nil {
		excludes = append(excludes, parseExcludeRules(v.config.ExcludeRules)...)
	}

	// Add fragment-specific excludes for Edit operations
	if isFragment {
		excludes = append(excludes, fragmentExcludes...)
	}

	return &linters.ShellCheckOptions{
		ExcludeCodes: excludes,
		Severity:     v.getShellcheckSeverity(),
	}
}

// getShellcheckSeverity returns the configured minimum severity level for shellcheck.
// Defaults to "warning" so info/style findings (like SC2016 in GraphQL scripts) don't block.
func (v *ShellScriptValidator) getShellcheckSeverity() string {
	if v.config != nil && v.config.ShellcheckSeverity != "" {
		return v.config.ShellcheckSeverity
	}

	return "warning"
}

// parseExcludeRules converts string rule codes (e.g., "SC1091") to integers.
func parseExcludeRules(rules []string) []int {
	codes := make([]int, 0, len(rules))

	for _, rule := range rules {
		// Strip "SC" prefix if present
		numStr := strings.TrimPrefix(rule, "SC")

		code, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}

		codes = append(codes, code)
	}

	return codes
}

// getTimeout returns the configured timeout for shellcheck operations.
func (v *ShellScriptValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultShellCheckTimeout
}

// getContextLines returns the configured number of context lines for edit validation.
func (v *ShellScriptValidator) getContextLines() int {
	if v.config != nil && v.config.ContextLines != nil {
		return *v.config.ContextLines
	}

	return defaultShellContextLines
}

// Category returns the validator category for parallel execution.
// ShellScriptValidator uses CategoryIO because it invokes shellcheck.
func (*ShellScriptValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
