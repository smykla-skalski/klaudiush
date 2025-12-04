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

	Describe("Comprehensive mode", func() {
		BeforeEach(func() {
			checkUnquoted := true
			suggestSingleQuotes := true
			cfg = &config.BacktickValidatorConfig{
				CheckAllCommands:    true,
				CheckUnquoted:       &checkUnquoted,
				SuggestSingleQuotes: &suggestSingleQuotes,
			}
			v = shell.NewBacktickValidator(log, cfg, nil)
		})

		Context("with unquoted backticks", func() {
			It("blocks unquoted backticks in any command", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo `date`",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details["help"]).To(ContainSubstring("Unquoted backticks"))
			})

			It("allows unquoted backticks when CheckUnquoted is disabled", func() {
				checkUnquoted := false
				cfg.CheckUnquoted = &checkUnquoted
				v = shell.NewBacktickValidator(log, cfg, nil)

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo `date`",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with backticks in double quotes", func() {
			It("blocks backticks without variables and suggests single quotes", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo \"Fix bug in `parser` module\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details["help"]).To(ContainSubstring("Use single quotes instead"))
			})

			It("blocks backticks with variables but doesn't suggest single quotes", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo \"Fix `parser` for $VERSION\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details["help"]).To(ContainSubstring("has variables: true"))
				Expect(result.Details["help"]).NotTo(ContainSubstring("Use single quotes instead"))
			})

			It("doesn't suggest single quotes when disabled", func() {
				suggestSingleQuotes := false
				cfg.SuggestSingleQuotes = &suggestSingleQuotes
				v = shell.NewBacktickValidator(log, cfg, nil)

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo \"Fix bug in `parser` module\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["help"]).NotTo(ContainSubstring("Use single quotes instead"))
				Expect(result.Details["help"]).To(ContainSubstring("Escape backticks"))
			})
		})

		Context("with any command type", func() {
			It("blocks backticks in non-git/gh commands", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "curl -d \"Testing `API`\" https://example.com",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("blocks backticks in ls command", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "ls \"`pwd`/files\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})
		})

		Context("edge cases", func() {
			It("allows single-quoted backticks", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo 'Backticks `are` safe here'",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("allows commands without backticks", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo \"Normal string\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("handles multiple backtick issues in one command", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "command \"arg1 `test`\" `arg2` \"arg3 `foo` with $VAR\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["help"]).To(ContainSubstring("Unquoted backticks"))
				Expect(result.Details["help"]).To(ContainSubstring("Backticks in double quotes"))
			})
		})
	})

	Describe("Legacy mode (default)", func() {
		BeforeEach(func() {
			cfg = &config.BacktickValidatorConfig{
				CheckAllCommands: false, // Legacy mode
			}
			v = shell.NewBacktickValidator(log, cfg, nil)
		})

		It("ignores backticks in non-git/gh commands", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "echo \"Testing `API`\"",
				},
			}

			result := v.Validate(ctx, hookCtx)

			Expect(result.Passed).To(BeTrue())
		})

		It("still blocks backticks in git commit", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git commit -m \"Fix `parser`\"",
				},
			}

			result := v.Validate(ctx, hookCtx)

			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
		})

		Context("edge cases for git commands", func() {
			It("ignores git command with no subcommand", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("ignores git non-commit subcommands", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git log --oneline \"$(echo test)\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("ignores backticks at wrong argument index for git commit", func() {
				// Backticks not in -m value position
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git \"commit\" -m 'safe message'",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("edge cases for gh commands", func() {
			It("ignores gh command with only one arg", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("ignores gh pr view (not create)", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr view --json \"body with `backticks`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("ignores gh issue view (not create)", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh issue view --json \"body with `backticks`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("ignores gh repo create (different subcommand)", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh repo create --description \"test `repo`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})

			It("blocks backticks in --body= form", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr create --body=\"Fix `parser`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("blocks backticks in --title= form", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh issue create --title=\"Bug in `validate`\"",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("ignores backticks at wrong argument position", func() {
				// Backticks in --json flag, not --body or --title
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr create --json \"$(echo test)\" --body 'safe body'",
					},
				}

				result := v.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})
	})
})
