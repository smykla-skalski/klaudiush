package file_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("GofumptValidator", func() {
	var (
		ctrl         *gomock.Controller
		mockChecker  *linters.MockGofumptChecker
		validator    *file.GofumptValidator
		ctx          context.Context
		hookCtx      *hook.Context
		log          logger.Logger
		testDir      string
		testFilePath string
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockChecker = linters.NewMockGofumptChecker(ctrl)
		log = logger.NewNoOpLogger()
		ctx = context.Background()

		// Create temp directory for tests
		var err error
		testDir, err = os.MkdirTemp("", "gofumpt-test-*")
		Expect(err).NotTo(HaveOccurred())

		testFilePath = filepath.Join(testDir, "main.go")

		hookCtx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{
				FilePath: testFilePath,
			},
		}
	})

	AfterEach(func() {
		ctrl.Finish()
		if testDir != "" {
			os.RemoveAll(testDir)
		}
	})

	Describe("Validate", func() {
		Context("when gofumpt passes", func() {
			It("should return Pass for properly formatted code", func() {
				goCode := "package main\n\nfunc main() {}\n"
				hookCtx.ToolInput.Content = goCode

				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Return(&linters.LintResult{
						Success: true,
					})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when gofumpt fails", func() {
			It("should return FailWithRef with formatted output", func() {
				goCode := "package main\nfunc main() {}"
				hookCtx.ToolInput.Content = goCode

				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Return(&linters.LintResult{
						Success: false,
						RawOut:  "formatting issues detected",
					})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Reference).NotTo(BeEmpty())
			})
		})

		Context("when no file path is provided", func() {
			It("should return Pass", func() {
				hookCtx.ToolInput.FilePath = ""

				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when Edit operation", func() {
			It("should return Pass (no fragment support)", func() {
				hookCtx.ToolName = hook.ToolTypeEdit
				hookCtx.ToolInput.OldString = "old"
				hookCtx.ToolInput.NewString = "new"

				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when content is empty", func() {
			It("should return Pass", func() {
				hookCtx.ToolInput.Content = ""

				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})
	})

	Describe("Configuration", func() {
		Context("when extra_rules is enabled", func() {
			It("should pass extra_rules to gofumpt", func() {
				goCode := "package main\n\nfunc main() {}\n"
				hookCtx.ToolInput.Content = goCode

				extraRules := true
				cfg := &config.GofumptValidatorConfig{
					ExtraRules: &extraRules,
				}
				validator = file.NewGofumptValidator(log, mockChecker, cfg, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Do(func(_ context.Context, _ string, opts *linters.GofumptOptions) {
						Expect(opts.ExtraRules).To(BeTrue())
					}).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when lang is configured", func() {
			It("should use configured lang", func() {
				goCode := "package main\n\nfunc main() {}\n"
				hookCtx.ToolInput.Content = goCode

				cfg := &config.GofumptValidatorConfig{
					Lang: "go1.21",
				}
				validator = file.NewGofumptValidator(log, mockChecker, cfg, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Do(func(_ context.Context, _ string, opts *linters.GofumptOptions) {
						Expect(opts.Lang).To(Equal("go1.21"))
					}).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when modpath is configured", func() {
			It("should use configured modpath", func() {
				goCode := "package main\n\nfunc main() {}\n"
				hookCtx.ToolInput.Content = goCode

				cfg := &config.GofumptValidatorConfig{
					ModPath: "github.com/example/repo",
				}
				validator = file.NewGofumptValidator(log, mockChecker, cfg, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Do(func(_ context.Context, _ string, opts *linters.GofumptOptions) {
						Expect(opts.ModPath).To(Equal("github.com/example/repo"))
					}).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})
	})

	Describe("go.mod auto-detection", func() {
		Context("when go.mod exists", func() {
			It("should auto-detect Go version and module path", func() {
				// Create go.mod file
				goModContent := `module github.com/example/test

go 1.21

require (
	github.com/example/dep v1.0.0
)
`
				goModPath := filepath.Join(testDir, "go.mod")
				err := os.WriteFile(goModPath, []byte(goModContent), 0o600)
				Expect(err).NotTo(HaveOccurred())

				goCode := "package main\n\nfunc main() {}\n"
				hookCtx.ToolInput.Content = goCode

				// No lang/modpath configured
				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Do(func(_ context.Context, _ string, opts *linters.GofumptOptions) {
						Expect(opts.Lang).To(Equal("go1.21"))
						Expect(opts.ModPath).To(Equal("github.com/example/test"))
					}).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when go.mod exists in parent directory", func() {
			It("should find go.mod by walking up", func() {
				// Create go.mod in parent
				goModContent := `module github.com/example/parent

go 1.20
`
				goModPath := filepath.Join(testDir, "go.mod")
				err := os.WriteFile(goModPath, []byte(goModContent), 0o600)
				Expect(err).NotTo(HaveOccurred())

				// Create subdirectory
				subDir := filepath.Join(testDir, "pkg", "subpkg")
				err = os.MkdirAll(subDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				// File in subdirectory
				subFilePath := filepath.Join(subDir, "code.go")
				hookCtx.ToolInput.FilePath = subFilePath

				goCode := "package subpkg\n\nfunc Test() {}\n"
				hookCtx.ToolInput.Content = goCode

				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Do(func(_ context.Context, _ string, opts *linters.GofumptOptions) {
						Expect(opts.Lang).To(Equal("go1.20"))
						Expect(opts.ModPath).To(Equal("github.com/example/parent"))
					}).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when go.mod does not exist", func() {
			It("should use empty lang and modpath", func() {
				goCode := "package main\n\nfunc main() {}\n"
				hookCtx.ToolInput.Content = goCode

				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Do(func(_ context.Context, _ string, opts *linters.GofumptOptions) {
						Expect(opts.Lang).To(Equal(""))
						Expect(opts.ModPath).To(Equal(""))
					}).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when config overrides auto-detection", func() {
			It("should use configured values instead of go.mod", func() {
				// Create go.mod with different values
				goModContent := `module github.com/example/test

go 1.21
`
				goModPath := filepath.Join(testDir, "go.mod")
				err := os.WriteFile(goModPath, []byte(goModContent), 0o600)
				Expect(err).NotTo(HaveOccurred())

				goCode := "package main\n\nfunc main() {}\n"
				hookCtx.ToolInput.Content = goCode

				// Config overrides
				cfg := &config.GofumptValidatorConfig{
					Lang:    "go1.22",
					ModPath: "github.com/example/override",
				}
				validator = file.NewGofumptValidator(log, mockChecker, cfg, nil)

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), goCode, gomock.Any()).
					Do(func(_ context.Context, _ string, opts *linters.GofumptOptions) {
						Expect(opts.Lang).To(Equal("go1.22"))
						Expect(opts.ModPath).To(Equal("github.com/example/override"))
					}).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeTrue())
			})
		})
	})
})
