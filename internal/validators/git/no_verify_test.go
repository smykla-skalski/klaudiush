package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("NoVerifyValidator", func() {
	var (
		validator *git.NoVerifyValidator
		log       logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		validator = git.NewNoVerifyValidator(log, nil, nil)
	})

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
			Expect(validator.Name()).To(Equal("validate-no-verify"))
		})
	})

	Describe("Validate", func() {
		Context("when --no-verify flag is present", func() {
			It("fails for git commit --no-verify", func() {
				ctx := createContext("git commit --no-verify -m 'test'")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(ContainSubstring("not allowed"))
			})

			It("fails for git commit -n", func() {
				ctx := createContext("git commit -n -m 'test'")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("fails for git commit with --no-verify in the middle", func() {
				ctx := createContext("git commit -m 'test' --no-verify")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("fails for chained commands with --no-verify", func() {
				ctx := createContext("git add . && git commit --no-verify -m 'test'")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})
		})

		Context("when --no-verify flag is not present", func() {
			It("passes for normal git commit", func() {
				ctx := createContext("git commit -sS -m 'test'")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for git commit with other flags", func() {
				ctx := createContext("git commit -sS --amend")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for non-commit git commands", func() {
				ctx := createContext("git push origin main")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for non-git commands", func() {
				ctx := createContext("echo 'hello'")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for git add", func() {
				ctx := createContext("git add file.txt")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("edge cases", func() {
			It("passes for commit message containing 'no-verify'", func() {
				ctx := createContext(`git commit -sS -m "fix: no-verify text in message"`)
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("fails for combined flags with -n", func() {
				ctx := createContext("git commit -nm 'test'")
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})
		})
	})
})
