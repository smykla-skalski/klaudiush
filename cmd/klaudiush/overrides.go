// Package main provides the CLI entry point for klaudiush.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// scopeGlobal is the display name for the global config scope.
const scopeGlobal = "global"

// Override command flags.
var (
	overrideReason   string
	overrideDuration string
	overrideGlobal   bool
	overrideForce    bool
	overrideAll      bool
	overrideExpired  bool
)

var overridesCmd = &cobra.Command{
	Use:   "overrides",
	Short: "List configured overrides",
	Long: `List configured overrides.

Examples:
  klaudiush overrides                 # List project overrides
  klaudiush overrides --global        # List global overrides
  klaudiush overrides --all           # List both global and project
  klaudiush overrides --expired       # Include expired entries`,
	RunE: runOverridesList,
}

var disableCmd = &cobra.Command{
	Use:   "disable TARGET...",
	Short: "Disable an error code or validator",
	Long: `Disable one or more error codes or validators.

Targets can be error codes (GIT014, SEC001) or validator names (git.commit, file.markdown).

Examples:
  klaudiush disable GIT014 --reason "tmp paths OK"
  klaudiush disable GIT014 SEC001 --reason "dev environment" --duration 30d
  klaudiush disable git.commit --reason "using commitlint" --global`,
	Args: cobra.MinimumNArgs(1),
	RunE: runDisable,
}

var enableCmd = &cobra.Command{
	Use:   "enable TARGET...",
	Short: "Re-enable an error code or validator",
	Long: `Re-enable one or more error codes or validators by removing their override entry.

Examples:
  klaudiush enable GIT014
  klaudiush enable GIT014 SEC001 --global`,
	Args: cobra.MinimumNArgs(1),
	RunE: runEnable,
}

func init() {
	rootCmd.AddCommand(overridesCmd)
	rootCmd.AddCommand(disableCmd)
	rootCmd.AddCommand(enableCmd)

	setupDisableFlags(disableCmd)

	enableCmd.Flags().
		BoolVar(&overrideGlobal, "global", false, "Remove from global config instead of project")

	overridesCmd.Flags().BoolVar(&overrideGlobal, "global", false, "Show only global overrides")
	overridesCmd.Flags().
		BoolVar(&overrideAll, "all", false, "Show both global and project overrides")
	overridesCmd.Flags().BoolVar(&overrideExpired, "expired", false, "Include expired entries")
}

func setupDisableFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(
		&overrideReason, "reason", "r", "", "Why this override exists",
	)
	cmd.Flags().StringVarP(
		&overrideDuration, "duration", "d", "", "Duration before expiry (e.g., 24h, 7d, 30d)",
	)
	cmd.Flags().
		BoolVar(&overrideGlobal, "global", false, "Write to global config instead of project")
	cmd.Flags().
		BoolVar(&overrideForce, "force", false, "Overwrite existing override without prompting")
}

func runDisable(_ *cobra.Command, args []string) error {
	return runOverridesAdd(args, true)
}

