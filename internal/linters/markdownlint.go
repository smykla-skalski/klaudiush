package linters

//go:generate mockgen -source=markdownlint.go -destination=markdownlint_mock.go -package=linters

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/validators"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/mdtable"
)

const (
	// configJSONBrackets is the number of brackets in JSON config (opening and closing)
	configJSONBrackets = 2
	// configWrapperLines is the number of extra lines for markdownlint-cli2 wrapper
	configWrapperLines = 2
)

// ErrMarkdownCustomRules indicates custom markdown rules found issues
var ErrMarkdownCustomRules = errors.New("custom markdown rules validation failed")

// ErrMarkdownlintFailed indicates markdownlint validation failed
var ErrMarkdownlintFailed = errors.New("markdownlint validation failed")

// ErrNoRulesConfigured indicates no markdownlint rules were configured
var ErrNoRulesConfigured = errors.New("no rules configured")

// MarkdownLinter validates Markdown files using markdownlint
type MarkdownLinter interface {
	Lint(ctx context.Context, content string, initialState *validators.MarkdownState) *LintResult
}

// RealMarkdownLinter implements MarkdownLinter using markdownlint and/or custom rules
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

// Lint validates Markdown content using markdownlint (if enabled and available)
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

	// Run markdownlint if enabled and available
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

// runMarkdownlint runs markdownlint-cli2 (or markdownlint-cli) on the content
//
//nolint:funlen // Complexity justified by dual tool support
func (l *RealMarkdownLinter) runMarkdownlint(
	ctx context.Context,
	content string,
	initialState *validators.MarkdownState,
) *LintResult {
	// Find markdownlint binary (prefer markdownlint-cli2, fallback to markdownlint-cli)
	var markdownlintPath string
	if l.config != nil && l.config.MarkdownlintPath != "" {
		markdownlintPath = l.config.MarkdownlintPath
	} else {
		// Try to find available tool: markdownlint-cli2 or markdownlint
		found := l.toolChecker.FindTool("markdownlint-cli2", "markdownlint")
		if found == "" {
			return &LintResult{
				Success: true, // Don't fail if tool not available
				RawOut:  "",
			}
		}

		markdownlintPath = found
	}

	// Generate preamble if we have list context
	// This establishes the correct markdown state for markdownlint
	preamble, preambleLines := validators.GeneratePreamble(initialState)
	contentToLint := preamble + content

	// Write content to temp file
	tempFile, cleanup, err := l.tempMgr.Create("markdownlint-*.md", contentToLint)
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

	// Determine if we need to disable MD047 for fragments
	// If initialState is provided and StartLine > 0, this is a fragment
	needsFragmentConfig := initialState != nil && initialState.StartLine > 0

	// MD047 should always be disabled for fragments (markdownlint-cli2 is stricter)
	disableMD047 := needsFragmentConfig

	// Add config file based on configuration and fragment status
	hasCustomConfig := l.config != nil && l.config.MarkdownlintConfig != ""
	hasCustomRules := l.config != nil && len(l.config.MarkdownlintRules) > 0

	switch {
	case hasCustomConfig:
		args = append(args, "--config", l.config.MarkdownlintConfig)
	case hasCustomRules:
		// Create temporary config file from rules map
		configPath, cleanupConfig, err := l.createTempConfig(
			markdownlintPath,
			needsFragmentConfig,
			disableMD047,
		)
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
		configPath, cleanupConfig, err := l.createFragmentConfig(markdownlintPath, disableMD047)
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

	// Adjust line numbers to account for preamble and filter out preamble errors
	if preambleLines > 0 {
		output = adjustLineNumbers(output, preambleLines)
	}

	// Check if any real errors remain after filtering
	if strings.TrimSpace(output) == "" {
		return &LintResult{
			Success: true,
			RawOut:  "",
		}
	}

	return &LintResult{
		Success: false,
		RawOut:  strings.TrimSpace(output),
		Err:     ErrMarkdownlintFailed,
	}
}

const (
	// minLineNumberMatches is the minimum number of matches expected for line number regex
	minLineNumberMatches = 2
)

// lineNumberRegex matches markdownlint output line numbers in formats like:
// <file>:10:1 or <file>:10
var lineNumberRegex = regexp.MustCompile(`<file>:(\d+)`)

// adjustLineNumbers adjusts line numbers in markdownlint output to account for preamble lines.
// It also filters out errors that occur in the preamble itself.
func adjustLineNumbers(output string, preambleLines int) string {
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Find line number in the output
		matches := lineNumberRegex.FindStringSubmatch(line)
		if len(matches) < minLineNumberMatches {
			// No line number found, keep the line as-is
			result = append(result, line)

			continue
		}

		lineNum := 0
		_, _ = fmt.Sscanf(matches[1], "%d", &lineNum)

		// Skip errors that occur in the preamble
		if lineNum <= preambleLines {
			continue
		}

		// Adjust line number by subtracting preamble lines
		adjustedLine := lineNum - preambleLines
		adjustedOutput := lineNumberRegex.ReplaceAllString(
			line,
			fmt.Sprintf("<file>:%d", adjustedLine),
		)

		result = append(result, adjustedOutput)
	}

	return strings.Join(result, "\n")
}

