// Package main provides the CLI entry point for klaudiush.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-labs/klaudiush/internal/backup"
	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// Backup command flags.
var (
	backupProject     string
	backupGlobal      bool
	backupAll         bool
	backupTag         string
	backupDescription string
	backupDryRun      bool
	backupForce       bool
	backupJSON        bool
	backupLimit       int
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage configuration backups",
	Long: `Manage configuration backups.

Create, restore, and manage automatic backups of klaudiush configuration files.

Subcommands:
  list     List available backups
  create   Create a manual backup
  restore  Restore a backup snapshot
  delete   Delete a backup snapshot
  prune    Remove old backups according to retention policy
  status   Show backup system status`,
}

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups",
	Long: `List available configuration backups.

Examples:
  klaudiush backup list                        # List all backups
  klaudiush backup list --global               # List global config backups
  klaudiush backup list --project /path        # List project config backups
  klaudiush backup list --all                  # List all backups (default)
  klaudiush backup list --limit 10             # Show last 10 backups
  klaudiush backup list --json                 # Output as JSON`,
	RunE: runBackupList,
}

var backupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a manual backup",
	Long: `Create a manual backup of a configuration file.

Examples:
  klaudiush backup create                                  # Backup current project config
  klaudiush backup create --global                         # Backup global config
  klaudiush backup create --tag "before-change"            # Backup with tag
  klaudiush backup create --description "Testing feature"  # Backup with description`,
	RunE: runBackupCreate,
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore SNAPSHOT_ID",
	Short: "Restore a backup snapshot",
	Long: `Restore a configuration file from a backup snapshot.

By default, creates a backup of the current config before restoring.
Use --force to skip the safety backup.

Examples:
  klaudiush backup restore abc123              # Restore with safety backup
  klaudiush backup restore abc123 --dry-run    # Preview restore operation
  klaudiush backup restore abc123 --force      # Restore without safety backup`,
	Args: cobra.ExactArgs(1),
	RunE: runBackupRestore,
}

var backupDeleteCmd = &cobra.Command{
	Use:   "delete SNAPSHOT_ID",
	Short: "Delete a backup snapshot",
	Long: `Delete a specific backup snapshot.

Examples:
  klaudiush backup delete abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runBackupDelete,
}

var backupPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old backups",
	Long: `Remove old backup snapshots according to retention policy.

Examples:
  klaudiush backup prune               # Remove old backups
  klaudiush backup prune --dry-run     # Preview what would be removed
  klaudiush backup prune --global      # Prune global config backups
  klaudiush backup prune --all         # Prune all backups (default)`,
	RunE: runBackupPrune,
}

var backupStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show backup system status",
	Long: `Show status of the backup system.

Displays storage usage, snapshot counts, and backup configuration.

Examples:
  klaudiush backup status`,
	RunE: runBackupStatus,
}

var backupAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Show backup audit log",
	Long: `Show audit log of backup operations.

Displays a chronological list of all backup operations with filters.

Examples:
  klaudiush backup audit                          # Show all audit entries
  klaudiush backup audit --operation create       # Show only create operations
  klaudiush backup audit --since "2025-01-01"     # Show entries since date
  klaudiush backup audit --snapshot abc123        # Show entries for snapshot
  klaudiush backup audit --success                # Show only successful operations
  klaudiush backup audit --failure                # Show only failed operations
  klaudiush backup audit --limit 20               # Show last 20 entries
  klaudiush backup audit --json                   # Output as JSON`,
	RunE: runBackupAudit,
}

// Audit command flags.
var (
	auditOperation string
	auditSince     string
	auditSnapshot  string
	auditSuccess   bool
	auditFailure   bool
)

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupDeleteCmd)
	backupCmd.AddCommand(backupPruneCmd)
	backupCmd.AddCommand(backupStatusCmd)
	backupCmd.AddCommand(backupAuditCmd)

	setupBackupListFlags()
	setupBackupCreateFlags()
	setupBackupRestoreFlags()
	setupBackupPruneFlags()
	setupBackupAuditFlags()
}

