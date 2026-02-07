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

var errTfLintFailed = errors.New("tflint failed")

var _ = Describe("TfLinter", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		linter          linters.TfLinter
		ctx             context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockRunner = execpkg.NewMockCommandRunner(ctrl)
		mockToolChecker = execpkg.NewMockToolChecker(ctrl)
		ctx = context.Background()
		linter = linters.NewTfLinterWithDeps(mockRunner, mockToolChecker)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Lint", func() {
		Context("when tflint is not available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().IsAvailable("tflint").Return(false)

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when tflint succeeds with no findings", func() {
			It("should return success", func() {
				mockToolChecker.EXPECT().IsAvailable("tflint").Return(true)
				mockRunner.EXPECT().Run(ctx, "tflint", "--format=compact", "main.tf").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
					})

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when tflint finds issues", func() {
			It("should return failure with findings", func() {
				compactOutput := `main.tf:3:1: Warning - Missing version constraint for provider "aws" (terraform_required_providers)
main.tf:10:5: Error - "instance_type" is a required field (aws_instance_invalid_type)`

				mockToolChecker.EXPECT().IsAvailable("tflint").Return(true)
				mockRunner.EXPECT().Run(ctx, "tflint", "--format=compact", "main.tf").
					Return(execpkg.CommandResult{
						Stdout:   compactOutput,
						Stderr:   "",
						ExitCode: 2,
						Err:      errTfLintFailed,
					})

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(compactOutput))
				Expect(result.Err).To(Equal(errTfLintFailed))
			})

			It("should use stderr if stdout is empty", func() {
				stderrOutput := "tflint: error parsing configuration"

				mockToolChecker.EXPECT().IsAvailable("tflint").Return(true)
				mockRunner.EXPECT().Run(ctx, "tflint", "--format=compact", "main.tf").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errTfLintFailed,
					})

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when tflint command fails with no output", func() {
			It("should return error", func() {
				mockToolChecker.EXPECT().IsAvailable("tflint").Return(true)
				mockRunner.EXPECT().Run(ctx, "tflint", "--format=compact", "main.tf").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 127,
						Err:      errTfLintFailed,
					})

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(Equal(errTfLintFailed))
			})
		})
	})
})
