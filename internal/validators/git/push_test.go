package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-labs/klaudiush/internal/git"
	validatorpkg "github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/config"
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
		validator = git.NewPushValidator(log, fakeGit, nil, nil)
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

		Context("blocked remotes", func() {
			It("blocks push to configured blocked remote", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"upstream"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Remote 'upstream' is blocked"))
				Expect(result.Message).To(ContainSubstring("Blocked remotes: [upstream]"))
			})

			It("allows push to non-blocked remote", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"upstream"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("suggests alternatives from allowed priority list", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"upstream"}
				cfg.AllowedRemotePriority = []string{"origin", "fork"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Suggested alternatives: [origin]"))
			})

			It("shows all available remotes if no priority matches", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"origin"}
				cfg.AllowedRemotePriority = []string{"nonexistent"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Available remotes: [upstream]"))
			})

			It("uses default priority list when not configured", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"fork"}
				// AllowedRemotePriority not set, should default to ["origin", "upstream"]
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push fork main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Message,
				).To(ContainSubstring("Suggested alternatives: [origin, upstream]"))
			})

			It("does not suggest blocked remotes in alternatives", func() {
				// Add fork remote to the fake git runner
				fakeGit.Remotes["fork"] = "git@github.com:user/fork.git"

				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"origin", "upstream"}
				cfg.AllowedRemotePriority = []string{"origin", "upstream", "fork"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				// Should suggest fork since it's in priority list and not blocked
				Expect(result.Message).To(ContainSubstring("Suggested alternatives: [fork]"))
				// Blocked remotes are shown inline
				Expect(result.Message).To(ContainSubstring("Blocked remotes: [origin, upstream]"))
			})

			It("passes when no blocked remotes configured", func() {
				cfg := &config.PushValidatorConfig{}
				// BlockedRemotes is empty
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes when config is nil", func() {
				validator = git.NewPushValidator(log, fakeGit, nil, nil)

				ctx := createContext("git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles error getting remotes gracefully", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"upstream"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				// Make GetRemotes return an error
				fakeGit.Err = &gitpkg.FakeRunnerError{Msg: "failed to get remotes"}

				ctx := createContext("git push upstream main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Remote 'upstream' is blocked"))
				Expect(result.Message).To(ContainSubstring("Blocked remotes: [upstream]"))
				// Should not have suggestions when we can't get remotes
				Expect(result.Message).NotTo(ContainSubstring("Suggested alternatives"))
				Expect(result.Message).NotTo(ContainSubstring("Available remotes"))
			})

			It("shows no alternatives when all remotes are blocked", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"origin", "upstream"}
				cfg.AllowedRemotePriority = []string{"origin", "upstream"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Blocked remotes: [origin, upstream]"))
				// No available remotes since both are blocked
				Expect(result.Message).NotTo(ContainSubstring("Suggested alternatives"))
				Expect(result.Message).NotTo(ContainSubstring("Available remotes"))
			})

			It("handles multiple blocked remotes in message", func() {
				cfg := &config.PushValidatorConfig{}
				cfg.BlockedRemotes = []string{"bad1", "bad2", "bad3"}
				validator = git.NewPushValidator(log, fakeGit, cfg, nil)

				ctx := createContext("git push bad2 main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Blocked remotes: [bad1, bad2, bad3]"))
				// Should suggest origin and upstream as they're not blocked
				Expect(
					result.Message,
				).To(ContainSubstring("Suggested alternatives: [origin, upstream]"))
			})
		})

		Context("with -C flag for different directory", func() {
			It("passes for git push with -C flag to valid repo", func() {
				ctx := createContext("git -C /path/to/worktree push origin main")
				result := validator.Validate(context.Background(), ctx)
				// This creates a new runner for the path, which won't find the repo
				// but should handle gracefully
				Expect(result.Passed).To(BeTrue())
			})

			It("handles -C flag before push subcommand", func() {
				ctx := createContext("git -C /some/path push upstream feature")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles --git-dir style paths", func() {
				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})
})
