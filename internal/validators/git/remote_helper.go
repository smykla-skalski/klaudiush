package git

import (
	"context"
	"sort"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/templates"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

const (
	// gitCmdName is the name of the git command.
	gitCmdName = "git"
)

// GitCommandValidatorFunc is a function that validates a parsed git command.
type GitCommandValidatorFunc func(gitCmd *parser.GitCommand) *validator.Result

// RemoteHelper provides shared remote validation logic for git validators.
type RemoteHelper struct{}

// NewRemoteHelper creates a new RemoteHelper.
func NewRemoteHelper() *RemoteHelper {
	return &RemoteHelper{}
}

// ValidateRemoteExists checks if the remote exists and returns a validation result.
// Returns Pass() if remote exists, Fail() with available remotes list if not.
func (*RemoteHelper) ValidateRemoteExists(
	remote string,
	runner GitRunner,
	ref validator.Reference,
	headerMsg string,
) *validator.Result {
	_, err := runner.GetRemoteURL(remote)
	if err != nil {
		remotes, remoteErr := runner.GetRemotes()
		if remoteErr != nil {
			return validator.FailWithRef(
				ref,
				headerMsg+"\n\n❌ Remote '"+remote+"' does not exist",
			)
		}

		return validator.FailWithRef(
			ref,
			headerMsg+"\n\n"+FormatRemoteNotFoundError(remote, remotes),
		)
	}

	return validator.Pass()
}

// FormatRemoteNotFoundError formats the error message for a missing remote.
func FormatRemoteNotFoundError(remote string, remotes map[string]string) string {
	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}

	sort.Strings(names)

	remoteInfos := make([]templates.RemoteInfo, len(names))
	for i, name := range names {
		remoteInfos[i] = templates.RemoteInfo{
			Name: name,
			URL:  remotes[name],
		}
	}

	return templates.MustExecute(
		templates.PushRemoteNotFoundTemplate,
		templates.PushRemoteNotFoundData{
			Remote:  remote,
			Remotes: remoteInfos,
		},
	)
}

// ValidateGitSubcommand provides common validation loop for git subcommand validators.
// It handles rule checking, command parsing, and subcommand filtering.
func ValidateGitSubcommand(
	ctx context.Context,
	hookCtx *hook.Context,
	ruleAdapter *rules.RuleValidatorAdapter,
	log logger.Logger,
	subcommand string,
	validateCmd GitCommandValidatorFunc,
) *validator.Result {
	// Check rules first if rule adapter is configured
	if ruleAdapter != nil {
		if result := ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	command := hookCtx.GetCommand()
	if command == "" {
		return validator.Pass()
	}

	bashParser := parser.NewBashParser()

	parseResult, err := bashParser.Parse(command)
	if err != nil {
		log.Debug("failed to parse command", "error", err)

		return validator.Pass()
	}

	for _, cmd := range parseResult.Commands {
		if cmd.Name != gitCmdName || len(cmd.Args) == 0 {
			continue
		}

		gitCmd, parseErr := parser.ParseGitCommand(cmd)
		if parseErr != nil {
			log.Debug("failed to parse git command", "error", parseErr)

			continue
		}

		if gitCmd.Subcommand != subcommand {
			continue
		}

		result := validateCmd(gitCmd)
		if !result.Passed {
			return result
		}
	}

	return validator.Pass()
}
