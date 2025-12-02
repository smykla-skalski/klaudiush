package backup

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

var (
	// ErrChecksumMismatch is returned when snapshot checksum doesn't match content.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrCorruptedSnapshot is returned when snapshot data is corrupted.
	ErrCorruptedSnapshot = errors.New("corrupted snapshot")

	// ErrTargetPathRequired is returned when target path is not provided.
	ErrTargetPathRequired = errors.New("target path is required")
)

// RestoreOptions contains options for restoring a snapshot.
type RestoreOptions struct {
	// TargetPath is the absolute path where the config should be restored.
	// If empty, the original ConfigPath from the snapshot will be used.
	TargetPath string

	// BackupBeforeRestore creates a backup of the existing file before restoring.
	BackupBeforeRestore bool

	// Force overwrites the target file if it exists without creating a backup.
	Force bool

	// Validate verifies the snapshot checksum before restoring.
	Validate bool
}

// RestoreResult contains information about a restore operation.
type RestoreResult struct {
	// RestoredPath is the path where the config was restored.
	RestoredPath string

	// BackupSnapshot is the snapshot created before restore (if BackupBeforeRestore was true).
	BackupSnapshot *Snapshot

	// BytesRestored is the number of bytes written to the target file.
	BytesRestored int64

	// ChecksumVerified indicates whether checksum validation was performed.
	ChecksumVerified bool
}

// Restorer handles snapshot restoration operations.
type Restorer struct {
	// storage provides access to snapshot data.
	storage Storage

	// manager is used for creating backups before restore.
	manager *Manager
}

// NewRestorer creates a new Restorer.
func NewRestorer(storage Storage, manager *Manager) (*Restorer, error) {
	if storage == nil {
		return nil, errors.New("storage cannot be nil")
	}

	if manager == nil {
		return nil, errors.New("manager cannot be nil")
	}

	return &Restorer{
		storage: storage,
		manager: manager,
	}, nil
}

// RestoreSnapshot restores a snapshot to the target path.
func (r *Restorer) RestoreSnapshot(
	snapshot *Snapshot,
	opts RestoreOptions,
) (*RestoreResult, error) {
	if snapshot == nil {
		return nil, errors.New("snapshot cannot be nil")
	}

	// Determine target path
	targetPath := opts.TargetPath
	if targetPath == "" {
		targetPath = snapshot.ConfigPath
	}

	if targetPath == "" {
		return nil, ErrTargetPathRequired
	}

	// Validate snapshot if requested
	if opts.Validate {
		if err := r.ValidateSnapshot(snapshot); err != nil {
			return nil, errors.Wrap(err, "snapshot validation failed")
		}
	}

	// Create backup before restore if requested
	var backupSnapshot *Snapshot

	if opts.BackupBeforeRestore && !opts.Force {
		if _, statErr := os.Stat(targetPath); statErr == nil {
			backup, err := r.manager.CreateBackup(CreateBackupOptions{
				ConfigPath: targetPath,
				Trigger:    TriggerManual,
				Metadata: SnapshotMetadata{
					Tag:         "before-restore",
					Description: "Automatic backup before restoring snapshot " + snapshot.ID,
				},
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create backup before restore")
			}

			backupSnapshot = backup
		}
	}

	// Reconstruct snapshot content
	content, err := r.ReconstructSnapshot(snapshot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconstruct snapshot")
	}

	// Verify checksum if validation requested
	checksumVerified := false

	if opts.Validate {
		actualHash := ComputeContentHash(content)
		if actualHash != snapshot.Metadata.ConfigHash {
			return nil, errors.Wrapf(
				ErrChecksumMismatch,
				"expected %s, got %s",
				snapshot.Metadata.ConfigHash,
				actualHash,
			)
		}

		checksumVerified = true
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, DirPerm); err != nil {
		return nil, errors.Wrap(err, "failed to create target directory")
	}

	// Write to target path
	if err := os.WriteFile(targetPath, content, FilePerm); err != nil {
		return nil, errors.Wrap(err, "failed to write restored content")
	}

	return &RestoreResult{
		RestoredPath:     targetPath,
		BackupSnapshot:   backupSnapshot,
		BytesRestored:    int64(len(content)),
		ChecksumVerified: checksumVerified,
	}, nil
}

// ReconstructSnapshot reconstructs the full content of a snapshot.
// For full snapshots, this simply reads the stored data.
// For patch snapshots, this applies patches to reconstruct the content.
func (r *Restorer) ReconstructSnapshot(snapshot *Snapshot) ([]byte, error) {
	if snapshot == nil {
		return nil, errors.New("snapshot cannot be nil")
	}

	// For full snapshots, directly read the data
	if snapshot.IsFull() {
		return r.storage.Load(snapshot.StoragePath)
	}

	// Patch reconstruction will be implemented when delta support is added
	return nil, errors.New("patch snapshot reconstruction not yet implemented")
}

// ValidateSnapshot validates a snapshot's integrity.
func (r *Restorer) ValidateSnapshot(snapshot *Snapshot) error {
	if snapshot == nil {
		return errors.New("snapshot cannot be nil")
	}

	// Load snapshot data
	content, err := r.storage.Load(snapshot.StoragePath)
	if err != nil {
		return errors.Wrap(err, "failed to load snapshot data")
	}

	// Verify checksum
	actualHash := ComputeContentHash(content)
	if actualHash != snapshot.Metadata.ConfigHash {
		return errors.Wrapf(
			ErrChecksumMismatch,
			"snapshot %s: expected %s, got %s",
			snapshot.ID,
			snapshot.Metadata.ConfigHash,
			actualHash,
		)
	}

	// For patch snapshots, additional validation will be added later
	if snapshot.IsPatch() {
		// Validate chain integrity when patch support is implemented
		return errors.New("patch snapshot validation not yet implemented")
	}

	return nil
}