// createTempConfig creates a temporary markdownlint config file from the rules map
func (l *RealMarkdownLinter) createTempConfig(
	toolPath string,
	disableMD041, disableMD047 bool,
) (string, func(), error) {
	if l.config == nil || len(l.config.MarkdownlintRules) == 0 {
		return "", nil, ErrNoRulesConfigured
	}

	// Create a copy of the rules map and apply fragment rules
	rules := l.prepareRules(disableMD041, disableMD047)

	// Generate config content based on tool type
	isMarkdownlintCli2 := strings.Contains(toolPath, "markdownlint-cli2")
	configContent := l.generateConfigContent(rules, isMarkdownlintCli2)

	// Use appropriate file naming pattern
	pattern := l.getConfigPattern(isMarkdownlintCli2)

	return l.tempMgr.Create(pattern, configContent)
}

// prepareRules creates a copy of rules and applies fragment-specific overrides
func (l *RealMarkdownLinter) prepareRules(disableMD041, disableMD047 bool) map[string]bool {
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

	return rules
}

// generateConfigContent creates JSON config content for the specified tool
func (l *RealMarkdownLinter) generateConfigContent(
	rules map[string]bool,
	isMarkdownlintCli2 bool,
) string {
	if isMarkdownlintCli2 {
		return l.generateCli2Config(rules)
	}

	return l.generateCliConfig(rules)
}

// generateCli2Config creates markdownlint-cli2 format config
func (l *RealMarkdownLinter) generateCli2Config(rules map[string]bool) string {
	configLines := make([]string, 0, len(rules)+configJSONBrackets+configWrapperLines)
	configLines = append(configLines, "{", `  "config": {`)
	configLines = append(configLines, l.formatRules(rules, "    ")...)
	configLines = append(configLines, "  }", "}")

	return strings.Join(configLines, "\n")
}

// generateCliConfig creates markdownlint-cli format config
func (l *RealMarkdownLinter) generateCliConfig(rules map[string]bool) string {
	configLines := make([]string, 0, len(rules)+configJSONBrackets)
	configLines = append(configLines, "{")
	configLines = append(configLines, l.formatRules(rules, "  ")...)
	configLines = append(configLines, "}")

	return strings.Join(configLines, "\n")
}

// formatRules converts rules map to JSON lines with specified indentation
func (*RealMarkdownLinter) formatRules(rules map[string]bool, indent string) []string {
	lines := make([]string, 0, len(rules))
	idx := 0
	totalRules := len(rules)

	for rule, enabled := range rules {
		enabledStr := "true"
		if !enabled {
			enabledStr = "false"
		}

		line := fmt.Sprintf(`%s"%s": %s`, indent, rule, enabledStr)

		if idx < totalRules-1 {
			line += ","
		}

		lines = append(lines, line)
		idx++
	}

	return lines
}

// getConfigPattern returns the appropriate temp file pattern for the tool
func (*RealMarkdownLinter) getConfigPattern(isMarkdownlintCli2 bool) string {
	if isMarkdownlintCli2 {
		return "config-*.markdownlint-cli2.jsonc"
	}

	return "markdownlint-config-*.json"
}

// createFragmentConfig creates a minimal config that disables fragment-incompatible rules
// Note: MD041 (first-line-heading) is no longer disabled because we use a preamble with a header.
// List-related rules (MD007, MD029, MD032) are also no longer disabled because the preamble
// establishes the correct list context.
//
//nolint:nestif // Dual tool support requires conditional logic
func (l *RealMarkdownLinter) createFragmentConfig(
	toolPath string,
	disableMD047 bool,
) (string, func(), error) {
	var configContent string

	// markdownlint-cli2 requires rules wrapped in "config" object
	isMarkdownlintCli2 := strings.Contains(toolPath, "markdownlint-cli2")

	if isMarkdownlintCli2 {
		if disableMD047 {
			configContent = `{
  "config": {
    "MD047": false
  }
}`
		} else {
			configContent = `{
  "config": {}
}`
		}
	} else {
		// markdownlint-cli uses flat structure
		if disableMD047 {
			configContent = `{
  "MD047": false
}`
		} else {
			configContent = "{}"
		}
	}

	// Use appropriate naming pattern based on tool
	// markdownlint-cli2 requires prefix.markdownlint-cli2.jsonc pattern (e.g., fragment-123.markdownlint-cli2.jsonc)
	// markdownlint-cli accepts any .json file
	pattern := "markdownlint-fragment-*.json"
	if isMarkdownlintCli2 {
		pattern = "fragment-*.markdownlint-cli2.jsonc"
	}

	return l.tempMgr.Create(pattern, configContent)
}
