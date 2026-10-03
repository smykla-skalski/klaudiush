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
	Content   string   // Content written (populated for heredoc operations)
	Location  Location // Position in source
	// WorkingDirectory is the effective directory (from preceding cd commands)
	// when the write occurs, used to resolve relative paths.
	WorkingDirectory string
	// ContentCaptured reports whether Content holds the exact bytes written.
	// It is true only for an overwrite heredoc (">") whose command copies stdin
	// to stdout verbatim (cat). It is false for append heredocs (">>"), heredocs
	// on transforming commands (grep, sed, ...), plain redirects, and tee/cp/mv,
	// where the content cannot be reconstructed from the command alone.
	ContentCaptured bool
	// RedirectContent holds the text a plain overwrite redirect (">") from a
	// literal echo/printf produced, captured for inline commit-message recovery
	// only (e.g. printf '%s' msg > "$MSG"; git commit -F "$MSG"). It is a
	// best-effort reconstruction - a trailing newline is dropped and echo
	// arguments are space-joined - which is sufficient for message validation
	// (callers trim anyway). It is deliberately kept out of Content so it is NOT
	// fanned out to file validators - linting partial program text written with
	// echo/printf (e.g. "printf 'package x' > f.go") would produce spurious
	// failures.
	RedirectContent string
	// DirUnknown reports that an earlier cd went somewhere the parser cannot
	// resolve, so a relative Path is relative to an unknown directory.
	DirUnknown bool
	// Dynamic reports that part of Path comes from command output, which the
	// rendered Path leaves out: "$(echo dir)/f" renders as "/f".
	Dynamic bool
	// RedirectContentCaptured reports whether RedirectContent was reconstructed.
	// It is true only for an overwrite redirect (">", not ">>") whose producer is
	// a literal echo or a printf using only %s/%% directives and known escapes;
	// otherwise the output cannot be reproduced.
	RedirectContentCaptured bool
	// Vars are the variables as they stood when the write happened.
	Vars *VarScope
}

// CapturedOverwrite returns the reconstructed content an overwrite write produced
// and whether it was captured. Heredoc bodies (stored in Content) are exact;
// literal echo/printf output (stored in RedirectContent, kept separate so it is
// not linted as a file) is a best-effort, normalized reconstruction. Appends and
// transforming or copying commands capture nothing, so ok is false for them.
func (f FileWrite) CapturedOverwrite() (string, bool) {
	if f.ContentCaptured {
		return f.Content, true
	}

	if f.RedirectContentCaptured {
		return f.RedirectContent, true
	}

	return "", false
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
