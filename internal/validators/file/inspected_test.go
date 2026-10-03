package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

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

// lintValidator builds a validator whose linter always returns result.
type lintValidator func(ctrl *gomock.Controller, result *linters.LintResult) validator.Validator

var _ = Describe("whole-file inspection after the tool ran", func() {
	var (
		ctrl *gomock.Controller
		dir  string
		log  logger.Logger
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		dir = GinkgoT().TempDir()
		log = logger.NewNoOpLogger()
	})

	hookCtx := func(event hook.CanonicalEvent, path string) *hook.Context {
		return &hook.Context{
			Provider:   hook.ProviderClaude,
			Event:      event,
			ToolName:   hook.ToolTypeWrite,
			WorkingDir: dir,
			ToolInput:  hook.ToolInput{FilePath: path, Content: "input"},
		}
	}

	passed := &linters.LintResult{Success: true}
	failed := &linters.LintResult{
		RawOut:   "x:1:1: problem",
		Findings: []linters.LintFinding{{Line: 1, Message: "problem"}},
	}
	skipped := &linters.LintResult{Success: true, Skipped: true}

	DescribeTable("linter-backed validators",
		func(name, content string, build lintValidator) {
			Expect(os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)).To(Succeed())

			validate := func(
				ctx context.Context,
				result *linters.LintResult,
				hc *hook.Context,
			) *validator.Result {
				return build(ctrl, result).Validate(ctx, hc)
			}

			after := hookCtx(hook.CanonicalEventAfterTool, name)
			bg := context.Background()

			By("counting a clean or failing run on the file on disk")
			Expect(validate(bg, passed, after).Inspected).To(BeTrue())

			result := validate(bg, failed, after)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Inspected).To(BeTrue())

			By("not counting a run when the tool is missing")
			Expect(validate(bg, skipped, after).Inspected).To(BeFalse())

			By("not counting a run cut short")

			cancelled, cancel := context.WithCancel(bg)
			cancel()
			Expect(validate(cancelled, passed, after).Inspected).To(BeFalse())

			By("not counting a file it could not read")

			missing := hookCtx(hook.CanonicalEventAfterTool, "missing"+filepath.Ext(name))
			result = validate(bg, passed, missing)
			Expect(result.Passed).To(BeTrue())
			Expect(result.Inspected).To(BeFalse())

			By("counting a Write's whole content before the tool ran only as proposed")

			before := hookCtx(hook.CanonicalEventBeforeTool, name)
			result = validate(bg, passed, before)
			Expect(result.Inspected).To(BeFalse())
			Expect(result.Proposed).To(BeTrue())
			Expect(validate(bg, failed, before).Proposed).To(BeTrue())
			Expect(validate(bg, skipped, before).Proposed).To(BeFalse())
			Expect(validate(cancelled, passed, before).Proposed).To(BeFalse())
			Expect(validate(bg, passed, after).Proposed).To(BeFalse())

			By("not counting an edit fragment as the whole file")

			edit := hookCtx(hook.CanonicalEventBeforeTool, name)
			edit.EventType = hook.EventTypePreToolUse
			edit.ToolName = hook.ToolTypeEdit
			edit.ToolInput = hook.ToolInput{
				FilePath:  filepath.Join(dir, name),
				OldString: content,
				NewString: content + content,
			}
			result = validate(bg, passed, edit)
			Expect(result.Inspected).To(BeFalse())
			Expect(result.Proposed).To(BeFalse())
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
	)

	Describe("GofumptValidator", func() {
		build := func(r *linters.LintResult) *file.GofumptValidator {
			m := linters.NewMockGofumptChecker(ctrl)
			m.EXPECT().CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(r).AnyTimes()

			return file.NewGofumptValidator(log, m, nil, nil)
		}

		It("counts only runs that finished with a verdict", func() {
			Expect(os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o600)).
				To(Succeed())

			after := hookCtx(hook.CanonicalEventAfterTool, "a.go")

			Expect(build(passed).Validate(context.Background(), after).Inspected).To(BeTrue())
			Expect(build(failed).Validate(context.Background(), after).Inspected).To(BeTrue())
			Expect(build(skipped).Validate(context.Background(), after).Inspected).To(BeFalse())

			crashed := &linters.LintResult{RawOut: "timeout"}
			result := build(crashed).Validate(context.Background(), after)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Inspected).To(BeFalse())

			missing := hookCtx(hook.CanonicalEventAfterTool, "missing.go")
			Expect(build(passed).Validate(context.Background(), missing).Inspected).To(BeFalse())
		})
	})

	Describe("TerraformValidator", func() {
		build := func(tool string, fmtResult, lintResult *linters.LintResult) *file.TerraformValidator {
			formatter := linters.NewMockTerraformFormatter(ctrl)
			formatter.EXPECT().DetectTool().Return(tool).AnyTimes()
			formatter.EXPECT().CheckFormat(gomock.Any(), gomock.Any()).Return(fmtResult).AnyTimes()

			linter := linters.NewMockTfLinter(ctrl)
			linter.EXPECT().Lint(gomock.Any(), gomock.Any()).Return(lintResult).AnyTimes()

			cfg := &config.TerraformValidatorConfig{UseTflint: new(true)}

			return file.NewTerraformValidator(formatter, linter, log, cfg, nil)
		}

		It("counts a run only when every enabled check ran", func() {
			Expect(os.WriteFile(filepath.Join(dir, "main.tf"), []byte("locals {}\n"), 0o600)).
				To(Succeed())

			after := hookCtx(hook.CanonicalEventAfterTool, "main.tf")
			validate := func(v *file.TerraformValidator) *validator.Result {
				return v.Validate(context.Background(), after)
			}

			tflintFindings := &linters.LintResult{RawOut: "main.tf:1:1: Warning - x (rule)"}
			fmtDiff := &linters.LintResult{
				RawOut:   "-a\n+b",
				Findings: []linters.LintFinding{{Message: "diff"}},
			}
			broken := &linters.LintResult{Err: context.DeadlineExceeded}

			Expect(validate(build("tofu", passed, passed)).Inspected).To(BeTrue())
			Expect(validate(build("tofu", fmtDiff, tflintFindings)).Inspected).To(BeTrue())

			Expect(validate(build("", passed, passed)).Inspected).To(BeFalse())
			Expect(validate(build("tofu", skipped, passed)).Inspected).To(BeFalse())
			Expect(validate(build("tofu", broken, passed)).Inspected).To(BeFalse())
			Expect(validate(build("tofu", passed, skipped)).Inspected).To(BeFalse())
			Expect(validate(build("tofu", passed, broken)).Inspected).To(BeFalse())
			Expect(validate(build("tofu", &linters.LintResult{}, passed)).Inspected).To(BeFalse())
		})

		It("counts an empty file as checked", func() {
			Expect(os.WriteFile(filepath.Join(dir, "main.tf"), nil, 0o600)).To(Succeed())

			result := build("tofu", passed, passed).
				Validate(context.Background(), hookCtx(hook.CanonicalEventAfterTool, "main.tf"))
			Expect(result.Inspected).To(BeTrue())
		})
	})

	Describe("MarkdownValidator", func() {
		build := func() *file.MarkdownValidator {
			return file.NewMarkdownValidator(
				nil,
				linters.NewMarkdownLinter(execpkg.NewCommandRunner(10*time.Second)),
				log,
				nil,
			)
		}

		It("counts a read of the file on disk, empty or not", func() {
			Expect(os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Title\n"), 0o600)).
				To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "empty.md"), nil, 0o600)).To(Succeed())

			for _, name := range []string{"a.md", "empty.md"} {
				result := build().Validate(
					context.Background(),
					hookCtx(hook.CanonicalEventAfterTool, name),
				)
				Expect(result.Inspected).To(BeTrue(), name)
			}

			missing := hookCtx(hook.CanonicalEventAfterTool, "missing.md")
			Expect(build().Validate(context.Background(), missing).Inspected).To(BeFalse())
		})
	})

	Describe("pattern validators", func() {
		It("count a Write's whole content, not an edit fragment", func() {
			validators := []validator.Validator{
				file.NewLinterIgnoreValidator(log, nil, nil),
				file.NewAICommentValidator(log, nil, nil),
			}

			write := hookCtx(hook.CanonicalEventBeforeTool, "a.py")
			edit := &hook.Context{
				Provider:  hook.ProviderClaude,
				Event:     hook.CanonicalEventBeforeTool,
				ToolName:  hook.ToolTypeEdit,
				ToolInput: hook.ToolInput{FilePath: "a.py", OldString: "a", NewString: "b"},
			}

			for _, v := range validators {
				Expect(v.Validate(context.Background(), write).Proposed).To(BeTrue(), v.Name())

				Expect(v.Validate(context.Background(), edit).Proposed).To(BeFalse(), v.Name())
			}

			write.ToolInput.Content = "x = 1  # no" + "qa\n"
			result := validators[0].Validate(context.Background(), write)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Proposed).To(BeTrue())

			write.ToolInput.Content = "x = 1\n\n\n# This is a comment that explains\nx = 2\n"
			result = file.NewAICommentValidator(log, &config.AICommentValidatorConfig{
				Mode: config.AICommentModeStrict,
			}, nil).Validate(context.Background(), write)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Proposed).To(BeTrue())
		})
	})

	Describe("MarkdownValidator before the tool ran", func() {
		It("counts a Write's whole content as proposed", func() {
			v := file.NewMarkdownValidator(
				nil,
				linters.NewMarkdownLinter(execpkg.NewCommandRunner(10*time.Second)),
				log,
				nil,
			)

			result := v.Validate(context.Background(), &hook.Context{
				Provider:  hook.ProviderClaude,
				Event:     hook.CanonicalEventBeforeTool,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{FilePath: "a.md", Content: "# Title\n"},
			})
			Expect(result.Inspected).To(BeFalse())
			Expect(result.Proposed).To(BeTrue())
		})
	})

	Describe("WorkflowValidator", func() {
		It("counts a read of the file on disk", func() {
			path := filepath.Join(".github", "workflows", "ci.yml")
			Expect(os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o700)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(dir, path),
				[]byte("on: push\njobs:\n  a:\n    steps:\n      - uses: actions/checkout@v4\n"),
				0o600,
			)).To(Succeed())

			linter := linters.NewMockActionLinter(ctrl)
			linter.EXPECT().Lint(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(passed).AnyTimes()

			v := file.NewWorkflowValidator(
				linter,
				func() github.Client { return &mockGitHubClient{} },
				log,
				nil,
				nil,
			)

			result := v.Validate(context.Background(), hookCtx(hook.CanonicalEventAfterTool, path))
			Expect(result.Passed).To(BeFalse())
			Expect(result.Inspected).To(BeTrue())

			missing := hookCtx(
				hook.CanonicalEventAfterTool,
				filepath.Join(".github", "workflows", "gone.yml"),
			)
			Expect(v.Validate(context.Background(), missing).Inspected).To(BeFalse())
		})
	})
})
