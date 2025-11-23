package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/smykla-labs/claude-hooks/internal/templates"
	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
	"github.com/smykla-labs/claude-hooks/pkg/parser"
)

const (
	gitCommand       = "git"
	commitSubcommand = "commit"
	addSubcommand    = "add"
)

// CommitValidator validates git commit commands and messages
type CommitValidator struct {
	validator.BaseValidator
	gitRunner GitRunner
}

// NewCommitValidator creates a new CommitValidator instance
func NewCommitValidator(log logger.Logger, gitRunner GitRunner) *CommitValidator {
	if gitRunner == nil {
		gitRunner = NewGitRunner()
	}

	return &CommitValidator{
		BaseValidator: *validator.NewBaseValidator("validate-commit", log),
		gitRunner:     gitRunner,
	}
}

// Validate checks git commit command and message
func (v *CommitValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running git commit validation")

	// Parse the command
	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(hookCtx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Check if there's a git add in the same command chain
	hasGitAdd := v.hasGitAddInChain(result.Commands)

	// Find git commit commands
	for _, cmd := range result.Commands {
		if cmd.Name != gitCommand || len(cmd.Args) == 0 || cmd.Args[0] != commitSubcommand {
			continue
		}

		// Parse git command for flags and message
		gitCmd, err := parser.ParseGitCommand(cmd)
		if err != nil {
			log.Error("Failed to parse git command", "error", err)
			return validator.Warn(fmt.Sprintf("Failed to parse git command: %v", err))
		}

		// Check -sS flags
		if res := v.checkFlags(gitCmd); !res.Passed {
			return res
		}

		// Check staging area (skip for --amend, --allow-empty, or if git add is in the chain)
		if !gitCmd.HasFlag("--amend") && !gitCmd.HasFlag("--allow-empty") && !hasGitAdd {
			if res := v.checkStagingArea(gitCmd); !res.Passed {
				return res
			}
		}

		// Extract and validate commit message
		commitMsg := gitCmd.ExtractCommitMessage()
		if commitMsg == "" {
			// No -m flag, message will come from editor
			log.Debug("No -m flag, message will come from editor")
			return validator.Pass()
		}

		// Validate the commit message
		return v.validateMessage(commitMsg)
	}

	log.Debug("No git commit commands found")

	return validator.Pass()
}

// checkFlags validates that the commit command has -sS flags
func (*CommitValidator) checkFlags(gitCmd *parser.GitCommand) *validator.Result {
	// Check for -s (signoff) and -S (GPG sign)
	hasSignoff := gitCmd.HasFlag("-s") || gitCmd.HasFlag("--signoff")
	hasGPGSign := gitCmd.HasFlag("-S") || gitCmd.HasFlag("--gpg-sign")

	if !hasSignoff || !hasGPGSign {
		message := templates.MustExecute(
			templates.GitCommitFlagsTemplate,
			templates.GitCommitFlagsData{
				ArgsStr: strings.Join(gitCmd.Args, " "),
			},
		)

		return validator.Fail(
			"Git commit must use -sS flags",
		).AddDetail("help", message)
	}

	return validator.Pass()
}

// checkStagingArea validates that there are files staged or -a/-A/--all flag is present
func (v *CommitValidator) checkStagingArea(gitCmd *parser.GitCommand) *validator.Result {
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

		return validator.Fail(
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
		if cmd.Name == gitCommand && len(cmd.Args) > 0 && cmd.Args[0] == addSubcommand {
			return true
		}
	}

	return false
}

// Ensure CommitValidator implements validator.Validator
var _ validator.Validator = (*CommitValidator)(nil)