func runOverridesAdd(targets []string, disabled bool) error {
	// Warn about unknown targets
	for _, target := range targets {
		if !config.IsKnownTarget(target) {
			fmt.Fprintf(
				os.Stderr,
				"warning: %q is not a known error code or validator name\n",
				target,
			)
		}
	}

	// Parse duration
	var expiresAt string

	if overrideDuration != "" {
		dur, err := parseDuration(overrideDuration)
		if err != nil {
			return errors.Wrapf(err, "invalid duration %q", overrideDuration)
		}

		expiresAt = time.Now().UTC().Add(dur).Format(time.RFC3339)
	}

	// Load config
	cfg, err := loadOverrideConfig(overrideGlobal)
	if err != nil {
		return err
	}

	// Ensure overrides section exists
	if cfg.Overrides == nil {
		cfg.Overrides = &config.OverridesConfig{}
	}

	if cfg.Overrides.Entries == nil {
		cfg.Overrides.Entries = make(map[string]*config.OverrideEntry)
	}

	// Add entries
	for _, target := range targets {
		if _, exists := cfg.Overrides.Entries[target]; exists && !overrideForce {
			return errors.Errorf(
				"override for %q already exists (use --force to overwrite)",
				target,
			)
		}

		entry := &config.OverrideEntry{
			Disabled:   &disabled,
			Reason:     overrideReason,
			DisabledAt: time.Now().UTC().Format(time.RFC3339),
			ExpiresAt:  expiresAt,
			DisabledBy: "cli",
		}
		cfg.Overrides.Entries[target] = entry
	}

	// Write config
	if err := writeOverrideConfig(cfg, overrideGlobal); err != nil {
		return err
	}

	// Print confirmation
	action := "DISABLED"
	if !disabled {
		action = "ENABLED"
	}

	reasonSuffix := ""
	if overrideReason != "" {
		reasonSuffix = fmt.Sprintf(" [%s]", overrideReason)
	}

	for _, target := range targets {
		fmt.Printf("%s %s%s\n", target, action, reasonSuffix)

		if expiresAt != "" {
			fmt.Printf("  Expires: %s\n", expiresAt)
		}
	}

	scope := "project"
	if overrideGlobal {
		scope = scopeGlobal
	}

	fmt.Printf("\nWritten to %s config.\n", scope)

	return nil
}

func runOverridesList(_ *cobra.Command, _ []string) error {
	showProject := !overrideGlobal || overrideAll
	showGlobal := overrideGlobal || overrideAll

	if showProject {
		cfg, err := loadOverrideConfig(false)
		if err != nil {
			return err
		}

		displayScopedOverrides("project", cfg.Overrides)
	}

	if showGlobal {
		if showProject {
			fmt.Println("")
		}

		cfg, err := loadOverrideConfig(true)
		if err != nil {
			return err
		}

		displayScopedOverrides(scopeGlobal, cfg.Overrides)
	}

	return nil
}

func runEnable(_ *cobra.Command, args []string) error {
	cfg, err := loadOverrideConfig(overrideGlobal)
	if err != nil {
		return err
	}

	if cfg.Overrides == nil || len(cfg.Overrides.Entries) == 0 {
		return errors.New("no overrides configured")
	}

	removed := 0

	for _, target := range args {
		if _, exists := cfg.Overrides.Entries[target]; !exists {
			fmt.Fprintf(os.Stderr, "warning: no override found for %q\n", target)

			continue
		}

		delete(cfg.Overrides.Entries, target)

		removed++

		fmt.Printf("Removed override for %s\n", target)
	}

	if removed == 0 {
		return nil
	}

	// Clean up empty overrides section
	if len(cfg.Overrides.Entries) == 0 {
		cfg.Overrides = nil
	}

	if err := writeOverrideConfig(cfg, overrideGlobal); err != nil {
		return err
	}

	scope := "project"
	if overrideGlobal {
		scope = scopeGlobal
	}

	fmt.Printf("\nWritten to %s config.\n", scope)

	return nil
}

// loadOverrideConfig loads the config for the given scope (project or global) in isolation.
func loadOverrideConfig(global bool) (*config.Config, error) {
	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return nil, errors.Wrap(err, "creating config loader")
	}

	if global {
		return loadGlobalConfigOnly(loader)
	}

	cfg, _, err := loader.LoadProjectConfigOnly()
	if err != nil {
		return nil, errors.Wrap(err, "loading project config")
	}

	if cfg == nil {
		// No project config exists yet - return empty config
		return &config.Config{Version: config.CurrentConfigVersion}, nil
	}

	if cfg.Version == 0 {
		cfg.Version = config.CurrentConfigVersion
	}

	return cfg, nil
}

