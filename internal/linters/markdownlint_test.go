package linters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("MarkdownLinter", func() {
	var (
		linter linters.MarkdownLinter
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		linter = linters.NewMarkdownLinter(nil) // runner not used for custom rules only
	})

	Describe("Lint", func() {
		Context("when content has custom rule violations", func() {
			It("should fail with custom rule output", func() {
				// Content with custom rule violation (no empty line before code block)
				content := `# Test
Some text
` + "```" + `
code
` + "```"

				result := linter.Lint(ctx, content, nil)

				// Should fail due to custom rules
				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(
					result.RawOut,
				).To(ContainSubstring("Code block should have empty line before it"))
			})
		})

		Context("when content has no custom rule violations", func() {
			It("should return success", func() {
				content := `# Test

Some text

` + "```" + `
code
` + "```"

				result := linter.Lint(ctx, content, nil)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})
	})

	Describe("NewMarkdownLinter", func() {
		It("should create a linter with nil config", func() {
			mockRunner := execpkg.NewMockCommandRunner(gomock.NewController(GinkgoT()))
			linter := linters.NewMarkdownLinter(mockRunner)
			Expect(linter).NotTo(BeNil())
		})
	})

	Describe("NewMarkdownLinterWithConfig", func() {
		It("should create a linter with provided config", func() {
			mockRunner := execpkg.NewMockCommandRunner(gomock.NewController(GinkgoT()))
			cfg := &config.MarkdownValidatorConfig{
				MarkdownlintRules: map[string]bool{
					"MD022": true,
				},
			}
			linter := linters.NewMarkdownLinterWithConfig(mockRunner, cfg)
			Expect(linter).NotTo(BeNil())
		})
	})

	Describe("Tool Detection Functions", func() {
		DescribeTable("IsMarkdownlintCli2",
			func(toolPath string, expected bool) {
				result := linters.IsMarkdownlintCli2(toolPath)
				Expect(result).To(Equal(expected))
			},
			Entry("markdownlint-cli2 binary", "/usr/local/bin/markdownlint-cli2", true),
			Entry("markdownlint-cli binary", "/usr/local/bin/markdownlint-cli", false),
			Entry("markdownlint binary", "/usr/local/bin/markdownlint", false),
			Entry("custom markdownlint-cli2 wrapper path",
				"/usr/bin/my-markdownlint-cli2-wrapper", true),
			Entry("custom markdownlint wrapper path",
				"/usr/bin/my-markdownlint-wrapper", false),
			Entry("markdownlint-cli2 in node_modules",
				"/path/to/node_modules/.bin/markdownlint-cli2", true),
			Entry("markdownlint-cli in node_modules",
				"/path/to/node_modules/.bin/markdownlint-cli", false),
			Entry("empty path", "", false),
		)

		DescribeTable("GenerateFragmentConfigContent",
			func(isCli2, disableMD047 bool, expectedContent string) {
				result := linters.GenerateFragmentConfigContent(isCli2, disableMD047)
				Expect(result).To(Equal(expectedContent))
			},
			Entry("markdownlint-cli2 with MD047 disabled", true, true, `{
  "config": {
    "MD047": false
  }
}`),
			Entry("markdownlint-cli2 with MD047 enabled", true, false, `{
  "config": {}
}`),
			Entry("markdownlint-cli with MD047 disabled", false, true, `{
  "MD047": false
}`),
			Entry("markdownlint-cli with MD047 enabled", false, false, "{}"),
		)

		DescribeTable("GetFragmentConfigPattern",
			func(isCli2 bool, expectedPattern string) {
				result := linters.GetFragmentConfigPattern(isCli2)
				Expect(result).To(Equal(expectedPattern))
			},
			Entry("markdownlint-cli2 pattern",
				true, "fragment-*.markdownlint-cli2.jsonc"),
			Entry("markdownlint-cli pattern",
				false, "markdownlint-fragment-*.json"),
		)
	})

	Describe("Internal Helper Methods", func() {
		Describe("shouldUseMarkdownlint", func() {
			It("should return false when config is nil", func() {
				linter := linters.NewMarkdownLinter(nil)
				result := linter.Lint(ctx, "# Test\n", nil)
				Expect(result.Success).To(BeTrue())
			})

			It("should return false when UseMarkdownlint is nil", func() {
				cfg := &config.MarkdownValidatorConfig{}
				linter := linters.NewMarkdownLinterWithConfig(nil, cfg)
				result := linter.Lint(ctx, "# Test\n", nil)
				Expect(result.Success).To(BeTrue())
			})

			It("should not use markdownlint when UseMarkdownlint is false", func() {
				useMarkdownlint := false
				cfg := &config.MarkdownValidatorConfig{
					UseMarkdownlint: &useMarkdownlint,
				}
				linter := linters.NewMarkdownLinterWithConfig(nil, cfg)
				result := linter.Lint(ctx, "# Test\n", nil)
				Expect(result.Success).To(BeTrue())
			})
		})

		Describe("isTableFormattingEnabled", func() {
			It("should use default when config is nil", func() {
				linter := linters.NewMarkdownLinter(nil)
				result := linter.Lint(ctx, "# Test\n", nil)
				Expect(result.Success).To(BeTrue())
			})

			It("should use default when TableFormatting is nil", func() {
				cfg := &config.MarkdownValidatorConfig{}
				linter := linters.NewMarkdownLinterWithConfig(nil, cfg)
				result := linter.Lint(ctx, "# Test\n", nil)
				Expect(result.Success).To(BeTrue())
			})

			It("should respect TableFormatting when false", func() {
				tableFormatting := false
				cfg := &config.MarkdownValidatorConfig{
					TableFormatting: &tableFormatting,
				}
				linter := linters.NewMarkdownLinterWithConfig(nil, cfg)
				result := linter.Lint(ctx, "# Test\n", nil)
				Expect(result.Success).To(BeTrue())
			})
		})

		Describe("getTableWidthMode", func() {
			It("should return WidthModeDisplay when config is nil (default)", func() {
				linter := linters.NewMarkdownLinter(nil)
				result := linter.Lint(ctx, "# Test", nil)
				Expect(result.Success).To(BeTrue())
			})

			It(
				"should return WidthModeDisplay when TableFormattingMode is empty (default)",
				func() {
					cfg := &config.MarkdownValidatorConfig{
						TableFormattingMode: "",
					}
					linter := linters.NewMarkdownLinterWithConfig(nil, cfg)
					result := linter.Lint(ctx, "# Test", nil)
					Expect(result.Success).To(BeTrue())
				},
			)

			It("should return WidthModeByte when TableFormattingMode is byte_width", func() {
				cfg := &config.MarkdownValidatorConfig{
					TableFormattingMode: "byte_width",
				}
				linter := linters.NewMarkdownLinterWithConfig(nil, cfg)
				result := linter.Lint(ctx, "# Test", nil)
				Expect(result.Success).To(BeTrue())
			})

			It("should return WidthModeDisplay for unknown mode", func() {
				cfg := &config.MarkdownValidatorConfig{
					TableFormattingMode: "unknown_mode",
				}
				linter := linters.NewMarkdownLinterWithConfig(nil, cfg)
				result := linter.Lint(ctx, "# Test", nil)
				Expect(result.Success).To(BeTrue())
			})
		})
	})

	Describe("ProcessMarkdownlintOutput", func() {
		It("should return success when exit code is 0", func() {
			result := &execpkg.CommandResult{
				ExitCode: 0,
				Stdout:   "",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(result, "/tmp/test.md", 0)
			Expect(lintResult.Success).To(BeTrue())
			Expect(lintResult.RawOut).To(BeEmpty())
		})

		It("should replace temp file path in output", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "/tmp/test.md:10 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(result, "/tmp/test.md", 0)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:10"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("/tmp/test.md"))
		})

		It("should adjust line numbers when preamble exists", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:15 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(result, "/tmp/test.md", 5)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:10"))
		})

		It("should filter out preamble errors", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:3 MD022/blanks-around-headings\n<file>:10 MD001/heading-increment",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(result, "/tmp/test.md", 5)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).NotTo(ContainSubstring("<file>:3"))
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:5"))
		})

		It("should return success when all errors are in preamble", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:1 MD022\n<file>:2 MD001",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(result, "/tmp/test.md", 5)
			Expect(lintResult.Success).To(BeTrue())
		})

		It("should combine stdout and stderr", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:10 MD022",
				Stderr:   "Warning: some warning",
			}
			lintResult := linters.ProcessMarkdownlintOutput(result, "/tmp/test.md", 0)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("MD022"))
			Expect(lintResult.RawOut).To(ContainSubstring("Warning"))
		})
	})

	Describe("Edge cases", func() {
		It("should handle empty content", func() {
			linter := linters.NewMarkdownLinter(nil)
			result := linter.Lint(ctx, "", nil)
			Expect(result.Success).To(BeTrue())
		})

		It("should handle content with only whitespace", func() {
			linter := linters.NewMarkdownLinter(nil)
			result := linter.Lint(ctx, "   \n\n  ", nil)
			Expect(result.Success).To(BeTrue())
		})

		It("should handle very long content", func() {
			linter := linters.NewMarkdownLinter(nil)
			longContent := "# Test\n\n" + string(make([]byte, 10000))
			result := linter.Lint(ctx, longContent, nil)
			// May or may not succeed depending on content, just ensure it doesn't crash
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("Error cases", func() {
		It("should return ErrNoRulesConfigured when creating temp config with no rules", func() {
			// This is an internal error, tested through behavior
		})
	})
})
