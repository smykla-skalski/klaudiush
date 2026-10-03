package protection

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// effect says which arguments of a command can name a file it changes.
type effect int

const (
	effectNone effect = iota
	effectDest
	effectAll
)

// Program names checked by name.
const (
	programFind      = "find"
	programGit       = "git"
	programDitto     = "ditto"
	programPatch     = "patch"
	programKlaudiush = "klaudiush"
	programRm        = "rm"
	gitHead          = "HEAD"
	wordStatus       = "status"
)

// Option spellings shared by several programs.
const (
	optDir       = "-C"
	optEndOfOpts = "--"
	optTargetDir = "--target-directory"
	optTarget    = "-t"
	gitSubApply  = "apply"
)

// readOnlyPrograms only read the files they name. Their output goes to
// stdout, where a redirect, which is checked on its own, may send it.
var readOnlyPrograms = map[string]bool{
	"[": true, "[[": true, "ack": true, "ag": true, "b2sum": true, "basename": true,
	"bat": true, "cat": true, "cd": true, "cksum": true, "cmp": true, "column": true,
	"comm": true, "cut": true, "date": true, "df": true, "diff": true, "dirname": true,
	"du": true, "echo": true, "egrep": true, "exit": true, "expand": true, "false": true,
	"fgrep": true, "file": true, "fmt": true, "fold": true, "gh": true, "gojq": true,
	"grep": true, "head": true, "hexdump": true, "id": true, "jq": true, "join": true,
	"less": true, "ls": true, "look": true, "md5": true, "md5sum": true, "mkdir": true,
	"more": true, "nl": true, "od": true, "paste": true, "popd": true, "printf": true,
	"pushd": true, "pwd": true, "readlink": true, "realpath": true, "rev": true,
	"rg": true, "sha1sum": true, "sha224sum": true, "sha256sum": true, "sha384sum": true,
	"sha512sum": true, "shasum": true, "sleep": true, "stat": true, "strings": true,
	"tac": true, "tail": true, "test": true, "tr": true, "tree": true, "true": true,
	"type": true, "unexpand": true, "wc": true, "whereis": true, "which": true,
	"whoami": true, "xxd": true,
}

// broadPrograms delete, move, rewrite or change permissions of whole trees,
// so naming the working directory or an ancestor of it with one of them
// takes in the protected files below.
var broadPrograms = map[string]bool{
	"chattr": true, "chflags": true, "chgrp": true, "chmod": true, "chown": true,
	"cp": true, "cpio": true, "dd": true, programDitto: true, programFind: true, programGit: true,
	"install": true, "ln": true, "mv": true, "perl": true, programRm: true, "rmdir": true,
	"rsync": true, "sed": true, "setfacl": true, "shred": true, "srm": true,
	"tar": true, "touch": true, "trash": true, "truncate": true, "unlink": true,
	"unzip": true, "xattr": true,
}

// destOnlyPrograms copy from their sources into a destination: only the
// destination changes, unless they link or remove the sources.
var destOnlyPrograms = map[string]bool{
	"cp": true, programDitto: true, "install": true, "rsync": true, "scp": true,
}

// gitPathCommands are git subcommands that change the working tree files
// they name.
var gitPathCommands = map[string]bool{
	"am": true, gitSubApply: true, "checkout": true, "checkout-index": true, "clean": true,
	"config": true, "mv": true, "read-tree": true, "reset": true, "restore": true,
	programRm: true, "stash": true, "switch": true, "update-index": true, "worktree": true,
}

// gitOptionsWithValue are git global options that take the next word.
var gitOptionsWithValue = map[string]bool{
	optDir: true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--exec-path": true, "--super-prefix": true, "--config-env": true,
}

var (
	sedWriteCommand = regexp.MustCompile(`(^|[/;}\s])[wW]\s*\S`)
	awkWrites       = regexp.MustCompile(`>|\bsystem\b|\binplace\b|\bclose\b|\|`)
)

var findWriteActions = []string{
	"-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0",
	"-fprintf", "-fls",
}

// programName returns the program a command runs, lower-cased and without
// its directory.
func programName(cmd parser.Command) string {
	return strings.ToLower(filepath.Base(cmd.Name))
}

