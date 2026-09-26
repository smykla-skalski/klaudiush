package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/templates"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
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
	gitRunner GitRunner
	config    *config.CommitValidatorConfig
}

// NewCommitValidator creates a new CommitValidator instance
func NewCommitValidator(
	log logger.Logger,
	gitRunner GitRunner,
	cfg *config.CommitValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *CommitValidator {
	return &CommitValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules(
			"validate-commit", log, ruleAdapter,
		),
		gitRunner: defaultGitRunner(gitRunner),
		config:    cfg,
	}
}

// Validate checks git commit command and message
func (v *CommitValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running git commit validation")

	// Check rules first
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	// Parse the command
	result, err := hookCtx.ParsedCommand()
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	return v.validateCommits(ctx, hookCtx, result)
}

// validateCommits checks every commit on the line, so a clean first one
// cannot let a later one through. A block wins over a warning.
func (v *CommitValidator) validateCommits(
	ctx context.Context,
	hookCtx *hook.Context,
	result *parser.ParseResult,
) *validator.Result {
	hasGitAdd := v.hasGitAddInChain(result.Commands)

	var warning *validator.Result

	for _, cmd := range result.GitOperations {
		// Parse git command to get the subcommand (handles global options like -C)
		gitCmd, err := parser.ParseGitCommand(cmd)
		if err != nil {
			v.Logger().Debug("Failed to parse git command", "error", err)
			continue
		}

		isCommit := gitCmd.Subcommand == commitSubcommand
		if !isCommit && !slices.Contains(otherMessageSubcommands, gitCmd.Subcommand) {
			continue
		}

		// merge, revert, cherry-pick and tag write a message too, but none of
		// the commit contract applies to them - only attribution does.
		if res := v.checkCommandAIAttribution(hookCtx.GetCommand()); res != nil {
			return res
		}

		if !isCommit {
			continue
		}

		res := v.validateGitCommit(ctx, gitCmd, hasGitAdd, result)

		switch {
		case res.ShouldBlock:
			return res
		case !res.Passed && warning == nil:
			warning = res
		}
	}

	if warning != nil {
		return warning
	}

	return validator.Pass()
}

// otherMessageSubcommands are the git subcommands that write a commit message
// without being a commit, so only the attribution rule applies to them.
var otherMessageSubcommands = []string{"merge", "revert", "cherry-pick", "tag"}

// checkCommandAIAttribution rejects AI attribution anywhere in the raw command
// text. The parsed message keeps one value per flag, so a second -m, a
// --trailer, or a --file= spelling would otherwise carry a footer past the
// message rules; the command text carries all of them.
func (v *CommitValidator) checkCommandAIAttribution(command string) *validator.Result {
	if !v.shouldBlockAIAttribution() {
		return nil
	}

	return aiAttributionResult(command, "Commit message")
}

