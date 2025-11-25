package validator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var _ = Describe("Git Predicates", func() {
	Describe("GitSubcommandIs", func() {
		It("matches git checkout command", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git checkout with -C global option", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /path/to/repo checkout main"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git checkout with long path in -C option", func() {
			ctx := &hook.Context{
				ToolName: hook.Bash,
				ToolInput: hook.ToolInput{
					Command: "git -C /Users/test/Projects/github.com/org/repo checkout -b feature",
				},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match different subcommand", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git commit -m 'test'"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match non-git command", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "ls -la"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match non-Bash tool", func() {
			ctx := &hook.Context{
				ToolName:  hook.Write,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandIn", func() {
		It("matches any of the specified subcommands", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /repo checkout -b feature"},
			}

			predicate := validator.GitSubcommandIn("checkout", "switch", "branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match unlisted subcommand", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git commit -m 'test'"},
			}

			predicate := validator.GitSubcommandIn("checkout", "switch", "branch")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitHasFlag", func() {
		It("matches flag in command", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git checkout -b feature"},
			}

			predicate := validator.GitHasFlag("-b")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches flag with -C global option", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /path/to/repo checkout -b feature"},
			}

			predicate := validator.GitHasFlag("-b")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match missing flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitHasFlag("-b")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitHasAnyFlag", func() {
		It("matches any of the specified flags", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /repo checkout -b feature upstream/main"},
			}

			predicate := validator.GitHasAnyFlag("-b", "--branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches long flag variant", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git checkout --branch feature"},
			}

			predicate := validator.GitHasAnyFlag("-b", "--branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match when none of the flags are present", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitHasAnyFlag("-b", "--branch")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandWithFlag", func() {
		It("matches subcommand with specific flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /repo checkout -b feature upstream/main"},
			}

			predicate := validator.GitSubcommandWithFlag("checkout", "-b")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match subcommand without flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitSubcommandWithFlag("checkout", "-b")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match different subcommand with flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git commit -m 'test'"},
			}

			predicate := validator.GitSubcommandWithFlag("checkout", "-b")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandWithAnyFlag", func() {
		It("matches checkout with -b flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /path/to/repo checkout -b feature upstream/main"},
			}

			predicate := validator.GitSubcommandWithAnyFlag("checkout", "-b", "--branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches switch with -c flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git switch -c feature"},
			}

			predicate := validator.GitSubcommandWithAnyFlag("switch", "-c", "--create", "-C", "--force-create")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match checkout without branch creation flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /repo checkout main"},
			}

			predicate := validator.GitSubcommandWithAnyFlag("checkout", "-b", "--branch")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandWithoutAnyFlag", func() {
		It("matches branch without delete flags", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /repo branch new-feature"},
			}

			predicate := validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match branch with delete flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git branch -d old-feature"},
			}

			predicate := validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match branch with -D flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{Command: "git -C /repo branch -D old-feature"},
			}

			predicate := validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("Real-world scenarios", func() {
		It("matches the original failing case: git -C with long path checkout -b", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: "git -C /Users/bart.smykla@konghq.com/Projects/github.com/kong/kong-mesh checkout -b revert-mink-workflow-dispatch upstream/master",
				},
			}

			// This is the predicate used by the branch validator
			predicate := validator.And(
				validator.EventTypeIs(hook.PreToolUse),
				validator.Or(
					validator.GitSubcommandWithAnyFlag("checkout", "-b", "--branch"),
					validator.GitSubcommandWithAnyFlag("switch", "-c", "--create", "-C", "--force-create"),
					validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete"),
				),
			)

			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git commit with -C path", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: "git -C /path/to/repo commit -sS -m 'feat: add feature'",
				},
			}

			predicate := validator.And(
				validator.EventTypeIs(hook.PreToolUse),
				validator.GitSubcommandIs("commit"),
			)

			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git push with --git-dir option", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: "git --git-dir=/custom/.git push origin main",
				},
			}

			predicate := validator.And(
				validator.EventTypeIs(hook.PreToolUse),
				validator.GitSubcommandIs("push"),
			)

			Expect(predicate(ctx)).To(BeTrue())
		})
	})
})
