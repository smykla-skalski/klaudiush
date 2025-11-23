package linters_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
	"github.com/smykla-labs/claude-hooks/internal/linters"
)

var errActionLintFailed = errors.New("actionlint failed")

var _ = Describe("ActionLinter", func() {
	var (
		linter     linters.ActionLinter
		mockRunner *mockCommandRunner
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockRunner = &mockCommandRunner{}
		linter = linters.NewActionLinter(mockRunner)
	})

	Describe("Lint", func() {
		Context("when actionlint is not available", func() {
			It("should return success without validation", func() {
				// When actionlint is not in PATH, IsAvailable returns false
				Skip("Requires ToolChecker injection - behavior verified in integration tests")
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

				mockRunner.runFunc = func(_ context.Context, name string, args ...string) execpkg.CommandResult {
					Expect(name).To(Equal("actionlint"))
					Expect(args).To(ContainElement("-no-color"))

					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
					}
				}

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

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   actionlintOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errActionLintFailed,
					}
				}

				result := linter.Lint(ctx, "workflow content", ".github/workflows/test.yml")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(ContainSubstring("property \"run-on\" is not defined"))
				Expect(result.Err).To(Equal(errActionLintFailed))
			})

			It("should include stderr when stdout is empty", func() {
				stderrOutput := "actionlint: error parsing workflow file"

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errActionLintFailed,
					}
				}

				result := linter.Lint(ctx, "invalid yaml", ".github/workflows/test.yml")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				// This test requires injecting a mock TempFileManager
				Skip("Requires TempFileManager injection support")
			})
		})
	})
})
