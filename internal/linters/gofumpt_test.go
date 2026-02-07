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
	errGofumptFailed           = errors.New("gofumpt failed")
	errGofumptTempFileCreation = errors.New("failed to create temp file")
)

var _ = Describe("GofumptChecker", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		mockTempManager *execpkg.MockTempFileManager
		checker         linters.GofumptChecker
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
		checker = linters.NewGofumptCheckerWithDeps(contentLinter)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Check", func() {
		Context("when gofumpt is not available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(false)

				result := checker.Check(ctx, "package main\n\nfunc main() {}\n")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when gofumpt passes", func() {
			It("should return success for properly formatted code", func() {
				goCode := "package main\n\nfunc main() {}\n"

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("/tmp/code-123.go", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "gofumpt", "-l", "-d", "/tmp/code-123.go").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.Check(ctx, goCode)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when gofumpt fails", func() {
			It("should return failure with diff output", func() {
				gofumptDiff := `--- /tmp/code-123.go
+++ /tmp/code-123.go (formatted)
@@ -1,2 +1,2 @@
 package main
-func main() {}
+
+func main() {}`
				goCode := "package main\nfunc main() {}"

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("/tmp/code-123.go", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "gofumpt", "-l", "-d", "/tmp/code-123.go").
					Return(execpkg.CommandResult{
						Stdout:   gofumptDiff,
						Stderr:   "",
						ExitCode: 1,
						Err:      errGofumptFailed,
					})

				result := checker.Check(ctx, goCode)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(gofumptDiff))
				Expect(result.Err).To(Equal(errGofumptFailed))
				Expect(result.Findings).To(HaveLen(1))
				Expect(result.Findings[0].Severity).To(Equal(linters.SeverityError))
			})

			It("should include stderr in output when present", func() {
				stderrOutput := "gofumpt: error parsing file"
				goCode := "package main\ninvalid syntax"

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("/tmp/code-123.go", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "gofumpt", "-l", "-d", "/tmp/code-123.go").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errGofumptFailed,
					})

				result := checker.Check(ctx, goCode)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				goCode := "package main\n\nfunc main() {}\n"

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("", nil, errGofumptTempFileCreation)

				result := checker.Check(ctx, goCode)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(HaveOccurred())
			})
		})
	})

	Describe("CheckWithOptions", func() {
		Context("when extra rules are enabled", func() {
			It("should pass -extra flag to gofumpt", func() {
				goCode := "package main\n\nfunc main() {}\n"
				opts := &linters.GofumptOptions{ExtraRules: true}

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("/tmp/code-123.go", func() {}, nil)
				mockRunner.EXPECT().Run(ctx, "gofumpt", "-l", "-d", "-extra", "/tmp/code-123.go").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, goCode, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when lang is specified", func() {
			It("should pass -lang flag to gofumpt", func() {
				goCode := "package main\n\nfunc main() {}\n"
				opts := &linters.GofumptOptions{Lang: "go1.21"}

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("/tmp/code-123.go", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "gofumpt", "-l", "-d", "-lang", "go1.21", "/tmp/code-123.go").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, goCode, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when modpath is specified", func() {
			It("should pass -modpath flag to gofumpt", func() {
				goCode := "package main\n\nfunc main() {}\n"
				opts := &linters.GofumptOptions{ModPath: "github.com/example/repo"}

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("/tmp/code-123.go", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "gofumpt", "-l", "-d", "-modpath", "github.com/example/repo", "/tmp/code-123.go").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, goCode, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when multiple options are specified", func() {
			It("should pass all flags to gofumpt", func() {
				goCode := "package main\n\nfunc main() {}\n"
				opts := &linters.GofumptOptions{
					ExtraRules: true,
					Lang:       "go1.21",
					ModPath:    "github.com/example/repo",
				}

				mockToolChecker.EXPECT().IsAvailable("gofumpt").Return(true)
				mockTempManager.EXPECT().Create("code-*.go", goCode).
					Return("/tmp/code-123.go", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "gofumpt", "-l", "-d", "-extra", "-lang", "go1.21", "-modpath", "github.com/example/repo", "/tmp/code-123.go").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, goCode, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})
	})
})
