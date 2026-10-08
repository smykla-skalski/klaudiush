package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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
		attribution := v.checkCommandAIAttribution(
			withoutPathArguments(hookCtx.GetCommand(), result),
		)
		if !isCommit {
			if attribution != nil {
				return attribution
			}

			continue
		}

		src := messageSource{
			cmd:    cmd,
			parsed: result,
			cwd:    shellDir(hookCtx),
			text:   hookCtx.GetCommand(),
		}

		res := v.validateGitCommit(ctx, gitCmd, hasGitAdd, src)
		if attribution != nil {
			return v.withAttribution(ctx, res, attribution, gitCmd, src)
		}

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

// withAttribution reports command-level AI attribution together with the
// message findings, so the agent repairs every violation in one retry. A
// result without findings (missing flags, nothing staged) still yields to the
// attribution block as before. When the message already has an attribution
// finding, attribution elsewhere on the command gets its own finding.
func (v *CommitValidator) withAttribution(
	ctx context.Context,
	res, attribution *validator.Result,
	gitCmd *parser.GitCommand,
	src messageSource,
) *validator.Result {
	if !res.ShouldBlock || len(res.Findings) == 0 {
		return attribution
	}

	extra := attribution.Findings

	if slices.ContainsFunc(res.Findings, func(f validator.Finding) bool {
		return f.Reference == validator.RefGitClaudeAttr
	}) {
		msg, err := v.extractCommitMessage(ctx, gitCmd, src)
		if err != nil || !containsAIAttribution(
			withoutPathArguments(withoutMessage(src.text, msg), src.parsed),
		) {
			return res
		}

		extra = []validator.Finding{commandAttributionFinding()}
	}

	res.Findings = validator.SortFindings(append(res.Findings, extra...), referenceFixOrder)

	return res
}

// withoutMessage removes one copy of each message line from the command, so
// what is left is the text the parsed message does not cover.
func withoutMessage(command, message string) string {
	for line := range strings.SplitSeq(message, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			command = strings.Replace(command, line, "", 1)
		}
	}

	return command
}

// relativePathArgPattern matches an argument that is a relative path word,
// such as src/claude or docs/claude-notes.md.
var relativePathArgPattern = regexp.MustCompile(`^[\w+%@~][\w.+%@~-]*(?:/[\w.+%@~-]*)+$`)

// textValueFlags take free text, so the argument after one is prose, not a
// path, even when it looks like one.
var textValueFlags = slices.Concat(commitMessageFlags, []string{
	"-t", "--title", "-b", "--body", "--subject", "--trailer",
})

// textCommands print their arguments as text, which may be a message.
var textCommands = []string{"echo", "printf"}

// withoutPathArguments removes from the command text every argument the
// parsed command passes as a relative path, as in "cd src/claude" or
// "git add docs/claude-notes.md". The attribution check reads the raw text
// line by line, so a name in such a path would otherwise pair with a marker
// word in the message. Prose is left alone: "w/Claude" in a message value,
// an echo or an assignment is not an argument the parser reads as a path.
func withoutPathArguments(command string, parsed *parser.ParseResult) string {
	if parsed == nil {
		return command
	}

	for _, cmd := range parsed.Commands {
		if slices.Contains(textCommands, cmd.Name) {
			continue
		}

		writes := writesMessage(cmd)

		for i, arg := range cmd.Args {
			if i > 0 && isTextValueFlag(cmd.Args[i-1], writes) {
				continue
			}

			if relativePathArgPattern.MatchString(arg) {
				command = withoutSoleOccurrence(command, arg)
			}
		}
	}

	return command
}

// shortTextFlagCluster matches a cluster of short flags ending in one that
// takes text, such as "-sm" or "-am".
var shortTextFlagCluster = regexp.MustCompile(`^-[a-zA-Z]*[mtb]$`)

// isTextValueFlag reports whether the argument after flag is free text. Only
// a command that writes a message takes text after -t, -b or a flag cluster;
// elsewhere those carry a branch or other name ("git checkout -b feat/x").
func isTextValueFlag(flag string, writes bool) bool {
	if slices.Contains(commitMessageFlags, flag) {
		return true
	}

	return writes &&
		(slices.Contains(textValueFlags, flag) || shortTextFlagCluster.MatchString(flag))
}

