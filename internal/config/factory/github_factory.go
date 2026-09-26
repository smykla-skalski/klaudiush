package factory

import (
	"time"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	githubvalidators "github.com/smykla-skalski/klaudiush/internal/validators/github"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const defaultLinterTimeout = 10 * time.Second

// ghAPICommandPattern is the cheap prefilter for the gh api validator: the gh
// api subcommand, the HTTP clients that can reach the same endpoints, and the
// markers an API client library leaves in an inline script. A client name only
// counts in command position, so an unrelated command that merely carries a
// URL does not pay for a parse.
const ghAPICommandPattern = `\bgh\s+api\b|(^|[\n|&;(])\s*(curl|wget|https?|xhs?)\b|` +
	`\b(sh|bash|zsh|ksh|dash)\s+-c\b|` +
	`\b(sudo|doas|env|command|nice|nohup|timeout|xargs|npx|bunx|uv|pnpm|yarn|poetry|pipx)\s|` +
	`octokit|\.request\(|api\.github\.com|/api/(v3|graphql)/`

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
	if ghCfg.Issue != nil && ghCfg.Issue.IsEnabled() &&
		!isValidatorOverridden(cfg.Overrides, "github.issue") {
		validators = append(validators, f.createIssueValidator(ghCfg.Issue))
	}

	// API validator - rejects gh api calls that create commits behind the hook.
	if ghCfg.API != nil && ghCfg.API.IsEnabled() &&
		!isValidatorOverridden(cfg.Overrides, "github.api") {
		validators = append(validators, f.createAPIValidator(ghCfg.API))
	}

	return validators
}

func (f *GitHubValidatorFactory) createAPIValidator(
	cfg *config.APIValidatorConfig,
) ValidatorWithPredicate {
	var rc validator.RuleChecker

	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitHubAPI,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			githubvalidators.NewAPIValidator(cfg, f.log, rc),
			cfg,
		),
		Predicate: validator.And(
			beforeToolOrProviderAfterToolPredicate(),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.Or(
				validator.CommandMatches(ghAPICommandPattern),
				validator.GHCommandIs("api"),
			),
		),
	}
}

func (f *GitHubValidatorFactory) createIssueValidator(
	cfg *config.IssueValidatorConfig,
) ValidatorWithPredicate {
	var rc validator.RuleChecker

	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorGitHubIssue,
			rules.WithAdapterLogger(f.log),
		)
	}

	// Create markdown linter.
	runner := execpkg.NewCommandRunner(defaultLinterTimeout)
	linter := linters.NewMarkdownLinter(runner)

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			githubvalidators.NewIssueValidator(cfg, linter, f.log, rc),
			cfg,
		),
		Predicate: validator.And(
			beforeToolOrProviderAfterToolPredicate(),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.Or(
				validator.CommandContains("gh issue create"),
				validator.GHCommandIs("issue", "create"),
			),
		),
	}
}
