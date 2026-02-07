package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
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
