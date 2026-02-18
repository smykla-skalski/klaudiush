// Package git provides an SDK-based implementation of git operations using go-git v6
package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

const (
	// ExcludeFileMode is the file mode for .git/info/exclude file.
	ExcludeFileMode = 0o644

	// ExcludeDirMode is the file mode for .git/info directory.
	ExcludeDirMode = 0o750
)

// ErrEntryAlreadyExists is returned when an entry already exists in the exclude file.
var ErrEntryAlreadyExists = errors.New("entry already exists")

// ExcludeManager manages .git/info/exclude file entries.
type ExcludeManager struct {
	repoRoot string
}

// NewExcludeManager creates a new ExcludeManager from a discovered repository.
func NewExcludeManager() (*ExcludeManager, error) {
	repo, err := DiscoverRepository()
	if err != nil {
		return nil, errors.Wrap(err, "failed to discover repository")
	}

	root, err := repo.GetRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repository root")
	}

	return &ExcludeManager{repoRoot: root}, nil
}

// NewExcludeManagerFromRoot creates a new ExcludeManager with a custom root (for testing).
func NewExcludeManagerFromRoot(repoRoot string) *ExcludeManager {
	return &ExcludeManager{repoRoot: repoRoot}
}

// GetExcludePath returns the path to .git/info/exclude.
func (m *ExcludeManager) GetExcludePath() string {
	return filepath.Join(m.repoRoot, ".git", "info", "exclude")
}

// AddEntry adds an entry to .git/info/exclude if it doesn't already exist.
func (m *ExcludeManager) AddEntry(pattern string) error {
	excludePath := m.GetExcludePath()

	// Check if entry already exists
	exists, err := m.HasEntry(pattern)
	if err != nil {
		return err
	}

	if exists {
		return errors.Wrapf(ErrEntryAlreadyExists, "pattern %q", pattern)
	}

	// Ensure .git/info directory exists
	infoDir := filepath.Dir(excludePath)
	if mkdirErr := os.MkdirAll(infoDir, ExcludeDirMode); mkdirErr != nil {
		return errors.Wrapf(mkdirErr, "failed to create directory %s", infoDir)
	}

	// Read existing content
	content, err := m.readExcludeFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Append new entry with comment
	var buf bytes.Buffer
	buf.Write(content)

	// Ensure content ends with newline
	if len(content) > 0 && content[len(content)-1] != '\n' {
		buf.WriteByte('\n')
	}

	// Add comment and entry
	fmt.Fprintf(&buf, "# Added by klaudiush\n%s\n", pattern)

	// Write back to file
	if err := os.WriteFile(excludePath, buf.Bytes(), ExcludeFileMode); err != nil {
		return errors.Wrapf(err, "failed to write to %s", excludePath)
	}

	return nil
}

// HasEntry checks if a pattern exists in .git/info/exclude.
func (m *ExcludeManager) HasEntry(pattern string) (bool, error) {
	content, err := m.readExcludeFile()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if line == pattern {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, errors.Wrap(err, "failed to scan exclude file")
	}

	return false, nil
}

// readExcludeFile reads the content of .git/info/exclude.
func (m *ExcludeManager) readExcludeFile() ([]byte, error) {
	excludePath := m.GetExcludePath()

	//nolint:gosec // G304: File path is within git repository
	content, err := os.ReadFile(excludePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", excludePath)
	}

	return content, nil
}

// IsInGitRepo checks if the current directory is within a git repository.
func IsInGitRepo() bool {
	repo, err := DiscoverRepository()
	if err != nil {
		return false
	}

	return repo.IsInRepo()
}
