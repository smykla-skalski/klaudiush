package git_test

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalgit "github.com/smykla-labs/klaudiush/internal/git"
)

var _ = Describe("OpenRepository", func() {
	var (
		tempDir string
		origDir string
		err     error
	)

	BeforeEach(func() {
		// Save current directory
		origDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		// Create temporary directory
		tempDir, err = os.MkdirTemp("", "open-repo-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Resolve symlinks (macOS /var -> /private/var)
		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Restore original directory
		if origDir != "" {
			err := os.Chdir(origDir) //nolint:govet // shadow for cleanup scope
			Expect(err).NotTo(HaveOccurred())
		}

		// Clean up temp directory
		if tempDir != "" {
			err := os.RemoveAll(tempDir) //nolint:govet // shadow for cleanup scope
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("when path is a valid git repository", func() {
		BeforeEach(func() {
			// Initialize git repository
			_, err = git.PlainInit(tempDir, false)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should open the repository", func() {
			repo, err := internalgit.OpenRepository(tempDir) //nolint:govet // shadow
			Expect(err).NotTo(HaveOccurred())
			Expect(repo).NotTo(BeNil())
			Expect(repo.IsInRepo()).To(BeTrue())
		})

		It("should return a working repository", func() {
			repo, err := internalgit.OpenRepository(tempDir) //nolint:govet // shadow
			Expect(err).NotTo(HaveOccurred())

			root, err := repo.GetRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal(tempDir))
		})
	})

	Context("when path is not a git repository", func() {
		It("should return ErrNotRepository", func() {
			_, err := internalgit.OpenRepository(tempDir) //nolint:govet // shadow
			Expect(err).To(MatchError(internalgit.ErrNotRepository))
		})
	})

	Context("when path does not exist", func() {
		It("should return an error", func() {
			nonExistentPath := filepath.Join(tempDir, "does-not-exist")
			_, err := internalgit.OpenRepository(nonExistentPath) //nolint:govet // shadow
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when opening a subdirectory of a git repository", func() {
		BeforeEach(func() {
			// Initialize git repository
			_, err = git.PlainInit(tempDir, false)
			Expect(err).NotTo(HaveOccurred())

			// Create a subdirectory
			subDir := filepath.Join(tempDir, "subdir")
			err = os.MkdirAll(subDir, 0o755)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should discover the parent repository", func() {
			subDir := filepath.Join(tempDir, "subdir")
			repo, err := internalgit.OpenRepository(subDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(repo).NotTo(BeNil())

			root, err := repo.GetRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal(tempDir))
		})
	})
})

var _ = Describe("SDKRepository", func() {
	var (
		tempDir    string
		repo       *git.Repository
		sdkRepo    *internalgit.SDKRepository
		err        error
		origDir    string
		testAuthor = &object.Signature{
			Name:  "Test User",
			Email: "test@klaudiu.sh",
		}
	)

	BeforeEach(func() {
		// Reset repository cache
		internalgit.ResetRepositoryCache()

		// Save current directory
		origDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		// Create temporary directory
		tempDir, err = os.MkdirTemp("", "git-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Resolve symlinks (macOS /var -> /private/var)
		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())

		// Change to temp directory
		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())

		// Initialize git repository
		repo, err = git.PlainInit(tempDir, false)
		Expect(err).NotTo(HaveOccurred())

		// Configure repository
		cfg, err := repo.Config() //nolint:govet // shadow
		Expect(err).NotTo(HaveOccurred())
		cfg.User.Name = testAuthor.Name
		cfg.User.Email = testAuthor.Email
		err = repo.SetConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Restore original directory
		if origDir != "" {
			err := os.Chdir(origDir) //nolint:govet // shadow for cleanup scope
			Expect(err).NotTo(HaveOccurred())
		}

		// Clean up temp directory
		if tempDir != "" {
			err := os.RemoveAll(tempDir) //nolint:govet // shadow for cleanup scope
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("DiscoverRepository", func() {
		Context("when in a git repository", func() {
			It("should discover the repository", func() {
				sdkRepo, err = internalgit.DiscoverRepository()
				Expect(err).NotTo(HaveOccurred())
				Expect(sdkRepo).NotTo(BeNil())
			})

			It("should return the same instance on multiple calls", func() {
				repo1, err1 := internalgit.DiscoverRepository()
				Expect(err1).NotTo(HaveOccurred())

				repo2, err2 := internalgit.DiscoverRepository()
				Expect(err2).NotTo(HaveOccurred())

				Expect(repo1).To(BeIdenticalTo(repo2))
			})
		})

		Context("when not in a git repository", func() {
			BeforeEach(func() {
				// Remove .git directory
				gitDir := filepath.Join(tempDir, ".git")
				err := os.RemoveAll(gitDir) //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return ErrNotRepository", func() {
				_, err := internalgit.DiscoverRepository() //nolint:govet // shadow
				Expect(err).To(MatchError(internalgit.ErrNotRepository))
			})
		})
	})

	Describe("IsInRepo", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return true when in a repository", func() {
			Expect(sdkRepo.IsInRepo()).To(BeTrue())
		})
	})

	Describe("GetRoot", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return the repository root directory", func() {
			root, err := sdkRepo.GetRoot() //nolint:govet // shadow
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal(tempDir))
		})
	})

	Describe("GetStagedFiles", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when no files are staged", func() {
			It("should return empty list", func() {
				files, err := sdkRepo.GetStagedFiles() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(BeEmpty())
			})
		})

		Context("when files are staged", func() {
			BeforeEach(func() {
				// Create and stage a file
				testFile := filepath.Join(tempDir, "test.txt")
				content := []byte("test content")
				err := os.WriteFile(testFile, content, 0o644) //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())

				worktree, err := repo.Worktree()
				Expect(err).NotTo(HaveOccurred())

				_, err = worktree.Add("test.txt")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the staged files", func() {
				files, err := sdkRepo.GetStagedFiles() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(ConsistOf("test.txt"))
			})
		})

		Context("when multiple files are staged", func() {
			BeforeEach(func() {
				worktree, err := repo.Worktree() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())

				for _, filename := range []string{"file1.txt", "file2.txt", "file3.txt"} {
					testFile := filepath.Join(tempDir, filename)
					err := os.WriteFile(testFile, []byte("content"), 0o644)
					Expect(err).NotTo(HaveOccurred())

					_, err = worktree.Add(filename)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should return all staged files", func() {
				files, err := sdkRepo.GetStagedFiles() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(ConsistOf("file1.txt", "file2.txt", "file3.txt"))
			})
		})
	})

	Describe("GetModifiedFiles", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())

			// Create initial commit
			testFile := filepath.Join(tempDir, "existing.txt")
			err := os.WriteFile(testFile, []byte("original"), 0o644) //nolint:govet // shadow
			Expect(err).NotTo(HaveOccurred())

			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Add("existing.txt")
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Commit("Initial commit", &git.CommitOptions{
				Author: testAuthor,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when no files are modified", func() {
			It("should return empty list", func() {
				files, err := sdkRepo.GetModifiedFiles() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(BeEmpty())
			})
		})

		Context("when files are modified but not staged", func() {
			BeforeEach(func() {
				testFile := filepath.Join(tempDir, "existing.txt")
				content := []byte("modified content")
				err := os.WriteFile(testFile, content, 0o644) //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the modified files", func() {
				files, err := sdkRepo.GetModifiedFiles() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(ConsistOf("existing.txt"))
			})
		})
	})

	Describe("GetUntrackedFiles", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when no untracked files exist", func() {
			It("should return empty list", func() {
				files, err := sdkRepo.GetUntrackedFiles() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(BeEmpty())
			})
		})

		Context("when untracked files exist", func() {
			BeforeEach(func() {
				testFile := filepath.Join(tempDir, "untracked.txt")
				err := os.WriteFile(testFile, []byte("untracked"), 0o644) //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the untracked files", func() {
				files, err := sdkRepo.GetUntrackedFiles() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(ConsistOf("untracked.txt"))
			})
		})
	})

	Describe("GetCurrentBranch", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())

			// Create initial commit to establish HEAD
			testFile := filepath.Join(tempDir, "initial.txt")
			err := os.WriteFile(testFile, []byte("initial"), 0o644) //nolint:govet // shadow
			Expect(err).NotTo(HaveOccurred())

			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Add("initial.txt")
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Commit("Initial commit", &git.CommitOptions{
				Author: testAuthor,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return the current branch name", func() {
			branch, err := sdkRepo.GetCurrentBranch() //nolint:govet // shadow
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("master"))
		})

		Context("when on a different branch", func() {
			BeforeEach(func() {
				worktree, err := repo.Worktree() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())

				err = worktree.Checkout(&git.CheckoutOptions{
					Branch: plumbing.NewBranchReferenceName("feature"),
					Create: true,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the feature branch name", func() {
				branch, err := sdkRepo.GetCurrentBranch() //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(branch).To(Equal("feature"))
			})
		})
	})

	Describe("GetBranchRemote", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())

			// Create initial commit
			testFile := filepath.Join(tempDir, "initial.txt")
			err := os.WriteFile(testFile, []byte("initial"), 0o644) //nolint:govet // shadow
			Expect(err).NotTo(HaveOccurred())

			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Add("initial.txt")
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Commit("Initial commit", &git.CommitOptions{
				Author: testAuthor,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when branch has tracking remote", func() {
			BeforeEach(func() {
				// Add remote
				_, err := repo.CreateRemote(&config.RemoteConfig{ //nolint:govet // shadow
					Name: "origin",
					URLs: []string{"https://github.com/test/repo.git"},
				})
				Expect(err).NotTo(HaveOccurred())

				// Configure branch to track remote
				cfg, err := repo.Config()
				Expect(err).NotTo(HaveOccurred())

				cfg.Branches["master"] = &config.Branch{
					Name:   "master",
					Remote: "origin",
					Merge:  "refs/heads/master",
				}

				err = repo.SetConfig(cfg)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the tracking remote", func() {
				remote, err := sdkRepo.GetBranchRemote("master") //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(remote).To(Equal("origin"))
			})
		})

		Context("when branch does not exist", func() {
			It("should return ErrBranchNotFound", func() {
				_, err := sdkRepo.GetBranchRemote("nonexistent") //nolint:govet // shadow
				Expect(err).To(MatchError(ContainSubstring("branch not found")))
			})
		})

		Context("when branch has no tracking remote", func() {
			It("should return ErrNoTracking", func() {
				_, err := sdkRepo.GetBranchRemote("master") //nolint:govet // shadow
				Expect(err).To(MatchError(ContainSubstring("no tracking remote")))
			})
		})
	})

	Describe("GetRemoteURL", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when remote exists", func() {
			BeforeEach(func() {
				_, err := repo.CreateRemote(&config.RemoteConfig{ //nolint:govet // shadow
					Name: "origin",
					URLs: []string{"https://github.com/test/repo.git"},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the remote URL", func() {
				url, err := sdkRepo.GetRemoteURL("origin") //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(url).To(Equal("https://github.com/test/repo.git"))
			})
		})

		Context("when remote does not exist", func() {
			It("should return ErrRemoteNotFound", func() {
				_, err := sdkRepo.GetRemoteURL("nonexistent") //nolint:govet // shadow
				Expect(err).To(MatchError(ContainSubstring("remote not found")))
			})
		})
	})

	Describe("GetRemotes", func() {
		BeforeEach(func() {
			sdkRepo, err = internalgit.DiscoverRepository()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when no remotes exist", func() {
			It("should return empty map", func() {
				remotes, err := sdkRepo.GetRemotes()
				Expect(err).NotTo(HaveOccurred())
				Expect(remotes).To(BeEmpty())
			})
		})

		Context("when multiple remotes exist", func() {
			BeforeEach(func() {
				_, err := repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{"https://github.com/test/repo.git"},
				})
				Expect(err).NotTo(HaveOccurred())

				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "upstream",
					URLs: []string{"https://github.com/upstream/repo.git"},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return all remotes with their URLs", func() {
				remotes, err := sdkRepo.GetRemotes()
				Expect(err).NotTo(HaveOccurred())
				Expect(remotes).To(HaveLen(2))
				Expect(remotes["origin"]).To(Equal("https://github.com/test/repo.git"))
				Expect(remotes["upstream"]).To(Equal("https://github.com/upstream/repo.git"))
			})
		})
	})
})

