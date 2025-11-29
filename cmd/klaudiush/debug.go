// Package main provides the CLI entry point for klaudiush.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/exceptions"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// Status display constants.
const (
	statusEnabled      = "ENABLED"
	statusDisabled     = "DISABLED"
	statusEnabledLower = "enabled"
)

var validatorFilter string

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug klaudiush configuration",
	Long: `Debug klaudiush configuration and internal state.

Subcommands:
  rules       Show loaded validation rules
  exceptions  Show exception workflow configuration`,
}

var debugRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Show loaded validation rules",
	Long: `Show loaded validation rules from configuration.

Displays all rules with their match conditions, actions, and priorities.
Rules are shown in evaluation order (highest priority first).

Examples:
  klaudiush debug rules                    # Show all rules
  klaudiush debug rules --validator git.push  # Show rules for git.push validator`,
	RunE: runDebugRules,
}

var debugExceptionsCmd = &cobra.Command{
	Use:   "exceptions",
	Short: "Show exception workflow configuration",
	Long: `Show exception workflow configuration and current state.

Displays exception policies, rate limit settings, audit configuration,
and current rate limit state if available.

Examples:
  klaudiush debug exceptions           # Show all exception config
  klaudiush debug exceptions --state   # Include rate limit state`,
	RunE: runDebugExceptions,
}

var showState bool

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.AddCommand(debugRulesCmd)
	debugCmd.AddCommand(debugExceptionsCmd)

	debugRulesCmd.Flags().StringVar(
		&validatorFilter,
		"validator",
		"",
		"Filter rules by validator type (e.g., git.push, file.*, secrets.secrets)",
	)

	debugExceptionsCmd.Flags().BoolVar(
		&showState,
		"state",
		false,
		"Include current rate limit state from state file",
	)
}

func runDebugRules(_ *cobra.Command, _ []string) error {
	cfg, err := setupDebugContext("debug rules", "validatorFilter", validatorFilter)
	if err != nil {
		return err
	}

	displayRulesConfig(cfg, validatorFilter)

	return nil
}

// setupDebugContext initializes logging and loads configuration for debug commands.
func setupDebugContext(cmdName, extraKey, extraVal string) (*config.Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get home directory")
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, false, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create logger")
	}

	log.Info(cmdName+" command invoked", extraKey, extraVal)

	cfg, err := loadConfigForDebug(log)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load configuration")
	}

	return cfg, nil
}

func loadConfigForDebug(log logger.Logger) (*config.Config, error) {
	flags := buildFlagsMap()

	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config loader")
	}

	cfg, err := loader.Load(flags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}

	log.Debug("configuration loaded for debug")

	return cfg, nil
}

func displayRulesConfig(cfg *config.Config, filter string) {
	rules := cfg.GetRules()
	if rules == nil || len(rules.Rules) == 0 {
		fmt.Println("No rules configured.")
		fmt.Println("")
		fmt.Println("To configure rules, add them to:")
		fmt.Println("  - Global: ~/.klaudiush/config.toml")
		fmt.Println("  - Project: .klaudiush/config.toml")
		fmt.Println("")
		fmt.Println("See docs/RULES_GUIDE.md for configuration examples.")

		return
	}

	// Header
	fmt.Println("Validation Rules")
	fmt.Println("================")
	fmt.Println("")

	// Rules engine status
	fmt.Printf("Engine Enabled: %v\n", rules.IsEnabled())
	fmt.Printf("Stop on First Match: %v\n", rules.ShouldStopOnFirstMatch())
	fmt.Printf("Total Rules: %d\n", len(rules.Rules))
	fmt.Println("")

	// Filter rules if needed
	filteredRules := filterRules(rules.Rules, filter)

	if len(filteredRules) == 0 {
		fmt.Printf("No rules match filter: %s\n", filter)

		return
	}

	// Display rules
	for i, rule := range filteredRules {
		displayRule(i+1, &rule)
	}
}

