// Package dispatcher provides validation orchestration.
package dispatcher

import (
	"context"
	"runtime"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	// ioWorkerMultiplier is the multiplier for I/O workers relative to CPU count.
	ioWorkerMultiplier = 2
)

// Executor runs validators and collects their results.
type Executor interface {
	// Execute runs validators and returns validation errors.
	Execute(
		ctx context.Context,
		hookCtx *hook.Context,
		validators []validator.Validator,
	) []*ValidationError
}

// SequentialExecutor runs validators one at a time in order.
type SequentialExecutor struct {
	logger logger.Logger
}

// NewSequentialExecutor creates a new SequentialExecutor.
func NewSequentialExecutor(log logger.Logger) *SequentialExecutor {
	return &SequentialExecutor{logger: log}
}

// Execute runs validators sequentially.
func (*SequentialExecutor) Execute(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
) []*ValidationError {
	errors := make([]*ValidationError, 0, len(validators))

	for _, v := range validators {
		select {
		case <-ctx.Done():
			return errors
		default:
		}

		result := v.Validate(ctx, hookCtx)
		if !result.Passed {
			errors = append(errors, toValidationError(v, result))
		}
	}

	return errors
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

// Execute runs validators concurrently, using category-specific worker pools.
func (e *ParallelExecutor) Execute(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
) []*ValidationError {
	if len(validators) == 0 {
		return nil
	}

	// For a single validator, run directly without goroutine overhead
	if len(validators) == 1 {
		result := validators[0].Validate(ctx, hookCtx)
		if !result.Passed {
			return []*ValidationError{toValidationError(validators[0], result)}
		}

		return nil
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []*ValidationError
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

			result := v.Validate(ctx, hookCtx)

			if !result.Passed {
				mu.Lock()

				results = append(results, toValidationError(v, result))

				mu.Unlock()
			}
		}(v)
	}

	wg.Wait()

	return results
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
	}
}
