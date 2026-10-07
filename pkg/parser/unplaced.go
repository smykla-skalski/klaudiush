package parser

import (
	"path/filepath"
	"slices"
	"strings"
)

// DetailScriptUnplacedWrite is the Opacity.Detail of a script read after a
// write whose target klaudiush cannot place.
const DetailScriptUnplacedWrite = "an earlier command on the line changes files klaudiush " +
	"cannot place (unzip, patch, git reset --hard, git stash, or a computed path), " +
	"so the file may differ from the disk"

// unplacedWriteBefore reports a write earlier on the line whose target
// klaudiush cannot place: a program that changes files it does not name, a
// target built from command output, or a relative target after a cd it
// cannot resolve. Any file read after it may differ from the disk. The
// redirects of the reading command itself are left out: they open their
// files as it starts, and a log named by date would otherwise block every
// run of a script. A program that changes files it does not name counts
// only when the reading command runs shell code and target is within the
// tree the program changes (the work tree for git, the extraction directory
// for archives).
func (w *astWalker) unplacedWriteBefore(cmd Command, target string) bool {
	own := func(loc Location) bool {
		return cmd.Location.Seq != 0 && loc.Seq == cmd.Location.Seq-1
	}

	for p := w; p != nil; p = p.parent {
		if slices.ContainsFunc(p.fileWrites, func(fw FileWrite) bool {
			if fw.TargetUnknown && fw.Source != "" &&
				(!followsShellCode(cmd) || w.outsideScope(fw, target)) {
				return false
			}

			if fw.Source != "" {
				return unplaced(fw) && fw.Location.Seq != cmd.Location.Seq
			}

			return unplaced(fw) && !own(fw.Location)
		}) {
			return true
		}

		if slices.ContainsFunc(p.dynamicWriteLocs, func(loc Location) bool { return !own(loc) }) {
			return true
		}
	}

	return false
}

// unplaced reports a write whose target cannot be told.
func unplaced(fw FileWrite) bool {
	relative := !filepath.IsAbs(fw.Path) && !strings.HasPrefix(fw.Path, "~")

	return fw.TargetUnknown || fw.Dynamic || (fw.DirUnknown && relative)
}

// splitsSubstitution reports an unquoted substitution among cmd's
// arguments: it may expand to any number of words, so which operand is the
// destination cannot be told.
func splitsSubstitution(cmd Command) bool {
	return slices.ContainsFunc(cmd.Args, func(arg string) bool {
		return marked(arg) &&
			cmd.quoting[strings.ReplaceAll(arg, unresolvedWord, "")]&unquotedWord != 0
	})
}

// placesIntoDirs are the writes whose target may be a directory the
// program puts files into under their own names.
var placesIntoDirs = map[WriteOp]bool{
	WriteOpCopy: true, WriteOpMove: true, WriteOpLink: true, WriteOpOutput: true,
}

// lineWriteAbove reports a copy, move, link or install earlier on the line
// whose destination is a directory above target, which may place a file
// there under target's name.
func (w *astWalker) lineWriteAbove(target string) bool {
	for p := w; p != nil; p = p.parent {
		for _, fw := range p.fileWrites {
			dir, known := w.writtenPath(fw)
			if !known {
				continue
			}

			if placesIntoDirs[fw.Operation] &&
				(fw.Operation != WriteOpOutput || fw.Source == "install") &&
				isBelow(target, dir) {
				return true
			}
		}
	}

	return false
}

// isBelow reports whether path is strictly below dir.
func isBelow(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)

	return err == nil && rel != "." && rel != parentDir &&
		!strings.HasPrefix(rel, parentDir+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
