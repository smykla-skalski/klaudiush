package backupchecker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/backup"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	backupchecker "github.com/smykla-labs/klaudiush/internal/doctor/checkers/backup"
)

func TestBackupChecker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backup Checker Suite")
}

var _ = Describe("DirectoryChecker", func() {
	var (
		ctx      context.Context
		ctrl     *gomock.Controller
		provider *backupchecker.MockStorageProvider
		checker  *backupchecker.DirectoryChecker
		tempDir  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		provider = backupchecker.NewMockStorageProvider(ctrl)

		var err error
		tempDir, err = os.MkdirTemp("", "klaudiush-test-*")
		Expect(err).NotTo(HaveOccurred())

		provider.EXPECT().GetBaseDir().Return(tempDir).AnyTimes()

		checker = backupchecker.NewDirectoryCheckerWithProvider(provider)
	})

	AfterEach(func() {
		ctrl.Finish()
		_ = os.RemoveAll(tempDir)
	})

	Context("when backup directory doesn't exist", func() {
		It("should return warning with fix ID", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
			Expect(result.Message).To(ContainSubstring("Not found"))
			Expect(result.FixID).To(Equal("create_backup_directory"))
		})
	})

	Context("when backup path exists but is not a directory", func() {
		BeforeEach(func() {
			backupPath := filepath.Join(tempDir, backup.DefaultBackupDir)
			err := os.WriteFile(backupPath, []byte("test"), 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityError))
			Expect(result.Message).To(ContainSubstring("not a directory"))
		})
	})

	Context("when backup directory has wrong permissions", func() {
		BeforeEach(func() {
			backupDir := filepath.Join(tempDir, backup.DefaultBackupDir)
			err := os.MkdirAll(backupDir, 0o755)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error with fix ID", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityError))
			Expect(result.Message).To(ContainSubstring("Insecure"))
			Expect(result.FixID).To(Equal("fix_backup_directory_permissions"))
		})
	})

	Context("when backup directory is properly configured", func() {
		BeforeEach(func() {
			backupDir := filepath.Join(tempDir, backup.DefaultBackupDir)
			err := os.MkdirAll(backupDir, backup.DirPerm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return pass", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(ContainSubstring("secure permissions"))
		})
	})
})

var _ = Describe("MetadataChecker", func() {
	var (
		ctx      context.Context
		ctrl     *gomock.Controller
		provider *backupchecker.MockStorageProvider
		storage  *backup.MockStorage
		checker  *backupchecker.MetadataChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		provider = backupchecker.NewMockStorageProvider(ctrl)
		storage = backup.NewMockStorage(ctrl)

		checker = backupchecker.NewMetadataCheckerWithProvider(provider)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("when storage doesn't exist", func() {
		BeforeEach(func() {
			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(false)
		})

		It("should skip check", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
			Expect(result.Message).To(ContainSubstring("not initialized"))
		})
	})

	Context("when storage exists but index is not found", func() {
		BeforeEach(func() {
			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(true)
			storage.EXPECT().LoadIndex().Return(nil, backup.ErrStorageNotInitialized)
		})

		It("should return warning with fix ID", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
			Expect(result.Message).To(ContainSubstring("Index not found"))
			Expect(result.FixID).To(Equal("rebuild_backup_metadata"))
		})
	})

	Context("when index is corrupted", func() {
		BeforeEach(func() {
			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(true)
			storage.EXPECT().LoadIndex().Return(nil, os.ErrInvalid)
		})

		It("should return error with fix ID", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityError))
			Expect(result.Message).To(ContainSubstring("Failed to load"))
			Expect(result.FixID).To(Equal("rebuild_backup_metadata"))
		})
	})

	Context("when index is valid", func() {
		BeforeEach(func() {
			index := &backup.SnapshotIndex{
				Version:   1,
				Snapshots: make(map[string]backup.Snapshot),
			}

			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(true)
			storage.EXPECT().LoadIndex().Return(index, nil)
		})

		It("should return pass", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(ContainSubstring("Valid"))
		})
	})
})

