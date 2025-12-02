package backup

//go:generate mockgen -source=storage.go -destination=storage_mock.go -package=backup Storage

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

var (
	// ErrStorageNotInitialized is returned when storage is not initialized.
	ErrStorageNotInitialized = errors.New("storage not initialized")

	// ErrInvalidPath is returned when an invalid path is provided.
	ErrInvalidPath = errors.New("invalid path")
)

const (
	// DefaultBackupDir is the default directory for centralized backups.
	DefaultBackupDir = ".backups"

	// GlobalBackupDir is the subdirectory for global config backups.
	GlobalBackupDir = "global"

	// ProjectBackupDir is the subdirectory for project config backups.
	ProjectBackupDir = "projects"

	// SnapshotsDir is the subdirectory for snapshot files.
	SnapshotsDir = "snapshots"

	// MetadataFile is the filename for the snapshot index.
	MetadataFile = "metadata.json"

	// AuditFile is the filename for the audit log.
	AuditFile = "audit.jsonl"

	// RetentionFile is the filename for retention state.
	RetentionFile = ".retention"

	// FilePerm is the file permission for backup files.
	FilePerm fs.FileMode = 0o600

	// DirPerm is the directory permission for backup directories.
	DirPerm fs.FileMode = 0o700
)

// Storage defines the interface for backup storage operations.
type Storage interface {
	// Save stores snapshot data and returns the storage path.
	Save(snapshotID string, data []byte) (string, error)

	// Load retrieves snapshot data by storage path.
	Load(storagePath string) ([]byte, error)

	// Delete removes snapshot data by storage path.
	Delete(storagePath string) error

	// List returns all snapshot storage paths.
	List() ([]string, error)

	// SaveIndex saves the snapshot index.
	SaveIndex(index *SnapshotIndex) error

	// LoadIndex loads the snapshot index.
	LoadIndex() (*SnapshotIndex, error)

	// Exists checks if storage is initialized.
	Exists() bool

	// Initialize creates the storage directory structure.
	Initialize() error
}

// FilesystemStorage implements Storage using the local filesystem.
type FilesystemStorage struct {
	// baseDir is the base directory for all backups (~/.klaudiush).
	baseDir string

	// configType indicates whether this storage is for global or project configs.
	configType ConfigType

	// projectPath is the sanitized project path (for project configs only).
	projectPath string
}

// NewFilesystemStorage creates a new filesystem-based storage.
func NewFilesystemStorage(
	baseDir string,
	configType ConfigType,
	projectPath string,
) (*FilesystemStorage, error) {
	if baseDir == "" {
		return nil, errors.Wrap(ErrInvalidPath, "baseDir cannot be empty")
	}

	if configType != ConfigTypeGlobal && configType != ConfigTypeProject {
		return nil, errors.Wrapf(ErrInvalidConfigType, "got: %s", configType)
	}

	if configType == ConfigTypeProject && projectPath == "" {
		return nil, errors.Wrap(ErrInvalidPath, "projectPath required for project configs")
	}

	return &FilesystemStorage{
		baseDir:     baseDir,
		configType:  configType,
		projectPath: SanitizePath(projectPath),
	}, nil
}

// getStorageRoot returns the root directory for this storage.
func (f *FilesystemStorage) getStorageRoot() string {
	backupDir := filepath.Join(f.baseDir, DefaultBackupDir)

	if f.configType == ConfigTypeGlobal {
		return filepath.Join(backupDir, GlobalBackupDir)
	}

	return filepath.Join(backupDir, ProjectBackupDir, f.projectPath)
}

// getSnapshotsDir returns the snapshots directory.
func (f *FilesystemStorage) getSnapshotsDir() string {
	return filepath.Join(f.getStorageRoot(), SnapshotsDir)
}

// getMetadataPath returns the metadata file path.
func (f *FilesystemStorage) getMetadataPath() string {
	return filepath.Join(f.getStorageRoot(), MetadataFile)
}

