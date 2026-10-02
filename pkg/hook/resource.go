package hook

import (
	"path/filepath"
	"slices"
)

// Resource kinds prefix the identity of what a validator checked.
const (
	ResourceFilePrefix    = "file:"
	ResourceCommandPrefix = "command:"
	ResourceToolPrefix    = "tool:"
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
// file tool touches, the command a shell tool runs, or else the tool itself.
func (c *Context) Resource() string {
	if c == nil {
		return ""
	}

	if c.IsFileTool() || c.Derived {
		if path := c.GetFilePath(); path != "" {
			return ResourceFilePrefix + CanonicalFilePath(c.WorkingDir, path)
		}
	}

	if command := c.GetCommand(); command != "" {
		return ResourceCommandPrefix + command
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
