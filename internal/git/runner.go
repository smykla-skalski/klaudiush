package git

//go:generate mockgen -source=runner.go -destination=runner_mock.go -package=git

// Runner defines the interface for git operations
type Runner interface {
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
