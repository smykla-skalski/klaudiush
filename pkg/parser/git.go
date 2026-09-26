package parser

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
)

const (
	// flagMessage is the short git/gh message flag.
	flagMessage = "-m"
	// flagUpperC ("-C") and flagLowerC ("-c") are reused with a different meaning
	// per subcommand, handled per context rather than globally: -C is a repo path
	// for the top-level git command, --reuse-message for commit, --force-create
	// for switch, and force-copy for branch; -c is a config override for the
	// top-level command, --create for switch, and copy for branch.
	flagUpperC = "-C"
	flagLowerC = "-c"
)

// Subcommands whose flag/argument handling needs special-casing.
const (
	subcmdCheckout = "checkout"
	subcmdSwitch   = "switch"
	subcmdBranch   = "branch"
	subcmdCommit   = "commit"
)

var (
	// ErrNotGitCommand is returned when the command is not a git command.
	ErrNotGitCommand = errors.New("not a git command")
	// ErrNoSubcommand is returned when git command has no subcommand.
	ErrNoSubcommand = errors.New("git command has no subcommand")
)

// GitCommand represents a parsed git command.
type GitCommand struct {
	Subcommand       string            // Git subcommand (e.g., "commit", "push", "add")
	Flags            []string          // Command flags
	Args             []string          // Positional arguments
	FlagMap          map[string]string // Flag values (e.g., "-m" -> "commit message")
	GlobalOptions    map[string]string // Global git options (e.g., "-C" -> "/path/to/repo")
	WorkingDirectory string            // Working directory from preceding cd commands
	Stdin            string            // Content fed to stdin (heredoc or piped echo/printf)
	Location         Location          // Position of the command in source
}

// globalValueOptions are the global git options that take the next argument
// as their value. Every other option before the subcommand stands alone.
var globalValueOptions = nameSet(flagUpperC + " " + flagLowerC + ` --git-dir --work-tree
	--namespace --super-prefix --config-env --exec-path --attr-source --list-cmds`)

// Flags that take a value regardless of subcommand. Context-dependent flags
// (-c/-C, and branch's -m) are handled per subcommand in flagTakesValue instead.
var flagsWithValues = map[string]bool{
	flagMessage: true,
	"--message": true,
	"-F":        true,
	"--file":    true,
}

// commitReuseFlags reuse another commit's message for "git commit" and consume
// the commit reference: -c/--reedit-message edits the reused message, -C/
// --reuse-message keeps it as-is.
var commitReuseFlags = []string{flagLowerC, "--reedit-message", flagUpperC, "--reuse-message"}

// checkoutCreationFlags and switchCreationFlags consume the following token as
// the new branch name for their own subcommand ("git checkout -b feat/x",
// "git switch -C feat/x"), listed in branch-name extraction order. They are kept
// per subcommand because the letters mean other things elsewhere (e.g. -C is
// commit's reuse-message, value-taking via flagsWithValues), so only a
// subcommand's own creation flags capture a branch name.
var (
	checkoutCreationFlags = []string{"-b", "--branch"}
	switchCreationFlags   = []string{flagLowerC, "--create", flagUpperC, "--force-create"}
)

// branchRenameCopyFlags mark a "git branch" rename or copy, which takes an
// optional old name followed by the new name - so the new name is the last
// positional argument rather than the first.
var branchRenameCopyFlags = []string{"-m", "-M", "--move", flagLowerC, flagUpperC, "--copy"}

// branchCreationFlagsFor returns the branch-creation flags for subcommand, or nil
// when it has none.
func branchCreationFlagsFor(subcommand string) []string {
	switch subcommand {
	case subcmdCheckout:
		return checkoutCreationFlags
	case subcmdSwitch:
		return switchCreationFlags
	default:
		return nil
	}
}

// flagTakesValue reports whether flag consumes the next token as its value within
// the given subcommand. It resolves context-dependent flags (-c/-C, branch's -m)
// per subcommand before falling back to the subcommand-independent set.
func flagTakesValue(flag, subcommand string) bool {
	switch subcommand {
	case subcmdCheckout, subcmdSwitch:
		// The creation flag captures the new branch name.
		if slices.Contains(branchCreationFlagsFor(subcommand), flag) {
			return true
		}
	case subcmdBranch:
		// Rename/copy mode flags (-m/-M/--move, -c/-C/--copy) are boolean; the
		// old and new names are positional.
		if slices.Contains(branchRenameCopyFlags, flag) {
			return false
		}
	case subcmdCommit:
		// -c/-C reuse another commit's message and consume the commit ref.
		if slices.Contains(commitReuseFlags, flag) {
			return true
		}
	}

	return flagsWithValues[flag]
}

