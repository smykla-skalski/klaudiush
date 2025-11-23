package git

import (
	"context"
	"strings"
	"time"

	"github.com/smykla-labs/claude-hooks/internal/exec"
)

// GitRunner defines the interface for git operations
type GitRunner interface {
	// IsInRepo checks if we're in a git repository
	IsInRepo() bool

	// GetStagedFiles returns the list of staged files
	GetStagedFiles() ([]string, error)

	// GetModifiedFiles returns the list of modified but unstaged files
	GetModifiedFiles() ([]string, error)

	// GetUntrackedFiles returns the list of untracked files
	GetUntrackedFiles() ([]string, error)

	// GetRepoRoot returns the git repository root directory
	GetRepoRoot() (string, error)

	// GetRemoteURL returns the URL for the given remote
	GetRemoteURL(remote string) (string, error)

	// GetCurrentBranch returns the current branch name
	GetCurrentBranch() (string, error)

	// GetBranchRemote returns the tracking remote for the given branch
	GetBranchRemote(branch string) (string, error)

	// GetRemotes returns the list of all remotes with their URLs
	GetRemotes() (map[string]string, error)
}

// RealGitRunner implements GitRunner using actual git commands
type RealGitRunner struct {
	runner  exec.CommandRunner
	timeout time.Duration
}

// NewRealGitRunner creates a new RealGitRunner instance
func NewRealGitRunner() *RealGitRunner {
	return &RealGitRunner{
		runner:  exec.NewCommandRunner(gitCommandTimeout),
		timeout: gitCommandTimeout,
	}
}

// IsInRepo checks if we're in a git repository
func (r *RealGitRunner) IsInRepo() bool {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	_, err := r.runner.Run(ctx, "git", "rev-parse", "--git-dir")
	return err == nil
}

// GetStagedFiles returns the list of staged files
func (r *RealGitRunner) GetStagedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result, err := r.runner.Run(ctx, "git", "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}

	return parseLines(result.Stdout), nil
}

// GetModifiedFiles returns the list of modified but unstaged files
func (r *RealGitRunner) GetModifiedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result, err := r.runner.Run(ctx, "git", "diff", "--name-only")
	if err != nil {
		return nil, err
	}

	return parseLines(result.Stdout), nil
}

// GetUntrackedFiles returns the list of untracked files
func (r *RealGitRunner) GetUntrackedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result, err := r.runner.Run(ctx, "git", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	return parseLines(result.Stdout), nil
}

// GetRepoRoot returns the git repository root directory
func (r *RealGitRunner) GetRepoRoot() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result, err := r.runner.Run(ctx, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetRemoteURL returns the URL for the given remote
func (r *RealGitRunner) GetRemoteURL(remote string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result, err := r.runner.Run(ctx, "git", "remote", "get-url", remote)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetCurrentBranch returns the current branch name
func (r *RealGitRunner) GetCurrentBranch() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result, err := r.runner.Run(ctx, "git", "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetBranchRemote returns the tracking remote for the given branch
func (r *RealGitRunner) GetBranchRemote(branch string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	configKey := "branch." + branch + ".remote"
	result, err := r.runner.Run(ctx, "git", "config", configKey)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result.Stdout), nil
}

// GetRemotes returns the list of all remotes with their URLs
func (r *RealGitRunner) GetRemotes() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	result, err := r.runner.Run(ctx, "git", "remote", "-v")
	if err != nil {
		return nil, err
	}

	remotes := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, line := range lines {
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
