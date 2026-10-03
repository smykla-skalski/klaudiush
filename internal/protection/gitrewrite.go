package protection

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// statusPrefix is the length of "XY " before a path in git status output.
const statusPrefix = 3

// gitTimeout bounds one git query made to see what a command would rewrite.
const gitTimeout = 3 * time.Second

// maxPatchBytes bounds how much of a patch file is read for its file names.
const maxPatchBytes = 4 << 20

// patchFileLine matches the file names of a unified or git diff.
var patchFileLine = regexp.MustCompile(
	`(?m)^(?:diff --git a/(\S+) b/(\S+)|(?:\+\+\+|---) (?:[ab]/)?([^\t\n]+)|rename (?:from|to) (.+))$`,
)

// gitRunner runs git for the protection checks; tests replace it.
var gitRunner execpkg.CommandRunner = execpkg.NewCommandRunner(gitTimeout)

// gitRewrite describes how a git subcommand rewrites the working tree
// without naming the files: which file states it discards and which
// revisions it brings in.
type gitRewrite struct {
	untracked bool
	ignored   bool
	modified  bool
	opaque    bool
	ref       string
	stash     string
	pathspecs []string
	diffs     [][]string
	patches   []string
}

// ReasonGitRevision names a git ref update to a revision klaudiush cannot
// see, such as one a command substitution creates.
const ReasonGitRevision = "git ref update to an unknown revision"

// ReasonGitRepository names a git command that writes the work tree from a
// repository, index or work tree set on the command line.
const ReasonGitRepository = "git work tree write from another repository or index"

// gitEnvironment matches settings that point git at another repository,
// index, object store or work tree: GIT_DIR=..., core.worktree, --git-dir.
var gitEnvironment = regexp.MustCompile(
	`\bGIT_(?:DIR|WORK_TREE|INDEX_FILE|OBJECT_DIRECTORY|ALTERNATE_OBJECT_DIRECTORIES|COMMON_DIR|` +
		`CONFIG_COUNT|CONFIG_KEY_\d+|CONFIG_VALUE_\d+|CONFIG_PARAMETERS|CONFIG|NAMESPACE)\s*=|` +
		`(?i:core\.worktree)|--git-dir|--work-tree|--namespace`,
)

// worktreeWriters are git subcommands, besides gitPathCommands, that
// write work tree files.
var worktreeWriters = map[string]bool{
	"merge": true, "rebase": true, "cherry-pick": true, "revert": true, "pull": true,
	"sparse-checkout": true, "submodule": true, "bisect": true,
}

// redirectsGit reports a git command that writes the work tree while the
// command line points git at another repository, index or work tree. The
// other checks compare against the project's own repository, so such a
// command cannot be judged and counts.
func (c *commandCheck) redirectsGit(cmd parser.Command) bool {
	sub, _ := gitSplit(cmd.Args)

	if !worktreeWriters[sub] && !gitPathCommands[sub] {
		return false
	}

	return namesGitEnvironment(c.raw) || shellEnvironmentOpaque.MatchString(c.raw) ||
		c.loadsUnknownShellState()
}

// gitEnvironmentName matches the variables that point git elsewhere, in
// any position: export $x after x=GIT_DIR, read GIT_DIR, printf -v.
var gitEnvironmentName = regexp.MustCompile(
	`GIT_(?:DIR|WORK_TREE|INDEX_FILE|OBJECT_DIRECTORY|ALTERNATE_OBJECT_DIRECTORIES|COMMON_DIR|` +
		`CONFIG|NAMESPACE)`,
)

// shellEnvironmentOpaque matches commands that set variables whose names
// or values the text does not show (export $(cat f), env $(...),
// declare -x "$v", read into a variable later exported), and startup
// files a shell reads before running a command.
var shellEnvironmentOpaque = regexp.MustCompile(
	`\b(?:export|declare|typeset|readonly|local|env)\b[^;&|\n]*\s["']?[$` + "`" + `]|` +
		`\bset\s+-[a-zA-Z]*a|\bBASH_ENV\b|\bENV=|--rcfile|--init-file`,
)

