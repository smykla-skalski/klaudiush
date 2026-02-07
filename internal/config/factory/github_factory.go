package factory

import (
	"time"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	githubvalidators "github.com/smykla-labs/klaudiush/internal/validators/github"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const defaultLinterTimeout = 10 * time.Second

// GitHubValidatorFactory creates GitHub CLI validators from configuration.
type GitHubValidatorFactory struct {
	cfg        *config.Config
	log        logger.Logger
	ruleEngine *rules.RuleEngine
}

// NewGitHubValidatorFactory creates a new GitHubValidatorFactory.
func NewGitHubValidatorFactory(log logger.Logger) *GitHubValidatorFactory {
	return &GitHubValidatorFactory{log: log}
}

// SetRuleEngine sets the rule engine for the factory.
func (f *GitHubValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.ruleEngine = engine
}

// CreateValidators creates all GitHub CLI validators based on configuration.
func (f *GitHubValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	f.cfg = cfg

	var validators []ValidatorWithPredicate

	// Check if GitHub config exists.
	if cfg.Validators == nil || cfg.Validators.GitHub == nil {
		return validators
	}

	ghCfg := cfg.Validators.GitHub

	// Issue validator - create only if explicitly configured and enabled.
	if ghCfg.Issue != nil && ghCfg.Issue.IsEnabled() {
		validators = append(validators, f.createIssueValidator(ghCfg.Issue))
	}

	return validators
}

func (f *GitHubValidatorFactory) createIssueValidator(
	cfg *config.IssueValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter

	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitHubIssue,
			rules.WithAdapterLogger(f.log),
		)
	}

	// Create markdown linter.
	runner := execpkg.NewCommandRunner(defaultLinterTimeout)
	linter := linters.NewMarkdownLinter(runner)

	return ValidatorWithPredicate{
		Validator: githubvalidators.NewIssueValidator(cfg, linter, f.log, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("gh issue create"),
		),
	}
}
