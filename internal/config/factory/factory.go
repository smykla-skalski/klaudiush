// Package factory provides factories for creating validators from configuration.
package factory

import (
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// ValidatorWithPredicate pairs a validator with its registration predicate.
type ValidatorWithPredicate struct {
	Validator validator.Validator
	Predicate validator.Predicate
}

// ValidatorFactory creates validators from configuration.
type ValidatorFactory interface {
	// SetRuleEngine sets the rule engine for all factories.
	SetRuleEngine(engine *rules.RuleEngine)

	// CreateGitValidators creates all git validators from config.
	CreateGitValidators(cfg *config.Config) []ValidatorWithPredicate

	// CreateFileValidators creates all file validators from config.
	CreateFileValidators(cfg *config.Config) []ValidatorWithPredicate

	// CreateNotificationValidators creates all notification validators from config.
	CreateNotificationValidators(cfg *config.Config) []ValidatorWithPredicate

	// CreateSecretsValidators creates all secrets validators from config.
	CreateSecretsValidators(cfg *config.Config) []ValidatorWithPredicate

	// CreateShellValidators creates all shell validators from config.
	CreateShellValidators(cfg *config.Config) []ValidatorWithPredicate

	// CreatePluginValidators creates all plugin validators from config.
	CreatePluginValidators(cfg *config.Config) []ValidatorWithPredicate

	// CreateAll creates all validators from config.
	CreateAll(cfg *config.Config) []ValidatorWithPredicate
}

// DefaultValidatorFactory is the default implementation of ValidatorFactory.
type DefaultValidatorFactory struct {
	gitFactory          *GitValidatorFactory
	fileFactory         *FileValidatorFactory
	notificationFactory *NotificationValidatorFactory
	secretsFactory      *SecretsValidatorFactory
	shellFactory        *ShellValidatorFactory
	pluginFactory       *PluginValidatorFactory
}

// NewValidatorFactory creates a new DefaultValidatorFactory.
func NewValidatorFactory(log logger.Logger) *DefaultValidatorFactory {
	return &DefaultValidatorFactory{
		gitFactory:          NewGitValidatorFactory(log),
		fileFactory:         NewFileValidatorFactory(log),
		notificationFactory: NewNotificationValidatorFactory(log),
		secretsFactory:      NewSecretsValidatorFactory(log),
		shellFactory:        NewShellValidatorFactory(log),
		pluginFactory:       NewPluginValidatorFactory(log),
	}
}

// SetRuleEngine sets the rule engine for all factories.
func (f *DefaultValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.gitFactory.SetRuleEngine(engine)
	f.fileFactory.SetRuleEngine(engine)
	f.notificationFactory.SetRuleEngine(engine)
	f.secretsFactory.SetRuleEngine(engine)
	f.shellFactory.SetRuleEngine(engine)
}

// CreateGitValidators creates all git validators from config.
func (f *DefaultValidatorFactory) CreateGitValidators(cfg *config.Config) []ValidatorWithPredicate {
	return f.gitFactory.CreateValidators(cfg)
}

// CreateFileValidators creates all file validators from config.
func (f *DefaultValidatorFactory) CreateFileValidators(
	cfg *config.Config,
) []ValidatorWithPredicate {
	return f.fileFactory.CreateValidators(cfg)
}

// CreateNotificationValidators creates all notification validators from config.
func (f *DefaultValidatorFactory) CreateNotificationValidators(
	cfg *config.Config,
) []ValidatorWithPredicate {
	return f.notificationFactory.CreateValidators(cfg)
}

// CreateSecretsValidators creates all secrets validators from config.
func (f *DefaultValidatorFactory) CreateSecretsValidators(
	cfg *config.Config,
) []ValidatorWithPredicate {
	return f.secretsFactory.CreateValidators(cfg)
}

// CreateShellValidators creates all shell validators from config.
func (f *DefaultValidatorFactory) CreateShellValidators(
	cfg *config.Config,
) []ValidatorWithPredicate {
	return f.shellFactory.CreateValidators(cfg)
}

// CreatePluginValidators creates all plugin validators from config.
func (f *DefaultValidatorFactory) CreatePluginValidators(
	cfg *config.Config,
) []ValidatorWithPredicate {
	return f.pluginFactory.CreateValidators(cfg)
}

// CreateAll creates all validators from config.
func (f *DefaultValidatorFactory) CreateAll(cfg *config.Config) []ValidatorWithPredicate {
	var all []ValidatorWithPredicate

	all = append(all, f.CreateGitValidators(cfg)...)
	all = append(all, f.CreateFileValidators(cfg)...)
	all = append(all, f.CreateNotificationValidators(cfg)...)
	all = append(all, f.CreateSecretsValidators(cfg)...)
	all = append(all, f.CreateShellValidators(cfg)...)
	all = append(all, f.CreatePluginValidators(cfg)...)

	return all
}
