package factory

import (
	"time"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	githubpkg "github.com/smykla-skalski/klaudiush/internal/github"
	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	filevalidators "github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
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

// fileValidatorCheck pairs an enablement condition with the validator it creates.
type fileValidatorCheck struct {
	enabled bool
	key     string
	create  func() ValidatorWithPredicate
}

// CreateValidators creates all file validators based on configuration.
func (f *FileValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	var validators []ValidatorWithPredicate

	// Determine timeout from config or use default
	timeout := DefaultLinterTimeout
	if cfg.Global != nil && cfg.Global.DefaultTimeout.ToDuration() > 0 {
		timeout = cfg.Global.DefaultTimeout.ToDuration()
	}

	runner := execpkg.NewCommandRunner(timeout)

	for _, c := range f.fileValidatorChecks(cfg, runner) {
		if c.enabled && !isValidatorOverridden(cfg.Overrides, c.key) {
			validators = append(validators, c.create())
		}
	}

	return validators
}

// fileValidatorChecks builds the enablement/factory pairs for every file validator.
func (f *FileValidatorFactory) fileValidatorChecks(
	cfg *config.Config,
	runner execpkg.CommandRunner,
) []fileValidatorCheck {
	shellChecker := linters.NewShellChecker(runner)
	terraformFormatter := linters.NewTerraformFormatter(runner)
	tfLinter := linters.NewTfLinter(runner)
	actionLinter := linters.NewActionLinter(runner)
	gofumptChecker := linters.NewGofumptChecker(runner)
	ruffChecker := linters.NewRuffChecker(runner)
	oxlintChecker := linters.NewOxlintChecker(runner)
	rustfmtChecker := linters.NewRustfmtChecker(runner)

	fc := cfg.Validators.File

	return []fileValidatorCheck{
		{
			enabled: fc.Markdown != nil && fc.Markdown.IsEnabled(),
			key:     "file.markdown",
			create: func() ValidatorWithPredicate {
				markdownLinter := linters.NewMarkdownLinterWithConfig(runner, fc.Markdown)
				return f.createMarkdownValidator(fc.Markdown, markdownLinter)
			},
		},
		{
			enabled: fc.Terraform != nil && fc.Terraform.IsEnabled(),
			key:     "file.terraform",
			create: func() ValidatorWithPredicate {
				return f.createTerraformValidator(fc.Terraform, terraformFormatter, tfLinter)
			},
		},
		{
			enabled: fc.ShellScript != nil && fc.ShellScript.IsEnabled(),
			key:     "file.shellscript",
			create: func() ValidatorWithPredicate {
				return f.createShellScriptValidator(fc.ShellScript, shellChecker)
			},
		},
		{
			enabled: fc.Workflow != nil && fc.Workflow.IsEnabled(),
			key:     "file.workflow",
			create: func() ValidatorWithPredicate {
				return f.createWorkflowValidator(
					fc.Workflow,
					actionLinter,
					func() githubpkg.Client { return githubpkg.NewClient() },
				)
			},
		},
		{
			enabled: fc.Gofumpt != nil && fc.Gofumpt.IsEnabled(),
			key:     "file.gofumpt",
			create: func() ValidatorWithPredicate {
				return f.createGofumptValidator(fc.Gofumpt, gofumptChecker)
			},
		},
		{
			enabled: fc.Python != nil && fc.Python.IsEnabled(),
			key:     "file.python",
			create: func() ValidatorWithPredicate {
				return f.createPythonValidator(fc.Python, ruffChecker)
			},
		},
		{
			enabled: fc.JavaScript != nil && fc.JavaScript.IsEnabled(),
			key:     "file.javascript",
			create: func() ValidatorWithPredicate {
				return f.createJavaScriptValidator(fc.JavaScript, oxlintChecker)
			},
		},
		{
			enabled: fc.Rust != nil && fc.Rust.IsEnabled(),
			key:     "file.rust",
			create: func() ValidatorWithPredicate {
				return f.createRustValidator(fc.Rust, rustfmtChecker)
			},
		},
		{
			enabled: fc.LinterIgnore != nil && fc.LinterIgnore.IsEnabled(),
			key:     "file.linter_ignore",
			create: func() ValidatorWithPredicate {
				return f.createLinterIgnoreValidator(fc.LinterIgnore)
			},
		},
		{
			enabled: fc.AIComments != nil && fc.AIComments.IsEnabled(),
			key:     "file.ai_comments",
			create: func() ValidatorWithPredicate {
				return f.createAICommentValidator(fc.AIComments)
			},
		},
	}
}

func (f *FileValidatorFactory) createMarkdownValidator(
	cfg *config.MarkdownValidatorConfig,
	linter linters.MarkdownLinter,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileMarkdown,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewMarkdownValidator(cfg, linter, f.log, rc),
			cfg,
		),
		Predicate: validator.And(
			fileResultPredicate(),
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
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileTerraform,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewTerraformValidator(formatter, linter, f.log, cfg, rc),
			cfg,
		),
		Predicate: validator.And(
			fileResultPredicate(),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.FileExtensionIs(".tf"),
		),
	}
}

