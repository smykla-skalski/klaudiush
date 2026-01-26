package dispatcher_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/exceptions"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// mockBlockingValidator is a test validator that always blocks with a reference.
type mockBlockingValidator struct {
	name      string
	reference validator.Reference
}

func (v *mockBlockingValidator) Name() string {
	return v.name
}

func (v *mockBlockingValidator) Validate(_ context.Context, _ *hook.Context) *validator.Result {
	return &validator.Result{
		Passed:      false,
		Message:     "validation blocked",
		ShouldBlock: true,
		Reference:   v.reference,
	}
}

func (*mockBlockingValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

var _ = Describe("Dispatcher Exception Integration", func() {
	var (
		disp    *dispatcher.Dispatcher
		reg     *validator.Registry
		tempDir string
		log     logger.Logger
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "dispatcher-integration-*")
		Expect(err).NotTo(HaveOccurred())

		log = logger.NewNoOpLogger()
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Context("without exception checker", func() {
		BeforeEach(func() {
			reg = validator.NewRegistry()

			// Register a blocking validator
			reg.Register(
				&mockBlockingValidator{
					name:      "git.push",
					reference: "https://klaudiu.sh/GIT022",
				},
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeBash),
				),
			)

			disp = dispatcher.NewDispatcher(reg, log)
		})

		It("blocks validation without exception token", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main",
				},
			}

			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeTrue())
		})

		It("still blocks even with exception token when no checker configured", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main # EXC:GIT022:Emergency+hotfix",
				},
			}

			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeTrue())
		})
	})

	Context("with exception checker", func() {
		BeforeEach(func() {
			reg = validator.NewRegistry()

			// Register a blocking validator
			reg.Register(
				&mockBlockingValidator{
					name:      "git.push",
					reference: "https://klaudiu.sh/GIT022",
				},
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeBash),
				),
			)

			// Create exception handler with temp files
			stateFile := filepath.Join(tempDir, "state.json")
			auditFile := filepath.Join(tempDir, "audit.jsonl")

			handler := exceptions.NewHandler(&config.ExceptionsConfig{
				RateLimit: &config.ExceptionRateLimitConfig{
					StateFile: stateFile,
				},
				Audit: &config.ExceptionAuditConfig{
					LogFile: auditFile,
				},
			})

			checker := dispatcher.NewExceptionChecker(handler)

			disp = dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithExceptionChecker(checker),
			)
		})

		It("still blocks without exception token", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main",
				},
			}

			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeTrue())
		})

		It("bypasses validation with matching exception token", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main # EXC:GIT022:Emergency+hotfix",
				},
			}

			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeFalse()) // Converted to warning
			Expect(errors[0].Message).To(ContainSubstring("BYPASSED"))
		})

		It("blocks with mismatched exception token", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main # EXC:SEC001:wrong+code",
				},
			}

			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeTrue())
		})

		It("writes to audit log when exception is used", func() {
			auditFile := filepath.Join(tempDir, "audit.jsonl")

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main # EXC:GIT022:Emergency+hotfix",
				},
			}

			_ = disp.Dispatch(context.Background(), hookCtx)

			// Verify audit file was created and has content
			content, err := os.ReadFile(auditFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("GIT022"))
			Expect(string(content)).To(ContainSubstring("Emergency hotfix"))
		})
	})

	Context("with disabled exception system", func() {
		BeforeEach(func() {
			reg = validator.NewRegistry()

			// Register a blocking validator
			reg.Register(
				&mockBlockingValidator{
					name:      "git.push",
					reference: "https://klaudiu.sh/GIT022",
				},
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeBash),
				),
			)

			// Create disabled exception handler
			enabled := false
			handler := exceptions.NewHandler(&config.ExceptionsConfig{
				Enabled: &enabled,
			})

			checker := dispatcher.NewExceptionChecker(handler)

			disp = dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithExceptionChecker(checker),
			)
		})

		It("blocks even with exception token when system is disabled", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main # EXC:GIT022:Emergency+hotfix",
				},
			}

			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeTrue())
		})
	})

	Context("with rate limiting", func() {
		BeforeEach(func() {
			reg = validator.NewRegistry()

			// Register a blocking validator
			reg.Register(
				&mockBlockingValidator{
					name:      "git.push",
					reference: "https://klaudiu.sh/GIT022",
				},
				validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.ToolTypeIs(hook.ToolTypeBash),
				),
			)

			// Create exception handler with very low rate limit
			stateFile := filepath.Join(tempDir, "state.json")
			auditFile := filepath.Join(tempDir, "audit.jsonl")
			maxHour := 2

			handler := exceptions.NewHandler(&config.ExceptionsConfig{
				RateLimit: &config.ExceptionRateLimitConfig{
					StateFile:  stateFile,
					MaxPerHour: &maxHour,
				},
				Audit: &config.ExceptionAuditConfig{
					LogFile: auditFile,
				},
			})

			checker := dispatcher.NewExceptionChecker(handler)

			disp = dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithExceptionChecker(checker),
			)
		})

		It("allows exceptions within rate limit", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main # EXC:GIT022:first",
				},
			}

			// First exception should pass
			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeFalse())

			// Second exception should also pass
			hookCtx.ToolInput.Command = "git push origin main # EXC:GIT022:second"
			errors = disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeFalse())
		})

		It("blocks when rate limit is exceeded", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main # EXC:GIT022:reason",
				},
			}

			// Use up rate limit (2 per hour)
			_ = disp.Dispatch(context.Background(), hookCtx)
			_ = disp.Dispatch(context.Background(), hookCtx)

			// Third should be blocked
			errors := disp.Dispatch(context.Background(), hookCtx)
			Expect(errors).To(HaveLen(1))
			Expect(errors[0].ShouldBlock).To(BeTrue()) // Rate limit exceeded
		})
	})

	Context("ShouldBlock helper", func() {
		It("returns true when any error blocks", func() {
			errors := []*dispatcher.ValidationError{
				{Message: "warning", ShouldBlock: false},
				{Message: "error", ShouldBlock: true},
			}
			Expect(dispatcher.ShouldBlock(errors)).To(BeTrue())
		})

		It("returns false when no errors block", func() {
			errors := []*dispatcher.ValidationError{
				{Message: "warning1", ShouldBlock: false},
				{Message: "warning2", ShouldBlock: false},
			}
			Expect(dispatcher.ShouldBlock(errors)).To(BeFalse())
		})

		It("returns false for empty errors", func() {
			Expect(dispatcher.ShouldBlock(nil)).To(BeFalse())
			Expect(dispatcher.ShouldBlock([]*dispatcher.ValidationError{})).To(BeFalse())
		})
	})

	Context("FormatErrors helper", func() {
		It("formats blocking errors with red emoji", func() {
			errors := []*dispatcher.ValidationError{
				{
					Validator:   "git.push",
					Message:     "cannot push",
					ShouldBlock: true,
				},
			}
			formatted := dispatcher.FormatErrors(errors)
			Expect(formatted).To(ContainSubstring("❌"))
			Expect(formatted).To(ContainSubstring("cannot push"))
		})

		It("formats warnings with warning emoji", func() {
			errors := []*dispatcher.ValidationError{
				{
					Validator:   "git.push",
					Message:     "warning message",
					ShouldBlock: false,
				},
			}
			formatted := dispatcher.FormatErrors(errors)
			Expect(formatted).To(ContainSubstring("⚠️"))
			Expect(formatted).To(ContainSubstring("warning message"))
		})

		It("returns empty string for no errors", func() {
			Expect(dispatcher.FormatErrors(nil)).To(BeEmpty())
			Expect(dispatcher.FormatErrors([]*dispatcher.ValidationError{})).To(BeEmpty())
		})
	})
})