// ParseGitCommand parses a Command into a GitCommand.
func ParseGitCommand(cmd Command) (*GitCommand, error) {
	if cmd.Name != gitProgram {
		return nil, ErrNotGitCommand
	}

	if len(cmd.Args) == 0 {
		return nil, ErrNoSubcommand
	}

	gitCmd := &GitCommand{
		Flags:            make([]string, 0),
		Args:             make([]string, 0),
		FlagMap:          make(map[string]string),
		GlobalOptions:    make(map[string]string),
		WorkingDirectory: cmd.WorkingDirectory,
		Stdin:            cmd.Stdin,
		Location:         cmd.Location,
	}

	// First, parse global options and find the subcommand
	subcommandIdx := parseGlobalOptions(cmd.Args, gitCmd)

	if subcommandIdx >= len(cmd.Args) {
		return nil, ErrNoSubcommand
	}

	gitCmd.Subcommand = cmd.Args[subcommandIdx]

	// Parse remaining arguments after the subcommand
	i := subcommandIdx + 1
	for i < len(cmd.Args) {
		arg := cmd.Args[i]

		if !strings.HasPrefix(arg, "-") {
			// Positional argument
			gitCmd.Args = append(gitCmd.Args, arg)
			i++

			continue
		}

		// It's a flag - determine how to parse it
		switch {
		case strings.HasPrefix(arg, "--"):
			// Long flag: --message, --signoff, etc.
			i = parseLongFlag(arg, cmd.Args, i, gitCmd)
		case len(arg) == 2: //nolint:mnd // Trivial check for single short flag format
			// Single short flag: -m, -s, etc.
			i = parseShortFlag(arg, cmd.Args, i, gitCmd)
		default:
			// Combined short flags: -sS, -sSm, etc.
			i = parseCombinedFlags(arg, cmd.Args, i, gitCmd)
		}
	}

	return gitCmd, nil
}

const (
	skipFlagOnly     = 1
	skipFlagAndValue = 2
	splitKeyValue    = 2 // Split into key=value pair
)

// parseGlobalOptions parses global git options from args and returns the index of the subcommand.
func parseGlobalOptions(args []string, gitCmd *GitCommand) int {
	i := 0

	for i < len(args) {
		arg := args[i]

		// Check if this is a global option
		if !strings.HasPrefix(arg, "-") {
			// Found the subcommand
			return i
		}

		// Handle --option=value format
		if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", splitKeyValue)
			gitCmd.GlobalOptions[parts[0]] = parts[1]
			i++

			continue
		}

		// Every option before the subcommand is a global option. One missing
		// from the table is skipped rather than taken for the subcommand, so
		// "git -P commit" still reads as commit. Git rejects a truly unknown one.
		if globalValueOptions[arg] && i+1 < len(args) {
			gitCmd.GlobalOptions[arg] = args[i+1]
			i += 2
		} else {
			gitCmd.GlobalOptions[arg] = ""
			i++
		}
	}

	return i
}

// addFlag adds a flag to the GitCommand and optionally captures its value.
// For flags that can appear multiple times (like -m), only the first value is stored
// in FlagMap since the first -m is the commit title which is validated.
func addFlag(flag string, args []string, idx int, gitCmd *GitCommand) int {
	gitCmd.Flags = append(gitCmd.Flags, flag)

	// Check if this flag takes a value
	if flagTakesValue(flag, gitCmd.Subcommand) {
		if idx+1 < len(args) {
			// Only store the first value for flags that can repeat (e.g., -m for title)
			if _, alreadySet := gitCmd.FlagMap[flag]; !alreadySet {
				gitCmd.FlagMap[flag] = args[idx+1]
			}

			return skipFlagAndValue
		}
	}

	return skipFlagOnly
}

// parseLongFlag handles long flags like --message, --signoff
func parseLongFlag(flag string, args []string, idx int, gitCmd *GitCommand) int {
	// "--file=path" and "--message=text" carry the value in the same token, so
	// the name is recorded without it and the value is taken from the token
	// rather than from the next argument.
	if name, value, found := strings.Cut(flag, "="); found {
		gitCmd.Flags = append(gitCmd.Flags, name)

		if _, alreadySet := gitCmd.FlagMap[name]; !alreadySet {
			gitCmd.FlagMap[name] = value
		}

		return idx + skipFlagOnly
	}

	return idx + addFlag(flag, args, idx, gitCmd)
}

// parseShortFlag handles single short flags like -m, -s
func parseShortFlag(flag string, args []string, idx int, gitCmd *GitCommand) int {
	return idx + addFlag(flag, args, idx, gitCmd)
}

// parseCombinedFlags handles combined short flags like -sS, -sSm "message"
func parseCombinedFlags(combined string, args []string, idx int, gitCmd *GitCommand) int {
	flags := combined[1:] // Remove leading '-'

	for j, flagChar := range flags {
		flag := "-" + string(flagChar)
		gitCmd.Flags = append(gitCmd.Flags, flag)

		// Check if this flag takes a value
		if !flagTakesValue(flag, gitCmd.Subcommand) {
			continue
		}

		// This flag takes a value
		if j != len(flags)-1 {
			// Not last flag: rest of string is the inline value
			gitCmd.FlagMap[flag] = flags[j+1:]
			return idx + skipFlagOnly
		}

		// Last flag: consume next arg if available
		if idx+1 < len(args) {
			gitCmd.FlagMap[flag] = args[idx+1]
			return idx + skipFlagAndValue
		}
	}

	return idx + skipFlagOnly
}

