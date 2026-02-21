// Package main provides the CLI entry point for klaudiush.
package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	maxTopPatterns   = 10
	sessionIDTruncAt = 16
)

var verbosePatterns bool

var debugPatternsCmd = &cobra.Command{
	Use:   "patterns",
	Short: "Show pattern learning status",
	Long: `Show pattern learning status and learned failure patterns.

Displays pattern tracking configuration, seed and learned pattern counts,
active sessions, and top learned patterns by observation count.

Examples:
  klaudiush debug patterns            # Show summary
  klaudiush debug patterns --verbose  # Show all patterns and sessions`,
	RunE: runDebugPatterns,
}

func init() {
	debugPatternsCmd.Flags().BoolVar(
		&verbosePatterns,
		"verbose",
		false,
		"Show all patterns and active sessions",
	)
}

func runDebugPatterns(cmd *cobra.Command, _ []string) error {
	cfg, err := setupDebugContext(
		loggerFromCmd(cmd),
		"debug patterns",
		"verbose",
		strconv.FormatBool(verbosePatterns),
	)
	if err != nil {
		return err
	}

	patternsCfg := cfg.GetPatterns()

	workDir, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "getting working directory")
	}

	store := patterns.NewFilePatternStore(patternsCfg, workDir)
	if loadErr := store.Load(); loadErr != nil {
		return errors.Wrap(loadErr, "loading pattern store")
	}

	displayPatternStatus(patternsCfg, store)

	return nil
}

func displayPatternStatus(cfg *config.PatternsConfig, store *patterns.FilePatternStore) {
	fmt.Println("Pattern Learning")
	fmt.Println("================")
	fmt.Println("")

	displayPatternConfig(cfg)

	all := store.GetAllPatterns()
	seedCount, learnedCount := countPatternTypes(all)
	activeSessions := store.GetActiveSessions()

	fmt.Printf("Seed Patterns: %d\n", seedCount)
	fmt.Printf("Learned Patterns: %d\n", learnedCount)
	fmt.Printf("Active Sessions: %d\n", activeSessions)
	fmt.Println("")

	if verbosePatterns {
		displayAllPatterns(all)
		displayActiveSessions(store)
	} else {
		displayTopLearned(all)
	}
}

func displayPatternConfig(cfg *config.PatternsConfig) {
	enabledStr := statusEnabled
	if !cfg.IsEnabled() {
		enabledStr = statusDisabled
	}

	fmt.Printf("Status: %s\n", enabledStr)
	fmt.Printf("Min Count: %d\n", cfg.GetMinCount())
	fmt.Printf("Max Age: %s\n", formatDuration(cfg.GetMaxAge()))
	fmt.Printf("Session Max Age: %s\n", formatDuration(cfg.GetSessionMaxAge()))
	fmt.Println("")
}

func countPatternTypes(all []*patterns.FailurePattern) (seed, learned int) {
	for _, p := range all {
		if p.Seed {
			seed++
		} else {
			learned++
		}
	}

	return seed, learned
}

func displayTopLearned(all []*patterns.FailurePattern) {
	learned := filterLearned(all)
	if len(learned) == 0 {
		return
	}

	fmt.Println("Top Learned Patterns:")

	limit := min(maxTopPatterns, len(learned))

	for i := range limit {
		p := learned[i]
		fmt.Printf("  %s -> %s  (count: %d, last: %s)\n",
			p.SourceCode, p.TargetCode, p.Count,
			p.LastSeen.Format("2006-01-02"))
	}

	if len(learned) > limit {
		fmt.Printf("  ... and %d more (use --verbose to see all)\n", len(learned)-limit)
	}

	fmt.Println("")
}

func displayAllPatterns(all []*patterns.FailurePattern) {
	if len(all) == 0 {
		fmt.Println("No patterns recorded.")
		fmt.Println("")

		return
	}

	// Separate seeds and learned
	var seeds, learned []*patterns.FailurePattern

	for _, p := range all {
		if p.Seed {
			seeds = append(seeds, p)
		} else {
			learned = append(learned, p)
		}
	}

	if len(seeds) > 0 {
		fmt.Println("Seed Patterns:")

		sortByCount(seeds)

		for _, p := range seeds {
			fmt.Printf("  %s -> %s  (count: %d)\n",
				p.SourceCode, p.TargetCode, p.Count)
		}

		fmt.Println("")
	}

	if len(learned) > 0 {
		fmt.Println("Learned Patterns:")

		sortByCount(learned)

		for _, p := range learned {
			fmt.Printf("  %s -> %s  (count: %d, first: %s, last: %s)\n",
				p.SourceCode, p.TargetCode, p.Count,
				p.FirstSeen.Format("2006-01-02"),
				p.LastSeen.Format("2006-01-02"))
		}

		fmt.Println("")
	}
}

func displayActiveSessions(store *patterns.FilePatternStore) {
	sessions := store.GetSessions()
	if len(sessions) == 0 {
		fmt.Println("No active sessions.")
		fmt.Println("")

		return
	}

	fmt.Println("Active Sessions:")

	ids := make([]string, 0, len(sessions))
	for id := range sessions {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	for _, id := range ids {
		entry := sessions[id]
		truncatedID := id

		if len(truncatedID) > sessionIDTruncAt {
			truncatedID = truncatedID[:sessionIDTruncAt] + "..."
		}

		fmt.Printf("  %s  codes: [%s]  last: %s\n",
			truncatedID,
			strings.Join(entry.Codes, ", "),
			entry.LastSeen.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("")
}

func filterLearned(all []*patterns.FailurePattern) []*patterns.FailurePattern {
	var learned []*patterns.FailurePattern

	for _, p := range all {
		if !p.Seed {
			learned = append(learned, p)
		}
	}

	sortByCount(learned)

	return learned
}

func sortByCount(ps []*patterns.FailurePattern) {
	slices.SortFunc(ps, func(a, b *patterns.FailurePattern) int {
		if a.Count != b.Count {
			return b.Count - a.Count // descending
		}

		return strings.Compare(a.SourceCode, b.SourceCode)
	})
}
