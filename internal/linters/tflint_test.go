package linters_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
	"github.com/smykla-labs/claude-hooks/internal/linters"
)

var errTfLintFailed = errors.New("tflint failed")

var _ = Describe("TfLinter", func() {
	var (
		linter     linters.TfLinter
		mockRunner *mockCommandRunner
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockRunner = &mockCommandRunner{}
		linter = linters.NewTfLinter(mockRunner)
	})

	Describe("Lint", func() {
		Context("when tflint is not available", func() {
			It("should return success without validation", func() {
				// When tflint is not in PATH, IsAvailable returns false
				// This requires system PATH modification, so we skip
				Skip("Requires ToolChecker injection - behavior verified in integration tests")
			})
		})

		Context("when tflint succeeds with no findings", func() {
			It("should return success", func() {
				mockRunner.runFunc = func(_ context.Context, name string, args ...string) execpkg.CommandResult {
					Expect(name).To(Equal("tflint"))
					Expect(args).To(ContainElements("--format=compact"))

					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
					}
				}

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

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   compactOutput,
						Stderr:   "",
						ExitCode: 2,
						Err:      errTfLintFailed,
					}
				}

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(compactOutput))
				Expect(result.Err).To(Equal(errTfLintFailed))
			})

			It("should use stderr if stdout is empty", func() {
				stderrOutput := "tflint: error parsing configuration"

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errTfLintFailed,
					}
				}

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when tflint command fails with no output", func() {
			It("should return error", func() {
				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 127,
						Err:      errTfLintFailed,
					}
				}

				result := linter.Lint(ctx, "main.tf")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(Equal(errTfLintFailed))
			})
		})
	})
})
