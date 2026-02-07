// Package main provides the CLI entry point for klaudiush.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dustin/go-humanize"
	"github.com/hako/durafmt"
	"github.com/spf13/cobra"

	"github.com/smykla-labs/klaudiush/internal/crashdump"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

const (
	unlimitedStr         = "unlimited"
	durationDisplayUnits = 2
)

var dryRun bool

var debugCrashCmd = &cobra.Command{
	Use:   "crash",
	Short: "Manage crash dumps",
	Long: `Manage crash dumps created by klaudiush on panic.

Subcommands:
  list   List crash dumps
  view   View crash dump details
  clean  Remove old crash dumps`,
}

var debugCrashListCmd = &cobra.Command{
	Use:   "list",
	Short: "List crash dumps",
	Long: `List all crash dumps with timestamps and panic values.

Displays crash dumps sorted by timestamp (newest first).

Examples:
  klaudiush debug crash list`,
	RunE: runDebugCrashList,
}

var debugCrashViewCmd = &cobra.Command{
	Use:   "view <id>",
	Short: "View crash dump details",
	Long: `View detailed information about a specific crash dump.

Displays full crash information including stack trace, runtime info,
hook context, and sanitized configuration.

Examples:
  klaudiush debug crash view crash-20251204T160432-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: runDebugCrashView,
}

var debugCrashCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove old crash dumps",
	Long: `Remove old crash dumps based on retention policy.

Removes crash dumps that exceed the configured maximum count or age.
Uses configuration from crash_dump.max_dumps and crash_dump.max_age.

Examples:
  klaudiush debug crash clean            # Clean based on config
  klaudiush debug crash clean --dry-run  # Show what would be removed`,
	RunE: runDebugCrashClean,
}

func init() {
	debugCrashCleanCmd.Flags().BoolVar(
		&dryRun,
		"dry-run",
		false,
		"Show what would be removed without actually deleting",
	)
}

func runDebugCrashList(_ *cobra.Command, _ []string) error {
	cfg, err := setupDebugContext("debug crash list", "", "")
	if err != nil {
		return err
	}

	return displayCrashList(cfg)
}

func runDebugCrashView(_ *cobra.Command, args []string) error {
	cfg, err := setupDebugContext("debug crash view", "id", args[0])
	if err != nil {
		return err
	}

	return displayCrashDump(cfg, args[0])
}

func runDebugCrashClean(_ *cobra.Command, _ []string) error {
	dryRunStr := "false"
	if dryRun {
		dryRunStr = "true"
	}

	cfg, err := setupDebugContext("debug crash clean", "dryRun", dryRunStr)
	if err != nil {
		return err
	}

	return cleanCrashDumps(cfg, dryRun)
}

func displayCrashList(cfg *config.Config) error {
	crashCfg := cfg.GetCrashDump()

	storage, err := crashdump.NewFilesystemStorage(crashCfg.GetDumpDir())
	if err != nil {
		return errors.Wrap(err, "failed to create storage")
	}

	if !storage.Exists() {
		fmt.Println("No crash dumps found.")
		fmt.Println("")
		fmt.Printf("Crash dumps will be stored in: %s\n", crashCfg.GetDumpDir())
		fmt.Println("")
		fmt.Println("Crash dumps are created automatically when klaudiush panics.")

		return nil
	}

	summaries, err := storage.List()
	if err != nil {
		return errors.Wrap(err, "failed to list crash dumps")
	}

	if len(summaries) == 0 {
		fmt.Println("No crash dumps found.")
		fmt.Println("")
		fmt.Printf("Directory: %s\n", crashCfg.GetDumpDir())

		return nil
	}

	// Header
	fmt.Println("Crash Dumps")
	fmt.Println("===========")
	fmt.Println("")
	fmt.Printf("Directory: %s\n", crashCfg.GetDumpDir())
	fmt.Printf("Total: %d\n", len(summaries))
	fmt.Println("")

	// List dumps
	for i, summary := range summaries {
		displaySummary(i+1, &summary)
	}

	// Footer
	fmt.Println("Commands:")
	fmt.Println("  klaudiush debug crash view <id>    # View full details")
	fmt.Println("  klaudiush debug crash clean         # Remove old dumps")

	return nil
}

