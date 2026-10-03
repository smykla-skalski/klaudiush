package main

import (
	"fmt"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// recordMetrics appends what the written response did to the local outcome
// metrics. It runs after the response is on stdout, and a failure here only
// loses the sample: enforcement never depends on it.
func (h *hookRun) recordMetrics(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	stopped bool,
) {
	obs := &metrics.Observation{
		Context:  hookCtx,
		Errors:   errs,
		Stopped:  stopped,
		Released: h.released.Load(),
		Skipped:  h.skipped.Load(),
	}

	if outcome := h.outcome.Load(); outcome != nil {
		obs.Checks = outcome.Checks
		obs.Timings = outcome.Timings
	}

	h.record(obs)
}

// recordSelection records a Gemini tool selection, advisory when it
// withheld tools.
func (h *hookRun) recordSelection(hookCtx *hook.Context, filtered bool) {
	h.record(&metrics.Observation{Context: hookCtx, Filtered: filtered})
}

func (h *hookRun) record(obs *metrics.Observation) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Error("recording metrics panicked", "panic", fmt.Sprint(r))
		}
	}()

	cfg := h.metricsConfig()
	if !cfg.IsEnabled() || obs.Context == nil {
		return
	}

	obs.Elapsed = time.Since(h.start)

	if err := metrics.NewStore(cfg).Record(obs); err != nil {
		h.log.Info("failed to record metrics", "error", err)
	}
}

// metricsConfig returns the loaded metrics settings, or what can be read of
// the configuration when loading it failed.
func (h *hookRun) metricsConfig() *config.MetricsConfig {
	if cfg := h.metrics.Load(); cfg != nil {
		return cfg
	}

	workDir := ""
	if dir := h.workDir.Load(); dir != nil {
		workDir = *dir
	}

	if cfg := looseConfig(workDir); cfg != nil {
		return cfg.GetMetrics()
	}

	return nil
}
