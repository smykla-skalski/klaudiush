package linters

//go:generate mockgen -source=markdownlint.go -destination=markdownlint_mock.go -package=linters

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"

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

	// defaultFragmentContextSize is the default number of lines before/after error to show
	defaultFragmentContextSize = 2
)

// ErrMarkdownCustomRules indicates custom markdown rules found issues
var ErrMarkdownCustomRules = errors.New("custom markdown rules validation failed")

// ErrMarkdownlintFailed indicates markdownlint validation failed
var ErrMarkdownlintFailed = errors.New("markdownlint validation failed")

// ErrNoRulesConfigured indicates no markdownlint rules were configured
var ErrNoRulesConfigured = errors.New("no rules configured")

// MarkdownLinter validates Markdown files using markdownlint
type MarkdownLinter interface {
	LintWithPath(
		ctx context.Context,
		content string,
		initialState *validators.MarkdownState,
		originalPath string,
	) *LintResult
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

// NewMarkdownLinterWithDeps creates a new RealMarkdownLinter with injected dependencies
func NewMarkdownLinterWithDeps(
	runner execpkg.CommandRunner,
	toolChecker execpkg.ToolChecker,
	tempMgr execpkg.TempFileManager,
	cfg *config.MarkdownValidatorConfig,
) *RealMarkdownLinter {
	return &RealMarkdownLinter{
		runner:      runner,
		toolChecker: toolChecker,
		config:      cfg,
		tempMgr:     tempMgr,
	}
}

// LintWithPath validates Markdown content with an optional original file path for error reporting.
func (l *RealMarkdownLinter) LintWithPath(
	ctx context.Context,
	content string,
	initialState *validators.MarkdownState,
	originalPath string,
) *LintResult {
	return l.lintInternal(ctx, content, initialState, originalPath)
}

// Lint validates Markdown content using markdownlint (if enabled and available)
// and/or custom rules
func (l *RealMarkdownLinter) Lint(
	ctx context.Context,
	content string,
	initialState *validators.MarkdownState,
) *LintResult {
	return l.lintInternal(ctx, content, initialState, "")
}

// lintInternal is the internal implementation shared by Lint and LintWithPath.
func (l *RealMarkdownLinter) lintInternal(
	ctx context.Context,
	content string,
	initialState *validators.MarkdownState,
	originalPath string,
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
		markdownlintResult := l.runMarkdownlint(ctx, content, initialState, originalPath)
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

// IsMarkdownlintCli2 determines if the tool path refers to markdownlint-cli2 or markdownlint-cli.
// Uses filepath.Base to avoid false positives from custom wrapper paths.
func IsMarkdownlintCli2(toolPath string) bool {
	baseName := filepath.Base(toolPath)

	return strings.Contains(baseName, "markdownlint-cli2")
}

// findMarkdownlintTool locates the markdownlint binary, preferring markdownlint-cli2.
func (l *RealMarkdownLinter) findMarkdownlintTool() string {
	if l.config != nil && l.config.MarkdownlintPath != "" {
		return l.config.MarkdownlintPath
	}

	return l.toolChecker.FindTool("markdownlint-cli2", "markdownlint")
}

// buildConfigArgs creates the config arguments for markdownlint command.
// disableMD041: disable first-line-heading rule (fragment doesn't start at line 0)
// disableMD047: disable single-trailing-newline rule (fragment doesn't reach EOF)
func (l *RealMarkdownLinter) buildConfigArgs(
	markdownlintPath string,
	disableMD041, disableMD047 bool,
) ([]string, func(), error) {
	args := []string{}
	noopCleanup := func() {}

	hasCustomConfig := l.config != nil && l.config.MarkdownlintConfig != ""
	hasCustomRules := l.config != nil && len(l.config.MarkdownlintRules) > 0

	// Need fragment config if any fragment-specific rule needs disabling
	needsFragmentConfig := disableMD041 || disableMD047

	switch {
	case hasCustomConfig:
		return append(args, "--config", l.config.MarkdownlintConfig), noopCleanup, nil
	case hasCustomRules:
		configPath, cleanup, err := l.createTempConfig(
			markdownlintPath,
			disableMD041,
			disableMD047,
		)
		if err != nil {
			return nil, nil, err
		}

		return append(args, "--config", configPath), cleanup, nil
	case needsFragmentConfig:
		configPath, cleanup, err := l.createFragmentConfig(markdownlintPath, disableMD047)
		if err != nil {
			return nil, nil, err
		}

		return append(args, "--config", configPath), cleanup, nil
	}

	return args, noopCleanup, nil
}

// ProcessMarkdownlintOutput processes the output from markdownlint execution.
// isFragment indicates whether we're validating a fragment (Edit operation) vs full content (Write).
func ProcessMarkdownlintOutput(
	result *execpkg.CommandResult,
	tempFile string,
	preambleLines int,
	fragmentStartLine int,
	isCli2 bool,
	displayPath string,
	fragmentContent string,
	isFragment bool,
) *LintResult {
	if result.ExitCode == 0 {
		return &LintResult{
			Success: true,
			RawOut:  "",
		}
	}

	output := result.Stdout + result.Stderr

	// For fragments, use <fragment> path to avoid misleading line numbers
	pathToUse := displayPath

	if isFragment {
		pathToUse = "<fragment>"
	}

	// Replace temp file path with display path
	output = replaceTempFilePath(output, tempFile, pathToUse)

	// Filter out markdownlint-cli2 status messages
	if isCli2 {
		output = filterStatusMessages(output)
	}

	// For fragments, enhance with actual problematic lines
	if isFragment && fragmentContent != "" {
		output = enhanceFragmentErrors(output, fragmentContent, preambleLines)
	} else if preambleLines > 0 {
		output = adjustLineNumbers(output, preambleLines, fragmentStartLine)
	}

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

// runMarkdownlint runs markdownlint-cli2 (or markdownlint-cli) on the content
func (l *RealMarkdownLinter) runMarkdownlint(
	ctx context.Context,
	content string,
	initialState *validators.MarkdownState,
	originalPath string,
) *LintResult {
	markdownlintPath := l.findMarkdownlintTool()
	if markdownlintPath == "" {
		return &LintResult{
			Success: true, // Don't fail if tool not available
			RawOut:  "",
		}
	}

	preamble, preambleLines := validators.GeneratePreamble(initialState)
	contentToLint := preamble + content

	tempFile, cleanup, err := l.tempMgr.Create("markdownlint-*.md", contentToLint)
	if err != nil {
		return &LintResult{
			Success: false,
			RawOut:  fmt.Sprintf("Failed to create temp file: %v", err),
			Err:     err,
		}
	}
	defer cleanup()

	// Determine if we're validating a fragment (Edit operation) vs full content (Write)
	// initialState is only set for Edit operations
	isFragment := initialState != nil

	// MD041 (first-line-heading) should only be disabled if fragment doesn't start at line 0
	// because if it starts at line 0, the fragment DOES include the first line
	disableMD041 := isFragment && initialState.StartLine > 0

	// MD047 (single-trailing-newline) should be disabled for any fragment that doesn't reach EOF
	disableMD047 := isFragment && !initialState.EndsAtEOF

	configArgs, cleanupConfig, err := l.buildConfigArgs(
		markdownlintPath,
		disableMD041,
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

	args := configArgs
	args = append(args, tempFile)
	result := l.runner.Run(ctx, markdownlintPath, args...)

	isCli2 := IsMarkdownlintCli2(markdownlintPath)

	// Use original file path if provided, otherwise use <file>
	displayPath := "<file>"
	if originalPath != "" {
		displayPath = originalPath
	}

	// Extract fragment start line if available (0 if fragment starts at beginning)
	fragmentStartLine := 0
	if initialState != nil {
		fragmentStartLine = initialState.StartLine
	}

	return ProcessMarkdownlintOutput(
		&result,
		tempFile,
		preambleLines,
		fragmentStartLine,
		isCli2,
		displayPath,
		content,
		isFragment,
	)
}

const (
	// minLineNumberMatches is the minimum number of matches expected for line number regex
	minLineNumberMatches = 2
)

// lineNumberRegex matches markdownlint output line numbers in formats like:
// <file>:10:1 or <file>:10
var lineNumberRegex = regexp.MustCompile(`<file>:(\d+)`)

// replaceTempFilePath replaces temp file paths in output with the display path.
// Handles both absolute paths and relative paths with ../ components.
func replaceTempFilePath(output, tempFile, displayPath string) string {
	// Extract just the filename from the temp file path
	filename := filepath.Base(tempFile)

	// Replace patterns like: ../../tmp/filename or ../../../var/folders/.../filename
	// This regex matches optional ../ components, then any path components, ending with the filename
	relativePathPattern := regexp.MustCompile(`(?:\.\./)*[^\s:]*` + regexp.QuoteMeta(filename))
	output = relativePathPattern.ReplaceAllString(output, displayPath)

	// Also replace exact absolute path match (in case it wasn't caught by the regex)
	output = strings.ReplaceAll(output, tempFile, displayPath)

	return output
}

// filterStatusMessages removes markdownlint-cli2 status messages from output.
// Status messages include version info, Finding, Linting, and Summary lines.
func filterStatusMessages(output string) string {
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip status messages from markdownlint-cli2
		if strings.HasPrefix(trimmed, "markdownlint-cli2 ") ||
			strings.HasPrefix(trimmed, "Finding:") ||
			strings.HasPrefix(trimmed, "Linting:") ||
			strings.HasPrefix(trimmed, "Summary:") {
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// enhanceFragmentErrors enhances markdownlint errors for fragments by including
// the actual problematic lines from the fragment content.
func enhanceFragmentErrors(output, fragmentContent string, preambleLines int) string {
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))
	fragmentLines := strings.Split(fragmentContent, "\n")

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

		// Skip errors in preamble
		if lineNum <= preambleLines {
			continue
		}

		// Convert to fragment line number (1-indexed)
		fragmentLineNum := lineNum - preambleLines

		// Remove line number from error message
		errorMsg := lineNumberRegex.ReplaceAllString(line, "<fragment>")
		result = append(result, errorMsg)

		// Add context showing the actual problematic lines
		contextLines := extractContextLines(
			fragmentLines,
			fragmentLineNum-1,
			defaultFragmentContextSize,
		)
		if contextLines != "" {
			result = append(result, "")
			result = append(result, "Problematic section:")
			result = append(result, contextLines)
		}
	}

	return strings.Join(result, "\n")
}

// extractContextLines extracts lines around the specified line number with context.
// lineNum is 0-indexed. Returns empty string if lineNum is out of bounds.
func extractContextLines(lines []string, lineNum, contextSize int) string {
	if lineNum < 0 || lineNum >= len(lines) {
		return ""
	}

	start := max(0, lineNum-contextSize)
	end := min(len(lines), lineNum+contextSize+1)

	contextLines := lines[start:end]

	return strings.Join(contextLines, "\n")
}

// adjustLineNumbers adjusts line numbers in markdownlint output to account for preamble lines
// and fragment offset. It also filters out errors that occur in the preamble itself.
// fragmentStartLine is 0-indexed and represents where the fragment starts in the original file.
func adjustLineNumbers(output string, preambleLines, fragmentStartLine int) string {
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

		// Adjust line number:
		// 1. Subtract preamble lines to get line in fragment content (1-indexed)
		// 2. Add fragment start line (0-indexed) + 1 to get absolute line (1-indexed)
		// Formula: fragmentStartLine + lineNum - preambleLines + 1
		adjustedLine := fragmentStartLine + lineNum - preambleLines + 1
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

	rules := l.prepareRules(disableMD041, disableMD047)
	isCli2 := IsMarkdownlintCli2(toolPath)
	configContent := l.generateConfigContent(rules, isCli2)
	pattern := l.getConfigPattern(isCli2)

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

// GenerateFragmentConfigContent creates config content for fragment linting.
// MD013 (line-length) is always disabled for fragments because context lines
// around edits may legitimately exceed 80 characters.
func GenerateFragmentConfigContent(isCli2, disableMD047 bool) string {
	if isCli2 {
		if disableMD047 {
			return `{
  "config": {
    "MD013": false,
    "MD047": false
  }
}`
		}

		return `{
  "config": {
    "MD013": false
  }
}`
	}

	if disableMD047 {
		return `{
  "MD013": false,
  "MD047": false
}`
	}

	return `{
  "MD013": false
}`
}

// GetFragmentConfigPattern returns the config file pattern for fragments.
func GetFragmentConfigPattern(isCli2 bool) string {
	if isCli2 {
		return "fragment-*.markdownlint-cli2.jsonc"
	}

	return "markdownlint-fragment-*.json"
}

// createFragmentConfig creates a minimal config that disables fragment-incompatible rules
// Note: MD041 (first-line-heading) is no longer disabled because we use a preamble with a header.
// List-related rules (MD007, MD029, MD032) are also no longer disabled because the preamble
// establishes the correct list context.
func (l *RealMarkdownLinter) createFragmentConfig(
	toolPath string,
	disableMD047 bool,
) (string, func(), error) {
	isCli2 := IsMarkdownlintCli2(toolPath)
	configContent := GenerateFragmentConfigContent(isCli2, disableMD047)
	pattern := GetFragmentConfigPattern(isCli2)

	return l.tempMgr.Create(pattern, configContent)
}
