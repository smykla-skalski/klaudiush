package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// metricsEnabledEnv turns metrics off even when no configuration loads.
const metricsEnabledEnv = "KLAUDIUSH_METRICS_ENABLED"

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

	if mask := h.releasedFindings.Load(); mask != nil {
		obs.ReleasedFindings = *mask
	}

	if outcome := h.outcome.Load(); outcome != nil {
		obs.Checks = outcome.Ran
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
// the configuration when loading it failed. The environment switch is read
// directly then, since a configuration that cannot be read cannot carry it.
func (h *hookRun) metricsConfig() *config.MetricsConfig {
	if cfg := h.metrics.Load(); cfg != nil {
		return cfg
	}

	if value, ok := os.LookupEnv(metricsEnabledEnv); ok {
		if enabled, err := strconv.ParseBool(value); err == nil && !enabled {
			return &config.MetricsConfig{Enabled: &enabled}
		}
	}

	workDir := ""
	if dir := h.workDir.Load(); dir != nil {
		workDir = *dir
	}

	if cfg := looseConfig(workDir); cfg != nil {
		return cfg.GetMetrics()
	}

	if scannedMetricsDisabled(workDir) {
		disabled := false

		return &config.MetricsConfig{Enabled: &disabled}
	}

	return nil
}

// scannedMetricsDisabled reports whether a project or global config file
// that does not parse says enabled = false inside [metrics].
func scannedMetricsDisabled(workDir string) bool {
	loader, err := configLoader(workDir)
	if err != nil {
		return false
	}

	for _, path := range append(loader.ProjectConfigPaths(), loader.GlobalConfigPath()) {
		data, readErr := readConfigFile(path)
		if readErr == nil && scanKey(string(data), cmdUseMetrics, "enabled") == valueFalse {
			return true
		}
	}

	return false
}
