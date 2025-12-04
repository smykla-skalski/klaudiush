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
			Expect(info.PoisonCode).To(Equal("GIT001"))
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
