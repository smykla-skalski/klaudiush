package exec

//go:generate mockgen -source=tempfile.go -destination=tempfile_mock.go -package=exec

import (
	"fmt"
	"os"
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
		return "", nil, fmt.Errorf("creating temp file: %w", err)
	}

	filePath := tmpFile.Name()

	// Write content
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(filePath)

		return "", nil, fmt.Errorf("writing to temp file: %w", err)
	}

	// Close file
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(filePath)
		return "", nil, fmt.Errorf("closing temp file: %w", err)
	}

	// Return cleanup function
	cleanup := func() {
		_ = os.Remove(filePath)
	}

	return filePath, cleanup, nil
}