func setupBackupListFlags() {
	backupListCmd.Flags().
		StringVar(&backupProject, "project", "", "Filter backups for specific project path")
	backupListCmd.Flags().BoolVar(&backupGlobal, "global", false, "Show only global config backups")
	backupListCmd.Flags().BoolVar(&backupAll, "all", false, "Show all backups (default)")
	backupListCmd.Flags().
		IntVar(&backupLimit, "limit", 0, "Limit number of backups to show (0 = all)")
	backupListCmd.Flags().BoolVar(&backupJSON, "json", false, "Output backups as JSON")
}

func setupBackupCreateFlags() {
	backupCreateCmd.Flags().
		BoolVar(&backupGlobal, "global", false, "Backup global config (default: current project config)")
	backupCreateCmd.Flags().StringVar(&backupTag, "tag", "", "Optional tag for the backup")
	backupCreateCmd.Flags().
		StringVar(&backupDescription, "description", "", "Optional description for the backup")
}

func setupBackupRestoreFlags() {
	backupRestoreCmd.Flags().
		BoolVar(&backupDryRun, "dry-run", false, "Preview restore operation without making changes")
	backupRestoreCmd.Flags().
		BoolVar(&backupForce, "force", false, "Skip safety backup before restore")
}

func setupBackupPruneFlags() {
	backupPruneCmd.Flags().
		BoolVar(&backupDryRun, "dry-run", false, "Preview what would be removed without making changes")
	backupPruneCmd.Flags().
		BoolVar(&backupGlobal, "global", false, "Prune only global config backups")
	backupPruneCmd.Flags().
		StringVar(&backupProject, "project", "", "Prune backups for specific project path")
	backupPruneCmd.Flags().BoolVar(&backupAll, "all", false, "Prune all backups (default)")
}

func setupBackupAuditFlags() {
	backupAuditCmd.Flags().
		StringVar(&auditOperation, "operation", "", "Filter by operation type (create, restore, delete, prune)")
	backupAuditCmd.Flags().
		StringVar(&auditSince, "since", "", "Show entries since this time (RFC3339 format)")
	backupAuditCmd.Flags().
		StringVar(&auditSnapshot, "snapshot", "", "Filter by snapshot ID")
	backupAuditCmd.Flags().
		BoolVar(&auditSuccess, "success", false, "Show only successful operations")
	backupAuditCmd.Flags().
		BoolVar(&auditFailure, "failure", false, "Show only failed operations")
	backupAuditCmd.Flags().
		IntVar(&backupLimit, "limit", 0, "Limit number of entries to show (0 = all)")
	backupAuditCmd.Flags().BoolVar(&backupJSON, "json", false, "Output as JSON")
}

func runBackupList(_ *cobra.Command, _ []string) error {
	log, managers, err := setupBackupManagers()
	if err != nil {
		return err
	}

	log.Info("backup list command invoked",
		"project", backupProject,
		"global", backupGlobal,
		"all", backupAll,
		"limit", backupLimit,
		"json", backupJSON,
	)

	// Collect snapshots from all relevant managers
	var allSnapshots []backup.Snapshot

	for _, mgr := range managers {
		snapshots, listErr := mgr.List()
		if listErr != nil {
			log.Error("failed to list backups", "error", listErr)

			continue
		}

		allSnapshots = append(allSnapshots, snapshots...)
	}

	// Sort by timestamp descending (newest first)
	slices.SortFunc(allSnapshots, func(a, b backup.Snapshot) int {
		if a.Timestamp.After(b.Timestamp) {
			return -1
		}

		if a.Timestamp.Before(b.Timestamp) {
			return 1
		}

		return 0
	})

	// Apply limit
	if backupLimit > 0 && len(allSnapshots) > backupLimit {
		allSnapshots = allSnapshots[:backupLimit]
	}

	// Output
	if backupJSON {
		return outputBackupJSON(allSnapshots)
	}

	outputBackupTable(allSnapshots)

	return nil
}

