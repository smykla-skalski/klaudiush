package crashdump

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

const (
	// FilePerm is the file permission for crash dump files.
	FilePerm fs.FileMode = 0o600

	// DirPerm is the directory permission for crash dump directories.
	DirPerm fs.FileMode = 0o700

	// FileExtension is the extension for crash dump files.
	FileExtension = ".json"

	// TempSuffix is the suffix for temporary files during atomic writes.
	TempSuffix = ".tmp"
)

var (
	// ErrWriteFailed is returned when writing a crash dump fails.
	ErrWriteFailed = errors.New("failed to write crash dump")

	// ErrInvalidDumpDir is returned when the dump directory is invalid.
	ErrInvalidDumpDir = errors.New("invalid dump directory")
)

// Writer writes crash dumps to storage.
type Writer interface {
	// Write writes a crash dump and returns the file path.
	Write(info *CrashInfo) (string, error)
}

// FilesystemWriter writes crash dumps to the filesystem.
type FilesystemWriter struct {
	// dumpDir is the directory where crash dumps are stored.
	dumpDir string
}

// NewFilesystemWriter creates a new filesystem-based writer.
func NewFilesystemWriter(dumpDir string) (*FilesystemWriter, error) {
	if dumpDir == "" {
		return nil, errors.Wrap(ErrInvalidDumpDir, "dump directory cannot be empty")
	}

	// Expand home directory
	expandedDir, err := xdg.ExpandPath(dumpDir)
	if err != nil {
		return nil, errors.Wrap(ErrInvalidDumpDir, err.Error())
	}

	return &FilesystemWriter{
		dumpDir: expandedDir,
	}, nil
}

// Write writes a crash dump and returns the file path.
func (w *FilesystemWriter) Write(info *CrashInfo) (string, error) {
	if info == nil {
		return "", errors.Wrap(ErrWriteFailed, "crash info is nil")
	}

	// Ensure directory exists
	if err := w.ensureDir(); err != nil {
		return "", err
	}

	// Generate file path
	filename := info.ID + FileExtension
	filePath := filepath.Join(w.dumpDir, filename)
	tempPath := filePath + TempSuffix

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", errors.Wrap(ErrWriteFailed, "failed to marshal crash info")
	}

	// Write to temp file first (atomic write pattern)
	if err := os.WriteFile(tempPath, data, FilePerm); err != nil {
		return "", errors.Wrap(ErrWriteFailed, err.Error())
	}

	// Rename temp file to final path (atomic on most filesystems)
	if err := os.Rename(tempPath, filePath); err != nil {
		// Clean up temp file on rename failure
		_ = os.Remove(tempPath)

		return "", errors.Wrap(ErrWriteFailed, err.Error())
	}

	return filePath, nil
}

// ensureDir creates the dump directory if it doesn't exist.
func (w *FilesystemWriter) ensureDir() error {
	if err := os.MkdirAll(w.dumpDir, DirPerm); err != nil {
		return errors.Wrap(ErrInvalidDumpDir, err.Error())
	}

	return nil
}

// GetDumpDir returns the dump directory path.
func (w *FilesystemWriter) GetDumpDir() string {
	return w.dumpDir
}
