package linters

//go:generate mockgen -source=markdownlint.go -destination=markdownlint_mock.go -package=linters

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/validators"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/mdtable"
)

const (
	// configJSONBrackets is the number of brackets in JSON config (opening and closing)
	configJSONBrackets = 2
)

// ErrMarkdownCustomRules indicates custom markdown rules found issues
var ErrMarkdownCustomRules = errors.New("custom markdown rules validation failed")

// ErrMarkdownlintFailed indicates markdownlint-cli validation failed
var ErrMarkdownlintFailed = errors.New("markdownlint validation failed")

// ErrNoRulesConfigured indicates no markdownlint rules were configured
var ErrNoRulesConfigured = errors.New("no rules configured")

// MarkdownLinter validates Markdown files using markdownlint
type MarkdownLinter interface {
	Lint(ctx context.Context, content string, initialState *validators.MarkdownState) *LintResult
}

// RealMarkdownLinter implements MarkdownLinter using markdownlint-cli and/or custom rules
type RealMarkdownLinter struct {
	runner      execpkg.CommandRunner
	toolChecker execpkg.ToolChecker
	config      *config.MarkdownValidatorConfig
	tempMgr     execpkg.TempFileManager
}

// NewMarkdownLinter creates a new RealMarkdownLinter
func NewMarkdownLinter(runner execpkg.CommandRunner) *RealMarkdownLinter {
	return &RealMarkdownLinter{
		runner:      runner,
		toolChecker: execpkg.NewToolChecker(),
		config:      nil,
		tempMgr:     execpkg.NewTempFileManager(),
	}
}

// NewMarkdownLinterWithConfig creates a new RealMarkdownLinter with configuration
func NewMarkdownLinterWithConfig(
	runner execpkg.CommandRunner,
	cfg *config.MarkdownValidatorConfig,
) *RealMarkdownLinter {
	return &RealMarkdownLinter{
		runner:      runner,
		toolChecker: execpkg.NewToolChecker(),
		config:      cfg,
		tempMgr:     execpkg.NewTempFileManager(),
	}
}

// Lint validates Markdown content using markdownlint-cli (if enabled and available)
// and/or custom rules
func (l *RealMarkdownLinter) Lint(
	ctx context.Context,
	content string,
	initialState *validators.MarkdownState,
) *LintResult {
	var allWarnings []string

	tableSuggested := make(map[int]string)

	// Run custom markdown analysis (built-in rules)
	analysisResult := validators.AnalyzeMarkdown(
		content,
		initialState,
		validators.AnalysisOptions{
			CheckTableFormatting: l.isTableFormattingEnabled(),
			TableWidthMode:       l.getTableWidthMode(),
		},
	)
	allWarnings = append(allWarnings, analysisResult.Warnings...)

	// Copy table suggestions from analysis result if table formatting is enabled
	if l.isTableFormattingEnabled() {
		maps.Copy(tableSuggested, analysisResult.TableSuggested)
	}

	// Run markdownlint-cli if enabled and available
	if l.shouldUseMarkdownlint() {
		markdownlintResult := l.runMarkdownlint(ctx, content, initialState)
		if !markdownlintResult.Success {
			allWarnings = append(allWarnings, markdownlintResult.RawOut)
		}
	}

	if len(allWarnings) > 0 {
		output := strings.Join(allWarnings, "\n")

		return &LintResult{
			Success:        false,
			RawOut:         output,
			Findings:       []LintFinding{},
			Err:            ErrMarkdownCustomRules,
			TableSuggested: tableSuggested,
		}
	}

	return &LintResult{
		Success:        true,
		RawOut:         "",
		Findings:       []LintFinding{},
		Err:            nil,
		TableSuggested: nil,
	}
}

// shouldUseMarkdownlint determines if markdownlint-cli should be used
func (l *RealMarkdownLinter) shouldUseMarkdownlint() bool {
	if l.config == nil || l.config.UseMarkdownlint == nil {
		return false // Default: disabled (external tool)
	}

	return *l.config.UseMarkdownlint
}

// isTableFormattingEnabled determines if table formatting validation is enabled
func (l *RealMarkdownLinter) isTableFormattingEnabled() bool {
	if l.config == nil || l.config.TableFormatting == nil {
		return true // Default: enabled
	}

	return *l.config.TableFormatting
}

// getTableWidthMode returns the configured table width calculation mode.
func (l *RealMarkdownLinter) getTableWidthMode() mdtable.WidthMode {
	if l.config == nil || l.config.TableFormattingMode == "" {
		return mdtable.WidthModeDisplay // Default: display width
	}

	switch l.config.TableFormattingMode {
	case "byte_width":
		return mdtable.WidthModeByte
	default:
		return mdtable.WidthModeDisplay
	}
}

