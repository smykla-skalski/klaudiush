// Package fixers provides auto-fix functionality for doctor checks.
package fixers

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
	ruleschecker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/rules"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	// disabledNote is the full description for rules with no existing description.
	disabledNote = "DISABLED BY DOCTOR: fix the rule configuration and re-enable"

	// disabledSuffix is appended to existing descriptions.
	disabledSuffix = "[DISABLED BY DOCTOR: fix and re-enable]"
)

// ErrGlobalRulesNotFixed is returned when invalid rules come from the global
// config. The fixer only edits the project config, so they need a manual fix.
var ErrGlobalRulesNotFixed = errors.New("invalid rules in global config were not modified")

// RulesFixer fixes invalid rules by disabling them.
type RulesFixer struct {
	prompter prompt.Prompter
	loader   *internalconfig.KoanfLoader
}

// NewRulesFixer creates a new RulesFixer.
func NewRulesFixer(prompter prompt.Prompter) *RulesFixer {
	loader, _ := internalconfig.NewKoanfLoader()

	return &RulesFixer{
		prompter: prompter,
		loader:   loader,
	}
}

// ID returns the fixer identifier.
func (*RulesFixer) ID() string {
	return "fix_invalid_rules"
}

// Description returns a human-readable description.
func (*RulesFixer) Description() string {
	return "Disable invalid rules in configuration (rules can be re-enabled after manual fix)"
}

// CanFix checks if this fixer can fix the given result.
func (*RulesFixer) CanFix(result doctor.CheckResult) bool {
	return result.FixID == "fix_invalid_rules" && result.Status == doctor.StatusFail
}

// Fix disables invalid rules in the project configuration.
//
// Rules are validated per source file instead of through the merged config,
// so each disabled rule is the one the project file actually defines. Invalid
// rules that come from the global config are left untouched and reported via
// ErrGlobalRulesNotFixed.
func (f *RulesFixer) Fix(_ context.Context, interactive bool) error {
	if err := f.ensureLoader(); err != nil {
		return err
	}

	// Load only the project config (not merged with defaults/global/env)
	// This ensures we only write back what was in the project config
	cfg, configPath, err := f.loader.LoadProjectConfigOnly()
	if err != nil {
		return errors.Wrap(err, "failed to load project config")
	}

	var projectRules []config.RuleConfig
	if cfg != nil && cfg.Rules != nil {
		projectRules = cfg.Rules.Rules
	}

	globalErr := f.checkGlobalRules(projectRules)

	rulesToDisable := collectFixableRules(projectRules)
	if len(rulesToDisable) == 0 {
		return globalErr
	}

	// Confirm with user if interactive
	if interactive && !f.confirmFix(len(rulesToDisable)) {
		return globalErr
	}

	// Disable the invalid rules
	disableRules(cfg, rulesToDisable)

	// Write back to the same project config file that was loaded
	writer := internalconfig.NewWriter()
	if err := writer.WriteFile(configPath, cfg); err != nil {
		return errors.Wrapf(err, "failed to write config to %s", configPath)
	}

	return globalErr
}

func (f *RulesFixer) ensureLoader() error {
	if f.loader != nil {
		return nil
	}

	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return errors.Wrap(err, "failed to create config loader")
	}

	f.loader = loader

	return nil
}

// checkGlobalRules returns ErrGlobalRulesNotFixed when the global config has
// invalid enabled rules that no project rule overrides by name.
func (f *RulesFixer) checkGlobalRules(projectRules []config.RuleConfig) error {
	globalCfg, globalPath, err := f.loader.LoadGlobalConfigOnly()
	if err != nil {
		return errors.Wrap(err, "failed to load global config")
	}

	if globalCfg == nil || globalCfg.Rules == nil {
		return nil
	}

	overridden := make(map[string]bool, len(projectRules))

	for i := range projectRules {
		if projectRules[i].Name != "" {
			overridden[projectRules[i].Name] = true
		}
	}

	var invalid []string

	for idx := range collectFixableRules(globalCfg.Rules.Rules) {
		rule := &globalCfg.Rules.Rules[idx]
		if rule.Name != "" && overridden[rule.Name] {
			continue
		}

		invalid = append(invalid, ruleLabel(idx, rule))
	}

	if len(invalid) == 0 {
		return nil
	}

	slices.Sort(invalid)

	return errors.Wrapf(ErrGlobalRulesNotFixed,
		"%s: %s; fix or disable them manually",
		globalPath, strings.Join(invalid, ", "))
}

// collectFixableRules returns the indices of enabled rules with fixable issues.
func collectFixableRules(rules []config.RuleConfig) map[int]bool {
	rulesToDisable := make(map[int]bool)

	for i := range rules {
		if !rules[i].IsRuleEnabled() {
			continue
		}

		for _, issue := range ruleschecker.ValidateRule(i, &rules[i]) {
			if issue.Fixable {
				rulesToDisable[i] = true

				break
			}
		}
	}

	return rulesToDisable
}

func ruleLabel(index int, rule *config.RuleConfig) string {
	if rule.Name != "" {
		return fmt.Sprintf("%q", rule.Name)
	}

	return fmt.Sprintf("rule #%d", index+1)
}

// confirmFix prompts the user for confirmation.
func (f *RulesFixer) confirmFix(count int) bool {
	msg := fmt.Sprintf(
		"Disable %d invalid rule(s)? (They can be re-enabled after manual fix)",
		count,
	)

	confirmed, err := f.prompter.Confirm(msg, true)
	if err != nil {
		return false
	}

	return confirmed
}

// disableRules marks the specified rules as disabled.
func disableRules(cfg *config.Config, rulesToDisable map[int]bool) {
	for idx := range rulesToDisable {
		if idx < len(cfg.Rules.Rules) {
			disabled := false
			cfg.Rules.Rules[idx].Enabled = &disabled

			// Add a description note if not present
			desc := cfg.Rules.Rules[idx].Description
			if desc == "" {
				cfg.Rules.Rules[idx].Description = disabledNote
			} else if !containsDisabledNote(desc) {
				cfg.Rules.Rules[idx].Description = desc + " " + disabledSuffix
			}
		}
	}
}

// containsDisabledNote checks if description already has the disabled note.
func containsDisabledNote(desc string) bool {
	if desc == "" {
		return false
	}

	return desc == disabledNote || strings.HasSuffix(desc, disabledSuffix)
}
