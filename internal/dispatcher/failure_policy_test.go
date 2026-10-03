package dispatcher_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// funcValidator runs fn as its Validate.
type funcValidator struct {
	name string
	fn   func(ctx context.Context) *validator.Result
}

func (v *funcValidator) Name() string { return v.name }

func (*funcValidator) Category() validator.ValidatorCategory { return validator.CategoryCPU }

func (v *funcValidator) Validate(ctx context.Context, _ *hook.Context) *validator.Result {
	return v.fn(ctx)
}

func panicking(name string) *funcValidator {
	return &funcValidator{name: name, fn: func(context.Context) *validator.Result {
		panic("boom")
	}}
}

func returning(name string, result *validator.Result) *funcValidator {
	return &funcValidator{name: name, fn: func(context.Context) *validator.Result {
		return result
	}}
}

// passingAfter passes once ctx ends, like a check that ignored its deadline.
func passingAfter(name string) *funcValidator {
	return &funcValidator{name: name, fn: func(ctx context.Context) *validator.Result {
		<-ctx.Done()

		return validator.Pass()
	}}
}

var _ = Describe("Failure handling", func() {
	var (
		log     logger.Logger
		hookCtx *hook.Context
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		hookCtx = &hook.Context{
			Event:     hook.CanonicalEventBeforeTool,
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: "ls"},
		}
	})

	executors := map[string]func() dispatcher.Executor{
		"sequential": func() dispatcher.Executor { return dispatcher.NewSequentialExecutor(log) },
		"parallel": func() dispatcher.Executor {
			return dispatcher.NewParallelExecutor(log, nil)
		},
	}

	for name, newExecutor := range executors {
		Context(name+" executor", func() {
			It("reports a panicking validator as crashed and keeps running the rest", func() {
				runs := newExecutor().Run(context.Background(), hookCtx, []validator.Validator{
					panicking("validate-boom"),
					returning("validate-ok", validator.Pass()),
				})

				Expect(runs).To(HaveLen(2))

				for _, run := range runs {
					if run.Validator.Name() != "validate-boom" {
						Expect(run.Result.Passed).To(BeTrue())

						continue
					}

					Expect(run.Result.Unavailable).To(BeTrue())
					Expect(run.Result.UnavailableReason).To(Equal(validator.ReasonPanic))
					Expect(run.Result.Message).To(ContainSubstring("boom crashed: boom"))
				}
			})

			It("reports a validator that returned nothing", func() {
				runs := newExecutor().Run(context.Background(), hookCtx, []validator.Validator{
					returning("validate-nil", nil),
				})

				Expect(runs).To(HaveLen(1))
				Expect(runs[0].Result.Unavailable).To(BeTrue())
				Expect(runs[0].Result.UnavailableReason).To(Equal(validator.ReasonError))
			})

			It("never records a pass that came after the deadline", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()

				runs := newExecutor().Run(ctx, hookCtx, []validator.Validator{
					passingAfter("validate-slow"),
				})

				Expect(runs).To(HaveLen(1))
				Expect(runs[0].Result.Passed).To(BeFalse())
				Expect(runs[0].Result.UnavailableReason).To(Equal(validator.ReasonTimeout))
				Expect(runs[0].Result.Message).To(ContainSubstring("slow did not finish"))
			})

			It("reports validators that never started once the deadline passed", func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
				defer cancel()

				<-ctx.Done()

				runs := newExecutor().Run(ctx, hookCtx, []validator.Validator{
					returning("validate-a", validator.Pass()),
					returning("validate-b", validator.Pass()),
				})

				Expect(runs).To(HaveLen(2))

				for _, run := range runs {
					Expect(run.Result.UnavailableReason).To(Equal(validator.ReasonTimeout))
					Expect(run.Result.Message).To(ContainSubstring("did not run"))
				}
			})

			It("keeps a violation found before the deadline", func() {
				ctx, cancel := context.WithCancel(context.Background())

				failing := &funcValidator{
					name: "validate-fail",
					fn: func(context.Context) *validator.Result {
						cancel()

						return validator.Fail("bad")
					},
				}

				runs := newExecutor().Run(ctx, hookCtx, []validator.Validator{failing})

				Expect(runs).To(HaveLen(1))
				Expect(runs[0].Result.ShouldBlock).To(BeTrue())
				Expect(runs[0].Result.Unavailable).To(BeFalse())
			})
		})
	}

	Describe("dispatcher policy", func() {
		dispatch := func(policy *failpolicy.Policy, vs ...validator.Validator) dispatcher.Outcome {
			reg := validator.NewRegistry()
			for _, v := range vs {
				reg.Register(v, validator.Always())
			}

			disp := dispatcher.NewDispatcherWithOptions(
				reg,
				log,
				dispatcher.NewSequentialExecutor(log),
				dispatcher.WithFailurePolicy(policy),
			)

			return disp.DispatchWithChecks(context.Background(), hookCtx)
		}

		timedOut := func(name string, blocks bool) validator.Validator {
			result := validator.Unavailable(validator.ReasonTimeout, name+" timed out")
			result.ShouldBlock = blocks

			return returning(name, result)
		}

		missing := func(name string) validator.Validator {
			return returning(
				name,
				validator.Unavailable(validator.ReasonMissingTool, "not installed"),
			)
		}

		It("keeps each check's choice and drops missing tools without a policy", func() {
			outcome := dispatch(nil,
				timedOut("validate-a", false),
				timedOut("validate-b", true),
				missing("validate-c"),
			)

			Expect(outcome.Errors).To(HaveLen(2))
			Expect(outcome.Errors[0].ShouldBlock).To(BeFalse())
			Expect(outcome.Errors[1].ShouldBlock).To(BeTrue())
			Expect(outcome.Errors[1].UnavailableReason).To(Equal(validator.ReasonTimeout))
			Expect(outcome.Checks).To(BeEmpty())
		})

		It("blocks every unavailable check in block mode", func() {
			policy := failpolicy.New(
				&config.FailurePolicyConfig{Mode: "block", MissingTools: "warn"},
			)
			outcome := dispatch(policy, timedOut("validate-a", false), missing("validate-c"))

			Expect(outcome.Errors).To(HaveLen(2))
			Expect(outcome.Errors[0].ShouldBlock).To(BeTrue())
			Expect(outcome.Errors[1].ShouldBlock).To(BeFalse())
			Expect(dispatcher.ShouldBlock(outcome.Errors)).To(BeTrue())
		})

		It("blocks a critical check whose tool is missing", func() {
			policy := failpolicy.New(&config.FailurePolicyConfig{Critical: []string{"c"}})
			outcome := dispatch(policy, missing("validate-c"))

			Expect(outcome.Errors).To(HaveLen(1))
			Expect(outcome.Errors[0].ShouldBlock).To(BeTrue())
			Expect(outcome.Errors[0].Reference.Code()).To(Equal("HOOK001"))
		})

		It("fills in a missing reason and downgrades in warn mode", func() {
			policy := failpolicy.New(&config.FailurePolicyConfig{Mode: "warn"})
			outcome := dispatch(
				policy,
				returning("plugin-registry", validator.Fail("x").MarkUnavailable()),
			)

			Expect(outcome.Errors).To(HaveLen(1))
			Expect(outcome.Errors[0].ShouldBlock).To(BeFalse())
			Expect(outcome.Errors[0].UnavailableReason).To(Equal(validator.ReasonError))
		})

		It("records a check only for validators that ran", func() {
			outcome := dispatch(
				nil,
				returning("validate-ok", validator.Pass()),
				timedOut("validate-a", false),
			)

			Expect(outcome.Checks).To(HaveLen(1))
			Expect(outcome.Checks[0].Validator).To(Equal("validate-ok"))
		})
	})
})
