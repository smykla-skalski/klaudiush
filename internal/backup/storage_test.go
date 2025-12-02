package backup_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/backup"
)

var _ = Describe("Storage", func() {
	Describe("SanitizePath", func() {
		It("sanitizes absolute paths", func() {
			result := backup.SanitizePath("/Users/bart/project")
			Expect(result).To(Equal("Users_bart_project"))
		})

		It("handles empty paths", func() {
			result := backup.SanitizePath("")
			Expect(result).To(Equal(""))
		})

		It("handles relative paths", func() {
			result := backup.SanitizePath("./foo/bar")
			Expect(result).To(ContainSubstring("foo_bar"))
		})

		It("replaces all separators", func() {
			result := backup.SanitizePath("/a/b/c/d")
			Expect(result).To(Equal("a_b_c_d"))
		})
	})

	Describe("FilesystemStorage", func() {
		var (
			tmpDir      string
			storage     *backup.FilesystemStorage
			projectPath string
		)

		BeforeEach(func() {
			var err error

			tmpDir, err = os.MkdirTemp("", "klaudiush-test-*")
			Expect(err).NotTo(HaveOccurred())

			projectPath = "/Users/test/project"
		})

		AfterEach(func() {
			if tmpDir != "" {
				os.RemoveAll(tmpDir)
			}
		})

		Describe("NewFilesystemStorage", func() {
			It("creates storage for global config", func() {
				var err error

				storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")

				Expect(err).NotTo(HaveOccurred())
				Expect(storage).NotTo(BeNil())
			})

			It("creates storage for project config", func() {
				var err error

				storage, err = backup.NewFilesystemStorage(
					tmpDir,
					backup.ConfigTypeProject,
					projectPath,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(storage).NotTo(BeNil())
			})

			It("returns error for empty base dir", func() {
				var err error

				_, err = backup.NewFilesystemStorage("", backup.ConfigTypeGlobal, "")

				Expect(err).To(MatchError(ContainSubstring("invalid path")))
			})

			It("returns error for invalid config type", func() {
				var err error

				_, err = backup.NewFilesystemStorage(tmpDir, "invalid", "")

				Expect(err).To(MatchError(ContainSubstring("invalid config type")))
			})

			It("returns error for project config without path", func() {
				var err error

				_, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeProject, "")

				Expect(err).To(MatchError(ContainSubstring("invalid path")))
			})
		})

		Describe("Initialize", func() {
			BeforeEach(func() {
				var err error

				storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates directory structure", func() {
				err := storage.Initialize()

				Expect(err).NotTo(HaveOccurred())
				Expect(storage.Exists()).To(BeTrue())
			})

			It("creates metadata file", func() {
				var err error

				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())

				index, err := storage.LoadIndex()

				Expect(err).NotTo(HaveOccurred())
				Expect(index).NotTo(BeNil())
				Expect(index.Version).To(Equal(1))
			})

			It("handles multiple initializations", func() {
				var err error

				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())

				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Exists", func() {
			BeforeEach(func() {
				var err error

				storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns false before initialization", func() {
				Expect(storage.Exists()).To(BeFalse())
			})

			It("returns true after initialization", func() {
				err := storage.Initialize()
				Expect(err).NotTo(HaveOccurred())

				Expect(storage.Exists()).To(BeTrue())
			})
		})

		Describe("Save and Load", func() {
			BeforeEach(func() {
				var err error

				storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")
				Expect(err).NotTo(HaveOccurred())
				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())
			})

			It("saves and loads data", func() {
				data := []byte("test data")

				storagePath, err := storage.Save("test-id", data)

				Expect(err).NotTo(HaveOccurred())
				Expect(storagePath).To(ContainSubstring("test-id"))

				loaded, err := storage.Load(storagePath)

				Expect(err).NotTo(HaveOccurred())
				Expect(loaded).To(Equal(data))
			})

			It("returns error when loading non-existent file", func() {
				_, err := storage.Load("/non/existent/path")

				Expect(err).To(MatchError(ContainSubstring("snapshot not found")))
			})

			It("returns error when saving without initialization", func() {
				uninitStorage, err := backup.NewFilesystemStorage(
					tmpDir+"/new",
					backup.ConfigTypeGlobal,
					"",
				)
				Expect(err).NotTo(HaveOccurred())

				_, err = uninitStorage.Save("test-id", []byte("data"))

				Expect(err).To(MatchError(ContainSubstring("storage not initialized")))
			})
		})

		Describe("Delete", func() {
			BeforeEach(func() {
				var err error

				storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")
				Expect(err).NotTo(HaveOccurred())
				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())
			})

			It("deletes existing file", func() {
				storagePath, err := storage.Save("test-id", []byte("data"))
				Expect(err).NotTo(HaveOccurred())

				err = storage.Delete(storagePath)

				Expect(err).NotTo(HaveOccurred())

				_, err = storage.Load(storagePath)
				Expect(err).To(HaveOccurred())
			})

			It("returns error for non-existent file", func() {
				err := storage.Delete("/non/existent/path")

				Expect(err).To(MatchError(ContainSubstring("snapshot not found")))
			})
		})

		Describe("List", func() {
			BeforeEach(func() {
				var err error

				storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")
				Expect(err).NotTo(HaveOccurred())
				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())
			})

			It("lists all snapshots", func() {
				var err error

				_, err = storage.Save("id-1", []byte("data1"))
				Expect(err).NotTo(HaveOccurred())

				_, err = storage.Save("id-2", []byte("data2"))
				Expect(err).NotTo(HaveOccurred())

				paths, err := storage.List()

				Expect(err).NotTo(HaveOccurred())
				Expect(paths).To(HaveLen(2))
			})

			It("returns empty list for no snapshots", func() {
				paths, err := storage.List()

				Expect(err).NotTo(HaveOccurred())
				Expect(paths).To(BeEmpty())
			})

			It("excludes metadata file", func() {
				var err error

				_, err = storage.Save("test-id", []byte("data"))
				Expect(err).NotTo(HaveOccurred())

				paths, err := storage.List()

				Expect(err).NotTo(HaveOccurred())

				for _, path := range paths {
					Expect(filepath.Base(path)).NotTo(Equal("metadata.json"))
				}
			})
		})

		Describe("SaveIndex and LoadIndex", func() {
			BeforeEach(func() {
				var err error

				storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")
				Expect(err).NotTo(HaveOccurred())
				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())
			})

			It("saves and loads index", func() {
				var err error

				index := backup.NewSnapshotIndex()
				snapshot := backup.Snapshot{ID: "test-id"}
				index.Add(snapshot)

				err = storage.SaveIndex(index)
				Expect(err).NotTo(HaveOccurred())

				loaded, err := storage.LoadIndex()

				Expect(err).NotTo(HaveOccurred())
				Expect(loaded.Snapshots).To(HaveLen(1))
				Expect(loaded.Snapshots["test-id"]).To(Equal(snapshot))
			})

			It("returns empty index for non-existent file", func() {
				uninitStorage, err := backup.NewFilesystemStorage(
					tmpDir+"/new",
					backup.ConfigTypeGlobal,
					"",
				)
				Expect(err).NotTo(HaveOccurred())

				index, err := uninitStorage.LoadIndex()

				Expect(err).NotTo(HaveOccurred())
				Expect(index.Snapshots).To(BeEmpty())
			})
		})

		Describe("Project storage", func() {
			BeforeEach(func() {
				var err error

				storage, err = backup.NewFilesystemStorage(
					tmpDir,
					backup.ConfigTypeProject,
					projectPath,
				)
				Expect(err).NotTo(HaveOccurred())
				err = storage.Initialize()
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates separate directory for project", func() {
				Expect(storage.Exists()).To(BeTrue())

				// Verify project-specific path is created
				sanitized := backup.SanitizePath(projectPath)
				projectDir := filepath.Join(tmpDir, ".backups", "projects", sanitized)

				_, err := os.Stat(projectDir)
				Expect(err).NotTo(HaveOccurred())
			})

			It("isolates project snapshots", func() {
				var err error

				_, err = storage.Save("project-snapshot", []byte("project data"))
				Expect(err).NotTo(HaveOccurred())

				// Create global storage
				globalStorage, err := backup.NewFilesystemStorage(
					tmpDir,
					backup.ConfigTypeGlobal,
					"",
				)
				Expect(err).NotTo(HaveOccurred())
				err = globalStorage.Initialize()
				Expect(err).NotTo(HaveOccurred())

				_, err = globalStorage.Save("global-snapshot", []byte("global data"))
				Expect(err).NotTo(HaveOccurred())

				// Verify isolation
				projectPaths, err := storage.List()
				Expect(err).NotTo(HaveOccurred())
				Expect(projectPaths).To(HaveLen(1))

				globalPaths, err := globalStorage.List()
				Expect(err).NotTo(HaveOccurred())
				Expect(globalPaths).To(HaveLen(1))
			})
		})
	})
})
