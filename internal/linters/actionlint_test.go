package linters_test

import (
	"context"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
)

var (
	errActionLintFailed           = errors.New("actionlint failed")
	errActionLintTempFileCreation = errors.New("failed to create temp file")
)

var _ = Describe("ActionLinter", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		mockTempManager *execpkg.MockTempFileManager
		linter          linters.ActionLinter
		ctx             context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockRunner = execpkg.NewMockCommandRunner(ctrl)
		mockToolChecker = execpkg.NewMockToolChecker(ctrl)
		mockTempManager = execpkg.NewMockTempFileManager(ctrl)
		ctx = context.Background()

		contentLinter := linters.NewContentLinterWithDeps(
			mockRunner,
			mockToolChecker,
			mockTempManager,
		)
		linter = linters.NewActionLinterWithDeps(contentLinter)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Lint", func() {
		Context("when actionlint is not available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().IsAvailable("actionlint").Return(false)

				result := linter.Lint(ctx, "workflow content", ".github/workflows/test.yml")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when actionlint succeeds with no issues", func() {
			It("should return success", func() {
				workflowContent := `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4`

				mockToolChecker.EXPECT().IsAvailable("actionlint").Return(true)
				mockTempManager.EXPECT().Create("workflow-*.yml", workflowContent).
					Return("/tmp/workflow-123.yml", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "actionlint", "-no-color", "/tmp/workflow-123.yml").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
					})

				result := linter.Lint(ctx, workflowContent, ".github/workflows/test.yml")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when actionlint finds issues", func() {
			It("should return failure with output", func() {
				actionlintOutput := `.github/workflows/test.yml:10:9: property "run-on" is not defined in object type {runs-on: string} [syntax-check]
.github/workflows/test.yml:12:15: undefined variable "secrets.INVALID" [expression]`
				workflowContent := "workflow content"

				mockToolChecker.EXPECT().IsAvailable("actionlint").Return(true)
				mockTempManager.EXPECT().Create("workflow-*.yml", workflowContent).
					Return("/tmp/workflow-123.yml", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "actionlint", "-no-color", "/tmp/workflow-123.yml").
					Return(execpkg.CommandResult{
						Stdout:   actionlintOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errActionLintFailed,
					})

				result := linter.Lint(ctx, workflowContent, ".github/workflows/test.yml")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(ContainSubstring("property \"run-on\" is not defined"))
				Expect(result.Err).To(Equal(errActionLintFailed))
			})

			It("should include stderr when stdout is empty", func() {
				stderrOutput := "actionlint: error parsing workflow file"
				workflowContent := "invalid yaml"

				mockToolChecker.EXPECT().IsAvailable("actionlint").Return(true)
				mockTempManager.EXPECT().Create("workflow-*.yml", workflowContent).
					Return("/tmp/workflow-123.yml", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "actionlint", "-no-color", "/tmp/workflow-123.yml").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errActionLintFailed,
					})

				result := linter.Lint(ctx, workflowContent, ".github/workflows/test.yml")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				workflowContent := "workflow content"

				mockToolChecker.EXPECT().IsAvailable("actionlint").Return(true)
				mockTempManager.EXPECT().Create("workflow-*.yml", workflowContent).
					Return("", nil, errActionLintTempFileCreation)

				result := linter.Lint(ctx, workflowContent, ".github/workflows/test.yml")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(HaveOccurred())
			})
		})
	})
})
