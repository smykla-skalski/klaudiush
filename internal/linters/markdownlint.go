package linters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/validators"
	"github.com/smykla-labs/klaudiush/pkg/config"
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

	// Run custom markdown analysis (built-in rules)
	analysisResult := validators.AnalyzeMarkdown(content, initialState)
	allWarnings = append(allWarnings, analysisResult.Warnings...)

	// Run markdownlint-cli if enabled and available
	if l.shouldUseMarkdownlint() {
		markdownlintResult := l.runMarkdownlint(ctx, content)
		if !markdownlintResult.Success {
			allWarnings = append(allWarnings, markdownlintResult.RawOut)
		}
	}

	if len(allWarnings) > 0 {
		output := strings.Join(allWarnings, "\n")

		return &LintResult{
			Success:  false,
			RawOut:   output,
			Findings: []LintFinding{},
			Err:      ErrMarkdownCustomRules,
		}
	}

	return &LintResult{
		Success:  true,
		RawOut:   "",
		Findings: []LintFinding{},
		Err:      nil,
	}
}

// shouldUseMarkdownlint determines if markdownlint-cli should be used
func (l *RealMarkdownLinter) shouldUseMarkdownlint() bool {
	if l.config == nil || l.config.UseMarkdownlint == nil {
		return false // Default: disabled (external tool)
	}

	return *l.config.UseMarkdownlint
}

// runMarkdownlint runs markdownlint-cli on the content
func (l *RealMarkdownLinter) runMarkdownlint(ctx context.Context, content string) *LintResult {
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

	// Add config file if specified
	if l.config != nil && l.config.MarkdownlintConfig != "" {
		args = append(args, "--config", l.config.MarkdownlintConfig)
	} else if l.config != nil && len(l.config.MarkdownlintRules) > 0 {
		// Create temporary config file from rules map
		configPath, cleanupConfig, err := l.createTempConfig()
		if err != nil {
			return &LintResult{
				Success: false,
				RawOut:  fmt.Sprintf("Failed to create markdownlint config: %v", err),
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

	return &LintResult{
		Success: false,
		RawOut:  strings.TrimSpace(output),
		Err:     ErrMarkdownlintFailed,
	}
}

// createTempConfig creates a temporary markdownlint config file from the rules map
func (l *RealMarkdownLinter) createTempConfig() (string, func(), error) {
	if l.config == nil || len(l.config.MarkdownlintRules) == 0 {
		return "", nil, ErrNoRulesConfigured
	}

	// Create JSON config for markdownlint
	// Format: { "rule-name": true/false, ... }
	// Preallocate slice with capacity for rules + open/close braces
	configLines := make([]string, 0, len(l.config.MarkdownlintRules)+configJSONBrackets)

	configLines = append(configLines, "{")

	idx := 0
	totalRules := len(l.config.MarkdownlintRules)

	for rule, enabled := range l.config.MarkdownlintRules {
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
