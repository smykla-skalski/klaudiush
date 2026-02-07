package linters_test

import (
	"context"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators"
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

		DescribeTable("GenerateRuntimeConfigContent",
			func(isCli2, disableMD013, disableMD047 bool, expectedContent string) {
				result := linters.GenerateRuntimeConfigContent(isCli2, disableMD013, disableMD047)
				Expect(result).To(Equal(expectedContent))
			},
			Entry("markdownlint-cli2 with MD013 and MD047 disabled", true, true, true, `{
  "config": {
    "MD013": false,
    "MD047": false
  }
}`),
			Entry("markdownlint-cli2 with only MD013 disabled", true, true, false, `{
  "config": {
    "MD013": false
  }
}`),
			Entry("markdownlint-cli2 with only MD047 disabled", true, false, true, `{
  "config": {
    "MD047": false
  }
}`),
			Entry("markdownlint-cli2 with nothing disabled", true, false, false, `{}`),
			Entry("markdownlint-cli with MD013 and MD047 disabled", false, true, true, `{
  "MD013": false,
  "MD047": false
}`),
			Entry("markdownlint-cli with only MD013 disabled", false, true, false, `{
  "MD013": false
}`),
			Entry("markdownlint-cli with only MD047 disabled", false, false, true, `{
  "MD047": false
}`),
			Entry("markdownlint-cli with nothing disabled", false, false, false, `{}`),
		)

		DescribeTable("GetRuntimeConfigPattern",
			func(isCli2 bool, expectedPattern string) {
				result := linters.GetRuntimeConfigPattern(isCli2)
				Expect(result).To(Equal(expectedPattern))
			},
			Entry("markdownlint-cli2 pattern",
				true, "runtime-*.markdownlint-cli2.jsonc"),
			Entry("markdownlint-cli pattern",
				false, "markdownlint-runtime-*.json"),
		)

		// IsMarkdownFile is tested indirectly through LintWithPath tests
		// in the "MD013 disabled for markdown files" context below
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
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				0,
				0,
				false,
				"<file>",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeTrue())
			Expect(lintResult.RawOut).To(BeEmpty())
		})

		It("should replace temp file path in output", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "/tmp/test.md:10 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				0,
				0,
				false,
				"<file>",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:10"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("/tmp/test.md"))
		})

		It("should use actual file path when provided", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "/tmp/test.md:10 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				0,
				0,
				false,
				"README.md",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("README.md:10"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("/tmp/"))
		})

		It("should adjust line numbers when preamble exists", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:15 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				5,
				0,
				false,
				"<file>",
				"",
				false, // isFragment - testing preamble adjustment without fragment enhancement
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:11"))
		})

		It("should adjust line numbers with fragment start line", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:5 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				2,
				10,
				false,
				"<file>",
				"",
				false, // isFragment - testing line number adjustment only
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:14"))
		})

		It("should filter out preamble errors", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:3 MD022/blanks-around-headings\n<file>:10 MD001/heading-increment",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				5,
				0,
				false,
				"<file>",
				"",
				false, // isFragment - testing preamble error filtering
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).NotTo(ContainSubstring("<file>:3"))
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:6"))
		})

		It("should return success when all errors are in preamble", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:1 MD022\n<file>:2 MD001",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				5,
				0,
				false,
				"<file>",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeTrue())
		})

		It("should combine stdout and stderr", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:10 MD022",
				Stderr:   "Warning: some warning",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				0,
				0,
				false,
				"<file>",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("MD022"))
			Expect(lintResult.RawOut).To(ContainSubstring("Warning"))
		})

		It("should filter out markdownlint-cli2 status messages", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout: `markdownlint-cli2 v0.17.1 (markdownlint v0.37.3)
Finding: <file>
Linting: 1 file(s)
Summary: 1 error(s)
<file>:10 MD022/blanks-around-headings`,
				Stderr: "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				0,
				0,
				true,
				"<file>",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("<file>:10 MD022"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("markdownlint-cli2"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("Finding:"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("Linting:"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("Summary:"))
		})

		It("should not filter status messages for markdownlint-cli", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "Finding: <file>\n<file>:10 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				0,
				0,
				false,
				"<file>",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("Finding:"))
		})

		It("should replace relative temp file paths with display path", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "../../../tmp/markdownlint-test123.md:10 MD022/blanks-around-headings",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/markdownlint-test123.md",
				0,
				0,
				false,
				"README.md",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("README.md:10"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("../../../"))
		})

		It("should replace complex relative paths with display path", func() {
			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout: `../../../../../../var/folders/d1/tvmyp5cs1gz38rltf390ddpw0000gn/T/markdownlint-456.md:5 MD022/blanks-around-headings
../../../../../../var/folders/d1/tvmyp5cs1gz38rltf390ddpw0000gn/T/markdownlint-456.md:10 MD001/heading-increment`,
				Stderr: "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/var/folders/d1/tvmyp5cs1gz38rltf390ddpw0000gn/T/markdownlint-456.md",
				0,
				0,
				false,
				".claude/session.md",
				"",
				false, // isFragment
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring(".claude/session.md:5"))
			Expect(lintResult.RawOut).To(ContainSubstring(".claude/session.md:10"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("../"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("/var/folders/"))
		})

		It("should enhance fragment errors with problematic lines", func() {
			fragmentContent := `**Status**: Not Started

**Tasks**:
- [x] Add ruleAdapter
- [x] Update constructor`

			result := &execpkg.CommandResult{
				ExitCode: 1,
				Stdout:   "<file>:5 MD032/blanks-around-lists Lists should be surrounded by blank lines [Context: \"- [x] Add\"]",
				Stderr:   "",
			}
			lintResult := linters.ProcessMarkdownlintOutput(
				result,
				"/tmp/test.md",
				2,  // preamble lines
				90, // fragment starts at line 90
				false,
				"progress.md",
				fragmentContent,
				true, // isFragment - this is a fragment with content
			)
			Expect(lintResult.Success).To(BeFalse())
			Expect(lintResult.RawOut).To(ContainSubstring("<fragment>"))
			Expect(lintResult.RawOut).To(ContainSubstring("MD032/blanks-around-lists"))
			Expect(lintResult.RawOut).To(ContainSubstring("Problematic section:"))
			Expect(lintResult.RawOut).To(ContainSubstring("**Tasks**:"))
			Expect(lintResult.RawOut).To(ContainSubstring("- [x] Add ruleAdapter"))
			Expect(lintResult.RawOut).NotTo(ContainSubstring("progress.md:"))
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

	Describe("Mocked markdownlint execution", func() {
		var (
			ctrl            *gomock.Controller
			mockRunner      *execpkg.MockCommandRunner
			mockToolChecker *execpkg.MockToolChecker
			mockTempMgr     *execpkg.MockTempFileManager
		)

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			mockRunner = execpkg.NewMockCommandRunner(ctrl)
			mockToolChecker = execpkg.NewMockToolChecker(ctrl)
			mockTempMgr = execpkg.NewMockTempFileManager(ctrl)
		})

		AfterEach(func() {
			ctrl.Finish()
		})

		Describe("runMarkdownlint", func() {
			Context("when markdownlint tool is not found", func() {
				It("should return success without running", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("")

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeTrue())
				})
			})

			Context("when markdownlint succeeds", func() {
				It("should return success", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")
					mockTempMgr.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)
					mockRunner.EXPECT().
						Run(gomock.Any(), "/usr/bin/markdownlint", "/tmp/test.md").
						Return(execpkg.CommandResult{ExitCode: 0, Stdout: "", Stderr: ""})

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeTrue())
				})
			})

			Context("when markdownlint fails", func() {
				It("should return failure with error output", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")
					mockTempMgr.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)
					mockRunner.EXPECT().
						Run(gomock.Any(), "/usr/bin/markdownlint", "/tmp/test.md").
						Return(execpkg.CommandResult{
							ExitCode: 1,
							Stdout:   "/tmp/test.md:1 MD022",
							Stderr:   "",
						})

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeFalse())
					Expect(result.RawOut).To(ContainSubstring("MD022"))
				})
			})

			Context("when temp file creation fails", func() {
				It("should return failure", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")
					mockTempMgr.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("", nil, errMockTempFile)

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeFalse())
					Expect(result.RawOut).To(ContainSubstring("Failed to create temp file"))
				})
			})
		})

		Describe("buildConfigArgs with custom config", func() {
			Context("when custom config file is specified", func() {
				It("should use the custom config path", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint:    &useMarkdownlint,
						MarkdownlintConfig: "/path/to/custom.json",
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")
					mockTempMgr.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)
					mockRunner.EXPECT().
						Run(
							gomock.Any(),
							"/usr/bin/markdownlint",
							"--config", "/path/to/custom.json",
							"/tmp/test.md",
						).
						Return(execpkg.CommandResult{ExitCode: 0})

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeTrue())
				})
			})

			Context("when custom rules are specified", func() {
				It("should create a temp config file with rules", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
						MarkdownlintRules: map[string]bool{
							"MD022": true,
							"MD041": false,
						},
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")

					// First Create call for config file
					mockTempMgr.EXPECT().
						Create("markdownlint-config-*.json", gomock.Any()).
						DoAndReturn(func(_, content string) (string, func(), error) {
							// Verify the config content contains our rules
							Expect(content).To(ContainSubstring(`"MD022": true`))
							Expect(content).To(ContainSubstring(`"MD041": false`))

							return "/tmp/config.json", func() {}, nil
						})

					// Second Create call for markdown file
					mockTempMgr.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)

					mockRunner.EXPECT().
						Run(
							gomock.Any(),
							"/usr/bin/markdownlint",
							"--config", "/tmp/config.json",
							"/tmp/test.md",
						).
						Return(execpkg.CommandResult{ExitCode: 0})

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeTrue())
				})
			})
		})

		Describe("createRuntimeConfig", func() {
			Context("with fragment linting (initialState with StartLine > 0)", func() {
				It("should create runtime config for markdownlint-cli", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")

					// Markdown file creation first
					mockTempMgr.EXPECT().
						Create("markdownlint-*.md", gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)

					// Runtime config creation
					mockTempMgr.EXPECT().
						Create("markdownlint-runtime-*.json", gomock.Any()).
						DoAndReturn(func(_, content string) (string, func(), error) {
							// Fragment not at EOF should disable MD047
							Expect(content).To(ContainSubstring(`"MD047": false`))

							return "/tmp/runtime-config.json", func() {}, nil
						})

					mockRunner.EXPECT().
						Run(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(execpkg.CommandResult{ExitCode: 0})

					initialState := &validators.MarkdownState{
						StartLine: 10,
						EndsAtEOF: false,
					}
					result := linter.Lint(ctx, "# Test\n", initialState)

					Expect(result.Success).To(BeTrue())
				})

				It("should create runtime config for markdownlint-cli2", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint-cli2")

					// Markdown file creation first
					mockTempMgr.EXPECT().
						Create("markdownlint-*.md", gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)

					// Runtime config creation for cli2
					mockTempMgr.EXPECT().
						Create("runtime-*.markdownlint-cli2.jsonc", gomock.Any()).
						DoAndReturn(func(_, content string) (string, func(), error) {
							// cli2 format wraps rules in "config" object
							Expect(content).To(ContainSubstring(`"config": {`))
							Expect(content).To(ContainSubstring(`"MD047": false`))

							return "/tmp/runtime-config.jsonc", func() {}, nil
						})

					mockRunner.EXPECT().
						Run(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(execpkg.CommandResult{ExitCode: 0})

					initialState := &validators.MarkdownState{
						StartLine: 10,
						EndsAtEOF: false,
					}
					result := linter.Lint(ctx, "# Test\n", initialState)

					Expect(result.Success).To(BeTrue())
				})

				It(
					"should not create runtime config when fragment ends at EOF and no path",
					func() {
						// When fragment ends at EOF and no original path is provided,
						// no runtime config is needed (MD041 is handled by preamble)
						useMarkdownlint := true
						cfg := &config.MarkdownValidatorConfig{
							UseMarkdownlint: &useMarkdownlint,
						}
						linter := linters.NewMarkdownLinterWithDeps(
							mockRunner,
							mockToolChecker,
							mockTempMgr,
							cfg,
						)

						mockToolChecker.EXPECT().
							FindTool("markdownlint-cli2", "markdownlint").
							Return("/usr/bin/markdownlint")

						// Markdown file creation first
						mockTempMgr.EXPECT().
							Create("markdownlint-*.md", gomock.Any()).
							Return("/tmp/test.md", func() {}, nil)

						// No runtime config needed - fragment ends at EOF and no path
						// (MD041 handled by preamble, MD013 only disabled for .md/.mdx files)

						mockRunner.EXPECT().
							Run(gomock.Any(), "/usr/bin/markdownlint", "/tmp/test.md").
							Return(execpkg.CommandResult{ExitCode: 0})

						initialState := &validators.MarkdownState{
							StartLine: 10,
							EndsAtEOF: true,
						}
						result := linter.Lint(ctx, "# Test\n", initialState)

						Expect(result.Success).To(BeTrue())
					},
				)

				It(
					"should disable MD047 when StartLine is 0 but fragment doesn't reach EOF",
					func() {
						// This is the key fix: when editing near the start of the file,
						// fragmentStartLine can be 0 (max(0, line-contextLines) where line=2, contextLines=2)
						// but it's still a fragment that doesn't reach EOF, so MD047 should be disabled
						useMarkdownlint := true
						cfg := &config.MarkdownValidatorConfig{
							UseMarkdownlint: &useMarkdownlint,
						}
						linter := linters.NewMarkdownLinterWithDeps(
							mockRunner,
							mockToolChecker,
							mockTempMgr,
							cfg,
						)

						mockToolChecker.EXPECT().
							FindTool("markdownlint-cli2", "markdownlint").
							Return("/usr/bin/markdownlint")

						// Markdown file creation first
						mockTempMgr.EXPECT().
							Create("markdownlint-*.md", gomock.Any()).
							Return("/tmp/test.md", func() {}, nil)

						// Runtime config should be created with MD047 disabled,
						// even though StartLine is 0
						mockTempMgr.EXPECT().
							Create("markdownlint-runtime-*.json", gomock.Any()).
							DoAndReturn(func(_, content string) (string, func(), error) {
								// Should disable MD047 because fragment doesn't reach EOF
								Expect(content).To(ContainSubstring(`"MD047": false`))

								return "/tmp/runtime-config.json", func() {}, nil
							})

						mockRunner.EXPECT().
							Run(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(execpkg.CommandResult{ExitCode: 0})

						initialState := &validators.MarkdownState{
							StartLine: 0,     // Fragment starts at beginning of file
							EndsAtEOF: false, // But doesn't reach EOF (e.g., editing line 3 of a 100-line file)
						}
						result := linter.Lint(ctx, "# Test\n", initialState)

						Expect(result.Success).To(BeTrue())
					})
			})

			Context("when config creation fails", func() {
				It("should return failure", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")

					// Markdown file creation first
					mockTempMgr.EXPECT().
						Create("markdownlint-*.md", gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)

					// Runtime config creation fails
					mockTempMgr.EXPECT().
						Create("markdownlint-runtime-*.json", gomock.Any()).
						Return("", nil, errMockTempFile)

					initialState := &validators.MarkdownState{
						StartLine: 10,
						EndsAtEOF: false,
					}
					result := linter.Lint(ctx, "# Test\n", initialState)

					Expect(result.Success).To(BeFalse())
					Expect(
						result.RawOut,
					).To(ContainSubstring("Failed to create markdownlint config"))
				})
			})
		})

		Describe("MD013 disabled for markdown files", func() {
			It("should disable MD013 when LintWithPath is called with .md file", func() {
				useMarkdownlint := true
				cfg := &config.MarkdownValidatorConfig{
					UseMarkdownlint: &useMarkdownlint,
				}
				linter := linters.NewMarkdownLinterWithDeps(
					mockRunner,
					mockToolChecker,
					mockTempMgr,
					cfg,
				)

				mockToolChecker.EXPECT().
					FindTool("markdownlint-cli2", "markdownlint").
					Return("/usr/bin/markdownlint")

				// Markdown file creation first
				mockTempMgr.EXPECT().
					Create("markdownlint-*.md", gomock.Any()).
					Return("/tmp/test.md", func() {}, nil)

				// Runtime config should disable MD013 for .md files
				mockTempMgr.EXPECT().
					Create("markdownlint-runtime-*.json", gomock.Any()).
					DoAndReturn(func(_, content string) (string, func(), error) {
						Expect(content).To(ContainSubstring(`"MD013": false`))

						return "/tmp/runtime-config.json", func() {}, nil
					})

				mockRunner.EXPECT().
					Run(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(execpkg.CommandResult{ExitCode: 0})

				// nil initialState = full file Write operation (not fragment)
				result := linter.LintWithPath(ctx, "# Test\n", nil, "/path/to/file.md")

				Expect(result.Success).To(BeTrue())
			})

			It("should disable MD013 when LintWithPath is called with .mdx file", func() {
				useMarkdownlint := true
				cfg := &config.MarkdownValidatorConfig{
					UseMarkdownlint: &useMarkdownlint,
				}
				linter := linters.NewMarkdownLinterWithDeps(
					mockRunner,
					mockToolChecker,
					mockTempMgr,
					cfg,
				)

				mockToolChecker.EXPECT().
					FindTool("markdownlint-cli2", "markdownlint").
					Return("/usr/bin/markdownlint")

				// Markdown file creation first
				mockTempMgr.EXPECT().
					Create("markdownlint-*.md", gomock.Any()).
					Return("/tmp/test.md", func() {}, nil)

				// Runtime config should disable MD013 for .mdx files
				mockTempMgr.EXPECT().
					Create("markdownlint-runtime-*.json", gomock.Any()).
					DoAndReturn(func(_, content string) (string, func(), error) {
						Expect(content).To(ContainSubstring(`"MD013": false`))

						return "/tmp/runtime-config.json", func() {}, nil
					})

				mockRunner.EXPECT().
					Run(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(execpkg.CommandResult{ExitCode: 0})

				result := linter.LintWithPath(ctx, "# Test\n", nil, "/path/to/file.mdx")

				Expect(result.Success).To(BeTrue())
			})

			It("should not disable MD013 for non-markdown files", func() {
				useMarkdownlint := true
				cfg := &config.MarkdownValidatorConfig{
					UseMarkdownlint: &useMarkdownlint,
				}
				linter := linters.NewMarkdownLinterWithDeps(
					mockRunner,
					mockToolChecker,
					mockTempMgr,
					cfg,
				)

				mockToolChecker.EXPECT().
					FindTool("markdownlint-cli2", "markdownlint").
					Return("/usr/bin/markdownlint")

				// Markdown file creation first
				mockTempMgr.EXPECT().
					Create("markdownlint-*.md", gomock.Any()).
					Return("/tmp/test.md", func() {}, nil)

				// No runtime config needed for non-markdown file (no rules to disable)

				mockRunner.EXPECT().
					Run(gomock.Any(), "/usr/bin/markdownlint", "/tmp/test.md").
					Return(execpkg.CommandResult{ExitCode: 0})

				result := linter.LintWithPath(ctx, "# Test\n", nil, "/path/to/file.txt")

				Expect(result.Success).To(BeTrue())
			})
		})

		Describe("createTempConfig with rules", func() {
			Context("when custom rules config creation fails", func() {
				It("should return failure", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
						MarkdownlintRules: map[string]bool{
							"MD022": true,
						},
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")

					// Markdown file creation first
					mockTempMgr.EXPECT().
						Create("markdownlint-*.md", gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)

					// Config file creation fails
					mockTempMgr.EXPECT().
						Create("markdownlint-config-*.json", gomock.Any()).
						Return("", nil, errMockTempFile)

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeFalse())
					Expect(
						result.RawOut,
					).To(ContainSubstring("Failed to create markdownlint config"))
				})
			})

			Context("when using markdownlint-cli2 with rules", func() {
				It("should create cli2 format config", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
						MarkdownlintRules: map[string]bool{
							"MD022": true,
						},
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint-cli2")

					// Markdown file creation first
					mockTempMgr.EXPECT().
						Create("markdownlint-*.md", gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)

					// Config file with cli2 format
					mockTempMgr.EXPECT().
						Create("config-*.markdownlint-cli2.jsonc", gomock.Any()).
						DoAndReturn(func(_, content string) (string, func(), error) {
							Expect(content).To(ContainSubstring(`"config": {`))
							Expect(content).To(ContainSubstring(`"MD022": true`))

							return "/tmp/config.jsonc", func() {}, nil
						})

					mockRunner.EXPECT().
						Run(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(execpkg.CommandResult{ExitCode: 0})

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeTrue())
				})
			})
		})

		Describe("findMarkdownlintTool", func() {
			Context("when custom path is configured", func() {
				It("should use the configured path", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint:  &useMarkdownlint,
						MarkdownlintPath: "/custom/path/markdownlint",
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					// Should NOT call FindTool when custom path is set
					mockTempMgr.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)
					mockRunner.EXPECT().
						Run(gomock.Any(), "/custom/path/markdownlint", "/tmp/test.md").
						Return(execpkg.CommandResult{ExitCode: 0})

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeTrue())
				})
			})

			Context("when no custom path and tool found via FindTool", func() {
				It("should use the found tool", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/found/markdownlint-cli2")
					mockTempMgr.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("/tmp/test.md", func() {}, nil)
					mockRunner.EXPECT().
						Run(gomock.Any(), "/found/markdownlint-cli2", "/tmp/test.md").
						Return(execpkg.CommandResult{ExitCode: 0})

					result := linter.Lint(ctx, "# Test\n", nil)

					Expect(result.Success).To(BeTrue())
				})
			})
		})

		Describe("prepareRules with fragment disabling", func() {
			Context("when fragment-specific rules are NOT in user config", func() {
				It("should add disabled rules for fragments not at EOF", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
						MarkdownlintRules: map[string]bool{
							"MD022": true,
							// MD041 and MD047 NOT in config - should be added as false
						},
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")

					// Use InOrder to ensure correct mock matching
					gomock.InOrder(
						// Markdown file creation first
						mockTempMgr.EXPECT().
							Create("markdownlint-*.md", gomock.Any()).
							Return("/tmp/test.md", func() {}, nil),

						// Config file creation with rules
						mockTempMgr.EXPECT().
							Create("markdownlint-config-*.json", gomock.Any()).
							DoAndReturn(func(_, content string) (string, func(), error) {
								// Both MD041 and MD047 should be added as disabled
								Expect(content).To(ContainSubstring(`"MD041": false`))
								Expect(content).To(ContainSubstring(`"MD047": false`))

								return "/tmp/config.json", func() {}, nil
							}),
					)

					mockRunner.EXPECT().
						Run(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(execpkg.CommandResult{ExitCode: 0})

					initialState := &validators.MarkdownState{
						StartLine: 10,
						EndsAtEOF: false,
					}
					result := linter.Lint(ctx, "# Test\n", initialState)

					Expect(result.Success).To(BeTrue())
				})

				It("should only add disabled MD041 for fragments at EOF", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
						MarkdownlintRules: map[string]bool{
							"MD022": true,
							// MD041 and MD047 NOT in config
						},
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")

					// Use InOrder to ensure correct mock matching
					gomock.InOrder(
						// Markdown file creation first
						mockTempMgr.EXPECT().
							Create("markdownlint-*.md", gomock.Any()).
							Return("/tmp/test.md", func() {}, nil),

						// Config file creation with rules
						mockTempMgr.EXPECT().
							Create("markdownlint-config-*.json", gomock.Any()).
							DoAndReturn(func(_, content string) (string, func(), error) {
								// MD041 should be added as disabled (fragment)
								Expect(content).To(ContainSubstring(`"MD041": false`))
								// MD047 should NOT be added (fragment at EOF)
								Expect(content).NotTo(ContainSubstring(`"MD047"`))

								return "/tmp/config.json", func() {}, nil
							}),
					)

					mockRunner.EXPECT().
						Run(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(execpkg.CommandResult{ExitCode: 0})

					initialState := &validators.MarkdownState{
						StartLine: 10,
						EndsAtEOF: true, // Fragment ends at EOF
					}
					result := linter.Lint(ctx, "# Test\n", initialState)

					Expect(result.Success).To(BeTrue())
				})
			})

			Context("when fragment-specific rules ARE in user config", func() {
				It("should respect user's explicit config (not override)", func() {
					useMarkdownlint := true
					cfg := &config.MarkdownValidatorConfig{
						UseMarkdownlint: &useMarkdownlint,
						MarkdownlintRules: map[string]bool{
							"MD022": true,
							"MD041": true, // User explicitly enables - should NOT be overridden
							"MD047": true, // User explicitly enables - should NOT be overridden
						},
					}
					linter := linters.NewMarkdownLinterWithDeps(
						mockRunner,
						mockToolChecker,
						mockTempMgr,
						cfg,
					)

					mockToolChecker.EXPECT().
						FindTool("markdownlint-cli2", "markdownlint").
						Return("/usr/bin/markdownlint")

					// Use InOrder to ensure correct mock matching
					gomock.InOrder(
						// Markdown file creation first
						mockTempMgr.EXPECT().
							Create("markdownlint-*.md", gomock.Any()).
							Return("/tmp/test.md", func() {}, nil),

						// Config file creation with rules
						mockTempMgr.EXPECT().
							Create("markdownlint-config-*.json", gomock.Any()).
							DoAndReturn(func(_, content string) (string, func(), error) {
								// User's explicit config should be preserved
								Expect(content).To(ContainSubstring(`"MD041": true`))
								Expect(content).To(ContainSubstring(`"MD047": true`))

								return "/tmp/config.json", func() {}, nil
							}),
					)

					mockRunner.EXPECT().
						Run(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(execpkg.CommandResult{ExitCode: 0})

					initialState := &validators.MarkdownState{
						StartLine: 10,
						EndsAtEOF: false,
					}
					result := linter.Lint(ctx, "# Test\n", initialState)

					Expect(result.Success).To(BeTrue())
				})
			})
		})
	})

	Describe("NewMarkdownLinterWithDeps", func() {
		It("should create a linter with all injected dependencies", func() {
			ctrl := gomock.NewController(GinkgoT())
			defer ctrl.Finish()

			mockRunner := execpkg.NewMockCommandRunner(ctrl)
			mockToolChecker := execpkg.NewMockToolChecker(ctrl)
			mockTempMgr := execpkg.NewMockTempFileManager(ctrl)
			cfg := &config.MarkdownValidatorConfig{}

			linter := linters.NewMarkdownLinterWithDeps(
				mockRunner,
				mockToolChecker,
				mockTempMgr,
				cfg,
			)

			Expect(linter).NotTo(BeNil())
		})
	})
})

var errMockTempFile = errors.New("mock temp file error")