// Exists checks if storage is initialized.
func (f *FilesystemStorage) Exists() bool {
	_, err := os.Stat(f.getStorageRoot())

	return err == nil
}

// Initialize creates the storage directory structure.
func (f *FilesystemStorage) Initialize() error {
	snapshotsDir := f.getSnapshotsDir()

	if err := os.MkdirAll(snapshotsDir, DirPerm); err != nil {
		return errors.Wrap(err, "failed to create snapshots directory")
	}

	metadataPath := f.getMetadataPath()
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		index := NewSnapshotIndex()
		if err := f.SaveIndex(index); err != nil {
			return errors.Wrap(err, "failed to initialize metadata")
		}
	}

	return nil
}

// Save stores snapshot data and returns the storage path.
func (f *FilesystemStorage) Save(snapshotID string, data []byte) (string, error) {
	if !f.Exists() {
		return "", errors.Wrap(ErrStorageNotInitialized, "call Initialize() first")
	}

	filename := snapshotID
	storagePath := filepath.Join(f.getSnapshotsDir(), filename)

	if err := os.WriteFile(storagePath, data, FilePerm); err != nil {
		return "", errors.Wrap(err, "failed to write snapshot data")
	}

	return storagePath, nil
}

// Load retrieves snapshot data by storage path.
func (*FilesystemStorage) Load(storagePath string) ([]byte, error) {
	// #nosec G304 - storagePath is controlled internally by storage layer
	data, err := os.ReadFile(storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrap(ErrSnapshotNotFound, storagePath)
		}

		return nil, errors.Wrap(err, "failed to read snapshot data")
	}

	return data, nil
}

// Delete removes snapshot data by storage path.
func (*FilesystemStorage) Delete(storagePath string) error {
	if err := os.Remove(storagePath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrap(ErrSnapshotNotFound, storagePath)
		}

		return errors.Wrap(err, "failed to delete snapshot data")
	}

	return nil
}

// List returns all snapshot storage paths.
func (f *FilesystemStorage) List() ([]string, error) {
	snapshotsDir := f.getSnapshotsDir()

	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}

		return nil, errors.Wrap(err, "failed to list snapshots")
	}

	paths := make([]string, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != MetadataFile {
			paths = append(paths, filepath.Join(snapshotsDir, entry.Name()))
		}
	}

	return paths, nil
}

// SaveIndex saves the snapshot index.
func (f *FilesystemStorage) SaveIndex(index *SnapshotIndex) error {
	metadataPath := f.getMetadataPath()

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal index")
	}

	if err := os.WriteFile(metadataPath, data, FilePerm); err != nil {
		return errors.Wrap(err, "failed to write index")
	}

	return nil
}

// LoadIndex loads the snapshot index.
func (f *FilesystemStorage) LoadIndex() (*SnapshotIndex, error) {
	metadataPath := f.getMetadataPath()

	// #nosec G304 - metadataPath is controlled internally by storage layer
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewSnapshotIndex(), nil
		}

		return nil, errors.Wrap(err, "failed to read index")
	}

	var index SnapshotIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal index")
	}

	return &index, nil
}

// SanitizePath sanitizes a file path for use as a directory name.
// Converts /Users/bart/project to Users_bart_project.
func SanitizePath(path string) string {
	if path == "" {
		return ""
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// Remove leading slash and replace separators with underscores
	sanitized := filepath.Clean(absPath)
	if len(sanitized) > 0 && sanitized[0] == filepath.Separator {
		sanitized = sanitized[1:]
	}

	sanitized = filepath.ToSlash(sanitized)
	sanitized = filepath.FromSlash(sanitized)

	// Replace path separators with underscores
	var builder strings.Builder
	builder.Grow(len(sanitized))

	for _, ch := range sanitized {
		if ch == filepath.Separator || ch == '/' {
			builder.WriteRune('_')
		} else {
			builder.WriteRune(ch)
		}
	}

	return builder.String()
}
