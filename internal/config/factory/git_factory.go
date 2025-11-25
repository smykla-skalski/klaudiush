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
	cfg *config.Config
	log logger.Logger
}

// NewGitValidatorFactory creates a new GitValidatorFactory.
func NewGitValidatorFactory(log logger.Logger) *GitValidatorFactory {
	return &GitValidatorFactory{log: log}
}

// CreateValidators creates all git validators based on configuration.
func (f *GitValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	f.cfg = cfg // Store config for use in create methods

	var validators []ValidatorWithPredicate

	if cfg.Validators.Git.Add != nil && cfg.Validators.Git.Add.IsEnabled() {
		validators = append(validators, f.createAddValidator(cfg.Validators.Git.Add))
	}

	if cfg.Validators.Git.NoVerify != nil && cfg.Validators.Git.NoVerify.IsEnabled() {
		validators = append(validators, f.createNoVerifyValidator(cfg.Validators.Git.NoVerify))
	}

	if cfg.Validators.Git.Commit != nil && cfg.Validators.Git.Commit.IsEnabled() {
		validators = append(validators, f.createCommitValidator(cfg.Validators.Git.Commit))
	}

	if cfg.Validators.Git.Push != nil && cfg.Validators.Git.Push.IsEnabled() {
		validators = append(validators, f.createPushValidator(cfg.Validators.Git.Push))
	}

	if cfg.Validators.Git.PR != nil && cfg.Validators.Git.PR.IsEnabled() {
		validators = append(validators, f.createPRValidator(cfg.Validators.Git.PR))
	}

	if cfg.Validators.Git.Branch != nil && cfg.Validators.Git.Branch.IsEnabled() {
		validators = append(validators, f.createBranchValidator(cfg.Validators.Git.Branch))
	}

	return validators
}

func (f *GitValidatorFactory) createAddValidator(
	cfg *config.AddValidatorConfig,
) ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewAddValidator(f.log, nil, cfg),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.GitSubcommandIs("add"),
		),
	}
}

func (f *GitValidatorFactory) createNoVerifyValidator(
	cfg *config.NoVerifyValidatorConfig,
) ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewNoVerifyValidator(f.log, cfg),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.GitSubcommandIs("commit"),
		),
	}
}

func (f *GitValidatorFactory) createCommitValidator(
	cfg *config.CommitValidatorConfig,
) ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewCommitValidator(f.log, nil, cfg),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.GitSubcommandIs("commit"),
		),
	}
}

func (f *GitValidatorFactory) createPushValidator(
	cfg *config.PushValidatorConfig,
) ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewPushValidator(f.log, nil, cfg),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.GitSubcommandIs("push"),
		),
	}
}

func (f *GitValidatorFactory) createPRValidator(
	cfg *config.PRValidatorConfig,
) ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewPRValidator(cfg, f.log),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("gh pr create"),
		),
	}
}

func (f *GitValidatorFactory) createBranchValidator(
	cfg *config.BranchValidatorConfig,
) ValidatorWithPredicate {
	return ValidatorWithPredicate{
		Validator: gitvalidators.NewBranchValidator(cfg, f.log),
		Predicate: validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.Or(
				// git checkout -b or --branch (create new branch)
				validator.GitSubcommandWithAnyFlag("checkout", "-b", "--branch"),
				// git switch -c/--create/-C/--force-create (create new branch)
				validator.GitSubcommandWithAnyFlag(
					"switch",
					"-c",
					"--create",
					"-C",
					"--force-create",
				),
				// git branch without delete flags (create new branch)
				validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete"),
			),
		),
	}
}
