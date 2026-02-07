package factory

import (
	"github.com/smykla-labs/klaudiush/internal/git"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	gitvalidators "github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// GitValidatorFactory creates git validators from configuration.
type GitValidatorFactory struct {
	cfg        *config.Config
	log        logger.Logger
	gitRunner  git.Runner
	ruleEngine *rules.RuleEngine
}

// NewGitValidatorFactory creates a new GitValidatorFactory.
func NewGitValidatorFactory(log logger.Logger) *GitValidatorFactory {
	return &GitValidatorFactory{log: log}
}

// getGitRunner returns the shared cached git runner, creating it lazily.
//
//nolint:ireturn,nolintlint // Method intentionally returns interface for flexibility
func (f *GitValidatorFactory) getGitRunner() git.Runner {
	if f.gitRunner == nil {
		// Create a cached runner wrapping the default git runner.
		// All validators created by this factory will share this cached runner,
		// eliminating redundant git operations within a single dispatch.
		f.gitRunner = git.NewCachedRunner(gitvalidators.NewGitRunner())
	}

	return f.gitRunner
}

// SetRuleEngine sets the rule engine for the factory.
func (f *GitValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.ruleEngine = engine
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

	if cfg.Validators.Git.Fetch != nil && cfg.Validators.Git.Fetch.IsEnabled() {
		validators = append(validators, f.createFetchValidator(cfg.Validators.Git.Fetch))
	}

	if cfg.Validators.Git.PR != nil && cfg.Validators.Git.PR.IsEnabled() {
		validators = append(validators, f.createPRValidator(cfg.Validators.Git.PR))
	}

	if cfg.Validators.Git.Branch != nil && cfg.Validators.Git.Branch.IsEnabled() {
		validators = append(validators, f.createBranchValidator(cfg.Validators.Git.Branch))
	}

	if cfg.Validators.Git.Merge != nil && cfg.Validators.Git.Merge.IsEnabled() {
		validators = append(validators, f.createMergeValidator(cfg.Validators.Git.Merge))
	}

	return validators
}

func (f *GitValidatorFactory) createAddValidator(
	cfg *config.AddValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitAdd,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewAddValidator(f.log, f.getGitRunner(), cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.GitSubcommandIs("add"),
		),
	}
}

func (f *GitValidatorFactory) createNoVerifyValidator(
	cfg *config.NoVerifyValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitNoVerify,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewNoVerifyValidator(f.log, cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.GitSubcommandIs("commit"),
		),
	}
}

func (f *GitValidatorFactory) createCommitValidator(
	cfg *config.CommitValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitCommit,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewCommitValidator(f.log, f.getGitRunner(), cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.GitSubcommandIs("commit"),
		),
	}
}

func (f *GitValidatorFactory) createPushValidator(
	cfg *config.PushValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitPush,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewPushValidator(f.log, f.getGitRunner(), cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.GitSubcommandIs("push"),
		),
	}
}

func (f *GitValidatorFactory) createFetchValidator(
	cfg *config.FetchValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitFetch,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewFetchValidator(f.log, f.getGitRunner(), cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.GitSubcommandIs("fetch"),
		),
	}
}

func (f *GitValidatorFactory) createPRValidator(
	cfg *config.PRValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitPR,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewPRValidator(cfg, f.log, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("gh pr create"),
		),
	}
}

func (f *GitValidatorFactory) createBranchValidator(
	cfg *config.BranchValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitBranch,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewBranchValidator(cfg, f.log, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
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

func (f *GitValidatorFactory) createMergeValidator(
	cfg *config.MergeValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitMerge,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: gitvalidators.NewMergeValidator(f.log, f.getGitRunner(), cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("gh pr merge"),
		),
	}
}
