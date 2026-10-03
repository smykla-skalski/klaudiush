// Package main provides the CLI entry point for klaudiush.
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Bypass command flags.
var (
	bypassReason   string
	bypassDuration string
	bypassGlobal   bool
	bypassAll      bool
)

var bypassCmd = &cobra.Command{
	Use:   "bypass",
	Short: "Control validation when approval prompts are off",
	Long: `Control validation when the session runs without approval prompts.

klaudiush validates every hook by default, including sessions started with
--dangerously-skip-permissions (Claude), --dangerously-bypass-approvals-and-sandbox
(Codex), or --yolo (Gemini).

Examples:
  klaudiush bypass status              # Show the effective setting
  klaudiush bypass skip                # Stop validating in bypass modes
  klaudiush bypass skip --global       # Same, for every project
  klaudiush bypass enforce             # Restore the default
  klaudiush bypass notify off          # Keep validating, hide the reminder`,
	RunE: runBypassStatus,
}

var bypassStatusCmd = &cobra.Command{
	Use:   cmdUseStatus,
	Short: "Show whether validation runs in bypass permission modes",
	RunE:  runBypassStatus,
}

var bypassSkipCmd = &cobra.Command{
	Use:   "skip",
	Short: "Skip validation while approval prompts are off",
	Long: `Skip validation while the session runs without approval prompts.

Examples:
  klaudiush bypass skip --reason "throwaway spike"
  klaudiush bypass skip --duration 4h --reason "migration sprint"
  klaudiush bypass skip --global`,
	Args: cobra.NoArgs,
	RunE: runBypassSkip,
}

var bypassEnforceCmd = &cobra.Command{
	Use:   "enforce",
	Short: "Validate even while approval prompts are off (default)",
	Args:  cobra.NoArgs,
	RunE:  runBypassEnforce,
}

var bypassNotifyCmd = &cobra.Command{
	Use:       "notify on|off",
	Short:     "Toggle the reminder shown in bypass permission modes",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{valueOn, valueOff},
	RunE:      runBypassNotify,
}

func init() {
	rootCmd.AddCommand(bypassCmd)

	bypassCmd.AddCommand(bypassStatusCmd)
	bypassCmd.AddCommand(bypassSkipCmd)
	bypassCmd.AddCommand(bypassEnforceCmd)
	bypassCmd.AddCommand(bypassNotifyCmd)

	bypassSkipCmd.Flags().StringVarP(
		&bypassReason, "reason", "r", "", "Why validation is skipped",
	)
	bypassSkipCmd.Flags().StringVarP(
		&bypassDuration, "duration", "d", "", "Duration before the skip expires (e.g., 4h, 7d)",
	)
	bypassSkipCmd.Flags().BoolVar(
		&bypassGlobal, "global", false, "Write to global config instead of project",
	)

	bypassEnforceCmd.Flags().BoolVar(
		&bypassGlobal, "global", false, "Write to global config instead of project",
	)
	bypassNotifyCmd.Flags().BoolVar(
		&bypassGlobal, "global", false, "Write to global config instead of project",
	)

	bypassStatusCmd.Flags().BoolVar(
		&bypassGlobal, "global", false, "Show only the global setting",
	)
	bypassStatusCmd.Flags().BoolVar(
		&bypassAll, "all", false, "Show both global and project settings",
	)
}

