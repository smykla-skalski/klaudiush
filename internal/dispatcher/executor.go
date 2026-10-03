// Package dispatcher provides validation orchestration.
package dispatcher

import (
	"cmp"
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const (
	// ioWorkerMultiplier is the multiplier for I/O workers relative to CPU count.
	ioWorkerMultiplier = 2
)

// Executor runs validators and collects their results.
type Executor interface {
	// Run runs validators and returns the result of each one that completed.
	Run(
		ctx context.Context,
		hookCtx *hook.Context,
		validators []validator.Validator,
	) []ValidatorRun
}

// ValidatorRun is the result of one validator that ran to completion.
type ValidatorRun struct {
	Validator validator.Validator
	Result    *validator.Result
}

// failures converts the failed runs to validation errors, in run order.
func failures(runs []ValidatorRun) []*ValidationError {
	errs := make([]*ValidationError, 0, len(runs))

	for _, run := range runs {
		if !run.Result.Passed {
			errs = append(errs, toValidationError(run.Validator, run.Result))
		}
	}

	return errs
}

// SequentialExecutor runs validators one at a time in order.
type SequentialExecutor struct {
	logger logger.Logger
}

// NewSequentialExecutor creates a new SequentialExecutor.
func NewSequentialExecutor(log logger.Logger) *SequentialExecutor {
	return &SequentialExecutor{logger: log}
}

// Execute runs validators sequentially and returns validation errors.
func (se *SequentialExecutor) Execute(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
) []*ValidationError {
	return failures(se.Run(ctx, hookCtx, validators))
}

// Run runs validators sequentially.
func (se *SequentialExecutor) Run(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
) []ValidatorRun {
	return se.RunObserved(ctx, hookCtx, validators, nil)
}

// RunObserved runs validators sequentially, handing each run to observe as
// it completes.
func (se *SequentialExecutor) RunObserved(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
	observe RunObserver,
) []ValidatorRun {
	runs := make([]ValidatorRun, 0, len(validators))

	for _, v := range validators {
		run := ValidatorRun{
			Validator: v,
			Result:    runValidator(ctx, hookCtx, v, se.logger),
		}

		observe.notify(run)

		runs = append(runs, run)
	}

	return runs
}

// notify hands run to the observer, if there is one.
func (observe RunObserver) notify(run ValidatorRun) {
	if observe != nil {
		observe(run)
	}
}

// runValidator runs one validator and turns every way it can fail to answer
// into an unavailable result: it never started because ctx ended, it
// panicked, it returned nothing, or it passed only after ctx ended, when
// whatever it ran may have been cut short.
func runValidator(
	ctx context.Context,
	hookCtx *hook.Context,
	v validator.Validator,
	log logger.Logger,
) (result *validator.Result) {
	if reason := validator.ReasonFromContext(ctx); reason != "" {
		return notRun(v, reason)
	}

	start := time.Now()

	defer func() {
		if r := recover(); r != nil {
			log.Error("validator panicked",
				"name", v.Name(),
				"panic", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)

			result = validator.Unavailable(
				validator.ReasonPanic,
				fmt.Sprintf("%s crashed: %v", shortName(v.Name()), r),
			)
		}
	}()

	result = v.Validate(ctx, hookCtx)

	switch reason := validator.ReasonFromContext(ctx); {
	case result == nil:
		result = validator.Unavailable(
			validator.ReasonError,
			shortName(v.Name())+" returned no result",
		)
	case reason != "" && result.Passed:
		result = notFinished(v, reason)
	}

	log.Debug("validator completed",
		"name", v.Name(),
		"passed", result.Passed,
		"unavailable", result.Unavailable,
		"elapsed_ms", time.Since(start).Milliseconds(),
	)

	return result
}

// notRun reports a validator skipped because the hook ran out of time or
// was canceled.
func notRun(v validator.Validator, reason validator.UnavailableReason) *validator.Result {
	return validator.Unavailable(
		reason,
		fmt.Sprintf("%s did not run: %s before it started", shortName(v.Name()), reason.Describe()),
	)
}

// notFinished reports a validator whose pass cannot be trusted because the
// hook ran out of time or was canceled while it ran.
func notFinished(v validator.Validator, reason validator.UnavailableReason) *validator.Result {
	return validator.Unavailable(
		reason,
		fmt.Sprintf("%s did not finish: %s while it ran", shortName(v.Name()), reason.Describe()),
	)
}

// ParallelExecutorConfig holds configuration for parallel execution.
type ParallelExecutorConfig struct {
	// MaxCPUWorkers is the maximum number of concurrent CPU-bound validators.
	// Default: runtime.NumCPU()
	MaxCPUWorkers int

	// MaxIOWorkers is the maximum number of concurrent I/O-bound validators.
	// Default: runtime.NumCPU() * 2
	MaxIOWorkers int

	// MaxGitWorkers is the maximum number of concurrent git operations.
	// Default: 1 (serialized to avoid index lock contention)
	MaxGitWorkers int
}

// DefaultParallelConfig returns the default parallel execution configuration.
func DefaultParallelConfig() *ParallelExecutorConfig {
	numCPU := runtime.NumCPU()

	return &ParallelExecutorConfig{
		MaxCPUWorkers: numCPU,
		MaxIOWorkers:  numCPU * ioWorkerMultiplier,
		MaxGitWorkers: 1,
	}
}

// ParallelExecutor runs validators concurrently using category-specific worker pools.
type ParallelExecutor struct {
	logger  logger.Logger
	cpuPool *semaphore.Weighted
	ioPool  *semaphore.Weighted
	gitPool *semaphore.Weighted
}

// NewParallelExecutor creates a new ParallelExecutor with the given configuration.
func NewParallelExecutor(log logger.Logger, cfg *ParallelExecutorConfig) *ParallelExecutor {
	if cfg == nil {
		cfg = DefaultParallelConfig()
	}

	return &ParallelExecutor{
		logger:  log,
		cpuPool: semaphore.NewWeighted(int64(cfg.MaxCPUWorkers)),
		ioPool:  semaphore.NewWeighted(int64(cfg.MaxIOWorkers)),
		gitPool: semaphore.NewWeighted(int64(cfg.MaxGitWorkers)),
	}
}

// Execute runs validators concurrently and returns validation errors.
func (e *ParallelExecutor) Execute(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
) []*ValidationError {
	errs := failures(e.Run(ctx, hookCtx, validators))
	if len(errs) == 0 {
		return nil
	}

	return errs
}

// Run runs validators concurrently, using category-specific worker pools.
func (e *ParallelExecutor) Run(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
) []ValidatorRun {
	return e.RunObserved(ctx, hookCtx, validators, nil)
}

// RunObserved runs validators like Run, handing each run to observe as it
// completes. observe is called from the validator goroutines.
func (e *ParallelExecutor) RunObserved(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
	observe RunObserver,
) []ValidatorRun {
	if len(validators) == 0 {
		return nil
	}

	// For a single validator, run directly without goroutine overhead
	if len(validators) == 1 {
		v := validators[0]
		run := ValidatorRun{Validator: v, Result: runValidator(ctx, hookCtx, v, e.logger)}

		observe.notify(run)

		return []ValidatorRun{run}
	}

	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		runs []ValidatorRun
	)

	for _, v := range validators {
		wg.Add(1)

		go func(v validator.Validator) {
			defer wg.Done()

			var result *validator.Result

			pool := e.poolFor(v.Category())
			if err := pool.Acquire(ctx, 1); err != nil {
				result = notRun(
					v,
					cmp.Or(validator.ReasonFromContext(ctx), validator.ReasonCanceled),
				)
			} else {
				e.logger.Debug("running validator",
					"validator", v.Name(),
					"category", v.Category().String(),
				)

				result = runValidator(ctx, hookCtx, v, e.logger)

				pool.Release(1)
			}

			run := ValidatorRun{Validator: v, Result: result}

			observe.notify(run)

			mu.Lock()

			runs = append(runs, run)

			mu.Unlock()
		}(v)
	}

	wg.Wait()

	return runs
}

// poolFor returns the appropriate semaphore pool for a validator category.
func (e *ParallelExecutor) poolFor(category validator.ValidatorCategory) *semaphore.Weighted {
	switch category {
	case validator.CategoryIO:
		return e.ioPool
	case validator.CategoryGit:
		return e.gitPool
	default:
		return e.cpuPool
	}
}

// toValidationError converts a validator and result to a ValidationError.
func toValidationError(v validator.Validator, result *validator.Result) *ValidationError {
	return &ValidationError{
		Validator:   v.Name(),
		Message:     result.Message,
		Details:     result.Details,
		ShouldBlock: result.ShouldBlock,
		Reference:   result.Reference,
		FixHint:     result.FixHint,
		Findings:    result.Findings,
		Unavailable: result.Unavailable,

		UnavailableReason: result.ReasonOf(),
	}
}
