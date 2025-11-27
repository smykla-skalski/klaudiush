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

var _ = Describe("PushValidator", func() {
	var (
		validator *git.PushValidator
		fakeGit   *gitpkg.FakeRunner
		log       logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		fakeGit = gitpkg.NewFakeRunner()
		fakeGit.InRepo = true
		fakeGit.RepoRoot = "/home/user/projects/github.com/user/my-project"
		validator = git.NewPushValidator(log, fakeGit, nil)
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
			Expect(validator.Name()).To(Equal("validate-git-push"))
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
				ctx := createContext("git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("remote existence validation", func() {
			It("passes when remote exists", func() {
				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("fails when remote does not exist", func() {
				ctx := createContext("git push nonexistent main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git push validation failed"))
				Expect(result.Message).To(ContainSubstring("Remote 'nonexistent' does not exist"))
				Expect(result.Message).To(ContainSubstring("Available remotes:"))
				Expect(result.Message).To(ContainSubstring("origin"))
				Expect(result.Message).To(ContainSubstring("upstream"))
			})

			It("extracts remote from various push formats", func() {
				testCases := []struct {
					command     string
					shouldPass  bool
					description string
				}{
					{
						command:     "git push origin main",
						shouldPass:  true,
						description: "explicit remote and branch",
					},
					{
						command:     "git push upstream feature-branch",
						shouldPass:  true,
						description: "upstream remote",
					},
					{
						command:     "git push badremote main",
						shouldPass:  false,
						description: "nonexistent remote",
					},
					{
						command:     "git push --force-with-lease upstream main",
						shouldPass:  true,
						description: "with flags before remote",
					},
					{
						command:     "git push origin main --force",
						shouldPass:  true,
						description: "with flags after branch",
					},
				}

				for _, tc := range testCases {
					ctx := createContext(tc.command)
					result := validator.Validate(context.Background(), ctx)
					Expect(result.Passed).To(Equal(tc.shouldPass), tc.description)
				}
			})
		})

		Context("default remote handling", func() {
			It("uses tracking remote when no remote specified", func() {
				fakeGit.CurrentBranch = "feature-branch"
				fakeGit.BranchRemotes = map[string]string{
					"feature-branch": "upstream",
				}
				ctx := createContext("git push")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("falls back to origin when no tracking remote", func() {
				fakeGit.CurrentBranch = "feature-branch"
				fakeGit.BranchRemotes = map[string]string{}
				ctx := createContext("git push")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("falls back to origin when getting branch fails", func() {
				fakeGit.Err = &gitpkg.FakeRunnerError{Msg: "not a git repository"}
				ctx := createContext("git push")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Kong organization projects", func() {
			BeforeEach(func() {
				fakeGit.RepoRoot = "/home/user/projects/github.com/kong/kong-mesh"
			})

			It("blocks push to origin remote", func() {
				ctx := createContext("git push origin feature-branch")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Reference).To(Equal(validatorpkg.RefGitKongOrgPush))
				Expect(result.FixHint).To(ContainSubstring("upstream"))
				Expect(result.Message).To(ContainSubstring("Git push validation failed"))
				Expect(
					result.Message,
				).To(ContainSubstring("Kong org projects should push to 'upstream' remote"))
				Expect(result.Message).To(ContainSubstring("'origin' is your fork"))
				Expect(result.Message).To(ContainSubstring("git push upstream branch-name"))
			})

			It("allows push to upstream remote", func() {
				ctx := createContext("git push upstream feature-branch")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("blocks origin with --force flag", func() {
				ctx := createContext("git push --force origin feature-branch")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Message,
				).To(ContainSubstring("Kong org projects should push to 'upstream' remote"))
			})

			It("detects Kong with capital K", func() {
				fakeGit.RepoRoot = "/home/user/projects/github.com/Kong/kong-mesh"
				ctx := createContext("git push origin feature-branch")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("kumahq/kuma projects", func() {
			BeforeEach(func() {
				fakeGit.RepoRoot = "/home/user/projects/github.com/kumahq/kuma"
			})

			It("warns when pushing to upstream", func() {
				ctx := createContext("git push upstream feature-branch")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeFalse())
				Expect(
					result.Message,
				).To(ContainSubstring("Warning: Pushing to 'upstream' remote in kumahq/kuma"))
				Expect(
					result.Message,
				).To(ContainSubstring("This should only be done when explicitly intended"))
				Expect(
					result.Message,
				).To(ContainSubstring("Normal workflow: push to 'origin' (your fork)"))
			})

			It("allows push to origin without warning", func() {
				ctx := createContext("git push origin feature-branch")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})

			It("warns for upstream with force flag", func() {
				ctx := createContext("git push --force-with-lease upstream feature-branch")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Warning"))
			})
		})

		Context("chained commands", func() {
			It("validates all push commands in chain", func() {
				fakeGit.RepoRoot = "/home/user/projects/github.com/kong/kong-mesh"
				ctx := createContext(`git add . && git commit -m "fix" && git push origin main`)
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Kong org projects"))
			})

			It("passes when all commands valid", func() {
				ctx := createContext(`git add . && git commit -m "fix" && git push upstream main`)
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("validates first failing command in chain", func() {
				ctx := createContext("git push badremote main && git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Remote 'badremote' does not exist"))
			})
		})

		Context("complex command formats", func() {
			It("handles subshell", func() {
				fakeGit.RepoRoot = "/home/user/projects/github.com/kong/kong-mesh"
				ctx := createContext("(cd /some/dir && git push origin main)")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("handles OR chain", func() {
				ctx := createContext("git push upstream main || echo failed")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles semicolon separator", func() {
				ctx := createContext("git status; git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("edge cases", func() {
			It("passes for empty command", func() {
				ctx := createContext("")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for git commands other than push", func() {
				ctx := createContext("git pull upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes when command has syntax that parser cannot handle", func() {
				// Use truly invalid syntax that the parser will reject
				ctx := createContext("git push $((")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles missing remote gracefully", func() {
				fakeGit.Remotes = map[string]string{}
				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("other project types", func() {
			It("passes for non-Kong, non-kuma projects", func() {
				fakeGit.RepoRoot = "/home/user/projects/github.com/user/my-project"
				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("allows any valid remote for other projects", func() {
				fakeGit.RepoRoot = "/home/user/projects/github.com/user/my-project"
				ctx := createContext("git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})
})
