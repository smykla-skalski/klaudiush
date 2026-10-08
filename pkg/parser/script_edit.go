package parser

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// DetailScriptEdited is the Opacity.Detail of a script whose content
// klaudiush captured from a write earlier on the line, when a command
// between that write and the run may have edited the file.
const DetailScriptEdited = "a command after it is written on the line may edit it, " +
	"so what runs may differ from the content klaudiush saw"

// readOnlyPrograms leave the content of the files they name as it is:
// readers, filters, checksums, system status and metadata changes. A
// redirect on one of them is a write klaudiush tracks on its own.
var readOnlyPrograms = nameSet(`cat head tail less more wc ls stat file grep egrep fgrep
	rg diff cmp test [ md5sum md5 shasum sha1sum sha256sum chmod chown chgrp touch echo
	printf realpath readlink basename dirname du true false pwd cd mkdir jq tr cut column
	nl od hexdump sleep date which uname whoami id hostname printenv tput clear seq base64
	df ps free nproc uptime curl wget`)

// editPrograms change files they do not name: builds, archives, patches and
// syncs.
var editPrograms = nameSet(`make gmake bmake just task ninja unzip tar bsdtar cpio 7z rsync
	patch install`)

// readOnlyGit are the git subcommands that leave work tree files as they
// are. Any other may rewrite them (checkout, restore, apply, stash, reset).
var readOnlyGit = nameSet(`add diff status log show ls-files blame commit fetch push
	branch tag remote config rev-parse describe shortlog grep`)

// codeFlags hand an interpreter its program as an argument (python -c,
// node -e, ruby -e, perl -e) or name a module it runs (python -m).
var codeFlags = nameSet("-c -e -E --eval -p --print -m -r")

// fileEdits matches program source that may change files: writes, dumps,
// renames, removals, permission changes, output redirection or starting
// another program, which may do any of these.
var fileEdits = regexp.MustCompile(
	`(?i)write|open\s*\(|dump\s*\(|rename|os\.replace|copy|move|unlink|os\.remove|` +
		`truncate|chmod|rmtree|shutil|system|popen|subprocess|exec|spawn|>`,
)

// lastLineWriteSeq returns the execution position of the last write
// lastLineWrite reads target from, or 0 when it is unknown.
func (w *astWalker) lastLineWriteSeq(target string) int {
	for p := w; p != nil; p = p.parent {
		seq, found := 0, false

		for _, fw := range p.fileWrites {
			if path, known := w.writtenPath(fw); known && filepath.Clean(path) == target {
				seq, found = fw.Location.Seq, true
			}
		}

		if found {
			return seq
		}
	}

	return 0
}

// editedAfter reports a command run after the write at seq and before cmd
// that may change target without klaudiush capturing the result (see
// mayEdit). Writes klaudiush tracks (redirects, tee, cp, mv, sed -i) are
// handled by lastLineWrite, which then stops trusting the capture.
func (w *astWalker) editedAfter(cmd Command, target string, seq int) bool {
	if seq == 0 || cmd.Location.Seq == 0 {
		return false
	}

	for earlier := range w.earlierCommands() {
		if earlier.Location.Seq <= seq || earlier.Location.Seq >= cmd.Location.Seq {
			continue
		}

		if w.mayEdit(earlier, target) {
			return true
		}
	}

	return false
}

// mayEdit reports whether cmd may change target without klaudiush seeing
// the write. Shells, launchers, builtins and functions and aliases defined
// on the line are left out: the commands they run are recorded and judged
// on their own. An interpreter counts when its program may change files;
// git when its subcommand may rewrite the work tree; editPrograms always;
// any other program outside readOnlyPrograms when an operand may stand for
// target: the file itself, a directory, a glob or a word klaudiush cannot
// resolve.
func (w *astWalker) mayEdit(cmd Command, target string) bool {
	name := commandName(cmd.Name)

	if spec, ok := interpreters[name]; ok {
		return w.interpreterEdits(cmd, spec, target)
	}

	_, launcher := launchers[name]
	_, function := w.funcs[cmd.Name]
	_, alias := w.aliases[cmd.Name]

	switch {
	case readOnlyPrograms[name] || shells[name] || shellBuiltins[name] || launcher ||
		function || alias:
		return false
	case editPrograms[name]:
		return true
	case name == gitProgram:
		return gitEdits(cmd)
	case name == "find":
		return slices.ContainsFunc(cmd.Args, func(arg string) bool {
			return arg == "-delete" || strings.HasPrefix(arg, "-fprint")
		})
	}

	return slices.ContainsFunc(cmd.Args, func(arg string) bool {
		return w.mayName(arg, target)
	})
}

// gitEdits reports whether a git command may rewrite work tree files. A
// new branch (checkout -b, switch -c) leaves them as they are.
func gitEdits(cmd Command) bool {
	gitCmd, err := ParseGitCommand(cmd)
	if err != nil {
		return true
	}

	switch gitCmd.Subcommand {
	case "checkout":
		return !gitCmd.HasFlag("-b") && !gitCmd.HasFlag("-B")
	case "switch":
		return !gitCmd.HasFlag("-c") && !gitCmd.HasFlag("-C")
	default:
		return !readOnlyGit[gitCmd.Subcommand]
	}
}

// mayName reports whether an argument may name target: it is the file
// (alone or as a flag value), a directory above it, a glob or a word
// klaudiush cannot resolve (a variable left after expansion). A URL names
// no local file.
func (w *astWalker) mayName(arg, target string) bool {
	if strings.Contains(arg, "://") {
		return false
	}

	if strings.Contains(arg, "$") {
		arg = w.expandName(arg)
	}

	if marked(arg) || strings.Contains(arg, "$") || arg == "{}" || strings.ContainsAny(arg, "*?[") {
		return true
	}

	if _, value, ok := strings.Cut(arg, "="); ok && strings.HasPrefix(arg, "-") {
		arg = value
	}

	base := filepath.Base(target)
	clean := filepath.Clean(arg)

	return clean == base || strings.HasSuffix(clean, string(filepath.Separator)+base) ||
		clean == "." || clean == parentDir || strings.HasSuffix(arg, "/")
}

// interpreterEdits reports whether an interpreter run may change target:
// its inline program, its program on stdin or the script it runs may change
// files or names target. Running target itself, whose content klaudiush
// has, does not count, nor does a module (python -m) or inline code that
// only reads. A script is read from its capture on the line or from disk,
// never through scriptSource, which would judge earlier edits again for
// every script and grow with each one.
func (w *astWalker) interpreterEdits(cmd Command, spec interpreter, target string) bool {
	operands := make([]string, 0, len(cmd.Args))
	inline := spec.codeFirst

	for _, arg := range cmd.Args {
		if codeFlags[arg] {
			inline = true
		}

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			operands = append(operands, arg)
		}
	}

	if inline {
		return codeEdits(strings.Join(cmd.Args, " "), target)
	}

	if len(operands) == 0 || operands[0] == "-" || operands[0] == devStdin {
		return cmd.Stdin == "" || codeEdits(cmd.Stdin, target)
	}

	script := filepath.Clean(w.trackedPath(cmd.WorkingDirectory, w.expandName(operands[0])))
	if script == target {
		return false
	}

	text, found, captured := w.lastLineWrite(script)
	if found {
		return !captured || codeEdits(text, target)
	}

	text, status := w.resolver.ReadScript(script)

	return status != ScriptText || codeEdits(text, target)
}

// codeEdits reports whether program source may change files or names
// target.
func codeEdits(code, target string) bool {
	return fileEdits.MatchString(code) || strings.Contains(code, filepath.Base(target))
}
