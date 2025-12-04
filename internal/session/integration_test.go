package session_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/session"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("Integration Tests", func() {
	var (
		tempDir   string
		stateFile string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "session-integration-*")
		Expect(err).NotTo(HaveOccurred())

		stateFile = filepath.Join(tempDir, "session_state.json")
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("End-to-end workflow", func() {
		It("tracks full command lifecycle across saves/loads", func() {
			// Create tracker and simulate first command
			tracker1 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)

			sessionID := "test-session-123"
			tracker1.RecordCommand(sessionID)
			tracker1.RecordCommand(sessionID)

			// Session should be clean
			poisoned, info := tracker1.IsPoisoned(sessionID)
			Expect(poisoned).To(BeFalse())
			Expect(info).NotTo(BeNil())
			Expect(info.CommandCount).To(Equal(2))

			// Poison the session
			tracker1.Poison(sessionID, []string{"GIT001"}, "blocked commit")

			// Save state
			err := tracker1.Save()
			Expect(err).NotTo(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(stateFile)
			Expect(err).NotTo(HaveOccurred())

			// Create new tracker instance and load
			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			// Verify poisoned state persisted
			poisoned, info = tracker2.IsPoisoned(sessionID)
			Expect(poisoned).To(BeTrue())
			Expect(info).NotTo(BeNil())
			Expect(info.PoisonCodes).To(Equal([]string{"GIT001"}))
			Expect(info.PoisonMessage).To(Equal("blocked commit"))
			Expect(info.CommandCount).To(Equal(2))
		})

		It("handles multiple sessions independently", func() {
			tracker := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)

			session1 := "session-1"
			session2 := "session-2"
			session3 := "session-3"

			// Session 1: Clean with commands
			tracker.RecordCommand(session1)
			tracker.RecordCommand(session1)
			tracker.RecordCommand(session1)

			// Session 2: Poisoned
			tracker.Poison(session2, []string{"SEC001"}, "secrets detected")

			// Session 3: Poisoned after commands
			tracker.RecordCommand(session3)
			tracker.Poison(session3, []string{"GIT022"}, "force push blocked")

			// Save and reload
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			// Verify session 1 is clean
			poisoned1, info1 := tracker2.IsPoisoned(session1)
			Expect(poisoned1).To(BeFalse())
			Expect(info1.CommandCount).To(Equal(3))

			// Verify session 2 is poisoned
			poisoned2, info2 := tracker2.IsPoisoned(session2)
			Expect(poisoned2).To(BeTrue())
			Expect(info2.PoisonCodes).To(Equal([]string{"SEC001"}))

			// Verify session 3 is poisoned
			poisoned3, info3 := tracker2.IsPoisoned(session3)
			Expect(poisoned3).To(BeTrue())
			Expect(info3.PoisonCodes).To(Equal([]string{"GIT022"}))
			Expect(info3.CommandCount).To(Equal(1))
		})

		It("clears individual sessions", func() {
			tracker := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)

			session1 := "session-1"
			session2 := "session-2"

			tracker.RecordCommand(session1)
			tracker.RecordCommand(session2)

			// Clear session 1
			tracker.ClearSession(session1)

			// Save and reload
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			// Session 1 should not exist
			info1 := tracker2.GetInfo(session1)
			Expect(info1).To(BeNil())

			// Session 2 should still exist
			info2 := tracker2.GetInfo(session2)
			Expect(info2).NotTo(BeNil())
		})

		It("resets all sessions", func() {
			tracker := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)

			tracker.RecordCommand("session-1")
			tracker.Poison("session-2", []string{"GIT001"}, "test")
			tracker.RecordCommand("session-3")

			// Reset all
			tracker.Reset()

			// Save and reload
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			// All sessions should be gone
			state := tracker2.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})
	})

	Describe("Stale session cleanup", func() {
		It("removes sessions older than max age on load", func() {
			// Create fixed time
			baseTime := time.Date(2025, 12, 4, 10, 0, 0, 0, time.UTC)
			currentTime := baseTime

			// Create tracker with 1 hour max age
			tracker1 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(func() time.Time { return currentTime }),
				session.WithMaxSessionAge(1*time.Hour),
			)

			// Create sessions at different times
			tracker1.RecordCommand("old-session")

			// Advance 30 minutes
			currentTime = currentTime.Add(30 * time.Minute)
			tracker1.RecordCommand("recent-session")

			// Save
			err := tracker1.Save()
			Expect(err).NotTo(HaveOccurred())

			// Advance 2 hours total (old-session now 2h old, recent-session 1.5h old)
			currentTime = currentTime.Add(90 * time.Minute)

			// Load with new tracker - should cleanup both expired sessions
			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(func() time.Time { return currentTime }),
				session.WithMaxSessionAge(1*time.Hour),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			// Both sessions should be removed
			state := tracker2.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})

		It("keeps recent sessions during cleanup", func() {
			baseTime := time.Date(2025, 12, 4, 10, 0, 0, 0, time.UTC)
			currentTime := baseTime

			tracker1 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(func() time.Time { return currentTime }),
				session.WithMaxSessionAge(1*time.Hour),
			)

			// Create old session
			tracker1.RecordCommand("old-session")

			// Advance 30 minutes
			currentTime = currentTime.Add(30 * time.Minute)

			// Create recent session
			tracker1.RecordCommand("recent-session")

			// Save
			err := tracker1.Save()
			Expect(err).NotTo(HaveOccurred())

			// Advance 45 more minutes (old-session 1h15m, recent-session 45m)
			currentTime = currentTime.Add(45 * time.Minute)

			// Load with new tracker
			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(func() time.Time { return currentTime }),
				session.WithMaxSessionAge(1*time.Hour),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			// Old session should be removed
			info1 := tracker2.GetInfo("old-session")
			Expect(info1).To(BeNil())

			// Recent session should remain
			info2 := tracker2.GetInfo("recent-session")
			Expect(info2).NotTo(BeNil())
		})

		It("manually cleans up stale sessions", func() {
			baseTime := time.Date(2025, 12, 4, 10, 0, 0, 0, time.UTC)
			currentTime := baseTime

			tracker := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(func() time.Time { return currentTime }),
				session.WithMaxSessionAge(1*time.Hour),
			)

			// Create sessions
			tracker.RecordCommand("session-1")
			tracker.RecordCommand("session-2")
			tracker.RecordCommand("session-3")

			// Advance 2 hours
			currentTime = currentTime.Add(2 * time.Hour)

			// Manually cleanup
			removed := tracker.CleanupExpired()
			Expect(removed).To(Equal(3))

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})

		It("persists cleanup results", func() {
			baseTime := time.Date(2025, 12, 4, 10, 0, 0, 0, time.UTC)
			currentTime := baseTime

			tracker1 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(func() time.Time { return currentTime }),
				session.WithMaxSessionAge(1*time.Hour),
			)

			tracker1.RecordCommand("session-1")
			tracker1.RecordCommand("session-2")

			// Advance past expiry
			currentTime = currentTime.Add(2 * time.Hour)

			// Cleanup and save
			removed := tracker1.CleanupExpired()
			Expect(removed).To(Equal(2))

			err := tracker1.Save()
			Expect(err).NotTo(HaveOccurred())

			// Load with new tracker
			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			// Sessions should still be gone
			state := tracker2.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})
	})

	Describe("Real file system operations", func() {
		It("handles nested directory creation", func() {
			nestedPath := filepath.Join(tempDir, "a", "b", "c", "state.json")
			tracker := session.NewTracker(
				nil,
				session.WithStateFile(nestedPath),
			)

			tracker.RecordCommand("session-1")
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			// Verify full path exists
			info, err := os.Stat(nestedPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

			// Verify directory permissions
			dirInfo, err := os.Stat(filepath.Dir(nestedPath))
			Expect(err).NotTo(HaveOccurred())
			Expect(dirInfo.Mode().Perm()).To(Equal(os.FileMode(0o700)))
		})

		It("maintains file permissions across saves", func() {
			tracker := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)

			// First save
			tracker.RecordCommand("session-1")
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			info1, err := os.Stat(stateFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(info1.Mode().Perm()).To(Equal(os.FileMode(0o600)))

			// Second save
			tracker.RecordCommand("session-1")
			err = tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			info2, err := os.Stat(stateFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(info2.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		})

		It("recovers from corrupted state files", func() {
			// Write invalid JSON
			err := os.WriteFile(stateFile, []byte("{ invalid json }"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			tracker := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)

			// Load should succeed with empty state
			err = tracker.Load()
			Expect(err).NotTo(HaveOccurred())

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())

			// Should be able to use tracker normally
			tracker.RecordCommand("session-1")
			err = tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			// Verify file now has valid JSON
			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			info := tracker2.GetInfo("session-1")
			Expect(info).NotTo(BeNil())
		})
	})

	Describe("Configuration integration", func() {
		It("respects disabled configuration", func() {
			disabled := false
			tracker := session.NewTracker(
				&config.SessionConfig{
					Enabled: &disabled,
				},
				session.WithStateFile(stateFile),
			)

			Expect(tracker.IsEnabled()).To(BeFalse())
		})

		It("uses default state file from config", func() {
			customPath := filepath.Join(tempDir, "custom.json")
			tracker := session.NewTracker(
				&config.SessionConfig{
					StateFile: customPath,
				},
			)

			tracker.RecordCommand("session-1")
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			// Verify file created at custom path
			_, err = os.Stat(customPath)
			Expect(err).NotTo(HaveOccurred())
		})

		It("uses max session age from config", func() {
			baseTime := time.Date(2025, 12, 4, 10, 0, 0, 0, time.UTC)
			currentTime := baseTime

			maxAge := config.Duration(30 * time.Minute)
			tracker := session.NewTracker(
				&config.SessionConfig{
					MaxSessionAge: maxAge,
				},
				session.WithStateFile(stateFile),
				session.WithTimeFunc(func() time.Time { return currentTime }),
			)

			tracker.RecordCommand("session-1")

			// Advance 45 minutes
			currentTime = currentTime.Add(45 * time.Minute)

			// Session should be expired
			poisoned, info := tracker.IsPoisoned("session-1")
			Expect(poisoned).To(BeFalse())
			Expect(info).To(BeNil())
		})
	})
})
