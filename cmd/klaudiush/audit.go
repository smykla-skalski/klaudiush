// Package main provides the CLI entry point for klaudiush.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/exceptions"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// Audit constants.
const (
	// maxCommandDisplayLen is the max length for command display before truncation.
	maxCommandDisplayLen = 60

	// truncatedCommandLen is the length of command shown when truncated.
	truncatedCommandLen = 57
)

// Audit command flags.
var (
	auditErrorCode string
	auditOutcome   string
	auditLimit     int
	auditJSON      bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Manage exception audit logs",
	Long: `Manage exception audit logs.

View, filter, and maintain the exception workflow audit trail.

Subcommands:
  list     List audit log entries
  stats    Show audit log statistics
  cleanup  Remove old entries and rotate logs`,
}

var auditListCmd = &cobra.Command{
	Use:   "list",
	Short: "List audit log entries",
	Long: `List audit log entries with optional filtering.

Examples:
  klaudiush audit list                          # List all entries
  klaudiush audit list --error-code GIT022      # Filter by error code
  klaudiush audit list --outcome allowed        # Filter by outcome
  klaudiush audit list --limit 10               # Show last 10 entries
  klaudiush audit list --json                   # Output as JSON`,
	RunE: runAuditList,
}

var auditStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show audit log statistics",
	Long: `Show statistics about the exception audit log.

Displays file size, entry count, backup count, and usage by error code.

Examples:
  klaudiush audit stats`,
	RunE: runAuditStats,
}

var auditCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up old audit entries",
	Long: `Remove audit entries older than the configured retention period.

Also rotates the log file if it exceeds the maximum size.

Examples:
  klaudiush audit cleanup`,
	RunE: runAuditCleanup,
}

func init() {
	rootCmd.AddCommand(auditCmd)
	auditCmd.AddCommand(auditListCmd)
	auditCmd.AddCommand(auditStatsCmd)
	auditCmd.AddCommand(auditCleanupCmd)

	auditListCmd.Flags().StringVar(
		&auditErrorCode,
		"error-code",
		"",
		"Filter entries by error code (e.g., GIT022, SEC001)",
	)

	auditListCmd.Flags().StringVar(
		&auditOutcome,
		"outcome",
		"",
		"Filter entries by outcome (allowed, denied)",
	)

	auditListCmd.Flags().IntVar(
		&auditLimit,
		"limit",
		0,
		"Limit number of entries to show (0 = all)",
	)

	auditListCmd.Flags().BoolVar(
		&auditJSON,
		"json",
		false,
		"Output entries as JSON",
	)
}

func runAuditList(_ *cobra.Command, _ []string) error {
	log, auditLogger, err := setupAuditLogger()
	if err != nil {
		return err
	}

	log.Info("audit list command invoked",
		"errorCode", auditErrorCode,
		"outcome", auditOutcome,
		"limit", auditLimit,
		"json", auditJSON,
	)

	entries, err := auditLogger.Read()
	if err != nil {
		return errors.Wrap(err, "reading audit log")
	}

	// Filter entries
	filtered := filterAuditEntries(entries, auditErrorCode, auditOutcome)

	// Sort by timestamp descending (newest first)
	slices.SortFunc(filtered, func(a, b *exceptions.AuditEntry) int {
		if a.Timestamp.After(b.Timestamp) {
			return -1
		}

		if a.Timestamp.Before(b.Timestamp) {
			return 1
		}

		return 0
	})

	// Apply limit
	if auditLimit > 0 && len(filtered) > auditLimit {
		filtered = filtered[:auditLimit]
	}

	// Output
	if auditJSON {
		return outputAuditJSON(filtered)
	}

	outputAuditTable(filtered)

	return nil
}

func runAuditStats(_ *cobra.Command, _ []string) error {
	log, auditLogger, err := setupAuditLogger()
	if err != nil {
		return err
	}

	log.Info("audit stats command invoked")

	stats, err := auditLogger.Stats()
	if err != nil {
		return errors.Wrap(err, "getting audit stats")
	}

	// Read entries for detailed stats
	entries, err := auditLogger.Read()
	if err != nil {
		return errors.Wrap(err, "reading audit log")
	}

	displayAuditStats(stats, entries)

	return nil
}

func runAuditCleanup(_ *cobra.Command, _ []string) error {
	log, auditLogger, err := setupAuditLogger()
	if err != nil {
		return err
	}

	log.Info("audit cleanup command invoked")

	// Get stats before cleanup
	statsBefore, err := auditLogger.Stats()
	if err != nil {
		return errors.Wrap(err, "getting audit stats")
	}

	entriesBefore := statsBefore.EntryCount

	// Run cleanup
	if cleanupErr := auditLogger.Cleanup(); cleanupErr != nil {
		return errors.Wrap(cleanupErr, "cleaning up audit log")
	}

	// Get stats after cleanup
	statsAfter, err := auditLogger.Stats()
	if err != nil {
		return errors.Wrap(err, "getting audit stats")
	}

	entriesAfter := statsAfter.EntryCount
	removed := entriesBefore - entriesAfter

	fmt.Printf("✅ Cleanup complete\n")
	fmt.Printf("   Entries before: %d\n", entriesBefore)
	fmt.Printf("   Entries after: %d\n", entriesAfter)
	fmt.Printf("   Entries removed: %d\n", removed)
	fmt.Printf("   Backup files: %d\n", statsAfter.BackupCount)

	return nil
}

