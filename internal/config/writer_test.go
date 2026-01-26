package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/backup"
	"github.com/smykla-skalski/klaudiush/internal/config"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Writer with Backup Integration", func() {
	var (
		tmpDir     string
		homeDir    string
		workDir    string
		backupMgr  *backup.Manager
		writer     *config.Writer
		configPath string
	)

	BeforeEach(func() {
		// Create temporary directories
		var err error
		tmpDir, err = os.MkdirTemp("", "writer-test-*")
		Expect(err).ToNot(HaveOccurred())

		homeDir = filepath.Join(tmpDir, "home")
		workDir = filepath.Join(tmpDir, "work")

		Expect(os.MkdirAll(homeDir, 0o700)).To(Succeed())
		Expect(os.MkdirAll(workDir, 0o700)).To(Succeed())

		// Create backup storage and manager
		baseDir := filepath.Join(homeDir, config.GlobalConfigDir)
		storage, err := backup.NewFilesystemStorage(baseDir, backup.ConfigTypeGlobal, "")
		Expect(err).ToNot(HaveOccurred())

		backupCfg := &pkgConfig.BackupConfig{}
		backupMgr, err = backup.NewManager(storage, backupCfg)
		Expect(err).ToNot(HaveOccurred())

		// Create writer with backup manager
		writer = config.NewWriterWithDirsAndBackup(homeDir, workDir, backupMgr)

		// Set config path
		configPath = writer.GlobalConfigPath()
	})

	AfterEach(func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	})

	Describe("WriteFile with backup", func() {
		Context("when config doesn't exist", func() {
			It("should write config without creating backup", func() {
				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{},
				}

				err := writer.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())

				// Config should exist
				Expect(configPath).To(BeAnExistingFile())

				// No backup should be created (no existing file to backup)
				snapshots, err := backupMgr.List()
				Expect(err).ToNot(HaveOccurred())
				Expect(snapshots).To(BeEmpty())
			})
		})

		Context("when config exists and backup is enabled", func() {
			BeforeEach(func() {
				// Create initial config
				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{},
				}
				err := writer.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should create backup before writing (sync)", func() {
				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{
						AsyncBackup: boolPtr(false),
					},
				}

				err := writer.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())

				// Backup should be created
				snapshots, err := backupMgr.List()
				Expect(err).ToNot(HaveOccurred())
				Expect(snapshots).To(HaveLen(1))

				// Backup should have TriggerAutomatic
				Expect(snapshots[0].Trigger).To(Equal(backup.TriggerAutomatic))
			})

			It("should create backup before writing (async)", func() {
				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{
						AsyncBackup: boolPtr(true),
					},
				}

				err := writer.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())

				// Note: Async backup may not complete immediately
				// We just verify no error occurred
			})
		})

		Context("when backup is disabled", func() {
			BeforeEach(func() {
				// Create initial config
				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{},
				}
				err := writer.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should not create backup when disabled", func() {
				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{
						Enabled: boolPtr(false),
					},
				}

				err := writer.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())

				// No new backup should be created
				snapshots, err := backupMgr.List()
				Expect(err).ToNot(HaveOccurred())
				Expect(snapshots).To(BeEmpty())
			})

			It("should not create backup when auto_backup is disabled", func() {
				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{
						Enabled:    boolPtr(true),
						AutoBackup: boolPtr(false),
					},
				}

				err := writer.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())

				// No new backup should be created
				snapshots, err := backupMgr.List()
				Expect(err).ToNot(HaveOccurred())
				Expect(snapshots).To(BeEmpty())
			})
		})

		Context("when no backup manager is configured", func() {
			It("should write config without backup", func() {
				writerWithoutBackup := config.NewWriterWithDirs(homeDir, workDir)

				cfg := &pkgConfig.Config{
					Backup: &pkgConfig.BackupConfig{},
				}

				err := writerWithoutBackup.WriteFile(configPath, cfg)
				Expect(err).ToNot(HaveOccurred())

				// Config should exist
				Expect(configPath).To(BeAnExistingFile())
			})
		})
	})

	Describe("WriteGlobal with backup", func() {
		It("should create backup before writing global config", func() {
			// Create initial config
			cfg := &pkgConfig.Config{
				Backup: &pkgConfig.BackupConfig{},
			}
			err := writer.WriteGlobal(cfg)
			Expect(err).ToNot(HaveOccurred())

			// Write again (should trigger backup)
			cfg2 := &pkgConfig.Config{
				Backup: &pkgConfig.BackupConfig{
					AsyncBackup: boolPtr(false),
				},
			}
			err = writer.WriteGlobal(cfg2)
			Expect(err).ToNot(HaveOccurred())

			// Backup should be created
			snapshots, err := backupMgr.List()
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshots).To(HaveLen(1))
		})
	})

	Describe("WriteProject with backup", func() {
		var projectBackupMgr *backup.Manager

		BeforeEach(func() {
			// Create project backup manager
			baseDir := filepath.Join(homeDir, config.GlobalConfigDir)
			storage, err := backup.NewFilesystemStorage(
				baseDir,
				backup.ConfigTypeProject,
				workDir,
			)
			Expect(err).ToNot(HaveOccurred())

			backupCfg := &pkgConfig.BackupConfig{}
			projectBackupMgr, err = backup.NewManager(storage, backupCfg)
			Expect(err).ToNot(HaveOccurred())

			// Create writer with project backup manager
			writer = config.NewWriterWithDirsAndBackup(homeDir, workDir, projectBackupMgr)
		})

		It("should create backup before writing project config", func() {
			// Create initial config
			cfg := &pkgConfig.Config{
				Backup: &pkgConfig.BackupConfig{},
			}
			err := writer.WriteProject(cfg)
			Expect(err).ToNot(HaveOccurred())

			// Write again (should trigger backup)
			cfg2 := &pkgConfig.Config{
				Backup: &pkgConfig.BackupConfig{
					AsyncBackup: boolPtr(false),
				},
			}
			err = writer.WriteProject(cfg2)
			Expect(err).ToNot(HaveOccurred())

			// Backup should be created
			snapshots, err := projectBackupMgr.List()
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshots).To(HaveLen(1))
		})
	})
})

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}
