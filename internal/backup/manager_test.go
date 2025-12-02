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

var _ = Describe("Manager", func() {
	var (
		tmpDir      string
		storage     *backup.FilesystemStorage
		cfg         *config.BackupConfig
		configPath  string
		manager     *backup.Manager
		enabled     bool
		autoBackup  bool
		asyncBackup bool
	)

	BeforeEach(func() {
		var err error

		tmpDir, err = os.MkdirTemp("", "klaudiush-test-*")
		Expect(err).NotTo(HaveOccurred())

		storage, err = backup.NewFilesystemStorage(tmpDir, backup.ConfigTypeGlobal, "")
		Expect(err).NotTo(HaveOccurred())

		enabled = true
		autoBackup = true
		asyncBackup = true

		cfg = &config.BackupConfig{
			Enabled:     &enabled,
			AutoBackup:  &autoBackup,
			AsyncBackup: &asyncBackup,
		}

		manager, err = backup.NewManager(storage, cfg)
		Expect(err).NotTo(HaveOccurred())

		// Create temporary config file
		configPath = filepath.Join(tmpDir, "config.toml")
		err = os.WriteFile(configPath, []byte("test = true"), 0o600)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	})

	Describe("NewManager", func() {
		It("creates manager with config", func() {
			mgr, err := backup.NewManager(storage, cfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(mgr).NotTo(BeNil())
		})

		It("creates manager with nil config", func() {
			mgr, err := backup.NewManager(storage, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(mgr).NotTo(BeNil())
		})

		It("returns error for nil storage", func() {
			_, err := backup.NewManager(nil, cfg)

			Expect(err).To(MatchError(ContainSubstring("storage cannot be nil")))
		})
	})

	Describe("CreateBackup", func() {
		It("creates full snapshot", func() {
			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
				Metadata: backup.SnapshotMetadata{
					User:        "test",
					Hostname:    "localhost",
					Command:     "test",
					Tag:         "test-tag",
					Description: "test description",
				},
			}

			snapshot, err := manager.CreateBackup(opts)

			Expect(err).NotTo(HaveOccurred())
			Expect(snapshot).NotTo(BeNil())
			Expect(snapshot.ID).NotTo(BeEmpty())
			Expect(snapshot.SequenceNum).To(Equal(1))
			Expect(snapshot.ConfigPath).To(Equal(configPath))
			Expect(snapshot.Trigger).To(Equal(backup.TriggerManual))
			Expect(snapshot.StorageType).To(Equal(backup.StorageTypeFull))
			Expect(snapshot.ChainID).To(Equal("chain-1"))
			Expect(snapshot.Metadata.Tag).To(Equal("test-tag"))
			Expect(snapshot.Metadata.Description).To(Equal("test description"))
			Expect(snapshot.Metadata.ConfigHash).NotTo(BeEmpty())
		})

		It("automatically initializes storage", func() {
			Expect(storage.Exists()).To(BeFalse())

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerAutomatic,
			}

			_, err := manager.CreateBackup(opts)

			Expect(err).NotTo(HaveOccurred())
			Expect(storage.Exists()).To(BeTrue())
		})

		It("returns error when backup is disabled", func() {
			disabled := false
			cfg.Enabled = &disabled

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			_, err := manager.CreateBackup(opts)

			Expect(err).To(MatchError(backup.ErrBackupDisabled))
		})

		It("returns error for non-existent config file", func() {
			opts := backup.CreateBackupOptions{
				ConfigPath: "/non/existent/config.toml",
				Trigger:    backup.TriggerManual,
			}

			_, err := manager.CreateBackup(opts)

			Expect(err).To(MatchError(ContainSubstring("config file not found")))
		})

		It("uses separate chains for full snapshots", func() {
			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create first backup
			snapshot1, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(snapshot1.SequenceNum).To(Equal(1))
			Expect(snapshot1.ChainID).To(Equal("chain-1"))

			// Update config to create different content
			err = os.WriteFile(configPath, []byte("test = false"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			// Create second backup
			snapshot2, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(snapshot2.SequenceNum).To(Equal(1)) // New chain, so sequence is 1
			Expect(snapshot2.ChainID).To(Equal("chain-2"))
		})

		Describe("Deduplication", func() {
			It("returns existing snapshot for identical content", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
				}

				// Create first backup
				snapshot1, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Try to create second backup with same content
				snapshot2, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Should return same snapshot
				Expect(snapshot2.ID).To(Equal(snapshot1.ID))
				Expect(snapshot2.Metadata.ConfigHash).To(Equal(snapshot1.Metadata.ConfigHash))
			})

			It("creates new snapshot for different content", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
				}

				// Create first backup
				snapshot1, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Update config
				err = os.WriteFile(configPath, []byte("test = false"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				// Create second backup
				snapshot2, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Should be different snapshots
				Expect(snapshot2.ID).NotTo(Equal(snapshot1.ID))
				Expect(snapshot2.Metadata.ConfigHash).NotTo(Equal(snapshot1.Metadata.ConfigHash))
			})

			It("deduplicates across multiple backups", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
				}

				// Create first backup
				snapshot1, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Create second backup with different content
				err = os.WriteFile(configPath, []byte("test = false"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				snapshot2, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Revert to original content
				err = os.WriteFile(configPath, []byte("test = true"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				// Create third backup
				snapshot3, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Should match first snapshot
				Expect(snapshot3.ID).To(Equal(snapshot1.ID))
				Expect(snapshot3.Metadata.ConfigHash).To(Equal(snapshot1.Metadata.ConfigHash))
				Expect(snapshot2.ID).NotTo(Equal(snapshot1.ID))
			})
		})

		Describe("Triggers", func() {
			It("records manual trigger", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
				}

				snapshot, err := manager.CreateBackup(opts)

				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.Trigger).To(Equal(backup.TriggerManual))
			})

			It("records automatic trigger", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerAutomatic,
				}

				snapshot, err := manager.CreateBackup(opts)

				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.Trigger).To(Equal(backup.TriggerAutomatic))
			})

			It("records before_init trigger", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerBeforeInit,
				}

				snapshot, err := manager.CreateBackup(opts)

				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.Trigger).To(Equal(backup.TriggerBeforeInit))
			})

			It("records migration trigger", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerMigration,
				}

				snapshot, err := manager.CreateBackup(opts)

				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.Trigger).To(Equal(backup.TriggerMigration))
			})
		})

		Describe("Config type detection", func() {
			It("detects global config", func() {
				var err error

				globalPath := filepath.Join(tmpDir, ".klaudiush-global", "config.toml")
				err = os.MkdirAll(filepath.Dir(globalPath), 0o700)
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(globalPath, []byte("test = true"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				opts := backup.CreateBackupOptions{
					ConfigPath: globalPath,
					Trigger:    backup.TriggerManual,
				}

				snapshot, err := manager.CreateBackup(opts)

				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.ConfigType).To(Equal(backup.ConfigTypeGlobal))
			})

			It("detects project config", func() {
				var err error

				projectPath := filepath.Join(tmpDir, ".klaudiush", "config.toml")
				err = os.MkdirAll(filepath.Dir(projectPath), 0o700)
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(projectPath, []byte("test = true"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				opts := backup.CreateBackupOptions{
					ConfigPath: projectPath,
					Trigger:    backup.TriggerManual,
				}

				snapshot, err := manager.CreateBackup(opts)

				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.ConfigType).To(Equal(backup.ConfigTypeProject))
			})
		})
	})

	Describe("List", func() {
		BeforeEach(func() {
			err := storage.Initialize()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns all snapshots", func() {
			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create two backups
			_, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = false"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// List all
			snapshots, err := manager.List()

			Expect(err).NotTo(HaveOccurred())
			Expect(snapshots).To(HaveLen(2))
		})

		It("returns empty list for no snapshots", func() {
			snapshots, err := manager.List()

			Expect(err).NotTo(HaveOccurred())
			Expect(snapshots).To(BeEmpty())
		})

		It("returns error when backup is disabled", func() {
			disabled := false
			cfg.Enabled = &disabled

			_, err := manager.List()

			Expect(err).To(MatchError(backup.ErrBackupDisabled))
		})

		It("returns empty list when storage not initialized", func() {
			newStorage, err := backup.NewFilesystemStorage(
				tmpDir+"/new",
				backup.ConfigTypeGlobal,
				"",
			)
			Expect(err).NotTo(HaveOccurred())

			newManager, err := backup.NewManager(newStorage, cfg)
			Expect(err).NotTo(HaveOccurred())

			snapshots, err := newManager.List()

			Expect(err).NotTo(HaveOccurred())
			Expect(snapshots).To(BeEmpty())
		})
	})

	Describe("Get", func() {
		var snapshotID string

		BeforeEach(func() {
			var err error

			err = storage.Initialize()
			Expect(err).NotTo(HaveOccurred())

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			snapshot, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())
			snapshotID = snapshot.ID
		})

		It("retrieves existing snapshot", func() {
			snapshot, err := manager.Get(snapshotID)

			Expect(err).NotTo(HaveOccurred())
			Expect(snapshot).NotTo(BeNil())
			Expect(snapshot.ID).To(Equal(snapshotID))
		})

		It("returns error for non-existent snapshot", func() {
			_, err := manager.Get("non-existent")

			Expect(err).To(MatchError(ContainSubstring("snapshot not found")))
		})

		It("returns error when backup is disabled", func() {
			disabled := false
			cfg.Enabled = &disabled

			_, err := manager.Get(snapshotID)

			Expect(err).To(MatchError(backup.ErrBackupDisabled))
		})

		It("returns error when storage not initialized", func() {
			newStorage, err := backup.NewFilesystemStorage(
				tmpDir+"/new",
				backup.ConfigTypeGlobal,
				"",
			)
			Expect(err).NotTo(HaveOccurred())

			newManager, err := backup.NewManager(newStorage, cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = newManager.Get("any-id")

			Expect(err).To(MatchError(backup.ErrSnapshotNotFound))
		})
	})

	Describe("Timestamps", func() {
		It("records accurate timestamps", func() {
			before := time.Now()

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			snapshot, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			after := time.Now()

			Expect(snapshot.Timestamp).To(BeTemporally(">=", before))
			Expect(snapshot.Timestamp).To(BeTemporally("<=", after))
		})
	})

	Describe("ApplyRetention", func() {
		var countPolicy *backup.CountRetentionPolicy

		BeforeEach(func() {
			var err error

			err = storage.Initialize()
			Expect(err).NotTo(HaveOccurred())

			countPolicy, err = backup.NewCountRetentionPolicy(2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error for nil policy", func() {
			_, err := manager.ApplyRetention(nil)
			Expect(err).To(MatchError(ContainSubstring("policy cannot be nil")))
		})

		It("returns error when backup is disabled", func() {
			disabled := false
			cfg.Enabled = &disabled

			_, err := manager.ApplyRetention(countPolicy)
			Expect(err).To(MatchError(backup.ErrBackupDisabled))
		})

		It("returns empty result when storage not initialized", func() {
			newStorage, err := backup.NewFilesystemStorage(
				tmpDir+"/new",
				backup.ConfigTypeGlobal,
				"",
			)
			Expect(err).NotTo(HaveOccurred())

			newManager, err := backup.NewManager(newStorage, cfg)
			Expect(err).NotTo(HaveOccurred())

			result, err := newManager.ApplyRetention(countPolicy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SnapshotsRemoved).To(Equal(0))
		})

		It("returns empty result when no snapshots exist", func() {
			result, err := manager.ApplyRetention(countPolicy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SnapshotsRemoved).To(Equal(0))
			Expect(result.ChainsRemoved).To(Equal(0))
			Expect(result.BytesFreed).To(Equal(int64(0)))
		})

		It("removes snapshots that violate retention policy", func() {
			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create 3 snapshots with different content
			snap1, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = false"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			snap2, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = nil"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			snap3, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Apply retention (keep only 2)
			result, err := manager.ApplyRetention(countPolicy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SnapshotsRemoved).To(Equal(1))
			Expect(result.BytesFreed).To(BeNumerically(">", 0))

			// Verify oldest snapshot was removed
			_, err = manager.Get(snap1.ID)
			Expect(err).To(HaveOccurred())

			// Verify newer snapshots still exist
			_, err = manager.Get(snap2.ID)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.Get(snap3.ID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes multiple snapshots when needed", func() {
			policy, err := backup.NewCountRetentionPolicy(1)
			Expect(err).NotTo(HaveOccurred())

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create 3 snapshots
			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = false"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = nil"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			snap3, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Apply retention (keep only 1)
			result, err := manager.ApplyRetention(policy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SnapshotsRemoved).To(Equal(2))

			// Only newest should remain
			snapshots, err := manager.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(snapshots).To(HaveLen(1))
			Expect(snapshots[0].ID).To(Equal(snap3.ID))
		})

		It("calculates bytes freed correctly", func() {
			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create snapshot
			snap1, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Create second snapshot with different content
			err = os.WriteFile(configPath, []byte("test = false\nanother = line"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Create third snapshot with different content
			err = os.WriteFile(configPath, []byte("test = nil\nyet = another"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Apply retention (keep only 2 chains)
			result, err := manager.ApplyRetention(countPolicy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BytesFreed).To(Equal(snap1.Size))
		})

		It("records removed snapshot IDs", func() {
			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create snapshots
			snap1, err := manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = false"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = nil"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Apply retention
			result, err := manager.ApplyRetention(countPolicy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RemovedSnapshots).To(ContainElement(snap1.ID))
		})

		It("works with age-based retention", func() {
			agePolicy, err := backup.NewAgeRetentionPolicy(1 * time.Hour)
			Expect(err).NotTo(HaveOccurred())

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create snapshot (will be current time)
			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Apply retention immediately (should keep all)
			result, err := manager.ApplyRetention(agePolicy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SnapshotsRemoved).To(Equal(0))

			// All snapshots should still exist
			snapshots, err := manager.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(snapshots).To(HaveLen(1))
		})

		It("works with size-based retention", func() {
			sizePolicy, err := backup.NewSizeRetentionPolicy(20) // Very small size
			Expect(err).NotTo(HaveOccurred())

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create multiple snapshots to exceed size limit
			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = false\nmore = content"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = nil\neven = more\nlines = here"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Apply retention
			result, err := manager.ApplyRetention(sizePolicy)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SnapshotsRemoved).To(BeNumerically(">", 0))
		})

		It("works with composite retention policy", func() {
			countPol, err := backup.NewCountRetentionPolicy(2)
			Expect(err).NotTo(HaveOccurred())

			agePol, err := backup.NewAgeRetentionPolicy(24 * time.Hour)
			Expect(err).NotTo(HaveOccurred())

			compositePolicy := backup.NewCompositeRetentionPolicy(countPol, agePol)

			opts := backup.CreateBackupOptions{
				ConfigPath: configPath,
				Trigger:    backup.TriggerManual,
			}

			// Create 3 snapshots
			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = false"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(configPath, []byte("test = nil"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.CreateBackup(opts)
			Expect(err).NotTo(HaveOccurred())

			// Apply composite retention
			result, err := manager.ApplyRetention(compositePolicy)

			Expect(err).NotTo(HaveOccurred())
			// Count policy will remove 1 (keep 2)
			Expect(result.SnapshotsRemoved).To(Equal(1))
		})
	})

	Describe("NewManagerWithAudit", func() {
		var auditLogger backup.AuditLogger

		BeforeEach(func() {
			auditFile := filepath.Join(tmpDir, "audit.jsonl")
			var err error
			auditLogger, err = backup.NewJSONLAuditLogger(auditFile)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if auditLogger != nil {
				auditLogger.Close()
			}
		})

		It("creates manager with audit logger", func() {
			mgr, err := backup.NewManagerWithAudit(storage, cfg, auditLogger)

			Expect(err).NotTo(HaveOccurred())
			Expect(mgr).NotTo(BeNil())
		})

		It("creates manager with nil config", func() {
			mgr, err := backup.NewManagerWithAudit(storage, nil, auditLogger)

			Expect(err).NotTo(HaveOccurred())
			Expect(mgr).NotTo(BeNil())
		})

		It("returns error for nil storage", func() {
			_, err := backup.NewManagerWithAudit(nil, cfg, auditLogger)

			Expect(err).To(MatchError(ContainSubstring("storage cannot be nil")))
		})
	})

	Describe("Internal Functions Coverage", func() {
		Context("getNextSequenceNumber", func() {
			It("assigns sequence 1 for first snapshot", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
				}

				snapshot, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.SequenceNum).To(Equal(1))
			})

			It("creates separate chains for different snapshots", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
				}

				snapshot1, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Change content to avoid deduplication
				err = os.WriteFile(configPath, []byte("test = false"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				snapshot2, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Phase 1 uses separate chains (each full snapshot is its own chain)
				Expect(snapshot1.SequenceNum).To(Equal(1))
				Expect(snapshot2.SequenceNum).To(Equal(1))
				Expect(snapshot2.ChainID).NotTo(Equal(snapshot1.ChainID))
			})

			It("handles empty snapshot list", func() {
				// Get should return error for non-existent storage
				_, err := manager.Get("non-existent")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("saveSnapshotToIndex", func() {
			It("persists snapshot to index", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
				}

				snapshot, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Verify snapshot is in index
				retrieved, err := manager.Get(snapshot.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved.ID).To(Equal(snapshot.ID))
			})

			It("updates existing snapshot in index", func() {
				opts := backup.CreateBackupOptions{
					ConfigPath: configPath,
					Trigger:    backup.TriggerManual,
					Metadata: backup.SnapshotMetadata{
						Tag: "v1",
					},
				}

				snapshot, err := manager.CreateBackup(opts)
				Expect(err).NotTo(HaveOccurred())

				// Verify tag is saved
				retrieved, err := manager.Get(snapshot.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved.Metadata.Tag).To(Equal("v1"))
			})
		})
	})
})
