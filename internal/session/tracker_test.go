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

var _ = Describe("Tracker", func() {
	var (
		tracker     *session.Tracker
		tempDir     string
		stateFile   string
		currentTime time.Time
		timeFunc    func() time.Time
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "session-test-*")
		Expect(err).NotTo(HaveOccurred())

		stateFile = filepath.Join(tempDir, "session_state.json")
		currentTime = time.Date(2025, 12, 4, 10, 30, 0, 0, time.UTC)
		timeFunc = func() time.Time { return currentTime }
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("NewTracker", func() {
		It("creates tracker with nil config", func() {
			t := session.NewTracker(nil)
			Expect(t).NotTo(BeNil())
		})

		It("creates tracker with config", func() {
			t := session.NewTracker(&config.SessionConfig{})
			Expect(t).NotTo(BeNil())
		})

		It("accepts custom state file", func() {
			t := session.NewTracker(
				nil,
				session.WithStateFile("/custom/path.json"),
			)
			Expect(t).NotTo(BeNil())
		})

		It("accepts custom time function", func() {
			customTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			t := session.NewTracker(
				nil,
				session.WithTimeFunc(func() time.Time { return customTime }),
			)
			Expect(t).NotTo(BeNil())
		})

		It("accepts custom max session age", func() {
			t := session.NewTracker(
				nil,
				session.WithMaxSessionAge(48*time.Hour),
			)
			Expect(t).NotTo(BeNil())
		})
	})

	Describe("IsPoisoned", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("returns false for unknown session", func() {
			poisoned, info := tracker.IsPoisoned("unknown-session-id")
			Expect(poisoned).To(BeFalse())
			Expect(info).To(BeNil())
		})

		It("returns false for empty session ID", func() {
			poisoned, info := tracker.IsPoisoned("")
			Expect(poisoned).To(BeFalse())
			Expect(info).To(BeNil())
		})

		It("returns false for clean session", func() {
			tracker.RecordCommand("session-1")

			poisoned, info := tracker.IsPoisoned("session-1")
			Expect(poisoned).To(BeFalse())
			Expect(info).NotTo(BeNil())
			Expect(info.Status).To(Equal(session.StatusClean))
		})

		It("returns true for poisoned session", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "blocked commit")

			poisoned, info := tracker.IsPoisoned("session-1")
			Expect(poisoned).To(BeTrue())
			Expect(info).NotTo(BeNil())
			Expect(info.PoisonCodes).To(Equal([]string{"GIT001"}))
			Expect(info.PoisonMessage).To(Equal("blocked commit"))
		})
	})

	Describe("Poison", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("poisons new session", func() {
			tracker.Poison("session-1", []string{"GIT022"}, "force push blocked")

			info := tracker.GetInfo("session-1")
			Expect(info).NotTo(BeNil())
			Expect(info.Status).To(Equal(session.StatusPoisoned))
			Expect(info.PoisonCodes).To(Equal([]string{"GIT022"}))
			Expect(info.PoisonMessage).To(Equal("force push blocked"))
			Expect(info.PoisonedAt).NotTo(BeNil())
			Expect(*info.PoisonedAt).To(Equal(currentTime))
		})

		It("poisons existing clean session", func() {
			tracker.RecordCommand("session-1")
			tracker.Poison("session-1", []string{"SEC001"}, "secrets detected")

			info := tracker.GetInfo("session-1")
			Expect(info.Status).To(Equal(session.StatusPoisoned))
			Expect(info.CommandCount).To(Equal(1))
		})

		It("ignores empty session ID", func() {
			tracker.Poison("", []string{"GIT001"}, "test")

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})

		It("updates last activity time", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "test")

			info := tracker.GetInfo("session-1")
			Expect(info.LastActivity).To(Equal(currentTime))
		})

		It("stores multiple poison codes", func() {
			tracker.Poison(
				"session-1",
				[]string{"GIT001", "GIT002", "SEC001"},
				"multiple violations",
			)

			info := tracker.GetInfo("session-1")
			Expect(info.PoisonCodes).To(Equal([]string{"GIT001", "GIT002", "SEC001"}))
			Expect(info.PoisonMessage).To(Equal("multiple violations"))
		})
	})

	Describe("Unpoison", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("clears poisoned session state", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "blocked commit")
			tracker.Unpoison("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info).NotTo(BeNil())
			Expect(info.Status).To(Equal(session.StatusClean))
			Expect(info.PoisonCodes).To(BeNil())
			Expect(info.PoisonMessage).To(BeEmpty())
			Expect(info.PoisonedAt).To(BeNil())
		})

		It("clears session with multiple poison codes", func() {
			tracker.Poison(
				"session-1",
				[]string{"GIT001", "GIT002", "SEC001"},
				"multiple violations",
			)
			tracker.Unpoison("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info.Status).To(Equal(session.StatusClean))
			Expect(info.PoisonCodes).To(BeNil())
		})

		It("ignores empty session ID", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "test")
			tracker.Unpoison("")

			// session-1 should still be poisoned
			poisoned, _ := tracker.IsPoisoned("session-1")
			Expect(poisoned).To(BeTrue())
		})

		It("ignores non-existent session", func() {
			// Should not panic
			tracker.Unpoison("non-existent")
		})

		It("updates last activity time", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "test")

			// Advance time
			currentTime = currentTime.Add(1 * time.Hour)

			tracker.Unpoison("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info.LastActivity).To(Equal(currentTime))
		})

		It("allows session to be poisoned again after unpoison", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "first poison")
			tracker.Unpoison("session-1")
			tracker.Poison("session-1", []string{"SEC001", "SEC002"}, "second poison")

			info := tracker.GetInfo("session-1")
			Expect(info.Status).To(Equal(session.StatusPoisoned))
			Expect(info.PoisonCodes).To(Equal([]string{"SEC001", "SEC002"}))
			Expect(info.PoisonMessage).To(Equal("second poison"))
		})
	})

	Describe("RecordCommand", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("creates new session on first command", func() {
			tracker.RecordCommand("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info).NotTo(BeNil())
			Expect(info.CommandCount).To(Equal(1))
			Expect(info.Status).To(Equal(session.StatusClean))
		})

		It("increments command count", func() {
			tracker.RecordCommand("session-1")
			tracker.RecordCommand("session-1")
			tracker.RecordCommand("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info.CommandCount).To(Equal(3))
		})

		It("ignores empty session ID", func() {
			tracker.RecordCommand("")

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})

		It("updates last activity time", func() {
			tracker.RecordCommand("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info.LastActivity).To(Equal(currentTime))
		})
	})

	Describe("GetInfo", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("returns nil for unknown session", func() {
			info := tracker.GetInfo("unknown")
			Expect(info).To(BeNil())
		})

		It("returns nil for empty session ID", func() {
			info := tracker.GetInfo("")
			Expect(info).To(BeNil())
		})

		It("returns copy of session info", func() {
			tracker.RecordCommand("session-1")

			info1 := tracker.GetInfo("session-1")
			info2 := tracker.GetInfo("session-1")

			// Modify info1
			info1.CommandCount = 999

			// info2 should not be affected
			Expect(info2.CommandCount).To(Equal(1))
		})
	})

	Describe("GetState", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("returns copy of state", func() {
			tracker.RecordCommand("session-1")

			state1 := tracker.GetState()
			state2 := tracker.GetState()

			// Modify state1
			state1.Sessions["session-1"].CommandCount = 999

			// state2 should not be affected
			Expect(state2.Sessions["session-1"].CommandCount).To(Equal(1))
		})
	})

	Describe("ClearSession", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("removes session from tracking", func() {
			tracker.RecordCommand("session-1")
			tracker.ClearSession("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info).To(BeNil())
		})

		It("ignores empty session ID", func() {
			tracker.RecordCommand("session-1")
			tracker.ClearSession("")

			info := tracker.GetInfo("session-1")
			Expect(info).NotTo(BeNil())
		})

		It("handles non-existent session", func() {
			// Should not panic
			tracker.ClearSession("non-existent")
		})
	})

	Describe("Reset", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("clears all sessions", func() {
			tracker.RecordCommand("session-1")
			tracker.RecordCommand("session-2")
			tracker.Poison("session-3", []string{"GIT001"}, "test")

			tracker.Reset()

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})
	})

	Describe("Load and Save", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("loads fresh state when file doesn't exist", func() {
			err := tracker.Load()
			Expect(err).NotTo(HaveOccurred())

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})

		It("saves state to file", func() {
			tracker.RecordCommand("session-1")
			tracker.Poison("session-2", []string{"GIT001"}, "test")

			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(stateFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("loads previously saved state", func() {
			tracker.RecordCommand("session-1")
			tracker.RecordCommand("session-1")
			tracker.Poison("session-2", []string{"GIT001"}, "test")

			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			// Create new tracker and load
			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			info1 := tracker2.GetInfo("session-1")
			Expect(info1).NotTo(BeNil())
			Expect(info1.CommandCount).To(Equal(2))
			Expect(info1.Status).To(Equal(session.StatusClean))

			info2 := tracker2.GetInfo("session-2")
			Expect(info2).NotTo(BeNil())
			Expect(info2.Status).To(Equal(session.StatusPoisoned))
			Expect(info2.PoisonCodes).To(Equal([]string{"GIT001"}))
		})

		It("handles corrupted state file gracefully", func() {
			err := os.WriteFile(stateFile, []byte("not valid json"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = tracker.Load()
			Expect(err).NotTo(HaveOccurred())

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})

		It("creates directory if needed", func() {
			nestedPath := filepath.Join(tempDir, "nested", "dir", "state.json")
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(nestedPath),
				session.WithTimeFunc(timeFunc),
			)

			tracker.RecordCommand("session-1")
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(nestedPath)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Session expiry", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
				session.WithMaxSessionAge(1*time.Hour),
			)
		})

		It("expires old sessions", func() {
			tracker.RecordCommand("session-1")

			// Advance time past max age
			currentTime = currentTime.Add(2 * time.Hour)

			poisoned, info := tracker.IsPoisoned("session-1")
			Expect(poisoned).To(BeFalse())
			Expect(info).To(BeNil())
		})

		It("does not expire recent sessions", func() {
			tracker.RecordCommand("session-1")

			// Advance time but not past max age
			currentTime = currentTime.Add(30 * time.Minute)

			info := tracker.GetInfo("session-1")
			Expect(info).NotTo(BeNil())
		})

		It("resets expired session on record command", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "test")

			// Advance time past max age
			currentTime = currentTime.Add(2 * time.Hour)

			// Record new command - should reset the expired session
			tracker.RecordCommand("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info).NotTo(BeNil())
			Expect(info.Status).To(Equal(session.StatusClean))
			Expect(info.CommandCount).To(Equal(1))
		})

		It("cleans up expired sessions during load", func() {
			tracker.RecordCommand("session-1")
			tracker.RecordCommand("session-2")
			err := tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			// Advance time past max age
			currentTime = currentTime.Add(2 * time.Hour)

			// Load with new tracker - should cleanup expired
			tracker2 := session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
				session.WithMaxSessionAge(1*time.Hour),
			)
			err = tracker2.Load()
			Expect(err).NotTo(HaveOccurred())

			state := tracker2.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})
	})

	Describe("CleanupExpired", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
				session.WithMaxSessionAge(1*time.Hour),
			)
		})

		It("removes expired sessions", func() {
			tracker.RecordCommand("session-1")
			tracker.RecordCommand("session-2")

			// Advance time past max age
			currentTime = currentTime.Add(2 * time.Hour)

			removed := tracker.CleanupExpired()
			Expect(removed).To(Equal(2))

			state := tracker.GetState()
			Expect(state.Sessions).To(BeEmpty())
		})

		It("keeps non-expired sessions", func() {
			tracker.RecordCommand("session-1")

			// Advance time but not past max age
			currentTime = currentTime.Add(30 * time.Minute)
			tracker.RecordCommand("session-2")

			// Advance to expire session-1 but not session-2
			currentTime = currentTime.Add(40 * time.Minute)

			removed := tracker.CleanupExpired()
			Expect(removed).To(Equal(1))

			info1 := tracker.GetInfo("session-1")
			Expect(info1).To(BeNil())

			info2 := tracker.GetInfo("session-2")
			Expect(info2).NotTo(BeNil())
		})
	})

	Describe("IsEnabled", func() {
		It("returns false with nil tracker", func() {
			var nilTracker *session.Tracker
			Expect(nilTracker.IsEnabled()).To(BeFalse())
		})

		It("returns false with nil config", func() {
			tracker = session.NewTracker(nil)
			Expect(tracker.IsEnabled()).To(BeFalse())
		})

		It("returns false with nil enabled", func() {
			tracker = session.NewTracker(&config.SessionConfig{})
			Expect(tracker.IsEnabled()).To(BeFalse())
		})

		It("returns false when disabled", func() {
			enabled := false
			tracker = session.NewTracker(&config.SessionConfig{
				Enabled: &enabled,
			})
			Expect(tracker.IsEnabled()).To(BeFalse())
		})

		It("returns true when enabled", func() {
			enabled := true
			tracker = session.NewTracker(&config.SessionConfig{
				Enabled: &enabled,
			})
			Expect(tracker.IsEnabled()).To(BeTrue())
		})
	})

	Describe("Home directory expansion", func() {
		It("expands ~ in state file path", func() {
			home, err := os.UserHomeDir()
			Expect(err).NotTo(HaveOccurred())

			homePath := filepath.Join(home, ".klaudiush-session-test", "state.json")
			tracker = session.NewTracker(
				nil,
				session.WithStateFile("~/.klaudiush-session-test/state.json"),
				session.WithTimeFunc(timeFunc),
			)

			tracker.RecordCommand("session-1")
			err = tracker.Save()
			Expect(err).NotTo(HaveOccurred())

			// Verify file was created at expanded path
			_, err = os.Stat(homePath)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			_ = os.RemoveAll(filepath.Join(home, ".klaudiush-session-test"))
		})
	})

	Describe("Concurrent access", func() {
		BeforeEach(func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)
		})

		It("handles concurrent RecordCommand calls safely", func() {
			done := make(chan bool)
			count := 100

			for range count {
				go func() {
					tracker.RecordCommand("session-1")
					done <- true
				}()
			}

			for range count {
				<-done
			}

			info := tracker.GetInfo("session-1")
			Expect(info.CommandCount).To(Equal(count))
		})

		It("handles concurrent Poison calls safely", func() {
			done := make(chan bool)
			count := 100

			for range count {
				go func() {
					tracker.Poison("session-1", []string{"GIT001"}, "test")
					done <- true
				}()
			}

			for range count {
				<-done
			}

			info := tracker.GetInfo("session-1")
			Expect(info.Status).To(Equal(session.StatusPoisoned))
		})

		It("handles concurrent IsPoisoned calls safely", func() {
			tracker.Poison("session-1", []string{"GIT001"}, "test")

			done := make(chan bool)
			count := 100

			for range count {
				go func() {
					poisoned, _ := tracker.IsPoisoned("session-1")
					Expect(poisoned).To(BeTrue())
					done <- true
				}()
			}

			for range count {
				<-done
			}
		})
	})

	Describe("Edge cases", func() {
		It("handles nil sessions map in loaded state", func() {
			// Write minimal JSON without sessions map
			data := `{"last_updated":"2025-12-04T10:00:00Z"}`
			err := os.WriteFile(stateFile, []byte(data), 0o600)
			Expect(err).NotTo(HaveOccurred())

			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)

			err = tracker.Load()
			Expect(err).NotTo(HaveOccurred())

			// Should be able to record without panic
			tracker.RecordCommand("session-1")

			info := tracker.GetInfo("session-1")
			Expect(info.CommandCount).To(Equal(1))
		})

		It("SessionInfo.IsPoisoned returns correct value", func() {
			tracker = session.NewTracker(
				nil,
				session.WithStateFile(stateFile),
				session.WithTimeFunc(timeFunc),
			)

			tracker.RecordCommand("session-1")
			tracker.Poison("session-2", []string{"GIT001"}, "test")

			info1 := tracker.GetInfo("session-1")
			Expect(info1.IsPoisoned()).To(BeFalse())

			info2 := tracker.GetInfo("session-2")
			Expect(info2.IsPoisoned()).To(BeTrue())
		})
	})
})

var _ = Describe("SessionState", func() {
	Describe("NewSessionState", func() {
		It("creates empty state with initialized map", func() {
			state := session.NewSessionState()
			Expect(state).NotTo(BeNil())
			Expect(state.Sessions).NotTo(BeNil())
			Expect(state.Sessions).To(BeEmpty())
		})
	})
})
