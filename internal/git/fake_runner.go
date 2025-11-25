package git

// FakeRunner implements Runner for testing without executing git commands.
// This is a struct-based fake (not a mock) that allows tests to set state directly.
// For expectation-based testing, use the generated MockRunner from runner_mock.go.
type FakeRunner struct {
	InRepo         bool
	StagedFiles    []string
	ModifiedFiles  []string
	UntrackedFiles []string
	RepoRoot       string
	Remotes        map[string]string
	CurrentBranch  string
	BranchRemotes  map[string]string
	Err            error
}

// NewFakeRunner creates a new FakeRunner instance with sensible defaults.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{
		InRepo:         true,
		StagedFiles:    []string{},
		ModifiedFiles:  []string{},
		UntrackedFiles: []string{},
		RepoRoot:       "/mock/repo",
		Remotes: map[string]string{
			"origin":   "git@github.com:user/repo.git",
			"upstream": "git@github.com:org/repo.git",
		},
		CurrentBranch: "main",
		BranchRemotes: map[string]string{
			"main": "origin",
		},
		Err: nil,
	}
}

// IsInRepo checks if we're in a git repository.
func (f *FakeRunner) IsInRepo() bool {
	return f.InRepo
}

// GetStagedFiles returns the list of staged files.
func (f *FakeRunner) GetStagedFiles() ([]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}

	return f.StagedFiles, nil
}

// GetModifiedFiles returns the list of modified but unstaged files.
func (f *FakeRunner) GetModifiedFiles() ([]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}

	return f.ModifiedFiles, nil
}

// GetUntrackedFiles returns the list of untracked files.
func (f *FakeRunner) GetUntrackedFiles() ([]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}

	return f.UntrackedFiles, nil
}

// GetRepoRoot returns the git repository root directory.
func (f *FakeRunner) GetRepoRoot() (string, error) {
	if f.Err != nil {
		return "", f.Err
	}

	return f.RepoRoot, nil
}

// GetRemoteURL returns the URL for the given remote.
func (f *FakeRunner) GetRemoteURL(remote string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}

	if url, ok := f.Remotes[remote]; ok {
		return url, nil
	}

	return "", &FakeRunnerError{Msg: "remote not found"}
}

// GetCurrentBranch returns the current branch name.
func (f *FakeRunner) GetCurrentBranch() (string, error) {
	if f.Err != nil {
		return "", f.Err
	}

	return f.CurrentBranch, nil
}

// GetBranchRemote returns the tracking remote for the given branch.
func (f *FakeRunner) GetBranchRemote(branch string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}

	if remote, ok := f.BranchRemotes[branch]; ok {
		return remote, nil
	}

	return "", &FakeRunnerError{Msg: "branch remote not found"}
}

// GetRemotes returns the list of all remotes with their URLs.
func (f *FakeRunner) GetRemotes() (map[string]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}

	return f.Remotes, nil
}

// FakeRunnerError is a simple error type for testing.
type FakeRunnerError struct {
	Msg string
}

// Error returns the error message.
func (e *FakeRunnerError) Error() string {
	return e.Msg
}

// Ensure FakeRunner implements Runner.
var _ Runner = (*FakeRunner)(nil)
