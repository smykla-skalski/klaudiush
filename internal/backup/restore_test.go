package backup_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/backup"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("Restore", func() {
	var (
		tempDir     string
		storage     *backup.FilesystemStorage
		manager     *backup.Manager
		restorer    *backup.Restorer
		testContent []byte
		snapshot    *backup.Snapshot
	)

	BeforeEach(func() {
		// Create temp directory
		var err error

		tempDir, err = os.MkdirTemp("", "klaudiush-restore-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create storage
		storage, err = backup.NewFilesystemStorage(
			tempDir,
			backup.ConfigTypeGlobal,
			"",
		)
		Expect(err).NotTo(HaveOccurred())

		err = storage.Initialize()
		Expect(err).NotTo(HaveOccurred())

		// Create manager
		enabled := true
		autoBackup := true
		maxBackups := 10
		asyncBackup := false

		maxAge := config.Duration(720 * time.Hour)

		cfg := &config.BackupConfig{
			Enabled:     &enabled,
			AutoBackup:  &autoBackup,
			MaxBackups:  &maxBackups,
			MaxAge:      maxAge,
			AsyncBackup: &asyncBackup,
		}

		manager, err = backup.NewManager(storage, cfg)
		Expect(err).NotTo(HaveOccurred())

		// Create restorer
		restorer, err = backup.NewRestorer(storage, manager)
		Expect(err).NotTo(HaveOccurred())

		// Create test content and snapshot
		testContent = []byte("test config content")
		configPath := filepath.Join(tempDir, "config.toml")

		err = os.WriteFile(configPath, testContent, 0o600)
		Expect(err).NotTo(HaveOccurred())

		snapshot, err = manager.CreateBackup(backup.CreateBackupOptions{
			ConfigPath: configPath,
			Trigger:    backup.TriggerManual,
			Metadata: backup.SnapshotMetadata{
				User:    "test-user",
				Command: "test-command",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot).NotTo(BeNil())
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("NewRestorer", func() {
		It("should create a new restorer", func() {
			r, err := backup.NewRestorer(storage, manager)
			Expect(err).NotTo(HaveOccurred())
			Expect(r).NotTo(BeNil())
		})

		It("should return error if storage is nil", func() {
			r, err := backup.NewRestorer(nil, manager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("storage cannot be nil"))
			Expect(r).To(BeNil())
		})

		It("should return error if manager is nil", func() {
			r, err := backup.NewRestorer(storage, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("manager cannot be nil"))
			Expect(r).To(BeNil())
		})
	})

	Describe("ReconstructSnapshot", func() {
		It("should reconstruct full snapshot", func() {
			content, err := restorer.ReconstructSnapshot(snapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(content).To(Equal(testContent))
		})

		It("should return error if snapshot is nil", func() {
			content, err := restorer.ReconstructSnapshot(nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot cannot be nil"))
			Expect(content).To(BeNil())
		})

		It("should return error for patch snapshots (not yet implemented)", func() {
			patchSnapshot := *snapshot
			patchSnapshot.StorageType = backup.StorageTypePatch

			content, err := restorer.ReconstructSnapshot(&patchSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not yet implemented"))
			Expect(content).To(BeNil())
		})
	})

	Describe("ValidateSnapshot", func() {
		It("should validate full snapshot successfully", func() {
			err := restorer.ValidateSnapshot(snapshot)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error if snapshot is nil", func() {
			err := restorer.ValidateSnapshot(nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot cannot be nil"))
		})

		It("should return error if checksum doesn't match", func() {
			// Corrupt the snapshot data
			corruptedContent := []byte("corrupted content")
			err := os.WriteFile(snapshot.StoragePath, corruptedContent, 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = restorer.ValidateSnapshot(snapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("checksum mismatch"))
		})

		It("should return error for patch snapshots (not yet implemented)", func() {
			patchSnapshot := *snapshot
			patchSnapshot.StorageType = backup.StorageTypePatch

			err := restorer.ValidateSnapshot(&patchSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not yet implemented"))
		})

		It("should return error if snapshot file doesn't exist", func() {
			// Delete the snapshot file
			err := os.Remove(snapshot.StoragePath)
			Expect(err).NotTo(HaveOccurred())

			err = restorer.ValidateSnapshot(snapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load snapshot data"))
		})
	})

	Describe("RestoreSnapshot", func() {
		var targetPath string

		BeforeEach(func() {
			targetPath = filepath.Join(tempDir, "restored.toml")
		})

		It("should restore snapshot to target path", func() {
			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath: targetPath,
				Validate:   true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RestoredPath).To(Equal(targetPath))
			Expect(result.BytesRestored).To(Equal(int64(len(testContent))))
			Expect(result.ChecksumVerified).To(BeTrue())
			Expect(result.BackupSnapshot).To(BeNil())

			// Verify content
			restoredContent, err := os.ReadFile(targetPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(restoredContent).To(Equal(testContent))
		})

		It("should restore to original path if target path not specified", func() {
			// Delete original file first
			err := os.Remove(snapshot.ConfigPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath: "",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RestoredPath).To(Equal(snapshot.ConfigPath))

			// Verify content
			restoredContent, err := os.ReadFile(snapshot.ConfigPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(restoredContent).To(Equal(testContent))
		})

		It("should create backup before restore if requested", func() {
			// Create existing file at target
			existingContent := []byte("existing content")

			err := os.WriteFile(targetPath, existingContent, 0o600)
			Expect(err).NotTo(HaveOccurred())

			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath:          targetPath,
				BackupBeforeRestore: true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.BackupSnapshot).NotTo(BeNil())
			Expect(result.BackupSnapshot.Metadata.Tag).To(Equal("before-restore"))

			// Verify the backup contains the existing content
			backupContent, err := restorer.ReconstructSnapshot(result.BackupSnapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(backupContent).To(Equal(existingContent))

			// Verify target has new content
			restoredContent, err := os.ReadFile(targetPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(restoredContent).To(Equal(testContent))
		})

		It("should not create backup if file doesn't exist", func() {
			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath:          targetPath,
				BackupBeforeRestore: true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.BackupSnapshot).To(BeNil())
		})

		It("should not create backup if force is true", func() {
			// Create existing file at target
			existingContent := []byte("existing content")

			err := os.WriteFile(targetPath, existingContent, 0o600)
			Expect(err).NotTo(HaveOccurred())

			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath:          targetPath,
				BackupBeforeRestore: true,
				Force:               true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.BackupSnapshot).To(BeNil())
		})

		It("should validate checksum if requested", func() {
			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath: targetPath,
				Validate:   true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ChecksumVerified).To(BeTrue())
		})

		It("should return error if checksum validation fails", func() {
			// Corrupt the snapshot data
			corruptedContent := []byte("corrupted content")
			err := os.WriteFile(snapshot.StoragePath, corruptedContent, 0o600)
			Expect(err).NotTo(HaveOccurred())

			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath: targetPath,
				Validate:   true,
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot validation failed"))
			Expect(result).To(BeNil())
		})

		It("should return error if snapshot is nil", func() {
			result, err := restorer.RestoreSnapshot(nil, backup.RestoreOptions{
				TargetPath: targetPath,
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot cannot be nil"))
			Expect(result).To(BeNil())
		})

		It("should return error if target path is empty and snapshot has no config path", func() {
			emptySnapshot := *snapshot
			emptySnapshot.ConfigPath = ""

			result, err := restorer.RestoreSnapshot(&emptySnapshot, backup.RestoreOptions{
				TargetPath: "",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("target path is required"))
			Expect(result).To(BeNil())
		})

		It("should create target directory if it doesn't exist", func() {
			nestedPath := filepath.Join(tempDir, "nested", "deep", "restored.toml")

			result, err := restorer.RestoreSnapshot(snapshot, backup.RestoreOptions{
				TargetPath: nestedPath,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Verify directory was created
			_, err = os.Stat(filepath.Dir(nestedPath))
			Expect(err).NotTo(HaveOccurred())

			// Verify content
			restoredContent, err := os.ReadFile(nestedPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(restoredContent).To(Equal(testContent))
		})
	})

	Describe("Manager.RestoreSnapshot", func() {
		var targetPath string

		BeforeEach(func() {
			targetPath = filepath.Join(tempDir, "manager-restored.toml")
		})

		It("should restore snapshot via manager", func() {
			result, err := manager.RestoreSnapshot(snapshot.ID, backup.RestoreOptions{
				TargetPath: targetPath,
				Validate:   true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RestoredPath).To(Equal(targetPath))
			Expect(result.ChecksumVerified).To(BeTrue())

			// Verify content
			restoredContent, err := os.ReadFile(targetPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(restoredContent).To(Equal(testContent))
		})

		It("should return error if backup is disabled", func() {
			disabled := false
			disabledCfg := &config.BackupConfig{Enabled: &disabled}
			disabledManager, err := backup.NewManager(storage, disabledCfg)
			Expect(err).NotTo(HaveOccurred())

			result, err := disabledManager.RestoreSnapshot(snapshot.ID, backup.RestoreOptions{
				TargetPath: targetPath,
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup system is disabled"))
			Expect(result).To(BeNil())
		})

		It("should return error if snapshot not found", func() {
			result, err := manager.RestoreSnapshot("nonexistent", backup.RestoreOptions{
				TargetPath: targetPath,
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot not found"))
			Expect(result).To(BeNil())
		})
	})

	Describe("Manager.ValidateSnapshot", func() {
		It("should validate snapshot via manager", func() {
			err := manager.ValidateSnapshot(snapshot.ID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error if backup is disabled", func() {
			disabled := false
			disabledCfg := &config.BackupConfig{Enabled: &disabled}
			disabledManager, err := backup.NewManager(storage, disabledCfg)
			Expect(err).NotTo(HaveOccurred())

			err = disabledManager.ValidateSnapshot(snapshot.ID)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup system is disabled"))
		})

		It("should return error if snapshot not found", func() {
			err := manager.ValidateSnapshot("nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot not found"))
		})

		It("should return error if checksum doesn't match", func() {
			// Corrupt the snapshot data
			corruptedContent := []byte("corrupted content")
			err := os.WriteFile(snapshot.StoragePath, corruptedContent, 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = manager.ValidateSnapshot(snapshot.ID)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
			Expect(err.Error()).To(ContainSubstring("checksum mismatch"))
		})
	})
})
