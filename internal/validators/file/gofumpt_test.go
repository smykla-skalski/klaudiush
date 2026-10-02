package file_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var unformatted = &linters.LintResult{
	Success:  false,
	RawOut:   "diff",
	Findings: []linters.LintFinding{{Message: "diff"}},
}

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
			const original = "package main\n\nfunc main() {}\n"

			BeforeEach(func() {
				Expect(os.WriteFile(testFilePath, []byte(original), 0o600)).To(Succeed())

				hookCtx.ToolName = hook.ToolTypeEdit
				hookCtx.ToolInput.OldString = "func main() {}"
				hookCtx.ToolInput.NewString = "func main()  {}"
				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)
			})

			It("blocks an edit that breaks formatting of a formatted file", func() {
				proposed := "package main\n\nfunc main()  {}\n"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), proposed, gomock.Any()).
					Return(&linters.LintResult{Success: false, RawOut: "diff"})
				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), original, gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(string(result.Reference)).To(ContainSubstring("FILE"))
			})

			It("only warns when the file was already unformatted", func() {
				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(unformatted).
					Times(2)

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("before this edit"))
			})

			It("still blocks when the baseline check fails without a diff", func() {
				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(unformatted)
				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), original, gomock.Any()).
					Return(&linters.LintResult{Success: false, RawOut: "signal: killed"})

				Expect(validator.Validate(ctx, hookCtx).ShouldBlock).To(BeTrue())
			})

			It("replaces every match when Gemini expects several", func() {
				Expect(
					os.WriteFile(testFilePath, []byte("package main\n\nvar a, b = 1, 1\n"), 0o600),
				).
					To(Succeed())

				hookCtx.ToolInput.OldString = "1"
				hookCtx.ToolInput.NewString = "2"
				hookCtx.ToolInput.Additional = map[string]json.RawMessage{
					"expected_replacements": json.RawMessage("2"),
				}

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), "package main\n\nvar a, b = 2, 2\n", gomock.Any()).
					Return(&linters.LintResult{Success: true})

				Expect(validator.Validate(ctx, hookCtx).Passed).To(BeTrue())
			})

			It("applies replace_all to every occurrence", func() {
				Expect(
					os.WriteFile(testFilePath, []byte("package main\n\nvar a, b = 1, 1\n"), 0o600),
				).
					To(Succeed())

				hookCtx.ToolInput.OldString = "1"
				hookCtx.ToolInput.NewString = "2"
				hookCtx.ToolInput.Additional = map[string]json.RawMessage{
					"replace_all": json.RawMessage("true"),
				}

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), "package main\n\nvar a, b = 2, 2\n", gomock.Any()).
					Return(&linters.LintResult{Success: true})

				Expect(validator.Validate(ctx, hookCtx).Passed).To(BeTrue())
			})

			It("passes when the edit does not apply to the file", func() {
				hookCtx.ToolInput.OldString = "not in file"

				Expect(validator.Validate(ctx, hookCtx).Passed).To(BeTrue())
			})

			It("checks a file created by an edit without a baseline", func() {
				Expect(os.Remove(testFilePath)).To(Succeed())

				hookCtx.ToolInput.OldString = ""
				hookCtx.ToolInput.NewString = "package main\n"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), "package main\n", gomock.Any()).
					Return(&linters.LintResult{Success: false, RawOut: "diff"})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("applies every MultiEdit replacement in order", func() {
				hookCtx.ToolName = hook.ToolTypeMultiEdit
				hookCtx.ToolInput.OldString = ""
				hookCtx.ToolInput.NewString = ""
				hookCtx.ToolInput.Additional = map[string]json.RawMessage{
					"edits": json.RawMessage(
						`[{"old_string":"main()","new_string":"run()"},` +
							`{"old_string":"run() {}","new_string":"run() { }"}]`,
					),
				}

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), "package main\n\nfunc run() { }\n", gomock.Any()).
					Return(&linters.LintResult{Success: true})

				Expect(validator.Validate(ctx, hookCtx).Passed).To(BeTrue())
			})

			It("passes a MultiEdit without edits", func() {
				hookCtx.ToolName = hook.ToolTypeMultiEdit
				hookCtx.ToolInput.Additional = map[string]json.RawMessage{
					"edits": json.RawMessage(`"bad"`),
				}

				Expect(validator.Validate(ctx, hookCtx).Passed).To(BeTrue())
			})
		})

		Context("after the tool ran", func() {
			BeforeEach(func() {
				hookCtx.Event = hook.CanonicalEventAfterTool
				hookCtx.EventType = hook.EventTypePostToolUse
				validator = file.NewGofumptValidator(log, mockChecker, nil, nil)
			})

			It("checks the file on disk, not the tool input", func() {
				onDisk := "package main\n\nfunc main()  {}\n"
				Expect(os.WriteFile(testFilePath, []byte(onDisk), 0o600)).To(Succeed())

				hookCtx.ToolName = hook.ToolTypeEdit
				hookCtx.ToolInput.OldString = "x"
				hookCtx.ToolInput.NewString = "y"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), onDisk, gomock.Any()).
					Return(&linters.LintResult{Success: false, RawOut: "diff"})

				result := validator.Validate(ctx, hookCtx)

				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("only warns when reverting the edit shows the file was unformatted", func() {
				onDisk := "package main\n\nfunc  run() {}\n"
				Expect(os.WriteFile(testFilePath, []byte(onDisk), 0o600)).To(Succeed())

				hookCtx.ToolName = hook.ToolTypeMultiEdit
				hookCtx.ToolInput.Additional = map[string]json.RawMessage{
					"edits": json.RawMessage(
						`[{"old_string":"main","new_string":"run"},` +
							`{"old_string":"x","new_string":"{}","replace_all":true}]`,
					),
				}

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), onDisk, gomock.Any()).
					Return(unformatted)
				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), "package main\n\nfunc  main() x\n", gomock.Any()).
					Return(unformatted)

				result := validator.Validate(ctx, hookCtx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeFalse())
			})

			It("blocks when the edit cannot be reverted", func() {
				Expect(os.WriteFile(testFilePath, []byte("package main\n"), 0o600)).To(Succeed())

				hookCtx.ToolName = hook.ToolTypeEdit
				hookCtx.ToolInput.OldString = "a"
				hookCtx.ToolInput.NewString = ""

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), "package main\n", gomock.Any()).
					Return(unformatted)

				Expect(validator.Validate(ctx, hookCtx).ShouldBlock).To(BeTrue())
			})

			It("passes when the file is gone", func() {
				hookCtx.ToolInput.Content = "package main\n"

				Expect(validator.Validate(ctx, hookCtx).Passed).To(BeTrue())
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
