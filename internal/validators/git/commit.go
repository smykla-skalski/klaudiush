package git

import (
	"fmt"
	"strings"

	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
	"github.com/smykla-labs/claude-hooks/pkg/parser"
)

// CommitValidator validates git commit commands and messages
type CommitValidator struct {
	validator.BaseValidator
	gitRunner GitRunner
}

// NewCommitValidator creates a new CommitValidator instance
func NewCommitValidator(log logger.Logger, gitRunner GitRunner) *CommitValidator {
	if gitRunner == nil {
		gitRunner = NewRealGitRunner()
	}
	return &CommitValidator{
		BaseValidator: *validator.NewBaseValidator("validate-commit", log),
		gitRunner:     gitRunner,
	}
}

// Validate checks git commit command and message
func (v *CommitValidator) Validate(ctx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running git commit validation")

	// Parse the command
	bashParser := parser.NewBashParser()
	result, err := bashParser.Parse(ctx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Find git commit commands
	for _, cmd := range result.Commands {
		if cmd.Name != "git" || len(cmd.Args) == 0 || cmd.Args[0] != "commit" {
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

		// Check staging area (skip for --amend or --allow-empty)
		if !gitCmd.HasFlag("--amend") && !gitCmd.HasFlag("--allow-empty") {
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
func (v *CommitValidator) checkFlags(gitCmd *parser.GitCommand) *validator.Result {
	// Check for -s (signoff) and -S (GPG sign)
	hasSignoff := gitCmd.HasFlag("-s") || gitCmd.HasFlag("--signoff")
	hasGPGSign := gitCmd.HasFlag("-S") || gitCmd.HasFlag("--gpg-sign")

	if !hasSignoff || !hasGPGSign {
		var details strings.Builder
		details.WriteString("Git commit must use -sS flags (signoff + GPG sign)\n\n")
		fmt.Fprintf(&details, "Current command: git commit %s\n", strings.Join(gitCmd.Args, " "))
		details.WriteString("Expected: git commit -sS -m \"message\"")

		return validator.Fail(
			"Git commit must use -sS flags",
		).AddDetail("help", details.String())
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

		var details strings.Builder
		details.WriteString("No files staged for commit and no -a/-A flag specified\n\n")
		details.WriteString("Current status:\n")
		fmt.Fprintf(&details, "  Modified files (not staged): %d\n", modifiedCount)
		fmt.Fprintf(&details, "  Untracked files: %d\n", untrackedCount)
		details.WriteString("  Staged files: 0\n\n")
		details.WriteString("Did you forget to:\n")
		details.WriteString("  • Stage files? Run 'git add <files>' or 'git add .'\n")
		details.WriteString("  • Use -a flag? Run 'git commit -a' to commit all modified files")

		return validator.Fail(
			"No files staged for commit",
		).AddDetail("help", details.String())
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

// Ensure CommitValidator implements validator.Validator
var _ validator.Validator = (*CommitValidator)(nil)
