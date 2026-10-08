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
// readers, filters, checksums and metadata changes. A redirect on one of
// them is a write klaudiush tracks on its own.
var readOnlyPrograms = nameSet(`cat head tail less more wc ls stat file grep egrep fgrep
	rg diff cmp test [ md5sum md5 shasum sha1sum sha256sum chmod chown chgrp touch echo
	printf realpath readlink basename dirname du true false pwd cd mkdir sort uniq jq tr
	cut column nl xxd od hexdump sleep date which uname whoami id hostname printenv tput
	clear seq base64`)

// readOnlyGit are the git subcommands that leave work tree files as they
// are. Any other may rewrite them (checkout, restore, apply, stash, reset).
var readOnlyGit = nameSet(`add diff status log show ls-files blame commit fetch push
	branch tag remote config rev-parse describe shortlog grep`)

// codeFlags hand an interpreter its program as an argument (python -c,
// node -e, ruby -e, perl -e) or name a module it runs (python -m).
var codeFlags = nameSet("-c -e -E --eval -p --print -m -r")

// fileEdits matches program source that may change files: writes, renames,
// removals, permission changes, output redirection or starting another
// program, which may do any of these.
var fileEdits = regexp.MustCompile(
	`(?i)write|open\s*\(|dump|rename|os\.replace|copy|move|unlink|remove|truncate|` +
		`chmod|rmtree|system|popen|subprocess|exec|spawn|>`,
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
// the write. Shells, launchers and builtins are left out: the commands they
// run are recorded and judged on their own. An interpreter counts when its
// program may change files; git when its subcommand may rewrite the work
// tree; any other program outside readOnlyPrograms when it takes no
// operands (make) or one may stand for target: the file itself, a
// directory, a glob or a word klaudiush cannot resolve.
func (w *astWalker) mayEdit(cmd Command, target string) bool {
	name := commandName(cmd.Name)

	if spec, ok := interpreters[name]; ok {
		return w.interpreterEdits(cmd, spec, target)
	}

	_, launcher := launchers[name]
	_, function := w.funcs[cmd.Name]
	_, alias := w.aliases[cmd.Name]

	if readOnlyPrograms[name] || shells[name] || shellBuiltins[name] || launcher ||
		function || alias {
		return false
	}

	if name == gitProgram {
		gitCmd, err := ParseGitCommand(cmd)

		return err != nil || !readOnlyGit[gitCmd.Subcommand]
	}

	operands := slices.DeleteFunc(slices.Clone(cmd.Args), func(arg string) bool {
		return strings.HasPrefix(arg, "-") && !strings.Contains(arg, "=")
	})

	return len(operands) == 0 || slices.ContainsFunc(cmd.Args, func(arg string) bool {
		return w.mayName(arg, target)
	})
}

// mayName reports whether an argument may name target: it is the file
// (alone or as a flag value), a directory above it, a glob or a word
// klaudiush cannot resolve (a variable left after expansion).
func (w *astWalker) mayName(arg, target string) bool {
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
// its inline program, or the script it runs, may change files or names
// target. Running target itself, whose content klaudiush has, does not
// count, nor does a module (python -m) or inline code that only reads.
func (w *astWalker) interpreterEdits(cmd Command, spec interpreter, target string) bool {
	operands := make([]string, 0, len(cmd.Args))
	inline := spec.codeFirst

	for _, arg := range cmd.Args {
		if codeFlags[arg] {
			inline = true
		}

		if !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
		}
	}

	if inline || len(operands) == 0 {
		return codeEdits(strings.Join(cmd.Args, " "), target)
	}

	script := w.trackedPath(cmd.WorkingDirectory, w.expandName(operands[0]))
	if filepath.Clean(script) == target {
		return false
	}

	text, status, _ := w.scriptSource(script, cmd)
	if status != ScriptText {
		return true
	}

	return codeEdits(text, target)
}

// codeEdits reports whether program source may change files or names
// target.
func codeEdits(code, target string) bool {
	return fileEdits.MatchString(code) || strings.Contains(code, filepath.Base(target))
}