// commandEffect says which arguments of cmd can name a file it changes.
func commandEffect(cmd parser.Command) effect {
	name := programName(cmd)

	switch {
	case readOnlyPrograms[name], isKlaudiush(cmd):
		return effectNone
	case name == "sed":
		return flagEffect(sedInPlace(cmd.Args))
	case name == "sort":
		return flagEffect(hasOption(cmd.Args, "-o", "--output"))
	case name == "yq":
		return flagEffect(hasOption(cmd.Args, "-i", "--inplace"))
	case name == programFind:
		return flagEffect(slices.ContainsFunc(cmd.Args, func(arg string) bool {
			return slices.Contains(findWriteActions, arg)
		}))
	case name == "awk" || name == "gawk" || name == "mawk" || name == "nawk":
		return flagEffect(slices.ContainsFunc(cmd.Args, awkWrites.MatchString))
	case name == programGit:
		return flagEffect(gitPathCommands[gitSubcommand(cmd.Args)])
	case destOnlyPrograms[name] && !linksOrRemovesSources(cmd.Args):
		return effectDest
	default:
		return effectAll
	}
}

func flagEffect(changes bool) effect {
	if changes {
		return effectAll
	}

	return effectNone
}

func sedInPlace(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--in-place") {
			return true
		}

		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.ContainsRune(strings.TrimLeft(arg, "-"), 'i') {
			return true
		}

		if !strings.HasPrefix(arg, "-") && sedWriteCommand.MatchString(arg) {
			return true
		}
	}

	return false
}

// hasOption reports whether args carry one of the options, alone, with an
// attached value or (for a short option) bundled with others.
func hasOption(args []string, short, long string) bool {
	for _, arg := range args {
		if arg == long || strings.HasPrefix(arg, long+"=") {
			return true
		}

		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.ContainsRune(arg[1:], rune(short[1])) {
			return true
		}
	}

	return false
}

func linksOrRemovesSources(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--link", arg == "--symbolic-link", arg == "--remove-source-files",
			strings.HasPrefix(arg, "--link-dest"):
			return true
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.ContainsAny(arg[1:], "ls"):
			return true
		}
	}

	return false
}

// gitSubcommand returns the git subcommand, skipping global options.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if gitOptionsWithValue[arg] {
			i++

			continue
		}

		if strings.HasPrefix(arg, "-") {
			continue
		}

		return arg
	}

	return ""
}

// gitDir returns the directory git -C moves to, relative to dir.
func gitDir(args []string, dir string) string {
	for i := 0; i+1 < len(args); i++ {
		arg := args[i]

		switch {
		case arg == optDir:
			if next := args[i+1]; filepath.IsAbs(next) {
				dir = next
			} else {
				dir = filepath.Join(dir, next)
			}

			i++
		case gitOptionsWithValue[arg]:
			i++
		case !strings.HasPrefix(arg, "-"):
			return dir
		}
	}

	return dir
}

// destinations returns the destination operands of a copy: the
// --target-directory value, or the last operand.
func destinations(args []string) []string {
	var operands []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == optTarget || arg == optTargetDir:
			if i+1 < len(args) {
				return []string{args[i+1]}
			}
		case strings.HasPrefix(arg, optTargetDir+"="):
			return []string{strings.TrimPrefix(arg, optTargetDir+"=")}
		case arg == optEndOfOpts:
			operands = append(operands, args[i+1:]...)
			i = len(args)
		case copyValueOptions[arg]:
			i++
		case strings.HasPrefix(arg, "-"):
		default:
			operands = append(operands, arg)
		}
	}

	if len(operands) == 0 {
		return nil
	}

	return operands[len(operands)-1:]
}

// copyValueOptions are copy options that take the next word.
var copyValueOptions = map[string]bool{
	"-m": true, "-o": true, "-g": true, "-S": true, optTarget: true, "--mode": true,
	"--owner": true, "--group": true, "--suffix": true, optTargetDir: true,
}

// sources returns every operand of a copy but the destination: all of them
// when -t names the destination.
func sources(args []string) []string {
	var operands []string

	target := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == optTarget || arg == optTargetDir ||
			strings.HasPrefix(arg, optTargetDir+"="):
			target = true

			if !strings.Contains(arg, "=") {
				i++
			}
		case copyValueOptions[arg]:
			i++
		case arg == optEndOfOpts:
			operands = append(operands, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(arg, "-"):
		default:
			operands = append(operands, arg)
		}
	}

	if target {
		return operands
	}

	if len(operands) <= 1 {
		return nil
	}

	return operands[:len(operands)-1]
}

// isTargetOption reports the -t or --target-directory option of a copy.
func isTargetOption(arg string) bool {
	return arg == optTarget || arg == optTargetDir || strings.HasPrefix(arg, optTargetDir+"=")
}