func runBackupCreate(_ *cobra.Command, _ []string) error {
	log, managers, err := setupBackupManagers()
	if err != nil {
		return err
	}

	log.Info("backup create command invoked",
		"global", backupGlobal,
		"tag", backupTag,
		"description", backupDescription,
	)

	// Determine which config to backup
	var configPath string

	var configType backup.ConfigType

	if backupGlobal {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return errors.Wrap(homeErr, "failed to get home directory")
		}

		configPath = filepath.Join(
			homeDir,
			internalconfig.GlobalConfigDir,
			internalconfig.GlobalConfigFile,
		)
		configType = backup.ConfigTypeGlobal
	} else {
		workDir, workErr := os.Getwd()
		if workErr != nil {
			return errors.Wrap(workErr, "failed to get working directory")
		}

		configPath = filepath.Join(
			workDir,
			internalconfig.ProjectConfigDir,
			internalconfig.ProjectConfigFile,
		)
		configType = backup.ConfigTypeProject
	}

	// Check if config exists
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		return errors.Errorf("config file not found: %s", configPath)
	}

	// Find the appropriate manager
	var manager *backup.Manager

	for _, mgr := range managers {
		manager = mgr

		break
	}

	if manager == nil {
		return errors.New("no backup manager available")
	}

	// Create backup
	opts := backup.CreateBackupOptions{
		ConfigPath: configPath,
		Trigger:    backup.TriggerManual,
		Metadata: backup.SnapshotMetadata{
			Command:     "backup create",
			Tag:         backupTag,
			Description: backupDescription,
		},
	}

	snapshot, err := manager.CreateBackup(opts)
	if err != nil {
		return errors.Wrap(err, "failed to create backup")
	}

	fmt.Printf("✅ Backup created successfully\n")
	fmt.Printf("   Snapshot ID: %s\n", snapshot.ID)
	fmt.Printf("   Config Type: %s\n", configType)
	fmt.Printf("   Config Path: %s\n", snapshot.ConfigPath)
	fmt.Printf("   Size: %s\n", formatBytes(snapshot.Size))

	if snapshot.Metadata.Tag != "" {
		fmt.Printf("   Tag: %s\n", snapshot.Metadata.Tag)
	}

	if snapshot.Metadata.Description != "" {
		fmt.Printf("   Description: %s\n", snapshot.Metadata.Description)
	}

	return nil
}

func runBackupRestore(_ *cobra.Command, args []string) error {
	snapshotID := args[0]

	log, managers, err := setupBackupManagers()
	if err != nil {
		return err
	}

	log.Info("backup restore command invoked",
		"snapshotID", snapshotID,
		"dryRun", backupDryRun,
		"force", backupForce,
	)

	// Find the snapshot across all managers
	var snapshot *backup.Snapshot

	var manager *backup.Manager

	for _, mgr := range managers {
		s, getErr := mgr.Get(snapshotID)
		if getErr == nil {
			snapshot = s
			manager = mgr

			break
		}
	}

	if snapshot == nil {
		return errors.Errorf("snapshot not found: %s", snapshotID)
	}

	// Dry run mode
	if backupDryRun {
		fmt.Printf("📋 Dry run mode - no changes will be made\n\n")
		fmt.Printf("Would restore:\n")
		fmt.Printf("   Snapshot ID: %s\n", snapshot.ID)
		fmt.Printf("   Target Path: %s\n", snapshot.ConfigPath)
		fmt.Printf("   Size: %s\n", formatBytes(snapshot.Size))
		fmt.Printf("   Created: %s\n", snapshot.Timestamp.Format("2006-01-02 15:04:05"))

		if !backupForce {
			fmt.Printf("\nWould create safety backup of existing config\n")
		}

		return nil
	}

	// Restore snapshot
	opts := backup.RestoreOptions{
		TargetPath:          snapshot.ConfigPath,
		BackupBeforeRestore: !backupForce,
		Force:               backupForce,
		Validate:            true,
	}

	result, err := manager.RestoreSnapshot(snapshotID, opts)
	if err != nil {
		return errors.Wrap(err, "failed to restore snapshot")
	}

	fmt.Printf("✅ Snapshot restored successfully\n")
	fmt.Printf("   Restored to: %s\n", result.RestoredPath)
	fmt.Printf("   Bytes restored: %s\n", formatBytes(result.BytesRestored))

	if result.BackupSnapshot != nil {
		fmt.Printf("   Safety backup created: %s\n", result.BackupSnapshot.ID)
	}

	if result.ChecksumVerified {
		fmt.Printf("   Checksum: verified ✓\n")
	}

	return nil
}

