package factory

import (
	"context"

	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// LifecycleValidatorFactory creates lifecycle-only rule validators.
type LifecycleValidatorFactory struct {
	log        logger.Logger
	ruleEngine *rules.RuleEngine
}

// NewLifecycleValidatorFactory creates a new LifecycleValidatorFactory.
func NewLifecycleValidatorFactory(log logger.Logger) *LifecycleValidatorFactory {
	return &LifecycleValidatorFactory{log: log}
}

// SetRuleEngine sets the rule engine for the factory.
func (f *LifecycleValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.ruleEngine = engine
}

// CreateValidators creates lifecycle-only rule validators.
func (f *LifecycleValidatorFactory) CreateValidators(*config.Config) []ValidatorWithPredicate {
	if f.ruleEngine == nil {
		return nil
	}

	adapter := rules.NewRuleValidatorAdapter(
		f.ruleEngine,
		rules.ValidatorAll,
		rules.WithAdapterLogger(f.log),
	)

	return []ValidatorWithPredicate{
		{
			Validator: &LifecycleRuleValidator{
				BaseValidator: validator.NewBaseValidator("lifecycle.rules", f.log),
				adapter:       adapter,
			},
			Predicate: lifecycleEventPredicate(),
		},
	}
}

// LifecycleRuleValidator evaluates lifecycle rules without a built-in validator match.
type LifecycleRuleValidator struct {
	*validator.BaseValidator
	adapter *rules.RuleValidatorAdapter
}

// Validate evaluates lifecycle rules and otherwise passes through.
func (v *LifecycleRuleValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	if v.adapter == nil {
		return validator.Pass()
	}

	if result := v.adapter.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	return validator.Pass()
}

// Category returns the validator category.
func (*LifecycleRuleValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}
