// Package metrics checks that hooks can record local outcome metrics.
package metrics

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const bytesPerKB = 1024

// RecordChecker verifies the metrics log is writable. Hooks drop a sample
// they cannot write without saying so, so this is where it shows.
type RecordChecker struct {
	cfg   *config.Config
	store func(*config.MetricsConfig) *metrics.Store
}

// NewRecordChecker creates a RecordChecker for cfg.
func NewRecordChecker(cfg *config.Config) *RecordChecker {
	return NewRecordCheckerWithStore(cfg, func(m *config.MetricsConfig) *metrics.Store {
		return metrics.NewStore(m)
	})
}

// NewRecordCheckerWithStore creates a RecordChecker using store.
func NewRecordCheckerWithStore(
	cfg *config.Config,
	store func(*config.MetricsConfig) *metrics.Store,
) *RecordChecker {
	return &RecordChecker{cfg: cfg, store: store}
}

// Name returns the name of the check.
func (*RecordChecker) Name() string {
	return "Outcome metrics are recorded"
}

// Category returns the category of the check.
func (*RecordChecker) Category() doctor.Category {
	return doctor.CategoryMetrics
}

// Check probes the log and reports how much of its size cap is used.
func (c *RecordChecker) Check(context.Context) doctor.CheckResult {
	var metricsCfg *config.MetricsConfig
	if c.cfg != nil {
		metricsCfg = c.cfg.Metrics
	}

	if !metricsCfg.IsEnabled() {
		return doctor.Skip(c.Name(), "Outcome metrics disabled")
	}

	store := c.store(metricsCfg)
	if err := store.Probe(); err != nil {
		return doctor.FailWarning(c.Name(), "Hooks cannot record outcome metrics").
			WithDetails(err.Error(), "Log: "+store.Path())
	}

	return doctor.Pass(c.Name(), fmt.Sprintf(
		"Recording to %s (%d of %d KB)",
		store.Path(),
		store.Size()/bytesPerKB,
		store.MaxSize()/bytesPerKB,
	))
}
