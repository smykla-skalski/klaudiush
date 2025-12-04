package dispatcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/dispatcher"
	"github.com/smykla-labs/klaudiush/internal/session"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("Dispatcher Session Integration", func() {
	var (
		disp    *dispatcher.Dispatcher
		reg     *validator.Registry
		tracker *session.Tracker
		log     logger.Logger
		ctx     context.Context
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		reg = validator.NewRegistry()
		ctx = context.Background()

		// Create session tracker with enabled config
		cfg := &config.SessionConfig{}
		enabled := true
		cfg.Enabled = &enabled
		tracker = session.NewTracker(cfg, session.WithLogger(log))
	})

	Context("without session tracking", func() {
		BeforeEach(func() {
			// Dispatcher without session tracker
			disp = dispatcher.NewDispatcher(reg, log)
		})

		It("should work normally without session tracker", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: "test-session-1",
				ToolInput: hook.ToolInput{
					Command: "echo test",
				},
			}

			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(BeEmpty())
		})
	})

	Context("with session tracking enabled", func() {
		var blockingValidator *mockBlockingValidator

		BeforeEach(func() {
			// Register a blocking validator
			blockingValidator = &mockBlockingValidator{
				name:      "test-blocker",
				reference: validator.RefGitNoSignoff,
			}

			reg.Register(
				blockingValidator,
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeBash),
				),
			)

			// Create dispatcher with session tracker
			disp = dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithSessionTracker(tracker),
			)
		})

		It("should allow first command when session is clean", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: "test-session-2",
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Validator).To(Equal("test-blocker"))
			Expect(errs[0].ShouldBlock).To(BeTrue())
		})

		It("should poison session after blocking error", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: "test-session-3",
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			// First command - should fail with original error
			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Reference.Code()).To(Equal("GIT001"))

			// Verify session is poisoned
			poisoned, info := tracker.IsPoisoned("test-session-3")
			Expect(poisoned).To(BeTrue())
			Expect(info.PoisonCodes).To(Equal([]string{"GIT001"}))
		})

		It("should block subsequent commands with poisoned session error", func() {
			sessionID := "test-session-4"

			// First command - blocks and poisons session
			hookCtx1 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			errs1 := disp.Dispatch(ctx, hookCtx1)
			Expect(errs1).To(HaveLen(1))
			Expect(errs1[0].Reference.Code()).To(Equal("GIT001"))

			// Second command - should immediately fail with SESS001
			hookCtx2 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "echo test",
				},
			}

			errs2 := disp.Dispatch(ctx, hookCtx2)
			Expect(errs2).To(HaveLen(1))
			Expect(errs2[0].Validator).To(Equal("session-poisoned"))
			Expect(errs2[0].Reference.Code()).To(Equal("SESS001"))
			Expect(errs2[0].Message).To(ContainSubstring("GIT001"))
		})

		It("should not poison session for non-blocking warnings", func() {
			// Register a warning validator (non-blocking)
			warningValidator := &mockWarningValidator{
				name: "test-warning",
			}

			reg.Register(
				warningValidator,
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeWrite),
				),
			)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				SessionID: "test-session-5",
				ToolInput: hook.ToolInput{
					FilePath: "test.txt",
					Content:  "test",
				},
			}

			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].ShouldBlock).To(BeFalse())

			// Session should not be poisoned
			poisoned, _ := tracker.IsPoisoned("test-session-5")
			Expect(poisoned).To(BeFalse())
		})

		It("should record command count for clean sessions", func() {
			sessionID := "test-session-6"

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					FilePath: "test.txt",
					Content:  "test",
				},
			}

			// Dispatch multiple commands
			disp.Dispatch(ctx, hookCtx)
			disp.Dispatch(ctx, hookCtx)
			disp.Dispatch(ctx, hookCtx)

			// Check command count
			info := tracker.GetInfo(sessionID)
			Expect(info).NotTo(BeNil())
			Expect(info.CommandCount).To(Equal(3))
		})

		It("should handle missing session ID gracefully", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: "", // Empty session ID
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			// Should run validators normally without session tracking
			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Reference.Code()).To(Equal("GIT001"))
		})
	})

	Context("with session tracking disabled", func() {
		BeforeEach(func() {
			// Create tracker with disabled config
			cfg := &config.SessionConfig{}
			disabled := false
			cfg.Enabled = &disabled
			tracker = session.NewTracker(cfg, session.WithLogger(log))

			// Register a blocking validator
			reg.Register(
				&mockBlockingValidator{
					name:      "test-blocker",
					reference: validator.RefGitNoSignoff,
				},
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeBash),
				),
			)

			// Create dispatcher with disabled session tracker
			disp = dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithSessionTracker(tracker),
			)
		})

		It("should not check or poison sessions when disabled", func() {
			sessionID := "test-session-7"

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			// First command - should fail with original error
			errs1 := disp.Dispatch(ctx, hookCtx)
			Expect(errs1).To(HaveLen(1))
			Expect(errs1[0].Reference.Code()).To(Equal("GIT001"))

			// Session should not be poisoned (tracking disabled)
			poisoned, _ := tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeFalse())

			// Second command - should also fail with original error (no fast-fail)
			errs2 := disp.Dispatch(ctx, hookCtx)
			Expect(errs2).To(HaveLen(1))
			Expect(errs2[0].Reference.Code()).To(Equal("GIT001"))
		})
	})

	Context("session unpoisoning", func() {
		BeforeEach(func() {
			// Register a blocking validator for git commit only
			reg.Register(
				&mockBlockingValidator{
					name:      "test-blocker",
					reference: validator.RefGitNoSignoff,
				},
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeBash),
					validator.CommandContains("git commit"),
				),
			)

			// Create dispatcher with session tracker
			disp = dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithSessionTracker(tracker),
			)
		})

		It("should unpoison session with valid acknowledgment token in env var", func() {
			sessionID := "test-session-unpoison-1"

			// First command - poisons session
			hookCtx1 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			errs1 := disp.Dispatch(ctx, hookCtx1)
			Expect(errs1).To(HaveLen(1))
			Expect(errs1[0].Reference.Code()).To(Equal("GIT001"))

			// Verify session is poisoned
			poisoned, _ := tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeTrue())

			// Second command with unpoison token in env var
			hookCtx2 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: `KLACK="SESS:GIT001" echo fixed`,
				},
			}

			errs2 := disp.Dispatch(ctx, hookCtx2)
			// Should pass (no matching validators for echo)
			Expect(errs2).To(BeEmpty())

			// Verify session is unpoisoned
			poisoned, _ = tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeFalse())
		})

		It("should unpoison session with valid acknowledgment token in comment", func() {
			sessionID := "test-session-unpoison-2"

			// First command - poisons session
			hookCtx1 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			errs1 := disp.Dispatch(ctx, hookCtx1)
			Expect(errs1).To(HaveLen(1))

			// Second command with unpoison token in comment
			hookCtx2 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: `echo fixed # SESS:GIT001`,
				},
			}

			errs2 := disp.Dispatch(ctx, hookCtx2)
			Expect(errs2).To(BeEmpty())

			// Verify session is unpoisoned
			poisoned, _ := tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeFalse())
		})

		It("should remain poisoned without acknowledgment token", func() {
			sessionID := "test-session-unpoison-3"

			// First command - poisons session
			hookCtx1 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			disp.Dispatch(ctx, hookCtx1)

			// Second command without token
			hookCtx2 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "echo no token",
				},
			}

			errs2 := disp.Dispatch(ctx, hookCtx2)
			Expect(errs2).To(HaveLen(1))
			Expect(errs2[0].Reference.Code()).To(Equal("SESS001"))

			// Session should still be poisoned
			poisoned, _ := tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeTrue())
		})

		It("should remain poisoned with wrong acknowledgment code", func() {
			sessionID := "test-session-unpoison-4"

			// First command - poisons session with GIT001
			hookCtx1 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: "git commit",
				},
			}

			disp.Dispatch(ctx, hookCtx1)

			// Second command with wrong code
			hookCtx2 := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: `KLACK="SESS:GIT999" echo wrong`,
				},
			}

			errs2 := disp.Dispatch(ctx, hookCtx2)
			Expect(errs2).To(HaveLen(1))
			Expect(errs2[0].Reference.Code()).To(Equal("SESS001"))
		})

		It("should require all codes for multi-code poison", func() {
			sessionID := "test-session-unpoison-5"

			// Manually poison with multiple codes
			tracker.Poison(sessionID, []string{"GIT001", "GIT002"}, "multiple errors")

			// Verify session is poisoned with multiple codes
			poisoned, info := tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeTrue())
			Expect(info.PoisonCodes).To(Equal([]string{"GIT001", "GIT002"}))

			// Try with only one code - should fail (partial acknowledgment)
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: `KLACK="SESS:GIT001" echo partial`,
				},
			}

			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Reference.Code()).To(Equal("SESS001"))

			// Still poisoned
			poisoned, _ = tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeTrue())
		})

		It("should unpoison with all codes acknowledged", func() {
			sessionID := "test-session-unpoison-6"

			// Manually poison with multiple codes
			tracker.Poison(sessionID, []string{"GIT001", "GIT002"}, "multiple errors")

			// Acknowledge all codes
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: `KLACK="SESS:GIT001,GIT002" echo all acked`,
				},
			}

			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(BeEmpty())

			// Should be unpoisoned
			poisoned, _ := tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeFalse())
		})

		It("should allow acknowledging superset of codes", func() {
			sessionID := "test-session-unpoison-7"

			// Manually poison with one code
			tracker.Poison(sessionID, []string{"GIT001"}, "single error")

			// Acknowledge more codes than needed (superset)
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				SessionID: sessionID,
				ToolInput: hook.ToolInput{
					Command: `KLACK="SESS:GIT001,GIT002,GIT003" echo superset`,
				},
			}

			errs := disp.Dispatch(ctx, hookCtx)
			Expect(errs).To(BeEmpty())

			// Should be unpoisoned
			poisoned, _ := tracker.IsPoisoned(sessionID)
			Expect(poisoned).To(BeFalse())
		})
	})
})

