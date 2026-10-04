package parser

import (
	"path/filepath"
	"slices"
	"strings"
)

// treeWritesOf returns the writes of programs that change files they do not
// name: archives, patches and git commands that rewrite the work tree.
func treeWritesOf(cmd Command) programWrite {
	switch cmd.Name {
	case "unzip":
		root, anywhere := optionDir(cmd.Args, nameSet("-d"))

		return programWrite{
			op: WriteOpUnpack, unknown: unzipExtracts(cmd.Args), root: root, anywhere: anywhere,
		}
	case "tar", "bsdtar", "gtar":
		root, anywhere := optionDir(cmd.Args, nameSet("-C --directory"))
		anywhere = anywhere || slices.ContainsFunc(cmd.Args, func(arg string) bool {
			return tarAbsoluteNames[arg] ||
				(strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 2 &&
					strings.ContainsAny(arg[1:], "CP"))
		})

		return programWrite{
			op: WriteOpUnpack, unknown: tarExtracts(cmd.Args), root: root, anywhere: anywhere,
		}
	case "patch":
		root, anywhere := optionDir(cmd.Args, nameSet("-d --directory"))

		return programWrite{
			op: WriteOpUnpack, unknown: patchApplies(cmd.Args), root: root, anywhere: anywhere,
		}
	case gitProgram:
		root, anywhere := gitTreeDir(cmd.Args)

		return programWrite{
			op: WriteOpUnpack, unknown: gitRewritesTree(cmd.Args),
			root: root, anywhere: anywhere, tree: true,
		}
	default:
		return programWrite{}
	}
}

// optionDir returns the directory the last of opts names, as NAME DIR or
// NAME=DIR, relative to the command's directory. A repeated option, or one
// whose value is built from command output, leaves the directory anywhere.
func optionDir(args []string, opts map[string]bool) (dir string, anywhere bool) {
	seen := 0

	for i, arg := range args {
		name, value, hasValue := strings.Cut(arg, "=")

		switch {
		case !opts[name]:
			continue
		case hasValue:
			dir = value
		case i+1 < len(args):
			dir = args[i+1]
		default:
			return "", true
		}

		seen++
	}

	return dir, seen > 1 || marked(dir) || HasUnresolvedVars(dir)
}

// gitTreeDir returns the directory git -C moves to before it looks for its
// work tree. --git-dir or --work-tree put the work tree anywhere.
func gitTreeDir(args []string) (dir string, anywhere bool) {
	idx := gitSubcommandIndex(args)
	if idx < 0 {
		return "", true
	}

	for i := 0; i < idx; i++ {
		name, _, _ := strings.Cut(args[i], "=")

		switch {
		case name == "--git-dir" || name == "--work-tree":
			return "", true
		case args[i] == flagUpperC && i+1 < idx && filepath.IsAbs(args[i+1]):
			dir = args[i+1]
			i++
		case args[i] == flagUpperC && i+1 < idx:
			dir = filepath.Join(dir, args[i+1])
			i++
		}
	}

	return dir, marked(dir) || HasUnresolvedVars(dir)
}

// tarAbsoluteNames keep leading slashes and .. in member names, so an
// archive may write anywhere.
var tarAbsoluteNames = nameSet("-P --absolute-names")

// unzipReadOnly are unzip options that list, test or print an archive
// instead of extracting it.
const unzipReadOnly = "lptvzZc"

// unzipExtracts reports unzip extracting an archive, which writes files the
// command line does not name.
func unzipExtracts(args []string) bool {
	operands := 0

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "-") && arg != "-" && !strings.HasPrefix(arg, "--"):
			if strings.ContainsAny(arg[1:], unzipReadOnly) {
				return false
			}
		case strings.HasPrefix(arg, "--"):
		default:
			operands++
		}
	}

	return operands > 0
}

// tarExtracts reports tar extracting an archive: -x in a short cluster,
// --extract or --get, or x in the dashless first word (tar xzf a.tgz).
func tarExtracts(args []string) bool {
	for i, arg := range args {
		switch {
		case arg == "--extract" || arg == "--get":
			return true
		case strings.HasPrefix(arg, "--"):
		case strings.HasPrefix(arg, "-"):
			if strings.Contains(arg[1:], "x") {
				return true
			}
		case i == 0 && strings.Contains(arg, "x"):
			return true
		}
	}

	return false
}

// patchApplies reports patch changing files, which are named in the patch
// rather than on the command line.
func patchApplies(args []string) bool {
	return !slices.ContainsFunc(args, func(arg string) bool {
		return arg == "--dry-run" || arg == "--check" || arg == "--help" || arg == "--version"
	})
}

// stashWrites are the git stash subcommands that change the work tree;
// stash without one pushes.
var stashWrites = nameSet("push save pop apply branch")

// treeRewriters are git subcommands that rewrite work tree files they do
// not name.
var treeRewriters = nameSet("merge pull rebase cherry-pick revert am")

// applyInspects are git apply options that only report on a patch unless
// --apply is given too.
var applyInspects = nameSet("--check --stat --numstat --summary")

// branchCreators create a branch from HEAD when given only its name.
var branchCreators = nameSet("-b -B -c -C --orphan")

// gitRewritesTree reports a git command that changes work tree files it
// does not name: reset --hard, --merge or --keep, stash push, pop, apply or
// branch, apply, checkout, switch and restore of the work tree, and merges,
// pulls, rebases and picks.
func gitRewritesTree(args []string) bool {
	idx := gitSubcommandIndex(args)
	if idx < 0 {
		return false
	}

	rest := args[idx+1:]

	switch sub := args[idx]; sub {
	case "reset":
		return slices.ContainsFunc(rest, func(arg string) bool {
			return arg == "--hard" || arg == "--merge" || arg == "--keep"
		})
	case "stash":
		return len(rest) == 0 || strings.HasPrefix(rest[0], "-") || stashWrites[rest[0]]
	case "apply":
		inspects := slices.ContainsFunc(
			rest,
			func(arg string) bool { return applyInspects[arg] },
		) &&
			!slices.Contains(rest, "--apply")
		indexOnly := slices.Contains(rest, "--cached") && !slices.Contains(rest, "--index")

		return !inspects && !indexOnly
	case "checkout", "switch":
		return !createsBranchOnly(rest)
	case "restore":
		staged := slices.Contains(rest, "--staged") || slices.Contains(rest, "-S")
		worktree := slices.Contains(rest, "--worktree") || slices.Contains(rest, "-W")

		return !staged || worktree
	default:
		return treeRewriters[sub]
	}
}

// createsBranchOnly reports checkout -b NAME or switch -c NAME with no start
// point or paths, which keeps the work tree as it is.
func createsBranchOnly(args []string) bool {
	if len(args) != 2 || !branchCreators[args[0]] {
		return false
	}

	return !strings.HasPrefix(args[1], "-")
}