// namesGitEnvironment reports a git environment variable or option in
// text, after removing the quotes and backslashes that split a name the
// shell joins ("GIT""_DIR", GIT\_DIR).
func namesGitEnvironment(text string) bool {
	joined := strings.NewReplacer(`"`, "", "'", "", `\`, "").Replace(text)

	return gitEnvironment.MatchString(joined) || gitEnvironmentName.MatchString(joined)
}

// loadsUnknownShellState reports a source, . or eval that may set git
// variables: a script that cannot be read or names one, or eval of text
// that comes from command output or names one. Sourcing a script that
// leaves git alone (a virtualenv's activate) passes.
func (c *commandCheck) loadsUnknownShellState() bool {
	return slices.ContainsFunc(c.result.Commands, func(cmd parser.Command) bool {
		switch programName(cmd) {
		case "source", ".":
			if len(cmd.Args) == 0 || cmd.Dynamic {
				return true
			}

			return scriptMaySetGit(
				c.set.absolute(c.expand(cmd.Args[0]), c.dir(cmd.WorkingDirectory, cmd.DirUnknown)),
			)
		case "eval":
			return cmd.Dynamic || namesGitEnvironment(strings.Join(cmd.Args, " "))
		default:
			return false
		}
	})
}

// maxSourcedBytes bounds how much of a sourced script is read.
const maxSourcedBytes = 1 << 20

// scriptMaySetGit reports a sourced script that cannot be read, or that
// names git variables, loads further state or sets variables opaquely.
func scriptMaySetGit(path string) bool {
	if strings.Contains(path, unknownPart) {
		return true
	}

	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSourcedBytes {
		return true
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return true
	}

	text := shellComment.ReplaceAllString(string(data), "")

	return namesGitEnvironment(text) || shellEnvironmentOpaque.MatchString(text) ||
		loadsMoreState.MatchString(text)
}

var (
	shellComment   = regexp.MustCompile(`(?m)(^|\s)#.*$`)
	loadsMoreState = regexp.MustCompile(`(?m)(^|[;&|]|\s)(source|\.|eval)\s`)
)

// movesCheckedOut reports whether ref is HEAD or the checked-out branch,
// so moving it changes what the work tree is compared with.
func (c *commandCheck) movesCheckedOut(dir, ref string) bool {
	if ref == gitHead {
		return true
	}

	current := strings.TrimSpace(c.git(dir, "symbolic-ref", "-q", gitHead))
	if current == "" {
		return true
	}

	return ref == current || "refs/heads/"+ref == current
}

// checkGitRewrite reports a protected file a git command would rewrite
// although the command names no path: clean, stash, reset --hard,
// checkout or switch to another revision, merge, cherry-pick, revert,
// rebase, apply and am.
func (c *commandCheck) checkGitRewrite(cmd parser.Command, dir string) (Match, bool) {
	if strings.HasPrefix(dir, unknownPart) {
		return Match{}, false
	}

	if c.redirectsGit(cmd) {
		return Match{Path: strings.Join(cmd.Args, " "), Reason: ReasonGitRepository}, true
	}

	rewrite, ok := gitRewriteOf(cmd.Args)
	if !ok {
		return Match{}, false
	}

	if rewrite.ref != "" && !c.movesCheckedOut(dir, rewrite.ref) {
		return Match{}, false
	}

	if rewrite.opaque || (cmd.Dynamic && len(rewrite.diffs) > 0) {
		return Match{Path: strings.Join(cmd.Args, " "), Reason: ReasonGitRevision}, true
	}

	top := c.git(dir, "rev-parse", "--show-toplevel")
	if top == "" {
		return Match{}, false
	}

	top = strings.TrimSpace(top)

	if m, found := c.checkGitStatus(top, dir, rewrite); found {
		return m, true
	}

	for _, diff := range rewrite.diffs {
		args := append([]string{"diff", "--name-only", "-z"}, diff...)
		if m, found := c.checkGitNames(top, c.git(top, args...)); found {
			return m, true
		}
	}

	if rewrite.stash != "" {
		names := c.git(
			top,
			"stash",
			"show",
			"--name-only",
			"-z",
			"--include-untracked",
			rewrite.stash,
		)
		if m, found := c.checkGitNames(top, names); found {
			return m, true
		}
	}

	for _, patch := range rewrite.patches {
		if m, found := c.checkPatchFile(c.set.absolute(c.expand(patch), dir), top); found {
			return m, true
		}
	}

	return Match{}, false
}

// gitRewriteOf says what a git command line rewrites without naming it.
func gitRewriteOf(args []string) (gitRewrite, bool) {
	sub, rest := gitSplit(args)
	operands := gitOperands(rest)

	switch sub {
	case "clean":
		if hasShortFlag(rest, 'n') || slices.Contains(rest, "--dry-run") {
			return gitRewrite{}, false
		}

		return gitRewrite{
			untracked: true,
			ignored:   hasShortFlag(rest, 'x') || hasShortFlag(rest, 'X'),
			pathspecs: operands,
		}, true
	case "stash":
		return stashRewrite(rest, operands), true
	case "reset":
		return resetRewrite(rest, operands)
	case "checkout", "switch":
		return checkoutRewrite(rest, operands)
	case "read-tree":
		if !hasShortFlag(rest, 'u') {
			return gitRewrite{}, false
		}

		return gitRewrite{modified: true, diffs: lastRevDiff(operands)}, true
	case "checkout-index":
		return gitRewrite{
			modified: true,
		}, hasShortFlag(rest, 'f') ||
			slices.Contains(rest, "--force")
	case "update-ref":
		return refMoveRewrite(operands, 1)
	case "symbolic-ref":
		return refMoveRewrite(operands, 1)
	case "branch":
		if !hasShortFlag(rest, 'f') && !slices.Contains(rest, "--force") {
			return gitRewrite{}, false
		}

		rewrite, ok := refMoveRewrite(operands, 1)
		if len(operands) > 0 {
			rewrite.ref = "refs/heads/" + operands[0]
		}

		return rewrite, ok
	default:
		return historyRewrite(sub, operands)
	}
}

// historyRewrite covers commands that bring in other commits or patches.
func historyRewrite(sub string, operands []string) (gitRewrite, bool) {
	var diffs [][]string

	switch sub {
	case "merge", "rebase":
		for _, rev := range operands {
			diffs = append(diffs, []string{"HEAD..." + rev})
		}
	case "cherry-pick", "revert":
		for _, rev := range operands {
			diffs = append(diffs, []string{rev + "^", rev})
		}
	case gitSubApply, "am":
		return gitRewrite{patches: operands}, len(operands) > 0
	default:
		return gitRewrite{}, false
	}

	return gitRewrite{diffs: diffs}, len(diffs) > 0
}

// resetRewrite: any reset to another revision moves HEAD, after which a
// protected file differs from it and the next forced checkout reverts it;
// --hard, --merge and --keep also discard changed files now.
func resetRewrite(rest, operands []string) (gitRewrite, bool) {
	if slices.Contains(rest, optEndOfOpts) {
		return gitRewrite{}, false
	}

	discards := slices.ContainsFunc(rest, func(arg string) bool {
		return arg == "--hard" || arg == "--merge" || arg == "--keep"
	})

	return gitRewrite{modified: discards, diffs: revDiffs(operands)},
		discards || len(operands) > 0
}

// checkoutRewrite: moving to another revision rewrites the files that
// differ, and --force also discards changed files. With -b, -B, -c or -C
// the revision is the start point after the new branch name.
func checkoutRewrite(rest, operands []string) (gitRewrite, bool) {
	if slices.Contains(rest, optEndOfOpts) {
		return gitRewrite{}, false
	}

	force := hasShortFlag(rest, 'f') || slices.Contains(rest, "--force") ||
		slices.Contains(rest, "--discard-changes")

	revs := operands
	if hasShortFlag(rest, 'b') || hasShortFlag(rest, 'B') || hasShortFlag(rest, 'c') ||
		hasShortFlag(rest, 'C') || slices.Contains(rest, "--orphan") {
		revs = nil
		if len(operands) > 1 {
			revs = operands[1:]
		}
	}

	return gitRewrite{modified: force, diffs: revDiffs(revs)}, force || len(revs) > 0
}

// refMoveRewrite compares HEAD with the revision a ref is set to, the
// operand after the ref name. A missing revision (one made by a command
// substitution) cannot be compared, so it counts.
func refMoveRewrite(operands []string, revIndex int) (gitRewrite, bool) {
	if len(operands) == 0 {
		return gitRewrite{}, false
	}

	ref := operands[0]

	if len(operands) <= revIndex {
		return gitRewrite{opaque: true, ref: ref}, true
	}

	return gitRewrite{diffs: [][]string{{gitHead, operands[revIndex]}}, ref: ref}, true
}

// lastRevDiff compares HEAD with the last operand, the tree read-tree reads.
func lastRevDiff(operands []string) [][]string {
	if len(operands) == 0 {
		return nil
	}

	return [][]string{{gitHead, operands[len(operands)-1]}}
}

func stashRewrite(rest, operands []string) gitRewrite {
	action := ""
	if len(operands) > 0 {
		action = operands[0]
	}

	switch action {
	case "pop", gitSubApply:
		ref := "stash@{0}"
		if len(operands) > 1 {
			ref = operands[1]
		}

		return gitRewrite{stash: ref}
	case "list", "show", "drop", "clear", "create", "store", "branch":
		return gitRewrite{}
	default:
		all := slices.Contains(rest, "--all") || hasShortFlag(rest, 'a')

		return gitRewrite{
			modified: true,
			untracked: all || slices.Contains(rest, "--include-untracked") ||
				hasShortFlag(rest, 'u'),
			ignored:   all,
			pathspecs: afterDoubleDash(rest),
		}
	}
}

// afterDoubleDash returns the words after "--".
func afterDoubleDash(args []string) []string {
	if i := slices.Index(args, optEndOfOpts); i >= 0 {
		return args[i+1:]
	}

	return nil
}

// revDiffs compares HEAD with the first operand, the revision a reset or
// checkout moves to.
func revDiffs(operands []string) [][]string {
	if len(operands) == 0 {
		return nil
	}

	return [][]string{{gitHead, operands[0]}}
}

// gitSplit returns the subcommand and the words after it.
func gitSplit(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		if gitOptionsWithValue[args[i]] {
			i++

			continue
		}

		if !strings.HasPrefix(args[i], "-") {
			return args[i], args[i+1:]
		}
	}

	return "", nil
}

