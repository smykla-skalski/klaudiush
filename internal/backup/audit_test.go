package backup_test

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/backup"
)

var _ = Describe("Audit", func() {
	var (
		tempDir string
		logger  *backup.JSONLAuditLogger
	)

	BeforeEach(func() {
		var err error

		tempDir, err = os.MkdirTemp("", "klaudiush-audit-test-*")
		Expect(err).NotTo(HaveOccurred())

		logger, err = backup.NewJSONLAuditLogger(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if logger != nil {
			_ = logger.Close()
		}

		_ = os.RemoveAll(tempDir)
	})

	Describe("NewJSONLAuditLogger", func() {
		It("should create a new audit logger", func() {
			Expect(logger).NotTo(BeNil())
		})

		It("should fail with empty baseDir", func() {
			_, err := backup.NewJSONLAuditLogger("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("baseDir cannot be empty"))
		})
	})

	Describe("Log", func() {
		It("should log a successful create operation", func() {
			entry := backup.AuditEntry{
				Timestamp:  time.Now(),
				Operation:  backup.OperationCreate,
				ConfigPath: "/path/to/config.toml",
				SnapshotID: "abc123",
				User:       "testuser",
				Hostname:   "testhost",
				Success:    true,
			}

			err := logger.Log(entry)
			Expect(err).NotTo(HaveOccurred())

			// Verify file was created
			logPath := filepath.Join(tempDir, backup.AuditLogFile)
			Expect(logPath).To(BeAnExistingFile())
		})

		It("should log a failed restore operation", func() {
			entry := backup.AuditEntry{
				Timestamp:  time.Now(),
				Operation:  backup.OperationRestore,
				ConfigPath: "/path/to/config.toml",
				SnapshotID: "def456",
				User:       "testuser",
				Hostname:   "testhost",
				Success:    false,
				Error:      "snapshot not found",
			}

			err := logger.Log(entry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should log operation with extra data", func() {
			entry := backup.AuditEntry{
				Timestamp: time.Now(),
				Operation: backup.OperationPrune,
				User:      "testuser",
				Success:   true,
				Extra: map[string]any{
					"snapshots_removed": 5,
					"bytes_freed":       int64(1024000),
				},
			}

			err := logger.Log(entry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create directory if it doesn't exist", func() {
			nestedDir := filepath.Join(tempDir, "nested", "dir")
			nestedLogger, err := backup.NewJSONLAuditLogger(nestedDir)
			Expect(err).NotTo(HaveOccurred())
			defer nestedLogger.Close()

			entry := backup.AuditEntry{
				Timestamp: time.Now(),
				Operation: backup.OperationCreate,
				Success:   true,
			}

			err = nestedLogger.Log(entry)
			Expect(err).NotTo(HaveOccurred())

			// Verify directory was created
			Expect(nestedDir).To(BeADirectory())
		})
	})

	Describe("Query", func() {
		BeforeEach(func() {
			// Populate log with test entries
			entries := []backup.AuditEntry{
				{
					Timestamp:  time.Now().Add(-5 * time.Hour),
					Operation:  backup.OperationCreate,
					SnapshotID: "snap1",
					Success:    true,
				},
				{
					Timestamp:  time.Now().Add(-4 * time.Hour),
					Operation:  backup.OperationRestore,
					SnapshotID: "snap1",
					Success:    true,
				},
				{
					Timestamp:  time.Now().Add(-3 * time.Hour),
					Operation:  backup.OperationCreate,
					SnapshotID: "snap2",
					Success:    false,
					Error:      "disk full",
				},
				{
					Timestamp: time.Now().Add(-2 * time.Hour),
					Operation: backup.OperationPrune,
					Success:   true,
					Extra: map[string]any{
						"snapshots_removed": 3,
					},
				},
				{
					Timestamp:  time.Now().Add(-1 * time.Hour),
					Operation:  backup.OperationDelete,
					SnapshotID: "snap3",
					Success:    true,
				},
			}

			for _, entry := range entries {
				err := logger.Log(entry)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should query all entries", func() {
			entries, err := logger.Query(backup.AuditFilter{})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(5))
		})

		It("should filter by operation", func() {
			entries, err := logger.Query(backup.AuditFilter{
				Operation: backup.OperationCreate,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(2))

			for _, entry := range entries {
				Expect(entry.Operation).To(Equal(backup.OperationCreate))
			}
		})

		It("should filter by time", func() {
			since := time.Now().Add(-3*time.Hour - 30*time.Minute)
			entries, err := logger.Query(backup.AuditFilter{
				Since: since,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(3))

			for _, entry := range entries {
				Expect(entry.Timestamp.After(since)).To(BeTrue())
			}
		})

		It("should filter by snapshot ID", func() {
			entries, err := logger.Query(backup.AuditFilter{
				SnapshotID: "snap1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(2))

			for _, entry := range entries {
				Expect(entry.SnapshotID).To(Equal("snap1"))
			}
		})

		It("should filter by success", func() {
			success := true
			entries, err := logger.Query(backup.AuditFilter{
				Success: &success,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(4))

			for _, entry := range entries {
				Expect(entry.Success).To(BeTrue())
			}
		})

		It("should filter by failure", func() {
			success := false
			entries, err := logger.Query(backup.AuditFilter{
				Success: &success,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Error).To(Equal("disk full"))
		})

		It("should apply multiple filters", func() {
			success := true
			entries, err := logger.Query(backup.AuditFilter{
				Operation: backup.OperationCreate,
				Success:   &success,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].SnapshotID).To(Equal("snap1"))
		})

		It("should limit results", func() {
			entries, err := logger.Query(backup.AuditFilter{
				Limit: 2,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(2))
		})

		It("should return empty list if log doesn't exist", func() {
			emptyLogger, err := backup.NewJSONLAuditLogger(
				filepath.Join(tempDir, "nonexistent"),
			)
			Expect(err).NotTo(HaveOccurred())
			defer emptyLogger.Close()

			entries, err := emptyLogger.Query(backup.AuditFilter{})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})

		It("should skip invalid JSON entries", func() {
			// Write invalid JSON to log file
			logPath := filepath.Join(tempDir, backup.AuditLogFile)
			file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = file.WriteString("invalid json\n")
			Expect(err).NotTo(HaveOccurred())
			file.Close()

			// Query should still succeed and return valid entries
			entries, err := logger.Query(backup.AuditFilter{})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(5))
		})
	})

	Describe("Concurrent operations", func() {
		It("should handle concurrent writes", func() {
			const numGoroutines = 10
			const entriesPerGoroutine = 10

			var wg sync.WaitGroup

			wg.Add(numGoroutines)

			for i := range numGoroutines {
				go func(id int) {
					defer GinkgoRecover()
					defer wg.Done()

					for j := range entriesPerGoroutine {
						entry := backup.AuditEntry{
							Timestamp:  time.Now(),
							Operation:  backup.OperationCreate,
							SnapshotID: "concurrent",
							Success:    true,
							Extra: map[string]any{
								"goroutine": id,
								"iteration": j,
							},
						}

						err := logger.Log(entry)
						Expect(err).NotTo(HaveOccurred())
					}
				}(i)
			}

			wg.Wait()

			// Verify all entries were written
			entries, err := logger.Query(backup.AuditFilter{
				SnapshotID: "concurrent",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(numGoroutines * entriesPerGoroutine))
		})

		It("should handle concurrent reads", func() {
			// Write some entries first
			for range 10 {
				entry := backup.AuditEntry{
					Timestamp: time.Now(),
					Operation: backup.OperationCreate,
					Success:   true,
				}

				err := logger.Log(entry)
				Expect(err).NotTo(HaveOccurred())
			}

			const numGoroutines = 10

			var wg sync.WaitGroup

			wg.Add(numGoroutines)

			for range numGoroutines {
				go func() {
					defer GinkgoRecover()
					defer wg.Done()

					entries, err := logger.Query(backup.AuditFilter{})
					Expect(err).NotTo(HaveOccurred())
					Expect(entries).To(HaveLen(10))
				}()
			}

			wg.Wait()
		})
	})

	Describe("Close", func() {
		It("should close without error", func() {
			err := logger.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
