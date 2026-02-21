package factory

import (
	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	notificationvalidators "github.com/smykla-skalski/klaudiush/internal/validators/notification"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
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

	if cfg.Validators.Notification.Bell != nil && cfg.Validators.Notification.Bell.IsEnabled() &&
		!isValidatorOverridden(cfg.Overrides, "notification.bell") {
		validators = append(validators, f.createBellValidator(cfg.Validators.Notification.Bell))
	}

	return validators
}

func (f *NotificationValidatorFactory) createBellValidator(
	cfg *config.BellValidatorConfig,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorNotification,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: notificationvalidators.NewBellValidator(f.log, cfg, rc),
		Predicate: validator.EventTypeIs(hook.EventTypeNotification),
	}
}
