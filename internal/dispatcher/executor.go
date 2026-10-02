// Package dispatcher provides validation orchestration.
package dispatcher

import (
	"context"
	"runtime"
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
	runs := make([]ValidatorRun, 0, len(validators))

	for _, v := range validators {
		select {
		case <-ctx.Done():
			return runs
		default:
		}

		start := time.Now()
		result := v.Validate(ctx, hookCtx)
		elapsed := time.Since(start)

		se.logger.Debug("validator completed",
			"name", v.Name(),
			"passed", result.Passed,
			"elapsed_ms", elapsed.Milliseconds(),
		)

		runs = append(runs, ValidatorRun{Validator: v, Result: result})
	}

	return runs
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
	if len(validators) == 0 {
		return nil
	}

	// For a single validator, run directly without goroutine overhead
	if len(validators) == 1 {
		v := validators[0]
		start := time.Now()
		result := v.Validate(ctx, hookCtx)
		elapsed := time.Since(start)

		e.logger.Debug("validator completed",
			"name", v.Name(),
			"passed", result.Passed,
			"elapsed_ms", elapsed.Milliseconds(),
		)

		return []ValidatorRun{{Validator: v, Result: result}}
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

			// Acquire semaphore for the appropriate pool
			pool := e.poolFor(v.Category())
			if err := pool.Acquire(ctx, 1); err != nil {
				// Context cancelled
				return
			}
			defer pool.Release(1)

			// Check context before running
			select {
			case <-ctx.Done():
				return
			default:
			}

			e.logger.Debug("running validator",
				"validator", v.Name(),
				"category", v.Category().String(),
			)

			start := time.Now()
			result := v.Validate(ctx, hookCtx)
			elapsed := time.Since(start)

			e.logger.Debug("validator completed",
				"name", v.Name(),
				"passed", result.Passed,
				"elapsed_ms", elapsed.Milliseconds(),
			)

			mu.Lock()

			runs = append(runs, ValidatorRun{Validator: v, Result: result})

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
	}
}
