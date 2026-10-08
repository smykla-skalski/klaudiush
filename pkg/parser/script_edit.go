package parser

import (
	"path/filepath"
	"slices"
	"strings"
)

// DetailScriptEdited is the Opacity.Detail of a script whose content
// klaudiush captured from a write earlier on the line, when a command
// between that write and the run may have edited the file.
const DetailScriptEdited = "a command after it is written on the line may edit it, " +
	"so what runs may differ from the content klaudiush saw"

// readOnlyPrograms leave the content of the files they name as it is:
// readers, checksums and metadata changes. A redirect on one of them is a
// write klaudiush tracks on its own.
var readOnlyPrograms = nameSet(`cat head tail less more wc ls stat file grep egrep fgrep
	rg diff cmp test [ md5sum md5 shasum sha1sum sha256sum chmod chown chgrp touch echo
	printf realpath readlink basename dirname du true false pwd cd mkdir`)

// readOnlyGit are the git subcommands that leave work tree files as they are.
var readOnlyGit = nameSet("add diff status log show ls-files blame")

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
// that may change target without klaudiush capturing the result: an
// interpreter, whose code may write any file (open(...).write, a script on
// disk), or a program outside readOnlyPrograms that names the file. Writes
// klaudiush tracks (redirects, tee, cp, mv, sed -i) are handled by
// lastLineWrite, which then stops trusting the capture.
func (w *astWalker) editedAfter(cmd Command, target string, seq int) bool {
	if seq == 0 || cmd.Location.Seq == 0 {
		return false
	}

	base := filepath.Base(target)

	for earlier := range w.earlierCommands() {
		if earlier.Location.Seq <= seq || earlier.Location.Seq >= cmd.Location.Seq {
			continue
		}

		if mayEdit(earlier, base) {
			return true
		}
	}

	return false
}

// mayEdit reports whether cmd may change a file named base that klaudiush
// does not see it write. Shells, launchers and builtins are left out: the
// commands they run are recorded and judged on their own.
func mayEdit(cmd Command, base string) bool {
	name := commandName(cmd.Name)
	if _, ok := interpreters[name]; ok {
		return true
	}

	_, launcher := launchers[name]
	if readOnlyPrograms[name] || shells[name] || shellBuiltins[name] || launcher {
		return false
	}

	if name == gitProgram {
		if gitCmd, err := ParseGitCommand(cmd); err == nil && readOnlyGit[gitCmd.Subcommand] {
			return false
		}
	}

	return slices.ContainsFunc(cmd.Args, func(arg string) bool {
		return strings.Contains(arg, base)
	})
}