// loadGlobalConfigOnly loads only the global config file without merging defaults
// or other sources. Returns an empty config if no global file exists.
func loadGlobalConfigOnly(loader *internalconfig.KoanfLoader) (*config.Config, error) {
	globalPath := loader.GlobalConfigPath()

	if _, err := os.Stat(globalPath); os.IsNotExist(err) {
		return &config.Config{}, nil
	}

	// Use a fresh loader pointed at the global config dir as "project" dir
	// so LoadProjectConfigOnly reads the global file. Instead, just load it
	// with the full merged loader for reading purposes, then isolate.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "getting home directory")
	}

	// Create a loader with the global config dir as the work dir
	// so LoadProjectConfigOnly picks up the global config.toml
	globalDir := filepath.Join(homeDir, internalconfig.GlobalConfigDir)

	isolatedLoader, err := internalconfig.NewKoanfLoaderWithDirs(homeDir, globalDir)
	if err != nil {
		return nil, errors.Wrap(err, "creating isolated loader")
	}

	cfg, _, err := isolatedLoader.LoadProjectConfigOnly()
	if err != nil {
		return nil, errors.Wrap(err, "loading global config")
	}

	if cfg == nil {
		return &config.Config{Version: config.CurrentConfigVersion}, nil
	}

	if cfg.Version == 0 {
		cfg.Version = config.CurrentConfigVersion
	}

	return cfg, nil
}

// writeOverrideConfig writes the config back to the appropriate file.
func writeOverrideConfig(cfg *config.Config, global bool) error {
	writer := internalconfig.NewWriter()

	if global {
		return errors.Wrap(writer.WriteGlobal(cfg), "writing global config")
	}

	return errors.Wrap(writer.WriteProject(cfg), "writing project config")
}

// displayScopedOverrides renders override entries for a given scope.
func displayScopedOverrides(scope string, overrides *config.OverridesConfig) {
	fmt.Printf("Overrides (%s)\n", scope)

	const headerPadding = 13
	fmt.Println(strings.Repeat("=", len(scope)+headerPadding))
	fmt.Println("")

	if overrides == nil || len(overrides.Entries) == 0 {
		fmt.Println("  (none)")

		return
	}

	// Sort keys for stable output
	keys := make([]string, 0, len(overrides.Entries))

	for key := range overrides.Entries {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	displayed := 0

	for _, key := range keys {
		entry := overrides.Entries[key]

		// Skip expired unless --expired flag set
		if entry.IsExpired() && !overrideExpired {
			continue
		}

		displayScopedOverrideEntry(key, entry)

		displayed++
	}

	if displayed == 0 {
		fmt.Println("  (none active)")
	}
}

// displayScopedOverrideEntry renders a single override entry for the list command.
func displayScopedOverrideEntry(key string, entry *config.OverrideEntry) {
	// Status tag
	status := "DISABLED"
	if entry.Disabled != nil && !*entry.Disabled {
		status = "ENABLED"
	}

	// Active/expired tag
	state := "active"
	if entry.IsExpired() {
		state = "expired"
	}

	fmt.Printf("%s [%s] (%s)\n", key, status, state)

	if entry.Reason != "" {
		fmt.Printf("  Reason: %s\n", entry.Reason)
	}

	if entry.DisabledAt != "" {
		fmt.Printf("  Since: %s\n", entry.DisabledAt)
	}

	expires := "never"
	if entry.ExpiresAt != "" {
		expires = entry.ExpiresAt
	}

	fmt.Printf("  Expires: %s\n", expires)
	fmt.Println("")
}

// parseDuration parses a duration string supporting Go durations and day shorthand.
// Accepts "24h", "168h", "7d", "30d", etc.
func parseDuration(s string) (time.Duration, error) {
	// Handle day shorthand: "7d" -> 7*24h
	if daysStr, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, errors.Wrapf(err, "parsing day count from %q", s)
		}

		if days <= 0 {
			return 0, errors.Errorf("duration must be positive, got %d days", days)
		}

		const hoursPerDay = 24

		return time.Duration(days*hoursPerDay) * time.Hour, nil
	}

	dur, err := time.ParseDuration(s)
	if err != nil {
		return 0, errors.Wrap(err, "parsing duration")
	}

	if dur <= 0 {
		return 0, errors.New("duration must be positive")
	}

	return dur, nil
}
