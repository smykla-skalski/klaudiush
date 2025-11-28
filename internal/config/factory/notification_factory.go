package factory

import (
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	notificationvalidators "github.com/smykla-labs/klaudiush/internal/validators/notification"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// NotificationValidatorFactory creates notification validators from configuration.
type NotificationValidatorFactory struct {
	log        logger.Logger
	ruleEngine *rules.RuleEngine
}

// NewNotificationValidatorFactory creates a new NotificationValidatorFactory.
func NewNotificationValidatorFactory(log logger.Logger) *NotificationValidatorFactory {
	return &NotificationValidatorFactory{log: log}
}

// SetRuleEngine sets the rule engine for the factory.
func (f *NotificationValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.ruleEngine = engine
}

// CreateValidators creates all notification validators based on configuration.
func (f *NotificationValidatorFactory) CreateValidators(
	cfg *config.Config,
) []ValidatorWithPredicate {
	var validators []ValidatorWithPredicate

	if cfg.Validators.Notification.Bell != nil && cfg.Validators.Notification.Bell.IsEnabled() {
		validators = append(validators, f.createBellValidator(cfg.Validators.Notification.Bell))
	}

	return validators
}

func (f *NotificationValidatorFactory) createBellValidator(
	cfg *config.BellValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorNotification,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: notificationvalidators.NewBellValidator(f.log, cfg, ruleAdapter),
		Predicate: validator.EventTypeIs(hook.EventTypeNotification),
	}
}
