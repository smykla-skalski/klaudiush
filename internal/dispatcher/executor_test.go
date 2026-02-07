package dispatcher_test

import (
	"context"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/dispatcher"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// testValidator is a simple test validator implementation
type testValidator struct {
	name     string
	category validator.ValidatorCategory
	result   *validator.Result
	delay    time.Duration

	// For tracking concurrent execution
	startTime time.Time
	endTime   time.Time
	started   atomic.Bool
	finished  atomic.Bool
}

func newTestValidator(
	name string,
	category validator.ValidatorCategory,
	result *validator.Result,
) *testValidator {
	return &testValidator{
		name:     name,
		category: category,
		result:   result,
	}
}

func (v *testValidator) Name() string {
	return v.name
}

func (v *testValidator) Category() validator.ValidatorCategory {
	return v.category
}

func (v *testValidator) Validate(_ context.Context, _ *hook.Context) *validator.Result {
	v.started.Store(true)
	v.startTime = time.Now()

	if v.delay > 0 {
		time.Sleep(v.delay)
	}

	v.endTime = time.Now()
	v.finished.Store(true)

	return v.result
}

var _ = Describe("Executor", func() {
	var (
		log     logger.Logger
		hookCtx *hook.Context
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		hookCtx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{
				Command: "test command",
			},
		}
	})

	Describe("SequentialExecutor", func() {
		var executor *dispatcher.SequentialExecutor

		BeforeEach(func() {
			executor = dispatcher.NewSequentialExecutor(log)
		})

		Context("with empty validators", func() {
			It("should return empty for nil validators", func() {
				result := executor.Execute(context.Background(), hookCtx, nil)
				Expect(result).To(BeEmpty())
			})

			It("should return empty for empty slice", func() {
				result := executor.Execute(context.Background(), hookCtx, []validator.Validator{})
				Expect(result).To(BeEmpty())
			})
		})

		Context("with passing validators", func() {
			It("should return no errors when all pass", func() {
				validators := []validator.Validator{
					newTestValidator("v1", validator.CategoryCPU, validator.Pass()),
					newTestValidator("v2", validator.CategoryIO, validator.Pass()),
					newTestValidator("v3", validator.CategoryGit, validator.Pass()),
				}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(BeEmpty())
			})
		})

		Context("with failing validators", func() {
			It("should collect all failures", func() {
				validators := []validator.Validator{
					newTestValidator("v1", validator.CategoryCPU, validator.Pass()),
					newTestValidator("v2", validator.CategoryIO, validator.Fail("error 1")),
					newTestValidator("v3", validator.CategoryGit, validator.Fail("error 2")),
				}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(HaveLen(2))
				Expect(result[0].Validator).To(Equal("v2"))
				Expect(result[1].Validator).To(Equal("v3"))
			})

			It("should include warnings that don't block", func() {
				validators := []validator.Validator{
					newTestValidator("v1", validator.CategoryCPU, validator.Warn("warning 1")),
					newTestValidator("v2", validator.CategoryIO, validator.Fail("error 1")),
				}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(HaveLen(2))
				Expect(result[0].ShouldBlock).To(BeFalse())
				Expect(result[1].ShouldBlock).To(BeTrue())
			})
		})

		Context("with context cancellation", func() {
			It("should stop on context cancellation", func() {
				ctx, cancel := context.WithCancel(context.Background())

				v1 := newTestValidator("v1", validator.CategoryCPU, validator.Pass())
				v1.delay = 50 * time.Millisecond

				v2 := newTestValidator("v2", validator.CategoryIO, validator.Pass())
				v2.delay = 50 * time.Millisecond

				validators := []validator.Validator{v1, v2}

				// Cancel after first validator should complete
				go func() {
					time.Sleep(75 * time.Millisecond)
					cancel()
				}()

				result := executor.Execute(ctx, hookCtx, validators)
				Expect(result).To(BeEmpty())

				// First should have started, second may or may not depending on timing
				Expect(v1.started.Load()).To(BeTrue())
			})
		})
	})

	Describe("ParallelExecutor", func() {
		var executor *dispatcher.ParallelExecutor

		BeforeEach(func() {
			cfg := &dispatcher.ParallelExecutorConfig{
				MaxCPUWorkers: 4,
				MaxIOWorkers:  8,
				MaxGitWorkers: 1,
			}
			executor = dispatcher.NewParallelExecutor(log, cfg)
		})

		Context("with empty validators", func() {
			It("should return nil for nil validators", func() {
				result := executor.Execute(context.Background(), hookCtx, nil)
				Expect(result).To(BeNil())
			})

			It("should return nil for empty slice", func() {
				result := executor.Execute(context.Background(), hookCtx, []validator.Validator{})
				Expect(result).To(BeNil())
			})
		})

		Context("with single validator", func() {
			It("should execute without goroutine overhead", func() {
				v := newTestValidator("v1", validator.CategoryCPU, validator.Pass())
				validators := []validator.Validator{v}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(BeEmpty())
				Expect(v.finished.Load()).To(BeTrue())
			})

			It("should return failure for single failing validator", func() {
				v := newTestValidator("v1", validator.CategoryCPU, validator.Fail("error"))
				validators := []validator.Validator{v}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(HaveLen(1))
				Expect(result[0].Message).To(Equal("error"))
			})
		})

		Context("with passing validators", func() {
			It("should return no errors when all pass", func() {
				validators := []validator.Validator{
					newTestValidator("v1", validator.CategoryCPU, validator.Pass()),
					newTestValidator("v2", validator.CategoryIO, validator.Pass()),
					newTestValidator("v3", validator.CategoryGit, validator.Pass()),
				}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(BeEmpty())
			})
		})

		Context("with failing validators", func() {
			It("should collect all failures from concurrent execution", func() {
				validators := []validator.Validator{
					newTestValidator("v1", validator.CategoryCPU, validator.Pass()),
					newTestValidator("v2", validator.CategoryIO, validator.Fail("error 1")),
					newTestValidator("v3", validator.CategoryGit, validator.Fail("error 2")),
					newTestValidator("v4", validator.CategoryCPU, validator.Fail("error 3")),
				}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(HaveLen(3))

				// Check all failures are captured (order may vary due to concurrency)
				failedNames := make([]string, len(result))
				for i, r := range result {
					failedNames[i] = r.Validator
				}
				Expect(failedNames).To(ContainElements("v2", "v3", "v4"))
			})
		})

		Context("with concurrent execution", func() {
			It("should run CPU validators concurrently", func() {
				// Create validators with delay
				v1 := newTestValidator("cpu1", validator.CategoryCPU, validator.Pass())
				v1.delay = 50 * time.Millisecond

				v2 := newTestValidator("cpu2", validator.CategoryCPU, validator.Pass())
				v2.delay = 50 * time.Millisecond

				v3 := newTestValidator("cpu3", validator.CategoryCPU, validator.Pass())
				v3.delay = 50 * time.Millisecond

				validators := []validator.Validator{v1, v2, v3}

				start := time.Now()
				result := executor.Execute(context.Background(), hookCtx, validators)
				elapsed := time.Since(start)

				Expect(result).To(BeEmpty())

				// If running in parallel, should complete in ~50ms, not 150ms
				// Allow some overhead but should be much less than sequential
				Expect(elapsed).To(BeNumerically("<", 120*time.Millisecond))
			})

			It("should serialize Git validators", func() {
				// Create Git validators with delay
				v1 := newTestValidator("git1", validator.CategoryGit, validator.Pass())
				v1.delay = 30 * time.Millisecond

				v2 := newTestValidator("git2", validator.CategoryGit, validator.Pass())
				v2.delay = 30 * time.Millisecond

				validators := []validator.Validator{v1, v2}

				start := time.Now()
				result := executor.Execute(context.Background(), hookCtx, validators)
				elapsed := time.Since(start)

				Expect(result).To(BeEmpty())

				// With MaxGitWorkers=1, should run sequentially (~60ms)
				Expect(elapsed).To(BeNumerically(">=", 55*time.Millisecond))
			})

			It("should run IO validators concurrently", func() {
				v1 := newTestValidator("io1", validator.CategoryIO, validator.Pass())
				v1.delay = 50 * time.Millisecond

				v2 := newTestValidator("io2", validator.CategoryIO, validator.Pass())
				v2.delay = 50 * time.Millisecond

				validators := []validator.Validator{v1, v2}

				start := time.Now()
				result := executor.Execute(context.Background(), hookCtx, validators)
				elapsed := time.Since(start)

				Expect(result).To(BeEmpty())

				// Should run in parallel
				Expect(elapsed).To(BeNumerically("<", 90*time.Millisecond))
			})
		})

		Context("with context cancellation", func() {
			It("should stop pending validators on cancellation", func() {
				ctx, cancel := context.WithCancel(context.Background())

				// Use MaxCPUWorkers=1 to force serialization for this test
				cfg := &dispatcher.ParallelExecutorConfig{
					MaxCPUWorkers: 1,
					MaxIOWorkers:  1,
					MaxGitWorkers: 1,
				}
				executor = dispatcher.NewParallelExecutor(log, cfg)

				v1 := newTestValidator("v1", validator.CategoryCPU, validator.Pass())
				v1.delay = 100 * time.Millisecond

				v2 := newTestValidator("v2", validator.CategoryCPU, validator.Pass())
				v2.delay = 100 * time.Millisecond

				validators := []validator.Validator{v1, v2}

				// Cancel shortly after start
				go func() {
					time.Sleep(50 * time.Millisecond)
					cancel()
				}()

				start := time.Now()
				result := executor.Execute(ctx, hookCtx, validators)
				elapsed := time.Since(start)

				Expect(result).To(BeEmpty())

				// Should complete faster than running both validators
				Expect(elapsed).To(BeNumerically("<", 180*time.Millisecond))
			})
		})

		Context("with default configuration", func() {
			It("should use default config when nil is provided", func() {
				executor := dispatcher.NewParallelExecutor(log, nil)
				validators := []validator.Validator{
					newTestValidator("v1", validator.CategoryCPU, validator.Pass()),
				}

				result := executor.Execute(context.Background(), hookCtx, validators)
				Expect(result).To(BeEmpty())
			})
		})
	})

	Describe("with gomock validators", func() {
		var ctrl *gomock.Controller

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
		})

		AfterEach(func() {
			ctrl.Finish()
		})

		It("should work with mock validators in parallel executor", func() {
			executor := dispatcher.NewParallelExecutor(log, nil)

			mock1 := validator.NewMockValidator(ctrl)
			mock1.EXPECT().Name().Return("mock1").AnyTimes()
			mock1.EXPECT().Category().Return(validator.CategoryCPU).AnyTimes()
			mock1.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(validator.Pass())

			mock2 := validator.NewMockValidator(ctrl)
			mock2.EXPECT().Name().Return("mock2").AnyTimes()
			mock2.EXPECT().Category().Return(validator.CategoryIO).AnyTimes()
			mock2.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(validator.Fail("error"))

			validators := []validator.Validator{mock1, mock2}

			result := executor.Execute(context.Background(), hookCtx, validators)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Validator).To(Equal("mock2"))
		})
	})

	Describe("race condition tests", Label("race"), func() {
		// These tests are specifically designed to catch race conditions
		// Run with: go test -race ./internal/dispatcher/...

		It("should safely collect results from many concurrent validators", func() {
			cfg := &dispatcher.ParallelExecutorConfig{
				MaxCPUWorkers: 10,
				MaxIOWorkers:  10,
				MaxGitWorkers: 1,
			}
			executor := dispatcher.NewParallelExecutor(log, cfg)

			// Create many validators that all fail
			numValidators := 100
			validators := make([]validator.Validator, numValidators)

			for i := range numValidators {
				validators[i] = newTestValidator(
					"v"+string(rune('0'+i%10)),
					validator.CategoryCPU,
					validator.Fail("error"),
				)
			}

			result := executor.Execute(context.Background(), hookCtx, validators)

			// All should be collected
			Expect(result).To(HaveLen(numValidators))
		})

		It("should handle mixed results from concurrent validators", func() {
			cfg := &dispatcher.ParallelExecutorConfig{
				MaxCPUWorkers: 5,
				MaxIOWorkers:  5,
				MaxGitWorkers: 1,
			}
			executor := dispatcher.NewParallelExecutor(log, cfg)

			const numMixedValidators = 50

			validators := make([]validator.Validator, numMixedValidators)

			for i := range numMixedValidators {
				var res *validator.Result
				if i%2 == 0 {
					res = validator.Pass()
				} else {
					res = validator.Fail("error")
				}

				category := validator.ValidatorCategory(i % 3)
				validators[i] = newTestValidator("v", category, res)
			}

			result := executor.Execute(context.Background(), hookCtx, validators)

			// 25 should fail
			Expect(result).To(HaveLen(25))
		})

		It("should not deadlock with limited worker pools", func() {
			// Very limited pools
			cfg := &dispatcher.ParallelExecutorConfig{
				MaxCPUWorkers: 1,
				MaxIOWorkers:  1,
				MaxGitWorkers: 1,
			}
			executor := dispatcher.NewParallelExecutor(log, cfg)

			const numDeadlockValidators = 20

			validators := make([]validator.Validator, numDeadlockValidators)

			for i := range numDeadlockValidators {
				category := validator.ValidatorCategory(i % 3)
				v := newTestValidator("v", category, validator.Pass())
				v.delay = 5 * time.Millisecond
				validators[i] = v
			}

			// Use timeout to detect deadlock
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := executor.Execute(ctx, hookCtx, validators)
			Expect(result).To(BeEmpty())
		})
	})

	Describe("ValidatorCategory", func() {
		It("should have correct string representation", func() {
			Expect(validator.CategoryCPU.String()).To(Equal("CPU"))
			Expect(validator.CategoryIO.String()).To(Equal("IO"))
			Expect(validator.CategoryGit.String()).To(Equal("Git"))
			Expect(validator.ValidatorCategory(99).String()).To(Equal("Unknown"))
		})
	})

	Describe("DefaultParallelConfig", func() {
		It("should return sensible defaults", func() {
			cfg := dispatcher.DefaultParallelConfig()

			Expect(cfg.MaxCPUWorkers).To(BeNumerically(">", 0))
			Expect(cfg.MaxIOWorkers).To(BeNumerically(">", 0))
			Expect(cfg.MaxGitWorkers).To(Equal(1))
			Expect(cfg.MaxIOWorkers).To(BeNumerically(">=", cfg.MaxCPUWorkers))
		})
	})
})
