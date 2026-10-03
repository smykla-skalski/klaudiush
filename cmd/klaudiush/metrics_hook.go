package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
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
		obs.Unavailable = outcome.Unavailable
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
// An explicit enabled = false in any config file wins over a fallback that
// could not read that file.
func (h *hookRun) metricsConfig() *config.MetricsConfig {
	if cfg := h.metrics.Load(); cfg != nil {
		return cfg
	}

	disabled := false

	if value, ok := os.LookupEnv(metricsEnabledEnv); ok {
		if enabled, err := strconv.ParseBool(value); err == nil && !enabled {
			return &config.MetricsConfig{Enabled: &disabled}
		}
	}

	workDir := ""
	if dir := h.workDir.Load(); dir != nil {
		workDir = *dir
	}

	loader, err := configLoader(workDir)
	if err != nil {
		return nil
	}

	if cfg, loadErr := loader.LoadWithoutValidation(buildFlagsMap()); loadErr == nil {
		return cfg.GetMetrics()
	}

	if scannedMetricsDisabled(loader) {
		return &config.MetricsConfig{Enabled: &disabled}
	}

	if cfg, _, loadErr := loader.LoadGlobalConfigOnly(); loadErr == nil && cfg != nil {
		return cfg.GetMetrics()
	}

	return nil
}

// scannedMetricsDisabled reports whether the project or global config file
// the loader reads says enabled = false inside [metrics], whether or not it
// parses.
func scannedMetricsDisabled(loader *internalconfig.KoanfLoader) bool {
	for _, path := range []string{loader.FindProjectConfigPath(), loader.GlobalConfigPath()} {
		if path == "" {
			continue
		}

		data, readErr := readConfigFile(path)
		if readErr == nil && scanKey(string(data), cmdUseMetrics, "enabled") == valueFalse {
			return true
		}
	}

	return false
}