// mockWarningValidator is a test validator that always warns (non-blocking).
type mockWarningValidator struct {
	name string
}

func (v *mockWarningValidator) Name() string {
	return v.name
}

func (*mockWarningValidator) Validate(_ context.Context, _ *hook.Context) *validator.Result {
	return &validator.Result{
		Passed:      false,
		Message:     "validation warning",
		ShouldBlock: false,
	}
}

func (*mockWarningValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

var _ = Describe("ValidationError", func() {
	Describe("Error", func() {
		It("returns formatted error with message", func() {
			err := &dispatcher.ValidationError{
				Validator: "test-validator",
				Message:   "validation failed",
			}
			Expect(err.Error()).To(Equal("test-validator: validation failed"))
		})

		It("returns just validator name when no message", func() {
			err := &dispatcher.ValidationError{
				Validator: "test-validator",
				Message:   "",
			}
			Expect(err.Error()).To(Equal("test-validator"))
		})
	})
})

var _ = Describe("FormatErrors", func() {
	It("formats errors with fix hints", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test-validator",
				Message:     "Something went wrong",
				ShouldBlock: true,
				FixHint:     "Try doing X instead",
				Reference:   "https://klaudiu.sh/GIT001",
			},
		}

		result := dispatcher.FormatErrors(errs)
		Expect(result).To(ContainSubstring("Something went wrong"))
		Expect(result).To(ContainSubstring("Fix: Try doing X instead"))
		Expect(result).To(ContainSubstring("Reference: https://klaudiu.sh/GIT001"))
	})

	It("formats errors with details", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test-validator",
				Message:     "Validation error",
				ShouldBlock: true,
				Details: map[string]string{
					"detail1": "first detail\nsecond line",
				},
			},
		}

		result := dispatcher.FormatErrors(errs)
		Expect(result).To(ContainSubstring("Validation error"))
		Expect(result).To(ContainSubstring("first detail"))
		Expect(result).To(ContainSubstring("second line"))
	})

	It("separates blocking errors and warnings", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "blocker",
				Message:     "Blocking error",
				ShouldBlock: true,
			},
			{
				Validator:   "warner",
				Message:     "Warning message",
				ShouldBlock: false,
			},
		}

		result := dispatcher.FormatErrors(errs)
		Expect(result).To(ContainSubstring("Validation Failed:"))
		Expect(result).To(ContainSubstring("blocker"))
		Expect(result).To(ContainSubstring("Warnings:"))
		Expect(result).To(ContainSubstring("warner"))
	})

	It("returns empty string for no errors", func() {
		result := dispatcher.FormatErrors([]*dispatcher.ValidationError{})
		Expect(result).To(BeEmpty())
	})
})