var _ = Describe("DiscoverRepository with linked worktrees", func() {
	var (
		mainRepoDir string
		worktreeDir string
		origDir     string
		repo        *git.Repository
		err         error
		testAuthor  = &object.Signature{
			Name:  "Test User",
			Email: "test@klaudiu.sh",
		}
	)

	BeforeEach(func() {
		// Reset repository cache
		internalgit.ResetRepositoryCache()

		// Save current directory
		origDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		// Create main repo directory
		mainRepoDir, err = os.MkdirTemp("", "main-repo-*")
		Expect(err).NotTo(HaveOccurred())
		mainRepoDir, err = filepath.EvalSymlinks(mainRepoDir)
		Expect(err).NotTo(HaveOccurred())

		// Create worktree directory
		worktreeDir, err = os.MkdirTemp("", "worktree-*")
		Expect(err).NotTo(HaveOccurred())
		worktreeDir, err = filepath.EvalSymlinks(worktreeDir)
		Expect(err).NotTo(HaveOccurred())

		// Remove worktree dir (git worktree add expects it to not exist)
		err = os.RemoveAll(worktreeDir)
		Expect(err).NotTo(HaveOccurred())

		// Initialize main git repository
		repo, err = git.PlainInit(mainRepoDir, false)
		Expect(err).NotTo(HaveOccurred())

		// Configure repository
		cfg, cfgErr := repo.Config()
		Expect(cfgErr).NotTo(HaveOccurred())
		cfg.User.Name = testAuthor.Name
		cfg.User.Email = testAuthor.Email
		err = repo.SetConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		// Create initial commit to establish HEAD
		testFile := filepath.Join(mainRepoDir, "initial.txt")
		err = os.WriteFile(testFile, []byte("initial"), 0o644)
		Expect(err).NotTo(HaveOccurred())

		worktree, wtErr := repo.Worktree()
		Expect(wtErr).NotTo(HaveOccurred())

		_, err = worktree.Add("initial.txt")
		Expect(err).NotTo(HaveOccurred())

		_, err = worktree.Commit("Initial commit", &git.CommitOptions{
			Author: testAuthor,
		})
		Expect(err).NotTo(HaveOccurred())

		// Add remotes to main repo
		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{"https://github.com/fork/repo.git"},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: "upstream",
			URLs: []string{"https://github.com/upstream/repo.git"},
		})
		Expect(err).NotTo(HaveOccurred())

		// Create worktree using git CLI (go-git doesn't support worktree add)
		// First, create a branch for the worktree
		headRef, headErr := repo.Head()
		Expect(headErr).NotTo(HaveOccurred())

		err = repo.Storer.SetReference(plumbing.NewHashReference(
			plumbing.NewBranchReferenceName("feature-branch"),
			headRef.Hash(),
		))
		Expect(err).NotTo(HaveOccurred())

		// Use git CLI to create worktree
		gitDir := filepath.Join(mainRepoDir, ".git")
		args := []string{"--git-dir=" + gitDir, "worktree", "add", worktreeDir, "feature-branch"}
		gitCmd := exec.Command("git", args...)
		output, cmdErr := gitCmd.CombinedOutput()
		if cmdErr != nil {
			// If git worktree command fails, skip test
			Skip("git worktree command not available: " + string(output))
		}
	})

	AfterEach(func() {
		// Restore original directory
		if origDir != "" {
			_ = os.Chdir(origDir)
		}

		// Clean up worktree first (must be done before removing main repo)
		if worktreeDir != "" {
			_ = os.RemoveAll(worktreeDir)
		}

		// Clean up main repo directory
		if mainRepoDir != "" {
			_ = os.RemoveAll(mainRepoDir)
		}
	})

	It("should discover remotes from linked worktree", func() {
		// Change to worktree directory
		err = os.Chdir(worktreeDir)
		Expect(err).NotTo(HaveOccurred())

		// Discover repository from worktree
		sdkRepo, discoverErr := internalgit.DiscoverRepository()
		Expect(discoverErr).NotTo(HaveOccurred())
		Expect(sdkRepo).NotTo(BeNil())

		// Verify we can find remotes
		remotes, remotesErr := sdkRepo.GetRemotes()
		Expect(remotesErr).NotTo(HaveOccurred())
		Expect(remotes).To(HaveLen(2))
		Expect(remotes["origin"]).To(Equal("https://github.com/fork/repo.git"))
		Expect(remotes["upstream"]).To(Equal("https://github.com/upstream/repo.git"))
	})

	It("should find specific remote URL from linked worktree", func() {
		// Change to worktree directory
		err = os.Chdir(worktreeDir)
		Expect(err).NotTo(HaveOccurred())

		// Discover repository from worktree
		sdkRepo, discoverErr := internalgit.DiscoverRepository()
		Expect(discoverErr).NotTo(HaveOccurred())

		// Verify we can find the upstream remote
		url, urlErr := sdkRepo.GetRemoteURL("upstream")
		Expect(urlErr).NotTo(HaveOccurred())
		Expect(url).To(Equal("https://github.com/upstream/repo.git"))
	})
})
