package git

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/smykla-labs/claude-hooks/internal/exec"
	gitpkg "github.com/smykla-labs/claude-hooks/internal/git"
)

// GitRunner is an alias for the git.Runner interface
// This maintains backward compatibility while using the centralized interface definition
type GitRunner = gitpkg.Runner

// CLIGitRunner implements GitRunner using actual git commands
type CLIGitRunner struct {
	runner  exec.CommandRunner
	timeout time.Duration
}

// NewCLIGitRunner creates a new CLIGitRunner instance
func NewCLIGitRunner() *CLIGitRunner {
	return &CLIGitRunner{
		runner:  exec.NewCommandRunner(gitCommandTimeout),
		timeout: gitCommandTimeout,
	}
}

// NewGitRunner creates a GitRunner instance based on environment configuration
// If CLAUDE_HOOKS_USE_SDK_GIT is set to "true" or "1", attempts to use SDK-based implementation
// Falls back to CLI-based implementation if SDK initialization fails or env var not set
// This function always returns a valid GitRunner instance
//
//nolint:ireturn,nolintlint // Factory function intentionally returns interface
func NewGitRunner() GitRunner {
	useSDK := os.Getenv("CLAUDE_HOOKS_USE_SDK_GIT")
	if useSDK == "true" || useSDK == "1" {
		runner, err := gitpkg.NewSDKRunner()
		if err == nil {
			return runner
		}
		// Fall back to CLI on SDK initialization failure
	}

	return NewCLIGitRunner()
}

// IsInRepo checks if we're in a git repository
func (r *CLIGitRunner) IsInRepo() bool {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "rev-parse", "--git-dir")

	return result.Err == nil
}

// GetStagedFiles returns the list of staged files
func (r *CLIGitRunner) GetStagedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "diff", "--cached", "--name-only")
	if result.Err != nil {
		return nil, result.Err
	}

	return parseLines(result.Stdout), nil
}

// GetModifiedFiles returns the list of modified but unstaged files
func (r *CLIGitRunner) GetModifiedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "diff", "--name-only")
	if result.Err != nil {
		return nil, result.Err
	}

	return parseLines(result.Stdout), nil
}

// GetUntrackedFiles returns the list of untracked files
func (r *CLIGitRunner) GetUntrackedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "ls-files", "--others", "--exclude-standard")
	if result.Err != nil {
		return nil, result.Err
	}

	return parseLines(result.Stdout), nil
}

// GetRepoRoot returns the git repository root directory
func (r *CLIGitRunner) GetRepoRoot() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "rev-parse", "--show-toplevel")
	if result.Err != nil {
		return "", result.Err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetRemoteURL returns the URL for the given remote
func (r *CLIGitRunner) GetRemoteURL(remote string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "remote", "get-url", remote)
	if result.Err != nil {
		return "", result.Err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetCurrentBranch returns the current branch name
func (r *CLIGitRunner) GetCurrentBranch() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "symbolic-ref", "--short", "HEAD")
	if result.Err != nil {
		return "", result.Err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetBranchRemote returns the tracking remote for the given branch
func (r *CLIGitRunner) GetBranchRemote(branch string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	configKey := "branch." + branch + ".remote"

	result := r.runner.Run(ctx, "git", "config", configKey)
	if result.Err != nil {
		return "", result.Err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetRemotes returns the list of all remotes with their URLs
func (r *CLIGitRunner) GetRemotes() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result := r.runner.Run(ctx, "git", "remote", "-v")
	if result.Err != nil {
		return nil, result.Err
	}

	remotes := make(map[string]string)

	lines := strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n")
	for line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)

		const minFieldsRequired = 2

		if len(fields) >= minFieldsRequired {
			remoteName := fields[0]
			remoteURL := fields[1]
			// Only add each remote once (git remote -v shows fetch and push separately)
			if _, exists := remotes[remoteName]; !exists {
				remotes[remoteName] = remoteURL
			}
		}
	}

	return remotes, nil
}

// parseLines splits output by newlines and filters empty lines
func parseLines(output string) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return []string{}
	}

	return strings.Split(output, "\n")
}
