package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-labs/klaudiush/internal/git"
	validatorpkg "github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("FetchValidator", func() {
	var (
		validator *git.FetchValidator
		fakeGit   *gitpkg.FakeRunner
		log       logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		fakeGit = gitpkg.NewFakeRunner()
		fakeGit.InRepo = true
		fakeGit.RepoRoot = "/home/user/projects/github.com/user/my-project"
		validator = git.NewFetchValidator(log, fakeGit, nil, nil)
	})

	// Helper function to create context with command
	createContext := func(command string) *hook.Context {
		return &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{
				Command: command,
			},
		}
	}

	Describe("Name", func() {
		It("returns the validator name", func() {
			Expect(validator.Name()).To(Equal("validate-git-fetch"))
		})
	})

	Describe("Category", func() {
		It("returns CategoryGit", func() {
			Expect(validator.Category()).To(Equal(validatorpkg.CategoryGit))
		})
	})

	Describe("Validate", func() {
		Context("when not a git command", func() {
			It("passes for non-git command", func() {
				ctx := createContext("ls -la")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when not in a git repository", func() {
			It("passes when not in repo", func() {
				fakeGit.InRepo = false
				ctx := createContext("git fetch upstream")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("remote existence validation", func() {
			It("passes when remote exists", func() {
				ctx := createContext("git fetch origin")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes when fetching upstream remote", func() {
				ctx := createContext("git fetch upstream")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("fails when remote does not exist", func() {
				ctx := createContext("git fetch nonexistent")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git fetch validation failed"))
				Expect(result.Message).To(ContainSubstring("Remote 'nonexistent' does not exist"))
				Expect(result.Message).To(ContainSubstring("Available remotes:"))
				Expect(result.Message).To(ContainSubstring("origin"))
				Expect(result.Message).To(ContainSubstring("upstream"))
			})

			It("extracts remote from various fetch formats", func() {
				testCases := []struct {
					command     string
					shouldPass  bool
					description string
				}{
					{
						command:     "git fetch origin",
						shouldPass:  true,
						description: "explicit remote",
					},
					{
						command:     "git fetch upstream",
						shouldPass:  true,
						description: "upstream remote",
					},
					{
						command:     "git fetch --prune origin",
						shouldPass:  true,
						description: "with --prune flag",
					},
					{
						command:     "git fetch -p origin",
						shouldPass:  true,
						description: "with -p flag",
					},
					{
						command:     "git fetch --prune badremote",
						shouldPass:  false,
						description: "prune with nonexistent remote",
					},
					{
						command:     "git fetch -p badremote",
						shouldPass:  false,
						description: "-p with nonexistent remote",
					},
					{
						command:     "git fetch --all",
						shouldPass:  true,
						description: "fetch all (no remote specified)",
					},
					{
						command:     "git fetch",
						shouldPass:  true,
						description: "no remote specified (uses default)",
					},
				}

				for _, tc := range testCases {
					ctx := createContext(tc.command)
					result := validator.Validate(context.Background(), ctx)
					Expect(result.Passed).To(Equal(tc.shouldPass), tc.description)
				}
			})
		})

		Context("edge cases", func() {
			It("passes for empty command", func() {
				ctx := createContext("")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for git commands other than fetch", func() {
				ctx := createContext("git pull upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes when command has syntax that parser cannot handle", func() {
				ctx := createContext("git fetch $((")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles missing remote gracefully", func() {
				fakeGit.Remotes = map[string]string{}
				ctx := createContext("git fetch origin")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("with -C flag for different directory", func() {
			It("passes for git fetch with -C flag to valid repo", func() {
				ctx := createContext("git -C /path/to/worktree fetch origin")
				result := validator.Validate(context.Background(), ctx)
				// This creates a new runner for the path, which won't find the repo
				// but should handle gracefully
				Expect(result.Passed).To(BeTrue())
			})

			It("handles -C flag before fetch subcommand", func() {
				ctx := createContext("git -C /some/path fetch upstream")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("combined flags", func() {
			It("handles multiple flags before remote", func() {
				ctx := createContext("git fetch --prune --tags origin")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles verbose flag", func() {
				ctx := createContext("git fetch -v origin")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles dry-run flag", func() {
				ctx := createContext("git fetch --dry-run origin")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})
})
