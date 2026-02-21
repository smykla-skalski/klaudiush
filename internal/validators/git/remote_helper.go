package git

import (
	"context"
	"sort"

	"github.com/smykla-skalski/klaudiush/internal/templates"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

const (
	// gitCmdName is the name of the git command.
	gitCmdName = "git"
)

// GitCommandValidatorFunc is a function that validates a parsed git command.
// pendingRemotes contains remote names being added by preceding commands
// in the same compound command chain (e.g., git remote add <name> && git fetch <name>).
type GitCommandValidatorFunc func(gitCmd *parser.GitCommand, pendingRemotes map[string]bool) *validator.Result

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
) *validator.Result {
	_, err := runner.GetRemoteURL(remote)
	if err != nil {
		remotes, remoteErr := runner.GetRemotes()
		if remoteErr != nil {
			return validator.FailWithRef(
				ref,
				"Remote '"+remote+"' does not exist",
			)
		}

		return validator.FailWithRef(
			ref,
			FormatRemoteNotFoundError(remote, remotes),
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
// It handles command parsing and subcommand filtering.
// Rule checking should be done by the caller via BaseValidator.CheckRules.
func ValidateGitSubcommand(
	_ context.Context,
	hookCtx *hook.Context,
	log logger.Logger,
	subcommand string,
	validateCmd GitCommandValidatorFunc,
) *validator.Result {
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

	// Track remotes being added by preceding commands in the chain.
	// This prevents false positives when "git remote add X && git fetch X"
	// is used in the same compound command.
	pendingRemotes := make(map[string]bool)

	for _, cmd := range parseResult.Commands {
		if cmd.Name != gitCmdName || len(cmd.Args) == 0 {
			continue
		}

		gitCmd, parseErr := parser.ParseGitCommand(cmd)
		if parseErr != nil {
			log.Debug("failed to parse git command", "error", parseErr)

			continue
		}

		// Collect remotes from "git remote add <name>" commands
		if gitCmd.Subcommand == "remote" && len(gitCmd.Args) >= 2 && gitCmd.Args[0] == "add" {
			pendingRemotes[gitCmd.Args[1]] = true

			continue
		}

		if gitCmd.Subcommand != subcommand {
			continue
		}

		result := validateCmd(gitCmd, pendingRemotes)
		if !result.Passed {
			return result
		}
	}

	return validator.Pass()
}