func runBackupDelete(_ *cobra.Command, args []string) error {
	snapshotID := args[0]

	log, managers, err := setupBackupManagers()
	if err != nil {
		return err
	}

	log.Info("backup delete command invoked", "snapshotID", snapshotID)

	// Find the snapshot across all managers
	var snapshot *backup.Snapshot

	var targetStorage backup.Storage

	for _, mgr := range managers {
		s, getErr := mgr.Get(snapshotID)
		if getErr == nil {
			snapshot = s

			// Get storage from manager (we need to access it)
			// For now, we'll recreate the storage based on the snapshot
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return errors.Wrap(homeErr, "failed to get home directory")
			}

			baseDir := filepath.Join(homeDir, internalconfig.GlobalConfigDir)

			var projectPath string

			if snapshot.ConfigType == backup.ConfigTypeProject {
				projectPath = filepath.Dir(filepath.Dir(snapshot.ConfigPath))
			}

			storage, storageErr := backup.NewFilesystemStorage(
				baseDir,
				snapshot.ConfigType,
				projectPath,
			)
			if storageErr != nil {
				return errors.Wrap(storageErr, "failed to create storage")
			}

			targetStorage = storage

			break
		}
	}

	if snapshot == nil {
		return errors.Errorf("snapshot not found: %s", snapshotID)
	}

	// Load index
	index, err := targetStorage.LoadIndex()
	if err != nil {
		return errors.Wrap(err, "failed to load index")
	}

	// Delete from storage
	if err := targetStorage.Delete(snapshot.StoragePath); err != nil {
		return errors.Wrap(err, "failed to delete snapshot file")
	}

	// Delete from index
	if err := index.Delete(snapshot.ID); err != nil {
		return errors.Wrap(err, "failed to delete from index")
	}

	// Save index
	if err := targetStorage.SaveIndex(index); err != nil {
		return errors.Wrap(err, "failed to save index")
	}

	fmt.Printf("✅ Snapshot deleted successfully\n")
	fmt.Printf("   Snapshot ID: %s\n", snapshotID)
	fmt.Printf("   Freed: %s\n", formatBytes(snapshot.Size))

	return nil
}

func runBackupPrune(_ *cobra.Command, _ []string) error {
	log, managers, err := setupBackupManagers()
	if err != nil {
		return err
	}

	log.Info("backup prune command invoked",
		"dryRun", backupDryRun,
		"global", backupGlobal,
		"project", backupProject,
		"all", backupAll,
	)

	// Load config to get retention policy
	cfg, err := loadConfig(log)
	if err != nil {
		return errors.Wrap(err, "failed to load config")
	}

	backupCfg := cfg.GetBackup()
	if backupCfg == nil {
		backupCfg = &config.BackupConfig{}
	}

	// Create retention policy
	countPolicy, err := backup.NewCountRetentionPolicy(backupCfg.GetMaxBackups())
	if err != nil {
		return errors.Wrap(err, "failed to create count policy")
	}

	maxAge := backupCfg.GetMaxAge()

	agePolicy, err := backup.NewAgeRetentionPolicy(maxAge.ToDuration())
	if err != nil {
		return errors.Wrap(err, "failed to create age policy")
	}

	sizePolicy, err := backup.NewSizeRetentionPolicy(backupCfg.GetMaxSize())
	if err != nil {
		return errors.Wrap(err, "failed to create size policy")
	}

	policy := backup.NewCompositeRetentionPolicy(countPolicy, agePolicy, sizePolicy)

	// Dry run mode
	if backupDryRun {
		return runBackupPruneDryRun(policy, backupCfg, managers)
	}

	// Apply retention policy
	return runBackupPruneApply(policy, managers, log)
}

