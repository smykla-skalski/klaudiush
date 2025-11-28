// Package factory provides factories for creating validators from configuration.
package factory

import (
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// RulesFactory creates a RuleEngine from configuration.
type RulesFactory struct {
	log logger.Logger
}

// NewRulesFactory creates a new RulesFactory.
func NewRulesFactory(log logger.Logger) *RulesFactory {
	return &RulesFactory{
		log: log,
	}
}

// CreateRuleEngine creates a RuleEngine from the provided configuration.
// Returns nil if rules are disabled or no rules are defined.
//
//nolint:nilnil // Returning nil engine without error is intentional when no rules are defined
func (f *RulesFactory) CreateRuleEngine(cfg *config.Config) (*rules.RuleEngine, error) {
	if cfg == nil || cfg.Rules == nil {
		return nil, nil
	}

	rulesConfig := cfg.Rules
	if !rulesConfig.IsEnabled() {
		f.log.Debug("rules engine disabled")

		return nil, nil
	}

	if len(rulesConfig.Rules) == 0 {
		f.log.Debug("no rules defined")

		return nil, nil
	}

	// Convert config rules to internal rules
	internalRules := make([]*rules.Rule, 0, len(rulesConfig.Rules))

	for _, ruleConfig := range rulesConfig.Rules {
		if !ruleConfig.IsRuleEnabled() {
			continue
		}

		internalRule := convertRuleConfig(ruleConfig)
		internalRules = append(internalRules, internalRule)
	}

	if len(internalRules) == 0 {
		f.log.Debug("no enabled rules")

		return nil, nil
	}

	// Create engine with options
	opts := []rules.EngineOption{
		rules.WithLogger(f.log),
		rules.WithEngineStopOnFirstMatch(rulesConfig.ShouldStopOnFirstMatch()),
	}

	engine, err := rules.NewRuleEngine(internalRules, opts...)
	if err != nil {
		return nil, err
	}

	f.log.Debug("rule engine created",
		"rule_count", engine.Size(),
	)

	return engine, nil
}

// convertRuleConfig converts a config.RuleConfig to a rules.Rule.
func convertRuleConfig(cfg config.RuleConfig) *rules.Rule {
	rule := &rules.Rule{
		Name:        cfg.Name,
		Description: cfg.Description,
		Enabled:     cfg.IsRuleEnabled(),
		Priority:    cfg.Priority,
	}

	// Convert match conditions
	if cfg.Match != nil {
		rule.Match = &rules.RuleMatch{
			ValidatorType:   rules.ValidatorType(cfg.Match.ValidatorType),
			RepoPattern:     cfg.Match.RepoPattern,
			RepoPatterns:    cfg.Match.RepoPatterns,
			Remote:          cfg.Match.Remote,
			BranchPattern:   cfg.Match.BranchPattern,
			BranchPatterns:  cfg.Match.BranchPatterns,
			FilePattern:     cfg.Match.FilePattern,
			FilePatterns:    cfg.Match.FilePatterns,
			ContentPattern:  cfg.Match.ContentPattern,
			ContentPatterns: cfg.Match.ContentPatterns,
			CommandPattern:  cfg.Match.CommandPattern,
			CommandPatterns: cfg.Match.CommandPatterns,
			ToolType:        cfg.Match.ToolType,
			EventType:       cfg.Match.EventType,
			CaseInsensitive: cfg.Match.IsCaseInsensitive(),
			PatternMode:     cfg.Match.GetPatternMode(),
		}
	}

	// Convert action
	if cfg.Action != nil {
		rule.Action = &rules.RuleAction{
			Type:      convertActionType(cfg.Action.GetActionType()),
			Message:   cfg.Action.Message,
			Reference: cfg.Action.Reference,
		}
	}

	return rule
}

// convertActionType converts a string action type to rules.ActionType.
func convertActionType(actionType string) rules.ActionType {
	switch actionType {
	case "block":
		return rules.ActionBlock
	case "warn":
		return rules.ActionWarn
	case "allow":
		return rules.ActionAllow
	default:
		return rules.ActionBlock
	}
}