// writesMessage reports whether a command writes a commit, tag or pull
// request message: gh, or git with a message-writing subcommand.
func writesMessage(cmd parser.Command) bool {
	switch cmd.Name {
	case "gh":
		return true
	case gitCommand:
		return slices.ContainsFunc(cmd.Args, func(arg string) bool {
			return arg == commitSubcommand || arg == "notes" ||
				slices.Contains(otherMessageSubcommands, arg)
		})
	default:
		return false
	}
}

// withoutSoleOccurrence removes word from the command when it appears there
// exactly once, ignoring case, and stands as a word of its own. A word that
// also appears elsewhere, such as inside a message, is kept everywhere:
// removing every copy would let a no-op argument ("; : w/claude") erase the
// same text from the message. A word inside a command substitution or a
// quoted string holding more than the word is part of some other text, so it
// is kept too.
func withoutSoleOccurrence(command, word string) string {
	spans := regexp.MustCompile(`(?i)`+regexp.QuoteMeta(word)).FindAllStringIndex(command, -1)
	if len(spans) != 1 || !standsAlone(command, spans[0][0], spans[0][1]) {
		return command
	}

	return command[:spans[0][0]] + command[spans[0][1]:]
}

// standsAlone reports whether command[start:end] is a shell word of its own:
// outside any command substitution, and either unquoted or the whole content
// of its quotes. It errs toward false, which only keeps text in the check.
func standsAlone(command string, start, end int) bool {
	var (
		quote      byte
		quoteStart int
		depth      int
		backtick   bool
	)

	for i := 0; i < start; i++ {
		c := command[i]

		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\':
			i++
		case c == '`':
			backtick = !backtick
		case c == '$' && i+1 < len(command) && command[i+1] == '(':
			depth++
			i++
		case c == ')' && depth > 0:
			depth--
		case quote == '"':
			if c == '"' {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote, quoteStart = c, i
		}
	}

	if depth > 0 || backtick {
		return false
	}

	if quote == 0 {
		return true
	}

	return quoteStart == start-1 && end < len(command) && command[end] == quote
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
	src messageSource,
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

	commitMsg, err := v.extractCommitMessage(ctx, gitCmd, src)
	if err != nil {
		log.Debug("Commit message cannot be inspected", "error", err)

		if res, ok := opaqueMessageResult(err); ok {
			return res
		}

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
	hasStageFlag := gitCmd.HasFlag("-a") || gitCmd.HasFlag("-A") || gitCmd.HasFlag(flagAll)
	if hasStageFlag {
		return validator.Pass()
	}

	runner := v.gitRunnerFor(gitCmd)

	// Check if we're in a git repository first
	inRepo, err := runner.IsInRepo()
	if err != nil || !inRepo {
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
	ctx context.Context,
	gitCmd *parser.GitCommand,
	src messageSource,
) (string, error) {
	if err := checkMessageFlags(gitCmd); err != nil {
		return "", err
	}

	fixupRev, fixupGap, fixupReuses := src.fixupRev(gitCmd)

	edits := editorRuns(gitCmd, fixupReuses)
	if edits {
		if err := src.checkEditor(gitCmd); err != nil {
			return "", err
		}
	}

	strip := edits && src.cleanupStrips(gitCmd)
	reuse, reuses := lastValue(gitCmd, reuseFlags)

	switch {
	case hasFileFlag(gitCmd):
		return v.readMessageFile(gitCmd, src, v.getFlagValue(gitCmd, commitFileFlags))
	case reuses:
		rev, gap := src.flagText(gitCmd, reuse)

		return v.reusedMessage(ctx, gitCmd, src, rev, gap, strip)
	case fixupReuses:
		return v.reusedMessage(ctx, gitCmd, src, fixupRev, fixupGap, strip)
	case slices.ContainsFunc(commitMessageFlags, gitCmd.HasFlag):
		return src.inlineMessage(gitCmd)
	case usesTemplate(gitCmd):
		return v.templateMessage(gitCmd, src, strip)
	default:
		return "", nil
	}
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