func runBackupPruneDryRun(
	policy backup.RetentionPolicy,
	backupCfg *config.BackupConfig,
	managers []*backup.Manager,
) error {
	fmt.Printf("📋 Dry run mode - no changes will be made\n\n")
	fmt.Printf("Retention policy:\n")
	fmt.Printf("   Max backups: %d\n", backupCfg.GetMaxBackups())
	fmt.Printf("   Max age: %s\n", backupCfg.GetMaxAge())
	fmt.Printf("   Max size: %s\n", formatBytes(backupCfg.GetMaxSize()))
	fmt.Printf("\nScanning backups...\n\n")

	totalRemoved := 0
	totalFreed := int64(0)

	for _, mgr := range managers {
		snapshots, listErr := mgr.List()
		if listErr != nil {
			continue
		}

		for _, snapshot := range snapshots {
			ctx := backup.RetentionContext{
				AllSnapshots: snapshots,
				Chain:        []backup.Snapshot{snapshot},
				TotalSize:    snapshot.Size,
				Now:          time.Now(),
			}

			if !policy.ShouldRetain(snapshot, ctx) {
				fmt.Printf("Would remove: %s (%s, %s)\n",
					snapshot.ID,
					snapshot.Timestamp.Format("2006-01-02"),
					formatBytes(snapshot.Size),
				)

				totalRemoved++
				totalFreed += snapshot.Size
			}
		}
	}

	if totalRemoved == 0 {
		fmt.Printf("No backups would be removed\n")
	} else {
		fmt.Printf("\nTotal: %d backups, %s freed\n", totalRemoved, formatBytes(totalFreed))
	}

	return nil
}

func runBackupPruneApply(
	policy backup.RetentionPolicy,
	managers []*backup.Manager,
	log logger.Logger,
) error {
	totalRemoved := 0
	totalFreed := int64(0)

	for _, mgr := range managers {
		result, applyErr := mgr.ApplyRetention(policy)
		if applyErr != nil {
			log.Error("failed to apply retention", "error", applyErr)

			continue
		}

		totalRemoved += result.SnapshotsRemoved
		totalFreed += result.BytesFreed
	}

	fmt.Printf("✅ Retention policy applied\n")
	fmt.Printf("   Snapshots removed: %d\n", totalRemoved)
	fmt.Printf("   Space freed: %s\n", formatBytes(totalFreed))

	return nil
}

func runBackupStatus(_ *cobra.Command, _ []string) error {
	log, managers, err := setupBackupManagers()
	if err != nil {
		return err
	}

	log.Info("backup status command invoked")

	cfg, err := loadConfig(log)
	if err != nil {
		return errors.Wrap(err, "failed to load config")
	}

	backupCfg := cfg.GetBackup()
	if backupCfg == nil {
		backupCfg = &config.BackupConfig{}
	}

	stats := collectBackupStats(managers)
	displayBackupStatus(backupCfg, stats)

	return nil
}

type backupStats struct {
	totalSnapshots   int
	globalSnapshots  int
	projectSnapshots int
	totalSize        int64
	chains           map[string]bool
}

func collectBackupStats(managers []*backup.Manager) backupStats {
	stats := backupStats{
		chains: make(map[string]bool),
	}

	for _, mgr := range managers {
		snapshots, listErr := mgr.List()
		if listErr != nil {
			continue
		}

		stats.totalSnapshots += len(snapshots)

		for _, snapshot := range snapshots {
			stats.totalSize += snapshot.Size
			stats.chains[snapshot.ChainID] = true

			if snapshot.ConfigType == backup.ConfigTypeGlobal {
				stats.globalSnapshots++
			} else {
				stats.projectSnapshots++
			}
		}
	}

	return stats
}

func displayBackupStatus(backupCfg *config.BackupConfig, stats backupStats) {
	fmt.Println("Backup System Status")
	fmt.Println("====================")
	fmt.Println("")

	displayBackupSystemStatus(backupCfg)
	displayStorageStats(stats)
	displayRetentionPolicy(backupCfg)
}

