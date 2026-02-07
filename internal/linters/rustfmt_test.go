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
	errRustfmtFailed           = errors.New("rustfmt failed")
	errRustfmtTempFileCreation = errors.New("failed to create temp file")
)

var _ = Describe("RustfmtChecker", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		mockTempManager *execpkg.MockTempFileManager
		checker         linters.RustfmtChecker
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
		checker = linters.NewRustfmtCheckerWithDeps(contentLinter)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Check", func() {
		Context("when rustfmt is not available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(false)

				result := checker.Check(ctx, "fn main() { println!(\"Hello\"); }")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when rustfmt passes", func() {
			It("should return success", func() {
				rustContent := "fn main() {\n    println!(\"Hello\");\n}\n"

				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(true)
				mockTempManager.EXPECT().Create("code-*.rs", rustContent).
					Return("/tmp/code-123.rs", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "rustfmt", "--check", "--edition", "2021", "/tmp/code-123.rs").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.Check(ctx, rustContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when rustfmt fails with formatting errors", func() {
			It("should return failure with findings", func() {
				rustContent := "fn main(){println!(\"Hello\");}"
				diffOutput := "Diff in /tmp/code-123.rs at line 1:\n fn main() {\n     println!(\"Hello\");\n }\n"

				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(true)
				mockTempManager.EXPECT().Create("code-*.rs", rustContent).
					Return("/tmp/code-123.rs", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "rustfmt", "--check", "--edition", "2021", "/tmp/code-123.rs").
					Return(execpkg.CommandResult{
						Stdout:   diffOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errRustfmtFailed,
					})

				result := checker.Check(ctx, rustContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(diffOutput))
				Expect(result.Err).To(Equal(errRustfmtFailed))
				Expect(result.Findings).To(HaveLen(1))
				Expect(result.Findings[0].Rule).To(Equal("rustfmt"))
				Expect(result.Findings[0].Severity).To(Equal(linters.SeverityError))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				rustContent := "fn main() {}"

				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(true)
				mockTempManager.EXPECT().Create("code-*.rs", rustContent).
					Return("", nil, errRustfmtTempFileCreation)

				result := checker.Check(ctx, rustContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(HaveOccurred())
			})
		})
	})

	Describe("CheckWithOptions", func() {
		Context("when using custom edition", func() {
			It("should pass edition to rustfmt", func() {
				rustContent := "fn main() {}\n"
				opts := &linters.RustfmtOptions{
					Edition: "2024",
				}

				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(true)
				mockTempManager.EXPECT().Create("code-*.rs", rustContent).
					Return("/tmp/code-123.rs", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "rustfmt", "--check", "--edition", "2024", "/tmp/code-123.rs").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, rustContent, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when using custom config path", func() {
			It("should pass config path to rustfmt", func() {
				rustContent := "fn main() {}\n"
				opts := &linters.RustfmtOptions{
					ConfigPath: "/path/to/rustfmt.toml",
				}

				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(true)
				mockTempManager.EXPECT().Create("code-*.rs", rustContent).
					Return("/tmp/code-123.rs", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "rustfmt", "--check", "--edition", "2021", "--config-path", "/path/to/rustfmt.toml", "/tmp/code-123.rs").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, rustContent, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when using both custom edition and config path", func() {
			It("should pass both options to rustfmt", func() {
				rustContent := "fn main() {}\n"
				opts := &linters.RustfmtOptions{
					Edition:    "2018",
					ConfigPath: "/path/to/rustfmt.toml",
				}

				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(true)
				mockTempManager.EXPECT().Create("code-*.rs", rustContent).
					Return("/tmp/code-123.rs", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "rustfmt", "--check", "--edition", "2018", "--config-path", "/path/to/rustfmt.toml", "/tmp/code-123.rs").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, rustContent, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when options is nil", func() {
			It("should default to edition 2021", func() {
				rustContent := "fn main() {}\n"

				mockToolChecker.EXPECT().IsAvailable("rustfmt").Return(true)
				mockTempManager.EXPECT().Create("code-*.rs", rustContent).
					Return("/tmp/code-123.rs", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "rustfmt", "--check", "--edition", "2021", "/tmp/code-123.rs").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, rustContent, nil)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})
	})
})
