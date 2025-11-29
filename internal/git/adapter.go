package git

// RepositoryAdapter adapts the Repository interface to implement Runner
type RepositoryAdapter struct {
	repo Repository
}

// NewRepositoryAdapter creates a new adapter that wraps a Repository
func NewRepositoryAdapter(repo Repository) *RepositoryAdapter {
	return &RepositoryAdapter{repo: repo}
}

// NewSDKRunner creates a Runner backed by the go-git SDK
//
//nolint:ireturn,nolintlint // Factory function intentionally returns interface
func NewSDKRunner() (Runner, error) {
	repo, err := DiscoverRepository()
	if err != nil {
		return nil, err
	}

	return NewRepositoryAdapter(repo), nil
}

// NewSDKRunnerForPath creates a Runner for a specific directory path.
// Use this when operating on a repository in a specific directory,
// e.g., when processing git commands with -C flag.
//
//nolint:ireturn,nolintlint // Factory function intentionally returns interface
func NewSDKRunnerForPath(path string) (Runner, error) {
	repo, err := OpenRepository(path)
	if err != nil {
		return nil, err
	}

	return NewRepositoryAdapter(repo), nil
}

// IsInRepo checks if we're in a git repository
func (a *RepositoryAdapter) IsInRepo() bool {
	return a.repo.IsInRepo()
}

// GetStagedFiles returns the list of staged files
func (a *RepositoryAdapter) GetStagedFiles() ([]string, error) {
	return a.repo.GetStagedFiles()
}

// GetModifiedFiles returns the list of modified but unstaged files
func (a *RepositoryAdapter) GetModifiedFiles() ([]string, error) {
	return a.repo.GetModifiedFiles()
}

// GetUntrackedFiles returns the list of untracked files
func (a *RepositoryAdapter) GetUntrackedFiles() ([]string, error) {
	return a.repo.GetUntrackedFiles()
}

// GetRepoRoot returns the git repository root directory
func (a *RepositoryAdapter) GetRepoRoot() (string, error) {
	return a.repo.GetRoot()
}

// GetRemoteURL returns the URL for the given remote
func (a *RepositoryAdapter) GetRemoteURL(remote string) (string, error) {
	return a.repo.GetRemoteURL(remote)
}

// GetCurrentBranch returns the current branch name
func (a *RepositoryAdapter) GetCurrentBranch() (string, error) {
	return a.repo.GetCurrentBranch()
}

// GetBranchRemote returns the tracking remote for the given branch
func (a *RepositoryAdapter) GetBranchRemote(branch string) (string, error) {
	return a.repo.GetBranchRemote(branch)
}

// GetRemotes returns the list of all remotes with their URLs
func (a *RepositoryAdapter) GetRemotes() (map[string]string, error) {
	return a.repo.GetRemotes()
}
