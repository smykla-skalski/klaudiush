package github_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validators/github"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("IssueValidator", func() {
	var (
		validator  *github.IssueValidator
		mockCtrl   *gomock.Controller
		mockLinter *linters.MockMarkdownLinter
		ctx        context.Context
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockLinter = linters.NewMockMarkdownLinter(mockCtrl)
		validator = github.NewIssueValidator(nil, mockLinter, logger.NewNoOpLogger(), nil)
		ctx = context.Background()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("Validate", func() {
		It("should pass for command without body", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug report"`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for non-gh commands", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -m "test"`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should validate body with --body flag", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug report" --body "### Description

This is a bug description.

### Steps to Reproduce

1. Step one
2. Step two
"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should validate body with heredoc", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug report" --body "$(cat <<'EOF'
### Description

This is a bug description.

### Steps to Reproduce

1. Step one
2. Step two
EOF
)"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should warn for markdown errors", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug report" --body "### Description
No empty line after heading"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "MD022 Headings should be surrounded by blank lines",
				})

			result := validator.Validate(ctx, hookCtx)
			// By default, issue validator only warns
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("markdown validation"))
		})
	})

	Describe("with RequireBody enabled", func() {
		BeforeEach(func() {
			requireBody := true
			cfg := &config.IssueValidatorConfig{
				RequireBody: &requireBody,
			}
			validator = github.NewIssueValidator(cfg, mockLinter, logger.NewNoOpLogger(), nil)
		})

		It("should fail when body is missing", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug report"`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(ContainSubstring("Issue body is required"))
		})
	})

	Describe("extractIssueData", func() {
		It("should extract title from double quotes", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "My bug report"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true}).
				AnyTimes()

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should extract title from single quotes", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title 'My bug report'`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true}).
				AnyTimes()

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should extract body from double quotes", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body "Body content here"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should extract body from single quotes", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body 'Body content here'`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("filterDisabledRules", func() {
		It("should filter out disabled rules from output", func() {
			output := `file.md:1 MD013 Line too long
file.md:3 MD022 Headings should be surrounded by blank lines
file.md:5 MD041 First line in a file should be a top-level heading`

			filtered := github.FilterDisabledRules(output, []string{"MD013", "MD041"})

			Expect(filtered).To(ContainSubstring("MD022"))
			Expect(filtered).NotTo(ContainSubstring("MD013"))
			Expect(filtered).NotTo(ContainSubstring("MD041"))
		})

		It("should return original output when no rules disabled", func() {
			output := "file.md:1 MD022 Headings should be surrounded by blank lines"

			filtered := github.FilterDisabledRules(output, []string{})

			Expect(filtered).To(Equal(output))
		})
	})

	Describe("Category", func() {
		It("should return CategoryIO", func() {
			Expect(validator.Category()).To(Equal(github.CategoryIO))
		})
	})

	Describe("with custom timeout configuration", func() {
		BeforeEach(func() {
			cfg := &config.IssueValidatorConfig{
				Timeout: config.Duration(5 * time.Second),
			}
			validator = github.NewIssueValidator(cfg, mockLinter, logger.NewNoOpLogger(), nil)
		})

		It("should use custom timeout", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body "Test body"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("with custom markdown disabled rules", func() {
		BeforeEach(func() {
			cfg := &config.IssueValidatorConfig{
				MarkdownDisabledRules: []string{"MD001", "MD002"},
			}
			validator = github.NewIssueValidator(cfg, mockLinter, logger.NewNoOpLogger(), nil)
		})

		It("should use custom disabled rules", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body "Test body"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "file.md:1 MD001 Heading level\nfile.md:2 MD022 Blank line",
				})

			result := validator.Validate(ctx, hookCtx)
			// Should filter out MD001 but keep MD022
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("MD022"))
			Expect(result.Message).NotTo(ContainSubstring("MD001"))
		})
	})

	Describe("with body-file flag", func() {
		var tempDir string

		BeforeEach(func() {
			var err error

			tempDir, err = os.MkdirTemp("", "issue-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		It("should read body from file with double quotes", func() {
			bodyFile := filepath.Join(tempDir, "body.md")
			err := os.WriteFile(bodyFile, []byte("### Description\n\nTest body content"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body-file "` + bodyFile + `"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should read body from file with single quotes", func() {
			bodyFile := filepath.Join(tempDir, "body.md")
			err := os.WriteFile(bodyFile, []byte("### Description\n\nTest body content"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body-file '` + bodyFile + `'`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should read body from file without quotes", func() {
			bodyFile := filepath.Join(tempDir, "body.md")
			err := os.WriteFile(bodyFile, []byte("### Description\n\nTest body content"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body-file ` + bodyFile,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should handle non-existent body file gracefully", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body-file "/nonexistent/path/body.md"`,
				},
			}

			// No body content, so no linter call
			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("with rule adapter", func() {
		var ruleAdapter *rules.RuleValidatorAdapter

		BeforeEach(func() {
			rulesList := []*rules.Rule{
				{
					Name:    "block-all-issues",
					Enabled: true,
					Match:   &rules.RuleMatch{ValidatorType: rules.ValidatorGitHubIssue},
					Action:  &rules.RuleAction{Type: rules.ActionBlock, Message: "Blocked by rule"},
				},
			}
			engine, _ := rules.NewRuleEngine(rulesList)
			ruleAdapter = rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitHubIssue)
			validator = github.NewIssueValidator(
				nil,
				mockLinter,
				logger.NewNoOpLogger(),
				ruleAdapter,
			)
		})

		It("should apply rules before validation", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body "Test"`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(ContainSubstring("Blocked by rule"))
		})
	})

	Describe("without linter", func() {
		BeforeEach(func() {
			validator = github.NewIssueValidator(nil, nil, logger.NewNoOpLogger(), nil)
		})

		It("should validate without calling linter", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "Bug" --body "### Heading

Content after heading"`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("edge cases in command parsing", func() {
		It("should handle gh command with insufficient args", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should handle gh with different subcommand", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "PR" --body "Content"`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should handle issue with different operation", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue list`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should warn on parse error", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "unterminated`,
				},
			}

			result := validator.Validate(ctx, hookCtx)
			// Parse errors result in warnings
			Expect(result.ShouldBlock).To(BeFalse())
		})
	})

	Describe("buildResult with errors", func() {
		It("should include title in error message when present", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh issue create --title "My Bug Report" --body "##BadHeading

Missing blank line after heading"`,
				},
			}

			mockLinter.EXPECT().
				Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := validator.Validate(ctx, hookCtx)
			// Internal validation catches heading spacing issues
			Expect(result.ShouldBlock).To(BeFalse())
		})
	})
})
