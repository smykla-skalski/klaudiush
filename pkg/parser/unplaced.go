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
// program target built from command output, or a relative target after a
// cd it cannot resolve. Any file read after it may differ from the disk.
// Redirects to computed names are left out: one on the script's own
// command, such as a log named by date, would block every such run.
func (w *astWalker) unplacedWriteBefore() bool {
	for p := w; p != nil; p = p.parent {
		if slices.ContainsFunc(p.fileWrites, unplaced) {
			return true
		}
	}

	return false
}

// unplaced reports a write whose target cannot be told.
func unplaced(fw FileWrite) bool {
	relative := !filepath.IsAbs(fw.Path) && !strings.HasPrefix(fw.Path, "~")

	return fw.TargetUnknown || (fw.Dynamic && fw.Source != "") || (fw.DirUnknown && relative)
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
			if placesIntoDirs[fw.Operation] &&
				isBelow(target, resolvePath(fw.WorkingDirectory, fw.Path)) {
				return true
			}
		}
	}

	return false
}

// isBelow reports whether path is strictly below dir.
func isBelow(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)

	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