var _ = Describe("IntegrityChecker", func() {
	var (
		ctx      context.Context
		ctrl     *gomock.Controller
		provider *backupchecker.MockStorageProvider
		storage  *backup.MockStorage
		checker  *backupchecker.IntegrityChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		provider = backupchecker.NewMockStorageProvider(ctrl)
		storage = backup.NewMockStorage(ctrl)

		checker = backupchecker.NewIntegrityCheckerWithProvider(provider)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("when storage doesn't exist", func() {
		BeforeEach(func() {
			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(false)
		})

		It("should skip check", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})
	})

	Context("when metadata cannot be loaded", func() {
		BeforeEach(func() {
			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(true)
			storage.EXPECT().LoadIndex().Return(nil, os.ErrInvalid)
		})

		It("should skip check", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
			Expect(result.Message).To(ContainSubstring("Cannot check without valid metadata"))
		})
	})

	Context("when snapshot file is missing", func() {
		BeforeEach(func() {
			index := &backup.SnapshotIndex{
				Version: 1,
				Snapshots: map[string]backup.Snapshot{
					"snap1": {
						ID:          "snap1",
						StoragePath: "snap1.full.toml",
					},
				},
			}

			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(true)
			storage.EXPECT().LoadIndex().Return(index, nil)
			storage.EXPECT().Load("snap1.full.toml").Return(nil, os.ErrNotExist)
		})

		It("should return error with fix ID", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityError))
			Expect(result.Message).To(ContainSubstring("integrity issues"))
			Expect(result.FixID).To(Equal("fix_backup_integrity"))
			Expect(result.Details).To(ContainElement(ContainSubstring("file missing")))
		})
	})

	Context("when all snapshots are valid", func() {
		BeforeEach(func() {
			index := &backup.SnapshotIndex{
				Version: 1,
				Snapshots: map[string]backup.Snapshot{
					"snap1": {
						ID:          "snap1",
						StoragePath: "snap1.full.toml",
					},
					"snap2": {
						ID:          "snap2",
						StoragePath: "snap2.full.toml",
					},
				},
			}

			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(true)
			storage.EXPECT().LoadIndex().Return(index, nil)
			storage.EXPECT().Load("snap1.full.toml").Return([]byte("data"), nil)
			storage.EXPECT().Load("snap2.full.toml").Return([]byte("data"), nil)
		})

		It("should return pass", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(ContainSubstring("2 snapshots verified"))
		})
	})

	Context("when index is empty", func() {
		BeforeEach(func() {
			index := &backup.SnapshotIndex{
				Version:   1,
				Snapshots: make(map[string]backup.Snapshot),
			}

			provider.EXPECT().GetGlobalStorage().Return(storage, nil)
			storage.EXPECT().Exists().Return(true)
			storage.EXPECT().LoadIndex().Return(index, nil)
		})

		It("should return pass with zero count", func() {
			result := checker.Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(ContainSubstring("0 snapshots verified"))
		})
	})
})

var _ = Describe("DefaultStorageProvider", func() {
	var provider *backupchecker.DefaultStorageProvider

	BeforeEach(func() {
		var err error
		provider, err = backupchecker.NewDefaultStorageProvider()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("GetGlobalStorage", func() {
		It("should return global storage", func() {
			storage, err := provider.GetGlobalStorage()

			Expect(err).NotTo(HaveOccurred())
			Expect(storage).NotTo(BeNil())
		})
	})

	Describe("GetProjectStorage", func() {
		It("should return project storage", func() {
			storage, err := provider.GetProjectStorage("/tmp/project")

			Expect(err).NotTo(HaveOccurred())
			Expect(storage).NotTo(BeNil())
		})
	})

	Describe("GetBaseDir", func() {
		It("should return base directory", func() {
			baseDir := provider.GetBaseDir()

			Expect(baseDir).To(ContainSubstring(".klaudiush"))
		})
	})
})
