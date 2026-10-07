package parser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

var (
	// ErrNotGHCommand is returned when the command is not a gh command.
	ErrNotGHCommand = errors.New("not a gh command")

	// ErrNotPRMergeCommand is returned when the gh command is not a pr merge command.
	ErrNotPRMergeCommand = errors.New("not a gh pr merge command")

	// ErrNoPRNumber is returned when the PR number cannot be determined.
	ErrNoPRNumber = errors.New("cannot determine PR number")

	// prURLRegex matches GitHub PR URLs.
	prURLRegex = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/(\d+)`)
)

const (
	ghCLI               = "gh"
	prSubCmd            = "pr"
	mergeSubCmd         = "merge"
	minGHPRMergeArgsLen = 2 // gh pr merge
)

// GHMergeCommand represents a parsed gh pr merge command.
type GHMergeCommand struct {
	// PRNumber is the PR number to merge (0 if not specified, uses current branch's PR).
	PRNumber int

	// Squash indicates if --squash flag is present.
	Squash bool

	// Merge indicates if --merge flag is present (create a merge commit).
	Merge bool

	// Rebase indicates if --rebase flag is present.
	Rebase bool

	// Auto indicates if --auto flag is present (enable auto-merge).
	Auto bool

	// DisableAuto indicates if --disable-auto flag is present.
	DisableAuto bool

	// Delete indicates if --delete-branch or -d flag is present.
	Delete bool

	// Admin indicates if --admin flag is present (bypass branch protection).
	Admin bool

	// Subject is the merge commit subject from --subject or -t flag.
	Subject string

	// Body is the merge commit body from --body or -b flag.
	Body string

	// BodyFile is the file path for merge commit body from --body-file or -F flag.
	BodyFile string

	// Match indicates if --match-head-commit flag is present.
	Match string

	// Repo is the repository from --repo or -R flag.
	Repo string

	// Hostname is the GitHub host of a merge made through the REST API, empty
	// for the default host. gh pr merge carries any host inside Repo instead.
	Hostname string

	// APIPath is the REST path of the pull request a REST merge targets, such
	// as repos/o/r/pulls/42. gh pr merge leaves it empty.
	APIPath string

	// RawArgs contains all the raw arguments for debugging.
	RawArgs []string
}

// ParseGHMergeCommand parses a Command into a GHMergeCommand.
func ParseGHMergeCommand(cmd Command) (*GHMergeCommand, error) {
	if cmd.Name != ghCLI {
		return nil, ErrNotGHCommand
	}

	if len(cmd.Args) < minGHPRMergeArgsLen {
		return nil, ErrNotPRMergeCommand
	}

	// Check if it's a pr merge command
	if cmd.Args[0] != prSubCmd || cmd.Args[1] != mergeSubCmd {
		return nil, ErrNotPRMergeCommand
	}

	ghCmd := &GHMergeCommand{
		RawArgs: cmd.Args,
	}

	// Parse arguments after "pr merge"
	args := cmd.Args[2:]
	i := 0

	for i < len(args) {
		skip := ghCmd.parseArg(args, i)
		i += skip
	}

	return ghCmd, nil
}

// parseArg parses a single argument and returns how many args to skip.
func (c *GHMergeCommand) parseArg(args []string, idx int) int {
	arg := args[idx]

	// Check boolean flags
	if c.parseBooleanFlag(arg) {
		return 1
	}

	// Check value flags (--flag value format)
	if skip := c.parseValueFlag(args, idx); skip > 0 {
		return skip
	}

	// Check --flag=value format
	if c.parseEqualFlag(arg) {
		return 1
	}

	// Positional argument (PR number or URL)
	if !strings.HasPrefix(arg, "-") {
		c.parsePositionalArg(arg)
	}

	return 1
}

// parseBooleanFlag handles boolean flags. Returns true if matched.
func (c *GHMergeCommand) parseBooleanFlag(arg string) bool {
	switch arg {
	case "--squash", "-s":
		c.Squash = true
	case "--merge", "-m":
		c.Merge = true
	case "--rebase", "-r":
		c.Rebase = true
	case "--auto":
		c.Auto = true
	case "--disable-auto":
		c.DisableAuto = true
	case "--delete-branch", "-d":
		c.Delete = true
	case "--admin":
		c.Admin = true
	default:
		return false
	}

	return true
}

// parseValueFlag handles --flag value format. Returns args to skip (0 if not matched).
func (c *GHMergeCommand) parseValueFlag(args []string, idx int) int {
	if idx+1 >= len(args) {
		return 0
	}

	arg := args[idx]

	switch arg {
	case "--subject", "-t":
		c.Subject = args[idx+1]
	case "--body", "-b":
		c.Body = args[idx+1]
	case "--body-file", "-F":
		c.BodyFile = args[idx+1]
	case "--match-head-commit":
		c.Match = args[idx+1]
	case "--repo", "-R":
		c.Repo = args[idx+1]
	default:
		return 0
	}

	return 2 //nolint:mnd // Skip flag and its value
}

// parseEqualFlag handles --flag=value format. Returns true if matched.
func (c *GHMergeCommand) parseEqualFlag(arg string) bool {
	switch {
	case strings.HasPrefix(arg, "--subject="), strings.HasPrefix(arg, "-t="):
		c.Subject = extractFlagValue(arg)
	case strings.HasPrefix(arg, "--body="), strings.HasPrefix(arg, "-b="):
		c.Body = extractFlagValue(arg)
	case strings.HasPrefix(arg, "--body-file="), strings.HasPrefix(arg, "-F="):
		c.BodyFile = extractFlagValue(arg)
	case strings.HasPrefix(arg, "--match-head-commit="):
		c.Match = extractFlagValue(arg)
	case strings.HasPrefix(arg, "--repo="), strings.HasPrefix(arg, "-R="):
		c.Repo = extractFlagValue(arg)
	default:
		return false
	}

	return true
}

// parsePositionalArg handles positional arguments (PR number or URL).
func (c *GHMergeCommand) parsePositionalArg(arg string) {
	// Try to parse as PR number
	if prNum, err := strconv.Atoi(arg); err == nil {
		c.PRNumber = prNum

		return
	}

	// Could be a URL or branch name - try to extract PR number from URL
	c.PRNumber = extractPRNumberFromURL(arg)
}

// IsSquashMerge returns true if this is a squash merge.
// Squash is the default merge method if no method is specified.
func (c *GHMergeCommand) IsSquashMerge() bool {
	// If no method specified, depends on repo default (often squash)
	// If --squash is specified, it's definitely a squash merge
	// If --merge or --rebase is specified, it's not a squash merge
	if c.Merge || c.Rebase {
		return false
	}

	return true // Squash is default if no method specified
}

// IsAutoMerge returns true if this enables auto-merge.
func (c *GHMergeCommand) IsAutoMerge() bool {
	return c.Auto
}

// NeedsPRFetch returns true if we need to fetch PR details from GitHub.
// This is needed when PR number is not specified (uses current branch's PR)
// or when we need to validate the merge message.
func (*GHMergeCommand) NeedsPRFetch() bool {
	return true // Always need to fetch PR details for validation
}

// extractFlagValue extracts the value from --flag=value or -f=value format.
func extractFlagValue(arg string) string {
	parts := strings.SplitN(arg, "=", 2) //nolint:mnd // Split into key=value pair
	if len(parts) == 2 {                 //nolint:mnd // Check for key=value pair
		return parts[1]
	}

	return ""
}

// extractPRNumberFromURL extracts PR number from a GitHub PR URL.
func extractPRNumberFromURL(url string) int {
	matches := prURLRegex.FindStringSubmatch(url)
	if len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}

	return 0
}

// IsGHPRMerge checks if a command is a gh pr merge command.
func IsGHPRMerge(cmd *Command) bool {
	if cmd.Name != ghCLI {
		return false
	}

	if len(cmd.Args) < minGHPRMergeArgsLen {
		return false
	}

	return cmd.Args[0] == prSubCmd && cmd.Args[1] == mergeSubCmd
}
