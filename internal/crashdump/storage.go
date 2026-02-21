package crashdump

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

var (
	// ErrDumpNotFound is returned when a crash dump is not found.
	ErrDumpNotFound = errors.New("crash dump not found")

	// ErrStorageNotInitialized is returned when storage is not initialized.
	ErrStorageNotInitialized = errors.New("storage not initialized")
)

// Storage provides operations for managing crash dumps.
type Storage interface {
	// List returns all crash dump summaries, sorted by timestamp (newest first).
	List() ([]DumpSummary, error)

	// Get retrieves a crash dump by ID.
	Get(id string) (*CrashInfo, error)

	// Delete removes a crash dump by ID.
	Delete(id string) error

	// Prune removes old dumps based on max count and max age.
	Prune(maxDumps int, maxAge time.Duration) (int, error)

	// Exists checks if the storage directory exists.
	Exists() bool

	// Initialize creates the storage directory if it doesn't exist.
	Initialize() error
}

// FilesystemStorage implements Storage using the local filesystem.
type FilesystemStorage struct {
	// dumpDir is the directory where crash dumps are stored.
	dumpDir string
}

// NewFilesystemStorage creates a new filesystem-based storage.
func NewFilesystemStorage(dumpDir string) (*FilesystemStorage, error) {
	if dumpDir == "" {
		return nil, errors.Wrap(ErrInvalidDumpDir, "dump directory cannot be empty")
	}

	// Expand home directory
	expandedDir, err := xdg.ExpandPath(dumpDir)
	if err != nil {
		return nil, err
	}

	return &FilesystemStorage{
		dumpDir: expandedDir,
	}, nil
}

// List returns all crash dump summaries, sorted by timestamp (newest first).
func (s *FilesystemStorage) List() ([]DumpSummary, error) {
	if !s.Exists() {
		return []DumpSummary{}, nil
	}

	entries, err := os.ReadDir(s.dumpDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read dump directory")
	}

	summaries := make([]DumpSummary, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), FileExtension) {
			continue
		}

		summary, err := s.loadSummary(entry.Name())
		if err != nil {
			// Skip corrupted files
			continue
		}

		summaries = append(summaries, summary)
	}

	// Sort by timestamp, newest first
	slices.SortFunc(summaries, func(a, b DumpSummary) int {
		return cmp.Compare(b.Timestamp.UnixNano(), a.Timestamp.UnixNano())
	})

	return summaries, nil
}

// loadSummary loads a dump summary from a file.
func (s *FilesystemStorage) loadSummary(filename string) (DumpSummary, error) {
	filePath := filepath.Join(s.dumpDir, filename)

	info, err := s.loadFile(filePath)
	if err != nil {
		return DumpSummary{}, err
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return DumpSummary{}, errors.Wrap(err, "failed to stat file")
	}

	// Truncate panic value for summary
	const maxPanicLen = 80

	panicValue := info.PanicValue

	if len(panicValue) > maxPanicLen {
		panicValue = panicValue[:maxPanicLen] + "..."
	}

	return DumpSummary{
		ID:         info.ID,
		Timestamp:  info.Timestamp,
		PanicValue: panicValue,
		FilePath:   filePath,
		Size:       fileInfo.Size(),
	}, nil
}

// Get retrieves a crash dump by ID.
func (s *FilesystemStorage) Get(id string) (*CrashInfo, error) {
	if !s.Exists() {
		return nil, errors.Wrapf(ErrDumpNotFound, "ID: %s", id)
	}

	filePath := filepath.Join(s.dumpDir, id+FileExtension)

	return s.loadFile(filePath)
}

// loadFile loads a crash dump from a file path.
func (*FilesystemStorage) loadFile(filePath string) (*CrashInfo, error) {
	// #nosec G304 - filePath is constructed internally from trusted components
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(ErrDumpNotFound, "file: %s", filePath)
		}

		return nil, errors.Wrap(err, "failed to read dump file")
	}

	var info CrashInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal dump file")
	}

	return &info, nil
}

// Delete removes a crash dump by ID.
func (s *FilesystemStorage) Delete(id string) error {
	if !s.Exists() {
		return errors.Wrapf(ErrDumpNotFound, "ID: %s", id)
	}

	filePath := filepath.Join(s.dumpDir, id+FileExtension)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrapf(ErrDumpNotFound, "ID: %s", id)
		}

		return errors.Wrap(err, "failed to delete dump file")
	}

	return nil
}

// Prune removes old dumps based on max count and max age.
// Returns the number of dumps removed.
func (s *FilesystemStorage) Prune(maxDumps int, maxAge time.Duration) (int, error) {
	summaries, err := s.List()
	if err != nil {
		return 0, err
	}

	if len(summaries) == 0 {
		return 0, nil
	}

	now := time.Now()
	removed := 0

	// Remove dumps older than maxAge
	for _, summary := range summaries {
		if maxAge > 0 && now.Sub(summary.Timestamp) > maxAge {
			if deleteErr := s.Delete(summary.ID); deleteErr != nil {
				// Continue with other deletions
				continue
			}

			removed++
		}
	}

	// Re-fetch list after age-based removal
	summaries, err = s.List()
	if err != nil {
		return removed, err
	}

	// Remove excess dumps (oldest first, list is already sorted newest first)
	for i := maxDumps; i < len(summaries); i++ {
		if deleteErr := s.Delete(summaries[i].ID); deleteErr != nil {
			// Continue with other deletions
			continue
		}

		removed++
	}

	return removed, nil
}

// Exists checks if the storage directory exists.
func (s *FilesystemStorage) Exists() bool {
	info, err := os.Stat(s.dumpDir)

	return err == nil && info.IsDir()
}

// Initialize creates the storage directory if it doesn't exist.
func (s *FilesystemStorage) Initialize() error {
	if err := os.MkdirAll(s.dumpDir, DirPerm); err != nil {
		return errors.Wrap(ErrInvalidDumpDir, err.Error())
	}

	return nil
}

// GetDumpDir returns the dump directory path.
func (s *FilesystemStorage) GetDumpDir() string {
	return s.dumpDir
}
