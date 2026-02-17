package file_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("LinterIgnoreValidator", func() {
	var (
		v   *file.LinterIgnoreValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		v = file.NewLinterIgnoreValidator(logger.NewNoOpLogger(), nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
		}
	})

	Describe("Name", func() {
		It("returns correct validator name", func() {
			Expect(v.Name()).To(Equal("validate-linter-ignore"))
		})
	})

	Describe("Category", func() {
		It("returns CategoryCPU", func() {
			Expect(v.Category()).To(Equal(validator.CategoryCPU))
		})
	})

	Describe("NewLinterIgnoreValidator", func() {
		It("handles invalid regex patterns gracefully", func() {
			cfg := &config.LinterIgnoreValidatorConfig{
				Patterns: []string{
					`valid-pattern`,
					`[invalid(regex`, // Invalid regex
					`another-valid`,
				},
			}

			validator := file.NewLinterIgnoreValidator(logger.NewNoOpLogger(), cfg, nil)
			Expect(validator).NotTo(BeNil())
			// Validator should be created even with some invalid patterns
		})

		It("uses default patterns when config is nil", func() {
			validator := file.NewLinterIgnoreValidator(logger.NewNoOpLogger(), nil, nil)
			Expect(validator).NotTo(BeNil())
		})

		It("uses custom patterns from config", func() {
			cfg := &config.LinterIgnoreValidatorConfig{
				Patterns: []string{`custom-ignore`},
			}

			validator := file.NewLinterIgnoreValidator(logger.NewNoOpLogger(), cfg, nil)
			testCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					Content: `x = 1  # custom-ignore`,
				},
			}

			result := validator.Validate(context.Background(), testCtx)
			Expect(result.Passed).To(BeFalse())
		})

		It("uses default patterns when config has empty patterns", func() {
			cfg := &config.LinterIgnoreValidatorConfig{
				Patterns: []string{},
			}

			validator := file.NewLinterIgnoreValidator(logger.NewNoOpLogger(), cfg, nil)
			testCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					Content: `x = 1  # noqa`,
				},
			}

			result := validator.Validate(context.Background(), testCtx)
			Expect(result.Passed).To(BeFalse())
		})
	})

	Describe("Validate", func() {
		Context("with clean code", func() {
			It("passes for code with no ignore directives", func() {
				ctx.ToolInput.Content = `
def my_function():
    """A clean function"""
    return 42
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for empty content", func() {
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for comments without ignore directives", func() {
				ctx.ToolInput.Content = `
// This is a regular comment
/* Another comment */
# Python comment
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("Python ignore directives", func() {
			It("fails for noqa directive", func() {
				ctx.ToolInput.Content = `
def bad_function():
    x = 1  # noqa
    return x
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(
					result.Message,
				).To(ContainSubstring("Linter ignore directives are not allowed"))
				Expect(result.Message).To(ContainSubstring("# noqa"))
			})

			It("fails for noqa with error code", func() {
				ctx.ToolInput.Content = `x = 1  # noqa: E501`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for type: ignore", func() {
				ctx.ToolInput.Content = `
x: int = "wrong"  # type: ignore
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("# type: ignore"))
			})

			It("fails for pylint: disable", func() {
				ctx.ToolInput.Content = `x = 1  # pylint: disable=invalid-name`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for pyright: ignore", func() {
				ctx.ToolInput.Content = `x = 1  # pyright: ignore`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for mypy: ignore", func() {
				ctx.ToolInput.Content = `x = 1  # mypy: ignore`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("JavaScript/TypeScript ignore directives", func() {
			It("fails for eslint-disable", func() {
				ctx.ToolInput.Content = `
// eslint-disable no-console
console.log("test");
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("// eslint-disable"))
			})

			It("fails for @ts-ignore", func() {
				ctx.ToolInput.Content = `
// @ts-ignore
const x: number = "wrong";
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for @ts-nocheck", func() {
				ctx.ToolInput.Content = `// @ts-nocheck`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for @ts-expect-error", func() {
				ctx.ToolInput.Content = `// @ts-expect-error`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for block comment eslint-disable", func() {
				ctx.ToolInput.Content = `/* eslint-disable */`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Go ignore directives", func() {
			It("fails for //nolint", func() {
				ctx.ToolInput.Content = `
func bad() { //nolint
    // code
}
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("//nolint"))
			})

			It("fails for //nolint with rule", func() {
				ctx.ToolInput.Content = `var x = 1 //nolint:errcheck`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for // nolint with space", func() {
				ctx.ToolInput.Content = `var x = 1 // nolint`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Rust ignore directives", func() {
			It("fails for #[allow(...)]", func() {
				ctx.ToolInput.Content = `
#[allow(dead_code)]
fn unused() {}
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("#[allow("))
			})

			It("fails for #![allow(...)]", func() {
				ctx.ToolInput.Content = `#![allow(missing_docs)]`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Ruby ignore directives", func() {
			It("fails for rubocop:disable", func() {
				ctx.ToolInput.Content = `x = 1 # rubocop:disable Metrics/LineLength`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Shell ignore directives", func() {
			It("fails for shellcheck disable", func() {
				ctx.ToolInput.Content = `# shellcheck disable=SC2086`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Java ignore directives", func() {
			It("fails for @SuppressWarnings", func() {
				ctx.ToolInput.Content = `@SuppressWarnings("unchecked")`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("C# ignore directives", func() {
			It("fails for #pragma warning disable", func() {
				ctx.ToolInput.Content = `#pragma warning disable CS0618`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("PHP ignore directives", func() {
			It("fails for phpcs:ignore", func() {
				ctx.ToolInput.Content = `// phpcs:ignore WordPress.Security.EscapeOutput`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})

			It("fails for @phpstan-ignore", func() {
				ctx.ToolInput.Content = `// @phpstan-ignore-next-line`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Swift ignore directives", func() {
			It("fails for swiftlint:disable", func() {
				ctx.ToolInput.Content = `// swiftlint:disable line_length`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
			})
		})

		Context("Edit operations", func() {
			BeforeEach(func() {
				ctx.ToolName = hook.ToolTypeEdit
			})

			It("checks new_string content", func() {
				ctx.ToolInput.OldString = `x = 1`
				ctx.ToolInput.NewString = `x = 1  # noqa`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("# noqa"))
			})

			It("passes when new_string is clean", func() {
				ctx.ToolInput.OldString = `x = 1  # noqa`
				ctx.ToolInput.NewString = `x = 1`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes when new_string is empty", func() {
				ctx.ToolInput.OldString = `x = 1`
				ctx.ToolInput.NewString = ``
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("multiple violations", func() {
			It("reports all violations", func() {
				ctx.ToolInput.Content = `
x = 1  # noqa
y = 2  # type: ignore
`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("# noqa"))
				Expect(result.Message).To(ContainSubstring("# type: ignore"))
			})

			It("reports only first match per line", func() {
				ctx.ToolInput.Content = `x = 1  # noqa  # type: ignore`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				// Should only report first match (noqa)
				Expect(result.Message).To(ContainSubstring("# noqa"))
			})
		})

		Context("error reference", func() {
			It("includes FILE010 reference", func() {
				ctx.ToolInput.Content = `x = 1  # noqa`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(string(result.Reference)).To(Equal("https://klaudiu.sh/FILE010"))
			})

			It("includes fix hint", func() {
				ctx.ToolInput.Content = `x = 1  # noqa`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.FixHint).To(ContainSubstring("Fix linter errors properly"))
			})
		})
	})
})
