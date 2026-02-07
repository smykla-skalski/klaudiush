package git_test

import (
	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/git"
)

var _ = Describe("CachedRunner", func() {
	var (
		ctrl       *gomock.Controller
		mockRunner *git.MockRunner
		cached     git.Runner
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockRunner = git.NewMockRunner(ctrl)
		cached = git.NewCachedRunner(mockRunner)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("IsInRepo", func() {
		It("caches the result after first call", func() {
			mockRunner.EXPECT().IsInRepo().Return(true).Times(1)

			// First call
			result := cached.IsInRepo()
			Expect(result).To(BeTrue())

			// Second call - should use cached value
			result = cached.IsInRepo()
			Expect(result).To(BeTrue())
		})

		It("caches false result", func() {
			mockRunner.EXPECT().IsInRepo().Return(false).Times(1)

			result := cached.IsInRepo()
			Expect(result).To(BeFalse())

			result = cached.IsInRepo()
			Expect(result).To(BeFalse())
		})
	})

	Describe("Status-derived operations", func() {
		Context("GetStagedFiles", func() {
			It("caches the result after first call", func() {
				mockRunner.EXPECT().
					GetStagedFiles().
					Return([]string{"file1.go", "file2.go"}, nil).
					Times(1)
				mockRunner.EXPECT().GetModifiedFiles().Return([]string{"file3.go"}, nil).Times(1)
				mockRunner.EXPECT().GetUntrackedFiles().Return([]string{}, nil).Times(1)

				// First call
				files, err := cached.GetStagedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(Equal([]string{"file1.go", "file2.go"}))

				// Second call - should use cached value
				files, err = cached.GetStagedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(Equal([]string{"file1.go", "file2.go"}))
			})

			It("returns error when delegate fails", func() {
				expectedErr := errors.New("git error")
				mockRunner.EXPECT().GetStagedFiles().Return(nil, expectedErr).Times(1)

				files, err := cached.GetStagedFiles()
				Expect(err).To(Equal(expectedErr))
				Expect(files).To(BeNil())
			})
		})

		Context("GetModifiedFiles", func() {
			It("caches the result after first call", func() {
				mockRunner.EXPECT().GetStagedFiles().Return([]string{}, nil).Times(1)
				mockRunner.EXPECT().GetModifiedFiles().Return([]string{"modified.go"}, nil).Times(1)
				mockRunner.EXPECT().GetUntrackedFiles().Return([]string{}, nil).Times(1)

				files, err := cached.GetModifiedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(Equal([]string{"modified.go"}))

				// Second call
				files, err = cached.GetModifiedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(Equal([]string{"modified.go"}))
			})
		})

		Context("GetUntrackedFiles", func() {
			It("caches the result after first call", func() {
				mockRunner.EXPECT().GetStagedFiles().Return([]string{}, nil).Times(1)
				mockRunner.EXPECT().GetModifiedFiles().Return([]string{}, nil).Times(1)
				mockRunner.EXPECT().GetUntrackedFiles().Return([]string{"new.txt"}, nil).Times(1)

				files, err := cached.GetUntrackedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(Equal([]string{"new.txt"}))

				// Second call
				files, err = cached.GetUntrackedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(files).To(Equal([]string{"new.txt"}))
			})
		})

		Context("Multiple status operations", func() {
			It("calls delegate once for all status-derived operations", func() {
				mockRunner.EXPECT().GetStagedFiles().Return([]string{"staged.go"}, nil).Times(1)
				mockRunner.EXPECT().GetModifiedFiles().Return([]string{"modified.go"}, nil).Times(1)
				mockRunner.EXPECT().
					GetUntrackedFiles().
					Return([]string{"untracked.txt"}, nil).
					Times(1)

				// Call all three methods
				staged, err := cached.GetStagedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(staged).To(Equal([]string{"staged.go"}))

				modified, err := cached.GetModifiedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(modified).To(Equal([]string{"modified.go"}))

				untracked, err := cached.GetUntrackedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(untracked).To(Equal([]string{"untracked.txt"}))

				// Call again - all should use cache
				staged, _ = cached.GetStagedFiles()
				modified, _ = cached.GetModifiedFiles()
				untracked, _ = cached.GetUntrackedFiles()

				Expect(staged).To(Equal([]string{"staged.go"}))
				Expect(modified).To(Equal([]string{"modified.go"}))
				Expect(untracked).To(Equal([]string{"untracked.txt"}))
			})

			It("stops on first error and returns it", func() {
				expectedErr := errors.New("staged files error")
				mockRunner.EXPECT().GetStagedFiles().Return(nil, expectedErr).Times(1)

				// Any status-derived operation should fail
				_, err := cached.GetStagedFiles()
				Expect(err).To(Equal(expectedErr))

				_, err = cached.GetModifiedFiles()
				Expect(err).To(Equal(expectedErr))
			})

			It("propagates error from GetModifiedFiles", func() {
				expectedErr := errors.New("modified files error")
				mockRunner.EXPECT().GetStagedFiles().Return([]string{}, nil).Times(1)
				mockRunner.EXPECT().GetModifiedFiles().Return(nil, expectedErr).Times(1)

				// GetStagedFiles succeeds but GetModifiedFiles fails
				staged, err := cached.GetStagedFiles()
				Expect(err).NotTo(HaveOccurred())
				Expect(staged).To(BeEmpty())

				_, err = cached.GetModifiedFiles()
				Expect(err).To(Equal(expectedErr))
			})
		})
	})

	Describe("GetRepoRoot", func() {
		It("caches the result after first call", func() {
			mockRunner.EXPECT().GetRepoRoot().Return("/path/to/repo", nil).Times(1)

			root, err := cached.GetRepoRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal("/path/to/repo"))

			// Second call
			root, err = cached.GetRepoRoot()
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(Equal("/path/to/repo"))
		})

		It("caches error result", func() {
			expectedErr := errors.New("not a repo")
			mockRunner.EXPECT().GetRepoRoot().Return("", expectedErr).Times(1)

			_, err := cached.GetRepoRoot()
			Expect(err).To(Equal(expectedErr))

			_, err = cached.GetRepoRoot()
			Expect(err).To(Equal(expectedErr))
		})
	})

	Describe("GetCurrentBranch", func() {
		It("caches the result after first call", func() {
			mockRunner.EXPECT().GetCurrentBranch().Return("main", nil).Times(1)

			branch, err := cached.GetCurrentBranch()
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("main"))

			// Second call
			branch, err = cached.GetCurrentBranch()
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("main"))
		})

		It("caches error result", func() {
			expectedErr := errors.New("detached HEAD")
			mockRunner.EXPECT().GetCurrentBranch().Return("", expectedErr).Times(1)

			_, err := cached.GetCurrentBranch()
			Expect(err).To(Equal(expectedErr))

			_, err = cached.GetCurrentBranch()
			Expect(err).To(Equal(expectedErr))
		})
	})

	Describe("GetRemoteURL", func() {
		It("caches results per remote name", func() {
			mockRunner.EXPECT().
				GetRemoteURL("origin").
				Return("git@github.com:user/repo.git", nil).
				Times(1)
			mockRunner.EXPECT().
				GetRemoteURL("upstream").
				Return("git@github.com:org/repo.git", nil).
				Times(1)

			// First calls
			url, err := cached.GetRemoteURL("origin")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("git@github.com:user/repo.git"))

			url, err = cached.GetRemoteURL("upstream")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("git@github.com:org/repo.git"))

			// Second calls - should use cache
			url, _ = cached.GetRemoteURL("origin")
			Expect(url).To(Equal("git@github.com:user/repo.git"))

			url, _ = cached.GetRemoteURL("upstream")
			Expect(url).To(Equal("git@github.com:org/repo.git"))
		})

		It("caches error result per remote", func() {
			expectedErr := errors.New("remote not found")
			mockRunner.EXPECT().GetRemoteURL("nonexistent").Return("", expectedErr).Times(1)
			mockRunner.EXPECT().
				GetRemoteURL("origin").
				Return("git@github.com:user/repo.git", nil).
				Times(1)

			_, err := cached.GetRemoteURL("nonexistent")
			Expect(err).To(Equal(expectedErr))

			// Second call - should use cached error
			_, err = cached.GetRemoteURL("nonexistent")
			Expect(err).To(Equal(expectedErr))

			// Different remote should still work
			url, err := cached.GetRemoteURL("origin")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("git@github.com:user/repo.git"))
		})
	})

	Describe("GetBranchRemote", func() {
		It("caches results per branch name", func() {
			mockRunner.EXPECT().GetBranchRemote("main").Return("origin", nil).Times(1)
			mockRunner.EXPECT().GetBranchRemote("feature").Return("upstream", nil).Times(1)

			// First calls
			remote, err := cached.GetBranchRemote("main")
			Expect(err).NotTo(HaveOccurred())
			Expect(remote).To(Equal("origin"))

			remote, err = cached.GetBranchRemote("feature")
			Expect(err).NotTo(HaveOccurred())
			Expect(remote).To(Equal("upstream"))

			// Second calls - should use cache
			remote, _ = cached.GetBranchRemote("main")
			Expect(remote).To(Equal("origin"))

			remote, _ = cached.GetBranchRemote("feature")
			Expect(remote).To(Equal("upstream"))
		})

		It("caches error result per branch", func() {
			expectedErr := errors.New("no tracking branch")
			mockRunner.EXPECT().GetBranchRemote("local-only").Return("", expectedErr).Times(1)

			_, err := cached.GetBranchRemote("local-only")
			Expect(err).To(Equal(expectedErr))

			// Second call - should use cached error
			_, err = cached.GetBranchRemote("local-only")
			Expect(err).To(Equal(expectedErr))
		})
	})

	Describe("GetRemotes", func() {
		It("caches the result after first call", func() {
			remotes := map[string]string{
				"origin":   "git@github.com:user/repo.git",
				"upstream": "git@github.com:org/repo.git",
			}
			mockRunner.EXPECT().GetRemotes().Return(remotes, nil).Times(1)

			result, err := cached.GetRemotes()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(remotes))

			// Second call
			result, err = cached.GetRemotes()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(remotes))
		})

		It("caches error result", func() {
			expectedErr := errors.New("git error")
			mockRunner.EXPECT().GetRemotes().Return(nil, expectedErr).Times(1)

			_, err := cached.GetRemotes()
			Expect(err).To(Equal(expectedErr))

			_, err = cached.GetRemotes()
			Expect(err).To(Equal(expectedErr))
		})
	})

	Describe("Concurrent access", func() {
		It("handles concurrent calls to IsInRepo", func() {
			mockRunner.EXPECT().IsInRepo().Return(true).Times(1)

			done := make(chan bool, 10)

			for range 10 {
				go func() {
					result := cached.IsInRepo()
					Expect(result).To(BeTrue())
					done <- true
				}()
			}

			for range 10 {
				<-done
			}
		})

		It("handles concurrent calls to status-derived operations", func() {
			mockRunner.EXPECT().GetStagedFiles().Return([]string{"staged.go"}, nil).Times(1)
			mockRunner.EXPECT().GetModifiedFiles().Return([]string{"modified.go"}, nil).Times(1)
			mockRunner.EXPECT().GetUntrackedFiles().Return([]string{"untracked.txt"}, nil).Times(1)

			done := make(chan bool, 30)

			for range 10 {
				go func() {
					files, err := cached.GetStagedFiles()
					Expect(err).NotTo(HaveOccurred())
					Expect(files).To(Equal([]string{"staged.go"}))
					done <- true
				}()

				go func() {
					files, err := cached.GetModifiedFiles()
					Expect(err).NotTo(HaveOccurred())
					Expect(files).To(Equal([]string{"modified.go"}))
					done <- true
				}()

				go func() {
					files, err := cached.GetUntrackedFiles()
					Expect(err).NotTo(HaveOccurred())
					Expect(files).To(Equal([]string{"untracked.txt"}))
					done <- true
				}()
			}

			for range 30 {
				<-done
			}
		})

		It("handles concurrent calls to GetRemoteURL with same remote", func() {
			mockRunner.EXPECT().
				GetRemoteURL("origin").
				Return("git@github.com:user/repo.git", nil).
				Times(1)

			done := make(chan bool, 10)

			for range 10 {
				go func() {
					url, err := cached.GetRemoteURL("origin")
					Expect(err).NotTo(HaveOccurred())
					Expect(url).To(Equal("git@github.com:user/repo.git"))
					done <- true
				}()
			}

			for range 10 {
				<-done
			}
		})
	})
})
