package exceptions_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/exceptions"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("AuditLogger", func() {
	var (
		auditLogger *exceptions.AuditLogger
		tempDir     string
		logFile     string
		currentTime time.Time
		timeFunc    func() time.Time
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "audit-test-*")
		Expect(err).NotTo(HaveOccurred())

		logFile = filepath.Join(tempDir, "audit.jsonl")
		currentTime = time.Date(2025, 11, 29, 10, 30, 0, 0, time.UTC)
		timeFunc = func() time.Time { return currentTime }
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("NewAuditLogger", func() {
		It("creates logger with nil config", func() {
			l := exceptions.NewAuditLogger(nil)
			Expect(l).NotTo(BeNil())
		})

		It("creates logger with config", func() {
			l := exceptions.NewAuditLogger(
				&config.ExceptionAuditConfig{
					LogFile: logFile,
				},
			)
			Expect(l).NotTo(BeNil())
		})

		It("accepts custom log file", func() {
			l := exceptions.NewAuditLogger(
				nil,
				exceptions.WithAuditFile(logFile),
			)
			Expect(l).NotTo(BeNil())
			Expect(l.GetLogPath()).To(Equal(logFile))
		})

		It("accepts custom time function", func() {
			customTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			l := exceptions.NewAuditLogger(
				nil,
				exceptions.WithAuditTimeFunc(func() time.Time { return customTime }),
			)
			Expect(l).NotTo(BeNil())
		})
	})

	Describe("Log", func() {
		Context("when audit logging is disabled", func() {
			BeforeEach(func() {
				enabled := false
				auditLogger = exceptions.NewAuditLogger(
					&config.ExceptionAuditConfig{
						Enabled: &enabled,
					},
					exceptions.WithAuditFile(logFile),
					exceptions.WithAuditTimeFunc(timeFunc),
				)
			})

			It("does not write to file", func() {
				entry := &exceptions.AuditEntry{
					Timestamp:     currentTime,
					ErrorCode:     "GIT022",
					ValidatorName: "git.commit",
					Allowed:       true,
					Reason:        "test reason",
					Source:        "comment",
				}

				err := auditLogger.Log(entry)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(logFile)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("when audit logging is enabled", func() {
			BeforeEach(func() {
				auditLogger = exceptions.NewAuditLogger(
					nil,
					exceptions.WithAuditFile(logFile),
					exceptions.WithAuditTimeFunc(timeFunc),
				)
			})

			It("writes entry to file", func() {
				entry := &exceptions.AuditEntry{
					Timestamp:     currentTime,
					ErrorCode:     "GIT022",
					ValidatorName: "git.commit",
					Allowed:       true,
					Reason:        "Emergency hotfix",
					Source:        "comment",
					Command:       "git commit -m 'fix'",
				}

				err := auditLogger.Log(entry)
				Expect(err).NotTo(HaveOccurred())

				// Read file and verify
				data, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())

				var logged exceptions.AuditEntry
				err = json.Unmarshal(data, &logged)
				Expect(err).NotTo(HaveOccurred())
				Expect(logged.ErrorCode).To(Equal("GIT022"))
				Expect(logged.ValidatorName).To(Equal("git.commit"))
				Expect(logged.Allowed).To(BeTrue())
				Expect(logged.Reason).To(Equal("Emergency hotfix"))
			})

			It("appends multiple entries", func() {
				entry1 := &exceptions.AuditEntry{
					Timestamp: currentTime,
					ErrorCode: "GIT022",
					Allowed:   true,
					Source:    "comment",
				}

				entry2 := &exceptions.AuditEntry{
					Timestamp: currentTime.Add(time.Minute),
					ErrorCode: "SEC001",
					Allowed:   false,
					Source:    "env_var",
				}

				err := auditLogger.Log(entry1)
				Expect(err).NotTo(HaveOccurred())

				err = auditLogger.Log(entry2)
				Expect(err).NotTo(HaveOccurred())

				// Read all entries
				entries, err := auditLogger.Read()
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(2))
				Expect(entries[0].ErrorCode).To(Equal("GIT022"))
				Expect(entries[1].ErrorCode).To(Equal("SEC001"))
			})

			It("handles nil entry gracefully", func() {
				err := auditLogger.Log(nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates directory if not exists", func() {
				nestedPath := filepath.Join(tempDir, "nested", "dir", "audit.jsonl")
				auditLogger = exceptions.NewAuditLogger(
					nil,
					exceptions.WithAuditFile(nestedPath),
				)

				entry := &exceptions.AuditEntry{
					Timestamp: currentTime,
					ErrorCode: "GIT022",
					Allowed:   true,
					Source:    "comment",
				}

				err := auditLogger.Log(entry)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(nestedPath)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("Read", func() {
		BeforeEach(func() {
			auditLogger = exceptions.NewAuditLogger(
				nil,
				exceptions.WithAuditFile(logFile),
				exceptions.WithAuditTimeFunc(timeFunc),
			)
		})

		It("returns empty slice for non-existent file", func() {
			entries, err := auditLogger.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})

		It("reads entries from file", func() {
			// Write entries directly to file
			entry1 := exceptions.AuditEntry{
				Timestamp: currentTime,
				ErrorCode: "GIT022",
				Allowed:   true,
				Source:    "comment",
			}

			entry2 := exceptions.AuditEntry{
				Timestamp: currentTime,
				ErrorCode: "SEC001",
				Allowed:   false,
				Source:    "env_var",
			}

			data1, _ := json.Marshal(entry1)
			data2, _ := json.Marshal(entry2)

			content := string(data1) + "\n" + string(data2) + "\n"
			err := os.WriteFile(logFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())

			entries, err := auditLogger.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(2))
		})

		It("skips empty lines", func() {
			entry := exceptions.AuditEntry{
				Timestamp: currentTime,
				ErrorCode: "GIT022",
				Allowed:   true,
				Source:    "comment",
			}

			data, _ := json.Marshal(entry)
			content := "\n" + string(data) + "\n\n"
			err := os.WriteFile(logFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())

			entries, err := auditLogger.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
		})

		It("skips malformed entries", func() {
			entry := exceptions.AuditEntry{
				Timestamp: currentTime,
				ErrorCode: "GIT022",
				Allowed:   true,
				Source:    "comment",
			}

			data, _ := json.Marshal(entry)
			content := "not json\n" + string(data) + "\n{broken\n"
			err := os.WriteFile(logFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())

			entries, err := auditLogger.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].ErrorCode).To(Equal("GIT022"))
		})
	})

	Describe("Rotate", func() {
		BeforeEach(func() {
			auditLogger = exceptions.NewAuditLogger(
				nil,
				exceptions.WithAuditFile(logFile),
				exceptions.WithAuditTimeFunc(timeFunc),
			)
		})

		It("does nothing for non-existent file", func() {
			err := auditLogger.Rotate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates backup file with timestamp", func() {
			// Create initial file
			err := os.WriteFile(logFile, []byte(`{"error_code":"GIT022"}`+"\n"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = auditLogger.Rotate()
			Expect(err).NotTo(HaveOccurred())

			// Original file should not exist
			_, err = os.Stat(logFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			// Backup file should exist with timestamp
			expectedBackup := filepath.Join(tempDir, "audit.20251129-103000.jsonl")
			_, err = os.Stat(expectedBackup)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("automatic rotation", func() {
		It("rotates when max size exceeded", func() {
			maxSize := 1 // 1 MB
			auditLogger = exceptions.NewAuditLogger(
				&config.ExceptionAuditConfig{
					MaxSizeMB: &maxSize,
				},
				exceptions.WithAuditFile(logFile),
				exceptions.WithAuditTimeFunc(timeFunc),
			)

			// Create a file larger than 1MB
			largeContent := strings.Repeat("x", 1024*1024+100)
			err := os.WriteFile(logFile, []byte(largeContent), 0o600)
			Expect(err).NotTo(HaveOccurred())

			// Log an entry should trigger rotation
			entry := &exceptions.AuditEntry{
				Timestamp: currentTime,
				ErrorCode: "GIT022",
				Allowed:   true,
				Source:    "comment",
			}

			err = auditLogger.Log(entry)
			Expect(err).NotTo(HaveOccurred())

			// Backup should exist
			expectedBackup := filepath.Join(tempDir, "audit.20251129-103000.jsonl")
			_, err = os.Stat(expectedBackup)
			Expect(err).NotTo(HaveOccurred())

			// New log file should have just the new entry
			entries, err := auditLogger.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
		})
	})

	Describe("Cleanup", func() {
		Context("backup cleanup", func() {
			BeforeEach(func() {
				maxBackups := 2
				auditLogger = exceptions.NewAuditLogger(
					&config.ExceptionAuditConfig{
						MaxBackups: &maxBackups,
					},
					exceptions.WithAuditFile(logFile),
					exceptions.WithAuditTimeFunc(timeFunc),
				)
			})

			It("removes excess backup files", func() {
				// Create backup files
				backup1 := filepath.Join(tempDir, "audit.20251129-100000.jsonl")
				backup2 := filepath.Join(tempDir, "audit.20251129-110000.jsonl")
				backup3 := filepath.Join(tempDir, "audit.20251129-120000.jsonl")

				_ = os.WriteFile(backup1, []byte("old"), 0o600)
				_ = os.WriteFile(backup2, []byte("mid"), 0o600)
				_ = os.WriteFile(backup3, []byte("new"), 0o600)
				_ = os.WriteFile(logFile, []byte("current"), 0o600)

				err := auditLogger.Cleanup()
				Expect(err).NotTo(HaveOccurred())

				// Oldest backup should be removed
				_, err = os.Stat(backup1)
				Expect(os.IsNotExist(err)).To(BeTrue())

				// Newer backups should remain
				_, err = os.Stat(backup2)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(backup3)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("old entry cleanup", func() {
			BeforeEach(func() {
				maxAge := 7 // 7 days
				auditLogger = exceptions.NewAuditLogger(
					&config.ExceptionAuditConfig{
						MaxAgeDays: &maxAge,
					},
					exceptions.WithAuditFile(logFile),
					exceptions.WithAuditTimeFunc(timeFunc),
				)
			})

			It("removes entries older than max age", func() {
				// Create entries with different ages
				oldEntry := exceptions.AuditEntry{
					Timestamp: currentTime.Add(-10 * 24 * time.Hour), // 10 days old
					ErrorCode: "OLD001",
					Allowed:   true,
					Source:    "comment",
				}

				newEntry := exceptions.AuditEntry{
					Timestamp: currentTime.Add(-3 * 24 * time.Hour), // 3 days old
					ErrorCode: "NEW001",
					Allowed:   true,
					Source:    "comment",
				}

				data1, _ := json.Marshal(oldEntry)
				data2, _ := json.Marshal(newEntry)

				content := string(data1) + "\n" + string(data2) + "\n"
				err := os.WriteFile(logFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				err = auditLogger.Cleanup()
				Expect(err).NotTo(HaveOccurred())

				// Only new entry should remain
				entries, err := auditLogger.Read()
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].ErrorCode).To(Equal("NEW001"))
			})

			It("keeps malformed entries to avoid data loss", func() {
				newEntry := exceptions.AuditEntry{
					Timestamp: currentTime.Add(-3 * 24 * time.Hour),
					ErrorCode: "NEW001",
					Allowed:   true,
					Source:    "comment",
				}

				data, _ := json.Marshal(newEntry)

				content := "malformed json\n" + string(data) + "\n"
				err := os.WriteFile(logFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				err = auditLogger.Cleanup()
				Expect(err).NotTo(HaveOccurred())

				// Read raw file to check malformed entry is kept
				rawData, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(rawData)).To(ContainSubstring("malformed json"))
			})
		})
	})

	Describe("Stats", func() {
		BeforeEach(func() {
			auditLogger = exceptions.NewAuditLogger(
				nil,
				exceptions.WithAuditFile(logFile),
				exceptions.WithAuditTimeFunc(timeFunc),
			)
		})

		It("returns empty stats for non-existent file", func() {
			stats, err := auditLogger.Stats()
			Expect(err).NotTo(HaveOccurred())
			Expect(stats.LogFile).To(Equal(logFile))
			Expect(stats.EntryCount).To(Equal(0))
			Expect(stats.SizeBytes).To(Equal(int64(0)))
		})

		It("counts entries correctly", func() {
			entry1 := exceptions.AuditEntry{
				Timestamp: currentTime,
				ErrorCode: "GIT022",
				Allowed:   true,
				Source:    "comment",
			}

			entry2 := exceptions.AuditEntry{
				Timestamp: currentTime,
				ErrorCode: "SEC001",
				Allowed:   false,
				Source:    "env_var",
			}

			data1, _ := json.Marshal(entry1)
			data2, _ := json.Marshal(entry2)

			content := string(data1) + "\n" + string(data2) + "\n"
			err := os.WriteFile(logFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())

			stats, err := auditLogger.Stats()
			Expect(err).NotTo(HaveOccurred())
			Expect(stats.EntryCount).To(Equal(2))
			Expect(stats.SizeBytes).To(BeNumerically(">", 0))
		})

		It("counts backup files", func() {
			_ = os.WriteFile(logFile, []byte("current"), 0o600)

			backup1 := filepath.Join(tempDir, "audit.20251129-100000.jsonl")
			_ = os.WriteFile(backup1, []byte("backup1"), 0o600)

			backup2 := filepath.Join(tempDir, "audit.20251129-110000.jsonl")
			_ = os.WriteFile(backup2, []byte("backup2"), 0o600)

			stats, err := auditLogger.Stats()
			Expect(err).NotTo(HaveOccurred())
			Expect(stats.BackupCount).To(Equal(2))
		})

		It("formats size correctly", func() {
			stats := &exceptions.AuditStats{
				SizeBytes: 5 * 1024 * 1024, // 5 MB
			}
			Expect(stats.FormatSize()).To(Equal("5.00 MB"))
		})
	})

	Describe("concurrent access", func() {
		BeforeEach(func() {
			auditLogger = exceptions.NewAuditLogger(
				nil,
				exceptions.WithAuditFile(logFile),
				exceptions.WithAuditTimeFunc(timeFunc),
			)
		})

		It("handles concurrent writes safely", func() {
			const numGoroutines = 10
			const entriesPerGoroutine = 10

			done := make(chan bool, numGoroutines)

			for i := range numGoroutines {
				go func(id int) {
					for j := range entriesPerGoroutine {
						errorCode := fmt.Sprintf("GIT%03d", id)

						entry := &exceptions.AuditEntry{
							Timestamp: currentTime,
							ErrorCode: errorCode,
							Allowed:   true,
							Source:    "comment",
							Command:   fmt.Sprintf("cmd-%d", j),
						}

						_ = auditLogger.Log(entry)
					}

					done <- true
				}(i)
			}

			// Wait for all goroutines
			for range numGoroutines {
				<-done
			}

			// All entries should be written
			entries, err := auditLogger.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(numGoroutines * entriesPerGoroutine))
		})
	})

	Describe("path resolution", func() {
		It("expands tilde in path", func() {
			auditLogger = exceptions.NewAuditLogger(
				nil,
				exceptions.WithAuditFile("~/test-audit.jsonl"),
			)

			path := auditLogger.GetLogPath()
			Expect(path).NotTo(HavePrefix("~"))
			Expect(path).To(ContainSubstring("test-audit.jsonl"))
		})
	})
})