func filterRules(rules []config.RuleConfig, filter string) []config.RuleConfig {
	if filter == "" {
		return rules
	}

	var filtered []config.RuleConfig

	for _, rule := range rules {
		if rule.Match == nil {
			continue
		}

		validatorType := rule.Match.ValidatorType

		// Check exact match
		if validatorType == filter {
			filtered = append(filtered, rule)

			continue
		}

		// Check wildcard match (e.g., "git.*" matches "git.push")
		if before, ok := strings.CutSuffix(filter, ".*"); ok {
			prefix := before
			if strings.HasPrefix(validatorType, prefix+".") {
				filtered = append(filtered, rule)

				continue
			}
		}

		// Check if filter is a wildcard that matches rule's validator type
		if before, ok := strings.CutSuffix(validatorType, ".*"); ok {
			prefix := before
			if strings.HasPrefix(filter, prefix+".") {
				filtered = append(filtered, rule)

				continue
			}
		}

		// Check "*" matches everything
		if validatorType == "*" {
			filtered = append(filtered, rule)
		}
	}

	return filtered
}

func displayRule(index int, rule *config.RuleConfig) {
	enabledStr := statusEnabledLower
	if !rule.IsRuleEnabled() {
		enabledStr = statusDisabled
	}

	fmt.Printf("Rule #%d: %s [%s]\n", index, rule.Name, enabledStr)
	fmt.Printf("  Priority: %d\n", rule.Priority)

	if rule.Description != "" {
		fmt.Printf("  Description: %s\n", rule.Description)
	}

	// Match conditions
	if rule.Match != nil {
		fmt.Println("  Match:")
		displayMatchCondition("    ", rule.Match)
	}

	// Action
	if rule.Action != nil {
		fmt.Println("  Action:")
		fmt.Printf("    Type: %s\n", rule.Action.GetActionType())

		if rule.Action.Message != "" {
			fmt.Printf("    Message: %s\n", rule.Action.Message)
		}

		if rule.Action.Reference != "" {
			fmt.Printf("    Reference: %s\n", rule.Action.Reference)
		}
	}

	fmt.Println("")
}

func displayMatchCondition(indent string, match *config.RuleMatchConfig) {
	if match.ValidatorType != "" {
		fmt.Printf("%sValidator Type: %s\n", indent, match.ValidatorType)
	}

	if match.RepoPattern != "" {
		fmt.Printf("%sRepo Pattern: %s\n", indent, match.RepoPattern)
	}

	if match.Remote != "" {
		fmt.Printf("%sRemote: %s\n", indent, match.Remote)
	}

	if match.BranchPattern != "" {
		fmt.Printf("%sBranch Pattern: %s\n", indent, match.BranchPattern)
	}

	if match.FilePattern != "" {
		fmt.Printf("%sFile Pattern: %s\n", indent, match.FilePattern)
	}

	if match.ContentPattern != "" {
		fmt.Printf("%sContent Pattern: %s\n", indent, match.ContentPattern)
	}

	if match.CommandPattern != "" {
		fmt.Printf("%sCommand Pattern: %s\n", indent, match.CommandPattern)
	}

	if match.ToolType != "" {
		fmt.Printf("%sTool Type: %s\n", indent, match.ToolType)
	}

	if match.EventType != "" {
		fmt.Printf("%sEvent Type: %s\n", indent, match.EventType)
	}
}

func runDebugExceptions(_ *cobra.Command, _ []string) error {
	showStateStr := strconv.FormatBool(showState)

	cfg, err := setupDebugContext("debug exceptions", "showState", showStateStr)
	if err != nil {
		return err
	}

	displayExceptionsConfig(cfg, showState)

	return nil
}

