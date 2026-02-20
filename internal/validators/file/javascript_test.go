package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("JavaScriptValidator", func() {
	var (
		v           *file.JavaScriptValidator
		ctx         *hook.Context
		mockCtrl    *gomock.Controller
		mockChecker *linters.MockOxlintChecker
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockChecker = linters.NewMockOxlintChecker(mockCtrl)
		v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("valid JavaScript/TypeScript code", func() {
		It("should pass for valid JavaScript code", func() {
			ctx.ToolInput.FilePath = "test.js"
			ctx.ToolInput.Content = `function greet(name) {
    console.log('Hello, ' + name + '!');
}

if (require.main === module) {
    greet('World');
}
`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for valid TypeScript code", func() {
			ctx.ToolInput.FilePath = "test.ts"
			ctx.ToolInput.Content = "function greet(name: string): void {\n" +
				"    console.log(`Hello, ${name}!`);\n" +
				"}\n\n" +
				"greet('World');\n"

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for simple script", func() {
			ctx.ToolInput.FilePath = "test.js"
			ctx.ToolInput.Content = `console.log("Hello, World!");
`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("invalid JavaScript/TypeScript code", func() {
		It("should fail for unused variable", func() {
			ctx.ToolInput.FilePath = "test.js"
			ctx.ToolInput.Content = `const unused = 42;

console.log("Hello, World!");
`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "test.js:1:7: 'unused' is assigned a value but never used (no-unused-vars)",
					Findings: []linters.LintFinding{
						{
							File:     "test.js",
							Line:     1,
							Column:   7,
							Severity: linters.SeverityError,
							Message:  "'unused' is assigned a value but never used",
							Rule:     "no-unused-vars",
						},
					},
				})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(Not(BeEmpty()))
		})

		It("should fail for undefined variable", func() {
			ctx.ToolInput.FilePath = "test.js"
			ctx.ToolInput.Content = `function hello() {
    console.log(undefinedVar);
}
`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "test.js:2:17: 'undefinedVar' is not defined (no-undef)",
					Findings: []linters.LintFinding{
						{
							File:     "test.js",
							Line:     2,
							Column:   17,
							Severity: linters.SeverityError,
							Message:  "'undefinedVar' is not defined",
							Rule:     "no-undef",
						},
					},
				})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(Not(BeEmpty()))
		})
	})

	Describe("configuration", func() {
		It("should respect exclude rules", func() {
			cfg := &config.JavaScriptValidatorConfig{
				ExcludeRules: []string{"no-unused-vars"}, // Exclude unused var rule
			}
			mockChecker = linters.NewMockOxlintChecker(mockCtrl)
			v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

			ctx.ToolInput.FilePath = "test.js"
			ctx.ToolInput.Content = `const unused = 42;

console.log("Hello, World!");
`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Edit operations with fragments", func() {
		var tempFile string

		BeforeEach(func() {
			// Create a temporary file for edit testing
			tmpDir := GinkgoT().TempDir()
			tempFile = filepath.Join(tmpDir, "test.js")

			content := `function greet(name) {
    // Say hello to someone
    console.log('Hello, ' + name + '!');
}

if (require.main === module) {
    greet('World');
}
`
			err := os.WriteFile(tempFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate edit fragment", func() {
			ctx.EventType = hook.EventTypePreToolUse
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tempFile
			ctx.ToolInput.OldString = `    // Say hello to someone`
			ctx.ToolInput.NewString = `    // Greet someone by name`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It(
			"should exclude no-unused-vars, no-undef, and import/no-unresolved for fragments",
			func() {
				// These rules should be excluded for fragments
				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = tempFile
				ctx.ToolInput.OldString = `    // Say hello to someone
    console.log('Hello, ' + name + '!');`
				ctx.ToolInput.NewString = `    // Say hello to someone
    const newVar = 42;  // This would normally trigger no-unused-vars
    console.log('Hello, ' + name + '!');`

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				// Should pass because no-unused-vars is excluded for fragments
				Expect(result.Passed).To(BeTrue())
			},
		)
	})

	Describe("no file path", func() {
		It("should pass when no file path is provided", func() {
			ctx.ToolInput.FilePath = ""
			ctx.ToolInput.Content = "const x = 42;"

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("configuration options", func() {
		Context("isUseOxlint", func() {
			It("should return true by default", func() {
				mockChecker = linters.NewMockOxlintChecker(mockCtrl)
				v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)

				ctx.ToolInput.FilePath = "test.js"
				ctx.ToolInput.Content = "console.log('hello');"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should respect UseOxlint config when disabled", func() {
				useOxlint := false
				cfg := &config.JavaScriptValidatorConfig{
					UseOxlint: &useOxlint,
				}
				mockChecker = linters.NewMockOxlintChecker(mockCtrl)
				v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.js"
				ctx.ToolInput.Content = "const unused = 42;"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should respect UseOxlint config when enabled", func() {
				useOxlint := true
				cfg := &config.JavaScriptValidatorConfig{
					UseOxlint: &useOxlint,
				}
				mockChecker = linters.NewMockOxlintChecker(mockCtrl)
				v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.js"
				ctx.ToolInput.Content = "console.log('hello');"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("timeout", func() {
			It("should use custom timeout from config", func() {
				cfg := &config.JavaScriptValidatorConfig{
					Timeout: config.Duration(30 * time.Second),
				}
				mockChecker = linters.NewMockOxlintChecker(mockCtrl)
				v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.js"
				ctx.ToolInput.Content = "console.log('hello');"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("context lines", func() {
			It("should use custom context lines from config", func() {
				contextLines := 5
				cfg := &config.JavaScriptValidatorConfig{
					ContextLines: &contextLines,
				}
				mockChecker = linters.NewMockOxlintChecker(mockCtrl)
				v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.js"
				ctx.ToolInput.Content = "console.log('hello');"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("oxlint config path", func() {
			It("should accept custom oxlint config path", func() {
				cfg := &config.JavaScriptValidatorConfig{
					OxlintConfig: "/path/to/oxlint.json",
				}
				mockChecker = linters.NewMockOxlintChecker(mockCtrl)
				v = file.NewJavaScriptValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				// Just verify validator is created with config
				Expect(v).NotTo(BeNil())
			})
		})
	})

	Describe("Category", func() {
		It("should return CategoryIO", func() {
			Expect(v.Category()).To(Equal(validator.CategoryIO))
		})
	})

	Describe("file operations", func() {
		var tempFile string

		BeforeEach(func() {
			tmpDir := GinkgoT().TempDir()
			tempFile = filepath.Join(tmpDir, "test.js")
		})

		Context("when file exists", func() {
			It("should read and validate file content", func() {
				content := `console.log("Hello, World!");
`
				err := os.WriteFile(tempFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				ctx.ToolInput.FilePath = tempFile
				ctx.ToolInput.Content = ""

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when file does not exist", func() {
			It("should pass gracefully", func() {
				ctx.ToolInput.FilePath = filepath.Join(GinkgoT().TempDir(), "nonexistent.js")
				ctx.ToolInput.Content = ""

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when edit operation has missing strings", func() {
			It("should pass when old_string is missing", func() {
				content := `console.log("Hello, World!");
`
				err := os.WriteFile(tempFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = tempFile
				ctx.ToolInput.OldString = ""
				ctx.ToolInput.NewString = "console.log('Hi');"

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass when new_string is missing", func() {
				content := `console.log("Hello, World!");
`
				err := os.WriteFile(tempFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = tempFile
				ctx.ToolInput.OldString = "console.log('Hello');"
				ctx.ToolInput.NewString = ""

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when edit file cannot be read", func() {
			It("should pass gracefully", func() {
				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = filepath.Join(GinkgoT().TempDir(), "nonexistent.js")
				ctx.ToolInput.OldString = "old"
				ctx.ToolInput.NewString = "new"

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when fragment cannot be extracted", func() {
			It("should pass gracefully", func() {
				content := `console.log("Hello, World!");
`
				err := os.WriteFile(tempFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = tempFile
				ctx.ToolInput.OldString = "nonexistent_code"
				ctx.ToolInput.NewString = "new_code"

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})
})