// gitOperands returns the words that are not options, up to "--".
func gitOperands(rest []string) []string {
	var operands []string

	for _, arg := range rest {
		if arg == optEndOfOpts {
			break
		}

		if !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
		}
	}

	return operands
}

func hasShortFlag(args []string, flag rune) bool {
	return slices.ContainsFunc(args, func(arg string) bool {
		return strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.ContainsRune(arg[1:], flag)
	})
}

// checkGitStatus reports a protected file in a state the command discards.
func (c *commandCheck) checkGitStatus(top, dir string, rewrite gitRewrite) (Match, bool) {
	if !rewrite.untracked && !rewrite.ignored && !rewrite.modified {
		return Match{}, false
	}

	args := []string{
		wordStatus,
		"--porcelain=v1",
		"-z",
		"--untracked-files=all",
		"--ignored=matching",
	}
	if len(rewrite.pathspecs) > 0 {
		args = append(append(args, optEndOfOpts), rewrite.pathspecs...)
	}

	status := c.git(dir, args...)

	for entry := range strings.SplitSeq(status, "\x00") {
		if len(entry) <= statusPrefix {
			continue
		}

		code, name := entry[:2], entry[statusPrefix:]

		switch {
		case code == "??" && !rewrite.untracked,
			code == "!!" && !rewrite.ignored,
			code != "??" && code != "!!" && !rewrite.modified:
			continue
		}

		if m, ok := c.set.CheckTree(filepath.Join(top, name)); ok {
			return m, true
		}
	}

	return Match{}, false
}

