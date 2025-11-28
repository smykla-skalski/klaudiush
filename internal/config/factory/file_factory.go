package factory

import (
	"time"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	githubpkg "github.com/smykla-labs/klaudiush/internal/github"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	filevalidators "github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	// DefaultLinterTimeout is the default timeout for linter operations.
	DefaultLinterTimeout = 10 * time.Second
)

// FileValidatorFactory creates file validators from configuration.
type FileValidatorFactory struct {
	log        logger.Logger
	ruleEngine *rules.RuleEngine
}

// NewFileValidatorFactory creates a new FileValidatorFactory.
func NewFileValidatorFactory(log logger.Logger) *FileValidatorFactory {
	return &FileValidatorFactory{log: log}
}

// SetRuleEngine sets the rule engine for the factory.
func (f *FileValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.ruleEngine = engine
}

// CreateValidators creates all file validators based on configuration.
func (f *FileValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	var validators []ValidatorWithPredicate

	// Determine timeout from config or use default
	timeout := DefaultLinterTimeout
	if cfg.Global != nil && cfg.Global.DefaultTimeout.ToDuration() > 0 {
		timeout = cfg.Global.DefaultTimeout.ToDuration()
	}

	// Initialize linters
	runner := execpkg.NewCommandRunner(timeout)
	shellChecker := linters.NewShellChecker(runner)
	terraformFormatter := linters.NewTerraformFormatter(runner)
	tfLinter := linters.NewTfLinter(runner)
	actionLinter := linters.NewActionLinter(runner)
	githubClient := githubpkg.NewClient()

	if cfg.Validators.File.Markdown != nil && cfg.Validators.File.Markdown.IsEnabled() {
		// Create markdown linter with config for rule support
		markdownLinter := linters.NewMarkdownLinterWithConfig(runner, cfg.Validators.File.Markdown)

		validators = append(
			validators,
			f.createMarkdownValidator(cfg.Validators.File.Markdown, markdownLinter),
		)
	}

	if cfg.Validators.File.Terraform != nil && cfg.Validators.File.Terraform.IsEnabled() {
		validators = append(validators, f.createTerraformValidator(
			cfg.Validators.File.Terraform, terraformFormatter, tfLinter))
	}

	if cfg.Validators.File.ShellScript != nil && cfg.Validators.File.ShellScript.IsEnabled() {
		validators = append(
			validators,
			f.createShellScriptValidator(cfg.Validators.File.ShellScript, shellChecker),
		)
	}

	if cfg.Validators.File.Workflow != nil && cfg.Validators.File.Workflow.IsEnabled() {
		validators = append(validators, f.createWorkflowValidator(
			cfg.Validators.File.Workflow, actionLinter, githubClient))
	}

	return validators
}

func (f *FileValidatorFactory) createMarkdownValidator(
	cfg *config.MarkdownValidatorConfig,
	linter linters.MarkdownLinter,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileMarkdown,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: filevalidators.NewMarkdownValidator(cfg, linter, f.log, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.FileExtensionIs(".md"),
		),
	}
}

func (f *FileValidatorFactory) createTerraformValidator(
	cfg *config.TerraformValidatorConfig,
	formatter linters.TerraformFormatter,
	linter linters.TfLinter,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileTerraform,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: filevalidators.NewTerraformValidator(formatter, linter, f.log, cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.FileExtensionIs(".tf"),
		),
	}
}

func (f *FileValidatorFactory) createShellScriptValidator(
	cfg *config.ShellScriptValidatorConfig,
	checker linters.ShellChecker,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileShell,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: filevalidators.NewShellScriptValidator(f.log, checker, cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.Or(
				validator.FileExtensionIs(".sh"),
				validator.FileExtensionIs(".bash"),
			),
		),
	}
}

func (f *FileValidatorFactory) createWorkflowValidator(
	cfg *config.WorkflowValidatorConfig,
	linter linters.ActionLinter,
	githubClient githubpkg.Client,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileWorkflow,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: filevalidators.NewWorkflowValidator(
			linter, githubClient, f.log, cfg, ruleAdapter,
		),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.Or(
				validator.FilePathContains(".github/workflows/"),
				validator.FilePathContains(".github/actions/"),
			),
			validator.Or(
				validator.FileExtensionIs(".yml"),
				validator.FileExtensionIs(".yaml"),
			),
		),
	}
}