func displayExceptionsConfig(cfg *config.Config, includeState bool) {
	exc := cfg.GetExceptions()

	// Header
	fmt.Println("Exception Workflow Configuration")
	fmt.Println("================================")
	fmt.Println("")

	// System status
	displaySystemStatus(exc)

	// Policies
	displayPolicies(exc)

	// Rate limit config
	displayRateLimitConfig(exc)

	// Audit config
	displayAuditConfig(exc)

	// Rate limit state
	if includeState {
		displayRateLimitState(exc)
	}

	// Help text
	displayExceptionsHelp()
}

func displaySystemStatus(exc *config.ExceptionsConfig) {
	enabledStr := statusEnabled
	if exc != nil && !exc.IsEnabled() {
		enabledStr = statusDisabled
	}

	fmt.Printf("System Status: %s\n", enabledStr)

	if exc != nil && exc.TokenPrefix != "" {
		fmt.Printf("Token Prefix: %s\n", exc.TokenPrefix)
	} else {
		fmt.Printf("Token Prefix: EXC (default)\n")
	}

	fmt.Println("")
}

func displayPolicies(exc *config.ExceptionsConfig) {
	fmt.Println("Policies")
	fmt.Println("--------")

	if exc == nil || len(exc.Policies) == 0 {
		fmt.Println("  No policies configured (all error codes use defaults)")
		fmt.Println("")

		return
	}

	// Sort error codes for consistent output
	codes := make([]string, 0, len(exc.Policies))
	for code := range exc.Policies {
		codes = append(codes, code)
	}

	slices.Sort(codes)

	for _, code := range codes {
		policy := exc.Policies[code]
		displayPolicy(code, policy)
	}

	fmt.Println("")
}

func displayPolicy(code string, policy *config.ExceptionPolicyConfig) {
	enabledStr := statusEnabledLower
	if !policy.IsPolicyEnabled() {
		enabledStr = statusDisabled
	}

	fmt.Printf("  %s [%s]\n", code, enabledStr)

	if policy.Description != "" {
		fmt.Printf("    Description: %s\n", policy.Description)
	}

	fmt.Printf("    Allow Exception: %v\n", policy.IsExceptionAllowed())
	fmt.Printf("    Require Reason: %v\n", policy.IsReasonRequired())

	if policy.IsReasonRequired() {
		fmt.Printf("    Min Reason Length: %d\n", policy.GetMinReasonLength())
	}

	if len(policy.ValidReasons) > 0 {
		fmt.Printf("    Valid Reasons: %v\n", policy.ValidReasons)
	}

	maxHour := policy.GetMaxPerHour()
	maxDay := policy.GetMaxPerDay()

	if maxHour > 0 || maxDay > 0 {
		fmt.Printf("    Rate Limits: %s\n", formatLimits(maxHour, maxDay))
	}
}

func displayRateLimitConfig(exc *config.ExceptionsConfig) {
	fmt.Println("Rate Limiting")
	fmt.Println("-------------")

	var rateCfg *config.ExceptionRateLimitConfig
	if exc != nil {
		rateCfg = exc.RateLimit
	}

	enabledStr := statusEnabled
	if rateCfg != nil && !rateCfg.IsRateLimitEnabled() {
		enabledStr = statusDisabled
	}

	fmt.Printf("  Status: %s\n", enabledStr)

	if rateCfg == nil {
		fmt.Printf("  Global Limits: %d/hour, %d/day (defaults)\n",
			config.DefaultRateLimitPerHour, config.DefaultRateLimitPerDay)
		fmt.Printf("  State File: %s (default)\n",
			(&config.ExceptionRateLimitConfig{}).GetStateFile())
	} else {
		fmt.Printf("  Global Limits: %d/hour, %d/day\n",
			rateCfg.GetMaxPerHour(), rateCfg.GetMaxPerDay())
		fmt.Printf("  State File: %s\n", rateCfg.GetStateFile())
	}

	fmt.Println("")
}

