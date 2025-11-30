package git

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/templates"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

const (
	gitCommand       = "git"
	commitSubcommand = "commit"
	addSubcommand    = "add"
)

var (
	// Commit message flags for inline messages.
	commitMessageFlags = []string{"-m", "--message"}

	// Commit file flags for message from file.
	commitFileFlags = []string{"-F", "--file"}
)

// CommitValidator validates git commit commands and messages
type CommitValidator struct {
	validator.BaseValidator
	gitRunner   GitRunner
	config      *config.CommitValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewCommitValidator creates a new CommitValidator instance
func NewCommitValidator(
	log logger.Logger,
	gitRunner GitRunner,
	cfg *config.CommitValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *CommitValidator {
	if gitRunner == nil {
		gitRunner = NewGitRunner()
	}

	return &CommitValidator{
		BaseValidator: *validator.NewBaseValidator("validate-commit", log),
		gitRunner:     gitRunner,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate checks git commit command and message
func (v *CommitValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running git commit validation")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Parse the command
	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(hookCtx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Check if there's a git add in the same command chain
	hasGitAdd := v.hasGitAddInChain(result.Commands)

	// Find and validate git commit commands
	for _, cmd := range result.Commands {
		if cmd.Name != gitCommand {
			continue
		}

		// Parse git command to get the subcommand (handles global options like -C)
		gitCmd, err := parser.ParseGitCommand(cmd)
		if err != nil {
			log.Debug("Failed to parse git command", "error", err)
			continue
		}

		// Check if this is a commit command
		if gitCmd.Subcommand != commitSubcommand {
			continue
		}

		// Validate the git commit command
		return v.validateGitCommit(gitCmd, hasGitAdd)
	}

	log.Debug("No git commit commands found")

	return validator.Pass()
}

// validateGitCommit validates a single git commit command
func (v *CommitValidator) validateGitCommit(
	gitCmd *parser.GitCommand,
	hasGitAdd bool,
) *validator.Result {
	log := v.Logger()

	// Check -sS flags
	if res := v.checkFlags(gitCmd); !res.Passed {
		return res
	}

	// Check staging area (skip for --amend, --allow-empty, or if git add is in the chain)
	if v.shouldCheckStaging(gitCmd, hasGitAdd) {
		if res := v.checkStagingArea(gitCmd); !res.Passed {
			return res
		}
	}

	// Extract and validate commit message (if enabled)
	if !v.isMessageValidationEnabled() {
		log.Debug("Commit message validation is disabled")
		return validator.Pass()
	}

	commitMsg, err := v.extractCommitMessage(gitCmd)
	if err != nil {
		log.Error("Failed to extract commit message", "error", err)
		return validator.Fail(fmt.Sprintf("Failed to read commit message: %v", err))
	}

	if commitMsg == "" {
		// No message flag, message will come from editor
		log.Debug("No message flag, message will come from editor")
		return validator.Pass()
	}

	// Validate the commit message
	return v.validateMessage(commitMsg)
}

// shouldCheckStaging determines if staging area should be checked
func (*CommitValidator) shouldCheckStaging(gitCmd *parser.GitCommand, hasGitAdd bool) bool {
	return !gitCmd.HasFlag("--amend") && !gitCmd.HasFlag("--allow-empty") && !hasGitAdd
}

// checkFlags validates that the commit command has required flags
func (v *CommitValidator) checkFlags(gitCmd *parser.GitCommand) *validator.Result {
	// Get required flags from config (default: ["-s", "-S"])
	requiredFlags := v.getRequiredFlags()

	if len(requiredFlags) == 0 {
		// No required flags configured
		return validator.Pass()
	}

	// Check each required flag
	missingFlags := make([]string, 0)

	for _, flag := range requiredFlags {
		hasFlag := gitCmd.HasFlag(flag)

		// For short flags, also check the long form
		switch flag {
		case "-s":
			hasFlag = hasFlag || gitCmd.HasFlag("--signoff")
		case "-S":
			hasFlag = hasFlag || gitCmd.HasFlag("--gpg-sign")
		}

		if !hasFlag {
			missingFlags = append(missingFlags, flag)
		}
	}

	if len(missingFlags) > 0 {
		message := templates.MustExecute(
			templates.GitCommitFlagsTemplate,
			templates.GitCommitFlagsData{
				ArgsStr: strings.Join(gitCmd.Args, " "),
			},
		)

		return validator.FailWithRef(
			validator.RefGitMissingFlags,
			"Git commit missing required flags: "+strings.Join(missingFlags, " "),
		).AddDetail("help", message)
	}

	return validator.Pass()
}

// checkStagingArea validates that there are files staged or -a/-A/--all flag is present
func (v *CommitValidator) checkStagingArea(gitCmd *parser.GitCommand) *validator.Result {
	// Check if staging area validation is enabled (default: true)
	if !v.shouldCheckStagingArea() {
		return validator.Pass()
	}

	// Check if -a, -A, or --all flags are present
	hasStageFlag := gitCmd.HasFlag("-a") || gitCmd.HasFlag("-A") || gitCmd.HasFlag("--all")
	if hasStageFlag {
		return validator.Pass()
	}

	// Check if we're in a git repository first
	if !v.gitRunner.IsInRepo() {
		// Not in a git repo or git not available, skip check
		v.Logger().Debug("Not in git repository, skipping staging check")
		return validator.Pass()
	}

	// Check if staging area has files
	stagedFiles, err := v.gitRunner.GetStagedFiles()
	if err != nil {
		v.Logger().Debug("Failed to check staging area", "error", err)
		return validator.Pass() // Don't block if we can't check
	}

	if len(stagedFiles) == 0 {
		// No files staged, get status info
		modifiedCount, untrackedCount := v.getStatusCounts()

		message := templates.MustExecute(
			templates.GitCommitNoStagedTemplate,
			templates.GitCommitNoStagedData{
				ModifiedCount:  modifiedCount,
				UntrackedCount: untrackedCount,
			},
		)

		return validator.FailWithRef(
			validator.RefGitNoStaged,
			"No files staged for commit",
		).AddDetail("help", message)
	}

	return validator.Pass()
}

// getStatusCounts returns the count of modified and untracked files
func (v *CommitValidator) getStatusCounts() (modified, untracked int) {
	// Get modified files
	modifiedFiles, err := v.gitRunner.GetModifiedFiles()
	if err == nil {
		modified = len(modifiedFiles)
	}

	// Get untracked files
	untrackedFiles, err2 := v.gitRunner.GetUntrackedFiles()
	if err2 == nil {
		untracked = len(untrackedFiles)
	}

	return modified, untracked
}

// hasGitAddInChain checks if there's a git add command in the command chain
// This is important because in PreToolUse hooks, the add hasn't executed yet,
// so we shouldn't check the staging area.
func (*CommitValidator) hasGitAddInChain(commands []parser.Command) bool {
	for _, cmd := range commands {
		if cmd.Name != gitCommand {
			continue
		}

		// Parse git command to get the subcommand (handles global options like -C)
		gitCmd, err := parser.ParseGitCommand(cmd)
		if err != nil {
			continue
		}

		if gitCmd.Subcommand == addSubcommand {
			return true
		}
	}

	return false
}

// extractCommitMessage extracts commit message from -m/--message or -F/--file flags.
func (v *CommitValidator) extractCommitMessage(gitCmd *parser.GitCommand) (string, error) {
	// Check for file flags first (-F/--file)
	if filePath := v.getFlagValue(gitCmd, commitFileFlags); filePath != "" {
		v.Logger().Debug("Reading commit message from file", "path", filePath)

		content, err := os.ReadFile(
			filePath,
		) //#nosec G304 -- file path is user-provided from git commit -F flag
		if err != nil {
			return "", errors.Wrapf(err, "failed to read commit message file %s", filePath)
		}

		return strings.TrimSpace(string(content)), nil
	}

	// Check for inline message flags (-m/--message)
	// TrimSpace handles trailing newlines from HEREDOC syntax: -m "$(cat <<'EOF'\n...\nEOF\n)"
	if msg := v.getFlagValue(gitCmd, commitMessageFlags); msg != "" {
		return strings.TrimSpace(msg), nil
	}

	return "", nil
}

// getFlagValue returns the value for any of the provided flags, or empty string if not found.
func (*CommitValidator) getFlagValue(gitCmd *parser.GitCommand, flags []string) string {
	for _, flag := range flags {
		if value := gitCmd.GetFlagValue(flag); value != "" {
			return value
		}
	}

	return ""
}

// getRequiredFlags returns the required flags from config, or defaults to ["-s", "-S"]
func (v *CommitValidator) getRequiredFlags() []string {
	if v.config != nil && len(v.config.RequiredFlags) > 0 {
		return v.config.RequiredFlags
	}

	// Default: require signoff and GPG sign
	return []string{"-s", "-S"}
}

// shouldCheckStagingArea returns whether staging area validation is enabled
func (v *CommitValidator) shouldCheckStagingArea() bool {
	if v.config != nil && v.config.CheckStagingArea != nil {
		return *v.config.CheckStagingArea
	}

	// Default: check staging area
	return true
}

// isMessageValidationEnabled returns whether commit message validation is enabled
func (v *CommitValidator) isMessageValidationEnabled() bool {
	if v.config != nil && v.config.Message != nil && v.config.Message.Enabled != nil {
		return *v.config.Message.Enabled
	}

	// Default: message validation enabled
	return true
}

// Category returns the validator category for parallel execution.
// CommitValidator uses CategoryGit because it accesses the git staging area.
func (*CommitValidator) Category() validator.ValidatorCategory {
	return validator.CategoryGit
}

// Ensure CommitValidator implements validator.Validator
var _ validator.Validator = (*CommitValidator)(nil)