// checkGitNames checks NUL-separated paths git printed, relative to top.
func (c *commandCheck) checkGitNames(top, names string) (Match, bool) {
	for name := range strings.SplitSeq(names, "\x00") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		if m, ok := c.set.Check(filepath.Join(top, name)); ok {
			return m, true
		}
	}

	return Match{}, false
}

// checkPatchFile reports a protected file a patch file changes. A patch
// that cannot be read counts when the command names a protected path.
func (c *commandCheck) checkPatchFile(path, base string) (Match, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxPatchBytes {
		if c.mentionsProtected() {
			return c.firstMention(), true
		}

		return Match{}, false
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Match{}, false
	}

	return c.checkPatchText(string(data), base)
}

// checkPatchText reports a protected file named in a diff. Names are tried
// as written and with their first component stripped, as patch -p1 does.
func (c *commandCheck) checkPatchText(text, base string) (Match, bool) {
	for _, m := range patchFileLine.FindAllStringSubmatch(text, -1) {
		for _, name := range m[1:] {
			name = strings.TrimSpace(name)
			if name == "" || name == "/dev/null" {
				continue
			}

			for _, candidate := range []string{name, stripFirst(name)} {
				if match, ok := c.set.Check(c.set.absolute(candidate, base)); ok {
					return match, true
				}
			}
		}
	}

	return Match{}, false
}

func stripFirst(name string) string {
	if _, rest, found := strings.Cut(name, "/"); found {
		return rest
	}

	return name
}

// checkPatchCommand reports a protected file the patch program changes,
// reading the patch from its -i option, file operand or stdin file.
func (c *commandCheck) checkPatchCommand(cmd parser.Command, dir string) (Match, bool) {
	var files []string

	for i := 0; i < len(cmd.Args); i++ {
		arg := cmd.Args[i]

		switch {
		case arg == "-i" || arg == "--input":
			if i+1 < len(cmd.Args) {
				files = append(files, cmd.Args[i+1])
				i++
			}
		case strings.HasPrefix(arg, "--input="):
			files = append(files, strings.TrimPrefix(arg, "--input="))
		case strings.HasPrefix(arg, "-"):
		default:
			files = append(files, arg)
		}
	}

	if cmd.StdinFile != "" {
		files = append(files, cmd.StdinFile)
	}

	if cmd.Stdin != "" {
		if m, ok := c.checkPatchText(cmd.Stdin, dir); ok {
			return m, true
		}
	}

	for _, file := range files {
		if m, ok := c.checkPatchFile(c.set.absolute(c.expand(file), dir), dir); ok {
			return m, true
		}
	}

	return Match{}, false
}

// gitOutput runs git in dir and returns its output, or "" when it fails.
func gitOutput(ctx context.Context, dir string, args ...string) string {
	result := gitRunner.Run(ctx, programGit, append([]string{optDir, dir}, args...)...)
	if result.Failed() {
		return ""
	}

	return result.Stdout
}
