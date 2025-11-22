// Package git provides validators for git operations
package git

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
	"github.com/smykla-labs/claude-hooks/pkg/parser"
)

const (
	gitCommandTimeout = 5 * time.Second
)

// AddValidator validates git add commands to block tmp/ files from being staged
type AddValidator struct {
	validator.BaseValidator
	gitRunner GitRunner
}

// NewAddValidator creates a new GitAddValidator instance
func NewAddValidator(log logger.Logger, gitRunner GitRunner) *AddValidator {
	if gitRunner == nil {
		gitRunner = NewRealGitRunner()
	}
	return &AddValidator{
		BaseValidator: *validator.NewBaseValidator("validate-git-add", log),
		gitRunner:     gitRunner,
	}
}

// Validate checks if git add command includes files from tmp/ directory
func (v *AddValidator) Validate(ctx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running git add validation")

	// Check if in git repository
	if !v.gitRunner.IsInRepo() {
		log.Debug("Not in a git repository, skipping validation")
		return validator.Pass()
	}

	gitRoot, err := v.gitRunner.GetRepoRoot()
	if err != nil {
		log.Debug("Failed to get git root", "error", err)
		return validator.Pass()
	}

	log.Debug("Git root found", "path", gitRoot)

	// Parse the command
	bashParser := parser.NewBashParser()
	result, err := bashParser.Parse(ctx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Find all git add commands
	var tmpFiles []string

	for _, cmd := range result.Commands {
		if cmd.Name != "git" || len(cmd.Args) == 0 || cmd.Args[0] != "add" {
			continue
		}

		// Extract file paths from git add command
		files := v.extractFilePaths(cmd.Args[1:])
		log.Debug("Extracted files from git add", "count", len(files), "files", files)

		// Check for tmp/ files
		for _, file := range files {
			if strings.HasPrefix(file, "tmp/") {
				tmpFiles = append(tmpFiles, file)
			}
		}
	}

	// Report errors if tmp/ files found
	if len(tmpFiles) > 0 {
		var details strings.Builder
		details.WriteString("Files in tmp/ should be in .gitignore or .git/info/exclude\n\n")
		details.WriteString("Files being added:\n")

		for _, file := range tmpFiles {
			fmt.Fprintf(&details, "  - %s\n", file)
		}

		details.WriteString("\nAdd tmp/ to .git/info/exclude:\n")
		details.WriteString("  echo 'tmp/' >> .git/info/exclude")

		return validator.Fail(
			"Attempting to add files from tmp/ directory",
		).AddDetail("help", details.String())
	}

	log.Debug("Git add validation passed")
	return validator.Pass()
}

// extractFilePaths extracts file paths from git add arguments, excluding flags
func (v *AddValidator) extractFilePaths(args []string) []string {
	files := make([]string, 0, len(args))
	skipNext := false

	for _, arg := range args {
		// Skip if previous flag expected a value
		if skipNext {
			skipNext = false
			continue
		}

		// Skip empty arguments
		if arg == "" || strings.TrimSpace(arg) == "" {
			continue
		}

		// Skip flag arguments
		if strings.HasPrefix(arg, "-") {
			// Some flags have values (e.g., -m "message")
			// For git add, flags like --chmod need values
			if arg == "--chmod" {
				skipNext = true
			}
			continue
		}

		// Handle special case: '.' means current directory
		if arg == "." {
			// We don't check for tmp/ in entire repo, only explicit tmp/ paths
			continue
		}

		// Clean the path
		cleanPath := filepath.Clean(arg)
		files = append(files, cleanPath)
	}

	return files
}

// Ensure AddValidator implements validator.Validator
var _ validator.Validator = (*AddValidator)(nil)
