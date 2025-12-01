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
	errRuffFailed           = errors.New("ruff failed")
	errRuffTempFileCreation = errors.New("failed to create temp file")
)

var _ = Describe("RuffChecker", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		mockTempManager *execpkg.MockTempFileManager
		checker         linters.RuffChecker
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
		checker = linters.NewRuffCheckerWithDeps(contentLinter)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Check", func() {
		Context("when ruff is not available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().IsAvailable("ruff").Return(false)

				result := checker.Check(ctx, "import os\nprint('hello')")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when ruff passes", func() {
			It("should return success", func() {
				scriptContent := "print('hello')"

				mockToolChecker.EXPECT().IsAvailable("ruff").Return(true)
				mockTempManager.EXPECT().Create("script-*.py", scriptContent).
					Return("/tmp/script-123.py", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "ruff", "check", "--output-format=json", "/tmp/script-123.py").
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

		Context("when ruff fails", func() {
			It("should return failure with findings", func() {
				ruffOutput := `[{"code":"F401","message":"` + "`os` imported but unused" + `","location":{"row":1,"column":8},"end_location":{"row":1,"column":10},"filename":"-"}]`
				scriptContent := "import os\nprint('hello')"

				mockToolChecker.EXPECT().IsAvailable("ruff").Return(true)
				mockTempManager.EXPECT().Create("script-*.py", scriptContent).
					Return("/tmp/script-123.py", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "ruff", "check", "--output-format=json", "/tmp/script-123.py").
					Return(execpkg.CommandResult{
						Stdout:   ruffOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errRuffFailed,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(ruffOutput))
				Expect(result.Err).To(Equal(errRuffFailed))
				Expect(result.Findings).To(HaveLen(1))
				Expect(result.Findings[0].Rule).To(Equal("F401"))
				Expect(result.Findings[0].Line).To(Equal(1))
				Expect(result.Findings[0].Column).To(Equal(8))
				Expect(result.Findings[0].Severity).To(Equal(linters.SeverityError))
			})

			It("should include stderr in output when stdout is empty", func() {
				stderrOutput := "ruff: error parsing file"
				scriptContent := "import os\ninvalid syntax"

				mockToolChecker.EXPECT().IsAvailable("ruff").Return(true)
				mockTempManager.EXPECT().Create("script-*.py", scriptContent).
					Return("/tmp/script-123.py", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "ruff", "check", "--output-format=json", "/tmp/script-123.py").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errRuffFailed,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				scriptContent := "print('hello')"

				mockToolChecker.EXPECT().IsAvailable("ruff").Return(true)
				mockTempManager.EXPECT().Create("script-*.py", scriptContent).
					Return("", nil, errRuffTempFileCreation)

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(HaveOccurred())
			})
		})
	})

	Describe("CheckWithOptions", func() {
		Context("when excluding rules", func() {
			It("should pass exclude rules to ruff", func() {
				scriptContent := "import os\nprint('hello')"
				opts := &linters.RuffCheckOptions{
					ExcludeRules: []string{"F401", "E501"},
				}

				mockToolChecker.EXPECT().IsAvailable("ruff").Return(true)
				mockTempManager.EXPECT().Create("script-*.py", scriptContent).
					Return("/tmp/script-123.py", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "ruff", "check", "--output-format=json", "--ignore=F401", "--ignore=E501", "/tmp/script-123.py").
					Return(execpkg.CommandResult{
						Stdout:   "[]",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, scriptContent, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when using custom config path", func() {
			It("should pass config path to ruff", func() {
				scriptContent := "print('hello')"
				opts := &linters.RuffCheckOptions{
					ConfigPath: "/path/to/ruff.toml",
				}

				mockToolChecker.EXPECT().IsAvailable("ruff").Return(true)
				mockTempManager.EXPECT().Create("script-*.py", scriptContent).
					Return("/tmp/script-123.py", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "ruff", "check", "--output-format=json", "--config=/path/to/ruff.toml", "/tmp/script-123.py").
					Return(execpkg.CommandResult{
						Stdout:   "[]",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, scriptContent, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})

		Context("when using both config path and exclude rules", func() {
			It("should pass both options to ruff", func() {
				scriptContent := "import os\nprint('hello')"
				opts := &linters.RuffCheckOptions{
					ConfigPath:   "/path/to/ruff.toml",
					ExcludeRules: []string{"F401"},
				}

				mockToolChecker.EXPECT().IsAvailable("ruff").Return(true)
				mockTempManager.EXPECT().Create("script-*.py", scriptContent).
					Return("/tmp/script-123.py", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "ruff", "check", "--output-format=json", "--config=/path/to/ruff.toml", "--ignore=F401", "/tmp/script-123.py").
					Return(execpkg.CommandResult{
						Stdout:   "[]",
						Stderr:   "",
						ExitCode: 0,
						Err:      nil,
					})

				result := checker.CheckWithOptions(ctx, scriptContent, opts)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
			})
		})
	})
})
