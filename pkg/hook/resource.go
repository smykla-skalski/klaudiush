package hook

import (
	"path/filepath"
	"slices"
)

// Resource kinds identify what a validator checked. Files are told apart by
// path. Commands are not: a command already ran, and the repair is a later,
// different command (an amended commit, a push to another branch), so a
// validator passing on any command clears its earlier command findings.
const (
	ResourceFilePrefix = "file:"
	ResourceCommand    = "command"
	ResourceToolPrefix = "tool:"
)

// CanonicalFilePath makes path absolute against workingDir and resolves
// symlinks, so one file reached through different spellings compares equal.
func CanonicalFilePath(workingDir, path string) string {
	if path == "" {
		return ""
	}

	if !filepath.IsAbs(path) && workingDir != "" {
		path = filepath.Join(workingDir, path)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}

	return resolved
}

// Resource identifies what the validators of this context check: the file a
// file tool touches, any shell command, or else the tool itself.
func (c *Context) Resource() string {
	if c == nil {
		return ""
	}

	if c.IsFileTool() || c.Derived {
		if path := c.GetFilePath(); path != "" {
			return ResourceFilePrefix + CanonicalFilePath(c.WorkingDir, path)
		}
	}

	if c.GetCommand() != "" {
		return ResourceCommand
	}

	if path := c.GetFilePath(); path != "" {
		return ResourceFilePrefix + CanonicalFilePath(c.WorkingDir, path)
	}

	return ResourceToolPrefix + c.ToolNameString()
}

// NeedsRecheck reports whether path has unresolved findings to verify again.
func (c *Context) NeedsRecheck(path string) bool {
	if c == nil || len(c.RecheckFiles) == 0 || path == "" {
		return false
	}

	return slices.Contains(c.RecheckFiles, CanonicalFilePath(c.WorkingDir, path))
}
