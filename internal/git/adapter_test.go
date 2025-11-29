package git_test

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalgit "github.com/smykla-labs/klaudiush/internal/git"
	gitvalidators "github.com/smykla-labs/klaudiush/internal/validators/git"
)

var _ = Describe("RepositoryAdapter", func() {
	var (
		mockRepo *mockRepository
		adapter  gitvalidators.GitRunner
	)

	BeforeEach(func() {
		mockRepo = &mockRepository{}
		adapter = internalgit.NewRepositoryAdapter(mockRepo)
	})

	Describe("IsInRepo", func() {
		It("should delegate to repository", func() {
			mockRepo.isInRepoResult = true
			Expect(adapter.IsInRepo()).To(BeTrue())
			Expect(mockRepo.isInRepoCalled).To(BeTrue())
		})
	})

	Describe("GetStagedFiles", func() {
		It("should delegate to repository", func() {
			mockRepo.stagedFiles = []string{"file1.txt", "file2.txt"}
			files, err := adapter.GetStagedFiles()
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(Equal([]string{"file1.txt", "file2.txt"}))
			Expect(mockRepo.getStagedFilesCalled).To(BeTrue())
		})

		It("should return error from repository", func() {
			mockRepo.stagedFilesErr = internalgit.ErrNotRepository
			_, err := adapter.GetStagedFiles()
			Expect(err).To(MatchError(internalgit.ErrNotRepository))
		})
	})

	Describe("GetModifiedFiles", func() {
		It("should delegate to repository", func() {
			mockRepo.modifiedFiles = []string{"modified.txt"}
			files, err := adapter.GetModifiedFiles()
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(Equal([]string{"modified.txt"}))
			Expect(mockRepo.getModifiedFilesCalled).To(BeTrue())
		})
	})

	Describe("GetUntrackedFiles", func() {
		It("should delegate to repository", func() {
			mockRepo.untrackedFiles = []string{"untracked.txt"}
			files, err := adapter.GetUntrackedFiles()
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(Equal([]string{"untracked.txt"}))
			Expect(mockRepo.getUntrackedFilesCalled).To(BeTrue())
		})
	})

	Describe("GetRepoRoot", func() {
		It("should delegate to repository.GetRoot", func() {
			mockRepo.root = "/path/to/repo"
			root, err := adapter.GetRepoRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal("/path/to/repo"))
			Expect(mockRepo.getRootCalled).To(BeTrue())
		})
	})

	Describe("GetRemoteURL", func() {
		It("should delegate to repository", func() {
			mockRepo.remoteURLs = map[string]string{
				"origin": "https://github.com/test/repo.git",
			}
			url, err := adapter.GetRemoteURL("origin")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://github.com/test/repo.git"))
			Expect(mockRepo.getRemoteURLCalled).To(BeTrue())
		})
	})

	Describe("GetCurrentBranch", func() {
		It("should delegate to repository", func() {
			mockRepo.currentBranch = "main"
			branch, err := adapter.GetCurrentBranch()
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("main"))
			Expect(mockRepo.getCurrentBranchCalled).To(BeTrue())
		})
	})

	Describe("GetBranchRemote", func() {
		It("should delegate to repository", func() {
			mockRepo.branchRemotes = map[string]string{
				"main": "origin",
			}
			remote, err := adapter.GetBranchRemote("main")
			Expect(err).NotTo(HaveOccurred())
			Expect(remote).To(Equal("origin"))
			Expect(mockRepo.getBranchRemoteCalled).To(BeTrue())
		})
	})

	Describe("GetRemotes", func() {
		It("should delegate to repository", func() {
			mockRepo.remotes = map[string]string{
				"origin":   "https://github.com/test/repo.git",
				"upstream": "https://github.com/upstream/repo.git",
			}
			remotes, err := adapter.GetRemotes()
			Expect(err).NotTo(HaveOccurred())
			Expect(remotes).To(Equal(mockRepo.remotes))
			Expect(mockRepo.getRemotesCalled).To(BeTrue())
		})
	})
})

