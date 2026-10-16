package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("BranchValidator", func() {
	var (
		v   *git.BranchValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		v = git.NewBranchValidator(nil, logger.NewNoOpLogger(), nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{},
		}
	})

	Describe("git checkout", func() {
		Context("with -b flag", func() {
			It("should pass for feat/add-feature", func() {
				ctx.ToolInput.Command = "git checkout -b feat/add-feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for fix/bug-123", func() {
				ctx.ToolInput.Command = "git checkout -b fix/bug-123"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for docs/update-readme", func() {
				ctx.ToolInput.Command = "git checkout -b docs/update-readme"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for chore/cleanup-code", func() {
				ctx.ToolInput.Command = "git checkout -b chore/cleanup-code"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for ci/update-workflow", func() {
				ctx.ToolInput.Command = "git checkout -b ci/update-workflow"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with --branch flag", func() {
			It("should pass for feat/add-feature", func() {
				ctx.ToolInput.Command = "git checkout --branch feat/add-feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with start-point", func() {
				ctx.ToolInput.Command = "git checkout --branch feat/new-feature upstream/main"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with a bare variable branch name", func() {
			It("skips validation for an unresolved variable", func() {
				// "$BRANCH" is an unresolved expansion; the hook cannot see its
				// runtime value, so its format must not be validated.
				ctx.ToolInput.Command = `git checkout -b "$BRANCH"`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with protected branches", func() {
			It("should skip validation for main", func() {
				ctx.ToolInput.Command = "git checkout -b main"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for master", func() {
				ctx.ToolInput.Command = "git checkout -b master"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with invalid branch names", func() {
			It("should fail for uppercase characters", func() {
				ctx.ToolInput.Command = "git checkout -b Feat/add-feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("lowercase"))
				Expect(result.Message).To(ContainSubstring("feat/add-feature"))
			})

			It("should fail for missing type", func() {
				ctx.ToolInput.Command = "git checkout -b add-feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("type/description"))
			})

			It("should fail for invalid type", func() {
				ctx.ToolInput.Command = "git checkout -b invalid/add-feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Invalid branch type"))
			})

			It("should fail for spaces in branch name with quotes", func() {
				ctx.ToolInput.Command = `git checkout -b "feat/add feature"`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("spaces"))
			})

			It("should pass with start-point argument", func() {
				ctx.ToolInput.Command = "git checkout -b feat/new-branch upstream/main"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail for uppercase in description", func() {
				ctx.ToolInput.Command = "git checkout -b feat/Add-Feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("lowercase"))
			})

			It("should fail for underscore separator", func() {
				ctx.ToolInput.Command = "git checkout -b feat_add_feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("with trailing flags after the branch name", func() {
			It("should pass and ignore a trailing --quiet", func() {
				ctx.ToolInput.Command = "git checkout -b feat/with-flag main --quiet"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should validate the branch name, not the trailing flag", func() {
				ctx.ToolInput.Command = "git checkout -b add-feature --quiet"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("type/description"))
				Expect(result.Message).NotTo(ContainSubstring("--quiet"))
			})

			It("should not be bypassed by a dash-prefixed branch name", func() {
				// "--quiet" as the branch name is captured as the -b value, so it
				// is validated (and rejected) rather than silently skipped.
				ctx.ToolInput.Command = "git checkout -b --quiet"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})
	})

	Describe("git switch", func() {
		Context("with -c flag", func() {
			It("should pass for feat/add-feature", func() {
				ctx.ToolInput.Command = "git switch -c feat/add-feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with start-point", func() {
				ctx.ToolInput.Command = "git switch -c feat/new-feature upstream/main"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail for spaces in branch name with quotes", func() {
				ctx.ToolInput.Command = `git switch -c "feat/add feature"`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("spaces"))
			})

			It("should pass and ignore a trailing --quiet", func() {
				ctx.ToolInput.Command = "git switch -c feat/new-feature main --quiet"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with --create flag", func() {
			It("should pass for fix/bug-fix", func() {
				ctx.ToolInput.Command = "git switch --create fix/bug-fix"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail for uppercase", func() {
				ctx.ToolInput.Command = "git switch --create Fix/bug-fix"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("lowercase"))
			})
		})

		Context("with -C flag", func() {
			It("should pass for feat/force-create", func() {
				ctx.ToolInput.Command = "git switch -C feat/force-create"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should validate the force-created branch name", func() {
				// -C takes the branch name as its value; the name is validated
				// rather than skipped.
				ctx.ToolInput.Command = "git switch -C BadName"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("lowercase"))
			})
		})

		Context("with --force-create flag", func() {
			It("should pass for feat/force-new", func() {
				ctx.ToolInput.Command = "git switch --force-create feat/force-new"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("without create flags", func() {
			It("should skip validation when switching existing branch", func() {
				ctx.ToolInput.Command = "git switch main"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for Invalid-Branch without -c", func() {
				ctx.ToolInput.Command = "git switch Invalid-Branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})

	Describe("git branch", func() {
		Context("with valid branch names", func() {
			It("should pass for feat/add-feature", func() {
				ctx.ToolInput.Command = "git branch feat/add-feature"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for fix/bug-456", func() {
				ctx.ToolInput.Command = "git branch fix/bug-456"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with delete operations", func() {
			It("should skip validation for -d flag", func() {
				ctx.ToolInput.Command = "git branch -d Invalid-Branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -D flag", func() {
				ctx.ToolInput.Command = "git branch -D Invalid-Branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --delete flag", func() {
				ctx.ToolInput.Command = "git branch --delete Invalid-Branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with invalid branch names", func() {
			It("should fail for uppercase characters", func() {
				ctx.ToolInput.Command = "git branch Fix/bug-456"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("lowercase"))
			})

			It("should fail for missing type", func() {
				ctx.ToolInput.Command = "git branch bug-fix"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("with query/list operations", func() {
			It("should skip validation for --contains with commit SHA", func() {
				ctx.ToolInput.Command = "git branch -a --contains 847b00e"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --contains with full SHA", func() {
				ctx.ToolInput.Command = "git branch --contains 847b00e1234567890abcdef"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -a flag", func() {
				ctx.ToolInput.Command = "git branch -a"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --all flag", func() {
				ctx.ToolInput.Command = "git branch --all"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -r flag", func() {
				ctx.ToolInput.Command = "git branch -r"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --remotes flag", func() {
				ctx.ToolInput.Command = "git branch --remotes"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -l flag", func() {
				ctx.ToolInput.Command = "git branch -l"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --list flag", func() {
				ctx.ToolInput.Command = "git branch --list"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -v flag", func() {
				ctx.ToolInput.Command = "git branch -v"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --verbose flag", func() {
				ctx.ToolInput.Command = "git branch --verbose"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --merged flag", func() {
				ctx.ToolInput.Command = "git branch --merged"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --no-merged flag", func() {
				ctx.ToolInput.Command = "git branch --no-merged"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --points-at flag", func() {
				ctx.ToolInput.Command = "git branch --points-at HEAD"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --show-current flag", func() {
				ctx.ToolInput.Command = "git branch --show-current"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -m flag (rename)", func() {
				ctx.ToolInput.Command = "git branch -m old-branch new-branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --move flag", func() {
				ctx.ToolInput.Command = "git branch --move old-branch new-branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -M flag (force rename)", func() {
				ctx.ToolInput.Command = "git branch -M old-branch new-branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -vv flag (double verbose)", func() {
				ctx.ToolInput.Command = "git branch -vv"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --no-contains flag", func() {
				ctx.ToolInput.Command = "git branch --no-contains HEAD"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --sort flag", func() {
				ctx.ToolInput.Command = "git branch --sort=-committerdate"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --format flag", func() {
				ctx.ToolInput.Command = "git branch --format='%(refname:short)'"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --column flag", func() {
				ctx.ToolInput.Command = "git branch --column"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --no-column flag", func() {
				ctx.ToolInput.Command = "git branch --no-column"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -c flag (copy)", func() {
				ctx.ToolInput.Command = "git branch -c old-branch new-branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for -C flag (force copy)", func() {
				ctx.ToolInput.Command = "git branch -C old-branch new-branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for --copy flag", func() {
				ctx.ToolInput.Command = "git branch --copy old-branch new-branch"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for combined flags with argument", func() {
				ctx.ToolInput.Command = "git branch -avv --contains abc123"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for piped head command", func() {
				ctx.ToolInput.Command = "git branch -a --contains 847b00e | head -10"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})

	Describe("existing refs that are not new branch names", func() {
		DescribeTable("passes when the argument names an existing ref",
			func(command string) {
				ctx.ToolInput.Command = command
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue(), result.Message)
			},
			Entry("reported command: rename then set upstream",
				"git branch -m feat/issue-63-launchd && git branch -u origin/main"),
			Entry("-u", "git branch -u origin/main"),
			Entry("--set-upstream-to=", "git branch --set-upstream-to=origin/main"),
			Entry("--set-upstream-to with separate value",
				"git branch --set-upstream-to origin/main"),
			Entry("-u with branch operand", "git branch -u origin/main fix/something"),
			Entry("-u glued to its value", "git branch -uorigin/main"),
			Entry("-u combined with another short flag", "git branch -vu origin/main"),
			Entry("--unset-upstream", "git branch --unset-upstream"),
			Entry("--unset-upstream with branch operand",
				"git branch --unset-upstream fix/something"),
			Entry("-d with a remote-style ref", "git branch -d origin/old"),
			Entry("-D with an unconventional name", "git branch -D Old_Branch"),
			Entry("--delete with several names", "git branch --delete origin/a origin/b"),
			Entry("-d with -r", "git branch -dr origin/old"),
			Entry("--list with a pattern", "git branch --list 'origin/*'"),
			Entry("-l with a pattern", "git branch -l 'origin/*'"),
			Entry("-v", "git branch -v"),
			Entry("-a", "git branch -a"),
			Entry("-r", "git branch -r"),
			Entry("--contains with a commit", "git branch --contains origin/main"),
			Entry("--merged with a ref", "git branch --merged origin/main"),
			Entry("--no-merged with a ref", "git branch --no-merged origin/main"),
			Entry("--points-at with a ref", "git branch --points-at origin/main"),
			Entry("--show-current", "git branch --show-current"),
			Entry("--edit-description", "git branch --edit-description"),
			Entry("after &&", "git fetch origin && git branch -u origin/main"),
			Entry("before &&", "git branch -u origin/main && git status"),
			Entry("after ;", "git status; git branch -u origin/main"),
			Entry("before a pipe", "git branch -u origin/main | cat"),
			Entry("after a cd", "cd /repo && git branch -u origin/main"),
			Entry("with git -C", "git -C /repo branch -u origin/main"),
			Entry("with git -C and -c before the subcommand",
				"git -C /repo -c core.pager=cat branch --set-upstream-to=origin/main"),
			Entry("with --no-pager before the subcommand",
				"git --no-pager branch -u origin/main"),
			Entry("with --git-dir before the subcommand",
				"git --git-dir /repo/.git branch --set-upstream-to origin/main"),
			Entry("in a bash -c script", `bash -c "git branch -u origin/main"`),
		)

		DescribeTable("still blocks new names next to such commands",
			func(command string) {
				ctx.ToolInput.Command = command
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			},
			Entry("creation before set-upstream",
				"git branch badtype/x && git branch -u origin/main"),
			Entry("creation after set-upstream",
				"git branch -u origin/main && git branch badtype/x"),
			Entry("creation after a pipe",
				"git branch -u origin/main | cat; git checkout -b badtype/x"),
		)
	})

	Describe("branch creation forms", func() {
		DescribeTable("blocks an invalid new name",
			func(command, message string) {
				ctx.ToolInput.Command = command
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring(message))
			},
			Entry("checkout -b", "git checkout -b feature/x", "Invalid branch type"),
			Entry("checkout --branch", "git checkout --branch feature/x", "Invalid branch type"),
			Entry("switch -c", "git switch -c badtype/x", "Invalid branch type"),
			Entry("switch --create", "git switch --create badtype/x", "Invalid branch type"),
			Entry("switch -C", "git switch -C badtype/x", "Invalid branch type"),
			Entry("branch name only", "git branch newname", "type/description"),
			Entry("branch name with invalid type", "git branch badtype/x", "Invalid branch type"),
			Entry("branch name with start point",
				"git branch badtype/x origin/main", "Invalid branch type"),
			Entry("branch with -f", "git branch -f badtype/x origin/main", "Invalid branch type"),
			Entry("branch with --force",
				"git branch --force badtype/x origin/main", "Invalid branch type"),
			Entry("branch with --track",
				"git branch --track badtype/x origin/main", "Invalid branch type"),
			Entry("branch with --track=direct",
				"git branch --track=direct badtype/x origin/main", "Invalid branch type"),
			Entry("branch with -t", "git branch -t badtype/x origin/main", "Invalid branch type"),
			Entry("branch with --no-track",
				"git branch --no-track badtype/x origin/main", "Invalid branch type"),
			Entry("branch with -q", "git branch -q badtype/x", "Invalid branch type"),
			Entry("branch with git -C", "git -C /repo branch badtype/x", "Invalid branch type"),
			Entry("checkout with git -C and -c",
				"git -C /repo -c core.pager=cat checkout -b badtype/x", "Invalid branch type"),
			Entry("branch with --no-pager",
				"git --no-pager branch badtype/x", "Invalid branch type"),
			Entry("branch after &&",
				"git fetch origin && git branch badtype/x", "Invalid branch type"),
			Entry("branch after ;", "git status; git branch badtype/x", "Invalid branch type"),
			Entry("branch with uppercase", "git branch Feat/x", "lowercase"),
		)

		DescribeTable("passes a valid new name",
			func(command string) {
				ctx.ToolInput.Command = command
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue(), result.Message)
			},
			Entry("checkout -b", "git checkout -b feat/x"),
			Entry("switch -c", "git switch -c fix/x origin/main"),
			Entry("branch with start point", "git branch feat/x origin/main"),
			Entry("branch with --track", "git branch --track feat/x origin/main"),
			Entry("branch with -f", "git branch -f feat/x HEAD~1"),
			Entry("branch with git -C", "git -C /repo branch feat/x"),
		)
	})

	Describe("chained commands", func() {
		It("should validate branch in chained command", func() {
			ctx.ToolInput.Command = "git fetch upstream && git checkout -b feat/new-feature"
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail for invalid branch in chained command", func() {
			ctx.ToolInput.Command = "git fetch upstream && git checkout -b Invalid-Branch"
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
		})
	})

	Describe("non-branch commands", func() {
		It("should pass for git checkout without -b", func() {
			ctx.ToolInput.Command = "git checkout main"
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for git status", func() {
			ctx.ToolInput.Command = "git status"
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for git commit", func() {
			ctx.ToolInput.Command = "git commit -sS -m 'test'"
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})
})
