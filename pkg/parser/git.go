package parser

import (
	"strings"

	"github.com/pkg/errors"
)

var (
	// ErrNotGitCommand is returned when the command is not a git command.
	ErrNotGitCommand = errors.New("not a git command")
	// ErrNoSubcommand is returned when git command has no subcommand.
	ErrNoSubcommand = errors.New("git command has no subcommand")
)

// GitCommand represents a parsed git command.
type GitCommand struct {
	Subcommand string            // Git subcommand (e.g., "commit", "push", "add")
	Flags      []string          // Command flags
	Args       []string          // Positional arguments
	FlagMap    map[string]string // Flag values (e.g., "-m" -> "commit message")
}

// Flags that take values.
var flagsWithValues = map[string]bool{
	"-m":              true,
	"--message":       true,
	"-F":              true,
	"--file":          true,
	"-C":              true,
	"--reuse-message": true,
	"-c":              false, // -c for switch/checkout is a boolean flag
	"-b":              false, // -b for checkout is a boolean flag
}

// ParseGitCommand parses a Command into a GitCommand.
func ParseGitCommand(cmd Command) (*GitCommand, error) {
	if cmd.Name != "git" {
		return nil, ErrNotGitCommand
	}

	if len(cmd.Args) == 0 {
		return nil, ErrNoSubcommand
	}

	gitCmd := &GitCommand{
		Subcommand: cmd.Args[0],
		Flags:      make([]string, 0),
		Args:       make([]string, 0),
		FlagMap:    make(map[string]string),
	}

	// Parse remaining arguments
	i := 1
	for i < len(cmd.Args) {
		arg := cmd.Args[i]

		if !strings.HasPrefix(arg, "-") {
			// Positional argument
			gitCmd.Args = append(gitCmd.Args, arg)
			i++
			continue
		}

		// It's a flag - determine how to parse it
		if strings.HasPrefix(arg, "--") {
			// Long flag: --message, --signoff, etc.
			i = parseLongFlag(arg, cmd.Args, i, gitCmd)
		} else if len(arg) == 2 { //nolint:mnd // Trivial check for single short flag format
			// Single short flag: -m, -s, etc.
			i = parseShortFlag(arg, cmd.Args, i, gitCmd)
		} else {
			// Combined short flags: -sS, -sSm, etc.
			i = parseCombinedFlags(arg, cmd.Args, i, gitCmd)
		}
	}

	return gitCmd, nil
}

const (
	skipFlagOnly     = 1
	skipFlagAndValue = 2
)

// addFlag adds a flag to the GitCommand and optionally captures its value
func addFlag(flag string, args []string, idx int, gitCmd *GitCommand) int {
	gitCmd.Flags = append(gitCmd.Flags, flag)

	// Check if this flag takes a value
	if takesValue, exists := flagsWithValues[flag]; exists && takesValue {
		if idx+1 < len(args) {
			gitCmd.FlagMap[flag] = args[idx+1]
			return skipFlagAndValue
		}
	}

	return skipFlagOnly
}

// parseLongFlag handles long flags like --message, --signoff
func parseLongFlag(flag string, args []string, idx int, gitCmd *GitCommand) int {
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
		takesValue, exists := flagsWithValues[flag]
		if !exists || !takesValue {
			continue
		}

		// This flag takes a value
		if j == len(flags)-1 {
			// Last flag: consume next arg if available
			if idx+1 < len(args) {
				gitCmd.FlagMap[flag] = args[idx+1]
				return idx + skipFlagAndValue
			}
		} else {
			// Not last flag: rest of string is the inline value
			gitCmd.FlagMap[flag] = flags[j+1:]
			return idx + skipFlagOnly
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
	for _, flag := range []string{"-m", "--message"} {
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
	case "checkout":
		// git checkout [-b] <branch>
		// Branch name is always in Args (first positional arg)
		if len(g.Args) > 0 {
			return g.Args[0]
		}

	case "branch":
		// git branch [-m] <new-branch>
		// git branch <branch>
		if len(g.Args) > 0 {
			// Last arg is the branch name
			return g.Args[len(g.Args)-1]
		}

	case "switch":
		// git switch [-c] <branch>
		// git switch <branch>
		// Branch name is always in Args (first positional arg)
		if len(g.Args) > 0 {
			return g.Args[0]
		}

	case "push", "pull":
		// git push/pull <remote> <branch>
		if len(g.Args) > 1 {
			return g.Args[1]
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
