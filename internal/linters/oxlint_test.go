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
	errOxlintFailed           = errors.New("oxlint failed")
	errOxlintTempFileCreation = errors.New("failed to create temp file")
)

var _ = Describe("OxlintChecker", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		mockTempManager *execpkg.MockTempFileManager
		checker         linters.OxlintChecker
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
		checker = linters.NewOxlintCheckerWithDeps(contentLinter)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Check", func() {
		Context("when oxlint is not available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(false)

				result := checker.Check(ctx, "const x = 1;\nconsole.log(x);")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when oxlint passes", func() {
			It("should return success", func() {
				scriptContent := "const x = 1;\nconsole.log(x);"

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("/tmp/script-123.js", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "oxlint", "--format=json", "/tmp/script-123.js").
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

		Context("when oxlint fails with errors", func() {
			It("should return failure with findings", func() {
				oxlintOutput := `[{"filePath":"script.js","messages":[{"ruleId":"no-unused-vars","message":"'x' is assigned a value but never used","line":1,"column":7,"severity":2}]}]`
				scriptContent := "const x = 1;\nconsole.log('hello');"

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("/tmp/script-123.js", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "oxlint", "--format=json", "/tmp/script-123.js").
					Return(execpkg.CommandResult{
						Stdout:   oxlintOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errOxlintFailed,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(oxlintOutput))
				Expect(result.Err).To(Equal(errOxlintFailed))
				Expect(result.Findings).To(HaveLen(1))
				Expect(result.Findings[0].Rule).To(Equal("no-unused-vars"))
				Expect(result.Findings[0].Line).To(Equal(1))
				Expect(result.Findings[0].Column).To(Equal(7))
				Expect(result.Findings[0].Severity).To(Equal(linters.SeverityError))
			})
		})

		Context("when oxlint fails with warnings", func() {
			It("should return failure with warning severity", func() {
				oxlintOutput := `[{"filePath":"script.js","messages":[{"ruleId":"no-console","message":"Unexpected console statement","line":1,"column":1,"severity":1}]}]`
				scriptContent := "console.log('hello');"

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("/tmp/script-123.js", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "oxlint", "--format=json", "/tmp/script-123.js").
					Return(execpkg.CommandResult{
						Stdout:   oxlintOutput,
						Stderr:   "",
						ExitCode: 1,
						Err:      errOxlintFailed,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Findings).To(HaveLen(1))
				Expect(result.Findings[0].Severity).To(Equal(linters.SeverityWarning))
			})
		})

		Context("when oxlint returns stderr", func() {
			It("should include stderr in output when stdout is empty", func() {
				stderrOutput := "oxlint: error parsing file"
				scriptContent := "const x = invalid syntax"

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("/tmp/script-123.js", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "oxlint", "--format=json", "/tmp/script-123.js").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errOxlintFailed,
					})

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				scriptContent := "const x = 1;"

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("", nil, errOxlintTempFileCreation)

				result := checker.Check(ctx, scriptContent)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(HaveOccurred())
			})
		})
	})

	Describe("CheckWithOptions", func() {
		Context("when excluding rules", func() {
			It("should pass exclude rules to oxlint", func() {
				scriptContent := "console.log('hello');"
				opts := &linters.OxlintCheckOptions{
					ExcludeRules: []string{"no-console", "no-debugger"},
				}

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("/tmp/script-123.js", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "oxlint", "--format=json", "--disable", "no-console", "--disable", "no-debugger", "/tmp/script-123.js").
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
			It("should pass config path to oxlint", func() {
				scriptContent := "const x = 1;"
				opts := &linters.OxlintCheckOptions{
					ConfigPath: "/path/to/.oxlintrc.json",
				}

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("/tmp/script-123.js", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "oxlint", "--format=json", "-c", "/path/to/.oxlintrc.json", "/tmp/script-123.js").
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
			It("should pass both options to oxlint", func() {
				scriptContent := "console.log('hello');"
				opts := &linters.OxlintCheckOptions{
					ConfigPath:   "/path/to/.oxlintrc.json",
					ExcludeRules: []string{"no-console"},
				}

				mockToolChecker.EXPECT().IsAvailable("oxlint").Return(true)
				mockTempManager.EXPECT().Create("script-*.js", scriptContent).
					Return("/tmp/script-123.js", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "oxlint", "--format=json", "-c", "/path/to/.oxlintrc.json", "--disable", "no-console", "/tmp/script-123.js").
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
