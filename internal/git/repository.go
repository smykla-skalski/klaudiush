// Package git provides an SDK-based implementation of git operations using go-git v6
package git

import (
	"os"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

// gitEnvVarsToUnset lists git environment variables that must be cleared before
// using go-git SDK to prevent index corruption when running as a hook.
//
// When klaudiush runs as a pre-commit hook, it inherits GIT_INDEX_FILE from the
// parent git process. If go-git reads or writes to this shared index file while
// the parent git process is modifying it, index corruption occurs, causing errors
// like "invalid object 100644<sha>" and "Error building trees".
//
// By clearing these variables, go-git will discover and use the correct index
// file for the worktree, avoiding conflicts with the parent process.
//
// See: https://github.com/pre-commit/pre-commit/issues/1849
var gitEnvVarsToUnset = []string{
	"GIT_INDEX_FILE",
}

func init() {
	clearGitEnvVars()
}

// clearGitEnvVars removes git environment variables that can cause index
// corruption when using go-git SDK in hook contexts.
func clearGitEnvVars() {
	for _, envVar := range gitEnvVarsToUnset {
		_ = os.Unsetenv(envVar)
	}
}

// Repository defines the interface for git repository operations
type Repository interface {
	// IsInRepo checks if we're in a git repository
	IsInRepo() bool

	// GetRoot returns the git repository root directory
	GetRoot() (string, error)

	// GetStagedFiles returns the list of staged files
	GetStagedFiles() ([]string, error)

	// GetModifiedFiles returns the list of modified but unstaged files
	GetModifiedFiles() ([]string, error)

	// GetUntrackedFiles returns the list of untracked files
	GetUntrackedFiles() ([]string, error)

	// GetCurrentBranch returns the current branch name
	GetCurrentBranch() (string, error)

	// GetBranchRemote returns the tracking remote for the given branch
	GetBranchRemote(branch string) (string, error)

	// GetRemoteURL returns the URL for the given remote
	GetRemoteURL(remote string) (string, error)

	// GetRemotes returns the list of all remotes with their URLs
	GetRemotes() (map[string]string, error)
}

// SDKRepository implements Repository using go-git SDK
type SDKRepository struct {
	repo *git.Repository
}

var (
	repoInstance *SDKRepository
	repoOnce     sync.Once
	errRepo      error
)

// ResetRepositoryCache resets the repository cache (for testing only)
func ResetRepositoryCache() {
	repoInstance = nil
	repoOnce = sync.Once{}
	errRepo = nil
}

// DiscoverRepository discovers and caches the git repository from the current directory
func DiscoverRepository() (*SDKRepository, error) {
	repoOnce.Do(func() {
		repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
			DetectDotGit:          true,
			EnableDotGitCommonDir: true,
		})
		if err != nil {
			if errors.Is(err, git.ErrRepositoryNotExists) {
				errRepo = ErrNotRepository
				return
			}

			errRepo = errors.Wrap(err, "failed to open repository")

			return
		}

		repoInstance = &SDKRepository{repo: repo}
	})

	return repoInstance, errRepo
}

// OpenRepository opens a git repository from a specific path (not cached).
// Use this when you need to operate on a repository in a specific directory,
// e.g., when processing git commands with -C flag.
//
// EnableDotGitCommonDir is set to true to properly support linked worktrees.
// Linked worktrees have a .git file (not directory) pointing to the main
// repository's .git/worktrees/<name> directory. This option tells go-git
// to follow the commondir reference to find the actual repository configuration
// including remotes. See: https://github.com/go-git/go-git/issues/225
func OpenRepository(path string) (*SDKRepository, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return nil, ErrNotRepository
		}

		return nil, errors.Wrap(err, "failed to open repository")
	}

	return &SDKRepository{repo: repo}, nil
}

// IsInRepo checks if we're in a git repository
func (r *SDKRepository) IsInRepo() bool {
	return r.repo != nil
}

// GetRoot returns the git repository root directory
func (r *SDKRepository) GetRoot() (string, error) {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return "", errors.Wrap(err, "failed to get worktree")
	}

	return worktree.Filesystem.Root(), nil
}

// GetStagedFiles returns the list of staged files
func (r *SDKRepository) GetStagedFiles() ([]string, error) {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get worktree")
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get status")
	}

	var staged []string

	for file, fileStatus := range status {
		// A file is staged if the staging area differs from HEAD
		if fileStatus.Staging != git.Unmodified {
			staged = append(staged, file)
		}
	}

	return staged, nil
}

// GetModifiedFiles returns the list of modified but unstaged files
func (r *SDKRepository) GetModifiedFiles() ([]string, error) {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get worktree")
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get status")
	}

	var modified []string

	for file, fileStatus := range status {
		// A file is modified if the worktree differs from staging and not untracked
		if fileStatus.Worktree != git.Unmodified && fileStatus.Worktree != git.Untracked {
			modified = append(modified, file)
		}
	}

	return modified, nil
}

// GetUntrackedFiles returns the list of untracked files
func (r *SDKRepository) GetUntrackedFiles() ([]string, error) {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get worktree")
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get status")
	}

	var untracked []string

	for file, fileStatus := range status {
		// A file is untracked if both staging and worktree show as untracked
		if fileStatus.Staging == git.Untracked {
			untracked = append(untracked, file)
		}
	}

	return untracked, nil
}

// GetCurrentBranch returns the current branch name
func (r *SDKRepository) GetCurrentBranch() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", ErrNoHead
		}

		return "", errors.Wrap(err, "failed to get HEAD")
	}

	if !head.Name().IsBranch() {
		return "", ErrDetachedHead
	}

	return head.Name().Short(), nil
}

// GetBranchRemote returns the tracking remote for the given branch
func (r *SDKRepository) GetBranchRemote(branch string) (string, error) {
	// First verify the branch exists
	branchRef := plumbing.NewBranchReferenceName(branch)

	_, err := r.repo.Reference(branchRef, true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", errors.Wrapf(ErrBranchNotFound, "branch %q", branch)
		}

		return "", errors.Wrap(err, "failed to lookup branch")
	}

	cfg, err := r.repo.Config()
	if err != nil {
		return "", errors.Wrap(err, "failed to get config")
	}

	branchCfg, ok := cfg.Branches[branch]
	if !ok || branchCfg.Remote == "" {
		return "", errors.Wrapf(ErrNoTracking, "branch %q", branch)
	}

	return branchCfg.Remote, nil
}

// GetRemoteURL returns the URL for the given remote
func (r *SDKRepository) GetRemoteURL(remote string) (string, error) {
	rem, err := r.repo.Remote(remote)
	if err != nil {
		if errors.Is(err, git.ErrRemoteNotFound) {
			return "", errors.Wrapf(ErrRemoteNotFound, "remote %q", remote)
		}

		return "", errors.Wrap(err, "failed to get remote")
	}

	urls := rem.Config().URLs
	if len(urls) == 0 {
		return "", errors.Errorf("remote %q has no URLs", remote)
	}

	return urls[0], nil
}

// GetRemotes returns the list of all remotes with their URLs
func (r *SDKRepository) GetRemotes() (map[string]string, error) {
	remotes, err := r.repo.Remotes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get remotes")
	}

	result := make(map[string]string)

	for _, remote := range remotes {
		cfg := remote.Config()
		if len(cfg.URLs) > 0 {
			result[cfg.Name] = cfg.URLs[0]
		}
	}

	return result, nil
}