//nolint:ireturn // Logger interface return is intentional for flexibility
func setupAuditLogger() (logger.Logger, *exceptions.AuditLogger, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get home directory")
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, false, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create logger")
	}

	cfg, err := loadAuditConfig(log)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load configuration")
	}

	var auditCfg *config.ExceptionAuditConfig
	if exc := cfg.GetExceptions(); exc != nil {
		auditCfg = exc.Audit
	}

	auditLogger := exceptions.NewAuditLogger(auditCfg, exceptions.WithAuditLoggerLogger(log))

	return log, auditLogger, nil
}

func loadAuditConfig(log logger.Logger) (*config.Config, error) {
	flags := buildFlagsMap()

	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config loader")
	}

	cfg, err := loader.Load(flags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}

	log.Debug("configuration loaded for audit")

	return cfg, nil
}

func filterAuditEntries(
	entries []*exceptions.AuditEntry,
	errorCode, outcome string,
) []*exceptions.AuditEntry {
	if errorCode == "" && outcome == "" {
		return entries
	}

	filtered := make([]*exceptions.AuditEntry, 0, len(entries))

	for _, entry := range entries {
		if errorCode != "" && !strings.EqualFold(entry.ErrorCode, errorCode) {
			continue
		}

		if outcome != "" {
			wantAllowed := strings.EqualFold(outcome, "allowed")
			if entry.Allowed != wantAllowed {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

func outputAuditJSON(entries []*exceptions.AuditEntry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(entries); err != nil {
		return errors.Wrap(err, "encoding JSON output")
	}

	return nil
}

func outputAuditTable(entries []*exceptions.AuditEntry) {
	if len(entries) == 0 {
		fmt.Println("No audit entries found.")

		return
	}

	fmt.Printf("Found %d entries:\n\n", len(entries))

	for _, entry := range entries {
		outcomeStr := "❌ DENIED"
		if entry.Allowed {
			outcomeStr = "✅ ALLOWED"
		}

		fmt.Printf("%s  %s  %s\n",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.ErrorCode,
			outcomeStr,
		)

		if entry.ValidatorName != "" {
			fmt.Printf("    Validator: %s\n", entry.ValidatorName)
		}

		if entry.Reason != "" {
			fmt.Printf("    Reason: %s\n", entry.Reason)
		}

		if entry.DenialReason != "" {
			fmt.Printf("    Denial: %s\n", entry.DenialReason)
		}

		if entry.Source != "" {
			fmt.Printf("    Source: %s\n", entry.Source)
		}

		if entry.Command != "" {
			cmd := entry.Command
			if len(cmd) > maxCommandDisplayLen {
				cmd = cmd[:truncatedCommandLen] + "..."
			}

			fmt.Printf("    Command: %s\n", cmd)
		}

		fmt.Println("")
	}
}

func displayAuditStats(stats *exceptions.AuditStats, entries []*exceptions.AuditEntry) {
	fmt.Println("Audit Log Statistics")
	fmt.Println("====================")
	fmt.Println("")

	fmt.Printf("Log File: %s\n", stats.LogFile)
	fmt.Printf("File Size: %s\n", stats.FormatSize())
	fmt.Printf("Entry Count: %d\n", stats.EntryCount)
	fmt.Printf("Backup Files: %d\n", stats.BackupCount)

	if !stats.ModTime.IsZero() {
		fmt.Printf("Last Modified: %s\n", stats.ModTime.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("")

	// Usage breakdown
	if len(entries) > 0 {
		displayUsageBreakdown(entries)
	}
}

func displayUsageBreakdown(entries []*exceptions.AuditEntry) {
	fmt.Println("Usage Breakdown")
	fmt.Println("---------------")

	// Count by error code
	codeCount := make(map[string]int)
	allowedCount := 0
	deniedCount := 0

	var oldest, newest time.Time

	for _, entry := range entries {
		codeCount[entry.ErrorCode]++

		if entry.Allowed {
			allowedCount++
		} else {
			deniedCount++
		}

		if oldest.IsZero() || entry.Timestamp.Before(oldest) {
			oldest = entry.Timestamp
		}

		if newest.IsZero() || entry.Timestamp.After(newest) {
			newest = entry.Timestamp
		}
	}

	fmt.Printf("Total: %d (✅ %d allowed, ❌ %d denied)\n", len(entries), allowedCount, deniedCount)

	if !oldest.IsZero() {
		fmt.Printf("Date Range: %s to %s\n",
			oldest.Format("2006-01-02"),
			newest.Format("2006-01-02"),
		)
	}

	fmt.Println("")
	fmt.Println("By Error Code:")

	codes := make([]string, 0, len(codeCount))
	for code := range codeCount {
		codes = append(codes, code)
	}

	slices.Sort(codes)

	for _, code := range codes {
		fmt.Printf("  %s: %d\n", code, codeCount[code])
	}
}
