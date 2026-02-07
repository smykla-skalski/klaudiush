package factory

import (
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// RegistryBuilder builds a validator registry from configuration.
type RegistryBuilder struct {
	factory      ValidatorFactory
	rulesFactory *RulesFactory
	log          logger.Logger
}

// NewRegistryBuilder creates a new RegistryBuilder.
func NewRegistryBuilder(log logger.Logger) *RegistryBuilder {
	return &RegistryBuilder{
		factory:      NewValidatorFactory(log),
		rulesFactory: NewRulesFactory(log),
		log:          log,
	}
}

// Build creates a validator registry from the provided configuration.
// It creates all enabled validators and registers them with their predicates.
func (b *RegistryBuilder) Build(cfg *config.Config) *validator.Registry {
	registry := validator.NewRegistry()

	// Get all validators with predicates from factory
	validatorsWithPredicates := b.factory.CreateAll(cfg)

	// Register each validator with its predicate
	for _, vp := range validatorsWithPredicates {
		registry.Register(vp.Validator, vp.Predicate)
	}

	b.log.Debug("registry built",
		"validator_count", len(validatorsWithPredicates),
	)

	return registry
}

// BuildWithRuleEngine creates a validator registry and rule engine from configuration.
// Returns the registry and the rule engine (which may be nil if rules are disabled).
func (b *RegistryBuilder) BuildWithRuleEngine(
	cfg *config.Config,
) (*validator.Registry, *rules.RuleEngine, error) {
	// Create rule engine first
	ruleEngine, err := b.rulesFactory.CreateRuleEngine(cfg)
	if err != nil {
		return nil, nil, err
	}

	if ruleEngine != nil {
		b.log.Debug("rule engine created",
			"rule_count", ruleEngine.Size(),
		)
		// Set rule engine in factory so validators can use it
		b.factory.SetRuleEngine(ruleEngine)
	}

	// Build registry with validators that have rule support
	registry := b.Build(cfg)

	return registry, ruleEngine, nil
}

// CreateRuleEngine creates a rule engine from configuration.
// Returns nil if rules are disabled or no rules are defined.
func (b *RegistryBuilder) CreateRuleEngine(cfg *config.Config) (*rules.RuleEngine, error) {
	return b.rulesFactory.CreateRuleEngine(cfg)
}
