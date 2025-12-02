package main

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/backup"
	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

func TestMigration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migration Suite")
}

var _ = Describe("Migration", func() {
	var (
		tempDir string
		log     logger.Logger
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "klaudiush-migration-test-*")
		Expect(err).NotTo(HaveOccurred())

		log = logger.NewNoOpLogger()
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("performFirstRunMigration", func() {
		Context("when migration marker does not exist", func() {
			It("should create migration marker", func() {
				err := performFirstRunMigration(tempDir, log)
				Expect(err).NotTo(HaveOccurred())

				markerPath := filepath.Join(
					tempDir,
					internalconfig.GlobalConfigDir,
					MigrationMarkerFile,
				)
				Expect(markerPath).To(BeAnExistingFile())

				content, err := os.ReadFile(markerPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("v1"))
			})

			It("should backup global config if it exists", func() {
				// Create global config
				configDir := filepath.Join(tempDir, internalconfig.GlobalConfigDir)
				err := os.MkdirAll(configDir, 0o700)
				Expect(err).NotTo(HaveOccurred())

				globalConfigPath := filepath.Join(configDir, internalconfig.GlobalConfigFile)
				err = os.WriteFile(
					globalConfigPath,
					[]byte("[validators]\nenabled = true\n"),
					0o600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = performFirstRunMigration(tempDir, log)
				Expect(err).NotTo(HaveOccurred())

				// Verify backup was created
				backupDir := filepath.Join(
					tempDir,
					internalconfig.GlobalConfigDir,
					".backups",
					"global",
					"snapshots",
				)
				Expect(backupDir).To(BeADirectory())

				entries, err := os.ReadDir(backupDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(entries)).To(BeNumerically(">", 0))
			})

			It("should backup project config if it exists", func() {
				// Create project directory with config
				projectDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(projectDir, internalconfig.ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o700)
				Expect(err).NotTo(HaveOccurred())

				projectConfigPath := filepath.Join(configDir, internalconfig.ProjectConfigFile)
				err = os.WriteFile(
					projectConfigPath,
					[]byte("[validators]\nenabled = false\n"),
					0o600,
				)
				Expect(err).NotTo(HaveOccurred())

				// Change to project directory
				originalWd, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())
				defer os.Chdir(originalWd)

				err = os.Chdir(projectDir)
				Expect(err).NotTo(HaveOccurred())

				// Get the actual working directory (may be different from projectDir due to symlinks)
				actualWorkDir, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())

				// Create global config dir for marker
				globalConfigDir := filepath.Join(tempDir, internalconfig.GlobalConfigDir)
				err = os.MkdirAll(globalConfigDir, 0o700)
				Expect(err).NotTo(HaveOccurred())

				err = performFirstRunMigration(tempDir, log)
				Expect(err).NotTo(HaveOccurred())

				// Verify project backup was created using the actual resolved working directory
				sanitizedPath := backup.SanitizePath(actualWorkDir)
				backupDir := filepath.Join(
					tempDir,
					internalconfig.GlobalConfigDir,
					".backups",
					"projects",
					sanitizedPath,
					"snapshots",
				)
				Expect(backupDir).To(BeADirectory())

				entries, err := os.ReadDir(backupDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(entries)).To(BeNumerically(">", 0))
			})

			It("should not fail if no configs exist", func() {
				err := performFirstRunMigration(tempDir, log)
				Expect(err).NotTo(HaveOccurred())

				markerPath := filepath.Join(
					tempDir,
					internalconfig.GlobalConfigDir,
					MigrationMarkerFile,
				)
				Expect(markerPath).To(BeAnExistingFile())
			})
		})

		Context("when migration marker exists", func() {
			It("should skip migration", func() {
				// Create marker file
				configDir := filepath.Join(tempDir, internalconfig.GlobalConfigDir)
				err := os.MkdirAll(configDir, 0o700)
				Expect(err).NotTo(HaveOccurred())

				markerPath := filepath.Join(configDir, MigrationMarkerFile)
				err = os.WriteFile(markerPath, []byte("v1"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				// Create a global config that should not be backed up
				globalConfigPath := filepath.Join(configDir, internalconfig.GlobalConfigFile)
				err = os.WriteFile(
					globalConfigPath,
					[]byte("[validators]\nenabled = true\n"),
					0o600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = performFirstRunMigration(tempDir, log)
				Expect(err).NotTo(HaveOccurred())

				// Verify no backup was created
				backupDir := filepath.Join(tempDir, internalconfig.GlobalConfigDir, ".backups")
				_, err = os.Stat(backupDir)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})
	})

	Describe("backupConfigIfExists", func() {
		var configDir string

		BeforeEach(func() {
			configDir = filepath.Join(tempDir, internalconfig.GlobalConfigDir)
			err := os.MkdirAll(configDir, 0o700)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when config file exists", func() {
			It("should create backup", func() {
				testConfigPath := filepath.Join(configDir, "test-config.toml")
				err := os.WriteFile(testConfigPath, []byte("[test]\nvalue = 1\n"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				err = backupConfigIfExists(
					testConfigPath,
					backup.ConfigTypeGlobal,
					"",
					tempDir,
					log,
				)
				Expect(err).NotTo(HaveOccurred())

				// Verify backup was created
				backupDir := filepath.Join(
					tempDir,
					internalconfig.GlobalConfigDir,
					".backups",
					"global",
					"snapshots",
				)
				Expect(backupDir).To(BeADirectory())

				entries, err := os.ReadDir(backupDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(entries)).To(BeNumerically(">", 0))
			})

			It("should create backup with correct trigger", func() {
				testConfigPath := filepath.Join(configDir, "test-config.toml")
				err := os.WriteFile(testConfigPath, []byte("[test]\nvalue = 2\n"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				err = backupConfigIfExists(
					testConfigPath,
					backup.ConfigTypeGlobal,
					"",
					tempDir,
					log,
				)
				Expect(err).NotTo(HaveOccurred())

				// Load backup and verify trigger
				baseDir := filepath.Join(tempDir, internalconfig.GlobalConfigDir)
				storage, err := backup.NewFilesystemStorage(baseDir, backup.ConfigTypeGlobal, "")
				Expect(err).NotTo(HaveOccurred())

				index, err := storage.LoadIndex()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(index.Snapshots)).To(BeNumerically(">", 0))

				// Get first snapshot
				var snapshot backup.Snapshot
				for _, s := range index.Snapshots {
					snapshot = s
					break
				}
				Expect(snapshot.Trigger).To(Equal(backup.TriggerMigration))
				Expect(snapshot.Metadata.Command).To(Equal("first-run migration"))
			})
		})

		Context("when config file does not exist", func() {
			It("should not create backup and not return error", func() {
				nonExistentPath := filepath.Join(configDir, "nonexistent.toml")

				err := backupConfigIfExists(
					nonExistentPath,
					backup.ConfigTypeGlobal,
					"",
					tempDir,
					log,
				)
				Expect(err).NotTo(HaveOccurred())

				// Verify no backup was created
				backupDir := filepath.Join(tempDir, internalconfig.GlobalConfigDir, ".backups")
				_, err = os.Stat(backupDir)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("for project configs", func() {
			It("should create backup in project-specific directory", func() {
				projectDir := filepath.Join(tempDir, "myproject")
				projectConfigDir := filepath.Join(projectDir, internalconfig.ProjectConfigDir)
				err := os.MkdirAll(projectConfigDir, 0o700)
				Expect(err).NotTo(HaveOccurred())

				projectConfigPath := filepath.Join(
					projectConfigDir,
					internalconfig.ProjectConfigFile,
				)
				err = os.WriteFile(
					projectConfigPath,
					[]byte("[validators]\nenabled = false\n"),
					0o600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = backupConfigIfExists(
					projectConfigPath,
					backup.ConfigTypeProject,
					projectDir,
					tempDir,
					log,
				)
				Expect(err).NotTo(HaveOccurred())

				// Verify backup in project-specific directory
				sanitizedPath := backup.SanitizePath(projectDir)
				backupDir := filepath.Join(
					tempDir,
					internalconfig.GlobalConfigDir,
					".backups",
					"projects",
					sanitizedPath,
					"snapshots",
				)
				Expect(backupDir).To(BeADirectory())

				entries, err := os.ReadDir(backupDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(entries)).To(BeNumerically(">", 0))
			})
		})
	})
})
