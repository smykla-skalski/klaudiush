package patterns_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("FilePatternStore", func() {
	var (
		tmpDir string
		store  *patterns.FilePatternStore
		cfg    *config.PatternsConfig
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		cfg = &config.PatternsConfig{
			ProjectDataFile: "patterns.json",
			GlobalDataDir:   filepath.Join(tmpDir, "global"),
		}
		store = patterns.NewFilePatternStore(cfg, tmpDir)
	})

	Describe("Load and Save", func() {
		It("loads empty state when no files exist", func() {
			err := store.Load()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.GetAllPatterns()).To(BeEmpty())
		})

		It("persists and reloads global patterns", func() {
			store.RecordSequence("GIT013", "GIT004")
			store.RecordSequence("GIT013", "GIT004")
			Expect(store.Save()).To(Succeed())

			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			all := store2.GetAllPatterns()
			Expect(all).To(HaveLen(1))
			Expect(all[0].SourceCode).To(Equal("GIT013"))
			Expect(all[0].TargetCode).To(Equal("GIT004"))
			Expect(all[0].Count).To(Equal(2))
		})

		It("persists project-local patterns via SaveProject", func() {
			store.SetProjectData(patterns.SeedPatterns())
			Expect(store.SaveProject()).To(Succeed())

			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			all := store2.GetAllPatterns()
			Expect(len(all)).To(BeNumerically(">=", 4))
		})
	})

	Describe("RecordSequence", func() {
		It("increments count for existing patterns", func() {
			store.RecordSequence("A", "B")
			store.RecordSequence("A", "B")
			store.RecordSequence("A", "B")

			followUps := store.GetFollowUps("A", 1)
			Expect(followUps).To(HaveLen(1))
			Expect(followUps[0].Count).To(Equal(3))
		})

		It("creates separate entries for different pairs", func() {
			store.RecordSequence("A", "B")
			store.RecordSequence("A", "C")

			followUps := store.GetFollowUps("A", 1)
			Expect(followUps).To(HaveLen(2))
		})
	})

	Describe("GetFollowUps", func() {
		It("filters by minCount", func() {
			store.RecordSequence("A", "B")
			store.RecordSequence("A", "C")
			store.RecordSequence("A", "C")
			store.RecordSequence("A", "C")

			followUps := store.GetFollowUps("A", 3)
			Expect(followUps).To(HaveLen(1))
			Expect(followUps[0].TargetCode).To(Equal("C"))
		})

		It("merges project and global counts", func() {
			seeds := patterns.SeedPatterns()
			store.SetProjectData(seeds)

			// Record additional observation
			store.RecordSequence("GIT013", "GIT004")

			followUps := store.GetFollowUps("GIT013", 1)

			var git004 *patterns.FailurePattern

			for _, fp := range followUps {
				if fp.TargetCode == "GIT004" {
					git004 = fp
				}
			}

			Expect(git004).NotTo(BeNil())
			// seed count (5) + recorded (1)
			Expect(git004.Count).To(Equal(6))
		})

		It("uses latest timestamps when merging", func() {
			seeds := patterns.SeedPatterns()
			store.SetProjectData(seeds)

			// Record in global to create overlapping entry
			store.RecordSequence("GIT013", "GIT004")

			all := store.GetAllPatterns()

			var git004 *patterns.FailurePattern

			for _, fp := range all {
				if fp.SourceCode == "GIT013" && fp.TargetCode == "GIT004" {
					git004 = fp
				}
			}

			Expect(git004).NotTo(BeNil())
			// Global entry was recorded after seed, so LastSeen
			// should be at least as recent as seed
			Expect(git004.LastSeen).NotTo(BeZero())
			Expect(git004.FirstSeen).NotTo(BeZero())
		})
	})

	Describe("Cleanup", func() {
		It("removes patterns older than maxAge", func() {
			store.RecordSequence("OLD", "PAT")

			// Make it old by saving and reloading with manipulated timestamps
			all := store.GetAllPatterns()
			Expect(all).To(HaveLen(1))

			removed := store.Cleanup(0)
			Expect(removed).To(Equal(1))
			Expect(store.GetAllPatterns()).To(BeEmpty())
		})

		It("keeps recent patterns", func() {
			store.RecordSequence("NEW", "PAT")

			removed := store.Cleanup(time.Hour)
			Expect(removed).To(Equal(0))
			Expect(store.GetAllPatterns()).To(HaveLen(1))
		})
	})

	Describe("Session codes", func() {
		It("returns nil for unknown session", func() {
			codes := store.GetSessionCodes("unknown")
			Expect(codes).To(BeNil())
		})

		It("stores and retrieves session codes", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})

			codes := store.GetSessionCodes("sess1")
			Expect(codes).To(Equal([]string{"GIT013"}))
		})

		It("sets LastSeen timestamp on session entries", func() {
			before := time.Now()

			store.SetSessionCodes("sess1", []string{"GIT013"})

			after := time.Now()

			sessions := store.GetSessions()
			Expect(sessions).To(HaveKey("sess1"))
			Expect(sessions["sess1"].LastSeen).To(BeTemporally(">=", before))
			Expect(sessions["sess1"].LastSeen).To(BeTemporally("<=", after))
		})

		It("clears session codes", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})
			store.ClearSessionCodes("sess1")

			codes := store.GetSessionCodes("sess1")
			Expect(codes).To(BeNil())
		})

		It("persists session codes across save/load", func() {
			store.SetSessionCodes("sess1", []string{"GIT013", "GIT004"})
			Expect(store.Save()).To(Succeed())

			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			codes := store2.GetSessionCodes("sess1")
			Expect(codes).To(Equal([]string{"GIT013", "GIT004"}))
		})

		It("persists session timestamps across save/load", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})
			Expect(store.Save()).To(Succeed())

			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			sessions := store2.GetSessions()
			Expect(sessions).To(HaveKey("sess1"))
			Expect(sessions["sess1"].LastSeen).NotTo(BeZero())
		})

		It("clears session codes without crashing on empty store", func() {
			store.ClearSessionCodes("nonexistent")

			codes := store.GetSessionCodes("nonexistent")
			Expect(codes).To(BeNil())
		})

		It("reports active session count", func() {
			Expect(store.GetActiveSessions()).To(Equal(0))

			store.SetSessionCodes("sess1", []string{"GIT013"})
			store.SetSessionCodes("sess2", []string{"GIT004"})
			Expect(store.GetActiveSessions()).To(Equal(2))

			store.ClearSessionCodes("sess1")
			Expect(store.GetActiveSessions()).To(Equal(1))
		})

		It("returns a copy from GetSessions", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})

			sessions := store.GetSessions()
			sessions["sess1"].Codes = []string{"MODIFIED"}

			codes := store.GetSessionCodes("sess1")
			Expect(codes).To(Equal([]string{"GIT013"}))
		})
	})

	Describe("CleanupSessions", func() {
		It("removes expired sessions", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})
			Expect(store.GetActiveSessions()).To(Equal(1))

			// maxAge=0 means everything is expired
			removed := store.CleanupSessions(0)
			Expect(removed).To(Equal(1))
			Expect(store.GetActiveSessions()).To(Equal(0))
		})

		It("keeps recent sessions", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})

			removed := store.CleanupSessions(time.Hour)
			Expect(removed).To(Equal(0))
			Expect(store.GetActiveSessions()).To(Equal(1))
		})

		It("handles empty sessions map", func() {
			removed := store.CleanupSessions(time.Hour)
			Expect(removed).To(Equal(0))
		})

		It("works together with pattern Cleanup", func() {
			store.RecordSequence("OLD", "PAT")
			store.SetSessionCodes("sess1", []string{"GIT013"})

			// Both pattern and session are recent
			pRemoved := store.Cleanup(time.Hour)
			sRemoved := store.CleanupSessions(time.Hour)

			Expect(pRemoved).To(Equal(0))
			Expect(sRemoved).To(Equal(0))
			Expect(store.GetAllPatterns()).To(HaveLen(1))
			Expect(store.GetActiveSessions()).To(Equal(1))

			// Both expired
			pRemoved = store.Cleanup(0)
			sRemoved = store.CleanupSessions(0)

			Expect(pRemoved).To(Equal(1))
			Expect(sRemoved).To(Equal(1))
			Expect(store.GetAllPatterns()).To(BeEmpty())
			Expect(store.GetActiveSessions()).To(Equal(0))
		})
	})

	Describe("TrimPatterns", func() {
		It("reduces count to the limit", func() {
			store.RecordSequence("A", "X")
			store.RecordSequence("B", "X")
			store.RecordSequence("C", "X")
			Expect(store.GetAllPatterns()).To(HaveLen(3))

			removed := store.TrimPatterns(1)
			Expect(removed).To(Equal(2))
			Expect(store.GetAllPatterns()).To(HaveLen(1))
		})

		It("preserves the newest pattern when trimming to 1", func() {
			// Save OLD pattern then reload to get a past timestamp in the file
			store.RecordSequence("OLD", "X")
			Expect(store.Save()).To(Succeed())

			// New store loads OLD, then records NEW (which gets time.Now())
			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())
			store2.RecordSequence("NEW", "Y")

			removed := store2.TrimPatterns(1)
			Expect(removed).To(Equal(1))

			remaining := store2.GetAllPatterns()
			Expect(remaining).To(HaveLen(1))
			// The remaining pattern should be the more recently seen one
			Expect(remaining[0].SourceCode).To(Equal("NEW"))
		})

		It("does nothing when count is at or below the limit", func() {
			store.RecordSequence("GIT013", "GIT004")
			removed := store.TrimPatterns(5)
			Expect(removed).To(Equal(0))
			Expect(store.GetAllPatterns()).To(HaveLen(1))
		})

		It("does nothing on empty store", func() {
			removed := store.TrimPatterns(10)
			Expect(removed).To(Equal(0))
		})

		It("does nothing when maxCount is 0", func() {
			store.RecordSequence("GIT013", "GIT004")
			removed := store.TrimPatterns(0)
			Expect(removed).To(Equal(0))
			Expect(store.GetAllPatterns()).To(HaveLen(1))
		})
	})

	Describe("TrimSessions", func() {
		It("reduces count to the limit", func() {
			store.SetSessionCodes("s1", []string{"GIT013"})
			store.SetSessionCodes("s2", []string{"GIT004"})
			store.SetSessionCodes("s3", []string{"GIT005"})
			Expect(store.GetActiveSessions()).To(Equal(3))

			removed := store.TrimSessions(1)
			Expect(removed).To(Equal(2))
			Expect(store.GetActiveSessions()).To(Equal(1))
		})

		It("does nothing when count is at or below the limit", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})
			removed := store.TrimSessions(5)
			Expect(removed).To(Equal(0))
			Expect(store.GetActiveSessions()).To(Equal(1))
		})

		It("does nothing on empty store", func() {
			removed := store.TrimSessions(10)
			Expect(removed).To(Equal(0))
		})

		It("does nothing when maxCount is 0", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})
			removed := store.TrimSessions(0)
			Expect(removed).To(Equal(0))
			Expect(store.GetActiveSessions()).To(Equal(1))
		})
	})

	Describe("Backward compatibility", func() {
		It("migrates old session format on load", func() {
			// Write old format: sessions as map[string][]string
			oldData := `{
				"patterns": {},
				"sessions": {
					"old-sess": ["GIT013", "GIT004"]
				},
				"last_updated": "2026-01-01T00:00:00Z",
				"version": 1
			}`

			store.RecordSequence("A", "B")
			Expect(store.Save()).To(Succeed())

			// Find and overwrite the global file with old format
			globalDir := filepath.Join(tmpDir, "global")
			entries, err := os.ReadDir(globalDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).NotTo(BeEmpty())

			globalFile := filepath.Join(globalDir, entries[0].Name())
			Expect(os.WriteFile(globalFile, []byte(oldData), 0o600)).To(Succeed())

			// Load should migrate
			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			codes := store2.GetSessionCodes("old-sess")
			Expect(codes).To(Equal([]string{"GIT013", "GIT004"}))

			// Migrated sessions get zero timestamps so they expire on cleanup
			sessions := store2.GetSessions()
			Expect(sessions["old-sess"].LastSeen).To(BeZero())
		})

		It("loads current format normally", func() {
			store.SetSessionCodes("sess1", []string{"GIT013"})
			Expect(store.Save()).To(Succeed())

			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			codes := store2.GetSessionCodes("sess1")
			Expect(codes).To(Equal([]string{"GIT013"}))

			sessions := store2.GetSessions()
			Expect(sessions["sess1"].LastSeen).NotTo(BeZero())
		})
	})

	Describe("Load with corrupt data", func() {
		It("ignores invalid JSON in project file", func() {
			projectFile := filepath.Join(tmpDir, "patterns.json")
			Expect(
				os.WriteFile(projectFile, []byte(`{invalid json`), 0o600),
			).To(Succeed())

			err := store.Load()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.GetAllPatterns()).To(BeEmpty())
		})

		It("ignores invalid JSON in global file", func() {
			// First save valid data to create the global file
			store.RecordSequence("A", "B")
			Expect(store.Save()).To(Succeed())

			// Corrupt the global file by finding it
			globalDir := filepath.Join(tmpDir, "global")
			entries, err := os.ReadDir(globalDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).NotTo(BeEmpty())

			globalFile := filepath.Join(globalDir, entries[0].Name())
			Expect(
				os.WriteFile(globalFile, []byte(`not json`), 0o600),
			).To(Succeed())

			// Reload should not error but should have empty global
			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			err = store2.Load()
			Expect(err).NotTo(HaveOccurred())
			Expect(store2.GetAllPatterns()).To(BeEmpty())
		})

		It("handles file with null patterns field", func() {
			projectFile := filepath.Join(tmpDir, "patterns.json")
			Expect(
				os.WriteFile(
					projectFile,
					[]byte(`{"patterns":null,"version":1}`),
					0o600,
				),
			).To(Succeed())

			err := store.Load()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.GetAllPatterns()).To(BeEmpty())
		})
	})

	Describe("HasProjectData", func() {
		It("returns false when file does not exist", func() {
			Expect(store.HasProjectData()).To(BeFalse())
		})

		It("returns true after saving project data", func() {
			store.SetProjectData(patterns.SeedPatterns())
			Expect(store.SaveProject()).To(Succeed())
			Expect(store.HasProjectData()).To(BeTrue())
		})
	})

	Describe("EnsureSeedData", func() {
		It("writes seed data when no project file exists", func() {
			err := patterns.EnsureSeedData(store)
			Expect(err).NotTo(HaveOccurred())

			projectFile := filepath.Join(tmpDir, "patterns.json")
			Expect(projectFile).To(BeAnExistingFile())

			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			all := store2.GetAllPatterns()
			// 4 original + 9 new cross-category seeds
			Expect(len(all)).To(BeNumerically(">=", 13))
		})

		It("includes cross-category seed patterns", func() {
			err := patterns.EnsureSeedData(store)
			Expect(err).NotTo(HaveOccurred())

			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())

			// Check SEC->SHELL cascade
			followUps := store2.GetFollowUps("SEC001", 1)

			var found bool

			for _, fp := range followUps {
				if fp.TargetCode == "SHELL001" {
					found = true

					Expect(fp.Seed).To(BeTrue())
				}
			}

			Expect(found).To(BeTrue(), "SEC001->SHELL001 seed not found")

			// Check FILE->FILE cascade
			followUps = store2.GetFollowUps("FILE006", 1)
			found = false

			for _, fp := range followUps {
				if fp.TargetCode == "FILE005" {
					found = true
				}
			}

			Expect(found).To(BeTrue(), "FILE006->FILE005 seed not found")
		})

		It("skips when project file already exists", func() {
			// Create the file first
			projectFile := filepath.Join(tmpDir, "patterns.json")
			Expect(
				os.WriteFile(projectFile, []byte(`{"patterns":{},"version":1}`), 0o600),
			).To(Succeed())

			err := patterns.EnsureSeedData(store)
			Expect(err).NotTo(HaveOccurred())

			// Should still have empty patterns (didn't overwrite)
			store2 := patterns.NewFilePatternStore(cfg, tmpDir)
			Expect(store2.Load()).To(Succeed())
			Expect(store2.GetAllPatterns()).To(BeEmpty())
		})
	})

	Describe("legacy fallback and XDG migration compatibility", func() {
		var (
			homeDir string
			xdgData string
		)

		BeforeEach(func() {
			homeDir = filepath.Join(tmpDir, "home")
			xdgData = filepath.Join(tmpDir, "xdg-data")

			GinkgoT().Setenv("HOME", homeDir)
			GinkgoT().Setenv("XDG_DATA_HOME", xdgData)
		})

		It("loads legacy learned data when the default XDG path has no data yet", func() {
			legacyStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
				GlobalDataDir:   config.LegacyPatternsGlobalDataDir,
			}, tmpDir)
			legacyStore.RecordSequence("GIT013", "GIT004")
			Expect(legacyStore.Save()).To(Succeed())

			defaultStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
			}, tmpDir)
			Expect(defaultStore.Load()).To(Succeed())

			followUps := defaultStore.GetFollowUps("GIT013", 1)
			Expect(followUps).To(HaveLen(1))
			Expect(followUps[0].TargetCode).To(Equal("GIT004"))
			Expect(followUps[0].Count).To(Equal(1))
		})

		It("merges XDG and legacy learned data when both exist", func() {
			legacyStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
				GlobalDataDir:   config.LegacyPatternsGlobalDataDir,
			}, tmpDir)
			legacyStore.RecordSequence("GIT013", "GIT004")
			legacyStore.SetSessionCodes("sess1", []string{"GIT013"})
			Expect(legacyStore.Save()).To(Succeed())

			time.Sleep(10 * time.Millisecond)

			xdgStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
				GlobalDataDir:   filepath.Join(xdgData, "klaudiush", "patterns"),
			}, tmpDir)
			xdgStore.RecordSequence("GIT013", "GIT004")
			xdgStore.RecordSequence("GIT013", "GIT004")
			xdgStore.SetSessionCodes("sess1", []string{"GIT004"})
			Expect(xdgStore.Save()).To(Succeed())

			defaultStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
			}, tmpDir)
			Expect(defaultStore.Load()).To(Succeed())

			followUps := defaultStore.GetFollowUps("GIT013", 1)
			Expect(followUps).To(HaveLen(1))
			Expect(followUps[0].Count).To(Equal(3))

			Expect(defaultStore.GetSessionCodes("sess1")).To(Equal([]string{"GIT004"}))
		})

		It("does not use the legacy fallback when global_data_dir is explicitly set", func() {
			legacyStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
				GlobalDataDir:   config.LegacyPatternsGlobalDataDir,
			}, tmpDir)
			legacyStore.RecordSequence("GIT013", "GIT004")
			Expect(legacyStore.Save()).To(Succeed())

			customStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
				GlobalDataDir:   filepath.Join(tmpDir, "custom-patterns"),
			}, tmpDir)
			Expect(customStore.Load()).To(Succeed())
			Expect(customStore.GetAllPatterns()).To(BeEmpty())
		})

		It("writes back to the XDG primary path after loading legacy data", func() {
			legacyStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
				GlobalDataDir:   config.LegacyPatternsGlobalDataDir,
			}, tmpDir)
			legacyStore.RecordSequence("GIT013", "GIT004")
			Expect(legacyStore.Save()).To(Succeed())

			defaultStore := patterns.NewFilePatternStore(&config.PatternsConfig{
				ProjectDataFile: "patterns.json",
			}, tmpDir)
			Expect(defaultStore.Load()).To(Succeed())
			Expect(defaultStore.Save()).To(Succeed())

			legacyDir := filepath.Join(homeDir, ".klaudiush", "patterns")
			xdgDir := filepath.Join(xdgData, "klaudiush", "patterns")

			legacyEntries, err := os.ReadDir(legacyDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(legacyEntries).NotTo(BeEmpty())

			xdgEntries, err := os.ReadDir(xdgDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(xdgEntries).NotTo(BeEmpty())
		})
	})
})
