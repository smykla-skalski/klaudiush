package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/github"
	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var errToolExit = errors.New("exit status 2")

var _ = Describe("checks that could not run", func() {
	var (
		ctrl *gomock.Controller
		log  logger.Logger
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		log = logger.NewNoOpLogger()
	})

	write := func(path, content string) *hook.Context {
		return &hook.Context{
			Provider:  hook.ProviderClaude,
			Event:     hook.CanonicalEventBeforeTool,
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{FilePath: path, Content: content},
		}
	}

	expectUnavailable := func(result *validator.Result, reason validator.UnavailableReason) {
		GinkgoHelper()

		Expect(result.Passed).To(BeFalse())
		Expect(result.Unavailable).To(BeTrue())
		Expect(result.UnavailableReason).To(Equal(reason))
		Expect(result.ShouldBlock).To(BeFalse())
		Expect(result.Inspected).To(BeFalse())
	}

	skipped := &linters.LintResult{Success: true, Skipped: true}
	silentFailure := &linters.LintResult{Err: errToolExit}
	passed := &linters.LintResult{Success: true}

	DescribeTable("linter-backed validators",
		func(name, content string, build lintValidator) {
			hc := write(name, content)
			bg := context.Background()

			By("reporting a missing tool")
			expectUnavailable(build(ctrl, skipped).Validate(bg, hc), validator.ReasonMissingTool)

			By("reporting a tool that failed without a finding")
			expectUnavailable(build(ctrl, silentFailure).Validate(bg, hc), validator.ReasonError)

			By("reporting a run cut short instead of its pass")

			canceled, cancel := context.WithCancel(bg)
			cancel()
			expectUnavailable(build(ctrl, passed).Validate(canceled, hc), validator.ReasonCanceled)

			expired, stop := context.WithDeadline(bg, time.Now().Add(-time.Second))
			defer stop()

			expectUnavailable(build(ctrl, passed).Validate(expired, hc), validator.ReasonTimeout)

			By("passing a clean run")
			Expect(build(ctrl, passed).Validate(bg, hc).Passed).To(BeTrue())
		},
		Entry("python", "a.py", "x = 1\n",
			lintValidator(func(ctrl *gomock.Controller, r *linters.LintResult) validator.Validator {
				m := linters.NewMockRuffChecker(ctrl)
				m.EXPECT().CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(r).AnyTimes()

				return file.NewPythonValidator(logger.NewNoOpLogger(), m, nil, nil)
			})),
		Entry("javascript", "a.js", "let x = 1\n",
			lintValidator(func(ctrl *gomock.Controller, r *linters.LintResult) validator.Validator {
				m := linters.NewMockOxlintChecker(ctrl)
				m.EXPECT().CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(r).AnyTimes()

				return file.NewJavaScriptValidator(logger.NewNoOpLogger(), m, nil, nil)
			})),
		Entry("rust", "a.rs", "fn main() {}\n",
			lintValidator(func(ctrl *gomock.Controller, r *linters.LintResult) validator.Validator {
				m := linters.NewMockRustfmtChecker(ctrl)
				m.EXPECT().CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(r).AnyTimes()

				return file.NewRustValidator(logger.NewNoOpLogger(), m, nil, nil)
			})),
		Entry("shell", "a.sh", "#!/bin/bash\necho hi\n",
			lintValidator(func(ctrl *gomock.Controller, r *linters.LintResult) validator.Validator {
				m := linters.NewMockShellChecker(ctrl)
				m.EXPECT().CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(r).AnyTimes()

				return file.NewShellScriptValidator(logger.NewNoOpLogger(), m, nil, nil)
			})),
		Entry("gofumpt", "a.go", "package a\n",
			lintValidator(func(ctrl *gomock.Controller, r *linters.LintResult) validator.Validator {
				m := linters.NewMockGofumptChecker(ctrl)
				m.EXPECT().CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(r).AnyTimes()

				return file.NewGofumptValidator(logger.NewNoOpLogger(), m, nil, nil)
			})),
	)

	Describe("GofumptValidator baseline", func() {
		It("keeps the block when the baseline could not be checked", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "a.go")
			Expect(os.WriteFile(path, []byte("package a\nvar x = 1\n"), 0o600)).To(Succeed())

			unformatted := &linters.LintResult{
				RawOut:   "-a\n+b",
				Findings: []linters.LintFinding{{Message: "diff"}},
			}

			m := linters.NewMockGofumptChecker(ctrl)
			gomock.InOrder(
				m.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(unformatted),
				m.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(skipped),
			)

			hc := &hook.Context{
				Provider:  hook.ProviderClaude,
				Event:     hook.CanonicalEventBeforeTool,
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeEdit,
				ToolInput: hook.ToolInput{FilePath: path, OldString: "x = 1", NewString: "x  =  2"},
			}

			result := file.NewGofumptValidator(log, m, nil, nil).Validate(context.Background(), hc)
			Expect(result.ShouldBlock).To(BeTrue())
		})
	})

	Describe("TerraformValidator", func() {
		build := func(tool string, fmtResult, lintResult *linters.LintResult) *file.TerraformValidator {
			formatter := linters.NewMockTerraformFormatter(ctrl)
			formatter.EXPECT().DetectTool().Return(tool).AnyTimes()
			formatter.EXPECT().CheckFormat(gomock.Any(), gomock.Any()).Return(fmtResult).AnyTimes()

			linter := linters.NewMockTfLinter(ctrl)
			linter.EXPECT().Lint(gomock.Any(), gomock.Any()).Return(lintResult).AnyTimes()

			return file.NewTerraformValidator(
				formatter, linter, log,
				&config.TerraformValidatorConfig{UseTflint: new(true)},
				nil,
			)
		}

		hc := write("main.tf", "locals {}\n")

		It("reports a missing formatter", func() {
			result := build("", passed, passed).Validate(context.Background(), hc)
			expectUnavailable(result, validator.ReasonMissingTool)
			Expect(result.Message).To(ContainSubstring("Neither 'tofu' nor 'terraform'"))
		})

		It("reports a missing tflint", func() {
			result := build("tofu", passed, skipped).Validate(context.Background(), hc)
			expectUnavailable(result, validator.ReasonMissingTool)
		})

		It("reports a formatter that failed without a diff", func() {
			result := build("tofu", &linters.LintResult{RawOut: "boom", Err: errToolExit}, passed).
				Validate(context.Background(), hc)
			expectUnavailable(result, validator.ReasonError)
		})

		It("keeps real findings ahead of an unavailable check", func() {
			findings := &linters.LintResult{RawOut: "main.tf:1:1: Warning - x (rule)"}

			result := build("", passed, findings).Validate(context.Background(), hc)
			Expect(result.Unavailable).To(BeFalse())
			Expect(result.Details["warnings"]).To(ContainSubstring("tflint findings"))
		})
	})

	Describe("WorkflowValidator", func() {
		build := func(r *linters.LintResult) *file.WorkflowValidator {
			linter := linters.NewMockActionLinter(ctrl)
			linter.EXPECT().Lint(gomock.Any(), gomock.Any(), gomock.Any()).Return(r).AnyTimes()

			return file.NewWorkflowValidator(
				linter,
				func() github.Client { return &mockGitHubClient{} },
				log,
				&config.WorkflowValidatorConfig{EnforceDigestPinning: new(false)},
				nil,
			)
		}

		hc := write(".github/workflows/ci.yml", "on: push\njobs: {}\n")

		It("reports a missing actionlint", func() {
			expectUnavailable(
				build(skipped).Validate(context.Background(), hc),
				validator.ReasonMissingTool,
			)
		})

		It("passes when actionlint ran", func() {
			findings := &linters.LintResult{RawOut: "ci.yml:1:1: problem"}
			Expect(build(findings).Validate(context.Background(), hc).Passed).To(BeTrue())
		})
	})

	Describe("MarkdownValidator", func() {
		It("reports a check cut short", func() {
			v := file.NewMarkdownValidator(
				nil,
				linters.NewMarkdownLinter(execpkg.NewCommandRunner(10*time.Second)),
				log,
				nil,
			)

			canceled, cancel := context.WithCancel(context.Background())
			cancel()

			expectUnavailable(
				v.Validate(canceled, write("a.md", "# Title\n")),
				validator.ReasonCanceled,
		withLintResult := func(r *linters.LintResult) *file.MarkdownValidator {
			linter := linters.NewMockMarkdownLinter(ctrl)
			linter.EXPECT().
				LintWithPath(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(r).AnyTimes()

			return file.NewMarkdownValidator(nil, linter, log, nil)
		}

		It("reports a missing markdownlint", func() {
			result := withLintResult(skipped).
				Validate(context.Background(), write("a.md", "# Title\n"))
			expectUnavailable(result, validator.ReasonMissingTool)
			Expect(result.Message).To(ContainSubstring("markdownlint is not installed"))
		})

		It("keeps built-in findings ahead of a missing markdownlint", func() {
			result := withLintResult(&linters.LintResult{
				Skipped: true,
				RawOut:  "a.md:3: Code block should be preceded by empty line",
				Err:     linters.ErrMarkdownCustomRules,
			}).Validate(context.Background(), write("a.md", "# Title\n"))

			Expect(result.Unavailable).To(BeFalse())
			Expect(result.Passed).To(BeFalse())
		})

			)
		})
	})
})
