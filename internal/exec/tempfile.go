package exec

//go:generate mockgen -source=tempfile.go -destination=tempfile_mock.go -package=exec

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// TempFileManager manages temporary files.
type TempFileManager interface {
	// Create creates a temporary file with the given pattern and content.
	// Returns the file path and a cleanup function that should be called with defer.
	Create(pattern, content string) (path string, cleanup func(), err error)
}

// tempFileManager implements TempFileManager.
type tempFileManager struct{}

// NewTempFileManager creates a new TempFileManager.
func NewTempFileManager() *tempFileManager {
	return &tempFileManager{}
}

// Create creates a temporary file with the given pattern and content.
func (*tempFileManager) Create(pattern, content string) (string, func(), error) {
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, errors.Wrap(err, "creating temp file")
	}

	filePath := filepath.Clean(tmpFile.Name())

	// Write content
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		//nolint:gosec // G703: filePath is from os.CreateTemp via filepath.Clean above; gosec cannot trace through variable assignment
		_ = os.Remove(filePath)

		return "", nil, errors.Wrap(err, "writing to temp file")
	}

	// Close file
	if err := tmpFile.Close(); err != nil {
		//nolint:gosec // G703: filePath is from os.CreateTemp via filepath.Clean above; gosec cannot trace through variable assignment
		_ = os.Remove(filePath)
		return "", nil, errors.Wrap(err, "closing temp file")
	}

	// Return cleanup function
	cleanup := func() {
		_ = os.Remove(filePath)
	}

	return filePath, cleanup, nil
}
