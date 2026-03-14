package factory

import (
	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	elicitationvalidators "github.com/smykla-skalski/klaudiush/internal/validators/elicitation"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// ElicitationValidatorFactory creates elicitation validators from configuration.
type ElicitationValidatorFactory struct {
	log        logger.Logger
	ruleEngine *rules.RuleEngine
}

// NewElicitationValidatorFactory creates a new ElicitationValidatorFactory.
func NewElicitationValidatorFactory(log logger.Logger) *ElicitationValidatorFactory {
	return &ElicitationValidatorFactory{log: log}
}

// SetRuleEngine sets the rule engine for the factory.
func (f *ElicitationValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.ruleEngine = engine
}

// CreateValidators creates all elicitation validators based on configuration.
func (f *ElicitationValidatorFactory) CreateValidators(
	cfg *config.Config,
) []ValidatorWithPredicate {
	var validators []ValidatorWithPredicate

	elicitationCfg := cfg.GetValidators().GetElicitation()

	if elicitationCfg.Server != nil && elicitationCfg.Server.IsEnabled() &&
		!isValidatorOverridden(cfg.Overrides, "mcp.server") {
		validators = append(validators, f.createServerValidator(elicitationCfg.Server))
	}

	return validators
}

func (f *ElicitationValidatorFactory) createServerValidator(
	cfg *config.ElicitationServerConfig,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorMCPServer,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			elicitationvalidators.NewServerValidator(f.log, cfg, rc),
			cfg,
		),
		Predicate: elicitationEventPredicate(),
	}
}
