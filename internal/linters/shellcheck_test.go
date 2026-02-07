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
	errShellcheckFailed           = errors.New("shellcheck failed")
	errShellcheckTempFileCreation = errors.New("failed to create temp file")
)

var _ = Describe("ShellChecker", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		mockTempManager *execpkg.MockTempFileManager
		checker         linters.ShellChecker
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
		checker = linters.NewShellCheckerWithDeps(contentLinter)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Check", func() {
		Context("when shellcheck is not available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().IsAvailable("shellcheck").Return(false)

				result := checker.Check(ctx, "#!/bin/bash\necho 'hello'")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when shellcheck passes", func() {
			It("should return success", func() {
				scriptContent := "#!/bin/bash\necho 'hello'"

				mockToolChecker.EXPECT().IsAvailable("shellcheck").Return(true)
				mockTempManager.EXPECT().Create("script-*.sh", scriptContent).
					Return("/tmp/script-123.sh", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "shellcheck", "--format=json", "/tmp/script-123.sh").
					Return(execpkg.CommandResult{
						Stdout:   "[]",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when shellcheck fails", func() {
			It("should return failure with output", func() {
				shellcheckOutput := "script.sh:2:1: warning: Use $(...) instead of legacy backticks"
				scriptContent := "#!/bin/bash\nvar=`ls`"

				mockToolChecker.EXPECT().IsAvailable("shellcheck").Return(true)
				mockTempManager.EXPECT().Create("script-*.sh", scriptContent).
					Return("/tmp/script-123.sh", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "shellcheck", "--format=json", "/tmp/script-123.sh").
					Return(execpkg.CommandResult{
						Stdout:   shellcheckOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errShellcheckFailed,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(shellcheckOutput))
				Expect(result.Err).To(Equal(errShellcheckFailed))
			})

			It("should include stderr in output when stdout is empty", func() {
				stderrOutput := "shellcheck: error parsing script"
				scriptContent := "#!/bin/bash\ninvalid syntax"

				mockToolChecker.EXPECT().IsAvailable("shellcheck").Return(true)
				mockTempManager.EXPECT().Create("script-*.sh", scriptContent).
					Return("/tmp/script-123.sh", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "shellcheck", "--format=json", "/tmp/script-123.sh").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errShellcheckFailed,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				scriptContent := "#!/bin/bash\necho 'hello'"

				mockToolChecker.EXPECT().IsAvailable("shellcheck").Return(true)
				mockTempManager.EXPECT().Create("script-*.sh", scriptContent).
					Return("", nil, errShellcheckTempFileCreation)

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(HaveOccurred())
			})
		})
	})
})
