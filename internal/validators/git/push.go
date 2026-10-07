package git

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/templates"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

const (
	defaultRemote = "origin"
)

// PushValidator validates git push commands
type PushValidator struct {
	validator.BaseValidator
	gitRunner GitRunner
	config    *config.PushValidatorConfig
}

// NewPushValidator creates a new PushValidator instance
func NewPushValidator(
	log logger.Logger,
	gitRunner GitRunner,
	cfg *config.PushValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *PushValidator {
	return &PushValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules(
			"validate-git-push", log, ruleAdapter,
		),
		gitRunner: defaultGitRunner(gitRunner),
		config:    cfg,
	}
}

// Name returns the validator name
func (*PushValidator) Name() string {
	return "validate-git-push"
}

// Validate validates git push commands
func (v *PushValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	return ValidateGitSubcommand(
		ctx,
		hookCtx,
		v.Logger(),
		"push",
		v.validatePushCommand,
	)
}

// validatePushCommand validates a single git push command
func (v *PushValidator) validatePushCommand(
	gitCmd *parser.GitCommand,
	pendingRemotes map[string]bool,
) *validator.Result {
	log := v.Logger()

	// Use path-specific runner if -C flag is present
	runner := v.getRunnerForCommand(gitCmd)

	inRepo, err := runner.IsInRepo()
	if err != nil {
		return repoCheckUnavailable(err)
	}

	if !inRepo {
		log.Debug("not in a git repository, skipping validation")
		return validator.Pass()
	}

	remote, unavailable := v.extractRemote(gitCmd, runner)
	if unavailable != nil {
		return unavailable
	}

	if remote == "" {
		log.Debug("no remote specified, skipping validation")
		return validator.Pass()
	}

	// A bare variable remote (e.g. "git push $REMOTE main") is an unresolved
	// expansion whose runtime value the hook cannot see, so neither the blocked
	// list nor existence can be checked meaningfully. Skip rather than block.
	if isBareExpansion(remote) {
		log.Debug("remote is an unresolved variable; skipping validation", "remote", remote)

		return validator.Pass()
	}

	// Check if remote is blocked (before checking if it exists)
	if result := v.validateNotBlockedRemote(remote, runner); !result.Passed {
		return result
	}

	if v.config != nil && len(v.config.BlockedBranches) > 0 {
		if result := v.validateBlockedBranches(gitCmd, runner); !result.Passed {
			return result
		}
	}

	// Skip remote existence check if a preceding command adds this remote
	if pendingRemotes[remote] {
		v.Logger().
			Debug("remote being added by preceding command, skipping check", "remote", remote)

		return validator.Pass()
	}

	return v.validateRemoteExists(remote, runner)
}

func repoCheckUnavailable(err error) *validator.Result {
	reason := validator.ReasonError
	if errors.Is(err, context.DeadlineExceeded) {
		reason = validator.ReasonTimeout
	} else if errors.Is(err, context.Canceled) {
		reason = validator.ReasonCanceled
	}

	return validator.Unavailable(
		reason,
		"Could not determine whether the command runs in a git repository",
	)
}

// getRunnerForCommand returns the appropriate git runner for the command.
// If the command specifies a working directory with -C, creates a runner for that path.
// Otherwise, returns the default cached runner.
//

func (v *PushValidator) getRunnerForCommand(gitCmd *parser.GitCommand) GitRunner {
	workDir := gitCmd.GetWorkingDirectory()
	if workDir != "" {
		v.Logger().Debug("using path-specific runner", "path", workDir)
		return NewGitRunnerForPath(workDir)
	}

	return v.gitRunner
}

// extractRemote extracts the remote name from a git push command
func (*PushValidator) extractRemote(
	gitCmd *parser.GitCommand,
	runner GitRunner,
) (string, *validator.Result) {
	if len(gitCmd.Args) == 0 {
		return extractImplicitRemote(gitCmd, runner)
	}

	for _, arg := range gitCmd.Args {
		if !strings.HasPrefix(arg, "-") {
			return arg, nil
		}
	}

	return "", nil
}