// runMarkdownlint runs markdownlint-cli on the content
func (l *RealMarkdownLinter) runMarkdownlint(
	ctx context.Context,
	content string,
	initialState *validators.MarkdownState,
) *LintResult {
	// Find markdownlint binary
	markdownlintPath := "markdownlint"
	if l.config != nil && l.config.MarkdownlintPath != "" {
		markdownlintPath = l.config.MarkdownlintPath
	}

	if !l.toolChecker.IsAvailable(markdownlintPath) {
		return &LintResult{
			Success: true, // Don't fail if tool not available
			RawOut:  "",
		}
	}

	// Write content to temp file
	tempFile, cleanup, err := l.tempMgr.Create("markdownlint-*.md", content)
	if err != nil {
		return &LintResult{
			Success: false,
			RawOut:  fmt.Sprintf("Failed to create temp file: %v", err),
			Err:     err,
		}
	}
	defer cleanup()

	// Build markdownlint command
	args := []string{}

	// Determine if we need to disable MD041 for fragments
	// If initialState is provided and StartLine > 0, this is a fragment
	needsFragmentConfig := initialState != nil && initialState.StartLine > 0

	// MD047 should only be disabled if fragment doesn't reach end of file
	disableMD047 := needsFragmentConfig && !initialState.EndsAtEOF

	// Add config file based on configuration and fragment status
	hasCustomConfig := l.config != nil && l.config.MarkdownlintConfig != ""
	hasCustomRules := l.config != nil && len(l.config.MarkdownlintRules) > 0

	switch {
	case hasCustomConfig:
		args = append(args, "--config", l.config.MarkdownlintConfig)
	case hasCustomRules:
		// Create temporary config file from rules map
		configPath, cleanupConfig, err := l.createTempConfig(needsFragmentConfig, disableMD047)
		if err != nil {
			return &LintResult{
				Success: false,
				RawOut:  fmt.Sprintf("Failed to create markdownlint config: %v", err),
				Err:     err,
			}
		}
		defer cleanupConfig()

		args = append(args, "--config", configPath)
	case needsFragmentConfig:
		// No custom config, but we need to disable fragment-incompatible rules
		configPath, cleanupConfig, err := l.createFragmentConfig(disableMD047)
		if err != nil {
			return &LintResult{
				Success: false,
				RawOut:  fmt.Sprintf("Failed to create fragment config: %v", err),
				Err:     err,
			}
		}
		defer cleanupConfig()

		args = append(args, "--config", configPath)
	}

	args = append(args, tempFile)

	// Run markdownlint
	result := l.runner.Run(ctx, markdownlintPath, args...)

	if result.ExitCode == 0 {
		return &LintResult{
			Success: true,
			RawOut:  "",
		}
	}

	// Combine stdout and stderr for error messages
	output := result.Stdout + result.Stderr

	// Clean up temp file path from output
	output = strings.ReplaceAll(output, tempFile, "<file>")

	return &LintResult{
		Success: false,
		RawOut:  strings.TrimSpace(output),
		Err:     ErrMarkdownlintFailed,
	}
}

// createTempConfig creates a temporary markdownlint config file from the rules map
func (l *RealMarkdownLinter) createTempConfig(
	disableMD041, disableMD047 bool,
) (string, func(), error) {
	if l.config == nil || len(l.config.MarkdownlintRules) == 0 {
		return "", nil, ErrNoRulesConfigured
	}

	// Create a copy of the rules map to avoid modifying the original
	rules := make(map[string]bool, len(l.config.MarkdownlintRules))
	maps.Copy(rules, l.config.MarkdownlintRules)

	// Disable fragment-incompatible rules (unless explicitly enabled in config)
	if disableMD041 {
		if _, exists := rules["MD041"]; !exists {
			rules["MD041"] = false // first-line-heading
		}
	}

	if disableMD047 {
		if _, exists := rules["MD047"]; !exists {
			rules["MD047"] = false // single-trailing-newline
		}
	}

	// Create JSON config for markdownlint
	// Format: { "rule-name": true/false, ... }
	// Preallocate slice with capacity for rules + open/close braces
	configLines := make([]string, 0, len(rules)+configJSONBrackets)

	configLines = append(configLines, "{")

	idx := 0
	totalRules := len(rules)

	for rule, enabled := range rules {
		enabledStr := "true"
		if !enabled {
			enabledStr = "false"
		}

		line := fmt.Sprintf(`  "%s": %s`, rule, enabledStr)

		// Add comma after all rules except the last one
		if idx < totalRules-1 {
			line += ","
		}

		configLines = append(configLines, line)
		idx++
	}

	configLines = append(configLines, "}")
	configContent := strings.Join(configLines, "\n")

	// Use TempFileManager to create temp file
	return l.tempMgr.Create("markdownlint-config-*.json", configContent)
}

// createFragmentConfig creates a minimal config that disables fragment-incompatible rules
func (l *RealMarkdownLinter) createFragmentConfig(disableMD047 bool) (string, func(), error) {
	configContent := `{
  "MD041": false`

	if disableMD047 {
		configContent += `,
  "MD047": false`
	}

	configContent += `
}`

	return l.tempMgr.Create("markdownlint-fragment-*.json", configContent)
}
