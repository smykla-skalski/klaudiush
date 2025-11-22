package git

// MockGitRunner implements GitRunner for testing without executing git commands
type MockGitRunner struct {
	InRepo         bool
	StagedFiles    []string
	ModifiedFiles  []string
	UntrackedFiles []string
	RepoRoot       string
	Err            error
}

// NewMockGitRunner creates a new MockGitRunner instance
func NewMockGitRunner() *MockGitRunner {
	return &MockGitRunner{
		InRepo:         true,
		StagedFiles:    []string{},
		ModifiedFiles:  []string{},
		UntrackedFiles: []string{},
		RepoRoot:       "/mock/repo",
		Err:            nil,
	}
}

// IsInRepo checks if we're in a git repository
func (m *MockGitRunner) IsInRepo() bool {
	return m.InRepo
}

// GetStagedFiles returns the list of staged files
func (m *MockGitRunner) GetStagedFiles() ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.StagedFiles, nil
}

// GetModifiedFiles returns the list of modified but unstaged files
func (m *MockGitRunner) GetModifiedFiles() ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.ModifiedFiles, nil
}

// GetUntrackedFiles returns the list of untracked files
func (m *MockGitRunner) GetUntrackedFiles() ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.UntrackedFiles, nil
}

// GetRepoRoot returns the git repository root directory
func (m *MockGitRunner) GetRepoRoot() (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	return m.RepoRoot, nil
}

// Ensure MockGitRunner implements GitRunner
var _ GitRunner = (*MockGitRunner)(nil)
