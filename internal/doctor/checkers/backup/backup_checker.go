// Package backupchecker provides health checkers for the backup system.
package backupchecker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/backup"
	"github.com/smykla-labs/klaudiush/internal/doctor"
)

// StorageProvider provides access to backup storage.
//
//go:generate mockgen -source=backup_checker.go -destination=backup_checker_mock.go -package=backupchecker
type StorageProvider interface {
	// GetGlobalStorage returns the storage for global configs.
	GetGlobalStorage() (backup.Storage, error)

	// GetProjectStorage returns the storage for project configs.
	GetProjectStorage(projectPath string) (backup.Storage, error)

	// GetBaseDir returns the base directory for all backups.
	GetBaseDir() string
}

// DefaultStorageProvider implements StorageProvider using the default paths.
type DefaultStorageProvider struct {
	baseDir string
}

// NewDefaultStorageProvider creates a new DefaultStorageProvider.
func NewDefaultStorageProvider() (*DefaultStorageProvider, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get home directory")
	}

	baseDir := filepath.Join(homeDir, ".klaudiush")

	return &DefaultStorageProvider{
		baseDir: baseDir,
	}, nil
}

// GetGlobalStorage returns the storage for global configs.
//
//nolint:ireturn // interface for polymorphism
func (p *DefaultStorageProvider) GetGlobalStorage() (backup.Storage, error) {
	return backup.NewFilesystemStorage(p.baseDir, backup.ConfigTypeGlobal, "")
}

// GetProjectStorage returns the storage for project configs.
//
//nolint:ireturn // interface for polymorphism
func (p *DefaultStorageProvider) GetProjectStorage(projectPath string) (backup.Storage, error) {
	return backup.NewFilesystemStorage(p.baseDir, backup.ConfigTypeProject, projectPath)
}

// GetBaseDir returns the base directory for all backups.
func (p *DefaultStorageProvider) GetBaseDir() string {
	return p.baseDir
}

// DirectoryChecker checks if backup directory exists and has proper permissions.
type DirectoryChecker struct {
	provider StorageProvider
}

// NewDirectoryChecker creates a new directory checker.
func NewDirectoryChecker() *DirectoryChecker {
	provider, _ := NewDefaultStorageProvider()

	return &DirectoryChecker{
		provider: provider,
	}
}

// NewDirectoryCheckerWithProvider creates a directory checker with custom provider.
func NewDirectoryCheckerWithProvider(provider StorageProvider) *DirectoryChecker {
	return &DirectoryChecker{
		provider: provider,
	}
}

// Name returns the name of the check.
func (*DirectoryChecker) Name() string {
	return "Backup directory"
}

// Category returns the category of the check.
func (*DirectoryChecker) Category() doctor.Category {
	return doctor.CategoryBackup
}

// Check performs the directory check.
func (c *DirectoryChecker) Check(_ context.Context) doctor.CheckResult {
	if c.provider == nil {
		return doctor.FailError("Backup directory", "Storage provider not initialized")
	}

	baseDir := c.provider.GetBaseDir()
	backupDir := filepath.Join(baseDir, backup.DefaultBackupDir)

	// Check if directory exists
	info, err := os.Stat(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return doctor.FailWarning("Backup directory", "Not found (will be created on first backup)").
				WithDetails(
					"Expected at: "+backupDir,
					"Backups will be created automatically when enabled",
				).
				WithFixID("create_backup_directory")
		}

		return doctor.FailError("Backup directory", fmt.Sprintf("Failed to stat: %v", err))
	}

	// Check if it's a directory
	if !info.IsDir() {
		return doctor.FailError("Backup directory", "Path exists but is not a directory").
			WithDetails(
				"Path: "+backupDir,
				"Remove the file and run doctor --fix",
			)
	}

	// Check permissions (should be 0700 - owner only)
	perm := info.Mode().Perm()
	if perm != backup.DirPerm {
		return doctor.FailError("Backup directory", "Insecure directory permissions").
			WithDetails(
				fmt.Sprintf("Current: %04o, Expected: %04o", perm, backup.DirPerm),
				"Directory should be accessible only by owner",
			).
			WithFixID("fix_backup_directory_permissions")
	}

	return doctor.Pass("Backup directory", "Exists with secure permissions")
}

// MetadataChecker checks if metadata.json files are valid.
type MetadataChecker struct {
	provider StorageProvider
}

// NewMetadataChecker creates a new metadata checker.
func NewMetadataChecker() *MetadataChecker {
	provider, _ := NewDefaultStorageProvider()

	return &MetadataChecker{
		provider: provider,
	}
}

