package file

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	sc, err := v.getContent(hookCtx, filePath)
	if err != nil {
		log.Debug("failed to get content", "error", err)
		return validator.Pass()
	}

	// Skip Fish scripts
	if v.isFishScript(filePath, sc.content) {
		log.Debug("skipping Fish script", "file", filePath)
		return validator.Pass()
	}

	// Run shellcheck using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	// Build exclude codes from config and fragment-specific excludes
	opts := v.buildShellCheckOptions(sc.isFragment)
	result := v.checker.CheckWithOptions(lintCtx, sc.content, opts)

	if result.Success {
		log.Debug("shellcheck passed")
		return validator.Pass()
	}

	log.Debug("shellcheck failed", "output", result.RawOut)

	return validator.FailWithRef(validator.RefShellcheck, v.formatShellCheckOutput(result.RawOut))
}

// shellContent holds shell script content and metadata for validation
type shellContent struct {
	content    string
	isFragment bool
}

// getContent extracts shell script content from context
func (v *ShellScriptValidator) getContent(
	ctx *hook.Context,
	filePath string,
) (*shellContent, error) {
	log := v.Logger()

	// For Edit operations, validate only the changed fragment with context
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		content, err := v.getEditContent(ctx, filePath)
		if err != nil {
			return nil, err
		}

		return &shellContent{content: content, isFragment: true}, nil
	}

	// Get content from context or read from file (Write operation)
	content := ctx.ToolInput.Content
	if content != "" {
		return &shellContent{content: content, isFragment: false}, nil
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

	return &shellContent{content: string(data), isFragment: false}, nil
}

// getEditContent extracts content for Edit operations with context
func (v *ShellScriptValidator) getEditContent(
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

	// Calculate fragment start line to determine if shebang needs to be prepended
	fragmentStartLine := getFragmentStartLine(originalStr, oldStr, v.getContextLines())

	// If fragment doesn't start at line 0, prepend shebang or shell directive
	// to avoid SC2148 (unknown shell) errors
	if fragmentStartLine > 0 {
		fragment = v.prependShellDirective(originalStr, fragment, log)
	}

	fragmentLineCount := len(strings.Split(fragment, "\n"))
	log.Debug("validating edit fragment with context",
		"fragment_lines", fragmentLineCount,
		"fragment_start_line", fragmentStartLine,
	)

	return fragment, nil
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
	firstLine := content
	if idx := strings.Index(content, "\n"); idx != -1 {
		firstLine = content[:idx]
	}

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
	// Clean up the output - remove empty lines
	lines := strings.Split(output, "\n")

	var cleanLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return "Shellcheck validation failed\n\n" + strings.Join(
		cleanLines,
		"\n",
	) + "\n\nFix these issues before committing."
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

	if len(excludes) == 0 {
		return nil
	}

	return &linters.ShellCheckOptions{ExcludeCodes: excludes}
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