func (f *FileValidatorFactory) createShellScriptValidator(
	cfg *config.ShellScriptValidatorConfig,
	checker linters.ShellChecker,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileShell,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewShellScriptValidator(f.log, checker, cfg, rc),
			cfg,
		),
		Predicate: validator.And(
			fileResultPredicate(),
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
	githubClientFactory func() githubpkg.Client,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileWorkflow,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewWorkflowValidator(
				linter, githubClientFactory, f.log, cfg, rc,
			),
			cfg,
		),
		Predicate: validator.And(
			fileResultPredicate(),
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

func (f *FileValidatorFactory) createGofumptValidator(
	cfg *config.GofumptValidatorConfig,
	checker linters.GofumptChecker,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileGofumpt,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewGofumptValidator(f.log, checker, cfg, rc),
			cfg,
		),
		Predicate: validator.And(
			fileResultPredicate(),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.FileExtensionIs(".go"),
		),
	}
}

func (f *FileValidatorFactory) createPythonValidator(
	cfg *config.PythonValidatorConfig,
	checker linters.RuffChecker,
) ValidatorWithPredicate {
	return f.createSingleExtensionValidator(
		rules.ValidatorFilePython,
		cfg,
		".py",
		func(rc validator.RuleChecker) validator.Validator {
			return filevalidators.NewPythonValidator(f.log, checker, cfg, rc)
		},
	)
}

func (f *FileValidatorFactory) createJavaScriptValidator(
	cfg *config.JavaScriptValidatorConfig,
	checker linters.OxlintChecker,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileJavaScript,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewJavaScriptValidator(f.log, checker, cfg, rc),
			cfg,
		),
		Predicate: validator.And(
			fileResultPredicate(),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.Or(
				validator.FileExtensionIs(".js"),
				validator.FileExtensionIs(".ts"),
				validator.FileExtensionIs(".jsx"),
				validator.FileExtensionIs(".tsx"),
			),
		),
	}
}

func (f *FileValidatorFactory) createRustValidator(
	cfg *config.RustValidatorConfig,
	checker linters.RustfmtChecker,
) ValidatorWithPredicate {
	return f.createSingleExtensionValidator(
		rules.ValidatorFileRust,
		cfg,
		".rs",
		func(rc validator.RuleChecker) validator.Validator {
			return filevalidators.NewRustValidator(f.log, checker, cfg, rc)
		},
	)
}

func (f *FileValidatorFactory) createSingleExtensionValidator(
	ruleType rules.ValidatorType,
	cfg severityConfig,
	extension string,
	builder func(validator.RuleChecker) validator.Validator,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			ruleType,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			builder(rc),
			cfg,
		),
		Predicate: validator.And(
			fileResultPredicate(),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
			validator.FileExtensionIs(extension),
		),
	}
}

func (f *FileValidatorFactory) createLinterIgnoreValidator(
	cfg *config.LinterIgnoreValidatorConfig,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileLinterIgnore,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewLinterIgnoreValidator(f.log, cfg, rc),
			cfg,
		),
		Predicate: validator.And(
			beforeToolOnlyPredicate(),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
		),
	}
}

func (f *FileValidatorFactory) createAICommentValidator(
	cfg *config.AICommentValidatorConfig,
) ValidatorWithPredicate {
	var rc validator.RuleChecker
	if f.ruleEngine != nil {
		rc = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorFileAIComments,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: wrapValidatorWithSeverity(
			filevalidators.NewAICommentValidator(f.log, cfg, rc),
			cfg,
		),
		Predicate: validator.And(
			beforeToolOnlyPredicate(),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit, hook.ToolTypeMultiEdit),
		),
	}
}
