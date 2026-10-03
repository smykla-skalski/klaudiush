package dispatcher

import (
	"context"
	"slices"
	"sync"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// RunObserver receives each validator run as soon as it completes.
type RunObserver func(ValidatorRun)

// ObservingExecutor is an Executor that also runs validators like Run while
// handing each completed run to observe before returning, so findings
// survive a later validator that never returns. observe may be called
// concurrently.
type ObservingExecutor interface {
	Executor
	RunObserved(
		ctx context.Context,
		hookCtx *hook.Context,
		validators []validator.Validator,
		observe RunObserver,
	) []ValidatorRun
}

// WithProgress sets a function that receives the findings made so far each
// time a validator completes, so a caller that stops waiting still has them.
// Exceptions are only applied to the final result, so a published blocking
// finding may still be waived once dispatch returns.
func WithProgress(publish func([]*ValidationError)) DispatcherOption {
	return func(d *Dispatcher) {
		d.publish = publish
	}
}

// progress collects the findings of one dispatch for publishing: settled
// ones from finished validator batches and the partial batch still running.
type progress struct {
	publish func([]*ValidationError)
	mu      sync.Mutex
	settled []*ValidationError
}

func newProgress(publish func([]*ValidationError)) *progress {
	if publish == nil {
		return nil
	}

	return &progress{publish: publish}
}

// settle records the findings of a finished batch.
func (p *progress) settle(errs []*ValidationError) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.settled = append(p.settled, errs...)
	p.publish(slices.Clone(p.settled))
}

// partial publishes the settled findings plus those of the running batch.
func (p *progress) partial(errs []*ValidationError) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.publish(append(slices.Clone(p.settled), errs...))
}

// run runs validators, publishing the findings of each completed run when
// the dispatch tracks progress and the executor can report it.
func (d *Dispatcher) run(
	ctx context.Context,
	hookCtx *hook.Context,
	validators []validator.Validator,
	p *progress,
) []ValidatorRun {
	observing, ok := d.executor.(ObservingExecutor)
	if p == nil || !ok {
		return d.executor.Run(ctx, hookCtx, validators)
	}

	var (
		mu   sync.Mutex
		done []ValidatorRun
	)

	return observing.RunObserved(ctx, hookCtx, validators, func(run ValidatorRun) {
		mu.Lock()
		defer mu.Unlock()

		done = append(done, run)
		p.partial(afterToolFindings(hookCtx, d.provisional(hookCtx, done)))
	})
}

// provisional turns completed runs into findings the way runValidators does,
// without exceptions, whose rate limits and audit log must see each finding
// once.
func (d *Dispatcher) provisional(hookCtx *hook.Context, runs []ValidatorRun) []*ValidationError {
	quiet := *d
	quiet.logger = logger.NewNoOpLogger()

	errs := quiet.applyOverrides(quiet.applyFailurePolicy(failures(runs)))

	resource := hookCtx.Resource()
	for _, verr := range errs {
		verr.Resource = resource
	}

	return errs
}