func displayBackupSystemStatus(backupCfg *config.BackupConfig) {
	fmt.Printf("System: ")

	if backupCfg.IsEnabled() {
		fmt.Printf("✅ Enabled\n")
	} else {
		fmt.Printf("❌ Disabled\n")
	}

	fmt.Printf("Auto-backup: ")

	if backupCfg.IsAutoBackupEnabled() {
		fmt.Printf("✅ Enabled\n")
	} else {
		fmt.Printf("❌ Disabled\n")
	}

	fmt.Printf("Async backup: ")

	if backupCfg.IsAsyncBackupEnabled() {
		fmt.Printf("✅ Enabled\n")
	} else {
		fmt.Printf("❌ Disabled\n")
	}

	fmt.Println("")
}

func displayStorageStats(stats backupStats) {
	fmt.Println("Storage")
	fmt.Println("-------")
	fmt.Printf("Total snapshots: %d\n", stats.totalSnapshots)
	fmt.Printf("  Global: %d\n", stats.globalSnapshots)
	fmt.Printf("  Project: %d\n", stats.projectSnapshots)
	fmt.Printf("Total size: %s\n", formatBytes(stats.totalSize))
	fmt.Printf("Chains: %d active\n", len(stats.chains))

	if stats.totalSnapshots > 0 {
		fmt.Printf("Avg size: %s\n", formatBytes(stats.totalSize/int64(stats.totalSnapshots)))
	}

	fmt.Println("")
}

func displayRetentionPolicy(backupCfg *config.BackupConfig) {
	fmt.Println("Retention Policy")
	fmt.Println("----------------")
	fmt.Printf("Max backups: %d per directory\n", backupCfg.GetMaxBackups())
	fmt.Printf("Max age: %s\n", backupCfg.GetMaxAge())
	fmt.Printf("Max size: %s total\n", formatBytes(backupCfg.GetMaxSize()))
}

func runBackupAudit(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(err, "failed to get home directory")
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, false, false)
	if err != nil {
		return errors.Wrap(err, "failed to create logger")
	}

	log.Info("backup audit command invoked",
		"operation", auditOperation,
		"since", auditSince,
		"snapshot", auditSnapshot,
		"success", auditSuccess,
		"failure", auditFailure,
		"limit", backupLimit,
		"json", backupJSON,
	)

	// Create audit logger
	baseDir := filepath.Join(homeDir, internalconfig.GlobalConfigDir, ".backups")

	auditLogger, err := backup.NewJSONLAuditLogger(baseDir)
	if err != nil {
		return errors.Wrap(err, "failed to create audit logger")
	}

	defer func() {
		if closeErr := auditLogger.Close(); closeErr != nil {
			log.Error("failed to close audit logger", "error", closeErr)
		}
	}()

	// Build filter
	filter := backup.AuditFilter{
		Operation:  auditOperation,
		SnapshotID: auditSnapshot,
		Limit:      backupLimit,
	}

	// Parse since time
	if auditSince != "" {
		sinceTime, parseErr := time.Parse(time.RFC3339, auditSince)
		if parseErr != nil {
			return errors.Wrapf(parseErr, "invalid since time format: %s", auditSince)
		}

		filter.Since = sinceTime
	}

	// Parse success/failure filter
	if auditSuccess && !auditFailure {
		success := true
		filter.Success = &success
	} else if auditFailure && !auditSuccess {
		success := false
		filter.Success = &success
	}

	// Query audit log
	entries, err := auditLogger.Query(filter)
	if err != nil {
		return errors.Wrap(err, "failed to query audit log")
	}

	// Output results
	if backupJSON {
		return outputBackupAuditJSON(entries)
	}

	outputBackupAuditTable(entries)

	return nil
}

func outputBackupAuditJSON(entries []backup.AuditEntry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(entries); err != nil {
		return errors.Wrap(err, "encoding JSON output")
	}

	return nil
}

func outputBackupAuditTable(entries []backup.AuditEntry) {
	if len(entries) == 0 {
		fmt.Println("No audit entries found.")

		return
	}

	fmt.Printf("Found %d audit entries:\n\n", len(entries))

	for _, entry := range entries {
		status := "✅"
		if !entry.Success {
			status = "❌"
		}

		fmt.Printf("%s %s  %s  %s\n",
			status,
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.Operation,
			entry.SnapshotID,
		)

		if entry.ConfigPath != "" {
			fmt.Printf("    Config: %s\n", entry.ConfigPath)
		}

		if entry.User != "" {
			fmt.Printf("    User: %s@%s\n", entry.User, entry.Hostname)
		}

		if !entry.Success && entry.Error != "" {
			fmt.Printf("    Error: %s\n", entry.Error)
		}

		if entry.Extra != nil {
			for key, value := range entry.Extra {
				fmt.Printf("    %s: %v\n", key, value)
			}
		}

		fmt.Println("")
	}
}

