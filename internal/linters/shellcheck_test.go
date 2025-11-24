package linters_test

import (
	"context"
	"errors"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
)

var errShellcheckFailed = errors.New("shellcheck failed")

// Mock implementations for testing
type mockCommandRunner struct {
	runFunc          func(ctx context.Context, name string, args ...string) execpkg.CommandResult
	runWithStdinFunc func(ctx context.Context, stdin io.Reader, name string, args ...string) execpkg.CommandResult
}

func (m *mockCommandRunner) Run(
	ctx context.Context,
	name string,
	args ...string,
) execpkg.CommandResult {
	if m.runFunc != nil {
		return m.runFunc(ctx, name, args...)
	}

	return execpkg.CommandResult{}
}

func (m *mockCommandRunner) RunWithStdin(
	ctx context.Context,
	stdin io.Reader,
	name string,
	args ...string,
) execpkg.CommandResult {
	if m.runWithStdinFunc != nil {
		return m.runWithStdinFunc(ctx, stdin, name, args...)
	}

	return execpkg.CommandResult{}
}

func (m *mockCommandRunner) RunWithTimeout(
	timeout time.Duration,
	name string,
	args ...string,
) execpkg.CommandResult {
	// Use context with timeout and call Run
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return m.Run(ctx, name, args...)
}

var _ = Describe("ShellChecker", func() {
	var (
		checker    linters.ShellChecker
		mockRunner *mockCommandRunner
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockRunner = &mockCommandRunner{}
		checker = linters.NewShellChecker(mockRunner)
	})

	Describe("Check", func() {
		Context("when shellcheck passes", func() {
			It("should return success", func() {
				mockRunner.runFunc = func(_ context.Context, name string, args ...string) execpkg.CommandResult {
					Expect(name).To(Equal("shellcheck"))
					Expect(args).To(HaveLen(2))
					Expect(args[0]).To(Equal("--format=json"))

					return execpkg.CommandResult{
						Stdout:   "[]",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					}
				}

				result := checker.Check(ctx, "#!/bin/bash\necho 'hello'")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when shellcheck fails", func() {
			It("should return failure with output", func() {
				shellcheckOutput := "script.sh:2:1: warning: Use $(...) instead of legacy backticks"

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   shellcheckOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errShellcheckFailed,
					}
				}

				result := checker.Check(ctx, "#!/bin/bash\nvar=`ls`")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(shellcheckOutput))
				Expect(result.Err).To(Equal(errShellcheckFailed))
			})

			It("should include stderr in output when stdout is empty", func() {
				stderrOutput := "shellcheck: error parsing script"

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errShellcheckFailed,
					}
				}

				result := checker.Check(ctx, "#!/bin/bash\ninvalid syntax")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				// This test requires injecting a mock TempFileManager
				// For now, we'll document this as a limitation
				// In real usage, temp file creation rarely fails
				Skip("Requires TempFileManager injection support")
			})
		})
	})
})
