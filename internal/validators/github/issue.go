// Package github provides validators for GitHub CLI operations.
package github

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

const (
	ghCommand        = "gh"
	issueSubcommand  = "issue"
	createOperation  = "create"
	minGHIssueCreate = 2

	defaultIssueTimeout = 10 * time.Second

	// defaultIssueHeadingLevel is the default heading level context for issues.
	// Issues often start with ### headings (level 3), so we set level 2 as context.
	defaultIssueHeadingLevel = 2
)

var (
	// Regex patterns for extracting issue metadata from gh command.
	issueTitleRegex       = regexp.MustCompile(`--title\s+"([^"]+)"`)
	issueTitleSingleRegex = regexp.MustCompile(`--title\s+'([^']+)'`)
	issueBodyRegex        = regexp.MustCompile(`--body\s+"([^"]+)"`)
	issueBodySingleRegex  = regexp.MustCompile(`--body\s+'([^']+)'`)
	issueBodyFileRegex    = regexp.MustCompile(`--body-file\s+"([^"]+)"`)
	issueBodyFileSingle   = regexp.MustCompile(`--body-file\s+'([^']+)'`)
	issueBodyFileUnquoted = regexp.MustCompile(`--body-file\s+([^\s]+)`)
	heredocRegex          = regexp.MustCompile(`<<'?EOF'?\s*\n((?s:.+?))\nEOF`)
)

// IssueValidator validates gh issue create commands for markdown body formatting.
type IssueValidator struct {
	validator.BaseValidator
	config      *config.IssueValidatorConfig
	linter      linters.MarkdownLinter
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewIssueValidator creates a new IssueValidator instance.
func NewIssueValidator(
	cfg *config.IssueValidatorConfig,
	linter linters.MarkdownLinter,
	log logger.Logger,
	ruleAdapter *rules.RuleValidatorAdapter,
) *IssueValidator {
	return &IssueValidator{
		BaseValidator: *validator.NewBaseValidator("validate-issue", log),
		config:        cfg,
		linter:        linter,
		ruleAdapter:   ruleAdapter,
	}
}

// getTimeout returns the timeout for markdown linting operations.
func (v *IssueValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultIssueTimeout
}

// getMarkdownDisabledRules returns the list of markdownlint rules to disable.
func (v *IssueValidator) getMarkdownDisabledRules() []string {
	if v.config != nil && len(v.config.MarkdownDisabledRules) > 0 {
		return v.config.MarkdownDisabledRules
	}

	// Default: disable line length, bare URLs, first line heading requirement, and trailing newline.
	// Issues often start with ### headings and don't need trailing newlines.
	return []string{"MD013", "MD034", "MD041", "MD047"}
}

// isRequireBody returns whether issue body is required.
func (v *IssueValidator) isRequireBody() bool {
	if v.config != nil && v.config.RequireBody != nil {
		return *v.config.RequireBody
	}

	return false // default: not required for issues
}

// Validate checks gh issue create command for proper markdown formatting in body.
func (v *IssueValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running issue validation")

	// Check rules first if rule adapter is configured.
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Parse the command.
	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(hookCtx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)

		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Find gh issue create commands.
	for _, cmd := range result.Commands {
		if !v.isGHIssueCreate(&cmd) {
			continue
		}

		// Extract issue metadata from the full command.
		fullCmd := hookCtx.GetCommand()
		issueData := v.extractIssueData(fullCmd)

		// Validate issue.
		return v.validateIssue(ctx, issueData)
	}

	log.Debug("No gh issue create commands found")

	return validator.Pass()
}

// isGHIssueCreate checks if a command is gh issue create.
func (*IssueValidator) isGHIssueCreate(cmd *parser.Command) bool {
	if cmd.Name != ghCommand {
		return false
	}

	if len(cmd.Args) < minGHIssueCreate {
		return false
	}

	return cmd.Args[0] == issueSubcommand && cmd.Args[1] == createOperation
}

// IssueData holds extracted issue metadata.
type IssueData struct {
	Title    string
	Body     string
	BodyFile string
}

// extractIssueData extracts issue title and body from gh command.
func (v *IssueValidator) extractIssueData(command string) IssueData {
	data := IssueData{}

	// Extract title (try double quotes first, then single quotes).
	if matches := issueTitleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Title = matches[1]
	} else if matches := issueTitleSingleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Title = matches[1]
	}

	// Extract body - try heredoc first, then quoted strings.
	if matches := heredocRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Body = matches[1] + "\n"
	} else if matches := issueBodyRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Body = matches[1] + "\n"
	} else if matches := issueBodySingleRegex.FindStringSubmatch(command); len(matches) > 1 {
		data.Body = matches[1] + "\n"
	}

	// Extract body-file path if --body wasn't used.
	if data.Body == "" {
		data.BodyFile = v.extractBodyFilePath(command)
		data.Body = v.loadBodyFromFile(data.BodyFile)
	}

	return data
}

