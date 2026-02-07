// Package git provides validators for git operations
package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/templates"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

const (
	gitCommandTimeout = 5 * time.Second
	gitCmd            = "git"
	addCmd            = "add"
)

// AddValidator validates git add commands to block files matching blocked patterns from being staged
type AddValidator struct {
	validator.BaseValidator
	gitRunner   GitRunner
	config      *config.AddValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewAddValidator creates a new GitAddValidator instance
func NewAddValidator(
	log logger.Logger,
	gitRunner GitRunner,
	cfg *config.AddValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *AddValidator {
	if gitRunner == nil {
		gitRunner = NewGitRunner()
	}

	return &AddValidator{
		BaseValidator: *validator.NewBaseValidator("validate-git-add", log),
		gitRunner:     gitRunner,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate checks if git add command includes files from tmp/ directory
func (v *AddValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running git add validation")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

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

	result, err := bashParser.Parse(hookCtx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	// Get blocked patterns from config (default: ["tmp/*"])
	blockedPatterns := v.getBlockedPatterns()
	log.Debug("Using blocked patterns", "patterns", blockedPatterns)

	// Find all git add commands and check for blocked files
	blockedFiles := v.findBlockedFiles(result.Commands, blockedPatterns)

	// Report errors if blocked files found
	if len(blockedFiles) > 0 {
		message := templates.MustExecute(
			templates.GitAddTmpFilesTemplate,
			templates.GitAddTmpFilesData{
				Files: blockedFiles,
			},
		)

		return validator.FailWithRef(
			validator.RefGitBlockedFiles,
			"Attempting to add blocked files",
		).AddDetail("help", message)
	}

	log.Debug("Git add validation passed")

	return validator.Pass()
}

// extractFilePaths extracts file paths from git add arguments, excluding flags
func (*AddValidator) extractFilePaths(args []string) []string {
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

// findBlockedFiles finds all files matching blocked patterns in git add commands
func (v *AddValidator) findBlockedFiles(
	commands []parser.Command,
	blockedPatterns []string,
) []string {
	log := v.Logger()

	var blockedFiles []string

	for _, cmd := range commands {
		if !v.isGitAddCommand(cmd) {
			continue
		}

		// Extract file paths from git add command
		files := v.extractFilePaths(cmd.Args[1:])
		log.Debug("Extracted files from git add", "count", len(files), "files", files)

		// Check each file against blocked patterns
		blocked := v.checkFilesAgainstPatterns(files, blockedPatterns)
		blockedFiles = append(blockedFiles, blocked...)
	}

	return blockedFiles
}

// isGitAddCommand checks if a command is a git add command
func (*AddValidator) isGitAddCommand(cmd parser.Command) bool {
	return cmd.Name == gitCmd && len(cmd.Args) > 0 && cmd.Args[0] == addCmd
}

// checkFilesAgainstPatterns checks files against blocked patterns
func (v *AddValidator) checkFilesAgainstPatterns(files, patterns []string) []string {
	var blocked []string

	for _, file := range files {
		if v.isFileBlocked(file, patterns) {
			blocked = append(blocked, file)
		}
	}

	return blocked
}

// isFileBlocked checks if a file matches any blocked pattern
func (v *AddValidator) isFileBlocked(file string, patterns []string) bool {
	log := v.Logger()

	for _, pattern := range patterns {
		// Try glob pattern match
		matched, err := filepath.Match(pattern, file)
		if err != nil {
			log.Debug("Invalid pattern", "pattern", pattern, "error", err)
			continue
		}

		if matched {
			return true
		}

		// Also check prefix for patterns like "tmp/*" to match "tmp/nested/file.txt"
		// Extract directory prefix from pattern by removing glob characters
		prefix := strings.TrimSuffix(pattern, "/*")
		prefix = strings.TrimSuffix(prefix, "/**")

		if strings.HasPrefix(file, prefix+"/") || file == prefix {
			return true
		}
	}

	return false
}

// getBlockedPatterns returns the blocked patterns from config, or defaults to ["tmp/*"]
func (v *AddValidator) getBlockedPatterns() []string {
	if v.config != nil && len(v.config.BlockedPatterns) > 0 {
		return v.config.BlockedPatterns
	}

	// Default: block tmp/ files
	return []string{"tmp/*"}
}

// Category returns the validator category for parallel execution.
// AddValidator uses CategoryGit because it queries git repository state.
func (*AddValidator) Category() validator.ValidatorCategory {
	return validator.CategoryGit
}

// Ensure AddValidator implements validator.Validator
var _ validator.Validator = (*AddValidator)(nil)
