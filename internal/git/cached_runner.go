// Package git provides git operations with optional caching.
package git

import (
	"sync"
)

// CachedRunner wraps a Runner and caches results for the duration of its lifetime.
// Use this for request-scoped caching where multiple validators need git status
// without redundant calls. The cache is per-instance, not global.
type CachedRunner struct {
	delegate Runner

	// stagedOnce caches GetStagedFiles independently. This is the hot path —
	// called on every commit validation — and must not trigger modified/untracked
	// fetching unless those are actually needed.
	stagedOnce sync.Once
	staged     []string
	stagedErr  error

	// modifiedUntrackedOnce caches GetModifiedFiles + GetUntrackedFiles together.
	// Only fetched when the caller explicitly needs them (e.g. the no-staged-files
	// error path in the commit validator).
	modifiedUntrackedOnce sync.Once
	modified              []string
	untracked             []string
	modifiedUntrackedErr  error

	// Branch cache
	branchOnce sync.Once
	branch     string
	branchErr  error

	// Repository root cache
	repoRootOnce sync.Once
	repoRoot     string
	repoRootErr  error

	// IsInRepo cache
	isInRepoOnce sync.Once
	isInRepo     bool

	// Remotes cache (all remotes)
	remotesOnce sync.Once
	remotes     map[string]string
	remotesErr  error

	// Remote URL cache (per remote name)
	remoteURLMu    sync.RWMutex
	remoteURLCache map[string]remoteURLCacheEntry

	// Branch remote cache (per branch name)
	branchRemoteMu    sync.RWMutex
	branchRemoteCache map[string]branchRemoteCacheEntry
}

type remoteURLCacheEntry struct {
	url string
	err error
}

type branchRemoteCacheEntry struct {
	remote string
	err    error
}

// NewCachedRunner creates a new CachedRunner that wraps the given Runner.
// The cached runner memoizes results for the duration of its lifetime.
func NewCachedRunner(delegate Runner) Runner {
	return &CachedRunner{
		delegate:          delegate,
		remoteURLCache:    make(map[string]remoteURLCacheEntry),
		branchRemoteCache: make(map[string]branchRemoteCacheEntry),
	}
}

// IsInRepo checks if we're in a git repository.
// Result is cached.
func (c *CachedRunner) IsInRepo() bool {
	c.isInRepoOnce.Do(func() {
		c.isInRepo = c.delegate.IsInRepo()
	})

	return c.isInRepo
}

// ensureModifiedUntracked fetches modified and untracked files together.
// Only called when the caller actually needs these (e.g. the no-staged-files
// error path). Kept separate from staged so the happy path never pays for it.
func (c *CachedRunner) ensureModifiedUntracked() {
	c.modifiedUntrackedOnce.Do(func() {
		c.modified, c.modifiedUntrackedErr = c.delegate.GetModifiedFiles()
		if c.modifiedUntrackedErr != nil {
			return
		}

		c.untracked, c.modifiedUntrackedErr = c.delegate.GetUntrackedFiles()
	})
}

// GetStagedFiles returns the list of staged files.
// Result is cached independently of modified/untracked files.
func (c *CachedRunner) GetStagedFiles() ([]string, error) {
	c.stagedOnce.Do(func() {
		c.staged, c.stagedErr = c.delegate.GetStagedFiles()
	})

	return c.staged, c.stagedErr
}

// GetModifiedFiles returns the list of modified but unstaged files.
// Result is cached together with untracked files.
func (c *CachedRunner) GetModifiedFiles() ([]string, error) {
	c.ensureModifiedUntracked()

	if c.modifiedUntrackedErr != nil && c.modified == nil {
		return nil, c.modifiedUntrackedErr
	}

	return c.modified, nil
}

// GetUntrackedFiles returns the list of untracked files.
// Result is cached together with modified files.
func (c *CachedRunner) GetUntrackedFiles() ([]string, error) {
	c.ensureModifiedUntracked()

	if c.modifiedUntrackedErr != nil && c.untracked == nil {
		return nil, c.modifiedUntrackedErr
	}

	return c.untracked, nil
}

// GetRepoRoot returns the git repository root directory.
// Result is cached.
func (c *CachedRunner) GetRepoRoot() (string, error) {
	c.repoRootOnce.Do(func() {
		c.repoRoot, c.repoRootErr = c.delegate.GetRepoRoot()
	})

	return c.repoRoot, c.repoRootErr
}

// GetCurrentBranch returns the current branch name.
// Result is cached.
func (c *CachedRunner) GetCurrentBranch() (string, error) {
	c.branchOnce.Do(func() {
		c.branch, c.branchErr = c.delegate.GetCurrentBranch()
	})

	return c.branch, c.branchErr
}

// GetRemoteURL returns the URL for the given remote.
// Results are cached per remote name.
//
//nolint:dupl // Similar pattern to GetBranchRemote but different types
func (c *CachedRunner) GetRemoteURL(remote string) (string, error) {
	// Check cache first with read lock
	c.remoteURLMu.RLock()
	entry, ok := c.remoteURLCache[remote]
	c.remoteURLMu.RUnlock()

	if ok {
		return entry.url, entry.err
	}

	// Cache miss - use write lock for fetch + store to prevent multiple calls
	c.remoteURLMu.Lock()
	defer c.remoteURLMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have populated)
	if entry, ok := c.remoteURLCache[remote]; ok {
		return entry.url, entry.err
	}

	// Fetch from delegate while holding write lock
	url, err := c.delegate.GetRemoteURL(remote)
	c.remoteURLCache[remote] = remoteURLCacheEntry{url: url, err: err}

	return url, err
}

// GetBranchRemote returns the tracking remote for the given branch.
// Results are cached per branch name.
//
//nolint:dupl // Similar pattern to GetRemoteURL but different types
func (c *CachedRunner) GetBranchRemote(branch string) (string, error) {
	// Check cache first with read lock
	c.branchRemoteMu.RLock()
	entry, ok := c.branchRemoteCache[branch]
	c.branchRemoteMu.RUnlock()

	if ok {
		return entry.remote, entry.err
	}

	// Cache miss - use write lock for fetch + store to prevent multiple calls
	c.branchRemoteMu.Lock()
	defer c.branchRemoteMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have populated)
	if entry, ok := c.branchRemoteCache[branch]; ok {
		return entry.remote, entry.err
	}

	// Fetch from delegate while holding write lock
	rem, err := c.delegate.GetBranchRemote(branch)
	c.branchRemoteCache[branch] = branchRemoteCacheEntry{remote: rem, err: err}

	return rem, err
}

// GetRemotes returns the list of all remotes with their URLs.
// Result is cached.
func (c *CachedRunner) GetRemotes() (map[string]string, error) {
	c.remotesOnce.Do(func() {
		c.remotes, c.remotesErr = c.delegate.GetRemotes()
	})

	return c.remotes, c.remotesErr
}

// Ensure CachedRunner implements Runner.
var _ Runner = (*CachedRunner)(nil)