func runBypassSkip(_ *cobra.Command, _ []string) error {
	var expiresAt string

	if bypassDuration != "" {
		dur, err := parseDuration(bypassDuration)
		if err != nil {
			return errors.Wrapf(err, "invalid duration %q", bypassDuration)
		}

		expiresAt = time.Now().UTC().Add(dur).Format(time.RFC3339)
	}

	cfg, err := loadScopedConfig(bypassGlobal)
	if err != nil {
		return err
	}

	skip := true
	bypassCfg := cfg.GetBypassPermissions()
	bypassCfg.SkipValidation = &skip
	bypassCfg.Reason = bypassReason
	bypassCfg.SkippedAt = time.Now().UTC().Format(time.RFC3339)
	bypassCfg.ExpiresAt = expiresAt
	bypassCfg.SkippedBy = "cli"

	if err := writeScopedConfig(cfg, bypassGlobal); err != nil {
		return err
	}

	fmt.Println("Validation SKIPPED while approval prompts are off.")

	if bypassReason != "" {
		fmt.Printf("  Reason: %s\n", bypassReason)
	}

	if expiresAt != "" {
		fmt.Printf("  Expires: %s\n", expiresAt)
	}

	fmt.Printf("\nWritten to %s config. Undo with: klaudiush bypass enforce%s\n",
		scopeName(bypassGlobal), globalFlagSuffix(bypassGlobal))

	return reportEffectiveValidation()
}

func runBypassEnforce(_ *cobra.Command, _ []string) error {
	cfg, err := loadScopedConfig(bypassGlobal)
	if err != nil {
		return err
	}

	// Store the explicit false rather than dropping the section: an absent
	// setting inherits, so it cannot turn off a skip set in the other scope.
	bypassCfg := cfg.GetBypassPermissions()
	bypassCfg.SkipValidation = new(false)
	bypassCfg.Reason = ""
	bypassCfg.SkippedAt = ""
	bypassCfg.ExpiresAt = ""
	bypassCfg.SkippedBy = ""

	if err := writeScopedConfig(cfg, bypassGlobal); err != nil {
		return err
	}

	fmt.Println("Validation ENFORCED, including while approval prompts are off.")
	fmt.Printf("\nWritten to %s config.\n", scopeName(bypassGlobal))

	return reportEffectiveValidation()
}

func runBypassNotify(_ *cobra.Command, args []string) error {
	notify, err := parseOnOff(args[0])
	if err != nil {
		return err
	}

	cfg, err := loadScopedConfig(bypassGlobal)
	if err != nil {
		return err
	}

	// Stored either way, for the same reason as enforce.
	cfg.GetBypassPermissions().Notify = &notify

	if writeErr := writeScopedConfig(cfg, bypassGlobal); writeErr != nil {
		return writeErr
	}

	fmt.Printf("Bypass reminder %s.\n", onOffLabel(notify))
	fmt.Printf("\nWritten to %s config.\n", scopeName(bypassGlobal))

	merged, err := mergedBypassConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Effective now: reminder %s\n", onOffLabel(merged.IsNotifyEnabled()))

	return nil
}

// reportEffectiveValidation prints the merged setting the hook will apply,
// so a scope that loses to another one is visible rather than assumed.
func reportEffectiveValidation() error {
	merged, err := mergedBypassConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Effective now: %s\n", validationLabel(merged))

	return nil
}

// mergedBypassConfig loads the bypass settings with full precedence applied.
func mergedBypassConfig() (*config.BypassPermissionsConfig, error) {
	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return nil, errors.Wrap(err, "creating config loader")
	}

	cfg, err := loader.Load(nil)
	if err != nil {
		return nil, errors.Wrap(err, "loading merged config")
	}

	return cfg.BypassPermissions, nil
}

func runBypassStatus(_ *cobra.Command, _ []string) error {
	showProject := !bypassGlobal || bypassAll
	showGlobal := bypassGlobal || bypassAll

	if showProject {
		cfg, err := loadScopedConfig(false)
		if err != nil {
			return err
		}

		displayBypassScope(scopeProject, cfg.BypassPermissions)
	}

	if showGlobal {
		if showProject {
			fmt.Println("")
		}

		cfg, err := loadScopedConfig(true)
		if err != nil {
			return err
		}

		displayBypassScope(scopeGlobal, cfg.BypassPermissions)
	}

	return displayBypassEffective()
}

