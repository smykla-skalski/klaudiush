package git

import (
	"context"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

// FetchValidator validates git fetch commands to ensure the remote exists.
type FetchValidator struct {
	validator.BaseValidator
	gitRunner    GitRunner
	config       *config.FetchValidatorConfig
	ruleAdapter  *rules.RuleValidatorAdapter
	remoteHelper *RemoteHelper
}

// NewFetchValidator creates a new FetchValidator instance.
func NewFetchValidator(
	log logger.Logger,
	gitRunner GitRunner,
	cfg *config.FetchValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *FetchValidator {
	if gitRunner == nil {
		gitRunner = NewGitRunner()
	}

	return &FetchValidator{
		BaseValidator: *validator.NewBaseValidator("validate-git-fetch", log),
		gitRunner:     gitRunner,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
		remoteHelper:  NewRemoteHelper(),
	}
}

// Name returns the validator name.
func (*FetchValidator) Name() string {
	return "validate-git-fetch"
}

// Validate validates git fetch commands.
func (v *FetchValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	return ValidateGitSubcommand(
		ctx,
		hookCtx,
		v.ruleAdapter,
		v.Logger(),
		"fetch",
		v.validateFetchCommand,
	)
}

// validateFetchCommand validates a single git fetch command.
func (v *FetchValidator) validateFetchCommand(gitCmd *parser.GitCommand) *validator.Result {
	log := v.Logger()

	// Use path-specific runner if -C flag is present
	runner := v.getRunnerForCommand(gitCmd)

	if !runner.IsInRepo() {
		log.Debug("not in a git repository, skipping validation")

		return validator.Pass()
	}

	remote := v.extractRemote(gitCmd)
	if remote == "" {
		log.Debug("no remote specified, skipping validation")

		return validator.Pass()
	}

	return v.remoteHelper.ValidateRemoteExists(
		remote,
		runner,
		validator.RefGitFetchNoRemote,
		"🚫 Git fetch validation failed:",
	)
}

// getRunnerForCommand returns the appropriate git runner for the command.
// If the command specifies a working directory with -C, creates a runner for that path.
// Otherwise, returns the default cached runner.
//
//nolint:ireturn // Returns interface for flexibility between cached and path-specific runners
func (v *FetchValidator) getRunnerForCommand(gitCmd *parser.GitCommand) GitRunner {
	workDir := gitCmd.GetWorkingDirectory()
	if workDir != "" {
		v.Logger().Debug("using path-specific runner", "path", workDir)

		return NewGitRunnerForPath(workDir)
	}

	return v.gitRunner
}

// extractRemote extracts the remote name from a git fetch command.
// Handles: git fetch <remote>, git fetch --prune <remote>, git fetch -p <remote>
func (*FetchValidator) extractRemote(gitCmd *parser.GitCommand) string {
	if len(gitCmd.Args) == 0 {
		// git fetch with no args fetches from configured upstream or origin
		return ""
	}

	// Find the first non-flag argument (the remote name)
	for _, arg := range gitCmd.Args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}

	return ""
}

// Category returns the validator category for parallel execution.
// FetchValidator uses CategoryGit because it queries git remote state.
func (*FetchValidator) Category() validator.ValidatorCategory {
	return validator.CategoryGit
}
