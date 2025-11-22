package git

import (
	"context"
	"os/exec"
	"strings"
	"time"
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
}

// RealGitRunner implements GitRunner using actual git commands
type RealGitRunner struct {
	timeout time.Duration
}

// NewRealGitRunner creates a new RealGitRunner instance
func NewRealGitRunner() *RealGitRunner {
	return &RealGitRunner{
		timeout: gitCommandTimeout,
	}
}

// IsInRepo checks if we're in a git repository
func (r *RealGitRunner) IsInRepo() bool {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// GetStagedFiles returns the list of staged files
func (r *RealGitRunner) GetStagedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	files := strings.TrimSpace(string(output))
	if files == "" {
		return []string{}, nil
	}

	return strings.Split(files, "\n"), nil
}

// GetModifiedFiles returns the list of modified but unstaged files
func (r *RealGitRunner) GetModifiedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	files := strings.TrimSpace(string(output))
	if files == "" {
		return []string{}, nil
	}

	return strings.Split(files, "\n"), nil
}

// GetUntrackedFiles returns the list of untracked files
func (r *RealGitRunner) GetUntrackedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-files", "--others", "--exclude-standard")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	files := strings.TrimSpace(string(output))
	if files == "" {
		return []string{}, nil
	}

	return strings.Split(files, "\n"), nil
}

// GetRepoRoot returns the git repository root directory
func (r *RealGitRunner) GetRepoRoot() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