// extractBodyFilePath extracts the --body-file path from the command.
func (*IssueValidator) extractBodyFilePath(command string) string {
	if matches := issueBodyFileRegex.FindStringSubmatch(command); len(matches) > 1 {
		return matches[1]
	}

	if matches := issueBodyFileSingle.FindStringSubmatch(command); len(matches) > 1 {
		return matches[1]
	}

	if matches := issueBodyFileUnquoted.FindStringSubmatch(command); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// loadBodyFromFile reads body content from a file if path is specified.
func (v *IssueValidator) loadBodyFromFile(filePath string) string {
	if filePath == "" {
		return ""
	}

	content, err := v.readBodyFile(filePath)
	if err != nil {
		return ""
	}

	return content
}

// readBodyFile reads the body content from a file.
func (*IssueValidator) readBodyFile(filePath string) (string, error) {
	//nolint:gosec // filePath is from Claude Code tool context
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// validateIssue performs markdown validation on the issue body.
func (v *IssueValidator) validateIssue(ctx context.Context, data IssueData) *validator.Result {
	log := v.Logger()

	// Handle missing body case.
	if data.Body == "" {
		return v.handleMissingBody(log)
	}

	// Validate markdown formatting.
	warnings := v.validateMarkdown(ctx, data.Body)

	return v.buildResult(nil, warnings, data.Title)
}

// handleMissingBody handles the case when issue body is not provided.
func (v *IssueValidator) handleMissingBody(log logger.Logger) *validator.Result {
	if !v.isRequireBody() {
		log.Debug("No issue body provided, skipping validation")

		return validator.Pass()
	}

	return validator.FailWithRef(
		validator.RefGHIssueValidation,
		"Issue body is required - ensure you're using --body or --body-file flag",
	)
}

// validateMarkdown validates the issue body markdown content.
func (v *IssueValidator) validateMarkdown(ctx context.Context, body string) []string {
	var warnings []string

	// Run internal markdown analysis with issue-specific options.
	// For issues, we're more lenient about headings and structure.
	analysisResult := validators.AnalyzeMarkdown(body, &validators.MarkdownState{
		// Don't require starting with level 1 heading (issues often use ### headings).
		LastHeadingLevel: defaultIssueHeadingLevel,
	})
	warnings = append(warnings, analysisResult.Warnings...)

	// Run markdownlint if available and linter is configured.
	if v.linter != nil {
		timeout := v.getTimeout()

		lintCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Pass issue-specific initial state.
		initialState := &validators.MarkdownState{
			// Issues can start with any heading level.
			LastHeadingLevel: 0,
			// Don't require ending with newline.
			EndsAtEOF: false,
		}

		result := v.linter.Lint(lintCtx, body, initialState)
		if !result.Success {
			disabledRules := v.getMarkdownDisabledRules()
			warnings = append(warnings, filterDisabledRules(result.RawOut, disabledRules))
		}
	}

	return warnings
}

// filterDisabledRules removes messages for disabled rules from markdownlint output.
func filterDisabledRules(output string, disabledRules []string) string {
	if len(disabledRules) == 0 {
		return output
	}

	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		shouldKeep := true

		for _, rule := range disabledRules {
			if strings.Contains(line, rule) {
				shouldKeep = false

				break
			}
		}

		if shouldKeep && strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// buildResult builds the final validation result.
func (*IssueValidator) buildResult(
	allErrors, allWarnings []string,
	title string,
) *validator.Result {
	if len(allErrors) > 0 {
		var message strings.Builder

		message.WriteString("Issue validation failed")
		message.WriteString("\n\n")

		for _, err := range allErrors {
			message.WriteString(err)
			message.WriteString("\n")
		}

		if len(allWarnings) > 0 {
			message.WriteString("\nWarnings:\n")

			for _, warn := range allWarnings {
				message.WriteString(warn)
				message.WriteString("\n")
			}
		}

		if title != "" {
			message.WriteString("\nIssue title: ")
			message.WriteString(title)
		}

		return validator.FailWithRef(validator.RefGHIssueValidation, message.String())
	}

	if len(allWarnings) > 0 {
		var message strings.Builder

		message.WriteString("Issue body markdown validation warnings:\n\n")

		for _, warn := range allWarnings {
			message.WriteString(warn)
			message.WriteString("\n")
		}

		return validator.WarnWithRef(validator.RefGHIssueValidation, message.String())
	}

	return validator.Pass()
}

// Category returns the validator category for parallel execution.
// IssueValidator uses CategoryIO because it may invoke markdownlint.
func (*IssueValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