func extractImplicitRemote(
	gitCmd *parser.GitCommand,
	runner GitRunner,
) (string, *validator.Result) {
	if repo := pushRepo(gitCmd); repo != "" {
		return repo, nil
	}

	branch, err := runner.GetCurrentBranch()
	if err != nil {
		if unavailable := runnerUnavailable(
			err,
			"Could not determine the current git branch",
		); unavailable != nil {
			return "", unavailable
		}

		return defaultRemote, nil
	}

	remote, err := runner.GetBranchRemote(branch)
	if err != nil {
		if unavailable := runnerUnavailable(
			err,
			"Could not determine the branch's git remote",
		); unavailable != nil {
			return "", unavailable
		}

		return defaultRemote, nil
	}

	return remote, nil
}

func runnerUnavailable(err error, message string) *validator.Result {
	if isBenignLookupError(err) {
		return nil
	}

	reason := validator.ReasonError

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		reason = validator.ReasonTimeout
	case errors.Is(err, context.Canceled):
		reason = validator.ReasonCanceled
	}

	return validator.Unavailable(reason, message)
}

func isBenignLookupError(err error) bool {
	return errors.Is(err, gitpkg.ErrNoHead) ||
		errors.Is(err, gitpkg.ErrDetachedHead) ||
		errors.Is(err, gitpkg.ErrRemoteNotFound) ||
		errors.Is(err, gitpkg.ErrBranchNotFound) ||
		errors.Is(err, gitpkg.ErrNoTracking)
}

// validateNotBlockedRemote checks if the remote is blocked
func (v *PushValidator) validateNotBlockedRemote(
	remote string,
	runner GitRunner,
) *validator.Result {
	// No config or no blocked remotes means all remotes are allowed
	if v.config == nil || len(v.config.BlockedRemotes) == 0 {
		return validator.Pass()
	}

	// Check if remote is in blocked list
	if !slices.Contains(v.config.BlockedRemotes, remote) {
		return validator.Pass()
	}

	// Format blocked remotes as comma-separated string
	blockedRemotesStr := strings.Join(v.config.BlockedRemotes, ", ")

	// Remote is blocked - get all available remotes
	allRemotes, err := runner.GetRemotes()
	if err != nil {
		// If we can't get remotes, just show blocked list without suggestions
		return validator.FailWithRef(
			validator.RefGitBlockedRemote,
			templates.MustExecute(
				templates.PushBlockedRemoteTemplate,
				templates.PushBlockedRemoteData{
					Remote:            remote,
					BlockedRemotesStr: blockedRemotesStr,
				},
			),
		)
	}

	// Get allowed remote priority list (default: ["origin", "upstream"])
	allowedPriority := v.config.AllowedRemotePriority
	if len(allowedPriority) == 0 {
		allowedPriority = []string{"origin", "upstream"}
	}

	// Find suggested remotes based on priority list
	var suggestedRemoteNames []string

	for _, priorityRemote := range allowedPriority {
		if _, exists := allRemotes[priorityRemote]; exists {
			// Don't suggest blocked remotes
			if !slices.Contains(v.config.BlockedRemotes, priorityRemote) {
				suggestedRemoteNames = append(suggestedRemoteNames, priorityRemote)
			}
		}
	}

	// If no suggested remotes from priority list, show all available remotes
	var availableRemoteNames []string

	if len(suggestedRemoteNames) == 0 {
		for name := range allRemotes {
			// Don't show blocked remotes
			if !slices.Contains(v.config.BlockedRemotes, name) {
				availableRemoteNames = append(availableRemoteNames, name)
			}
		}
	}

	result := validator.FailWithRef(
		validator.RefGitBlockedRemote,
		templates.MustExecute(
			templates.PushBlockedRemoteTemplate,
			templates.PushBlockedRemoteData{
				Remote:              remote,
				BlockedRemotesStr:   blockedRemotesStr,
				SuggestedRemotesStr: strings.Join(suggestedRemoteNames, ", "),
				AvailableRemotesStr: strings.Join(availableRemoteNames, ", "),
			},
		),
	)

	if len(suggestedRemoteNames) > 0 {
		result = result.WithFixHint(
			"Push to '" + suggestedRemoteNames[0] + "' instead: git push " + suggestedRemoteNames[0] + " <branch>",
		)
	}

	return result
}

// pushRepo returns the repository --repo names, which git pushes to when no
// repository argument is given.
func pushRepo(gitCmd *parser.GitCommand) string {
	return gitCmd.FlagMap["--repo"]
}

// allBranchFlags push every local branch, so any blocked branch that exists
// locally is pushed too.
var allBranchFlags = []string{flagAll, "--branches", "--mirror"}

