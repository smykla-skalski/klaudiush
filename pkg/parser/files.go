package parser

import "fmt"

// WriteOp represents the type of file write operation.
type WriteOp int

const (
	// WriteOpNone indicates no file write operation.
	WriteOpNone WriteOp = iota
	// WriteOpRedirect indicates output redirection (>).
	WriteOpRedirect
	// WriteOpAppend indicates append redirection (>>).
	WriteOpAppend
	// WriteOpTee indicates tee command.
	WriteOpTee
	// WriteOpCopy indicates cp/copy command.
	WriteOpCopy
	// WriteOpMove indicates mv/move command.
	WriteOpMove
	// WriteOpHeredoc indicates heredoc (<<).
	WriteOpHeredoc
)

// String returns string representation of WriteOp.
func (w WriteOp) String() string {
	switch w {
	case WriteOpNone:
		return "None"
	case WriteOpRedirect:
		return "Redirect"
	case WriteOpAppend:
		return "Append"
	case WriteOpTee:
		return "Tee"
	case WriteOpCopy:
		return "Copy"
	case WriteOpMove:
		return "Move"
	case WriteOpHeredoc:
		return "Heredoc"
	default:
		return "Unknown"
	}
}

// FileWrite represents a file write operation detected in the command.
type FileWrite struct {
	Path      string   // Target file path
	Operation WriteOp  // Type of write operation
	Source    string   // Source command (for cp, mv, tee)
	Content   string   // Content for heredoc operations
	Location  Location // Position in source
}

// String returns a string representation of the file write operation.
func (f *FileWrite) String() string {
	return fmt.Sprintf("%s %s -> %s", f.Operation, f.Source, f.Path)
}

// IsProtectedPath checks if the path is a protected location.
func (f *FileWrite) IsProtectedPath() bool {
	return IsProtectedPath(f.Path)
}

// IsProtectedPath checks if a path is protected (e.g., /tmp, /var/tmp).
func IsProtectedPath(path string) bool {
	// Check for /tmp prefix
	if len(path) >= 4 && path[:4] == "/tmp" {
		// Exact match or /tmp/...
		return len(path) == 4 || path[4] == '/'
	}

	// Check for /var/tmp prefix
	if len(path) >= 8 && path[:8] == "/var/tmp" {
		return len(path) == 8 || path[8] == '/'
	}

	return false
}
