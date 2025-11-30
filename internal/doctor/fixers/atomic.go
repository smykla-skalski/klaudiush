// Package fixers provides auto-fix implementations for health check issues.
package fixers

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
)

const (
	defaultDirPermissions  = 0o750
	defaultFilePermissions = 0o600
)

// AtomicWriteFile writes data to a file atomically using a temp file and rename.
// It creates a backup of the original file if it exists.
// The temp file is written with the target file's permissions (or 0600 for new files).
func AtomicWriteFile(path string, data []byte, createBackup bool) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, defaultDirPermissions); err != nil {
		return errors.Wrap(err, "failed to create directory")
	}

	// Get existing file permissions, or use 0600 for new files
	perm := os.FileMode(defaultFilePermissions)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	// Create backup if requested and file exists
	if createBackup {
		if _, err := os.Stat(path); err == nil {
			backupPath := fmt.Sprintf("%s.backup.%d", path, time.Now().Unix())
			if err := copyFile(path, backupPath); err != nil {
				return errors.Wrap(err, "failed to create backup")
			}
		}
	}

	// Write to temporary file
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, perm); err != nil {
		return errors.Wrap(err, "failed to write temp file")
	}

	// Atomic rename
	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return errors.Wrap(err, "failed to rename temp file")
	}

	return nil
}

// copyFile copies src file to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) //nolint:gosec // src is controlled by caller
	if err != nil {
		return errors.Wrap(err, "failed to read source file")
	}

	info, err := os.Stat(src)
	if err != nil {
		return errors.Wrap(err, "failed to stat source file")
	}

	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return errors.Wrap(err, "failed to write destination file")
	}

	return nil
}
