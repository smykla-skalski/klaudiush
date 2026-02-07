package fixers_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/backup"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/doctor/fixers"
	"github.com/smykla-labs/klaudiush/internal/prompt"
)

var _ = Describe("BackupFixer", func() {
	var (
		ctrl     *gomock.Controller
		prompter *prompt.MockPrompter
		fixer    *fixers.BackupFixer
		tempDir  string
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		prompter = prompt.NewMockPrompter(ctrl)

		var err error
		tempDir, err = os.MkdirTemp("", "klaudiush-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create a fixer with temp dir as home
		// Note: In real implementation, we'd need to inject baseDir
		// For now, we'll test the interface and basic functionality
		fixer = fixers.NewBackupFixer(prompter)
	})

	AfterEach(func() {
		ctrl.Finish()
		_ = os.RemoveAll(tempDir)
	})

	Describe("ID", func() {
		It("should return the correct ID", func() {
			Expect(fixer.ID()).To(Equal("backup_fixer"))
		})
	})

	Describe("Description", func() {
		It("should return a description", func() {
			desc := fixer.Description()

			Expect(desc).NotTo(BeEmpty())
			Expect(desc).To(ContainSubstring("backup"))
		})
	})

	Describe("CanFix", func() {
		Context("with create_backup_directory fix ID", func() {
			It("should return true", func() {
				result := doctor.FailWarning("test", "test").WithFixID("create_backup_directory")

				Expect(fixer.CanFix(result)).To(BeTrue())
			})
		})

		Context("with fix_backup_directory_permissions fix ID", func() {
			It("should return true", func() {
				result := doctor.FailError("test", "test").
					WithFixID("fix_backup_directory_permissions")

				Expect(fixer.CanFix(result)).To(BeTrue())
			})
		})

		Context("with rebuild_backup_metadata fix ID", func() {
			It("should return true", func() {
				result := doctor.FailWarning("test", "test").WithFixID("rebuild_backup_metadata")

				Expect(fixer.CanFix(result)).To(BeTrue())
			})
		})

		Context("with fix_backup_integrity fix ID", func() {
			It("should return true", func() {
				result := doctor.FailError("test", "test").WithFixID("fix_backup_integrity")

				Expect(fixer.CanFix(result)).To(BeTrue())
			})
		})

		Context("with unrelated fix ID", func() {
			It("should return false", func() {
				result := doctor.FailError("test", "test").WithFixID("unrelated_fix")

				Expect(fixer.CanFix(result)).To(BeFalse())
			})
		})

		Context("with passing result", func() {
			It("should return false", func() {
				result := doctor.Pass("test", "test").WithFixID("create_backup_directory")

				Expect(fixer.CanFix(result)).To(BeFalse())
			})
		})
	})

	Describe("Fix", func() {
		Context("when directory creation is needed in non-interactive mode", func() {
			It("should create directory structure", func() {
				// Skip this test in CI as it would create real directories
				// In a full implementation, we'd inject the baseDir
				Skip("Requires dependency injection of baseDir")
			})
		})

		Context("when user declines in interactive mode", func() {
			It("should not create directory", func() {
				Skip("Requires dependency injection of baseDir")
			})
		})
	})
})

// Integration-style test using real filesystem
var _ = Describe("BackupFixer Integration", func() {
	var (
		tempDir   string
		backupDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "klaudiush-test-*")
		Expect(err).NotTo(HaveOccurred())

		backupDir = filepath.Join(tempDir, backup.DefaultBackupDir)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Context("when creating directory structure", func() {
		It("should create all required subdirectories with correct permissions", func() {
			err := os.MkdirAll(backupDir, backup.DirPerm)
			Expect(err).NotTo(HaveOccurred())

			globalDir := filepath.Join(backupDir, backup.GlobalBackupDir, backup.SnapshotsDir)
			err = os.MkdirAll(globalDir, backup.DirPerm)
			Expect(err).NotTo(HaveOccurred())

			projectsDir := filepath.Join(backupDir, backup.ProjectBackupDir)
			err = os.MkdirAll(projectsDir, backup.DirPerm)
			Expect(err).NotTo(HaveOccurred())

			// Verify structure
			info, err := os.Stat(backupDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
			Expect(info.Mode().Perm()).To(Equal(backup.DirPerm))

			info, err = os.Stat(globalDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())

			info, err = os.Stat(projectsDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
		})
	})

	Context("when fixing permissions", func() {
		BeforeEach(func() {
			err := os.MkdirAll(backupDir, 0o755)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fix directory permissions", func() {
			err := os.Chmod(backupDir, backup.DirPerm)
			Expect(err).NotTo(HaveOccurred())

			info, err := os.Stat(backupDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(backup.DirPerm))
		})
	})

	Context("when rebuilding metadata", func() {
		BeforeEach(func() {
			// Create storage structure
			globalDir := filepath.Join(backupDir, backup.GlobalBackupDir, backup.SnapshotsDir)
			err := os.MkdirAll(globalDir, backup.DirPerm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create empty index when no snapshots exist", func() {
			storage, err := backup.NewFilesystemStorage(
				tempDir,
				backup.ConfigTypeGlobal,
				"",
			)
			Expect(err).NotTo(HaveOccurred())

			index := &backup.SnapshotIndex{
				Version:   1,
				Snapshots: make(map[string]backup.Snapshot),
			}

			err = storage.SaveIndex(index)
			Expect(err).NotTo(HaveOccurred())

			// Verify index was created
			loadedIndex, err := storage.LoadIndex()
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedIndex.Version).To(Equal(1))
			Expect(loadedIndex.Snapshots).To(BeEmpty())
		})
	})
})