// displayBypassScope renders the bypass settings stored in one config file.
func displayBypassScope(scope string, cfg *config.BypassPermissionsConfig) {
	fmt.Printf("Bypass permissions (%s)\n", scope)

	const headerPadding = 23
	fmt.Println(strings.Repeat("=", len(scope)+headerPadding))
	fmt.Println("")

	if cfg == nil {
		fmt.Println("  (not configured)")

		return
	}

	fmt.Printf("  Validation: %s\n", validationLabel(cfg))
	fmt.Printf("  Reminder: %s\n", onOffLabel(cfg.IsNotifyEnabled()))

	if cfg.Reason != "" {
		fmt.Printf("  Reason: %s\n", cfg.Reason)
	}

	if cfg.SkippedAt != "" {
		fmt.Printf("  Since: %s\n", cfg.SkippedAt)
	}

	if cfg.ExpiresAt != "" {
		fmt.Printf("  Expires: %s\n", cfg.ExpiresAt)
	}

	if len(cfg.Modes) > 0 {
		fmt.Printf("  Modes: %s\n", strings.Join(cfg.Modes, ", "))
	}
}

// displayBypassEffective renders the merged setting the hook actually applies.
func displayBypassEffective() error {
	bypassCfg, err := mergedBypassConfig()
	if err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Effective")
	fmt.Println("=========")
	fmt.Println("")
	fmt.Printf("  Validation: %s\n", validationLabel(bypassCfg))
	fmt.Printf("  Reminder: %s\n", onOffLabel(bypassCfg.IsNotifyEnabled()))
	fmt.Printf("  Bypass modes: %s\n", strings.Join(effectiveModes(bypassCfg), ", "))

	return nil
}

// validationLabel describes what happens in the configured permission modes.
func validationLabel(cfg *config.BypassPermissionsConfig) string {
	if cfg.IsSkipValidation() {
		return "SKIPPED in " + modesPhrase(cfg)
	}

	if !skipRequested(cfg) {
		return "ENFORCED in every permission mode"
	}

	if cfg.HasInvalidExpiry() {
		return "ENFORCED (expires_at is not RFC3339, skip ignored)"
	}

	return "ENFORCED (skip expired)"
}

// modesPhrase names the modes a skip applies to, spelling out custom lists
// so a setting that reaches beyond prompt-free sessions is not hidden.
func modesPhrase(cfg *config.BypassPermissionsConfig) string {
	if len(cfg.GetModes()) == 0 {
		return "bypass permission modes"
	}

	return "permission modes: " + strings.Join(cfg.GetModes(), ", ")
}

// effectiveModes returns the modes in force, falling back to the defaults.
func effectiveModes(cfg *config.BypassPermissionsConfig) []string {
	if modes := cfg.GetModes(); len(modes) > 0 {
		return modes
	}

	return hook.DefaultBypassModes()
}

// skipRequested reports whether the config asks to skip, expiry aside.
func skipRequested(cfg *config.BypassPermissionsConfig) bool {
	return cfg != nil && cfg.SkipValidation != nil && *cfg.SkipValidation
}

// parseOnOff converts an on/off argument to a boolean.
func parseOnOff(value string) (bool, error) {
	switch strings.ToLower(value) {
	case valueOn, valueTrue, "enable", "enabled":
		return true, nil
	case valueOff, valueFalse, "disable", "disabled":
		return false, nil
	default:
		return false, errors.Errorf("expected \"on\" or \"off\", got %q", value)
	}
}

// onOffLabel renders a boolean as on/off.
func onOffLabel(value bool) string {
	if value {
		return valueOn
	}

	return valueOff
}

// scopeName renders the config scope being written.
func scopeName(global bool) string {
	if global {
		return scopeGlobal
	}

	return scopeProject
}

// globalFlagSuffix appends --global to a suggested command when relevant.
func globalFlagSuffix(global bool) string {
	if global {
		return " --global"
	}

	return ""
}
