package exec

//go:generate mockgen -source=tempfile.go -destination=tempfile_mock.go -package=exec

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

var (
	createTempFile = os.CreateTemp
	writeString    = func(file *os.File, content string) (int, error) {
		return file.WriteString(content)
	}
	closeFile = func(file *os.File) error {
		return file.Close()
	}
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
	tmpFile, err := createTempFile("", pattern)
	if err != nil {
		return "", nil, errors.Wrap(err, "creating temp file")
	}

	filePath := filepath.Clean(tmpFile.Name())

	// Write content
	if _, err := writeString(tmpFile, content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove( // #nosec G703 -- filePath comes from os.CreateTemp, not user input
			filePath,
		)

		return "", nil, errors.Wrap(err, "writing to temp file")
	}

	// Close file
	if err := closeFile(tmpFile); err != nil {
		_ = os.Remove( // #nosec G703 -- filePath comes from os.CreateTemp, not user input
			filePath,
		)

		return "", nil, errors.Wrap(err, "closing temp file")
	}

	// Return cleanup function
	cleanup := func() {
		_ = os.Remove(filePath)
	}

	return filePath, cleanup, nil
}
