package fixers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/backup"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/prompt"
)

// BackupFixer fixes issues with the backup system.
type BackupFixer struct {
	prompter prompt.Prompter
	baseDir  string
}

// NewBackupFixer creates a new BackupFixer.
func NewBackupFixer(prompter prompt.Prompter) *BackupFixer {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".klaudiush")

	return &BackupFixer{
		prompter: prompter,
		baseDir:  baseDir,
	}
}

// ID returns the fixer identifier.
func (*BackupFixer) ID() string {
	return "backup_fixer"
}

// Description returns a human-readable description.
func (*BackupFixer) Description() string {
	return "Fix backup system issues (directory, permissions, metadata)"
}

// CanFix checks if this fixer can fix the given result.
func (*BackupFixer) CanFix(result doctor.CheckResult) bool {
	return (result.FixID == "create_backup_directory" ||
		result.FixID == "fix_backup_directory_permissions" ||
		result.FixID == "rebuild_backup_metadata" ||
		result.FixID == "fix_backup_integrity") &&
		result.Status == doctor.StatusFail
}

// Fix attempts to fix backup system issues.
func (f *BackupFixer) Fix(ctx context.Context, interactive bool) error {
	backupDir := filepath.Join(f.baseDir, backup.DefaultBackupDir)

	// Check what needs fixing
	info, err := os.Stat(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return f.createDirectory(ctx, interactive, backupDir)
		}

		return errors.Wrap(err, "failed to stat backup directory")
	}

	// If it's not a directory, we can't fix it automatically
	if !info.IsDir() {
		return errors.New(
			"backup path exists but is not a directory - manual intervention required",
		)
	}

	// Fix permissions if needed
	perm := info.Mode().Perm()
	if perm != backup.DirPerm {
		return f.fixPermissions(ctx, interactive, backupDir)
	}

	// Check if metadata needs rebuilding
	globalStorage, err := backup.NewFilesystemStorage(f.baseDir, backup.ConfigTypeGlobal, "")
	if err != nil {
		return errors.Wrap(err, "failed to create storage")
	}

	if globalStorage.Exists() {
		_, loadErr := globalStorage.LoadIndex()
		if loadErr != nil {
			return f.rebuildMetadata(ctx, interactive, globalStorage)
		}
	}

	return nil
}

// createDirectory creates the backup directory structure.
func (f *BackupFixer) createDirectory(
	_ context.Context,
	interactive bool,
	backupDir string,
) error {
	if interactive {
		msg := fmt.Sprintf("Create backup directory at %s?", backupDir)

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	// Create the base backup directory
	if err := os.MkdirAll(backupDir, backup.DirPerm); err != nil {
		return errors.Wrap(err, "failed to create backup directory")
	}

	// Create subdirectories for global and projects
	globalDir := filepath.Join(backupDir, backup.GlobalBackupDir, backup.SnapshotsDir)
	if err := os.MkdirAll(globalDir, backup.DirPerm); err != nil {
		return errors.Wrap(err, "failed to create global snapshots directory")
	}

	projectsDir := filepath.Join(backupDir, backup.ProjectBackupDir)
	if err := os.MkdirAll(projectsDir, backup.DirPerm); err != nil {
		return errors.Wrap(err, "failed to create projects directory")
	}

	return nil
}

// fixPermissions fixes the permissions on the backup directory.
func (f *BackupFixer) fixPermissions(
	_ context.Context,
	interactive bool,
	backupDir string,
) error {
	if interactive {
		msg := fmt.Sprintf("Fix permissions on %s to %04o?", backupDir, backup.DirPerm)

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	// Fix the base directory
	if err := os.Chmod(backupDir, backup.DirPerm); err != nil {
		return errors.Wrap(err, "failed to fix directory permissions")
	}

	// Also fix subdirectories
	entries := []string{
		filepath.Join(backupDir, backup.GlobalBackupDir),
		filepath.Join(backupDir, backup.GlobalBackupDir, backup.SnapshotsDir),
		filepath.Join(backupDir, backup.ProjectBackupDir),
	}

	for _, dir := range entries {
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			if chmodErr := os.Chmod(dir, backup.DirPerm); chmodErr != nil {
				return errors.Wrapf(chmodErr, "failed to fix permissions on %s", dir)
			}
		}
	}

	return nil
}

// rebuildMetadata rebuilds the metadata.json index by scanning snapshot files.
func (f *BackupFixer) rebuildMetadata(
	_ context.Context,
	interactive bool,
	storage backup.Storage,
) error {
	if interactive {
		msg := "Rebuild backup metadata index from snapshot files?"

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	// List all snapshot files
	paths, err := storage.List()
	if err != nil {
		return errors.Wrap(err, "failed to list snapshots")
	}

	// Create new empty index using the NewSnapshotIndex constructor
	index := backup.NewSnapshotIndex()

	// For now, we can't fully reconstruct metadata from files alone
	// as we need additional info like triggers, metadata, etc.
	// This fixer creates an empty index that will be populated
	// as new backups are created.

	// In a real implementation, we could:
	// 1. Parse snapshot filenames to extract IDs
	// 2. Read files to compute checksums
	// 3. Infer chain relationships from sequences
	// For Phase 7, we'll create a minimal index

	if len(paths) > 0 {
		// Note: This is a simplified implementation
		// A full implementation would parse each snapshot file
		return errors.New("rebuilding index from existing snapshots requires manual intervention")
	}

	// Save the empty index
	if err := storage.SaveIndex(index); err != nil {
		return errors.Wrap(err, "failed to save index")
	}

	return nil
}
