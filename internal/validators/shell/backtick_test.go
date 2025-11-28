package shell_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators/shell"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("BacktickValidator", func() {
	var (
		v   *shell.BacktickValidator
		ctx context.Context
		log logger.Logger
		cfg *config.BacktickValidatorConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		log = logger.NewNoOpLogger()
		cfg = &config.BacktickValidatorConfig{}
		v = shell.NewBacktickValidator(log, cfg, nil)
	})

	Describe("Validate", func() {
		Context("with git commit commands", func() {
			It("blocks backticks in -m flag", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m \"Fix bug in `parser` module\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(ContainSubstring("Command substitution detected"))
				Expect(result.Reference).To(Equal(validator.RefShellBackticks))
			})

			It("blocks backticks in --message flag", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit --message=\"Update `config` file\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("allows single-quoted strings with backticks", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m 'Fix bug in `parser` module'",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("detects HEREDOC pattern", func() {
				cmd := "git commit -m \"$(cat <<'EOF'\nFix bug in parser module\nEOF\n)\""
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: cmd,
					},
				}

				result := v.Validate(ctx, hookCtx)

				// HEREDOC uses $() which is command substitution in double quotes
				// This is detected, though it's the recommended pattern
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("with gh pr create commands", func() {
			It("blocks backticks in --body flag", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr create --body \"Updated `config.toml` handling\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("blocks backticks in --title flag", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr create --title \"Fix `parser`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})
		})

		Context("with gh issue create commands", func() {
			It("blocks backticks in --body flag", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh issue create --body \"The `validate` function fails\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("blocks backticks in --title flag", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh issue create --title \"Bug in `validate`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})
		})

		Context("with non-relevant commands", func() {
			It("ignores backticks in other commands", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo \"Testing `backticks`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("ignores backticks in git status", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git status",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with empty or invalid input", func() {
			It("passes on empty command", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("passes on invalid bash syntax", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m \"unclosed",
					},
				}

				result := v.Validate(ctx, hookCtx)

				// Should pass because parser fails gracefully
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("error messages", func() {
			It("includes helpful suggestions", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m \"Fix `parser`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Details["help"]).To(ContainSubstring("HEREDOC"))
				Expect(result.Details["help"]).To(ContainSubstring("file-based input"))
				Expect(result.Details["help"]).To(ContainSubstring("--body-file"))
			})

			It("includes fix hint from suggestions registry", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m \"Fix `parser`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.FixHint).NotTo(BeEmpty())
				Expect(result.FixHint).To(ContainSubstring("HEREDOC"))
			})
		})
	})

	Describe("Category", func() {
		It("returns CategoryCPU", func() {
			Expect(v.Category()).To(Equal(validator.CategoryCPU))
		})
	})
})