func displaySummary(index int, summary *crashdump.DumpSummary) {
	timestamp := summary.Timestamp.Format("2006-01-02 15:04:05")

	// Safe conversion: only convert non-negative sizes to uint64
	size := "unknown"
	if summary.Size >= 0 {
		size = humanize.Bytes(uint64(summary.Size))
	}

	fmt.Printf("%d. %s\n", index, summary.ID)
	fmt.Printf("   Time: %s\n", timestamp)
	fmt.Printf("   Panic: %s\n", summary.PanicValue)
	fmt.Printf("   Size: %s\n", size)
	fmt.Println("")
}

func displayCrashDump(cfg *config.Config, id string) error {
	crashCfg := cfg.GetCrashDump()

	storage, err := crashdump.NewFilesystemStorage(crashCfg.GetDumpDir())
	if err != nil {
		return errors.Wrap(err, "failed to create storage")
	}

	info, err := storage.Get(id)
	if err != nil {
		if errors.Is(err, crashdump.ErrDumpNotFound) {
			fmt.Printf("Crash dump not found: %s\n", id)
			fmt.Println("")
			fmt.Println("Use 'klaudiush debug crash list' to see available dumps.")

			return nil
		}

		return errors.Wrap(err, "failed to get crash dump")
	}

	// Header
	fmt.Println("Crash Dump Details")
	fmt.Println("==================")
	fmt.Println("")

	// Basic info
	fmt.Printf("ID: %s\n", info.ID)
	fmt.Printf("Timestamp: %s\n", info.Timestamp.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Panic Value: %s\n", info.PanicValue)
	fmt.Println("")

	// Runtime info
	displayRuntimeInfo(&info.Runtime)

	// Metadata
	displayMetadata(&info.Metadata)

	// Context
	if info.Context != nil {
		displayContextInfo(info.Context)
	}

	// Config
	if len(info.Config) > 0 {
		displayConfigSnapshot(info.Config)
	}

	// Stack trace (last, as it's typically long)
	displayStackTrace(info.StackTrace)

	return nil
}

func displayRuntimeInfo(runtime *crashdump.RuntimeInfo) {
	fmt.Println("Runtime")
	fmt.Println("-------")
	fmt.Printf("  Go Version: %s\n", runtime.GoVersion)
	fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  NumCPU: %d\n", runtime.NumCPU)
	fmt.Printf("  NumGoroutine: %d\n", runtime.NumGoroutine)
	fmt.Println("")
}

func displayMetadata(metadata *crashdump.DumpMetadata) {
	fmt.Println("Metadata")
	fmt.Println("--------")

	if metadata.Version != "" {
		fmt.Printf("  Version: %s\n", metadata.Version)
	}

	if metadata.User != "" {
		fmt.Printf("  User: %s\n", metadata.User)
	}

	if metadata.Hostname != "" {
		fmt.Printf("  Hostname: %s\n", metadata.Hostname)
	}

	if metadata.WorkingDir != "" {
		fmt.Printf("  Working Dir: %s\n", metadata.WorkingDir)
	}

	fmt.Println("")
}

func displayContextInfo(ctx *crashdump.ContextInfo) {
	fmt.Println("Hook Context")
	fmt.Println("------------")
	fmt.Printf("  Event Type: %s\n", ctx.EventType)
	fmt.Printf("  Tool Name: %s\n", ctx.ToolName)

	if ctx.Command != "" {
		fmt.Printf("  Command: %s\n", ctx.Command)
	}

	if ctx.FilePath != "" {
		fmt.Printf("  File Path: %s\n", ctx.FilePath)
	}

	fmt.Println("")
}

func displayConfigSnapshot(cfg map[string]any) {
	fmt.Println("Configuration Snapshot")
	fmt.Println("---------------------")

	// Format as indented JSON for readability
	data, err := json.MarshalIndent(cfg, "  ", "  ")
	if err != nil {
		fmt.Printf("  (failed to format config: %v)\n", err)
	} else {
		fmt.Printf("  %s\n", data)
	}

	fmt.Println("")
}

func displayStackTrace(stackTrace string) {
	fmt.Println("Stack Trace")
	fmt.Println("-----------")

	// Split by newlines and indent each line
	lines := strings.SplitSeq(stackTrace, "\n")
	for line := range lines {
		if line != "" {
			fmt.Printf("  %s\n", line)
		}
	}

	fmt.Println("")
}

func cleanCrashDumps(cfg *config.Config, isDryRun bool) error {
	crashCfg := cfg.GetCrashDump()

	storage, err := crashdump.NewFilesystemStorage(crashCfg.GetDumpDir())
	if err != nil {
		return errors.Wrap(err, "failed to create storage")
	}

	if !storage.Exists() {
		fmt.Println("No crash dumps directory found.")

		return nil
	}

	maxDumps := crashCfg.GetMaxDumps()
	maxAge := crashCfg.GetMaxAge().ToDuration()

	if isDryRun {
		return dryRunClean(storage, maxDumps, maxAge)
	}

	removed, err := storage.Prune(maxDumps, maxAge)
	if err != nil {
		return errors.Wrap(err, "failed to prune crash dumps")
	}

	// Display results
	fmt.Println("Crash Dumps Cleanup")
	fmt.Println("===================")
	fmt.Println("")
	fmt.Printf("Directory: %s\n", crashCfg.GetDumpDir())
	fmt.Printf("Retention: %d dumps, %s age\n", maxDumps, formatDuration(maxAge))
	fmt.Println("")
	fmt.Printf("Removed: %d dump(s)\n", removed)

	return nil
}

func dryRunClean(storage crashdump.Storage, maxDumps int, maxAge time.Duration) error {
	summaries, err := storage.List()
	if err != nil {
		return errors.Wrap(err, "failed to list crash dumps")
	}

	if len(summaries) == 0 {
		fmt.Println("No crash dumps found.")

		return nil
	}

	now := time.Now()
	toRemove := determineRemovableDumps(summaries, maxDumps, maxAge, now)

	displayDryRunResults(toRemove, maxDumps, maxAge, now)

	return nil
}

func determineRemovableDumps(
	summaries []crashdump.DumpSummary,
	maxDumps int,
	maxAge time.Duration,
	now time.Time,
) []crashdump.DumpSummary {
	toRemove := make([]crashdump.DumpSummary, 0)

	// Add dumps that exceed age limit
	toRemove = appendOldDumps(summaries, maxAge, now, toRemove)

	// Add dumps that exceed count limit
	toRemove = appendExcessDumps(summaries, maxDumps, toRemove)

	return toRemove
}

func appendOldDumps(
	summaries []crashdump.DumpSummary,
	maxAge time.Duration,
	now time.Time,
	toRemove []crashdump.DumpSummary,
) []crashdump.DumpSummary {
	if maxAge <= 0 {
		return toRemove
	}

	for _, summary := range summaries {
		if now.Sub(summary.Timestamp) > maxAge {
			toRemove = append(toRemove, summary)
		}
	}

	return toRemove
}

func appendExcessDumps(
	summaries []crashdump.DumpSummary,
	maxDumps int,
	toRemove []crashdump.DumpSummary,
) []crashdump.DumpSummary {
	if len(summaries) <= maxDumps {
		return toRemove
	}

	for i := maxDumps; i < len(summaries); i++ {
		if !isAlreadyMarked(summaries[i], toRemove) {
			toRemove = append(toRemove, summaries[i])
		}
	}

	return toRemove
}

func isAlreadyMarked(summary crashdump.DumpSummary, toRemove []crashdump.DumpSummary) bool {
	for _, removed := range toRemove {
		if removed.ID == summary.ID {
			return true
		}
	}

	return false
}

func displayDryRunResults(
	toRemove []crashdump.DumpSummary,
	maxDumps int,
	maxAge time.Duration,
	now time.Time,
) {
	fmt.Println("Crash Dumps Cleanup (Dry Run)")
	fmt.Println("=============================")
	fmt.Println("")
	fmt.Printf("Retention: %d dumps, %s age\n", maxDumps, formatDuration(maxAge))
	fmt.Println("")

	if len(toRemove) == 0 {
		fmt.Println("No dumps would be removed.")

		return
	}

	fmt.Printf("Would remove %d dump(s):\n", len(toRemove))
	fmt.Println("")

	for _, summary := range toRemove {
		age := now.Sub(summary.Timestamp)
		fmt.Printf("  - %s (age: %s)\n", summary.ID, formatDuration(age))
	}

	fmt.Println("")
	fmt.Println("Run without --dry-run to actually remove these dumps.")
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return unlimitedStr
	}

	return durafmt.Parse(d).LimitFirstN(durationDisplayUnits).String()
}
