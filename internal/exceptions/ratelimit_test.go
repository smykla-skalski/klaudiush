package exceptions_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/exceptions"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("RateLimiter", func() {
	var (
		limiter     *exceptions.RateLimiter
		tempDir     string
		stateFile   string
		currentTime time.Time
		timeFunc    func() time.Time
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "ratelimit-test-*")
		Expect(err).NotTo(HaveOccurred())

		stateFile = filepath.Join(tempDir, "state.json")
		currentTime = time.Date(2025, 11, 29, 10, 30, 0, 0, time.UTC)
		timeFunc = func() time.Time { return currentTime }
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("NewRateLimiter", func() {
		It("creates limiter with nil config", func() {
			l := exceptions.NewRateLimiter(nil, nil)
			Expect(l).NotTo(BeNil())
		})

		It("creates limiter with config", func() {
			maxHour := 5
			l := exceptions.NewRateLimiter(
				&config.ExceptionRateLimitConfig{
					MaxPerHour: &maxHour,
				},
				nil,
			)
			Expect(l).NotTo(BeNil())
		})

		It("accepts custom state file", func() {
			l := exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile("/custom/path.json"),
			)
			Expect(l).NotTo(BeNil())
		})

		It("accepts custom time function", func() {
			customTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			l := exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithTimeFunc(func() time.Time { return customTime }),
			)
			Expect(l).NotTo(BeNil())
		})
	})

	Describe("Check", func() {
		Context("when rate limiting is disabled", func() {
			BeforeEach(func() {
				enabled := false
				limiter = exceptions.NewRateLimiter(
					&config.ExceptionRateLimitConfig{
						Enabled: &enabled,
					},
					nil,
					exceptions.WithStateFile(stateFile),
					exceptions.WithTimeFunc(timeFunc),
				)
			})

			It("always allows", func() {
				result := limiter.Check("GIT022")
				Expect(result.Allowed).To(BeTrue())
				Expect(result.Reason).To(ContainSubstring("disabled"))
				Expect(result.GlobalHourlyRemaining).To(Equal(-1))
			})
		})

		Context("with default limits", func() {
			BeforeEach(func() {
				limiter = exceptions.NewRateLimiter(
					nil,
					nil,
					exceptions.WithStateFile(stateFile),
					exceptions.WithTimeFunc(timeFunc),
				)
			})

			It("allows within limits", func() {
				result := limiter.Check("GIT022")
				Expect(result.Allowed).To(BeTrue())
				Expect(result.GlobalHourlyRemaining).To(Equal(config.DefaultRateLimitPerHour))
				Expect(result.GlobalDailyRemaining).To(Equal(config.DefaultRateLimitPerDay))
			})
		})

		Context("with custom global limits", func() {
			BeforeEach(func() {
				maxHour := 2
				maxDay := 5
				limiter = exceptions.NewRateLimiter(
					&config.ExceptionRateLimitConfig{
						MaxPerHour: &maxHour,
						MaxPerDay:  &maxDay,
					},
					nil,
					exceptions.WithStateFile(stateFile),
					exceptions.WithTimeFunc(timeFunc),
				)
			})

			It("allows when under hourly limit", func() {
				_ = limiter.Record("GIT022")
				result := limiter.Check("GIT022")
				Expect(result.Allowed).To(BeTrue())
				Expect(result.GlobalHourlyRemaining).To(Equal(1))
			})

			It("denies when hourly limit reached", func() {
				_ = limiter.Record("GIT022")
				_ = limiter.Record("SEC001")
				result := limiter.Check("FILE001")
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("hourly limit"))
				Expect(result.GlobalHourlyRemaining).To(Equal(0))
			})

			It("denies when daily limit reached", func() {
				maxHour := 10
				maxDay := 3
				limiter = exceptions.NewRateLimiter(
					&config.ExceptionRateLimitConfig{
						MaxPerHour: &maxHour,
						MaxPerDay:  &maxDay,
					},
					nil,
					exceptions.WithStateFile(stateFile),
					exceptions.WithTimeFunc(timeFunc),
				)

				_ = limiter.Record("GIT022")
				_ = limiter.Record("SEC001")
				_ = limiter.Record("FILE001")
				result := limiter.Check("SHELL001")
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("daily limit"))
			})
		})

		Context("with per-error-code limits", func() {
			BeforeEach(func() {
				maxCodeHour := 1
				maxCodeDay := 2
				limiter = exceptions.NewRateLimiter(
					&config.ExceptionRateLimitConfig{},
					&config.ExceptionsConfig{
						Policies: map[string]*config.ExceptionPolicyConfig{
							"GIT022": {
								MaxPerHour: &maxCodeHour,
								MaxPerDay:  &maxCodeDay,
							},
						},
					},
					exceptions.WithStateFile(stateFile),
					exceptions.WithTimeFunc(timeFunc),
				)
			})

			It("allows different error codes", func() {
				_ = limiter.Record("GIT022")
				result := limiter.Check("SEC001")
				Expect(result.Allowed).To(BeTrue())
			})

			It("denies when code-specific hourly limit reached", func() {
				_ = limiter.Record("GIT022")
				result := limiter.Check("GIT022")
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("hourly limit exceeded for GIT022"))
				Expect(result.ErrorCodeHourlyRemaining).To(Equal(0))
			})

			It("reports remaining quota per error code", func() {
				result := limiter.Check("GIT022")
				Expect(result.Allowed).To(BeTrue())
				Expect(result.ErrorCodeHourlyRemaining).To(Equal(1))
				Expect(result.ErrorCodeDailyRemaining).To(Equal(2))
			})
		})
	})

	Describe("Record", func() {
		BeforeEach(func() {
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
		})

		It("increments global counters", func() {
			err := limiter.Record("GIT022")
			Expect(err).NotTo(HaveOccurred())

			state := limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(1))
			Expect(state.GlobalDailyCount).To(Equal(1))
		})

		It("increments per-code counters", func() {
			err := limiter.Record("GIT022")
			Expect(err).NotTo(HaveOccurred())
			err = limiter.Record("GIT022")
			Expect(err).NotTo(HaveOccurred())
			err = limiter.Record("SEC001")
			Expect(err).NotTo(HaveOccurred())

			state := limiter.GetState()
			Expect(state.HourlyUsage["GIT022"]).To(Equal(2))
			Expect(state.HourlyUsage["SEC001"]).To(Equal(1))
			Expect(state.DailyUsage["GIT022"]).To(Equal(2))
			Expect(state.DailyUsage["SEC001"]).To(Equal(1))
		})

		It("updates last modified time", func() {
			err := limiter.Record("GIT022")
			Expect(err).NotTo(HaveOccurred())

			state := limiter.GetState()
			Expect(state.LastUpdated).To(Equal(currentTime))
		})
	})

	Describe("Load and Save", func() {
		BeforeEach(func() {
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
		})

		It("loads fresh state when file doesn't exist", func() {
			err := limiter.Load()
			Expect(err).NotTo(HaveOccurred())

			state := limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(0))
		})

		It("saves state to file", func() {
			_ = limiter.Record("GIT022")
			_ = limiter.Record("SEC001")

			err := limiter.Save()
			Expect(err).NotTo(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(stateFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("loads previously saved state", func() {
			_ = limiter.Record("GIT022")
			_ = limiter.Record("SEC001")
			err := limiter.Save()
			Expect(err).NotTo(HaveOccurred())

			// Create new limiter and load
			limiter2 := exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
			err = limiter2.Load()
			Expect(err).NotTo(HaveOccurred())

			state := limiter2.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(2))
			Expect(state.GlobalDailyCount).To(Equal(2))
			Expect(state.HourlyUsage["GIT022"]).To(Equal(1))
		})

		It("handles corrupted state file gracefully", func() {
			err := os.WriteFile(stateFile, []byte("not valid json"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = limiter.Load()
			Expect(err).NotTo(HaveOccurred())

			// Should have fresh state
			state := limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(0))
		})

		It("creates directory if needed", func() {
			nestedPath := filepath.Join(tempDir, "nested", "dir", "state.json")
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(nestedPath),
				exceptions.WithTimeFunc(timeFunc),
			)

			_ = limiter.Record("GIT022")
			err := limiter.Save()
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(nestedPath)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Reset", func() {
		BeforeEach(func() {
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
		})

		It("clears all counters", func() {
			_ = limiter.Record("GIT022")
			_ = limiter.Record("SEC001")

			limiter.Reset()

			state := limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(0))
			Expect(state.GlobalDailyCount).To(Equal(0))
			Expect(state.HourlyUsage).To(BeEmpty())
			Expect(state.DailyUsage).To(BeEmpty())
		})
	})

	Describe("GetState", func() {
		BeforeEach(func() {
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
		})

		It("returns a copy of state", func() {
			_ = limiter.Record("GIT022")

			state1 := limiter.GetState()
			state2 := limiter.GetState()

			// Modify state1
			state1.HourlyUsage["SEC001"] = 100

			// state2 should not be affected
			Expect(state2.HourlyUsage["SEC001"]).To(Equal(0))
		})
	})

	Describe("Window expiry", func() {
		BeforeEach(func() {
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
		})

		It("resets hourly counter when hour changes", func() {
			_ = limiter.Record("GIT022")
			_ = limiter.Record("SEC001")

			state := limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(2))
			Expect(state.GlobalDailyCount).To(Equal(2))

			// Advance time by 1 hour
			currentTime = currentTime.Add(time.Hour)

			// Check should trigger reset
			result := limiter.Check("GIT022")
			Expect(result.Allowed).To(BeTrue())

			state = limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(0))
			Expect(state.GlobalDailyCount).To(Equal(2)) // Daily should persist
			Expect(state.HourlyUsage).To(BeEmpty())
		})

		It("resets daily counter when day changes", func() {
			_ = limiter.Record("GIT022")

			state := limiter.GetState()
			Expect(state.GlobalDailyCount).To(Equal(1))

			// Advance time by 24 hours
			currentTime = currentTime.Add(24 * time.Hour)

			// Check should trigger reset
			_ = limiter.Check("GIT022")

			state = limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(0))
			Expect(state.GlobalDailyCount).To(Equal(0))
			Expect(state.DailyUsage).To(BeEmpty())
		})

		It("persists window start times correctly", func() {
			_ = limiter.Record("GIT022")
			err := limiter.Save()
			Expect(err).NotTo(HaveOccurred())

			// Advance time by 20 minutes (still same hour: 10:30 -> 10:50)
			currentTime = currentTime.Add(20 * time.Minute)

			limiter2 := exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
			err = limiter2.Load()
			Expect(err).NotTo(HaveOccurred())

			state := limiter2.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(1)) // Should persist
		})

		It("resets counters when loading old state", func() {
			_ = limiter.Record("GIT022")
			err := limiter.Save()
			Expect(err).NotTo(HaveOccurred())

			// Advance time by 2 hours
			currentTime = currentTime.Add(2 * time.Hour)

			limiter2 := exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
			err = limiter2.Load()
			Expect(err).NotTo(HaveOccurred())

			state := limiter2.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(0)) // Should be reset
			Expect(state.GlobalDailyCount).To(Equal(1))  // Should persist
		})
	})

	Describe("Home directory expansion", func() {
		It("expands ~ in state file path", func() {
			home, err := os.UserHomeDir()
			Expect(err).NotTo(HaveOccurred())

			// Create limiter with ~ path
			homePath := filepath.Join(home, ".klaudiush-test", "state.json")
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile("~/.klaudiush-test/state.json"),
				exceptions.WithTimeFunc(timeFunc),
			)

			_ = limiter.Record("GIT022")
			err = limiter.Save()
			Expect(err).NotTo(HaveOccurred())

			// Verify file was created at expanded path
			_, err = os.Stat(homePath)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			_ = os.RemoveAll(filepath.Join(home, ".klaudiush-test"))
		})
	})

	Describe("Concurrent access", func() {
		BeforeEach(func() {
			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)
		})

		It("handles concurrent Record calls safely", func() {
			done := make(chan bool)
			count := 100

			for range count {
				go func() {
					_ = limiter.Record("GIT022")
					done <- true
				}()
			}

			for range count {
				<-done
			}

			state := limiter.GetState()
			Expect(state.GlobalHourlyCount).To(Equal(count))
		})

		It("handles concurrent Check calls safely", func() {
			done := make(chan bool)
			count := 100

			for range count {
				go func() {
					result := limiter.Check("GIT022")
					Expect(result).NotTo(BeNil())
					done <- true
				}()
			}

			for range count {
				<-done
			}
		})
	})

	Describe("Edge cases", func() {
		It("handles zero limits as unlimited", func() {
			zero := 0
			limiter = exceptions.NewRateLimiter(
				&config.ExceptionRateLimitConfig{
					MaxPerHour: &zero,
					MaxPerDay:  &zero,
				},
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)

			// Record many
			for range 100 {
				_ = limiter.Record("GIT022")
			}

			result := limiter.Check("GIT022")
			Expect(result.Allowed).To(BeTrue())
			Expect(result.GlobalHourlyRemaining).To(Equal(-1)) // Unlimited
		})

		It("handles nil maps in loaded state", func() {
			// Write minimal JSON without maps
			data := `{"global_hourly_count":5,"global_daily_count":10,"hour_start_time":"2025-11-29T10:00:00Z","day_start_time":"2025-11-29T00:00:00Z","last_updated":"2025-11-29T10:30:00Z"}`
			err := os.WriteFile(stateFile, []byte(data), 0o600)
			Expect(err).NotTo(HaveOccurred())

			limiter = exceptions.NewRateLimiter(
				nil,
				nil,
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)

			err = limiter.Load()
			Expect(err).NotTo(HaveOccurred())

			// Should be able to record without panic
			err = limiter.Record("GIT022")
			Expect(err).NotTo(HaveOccurred())

			state := limiter.GetState()
			Expect(state.HourlyUsage["GIT022"]).To(Equal(1))
		})

		It("applies both global and code limits", func() {
			globalMaxHour := 5
			codeMaxHour := 2
			limiter = exceptions.NewRateLimiter(
				&config.ExceptionRateLimitConfig{
					MaxPerHour: &globalMaxHour,
				},
				&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"GIT022": {
							MaxPerHour: &codeMaxHour,
						},
					},
				},
				exceptions.WithStateFile(stateFile),
				exceptions.WithTimeFunc(timeFunc),
			)

			// Use up code limit
			_ = limiter.Record("GIT022")
			_ = limiter.Record("GIT022")

			// GIT022 should be denied
			result := limiter.Check("GIT022")
			Expect(result.Allowed).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("hourly limit exceeded for GIT022"))

			// But other codes should still be allowed
			result = limiter.Check("SEC001")
			Expect(result.Allowed).To(BeTrue())
		})
	})
})