// validateGitCommit validates a single git commit command
func (v *CommitValidator) validateGitCommit(
	ctx context.Context,
	gitCmd *parser.GitCommand,
	hasGitAdd bool,
	parsed *parser.ParseResult,
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

	commitMsg, err := v.extractCommitMessage(gitCmd, parsed)
	if err != nil {
		log.Error("Failed to extract commit message", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to read commit message: %v", err))
	}

	if commitMsg == "" {
		// No message flag, message will come from editor
		log.Debug("No message flag, message will come from editor")
		return validator.Pass()
	}

	// Validate the commit message
	return v.validateMessage(ctx, commitMsg)
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

// gitRunnerFor returns a runner scoped to the git command's working directory.
// When the command has an explicit path (via -C flag or preceding cd), staging
// checks must run against that directory, not the hook's cwd.
func (v *CommitValidator) gitRunnerFor(gitCmd *parser.GitCommand) GitRunner {
	if workDir := gitCmd.GetWorkingDirectory(); workDir != "" {
		return NewGitRunnerForPath(workDir)
	}

	return v.gitRunner
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

	runner := v.gitRunnerFor(gitCmd)

	// Check if we're in a git repository first
	if !runner.IsInRepo() {
		// Not in a git repo or git not available, skip check
		v.Logger().Debug("Not in git repository, skipping staging check")
		return validator.Pass()
	}

	// Check if staging area has files
	stagedFiles, err := runner.GetStagedFiles()
	if err != nil {
		v.Logger().Debug("Failed to check staging area", "error", err)
		return validator.Pass() // Don't block if we can't check
	}

	if len(stagedFiles) == 0 {
		// No files staged, get status info
		modifiedCount, untrackedCount := v.getStatusCounts(runner)

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
func (*CommitValidator) getStatusCounts(runner GitRunner) (modified, untracked int) {
	// Get modified files
	modifiedFiles, err := runner.GetModifiedFiles()
	if err == nil {
		modified = len(modifiedFiles)
	}

	// Get untracked files
	untrackedFiles, err2 := runner.GetUntrackedFiles()
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
func (v *CommitValidator) extractCommitMessage(
	gitCmd *parser.GitCommand,
	parsed *parser.ParseResult,
) (string, error) {
	// Check for file flags first (-F/--file)
	if filePath := v.getFlagValue(gitCmd, commitFileFlags); filePath != "" {
		return v.extractMessageFromFile(gitCmd, parsed, filePath)
	}

	// Check for inline message flags (-m/--message)
	// TrimSpace handles trailing newlines from HEREDOC syntax: -m "$(cat <<'EOF'\n...\nEOF\n)"
	if msg := v.getFlagValue(gitCmd, commitMessageFlags); msg != "" {
		// A bare variable (e.g. -m "$MSG") is an unresolved expansion whose
		// runtime content the hook cannot see. Skip rather than validate the
		// literal token, which would always fail the conventional-commit check.
		if isBareExpansion(msg) {
			v.Logger().
				Debug("commit message is an unresolved variable; skipping validation", "value", msg)

			return "", nil
		}

		return strings.TrimSpace(msg), nil
	}

	return "", nil
}

// extractMessageFromFile resolves the commit message for a -F/--file flag.
// It handles stdin ("-"), content written to the file earlier in the same
// command, and finally the file on disk.
func (v *CommitValidator) extractMessageFromFile(
	gitCmd *parser.GitCommand,
	parsed *parser.ParseResult,
	filePath string,
) (string, error) {
	// "-" means read from stdin. The parser captures stdin fed via a heredoc or
	// a piped echo/printf, so validate that when available.
	if filePath == "-" {
		stdin := strings.TrimSpace(gitCmd.Stdin)
		if stdin == "" {
			// Stdin wasn't capturable (e.g. message piped from a process the
			// parser can't read); treat it like no inline message.
			v.Logger().Debug("Commit message comes from uncaptured stdin (-F -)")

			return "", nil
		}

		v.Logger().Debug("Reading commit message from stdin (-F -)")

		return stdin, nil
	}

	// Prefer content written to the file earlier in the same command. At
	// PreToolUse time the file usually doesn't exist on disk yet (e.g.
	// "cat > msg.txt <<EOF ... EOF; git commit -F msg.txt"), so reading it
	// directly would fail and silently skip message validation. Only writes
	// before this commit are considered, so a later rewrite is ignored.
	if parsed != nil {
		content, ok := parsed.InlineFileContent(
			filePath,
			gitCmd.GetWorkingDirectory(),
			gitCmd.Location,
		)
		if ok {
			v.Logger().Debug("Reading commit message from inline file write", "path", filePath)

			return strings.TrimSpace(content), nil
		}
	}

	// A bare variable path (e.g. -F "$MSG") that did not resolve to inline
	// content names a file whose runtime location the hook cannot determine.
	// Skip rather than attempt a literal "$MSG" disk read that always fails.
	if isBareExpansion(filePath) {
		v.Logger().
			Debug("commit message file path is an unresolved variable; skipping validation", "path", filePath)

		return "", nil
	}

	// Resolve the path the way the shell would: expand a leading ~ and, for a
	// relative path, join it onto the commit's effective working directory (from
	// cd or git -C), since git reads -F relative to where it runs, not the hook's
	// cwd. Resolution is best-effort; an unresolved path just fails the read
	// below and warns, as before.
	readPath := expandTilde(filePath)
	if !filepath.IsAbs(readPath) {
		if workDir := expandTilde(gitCmd.GetWorkingDirectory()); workDir != "" {
			readPath = filepath.Join(workDir, readPath)
		}
	}

	readPath = filepath.Clean(readPath)

	v.Logger().Debug("Reading commit message from file", "path", readPath)

	content, err := os.ReadFile(
		readPath,
	) //#nosec G304 -- file path is user-provided from git commit -F flag
	if err != nil {
		return "", errors.Wrapf(err, "failed to read commit message file %s", readPath)
	}

	return strings.TrimSpace(string(content)), nil
}

// expandTilde best-effort expands a leading ~ or ~/ to the user's home
// directory, mirroring shell expansion the parser leaves intact. Other forms
// (e.g. ~user) and home-lookup failures return the path unchanged.
func expandTilde(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if path == "~" {
		return home
	}

	return filepath.Join(home, path[2:])
}

// isBareExpansion reports whether s is exactly one shell parameter expansion as
// rendered by the parser - the braced token "${...}" produced by
// paramExpToString - and nothing else. Such a value is an unresolved variable
// whose runtime content the hook cannot observe, so callers skip validation
// instead of treating the token as a real message, file path, remote, or branch.
// A single-quoted literal that merely looks like a variable (e.g. '$MSG') renders
// without braces and is still validated; a value mixing an expansion with literal
// text (e.g. "${1}foo") does not end in "}" and is likewise not skipped.
func isBareExpansion(s string) bool {
	if len(s) < 4 || s[0] != '$' || s[1] != '{' || s[len(s)-1] != '}' {
		return false
	}

	inner := s[2 : len(s)-1]

	return inner != "" && !strings.ContainsAny(inner, "{}")
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