func displayAuditConfig(exc *config.ExceptionsConfig) {
	fmt.Println("Audit Logging")
	fmt.Println("-------------")

	var auditCfg *config.ExceptionAuditConfig
	if exc != nil {
		auditCfg = exc.Audit
	}

	enabledStr := statusEnabled
	if auditCfg != nil && !auditCfg.IsAuditEnabled() {
		enabledStr = statusDisabled
	}

	fmt.Printf("  Status: %s\n", enabledStr)

	if auditCfg == nil {
		fmt.Printf("  Log File: %s (default)\n",
			(&config.ExceptionAuditConfig{}).GetLogFile())
		fmt.Printf("  Max Size: %d MB (default)\n", config.DefaultAuditMaxSizeMB)
		fmt.Printf("  Max Age: %d days (default)\n", config.DefaultAuditMaxAgeDays)
		fmt.Printf("  Max Backups: %d (default)\n", config.DefaultAuditMaxBackups)
	} else {
		fmt.Printf("  Log File: %s\n", auditCfg.GetLogFile())
		fmt.Printf("  Max Size: %d MB\n", auditCfg.GetMaxSizeMB())
		fmt.Printf("  Max Age: %d days\n", auditCfg.GetMaxAgeDays())
		fmt.Printf("  Max Backups: %d\n", auditCfg.GetMaxBackups())
	}

	fmt.Println("")
}

func displayRateLimitState(exc *config.ExceptionsConfig) {
	fmt.Println("Rate Limit State")
	fmt.Println("----------------")

	var rateCfg *config.ExceptionRateLimitConfig
	if exc != nil {
		rateCfg = exc.RateLimit
	}

	// Create rate limiter to load state
	limiter := exceptions.NewRateLimiter(rateCfg, exc)

	if loadErr := limiter.Load(); loadErr != nil {
		fmt.Printf("  ⚠️  Could not load state: %v\n", loadErr)
		fmt.Println("")

		return
	}

	state := limiter.GetState()

	fmt.Printf("  Hour Window Start: %s\n", state.HourStartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Day Window Start: %s\n", state.DayStartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Global Hourly Count: %d\n", state.GlobalHourlyCount)
	fmt.Printf("  Global Daily Count: %d\n", state.GlobalDailyCount)
	fmt.Printf("  Last Updated: %s\n", state.LastUpdated.Format("2006-01-02 15:04:05"))

	if len(state.HourlyUsage) > 0 {
		fmt.Println("  Hourly Usage by Code:")

		codes := make([]string, 0, len(state.HourlyUsage))
		for code := range state.HourlyUsage {
			codes = append(codes, code)
		}

		slices.Sort(codes)

		for _, code := range codes {
			fmt.Printf("    %s: %d\n", code, state.HourlyUsage[code])
		}
	}

	if len(state.DailyUsage) > 0 {
		fmt.Println("  Daily Usage by Code:")

		codes := make([]string, 0, len(state.DailyUsage))
		for code := range state.DailyUsage {
			codes = append(codes, code)
		}

		slices.Sort(codes)

		for _, code := range codes {
			fmt.Printf("    %s: %d\n", code, state.DailyUsage[code])
		}
	}

	fmt.Println("")
}

func displayExceptionsHelp() {
	fmt.Println("Configuration Files")
	fmt.Println("-------------------")
	fmt.Println("  Global: ~/.klaudiush/config.toml")
	fmt.Println("  Project: .klaudiush/config.toml")
	fmt.Println("")
	fmt.Println("See docs/EXCEPTIONS_GUIDE.md for configuration examples.")
}

func formatLimits(maxHour, maxDay int) string {
	hourStr := "unlimited"
	if maxHour > 0 {
		hourStr = strconv.Itoa(maxHour)
	}

	dayStr := "unlimited"
	if maxDay > 0 {
		dayStr = strconv.Itoa(maxDay)
	}

	return hourStr + "/hour, " + dayStr + "/day"
}