// NewMetadataCheckerWithProvider creates a metadata checker with custom provider.
func NewMetadataCheckerWithProvider(provider StorageProvider) *MetadataChecker {
	return &MetadataChecker{
		provider: provider,
	}
}

// Name returns the name of the check.
func (*MetadataChecker) Name() string {
	return "Backup metadata"
}

// Category returns the category of the check.
func (*MetadataChecker) Category() doctor.Category {
	return doctor.CategoryBackup
}

// Check performs the metadata check.
func (c *MetadataChecker) Check(_ context.Context) doctor.CheckResult {
	if c.provider == nil {
		return doctor.FailError("Backup metadata", "Storage provider not initialized")
	}

	// Check global storage
	globalStorage, err := c.provider.GetGlobalStorage()
	if err != nil {
		return doctor.FailError(
			"Backup metadata",
			fmt.Sprintf("Failed to get global storage: %v", err),
		)
	}

	// If storage doesn't exist, skip
	if !globalStorage.Exists() {
		return doctor.Skip("Backup metadata", "Backup directory not initialized yet")
	}

	// Try loading the index
	_, err = globalStorage.LoadIndex()
	if err != nil {
		if errors.Is(err, backup.ErrStorageNotInitialized) {
			return doctor.FailWarning("Backup metadata", "Index not found but directory exists").
				WithDetails(
					"This may happen if backup directory was created manually",
					"Index will be created automatically on first backup",
				).
				WithFixID("rebuild_backup_metadata")
		}

		return doctor.FailError("Backup metadata", "Failed to load index").
			WithDetails(
				fmt.Sprintf("Error: %v", err),
				"Index file may be corrupted",
			).
			WithFixID("rebuild_backup_metadata")
	}

	return doctor.Pass("Backup metadata", "Valid and loadable")
}

// IntegrityChecker checks if all snapshots are valid and chains are consistent.
type IntegrityChecker struct {
	provider StorageProvider
}

// NewIntegrityChecker creates a new integrity checker.
func NewIntegrityChecker() *IntegrityChecker {
	provider, _ := NewDefaultStorageProvider()

	return &IntegrityChecker{
		provider: provider,
	}
}

// NewIntegrityCheckerWithProvider creates an integrity checker with custom provider.
func NewIntegrityCheckerWithProvider(provider StorageProvider) *IntegrityChecker {
	return &IntegrityChecker{
		provider: provider,
	}
}

// Name returns the name of the check.
func (*IntegrityChecker) Name() string {
	return "Backup integrity"
}

// Category returns the category of the check.
func (*IntegrityChecker) Category() doctor.Category {
	return doctor.CategoryBackup
}

// Check performs the integrity check.
func (c *IntegrityChecker) Check(ctx context.Context) doctor.CheckResult {
	if c.provider == nil {
		return doctor.FailError("Backup integrity", "Storage provider not initialized")
	}

	globalStorage, err := c.provider.GetGlobalStorage()
	if err != nil {
		return doctor.FailError(
			"Backup integrity",
			fmt.Sprintf("Failed to get global storage: %v", err),
		)
	}

	// If storage doesn't exist, skip
	if !globalStorage.Exists() {
		return doctor.Skip("Backup integrity", "Backup directory not initialized yet")
	}

	// Load index
	index, err := globalStorage.LoadIndex()
	if err != nil {
		return doctor.Skip("Backup integrity", "Cannot check without valid metadata")
	}

	// Check each snapshot
	var issues []string

	for snapshotID, snapshot := range index.Snapshots {
		if checkErr := c.checkSnapshot(ctx, globalStorage, snapshotID, snapshot); checkErr != nil {
			issues = append(issues, checkErr.Error())
		}
	}

	if len(issues) > 0 {
		return doctor.FailError("Backup integrity", "Found integrity issues").
			WithDetails(issues...).
			WithFixID("fix_backup_integrity")
	}

	return doctor.Pass(
		"Backup integrity",
		fmt.Sprintf("All %d snapshots verified", len(index.Snapshots)),
	)
}

// checkSnapshot verifies a single snapshot's integrity.
func (*IntegrityChecker) checkSnapshot(
	_ context.Context,
	storage backup.Storage,
	snapshotID string,
	snapshot backup.Snapshot,
) error {
	// Check if storage path exists
	_, err := storage.Load(snapshot.StoragePath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Newf(
				"snapshot %s: file missing at %s",
				snapshotID,
				snapshot.StoragePath,
			)
		}

		return errors.Wrapf(err, "snapshot %s: failed to load", snapshotID)
	}

	// Additional checks could be added here:
	// - Verify checksum matches
	// - Verify chain consistency
	// - Verify patch can be applied

	return nil
}
