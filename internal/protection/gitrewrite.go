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
	stash     string
	diffs     [][]string
	patches   []string
}

// checkGitRewrite reports a protected file a git command would rewrite
// although the command names no path: clean, stash, reset --hard,
// checkout or switch to another revision, merge, cherry-pick, revert,
// rebase, apply and am.
func (c *commandCheck) checkGitRewrite(cmd parser.Command, dir string) (Match, bool) {
	if strings.HasPrefix(dir, unknownPart) {
		return Match{}, false
	}

	rewrite, ok := gitRewriteOf(cmd.Args)
	if !ok {
		return Match{}, false
	}

	top := c.git(dir, "rev-parse", "--show-toplevel")
	if top == "" {
		return Match{}, false
	}

	top = strings.TrimSpace(top)

	if m, found := c.checkGitStatus(top, rewrite); found {
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
		return gitRewrite{
			untracked: true,
			ignored:   hasShortFlag(rest, 'x') || hasShortFlag(rest, 'X'),
		}, true
	case "stash":
		return stashRewrite(rest, operands), true
	case "reset":
		if !slices.ContainsFunc(rest, func(arg string) bool {
			return arg == "--hard" || arg == "--merge" || arg == "--keep"
		}) {
			return gitRewrite{}, false
		}

		return gitRewrite{modified: true, diffs: revDiffs(operands)}, true
	case "checkout", "switch":
		if slices.Contains(rest, optEndOfOpts) {
			return gitRewrite{}, false
		}

		return gitRewrite{diffs: revDiffs(operands)}, len(operands) > 0
	case "merge", "rebase":
		var diffs [][]string
		for _, rev := range operands {
			diffs = append(diffs, []string{"HEAD..." + rev})
		}

		return gitRewrite{diffs: diffs}, len(diffs) > 0
	case "cherry-pick", "revert":
		var diffs [][]string
		for _, rev := range operands {
			diffs = append(diffs, []string{rev + "^", rev})
		}

		return gitRewrite{diffs: diffs}, len(diffs) > 0
	case gitSubApply, "am":
		return gitRewrite{patches: operands}, len(operands) > 0
	default:
		return gitRewrite{}, false
	}
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
			ignored: all,
		}
	}
}

// revDiffs compares HEAD with the first operand, the revision a reset or
// checkout moves to.
func revDiffs(operands []string) [][]string {
	if len(operands) == 0 {
		return nil
	}

	return [][]string{{"HEAD", operands[0]}}
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
func (c *commandCheck) checkGitStatus(top string, rewrite gitRewrite) (Match, bool) {
	if !rewrite.untracked && !rewrite.ignored && !rewrite.modified {
		return Match{}, false
	}

	status := c.git(
		top,
		"status",
		"--porcelain=v1",
		"-z",
		"--untracked-files=all",
		"--ignored=matching",
	)

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