//nolint:ireturn // Logger interface return is intentional for flexibility
func setupBackupManagers() (logger.Logger, []*backup.Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get home directory")
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, false, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create logger")
	}

	// Load configuration
	cfg, err := loadConfig(log)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load configuration")
	}

	backupCfg := cfg.GetBackup()
	if backupCfg == nil {
		backupCfg = &config.BackupConfig{}
	}

	managers := make([]*backup.Manager, 0)
	baseDir := filepath.Join(homeDir, internalconfig.GlobalConfigDir)

	// Create manager for global config
	if !backupGlobal || backupAll || (!backupGlobal && backupProject == "") {
		globalStorage, storageErr := backup.NewFilesystemStorage(
			baseDir,
			backup.ConfigTypeGlobal,
			"",
		)
		if storageErr != nil {
			return nil, nil, errors.Wrap(storageErr, "failed to create global storage")
		}

		globalManager, mgrErr := backup.NewManager(globalStorage, backupCfg)
		if mgrErr != nil {
			return nil, nil, errors.Wrap(mgrErr, "failed to create global manager")
		}

		managers = append(managers, globalManager)
	}

	// Create manager for project config
	if !backupGlobal {
		projectPath := backupProject
		if projectPath == "" {
			projectPath, _ = os.Getwd()
		}

		projectStorage, storageErr := backup.NewFilesystemStorage(
			baseDir,
			backup.ConfigTypeProject,
			projectPath,
		)
		if storageErr != nil {
			return nil, nil, errors.Wrap(storageErr, "failed to create project storage")
		}

		projectManager, mgrErr := backup.NewManager(projectStorage, backupCfg)
		if mgrErr != nil {
			return nil, nil, errors.Wrap(mgrErr, "failed to create project manager")
		}

		managers = append(managers, projectManager)
	}

	return log, managers, nil
}

func outputBackupJSON(snapshots []backup.Snapshot) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(snapshots); err != nil {
		return errors.Wrap(err, "encoding JSON output")
	}

	return nil
}

func outputBackupTable(snapshots []backup.Snapshot) {
	if len(snapshots) == 0 {
		fmt.Println("No backups found.")

		return
	}

	fmt.Printf("Found %d backups:\n\n", len(snapshots))

	// Calculate total size
	var totalSize int64

	for _, snapshot := range snapshots {
		totalSize += snapshot.Size
	}

	// Display table
	for _, snapshot := range snapshots {
		typeStr := "FULL"
		if snapshot.StorageType == backup.StorageTypePatch {
			typeStr = "PATCH"
		}

		fmt.Printf("%s  %-5s  %s  %s  %s\n",
			snapshot.ID[:8],
			typeStr,
			snapshot.Timestamp.Format("2006-01-02 15:04:05"),
			formatBytes(snapshot.Size),
			snapshot.ChainID,
		)

		// Show additional info
		if snapshot.ConfigType == backup.ConfigTypeProject {
			projectPath := filepath.Dir(filepath.Dir(snapshot.ConfigPath))
			fmt.Printf("    Project: %s\n", projectPath)
		} else {
			fmt.Printf("    Config: global\n")
		}

		if snapshot.Metadata.Tag != "" {
			fmt.Printf("    Tag: %s\n", snapshot.Metadata.Tag)
		}

		if snapshot.Metadata.Description != "" {
			fmt.Printf("    Description: %s\n", snapshot.Metadata.Description)
		}

		if snapshot.Trigger != backup.TriggerAutomatic {
			fmt.Printf("    Trigger: %s\n", snapshot.Trigger)
		}

		fmt.Println("")
	}

	fmt.Printf("Total: %s storage\n", formatBytes(totalSize))
}

func formatBytes(bytes int64) string {
	const unit = 1024

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0

	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
