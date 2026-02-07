package git_test

import (
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing/object"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validators/git"
)

// gitEnvVars lists environment variables that can interfere with git command isolation.
// These must be cleared during tests to ensure git commands operate on the test
// repository, not the parent repository (especially important in worktrees).
var gitEnvVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_COMMON_DIR",
	"GIT_CONFIG",
	"GIT_CONFIG_GLOBAL",
	"GIT_CONFIG_SYSTEM",
	"GIT_INDEX_FILE",
}

var _ = Describe("CLIGitRunnerWithPath", func() {
	var (
		tempDir      string
		repo         *gogit.Repository
		runner       *git.CLIGitRunnerWithPath
		err          error
		savedGitEnvs map[string]string
		testAuthor   = &object.Signature{
			Name:  "Test User",
			Email: "test@klaudiu.sh",
		}
	)

	BeforeEach(func() {
		// Save and clear git environment variables to ensure test isolation.
		// This is critical when running from a worktree or when git env vars are set.
		savedGitEnvs = make(map[string]string)

		for _, envVar := range gitEnvVars {
			if val, exists := os.LookupEnv(envVar); exists {
				savedGitEnvs[envVar] = val
				os.Unsetenv(envVar)
			}
		}

		// Create temporary directory
		tempDir, err = os.MkdirTemp("", "cli-runner-path-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Resolve symlinks (macOS /var -> /private/var)
		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())

		// Initialize git repository
		repo, err = gogit.PlainInit(tempDir, false)
		Expect(err).NotTo(HaveOccurred())

		// Configure repository
		cfg, err := repo.Config()
		Expect(err).NotTo(HaveOccurred())
		cfg.User.Name = testAuthor.Name
		cfg.User.Email = testAuthor.Email
		err = repo.SetConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		// Set GIT_DIR and GIT_WORK_TREE explicitly to ensure complete isolation
		// when running from a worktree. The -C flag alone is insufficient because
		// git still discovers the parent worktree through process environment.
		os.Setenv("GIT_DIR", filepath.Join(tempDir, ".git"))
		os.Setenv("GIT_WORK_TREE", tempDir)

		// Create runner for the temp directory
		runner = git.NewCLIGitRunnerForPath(tempDir)
	})

	AfterEach(func() {
		// Clean up temp directory
		if tempDir != "" {
			removeErr := os.RemoveAll(tempDir)
			Expect(removeErr).NotTo(HaveOccurred())
		}

		// Restore git environment variables
		for envVar, val := range savedGitEnvs {
			os.Setenv(envVar, val)
		}
	})

	Describe("IsInRepo", func() {
		It("should return true when path is in a git repository", func() {
			Expect(runner.IsInRepo()).To(BeTrue())
		})

		It("should return false when path is not in a git repository", func() {
			nonRepoDir, err := os.MkdirTemp("", "non-repo-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(nonRepoDir)

			// Clear GIT_DIR/GIT_WORK_TREE so the non-repo check isn't affected
			// by the isolation env vars set in BeforeEach
			os.Unsetenv("GIT_DIR")
			os.Unsetenv("GIT_WORK_TREE")

			nonRepoRunner := git.NewCLIGitRunnerForPath(nonRepoDir)
			Expect(nonRepoRunner.IsInRepo()).To(BeFalse())
		})
	})

	Describe("GetRepoRoot", func() {
		It("should return the repository root", func() {
			root, err := runner.GetRepoRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal(tempDir))
		})

		It("should work from a subdirectory", func() {
			subDir := filepath.Join(tempDir, "subdir")
			err := os.MkdirAll(subDir, 0o755)
			Expect(err).NotTo(HaveOccurred())

			subRunner := git.NewCLIGitRunnerForPath(subDir)
			root, err := subRunner.GetRepoRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal(tempDir))
		})
	})

	Describe("GetStagedFiles", func() {
		Context("when no files are staged", func() {
			It("should return empty list", func() {
				files, err := runner.GetStagedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(BeEmpty())
			})
		})

		Context("when files are staged", func() {
			BeforeEach(func() {
				testFile := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(testFile, []byte("test content"), 0o644)
				Expect(err).NotTo(HaveOccurred())

				worktree, err := repo.Worktree()
				Expect(err).NotTo(HaveOccurred())

				_, err = worktree.Add("test.txt")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the staged files", func() {
				files, err := runner.GetStagedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(ConsistOf("test.txt"))
			})
		})
	})

	Describe("GetModifiedFiles", func() {
		BeforeEach(func() {
			// Create initial commit
			testFile := filepath.Join(tempDir, "existing.txt")
			err := os.WriteFile(testFile, []byte("original"), 0o644)
			Expect(err).NotTo(HaveOccurred())

			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Add("existing.txt")
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Commit("Initial commit", &gogit.CommitOptions{
				Author: testAuthor,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when no files are modified", func() {
			It("should return empty list", func() {
				files, err := runner.GetModifiedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(BeEmpty())
			})
		})

		Context("when files are modified", func() {
			BeforeEach(func() {
				testFile := filepath.Join(tempDir, "existing.txt")
				err := os.WriteFile(testFile, []byte("modified"), 0o644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the modified files", func() {
				files, err := runner.GetModifiedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(ConsistOf("existing.txt"))
			})
		})
	})

	Describe("GetUntrackedFiles", func() {
		Context("when no untracked files exist", func() {
			It("should return empty list", func() {
				files, err := runner.GetUntrackedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(BeEmpty())
			})
		})

		Context("when untracked files exist", func() {
			BeforeEach(func() {
				testFile := filepath.Join(tempDir, "untracked.txt")
				err := os.WriteFile(testFile, []byte("untracked"), 0o644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the untracked files", func() {
				files, err := runner.GetUntrackedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(ConsistOf("untracked.txt"))
			})
		})
	})

	Describe("GetCurrentBranch", func() {
		BeforeEach(func() {
			// Create initial commit to establish HEAD
			testFile := filepath.Join(tempDir, "initial.txt")
			err := os.WriteFile(testFile, []byte("initial"), 0o644)
			Expect(err).NotTo(HaveOccurred())

			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Add("initial.txt")
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Commit("Initial commit", &gogit.CommitOptions{
				Author: testAuthor,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return the current branch name", func() {
			branch, err := runner.GetCurrentBranch()
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("master"))
		})
	})

	Describe("GetRemoteURL", func() {
		Context("when remote exists", func() {
			BeforeEach(func() {
				_, err := repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{"https://github.com/test/repo.git"},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the remote URL", func() {
				url, err := runner.GetRemoteURL("origin")
				Expect(err).NotTo(HaveOccurred())
				Expect(url).To(Equal("https://github.com/test/repo.git"))
			})
		})

		Context("when remote does not exist", func() {
			It("should return an error", func() {
				_, err := runner.GetRemoteURL("nonexistent")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("GetBranchRemote", func() {
		BeforeEach(func() {
			// Create initial commit
			testFile := filepath.Join(tempDir, "initial.txt")
			err := os.WriteFile(testFile, []byte("initial"), 0o644)
			Expect(err).NotTo(HaveOccurred())

			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Add("initial.txt")
			Expect(err).NotTo(HaveOccurred())

			_, err = worktree.Commit("Initial commit", &gogit.CommitOptions{
				Author: testAuthor,
			})
			Expect(err).NotTo(HaveOccurred())

			// Add remote
			_, err = repo.CreateRemote(&config.RemoteConfig{
				Name: "origin",
				URLs: []string{"https://github.com/test/repo.git"},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when branch has tracking remote", func() {
			BeforeEach(func() {
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
				remote, err := runner.GetBranchRemote("master")
				Expect(err).NotTo(HaveOccurred())
				Expect(remote).To(Equal("origin"))
			})
		})

		Context("when branch has no tracking remote", func() {
			It("should return an error", func() {
				_, err := runner.GetBranchRemote("master")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("GetRemotes", func() {
		Context("when no remotes exist", func() {
			It("should return empty map", func() {
				remotes, err := runner.GetRemotes()
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
				remotes, err := runner.GetRemotes()
				Expect(err).NotTo(HaveOccurred())
				Expect(remotes).To(HaveLen(2))
				Expect(remotes["origin"]).To(Equal("https://github.com/test/repo.git"))
				Expect(remotes["upstream"]).To(Equal("https://github.com/upstream/repo.git"))
			})
		})
	})
})

var _ = Describe("NewGitRunnerForPath", func() {
	var (
		tempDir      string
		err          error
		savedGitEnvs map[string]string
	)

	BeforeEach(func() {
		// Save and clear git environment variables to ensure test isolation.
		savedGitEnvs = make(map[string]string)

		for _, envVar := range gitEnvVars {
			if val, exists := os.LookupEnv(envVar); exists {
				savedGitEnvs[envVar] = val
				os.Unsetenv(envVar)
			}
		}

		// Create temporary directory
		tempDir, err = os.MkdirTemp("", "git-runner-path-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Resolve symlinks (macOS /var -> /private/var)
		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())

		// Initialize git repository
		_, err = gogit.PlainInit(tempDir, false)
		Expect(err).NotTo(HaveOccurred())

		// Set GIT_DIR and GIT_WORK_TREE explicitly to ensure complete isolation
		// when running from a worktree. The -C flag alone is insufficient because
		// git still discovers the parent worktree through process environment.
		os.Setenv("GIT_DIR", filepath.Join(tempDir, ".git"))
		os.Setenv("GIT_WORK_TREE", tempDir)
	})

	AfterEach(func() {
		// Clean up temp directory
		if tempDir != "" {
			err := os.RemoveAll(tempDir) //nolint:govet // shadow for cleanup scope
			Expect(err).NotTo(HaveOccurred())
		}

		// Reset env var
		os.Unsetenv("KLAUDIUSH_USE_SDK_GIT")

		// Restore git environment variables
		for envVar, val := range savedGitEnvs {
			os.Setenv(envVar, val)
		}
	})

	It("should return a runner that works for the specified path", func() {
		runner := git.NewGitRunnerForPath(tempDir)
		Expect(runner).NotTo(BeNil())
		Expect(runner.IsInRepo()).To(BeTrue())

		root, rootErr := runner.GetRepoRoot()
		Expect(rootErr).NotTo(HaveOccurred())
		Expect(root).To(Equal(tempDir))
	})

	Context("when SDK is explicitly disabled", func() {
		BeforeEach(func() {
			os.Setenv("KLAUDIUSH_USE_SDK_GIT", "false")
		})

		It("should return a CLI runner", func() {
			runner := git.NewGitRunnerForPath(tempDir)
			Expect(runner).NotTo(BeNil())
			Expect(runner.IsInRepo()).To(BeTrue())

			// Verify it's working correctly
			root, rootErr := runner.GetRepoRoot()
			Expect(rootErr).NotTo(HaveOccurred())
			Expect(root).To(Equal(tempDir))
		})
	})

	Context("when SDK is disabled with 0", func() {
		BeforeEach(func() {
			os.Setenv("KLAUDIUSH_USE_SDK_GIT", "0")
		})

		It("should return a CLI runner", func() {
			runner := git.NewGitRunnerForPath(tempDir)
			Expect(runner).NotTo(BeNil())
			Expect(runner.IsInRepo()).To(BeTrue())
		})
	})

	Context("when path is not a git repository", func() {
		var nonRepoDir string

		BeforeEach(func() {
			// Clear GIT_DIR/GIT_WORK_TREE so the non-repo check isn't affected
			// by the isolation env vars set in parent BeforeEach
			os.Unsetenv("GIT_DIR")
			os.Unsetenv("GIT_WORK_TREE")

			nonRepoDir, err = os.MkdirTemp("", "non-repo-*")
			Expect(err).NotTo(HaveOccurred())

			nonRepoDir, err = filepath.EvalSymlinks(nonRepoDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if nonRepoDir != "" {
				os.RemoveAll(nonRepoDir)
			}
		})

		It("should fall back to CLI runner", func() {
			runner := git.NewGitRunnerForPath(nonRepoDir)
			Expect(runner).NotTo(BeNil())
			// CLI runner should return false for non-repo
			Expect(runner.IsInRepo()).To(BeFalse())
		})
	})
})
