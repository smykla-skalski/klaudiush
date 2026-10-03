package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const hoursPerDay = 24

// maxSinceDays keeps --since within what a time.Duration can hold.
const maxSinceDays = 36500

var (
	metricsSince    string
	metricsProvider string
	metricsEvent    string
	metricsJSON     bool

	errInvalidSince = errors.New("invalid --since")
)

var metricsCmd = &cobra.Command{
	Use:   cmdUseMetrics,
	Short: "Report local enforcement outcome metrics",
	Long: `Report what klaudiush hooks actually did, from local metrics.

Every hook appends one redacted line to a size-capped log in the state
directory: what the response did (prevented the action, kept a completion
gate shut, only advised, warned, accepted an exception, could not validate),
the error codes it reported, checks that could not run, and how long the hook
and each validator took. No command, message, file path or session ID is
stored, and nothing leaves the machine.

Findings after a tool ran are advisory: the action already happened, so they
never count as prevented.

Subcommands:
  report  Show the report (default)
  prune   Drop records older than the retention
  clear   Remove all metrics and the hashing salt`,
	RunE: runMetricsReport,
}

var metricsReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show enforcement outcomes, repairs, unavailable checks and latency",
	Long: `Show enforcement outcomes by provider and event, repair retries, recurring
violations, exceptions by code, checks that could not run, and hook and
validator latency.

Examples:
  klaudiush metrics report                    # Default window (metrics.retention)
  klaudiush metrics report --since 7d         # Last 7 days
  klaudiush metrics report --provider codex   # One provider
  klaudiush metrics report --json             # Machine-readable`,
	RunE: runMetricsReport,
}

var metricsPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Drop metrics older than the retention",
	RunE:  runMetricsPrune,
}

var metricsClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all metrics and the hashing salt",
	RunE:  runMetricsClear,
}

func init() {
	rootCmd.AddCommand(metricsCmd)
	metricsCmd.AddCommand(metricsReportCmd, metricsPruneCmd, metricsClearCmd)

	for _, cmd := range []*cobra.Command{metricsCmd, metricsReportCmd} {
		cmd.Flags().StringVar(&metricsSince, "since", "",
			"Report window, e.g. 24h or 7d (default: metrics.retention)")
		cmd.Flags().StringVar(&metricsProvider, "provider", "",
			"Only hooks of this provider (claude, codex, gemini, opencode)")
		cmd.Flags().StringVar(&metricsEvent, "event", "",
			"Only hooks of this native event (e.g. PreToolUse)")
		cmd.Flags().BoolVar(&metricsJSON, "json", false, "Output the report as JSON")
	}
}

func runMetricsReport(cmd *cobra.Command, _ []string) error {
	cfg := loadMetricsConfig(loggerFromCmd(cmd))
	store := metrics.NewStore(cfg)

	window := store.Retention()

	if metricsSince != "" {
		parsed, err := parseSince(metricsSince)
		if err != nil {
			return err
		}

		window = parsed
	}

	until := time.Now()
	since := until.Add(-window)

	records, skipped, err := store.Load(since)
	if err != nil {
		return errors.Wrap(err, "failed to read metrics")
	}

	report := metrics.Summarize(records, since, until, metrics.Filter{
		Provider: metricsProvider,
		Event:    metricsEvent,
	})
	report.SkippedLines = skipped

	out := cmd.OutOrStdout()

	if metricsJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")

		return errors.Wrap(enc.Encode(report), "failed to write report")
	}

	if !cfg.IsEnabled() {
		_, err := fmt.Fprintln(out, "Metrics recording is disabled (metrics.enabled = false).")
		if err != nil {
			return errors.Wrap(err, "failed to write report")
		}
	}

	return errors.Wrap(metrics.Render(out, report), "failed to write report")
}

func runMetricsPrune(cmd *cobra.Command, _ []string) error {
	store := metrics.NewStore(loadMetricsConfig(loggerFromCmd(cmd)))

	dropped, err := store.Prune()
	if err != nil {
		return errors.Wrap(err, "failed to prune metrics")
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(),
		"Dropped %d records older than %s or unreadable\n",
		dropped, store.Retention())

	return errors.Wrap(err, "failed to write output")
}

func runMetricsClear(cmd *cobra.Command, _ []string) error {
	store := metrics.NewStore(loadMetricsConfig(loggerFromCmd(cmd)))

	if err := store.Clear(); err != nil {
		return errors.Wrap(err, "failed to clear metrics")
	}

	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Removed metrics in %s\n", store.Path())

	return errors.Wrap(err, "failed to write output")
}

// loadMetricsConfig reads the metrics settings, falling back to defaults
// when the configuration cannot be loaded: a report must still work then.
func loadMetricsConfig(log logger.Logger) *config.MetricsConfig {
	cfg, err := loadConfig(log, "")
	if err != nil {
		log.Info("failed to load configuration for metrics, using defaults", "error", err)

		return &config.MetricsConfig{}
	}

	return cfg.GetMetrics()
}

// parseSince reads a Go duration or a whole number of days ("7d").
func parseSince(value string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(value, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 || n > maxSinceDays {
			return 0, errors.Wrapf(errInvalidSince, "%q", value)
		}

		return time.Duration(n) * hoursPerDay * time.Hour, nil
	}

	window, err := time.ParseDuration(value)
	if err != nil || window <= 0 {
		return 0, errors.Wrapf(errInvalidSince, "%q", value)
	}

	return window, nil
}