var _ = Describe("Dispatcher constructors", func() {
	var (
		reg *validator.Registry
		log logger.Logger
	)

	BeforeEach(func() {
		reg = validator.NewRegistry()
		log = logger.NewNoOpLogger()
	})

	Describe("NewDispatcherWithExecutor", func() {
		It("creates dispatcher with custom executor", func() {
			executor := dispatcher.NewSequentialExecutor(log)
			d := dispatcher.NewDispatcherWithExecutor(reg, log, executor)
			Expect(d).NotTo(BeNil())
		})
	})

	Describe("WithSessionAuditLogger", func() {
		It("sets session audit logger", func() {
			mockAuditLogger := &mockSessionAuditLogger{}
			d := dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithSessionAuditLogger(mockAuditLogger),
			)
			Expect(d).NotTo(BeNil())
		})

		It("ignores nil audit logger", func() {
			d := dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithSessionAuditLogger(nil),
			)
			Expect(d).NotTo(BeNil())
		})
	})
})

var _ = Describe("Dispatcher with Session Audit Logger", func() {
	var (
		disp        *dispatcher.Dispatcher
		reg         *validator.Registry
		tracker     *session.Tracker
		auditLogger *mockSessionAuditLogger
		log         logger.Logger
		ctx         context.Context
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		reg = validator.NewRegistry()
		ctx = context.Background()

		// Create session tracker
		cfg := &config.SessionConfig{}
		enabled := true
		cfg.Enabled = &enabled
		tracker = session.NewTracker(cfg, session.WithLogger(log))

		// Create mock audit logger
		auditLogger = &mockSessionAuditLogger{enabled: true}
	})

	It("logs audit entry on session poison", func() {
		// Register a blocking validator
		reg.Register(
			&mockBlockingValidator{
				name:      "test-blocker",
				reference: validator.RefGitNoSignoff,
			},
			validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.ToolTypeIs(hook.ToolTypeBash),
			),
		)

		disp = dispatcher.NewDispatcherWithOptions(
			reg,
			log,
			dispatcher.NewSequentialExecutor(log),
			dispatcher.WithSessionTracker(tracker),
			dispatcher.WithSessionAuditLogger(auditLogger),
		)

		hookCtx := &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			SessionID: "audit-test-1",
			ToolInput: hook.ToolInput{
				Command: "git commit",
			},
		}

		disp.Dispatch(ctx, hookCtx)

		// Should have logged a poison entry
		Expect(auditLogger.entries).To(HaveLen(1))
		Expect(auditLogger.entries[0].Action).To(Equal(session.AuditActionPoison))
	})

	It("logs audit entry on session unpoison", func() {
		// Register a blocking validator for git commit only
		reg.Register(
			&mockBlockingValidator{
				name:      "test-blocker",
				reference: validator.RefGitNoSignoff,
			},
			validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.ToolTypeIs(hook.ToolTypeBash),
				validator.CommandContains("git commit"),
			),
		)

		disp = dispatcher.NewDispatcherWithOptions(
			reg,
			log,
			dispatcher.NewSequentialExecutor(log),
			dispatcher.WithSessionTracker(tracker),
			dispatcher.WithSessionAuditLogger(auditLogger),
		)

		sessionID := "audit-test-2"

		// Poison the session
		hookCtx1 := &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			SessionID: sessionID,
			ToolInput: hook.ToolInput{
				Command: "git commit",
			},
		}
		disp.Dispatch(ctx, hookCtx1)

		// Clear entries to check unpoison
		auditLogger.entries = nil

		// Unpoison the session
		hookCtx2 := &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			SessionID: sessionID,
			ToolInput: hook.ToolInput{
				Command: `KLACK="SESS:GIT001" echo unpoison`,
			},
		}
		disp.Dispatch(ctx, hookCtx2)

		// Should have logged an unpoison entry
		Expect(auditLogger.entries).To(HaveLen(1))
		Expect(auditLogger.entries[0].Action).To(Equal(session.AuditActionUnpoison))
	})

	It("does not log when audit logger is disabled", func() {
		auditLogger.enabled = false

		reg.Register(
			&mockBlockingValidator{
				name:      "test-blocker",
				reference: validator.RefGitNoSignoff,
			},
			validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.ToolTypeIs(hook.ToolTypeBash),
			),
		)

		disp = dispatcher.NewDispatcherWithOptions(
			reg,
			log,
			dispatcher.NewSequentialExecutor(log),
			dispatcher.WithSessionTracker(tracker),
			dispatcher.WithSessionAuditLogger(auditLogger),
		)

		hookCtx := &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			SessionID: "audit-test-3",
			ToolInput: hook.ToolInput{
				Command: "git commit",
			},
		}

		disp.Dispatch(ctx, hookCtx)

		// Should not have logged anything
		Expect(auditLogger.entries).To(BeEmpty())
	})
})

// mockSessionAuditLogger is a mock implementation of SessionAuditLogger.
type mockSessionAuditLogger struct {
	entries []*session.AuditEntry
	enabled bool
}

func (m *mockSessionAuditLogger) Log(entry *session.AuditEntry) error {
	if m.enabled {
		m.entries = append(m.entries, entry)
	}

	return nil
}

func (m *mockSessionAuditLogger) IsEnabled() bool {
	return m.enabled
}