const flagAll = "--all"

// validateBlockedBranches checks every branch a push updates against the
// blocked list. A target it cannot name (HEAD on a detached or unreadable
// repository, a matching ":" refspec, --all) fails closed.
func (v *PushValidator) validateBlockedBranches(
	gitCmd *parser.GitCommand,
	runner GitRunner,
) *validator.Result {
	for _, flag := range allBranchFlags {
		if gitCmd.HasFlag(flag) {
			return v.uncheckedBranch(flag + " (every local branch)")
		}
	}

	if len(gitCmd.Args) <= 1 {
		branch, err := runner.GetCurrentBranch()
		if err != nil {
			if unavailable := runnerUnavailable(
				err,
				"Could not determine the current git branch",
			); unavailable != nil {
				return unavailable
			}

			return v.uncheckedBranch("current branch")
		}

		return v.validateNotBlockedBranch(branch)
	}

	for _, refspec := range gitCmd.Args[1:] {
		branch, known, err := refspecBranch(refspec, runner)
		if err != nil {
			if unavailable := runnerUnavailable(
				err,
				"Could not determine the push target branch",
			); unavailable != nil {
				return unavailable
			}
		}

		if !known {
			return v.uncheckedBranch("'" + refspec + "'")
		}

		if result := v.validateNotBlockedBranch(branch); !result.Passed {
			return result
		}
	}

	return validator.Pass()
}

// refspecBranch returns the branch a refspec updates: its destination, or
// its source when it has none, with a force + and refs/heads/ removed and
// HEAD or @ resolved to the current branch. It returns "" for a ref that is
// not a branch (a tag), and false when the branch cannot be known.
func refspecBranch(refspec string, runner GitRunner) (string, bool, error) {
	spec := strings.TrimPrefix(refspec, "+")
	if spec == "" || spec == ":" {
		return "", false, nil
	}

	target := spec
	if src, dst, found := strings.Cut(spec, ":"); found {
		target = dst
		if dst == "" {
			target = src
		}
	}

	if target == "HEAD" || target == "@" {
		branch, err := runner.GetCurrentBranch()
		if err != nil || branch == "" || branch == "HEAD" {
			return "", false, err
		}

		return branch, true, nil
	}

	if branch, ok := strings.CutPrefix(target, "refs/heads/"); ok {
		return branch, true, nil
	}

	if strings.HasPrefix(target, "refs/") {
		return "", true, nil
	}

	return target, true, nil
}

// validateNotBlockedBranch checks if the branch is blocked. A branch pattern
// such as * in a wildcard refspec counts when it matches a blocked branch.
func (v *PushValidator) validateNotBlockedBranch(branch string) *validator.Result {
	if v.config == nil || len(v.config.BlockedBranches) == 0 || branch == "" {
		return validator.Pass()
	}

	blocked := slices.IndexFunc(v.config.BlockedBranches, func(name string) bool {
		matched, err := path.Match(branch, name)

		return name == branch || err == nil && matched
	})
	if blocked < 0 {
		return validator.Pass()
	}

	return validator.FailWithRef(
		validator.RefGitBlockedBranch,
		templates.MustExecute(
			templates.PushBlockedBranchTemplate,
			templates.PushBlockedBranchData{
				Branch:             v.config.BlockedBranches[blocked],
				BlockedBranchesStr: strings.Join(v.config.BlockedBranches, ", "),
			},
		),
	)
}

// uncheckedBranch blocks a push whose target branches cannot be named.
func (v *PushValidator) uncheckedBranch(target string) *validator.Result {
	return validator.FailWithRef(
		validator.RefGitBlockedBranch,
		templates.MustExecute(
			templates.PushUncheckedBranchTemplate,
			templates.PushBlockedBranchData{
				Branch:             target,
				BlockedBranchesStr: strings.Join(v.config.BlockedBranches, ", "),
			},
		),
	)
}

// validateRemoteExists checks if the remote exists
func (*PushValidator) validateRemoteExists(remote string, runner GitRunner) *validator.Result {
	helper := NewRemoteHelper()

	return helper.ValidateRemoteExists(
		remote,
		runner,
		validator.RefGitNoRemote,
	)
}

// Category returns the validator category for parallel execution.
// PushValidator uses CategoryGit because it queries git remote and branch state.
func (*PushValidator) Category() validator.ValidatorCategory {
	return validator.CategoryGit
}
