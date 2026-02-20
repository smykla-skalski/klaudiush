// Package main provides the CLI entry point for klaudiush.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// topPatternsLimit is the maximum number of patterns shown in stats.
const topPatternsLimit = 5

// Patterns command flags.
var (
	patternsMinCount  int
	patternsErrorCode string
	patternsJSON      bool
)

var patternsCmd = &cobra.Command{
	Use:   "patterns",
	Short: "Manage failure pattern tracking",
	Long: `Manage failure pattern tracking data.

View, filter, and maintain the failure pattern database that tracks
which validation errors commonly follow each other.

Subcommands:
  list     List known patterns
  stats    Show pattern statistics
  reset    Clear learned patterns (keep seeds)`,
}

var patternsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List known failure patterns",
	Long: `List known failure patterns with optional filtering.

Examples:
  klaudiush patterns list                          # List all patterns
  klaudiush patterns list --min-count 5            # Filter by minimum count
  klaudiush patterns list --error-code GIT013      # Filter by source or target
  klaudiush patterns list --json                   # Output as JSON`,
	RunE: runPatternsList,
}

var patternsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show pattern statistics",
	Long: `Show statistics about failure patterns.

Displays total patterns, top cascades, and data file locations.

Examples:
  klaudiush patterns stats`,
	RunE: runPatternsStats,
}

var patternsResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Clear learned patterns",
	Long: `Clear learned patterns while keeping seed data.

This removes all patterns learned from actual usage but preserves
the built-in seed patterns in the project-local file.

Examples:
  klaudiush patterns reset`,
	RunE: runPatternsReset,
}

func init() {
	rootCmd.AddCommand(patternsCmd)
	patternsCmd.AddCommand(patternsListCmd)
	patternsCmd.AddCommand(patternsStatsCmd)
	patternsCmd.AddCommand(patternsResetCmd)

	patternsListCmd.Flags().IntVar(
		&patternsMinCount,
		"min-count",
		0,
		"Filter patterns by minimum observation count",
	)

	patternsListCmd.Flags().StringVar(
		&patternsErrorCode,
		"error-code",
		"",
		"Filter patterns by source or target error code (e.g., GIT013)",
	)

	patternsListCmd.Flags().BoolVar(
		&patternsJSON,
		"json",
		false,
		"Output patterns as JSON",
	)
}

func runPatternsList(_ *cobra.Command, _ []string) error {
	store, _, err := setupPatternStore()
	if err != nil {
		return err
	}

	all := store.GetAllPatterns()

	// Filter
	filtered := filterPatterns(all, patternsErrorCode, patternsMinCount)

	// Sort by count descending
	slices.SortFunc(filtered, func(a, b *patterns.FailurePattern) int {
		return b.Count - a.Count
	})

	if patternsJSON {
		return outputPatternsJSON(filtered)
	}

	outputPatternsTable(filtered)

	return nil
}

func runPatternsStats(_ *cobra.Command, _ []string) error {
	store, cfg, err := setupPatternStore()
	if err != nil {
		return err
	}

	all := store.GetAllPatterns()

	fmt.Println("Failure Pattern Statistics")
	fmt.Println("==========================")
	fmt.Println("")
	fmt.Printf("Total patterns: %d\n", len(all))
	fmt.Printf("Project data file: %s\n", cfg.GetProjectDataFile())
	fmt.Printf("Global data dir: %s\n", cfg.GetGlobalDataDir())

	if len(all) == 0 {
		return nil
	}

	fmt.Println("")
	fmt.Println("Top patterns:")

	// Sort by count descending
	slices.SortFunc(all, func(a, b *patterns.FailurePattern) int {
		return b.Count - a.Count
	})

	limit := min(len(all), topPatternsLimit)

	for _, p := range all[:limit] {
		seedTag := ""
		if p.Seed {
			seedTag = " [seed]"
		}

		fmt.Printf("  %s -> %s  (count: %d)%s\n",
			p.SourceCode, p.TargetCode, p.Count, seedTag)
	}

	// Count seeds vs learned
	seedCount := 0
	learnedCount := 0

	for _, p := range all {
		if p.Seed {
			seedCount++
		} else {
			learnedCount++
		}
	}

	fmt.Println("")
	fmt.Printf("Seed patterns: %d\n", seedCount)
	fmt.Printf("Learned patterns: %d\n", learnedCount)

	return nil
}

func runPatternsReset(_ *cobra.Command, _ []string) error {
	store, cfg, err := setupPatternStore()
	if err != nil {
		return err
	}

	before := len(store.GetAllPatterns())

	// Clean up all global patterns by removing everything
	removed := store.Cleanup(0)

	if err := store.Save(); err != nil {
		return errors.Wrap(err, "saving pattern store after reset")
	}

	// Re-seed if configured
	if cfg.IsUseSeedData() {
		store.SetProjectData(patterns.SeedPatterns())

		if err := store.SaveProject(); err != nil {
			return errors.Wrap(err, "saving seed data")
		}
	}

	after := len(store.GetAllPatterns())

	fmt.Printf("Patterns reset complete\n")
	fmt.Printf("  Before: %d\n", before)
	fmt.Printf("  Removed (learned): %d\n", removed)
	fmt.Printf("  After (seeds): %d\n", after)

	return nil
}

func setupPatternStore() (*patterns.FilePatternStore, *config.PatternsConfig, error) {
	log, cfg, err := loadPatternsConfig()
	if err != nil {
		return nil, nil, err
	}

	patternsCfg := cfg.GetPatterns()

	projectDir, dirErr := os.Getwd()
	if dirErr != nil {
		return nil, nil, errors.Wrap(dirErr, "getting working directory")
	}

	store := patterns.NewFilePatternStore(patternsCfg, projectDir)
	if loadErr := store.Load(); loadErr != nil {
		log.Debug("failed to load pattern store", "error", loadErr)
	}

	return store, patternsCfg, nil
}

func loadPatternsConfig() (logger.Logger, *config.Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get home directory")
	}

	logFile := homeDir + "/.claude/hooks/dispatcher.log"

	log, err := logger.NewFileLogger(logFile, false, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create logger")
	}

	cfg, err := loadAuditConfig(log)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load configuration")
	}

	return log, cfg, nil
}

func filterPatterns(
	all []*patterns.FailurePattern,
	errorCode string,
	minCount int,
) []*patterns.FailurePattern {
	if errorCode == "" && minCount == 0 {
		return all
	}

	filtered := make([]*patterns.FailurePattern, 0, len(all))

	for _, p := range all {
		if minCount > 0 && p.Count < minCount {
			continue
		}

		if errorCode != "" {
			codeUpper := strings.ToUpper(errorCode)
			if !strings.EqualFold(p.SourceCode, codeUpper) &&
				!strings.EqualFold(p.TargetCode, codeUpper) {
				continue
			}
		}

		filtered = append(filtered, p)
	}

	return filtered
}

func outputPatternsJSON(patterns []*patterns.FailurePattern) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(patterns); err != nil {
		return errors.Wrap(err, "encoding JSON output")
	}

	return nil
}

func outputPatternsTable(all []*patterns.FailurePattern) {
	if len(all) == 0 {
		fmt.Println("No patterns found.")

		return
	}

	fmt.Printf("Found %d patterns:\n\n", len(all))

	for _, p := range all {
		seedTag := ""
		if p.Seed {
			seedTag = " [seed]"
		}

		fmt.Printf("%s -> %s  (count: %d, last: %s)%s\n",
			p.SourceCode,
			p.TargetCode,
			p.Count,
			p.LastSeen.Format("2006-01-02"),
			seedTag,
		)
	}
}
