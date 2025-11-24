package factory

import (
	"github.com/smykla-labs/klaudiush/internal/validator"
	gitvalidators "github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// GitValidatorFactory creates git validators from configuration.
type GitValidatorFactory struct {
	log logger.Logger
}

// NewGitValidatorFactory creates a new GitValidatorFactory.
func NewGitValidatorFactory(log logger.Logger) *GitValidatorFactory {
	return &GitValidatorFactory{log: log}
}

// CreateValidators creates all git validators based on configuration.
func (f *GitValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	var validators []ValidatorWithPredicate

	if cfg.Validators.Git.Add != nil && cfg.Validators.Git.Add.IsEnabled() {
		validators = append(validators, f.createAddValidator())
	}

	if cfg.Validators.Git.NoVerify != nil && cfg.Validators.Git.NoVerify.IsEnabled() {
		validators = append(validators, f.createNoVerifyValidator())
	}

	if cfg.Validators.Git.Commit != nil && cfg.Validators.Git.Commit.IsEnabled() {
		validators = append(validators, f.createCommitValidator())
	}

	if cfg.Validators.Git.Push != nil && cfg.Validators.Git.Push.IsEnabled() {
		validators = append(validators, f.createPushValidator())
	}

	if cfg.Validators.Git.PR != nil && cfg.Validators.Git.PR.IsEnabled() {
		validators = append(validators, f.createPRValidator())
	}

	if cfg.Validators.Git.Branch != nil && cfg.Validators.Git.Branch.IsEnabled() {
		validators = append(validators, f.createBranchValidator())
	}

	return validators
}

func (f *GitValidatorFactory) createAddValidator() ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewAddValidator(f.log, nil, nil),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git add"),
		),
	}
}

func (f *GitValidatorFactory) createNoVerifyValidator() ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewNoVerifyValidator(f.log, nil),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git commit"),
		),
	}
}

func (f *GitValidatorFactory) createCommitValidator() ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewCommitValidator(f.log, nil, nil),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git commit"),
		),
	}
}

func (f *GitValidatorFactory) createPushValidator() ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewPushValidator(f.log, nil, nil),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git push"),
		),
	}
}

func (f *GitValidatorFactory) createPRValidator() ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewPRValidator(f.log),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("gh pr create"),
		),
	}
}

func (f *GitValidatorFactory) createBranchValidator() ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewBranchValidator(f.log),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.Or(
				validator.CommandContains("git checkout -b"),
				validator.And(
					validator.CommandContains("git branch"),
					validator.Not(validator.Or(
						validator.CommandContains("-d"),
						validator.CommandContains("-D"),
						validator.CommandContains("--delete"),
					)),
				),
			),
		),
	}
}