// mockRepository is a mock implementation of the Repository interface for testing
type mockRepository struct {
	// IsInRepo
	isInRepoResult bool
	isInRepoCalled bool

	// GetRoot
	root          string
	rootErr       error
	getRootCalled bool

	// GetStagedFiles
	stagedFiles          []string
	stagedFilesErr       error
	getStagedFilesCalled bool

	// GetModifiedFiles
	modifiedFiles          []string
	modifiedFilesErr       error
	getModifiedFilesCalled bool

	// GetUntrackedFiles
	untrackedFiles          []string
	untrackedFilesErr       error
	getUntrackedFilesCalled bool

	// GetCurrentBranch
	currentBranch          string
	currentBranchErr       error
	getCurrentBranchCalled bool

	// GetBranchRemote
	branchRemotes         map[string]string
	branchRemoteErr       error
	getBranchRemoteCalled bool
	lastBranchRemoteArg   string

	// GetRemoteURL
	remoteURLs         map[string]string
	remoteURLErr       error
	getRemoteURLCalled bool
	lastRemoteURLArg   string

	// GetRemotes
	remotes          map[string]string
	remotesErr       error
	getRemotesCalled bool
}

func (m *mockRepository) IsInRepo() bool {
	m.isInRepoCalled = true
	return m.isInRepoResult
}

func (m *mockRepository) GetRoot() (string, error) {
	m.getRootCalled = true
	return m.root, m.rootErr
}

func (m *mockRepository) GetStagedFiles() ([]string, error) {
	m.getStagedFilesCalled = true
	return m.stagedFiles, m.stagedFilesErr
}

func (m *mockRepository) GetModifiedFiles() ([]string, error) {
	m.getModifiedFilesCalled = true
	return m.modifiedFiles, m.modifiedFilesErr
}

func (m *mockRepository) GetUntrackedFiles() ([]string, error) {
	m.getUntrackedFilesCalled = true
	return m.untrackedFiles, m.untrackedFilesErr
}

func (m *mockRepository) GetCurrentBranch() (string, error) {
	m.getCurrentBranchCalled = true
	return m.currentBranch, m.currentBranchErr
}

func (m *mockRepository) GetBranchRemote(branch string) (string, error) {
	m.getBranchRemoteCalled = true
	m.lastBranchRemoteArg = branch

	if m.branchRemoteErr != nil {
		return "", m.branchRemoteErr
	}

	return m.branchRemotes[branch], nil
}

func (m *mockRepository) GetRemoteURL(remote string) (string, error) {
	m.getRemoteURLCalled = true
	m.lastRemoteURLArg = remote

	if m.remoteURLErr != nil {
		return "", m.remoteURLErr
	}

	return m.remoteURLs[remote], nil
}

func (m *mockRepository) GetRemotes() (map[string]string, error) {
	m.getRemotesCalled = true
	return m.remotes, m.remotesErr
}

var _ = Describe("NewSDKRunnerForPath", func() {
	var (
		tempDir string
		err     error
	)

	BeforeEach(func() {
		// Create temporary directory
		tempDir, err = os.MkdirTemp("", "sdk-runner-path-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Resolve symlinks (macOS /var -> /private/var)
		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
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

		It("should create a runner for the repository", func() {
			runner, err := internalgit.NewSDKRunnerForPath(tempDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(runner).NotTo(BeNil())
			Expect(runner.IsInRepo()).To(BeTrue())
		})

		It("should return correct repo root", func() {
			runner, err := internalgit.NewSDKRunnerForPath(tempDir)
			Expect(err).NotTo(HaveOccurred())

			root, err := runner.GetRepoRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal(tempDir))
		})
	})

	Context("when path is not a git repository", func() {
		It("should return ErrNotRepository", func() {
			_, err := internalgit.NewSDKRunnerForPath(tempDir)
			Expect(err).To(MatchError(internalgit.ErrNotRepository))
		})
	})

	Context("when path does not exist", func() {
		It("should return an error", func() {
			nonExistentPath := filepath.Join(tempDir, "does-not-exist")
			_, err := internalgit.NewSDKRunnerForPath(nonExistentPath)
			Expect(err).To(HaveOccurred())
		})
	})
})
