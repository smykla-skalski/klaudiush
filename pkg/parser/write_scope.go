package parser

import (
	"path/filepath"
	"strings"
)

// showToplevel is the allowed lookup that prints the work tree around a
// directory.
var showToplevel = []string{gitProgram, "rev-parse", "--show-toplevel"}

// writeScope returns the absolute directory below which a program changes
// files it does not name, or "" when they may be anywhere. For git it is the
// work tree around the command's directory, asked of the resolver without
// touching the network; when that cannot be told the write is anywhere.
func (w *astWalker) writeScope(cmd Command, write programWrite) string {
	if write.anywhere {
		return ""
	}

	base, ok := w.absDir(cmd.WorkingDirectory, cmd.DirUnknown)
	if !ok {
		return ""
	}

	dir := base

	switch root := write.root; {
	case root == "":
	case filepath.IsAbs(root) || strings.HasPrefix(root, "~"):
		if dir, ok = w.absDir(root, false); !ok {
			return ""
		}
	case HasUnresolvedVars(root) || marked(root):
		return ""
	default:
		dir = filepath.Join(base, root)
	}

	if !write.tree {
		return filepath.Clean(dir)
	}

	resolver, isOutput := w.resolver.(OutputResolver)
	if !isOutput {
		return ""
	}

	top, found := resolver.CommandOutput(dir, showToplevel)
	if !found || !filepath.IsAbs(top) {
		return ""
	}

	return filepath.Clean(top)
}

// absDir anchors a directory the walker tracks: "" is the directory the
// line started in, a relative one is below it, and ~ is HOME unless the
// line changed it.
func (w *astWalker) absDir(dir string, unknown bool) (string, bool) {
	if unknown {
		return "", false
	}

	if strings.HasPrefix(dir, "~") {
		if w.homeChanged() {
			return "", false
		}

		dir = ExpandHome(dir, w.resolver)
	}

	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), true
	}

	if strings.HasPrefix(dir, "~") || HasUnresolvedVars(dir) || marked(dir) {
		return "", false
	}

	start, ok := w.startPWD()
	if !ok {
		return "", false
	}

	return filepath.Join(start, dir), true
}

// outsideScope reports a script read that an unknown-target write cannot
// reach: the write has a scope and the script, anchored like the walker's
// directories, lies outside it.
func (w *astWalker) outsideScope(fw FileWrite, target string) bool {
	if fw.Scope == "" {
		return false
	}

	path, ok := w.absDir(target, false)
	if !ok {
		return false
	}

	return path != fw.Scope && !isBelow(path, fw.Scope)
}

// followsShellCode reports whether cmd reads a file as shell code, which an
// earlier unknown-target write may have changed. Other interpreters run
// code klaudiush reads only for the commands it names.
func followsShellCode(cmd Command) bool {
	_, interpreter := interpreters[cmd.Name]

	return !interpreter || shells[cmd.Name]
}
