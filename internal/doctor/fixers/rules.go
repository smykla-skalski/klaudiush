// Package fixers provides auto-fix functionality for doctor checks.
package fixers

import (
	"context"
	"fmt"
	"io"
	"os"
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

// RulesFixer fixes invalid rules by disabling them.
type RulesFixer struct {
	prompter prompt.Prompter
	loader   *internalconfig.KoanfLoader
	warnOut  io.Writer
}

// NewRulesFixer creates a new RulesFixer.
func NewRulesFixer(prompter prompt.Prompter) *RulesFixer {
	loader, _ := internalconfig.NewKoanfLoader()

	return &RulesFixer{
		prompter: prompter,
		loader:   loader,
		warnOut:  os.Stderr,
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
// rules that come from the global config are never modified; Fix prints a
// warning naming them instead, and the doctor re-run keeps reporting them.
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

	globalRules, globalPath, globalErr := f.loadGlobalRules()

	defer f.warnGlobalRules(projectRules, globalRules, globalPath, globalErr)

	rulesToDisable := collectFixableRules(projectRules)
	if len(rulesToDisable) == 0 {
		return nil
	}

	// Confirm with user if interactive
	if interactive && !f.confirmFix(len(rulesToDisable)) {
		return nil
	}

	// Disable the invalid rules
	disableRules(cfg, rulesToDisable)

	// Write back to the same project config file that was loaded
	writer := internalconfig.NewWriter()
	if err := writer.WriteFile(configPath, cfg); err != nil {
		return errors.Wrapf(err, "failed to write config to %s", configPath)
	}

	f.warnShadowedGlobalRules(projectRules, rulesToDisable, globalRules, globalPath)

	return nil
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

func (f *RulesFixer) loadGlobalRules() ([]config.RuleConfig, string, error) {
	globalCfg, globalPath, err := f.loader.LoadGlobalConfigOnly()
	if err != nil {
		return nil, globalPath, err
	}

	if globalCfg == nil || globalCfg.Rules == nil {
		return nil, globalPath, nil
	}

	return globalCfg.Rules.Rules, globalPath, nil
}

func (f *RulesFixer) warnf(format string, args ...any) {
	if f.warnOut == nil {
		return
	}

	_, _ = fmt.Fprintf(f.warnOut, "warning: "+format+"\n", args...)
}

// warnGlobalRules warns about invalid enabled global rules that no project
// rule overrides by name. The fixer only edits the project config.
func (f *RulesFixer) warnGlobalRules(
	projectRules, globalRules []config.RuleConfig,
	globalPath string,
	loadErr error,
) {
	if loadErr != nil {
		f.warnf("global config %s not checked for invalid rules: %v", globalPath, loadErr)

		return
	}

	overridden := namedRules(projectRules)

	var invalid []string

	for idx := range collectFixableRules(globalRules) {
		rule := &globalRules[idx]
		if rule.Name != "" && overridden[rule.Name] {
			continue
		}

		invalid = append(invalid, ruleLabel(idx, rule))
	}

	if len(invalid) == 0 {
		return
	}

	slices.Sort(invalid)

	f.warnf(
		"invalid rule(s) in global config %s were not modified: %s; fix or disable them manually",
		globalPath,
		strings.Join(invalid, ", "),
	)
}

// warnShadowedGlobalRules warns when a disabled project rule overrides a
// global rule of the same name: the global rule stays replaced by it, so it
// does not apply until the project rule is fixed or removed.
func (f *RulesFixer) warnShadowedGlobalRules(
	projectRules []config.RuleConfig,
	rulesToDisable map[int]bool,
	globalRules []config.RuleConfig,
	globalPath string,
) {
	globalNames := namedRules(globalRules)

	var shadowed []string

	for idx := range rulesToDisable {
		if name := projectRules[idx].Name; name != "" && globalNames[name] {
			shadowed = append(shadowed, fmt.Sprintf("%q", name))
		}
	}

	if len(shadowed) == 0 {
		return
	}

	slices.Sort(shadowed)

	f.warnf(
		"disabled project rule(s) %s still override the same-named rule(s) in global config %s; "+
			"fix or remove them to apply the global rule(s)",
		strings.Join(shadowed, ", "),
		globalPath,
	)
}

func namedRules(rules []config.RuleConfig) map[string]bool {
	names := make(map[string]bool, len(rules))

	for i := range rules {
		if rules[i].Name != "" {
			names[rules[i].Name] = true
		}
	}

	return names
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
