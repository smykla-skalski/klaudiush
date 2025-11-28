// Package shell provides validators for shell command operations
package shell

import (
	"context"
	"fmt"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

// BacktickValidator validates against unescaped backticks in double-quoted strings.
type BacktickValidator struct {
	validator.BaseValidator
	config      *config.BacktickValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewBacktickValidator creates a new BacktickValidator instance.
func NewBacktickValidator(
	log logger.Logger,
	cfg *config.BacktickValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *BacktickValidator {
	return &BacktickValidator{
		BaseValidator: *validator.NewBaseValidator("validate-backticks", log),
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate checks for backticks in double-quoted strings for specific commands.
func (v *BacktickValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running backtick validation")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	command := hookCtx.GetCommand()
	if command == "" {
		log.Debug("Empty command, skipping validation")
		return validator.Pass()
	}

	// Parse the command to detect backticks
	bashParser := parser.NewBashParser()

	issues, err := bashParser.FindDoubleQuotedBackticks(command)
	if err != nil {
		log.Debug("Failed to parse command for backtick detection", "error", err)
		return validator.Pass()
	}

	if len(issues) == 0 {
		log.Debug("No backtick issues found")
		return validator.Pass()
	}

	// Parse the command normally to get command structure
	parseResult, err := bashParser.Parse(command)
	if err != nil {
		log.Debug("Failed to parse command structure", "error", err)
		return validator.Pass()
	}

	// Check if any of the commands are ones we care about
	relevantIssues := v.filterRelevantIssues(parseResult, issues)
	if len(relevantIssues) == 0 {
		log.Debug("Backticks found but not in relevant commands")
		return validator.Pass()
	}

	// Build error message
	message := v.buildErrorMessage(parseResult, relevantIssues)

	return validator.FailWithRef(
		validator.RefShellBackticks,
		"Command substitution detected in double-quoted strings",
	).AddDetail("help", message)
}

// filterRelevantIssues filters backtick issues to only those in relevant commands.
func (v *BacktickValidator) filterRelevantIssues(
	parseResult *parser.ParseResult,
	issues []parser.BacktickIssue,
) []parser.BacktickIssue {
	var relevant []parser.BacktickIssue

	for _, issue := range issues {
		if v.isRelevantCommand(parseResult, issue) {
			relevant = append(relevant, issue)
		}
	}

	return relevant
}

// isRelevantCommand checks if the issue is in a relevant command and argument.
func (v *BacktickValidator) isRelevantCommand(
	parseResult *parser.ParseResult,
	issue parser.BacktickIssue,
) bool {
	// Find which command this issue belongs to
	for _, cmd := range parseResult.Commands {
		if v.isCommandRelevant(cmd, issue.ArgIndex) {
			return true
		}
	}

	return false
}

// isCommandRelevant checks if a command and argument index match our criteria.
func (v *BacktickValidator) isCommandRelevant(cmd parser.Command, argIndex int) bool {
	switch cmd.Name {
	case "git":
		return v.isGitCommandRelevant(cmd, argIndex)
	case "gh":
		return v.isGhCommandRelevant(cmd, argIndex)
	default:
		return false
	}
}

// isGitCommandRelevant checks if this is a relevant git command.
func (v *BacktickValidator) isGitCommandRelevant(cmd parser.Command, argIndex int) bool {
	if len(cmd.Args) == 0 {
		return false
	}

	subcommand := cmd.Args[0]

	// git commit with -m or --message
	if subcommand == "commit" {
		return v.isMessageArgument(cmd.Args, argIndex)
	}

	return false
}

// isGhCommandRelevant checks if this is a relevant gh command.
func (v *BacktickValidator) isGhCommandRelevant(cmd parser.Command, argIndex int) bool {
	if len(cmd.Args) < 2 { //nolint:mnd // Need at least 2 args for gh subcommand
		return false
	}

	subcommand := cmd.Args[0]
	action := cmd.Args[1]

	// gh pr create with --body or --title
	// gh issue create with --body or --title
	if (subcommand == "pr" || subcommand == "issue") && action == "create" {
		return v.isBodyOrTitleArgument(cmd.Args, argIndex)
	}

	return false
}

// convertArgIndexToCommandArgsIndex converts ArgIndex (from BacktickIssue) to Command.Args index.
// ArgIndex uses call.Args indexing where 0=command name.
// Command.Args uses indexing where 0=first argument after command.
func convertArgIndexToCommandArgsIndex(argIndex int) int {
	return argIndex - 1
}

// isMessageArgument checks if argIndex points to a -m or --message argument value.
func (*BacktickValidator) isMessageArgument(args []string, argIndex int) bool {
	// argIndex includes command name, so adjust for args slice
	// If argIndex is 3 for "git commit -m MSG", then:
	// - 0=git, 1=commit, 2=-m, 3=MSG
	// In args: 0=commit, 1=-m, 2=MSG
	// So we need argIndex-1 in the args slice
	if argIndex <= 1 { // Skip "git" and subcommand
		return false
	}

	argsIdx := convertArgIndexToCommandArgsIndex(argIndex)

	if argsIdx >= len(args) {
		return false
	}

	// Check if previous argument is -m or --message
	if argsIdx > 0 {
		prevArg := args[argsIdx-1]
		if prevArg == "-m" || prevArg == "--message" {
			return true
		}
	}

	// Check for --message=value form
	arg := args[argsIdx]

	return strings.HasPrefix(arg, "--message=")
}

// isBodyOrTitleArgument checks if argIndex points to --body or --title argument value.
func (*BacktickValidator) isBodyOrTitleArgument(args []string, argIndex int) bool {
	if argIndex <= 2 { //nolint:mnd // Need to skip gh, subcommand, action (3 arguments)
		return false
	}

	argsIdx := convertArgIndexToCommandArgsIndex(argIndex)

	if argsIdx >= len(args) {
		return false
	}

	// Check if previous argument is --body or --title
	if argsIdx > 0 {
		prevArg := args[argsIdx-1]
		if prevArg == "--body" || prevArg == "--title" {
			return true
		}
	}

	// Check for --body=value or --title=value form
	arg := args[argsIdx]
	if strings.HasPrefix(arg, "--body=") || strings.HasPrefix(arg, "--title=") {
		return true
	}

	return false
}

// buildErrorMessage creates a helpful error message with suggestions.
func (*BacktickValidator) buildErrorMessage(
	_ *parser.ParseResult,
	issues []parser.BacktickIssue,
) string {
	var sb strings.Builder

	sb.WriteString(
		"Command substitution (backticks or $()) in double-quoted strings can cause unexpected behavior.\n\n",
	)
	sb.WriteString("Found in:\n")

	for _, issue := range issues {
		sb.WriteString(fmt.Sprintf("- Argument at index %d\n", issue.ArgIndex))
	}

	sb.WriteString("\nRecommended solutions:\n\n")
	sb.WriteString("1. Use HEREDOC with single-quoted delimiter (recommended):\n")
	sb.WriteString("   git commit -m \"$(cat <<'EOF'\n")
	sb.WriteString("   Fix bug in `parser` module\n")
	sb.WriteString("   EOF\n")
	sb.WriteString("   )\"\n\n")

	sb.WriteString("2. Use file-based input:\n")
	sb.WriteString("   git commit -F commit-message.txt\n")
	sb.WriteString("   gh pr create --body-file pr-body.md\n")
	sb.WriteString("   gh issue create --body-file issue-body.md\n\n")

	sb.WriteString("3. Escape backticks if intentional:\n")
	sb.WriteString("   git commit -m \"Fix bug in \\`parser\\` module\"\n")

	return sb.String()
}

// Category returns the validator category for parallel execution.
func (*BacktickValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

// Ensure BacktickValidator implements validator.Validator
var _ validator.Validator = (*BacktickValidator)(nil)