// HasFlag checks if the git command has a specific flag.
// For short flags (single dash, single letter), it also checks combined flags.
// For example, HasFlag("-s") will match both "-s" and "-sS".
func (g *GitCommand) HasFlag(flag string) bool {
	for _, f := range g.Flags {
		if f == flag {
			return true
		}

		// Check for combined short flags (e.g., -sS contains -s and -S)
		if len(flag) == 2 && flag[0] == '-' && len(f) > 2 && f[0] == '-' && f[1] != '-' {
			// flag is a short flag like "-s", f is like "-sS" or "-abc"
			// Check if flag letter appears in f
			flagLetter := flag[1]
			for i := 1; i < len(f); i++ {
				if f[i] == flagLetter {
					return true
				}
			}
		}
	}

	return false
}

// GetFlagValue returns the value for a flag, or empty string if not found.
func (g *GitCommand) GetFlagValue(flag string) string {
	return g.FlagMap[flag]
}

// ExtractCommitMessage extracts commit message from -m flag or returns empty.
func (g *GitCommand) ExtractCommitMessage() string {
	// Try various message flags
	for _, flag := range []string{flagMessage, "--message"} {
		if msg := g.GetFlagValue(flag); msg != "" {
			return msg
		}
	}

	return ""
}

// ExtractRemote extracts remote name from push/pull/fetch commands.
func (g *GitCommand) ExtractRemote() string {
	// For push/pull/fetch, first positional arg is usually the remote
	if g.Subcommand == "push" || g.Subcommand == "pull" || g.Subcommand == "fetch" {
		if len(g.Args) > 0 {
			return g.Args[0]
		}
	}

	return ""
}

// ExtractBranchName extracts branch name from various git commands.
func (g *GitCommand) ExtractBranchName() string {
	switch g.Subcommand {
	case subcmdCheckout, subcmdSwitch:
		// git checkout -b/--branch <branch>, git switch -c/--create/-C/
		// --force-create <branch>: the creation flag captures the new branch
		// name (including a dash-prefixed one). Existing-branch checkout/switch
		// has no creation flag, so fall back to the first positional argument.
		if name := g.branchCreationName(); name != "" {
			return name
		}

		if len(g.Args) > 0 {
			return g.Args[0]
		}

	case subcmdBranch:
		if len(g.Args) == 0 {
			return ""
		}

		// Rename/copy (-m/-M/--move, -c/-C/--copy) take an optional old name then
		// the new name, so the new name is the last positional. Plain creation,
		// "git branch <new> [start-point]", puts the new name first.
		if slices.ContainsFunc(branchRenameCopyFlags, g.HasFlag) {
			return g.Args[len(g.Args)-1]
		}

		return g.Args[0]

	case "push", "pull":
		// git push/pull <remote> <branch>
		if len(g.Args) > 1 {
			return g.Args[1]
		}
	}

	return ""
}

// branchCreationName returns the value captured for the subcommand's branch
// creation flag, or "" when no such flag is present.
func (g *GitCommand) branchCreationName() string {
	for _, flag := range branchCreationFlagsFor(g.Subcommand) {
		if name := g.FlagMap[flag]; name != "" {
			return name
		}
	}

	return ""
}

// ExtractFilePaths extracts file paths from git add/rm/mv commands.
func (g *GitCommand) ExtractFilePaths() []string {
	switch g.Subcommand {
	case "add", "rm":
		// All non-flag args are file paths
		return g.Args

	case "mv":
		// Last arg is destination
		if len(g.Args) >= 2 { //nolint:mnd // Trivial check for minimum args (source + dest)
			return []string{g.Args[len(g.Args)-1]}
		}
	}

	return nil
}

// GetWorkingDirectory returns git's effective working directory, combining a
// preceding cd with git's -C flag the way the shell and git do: "cd a" then
// "git -C b" runs in a/b, while an absolute -C ignores the cd. Returns the cd
// directory alone when there is no -C, or "" when neither is present.
func (g *GitCommand) GetWorkingDirectory() string {
	cdDir := g.WorkingDirectory

	cDir, hasC := g.GlobalOptions[flagUpperC]
	if !hasC {
		return cdDir
	}

	// -C is relative to the cd directory unless it is absolute (or there is no
	// preceding cd).
	if cdDir == "" || filepath.IsAbs(cDir) {
		return cDir
	}

	return filepath.Join(cdDir, cDir)
}

// GetGitDir returns the git directory from --git-dir global option.
func (g *GitCommand) GetGitDir() string {
	if dir, ok := g.GlobalOptions["--git-dir"]; ok {
		return dir
	}

	return ""
}

// HasGlobalOption checks if the git command has a specific global option.
func (g *GitCommand) HasGlobalOption(option string) bool {
	_, ok := g.GlobalOptions[option]

	return ok
}
