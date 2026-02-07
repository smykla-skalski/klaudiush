package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("PythonValidator", func() {
	var (
		v           *file.PythonValidator
		ctx         *hook.Context
		mockCtrl    *gomock.Controller
		mockChecker *linters.MockRuffChecker
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockChecker = linters.NewMockRuffChecker(mockCtrl)
		v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("valid Python scripts", func() {
		It("should pass for valid Python code", func() {
			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `def greet(name):
    print(f"Hello, {name}!")

if __name__ == "__main__":
    greet("World")
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for simple script", func() {
			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `print("Hello, World!")
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("invalid Python scripts", func() {
		It("should fail for unused import", func() {
			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `import os

print("Hello, World!")
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "test.py:1:1: F401 'os' imported but unused",
					Findings: []linters.LintFinding{
						{
							File:     "test.py",
							Line:     1,
							Column:   1,
							Severity: linters.SeverityError,
							Message:  "'os' imported but unused",
							Rule:     "F401",
						},
					},
				})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Ruff validation failed"))
		})

		It("should fail for undefined variable", func() {
			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `def hello():
    print(undefined_var)
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "test.py:2:11: F821 undefined name 'undefined_var'",
					Findings: []linters.LintFinding{
						{
							File:     "test.py",
							Line:     2,
							Column:   11,
							Severity: linters.SeverityError,
							Message:  "undefined name 'undefined_var'",
							Rule:     "F821",
						},
					},
				})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Ruff validation failed"))
		})
	})

	Describe("configuration", func() {
		It("should respect exclude rules", func() {
			cfg := &config.PythonValidatorConfig{
				ExcludeRules: []string{"F401"}, // Exclude unused import rule
			}
			mockChecker = linters.NewMockRuffChecker(mockCtrl)
			v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `import os

print("Hello, World!")
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
			tempFile = filepath.Join(tmpDir, "test.py")

			content := `def greet(name):
    """Greet someone by name."""
    print(f"Hello, {name}!")

if __name__ == "__main__":
    greet("World")
`
			err := os.WriteFile(tempFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate edit fragment", func() {
			ctx.EventType = hook.EventTypePreToolUse
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tempFile
			ctx.ToolInput.OldString = `    """Greet someone by name."""`
			ctx.ToolInput.NewString = `    """Say hello to someone by name."""`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should exclude F401 and F841 for fragments", func() {
			// F401 (unused import) should be excluded for fragments
			ctx.EventType = hook.EventTypePreToolUse
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tempFile
			ctx.ToolInput.OldString = `    """Greet someone by name."""
    print(f"Hello, {name}!")`
			ctx.ToolInput.NewString = `    """Greet someone by name."""
    import os  # This would normally trigger F401
    print(f"Hello, {name}!")`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			// Should pass because F401 is excluded for fragments
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("no file path", func() {
		It("should pass when no file path is provided", func() {
			ctx.ToolInput.FilePath = ""
			ctx.ToolInput.Content = "import os"

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("configuration options", func() {
		Context("isUseRuff", func() {
			It("should return true by default", func() {
				mockChecker = linters.NewMockRuffChecker(mockCtrl)
				v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)

				ctx.ToolInput.FilePath = "test.py"
				ctx.ToolInput.Content = "print('hello')"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should respect UseRuff config when disabled", func() {
				useRuff := false
				cfg := &config.PythonValidatorConfig{
					UseRuff: &useRuff,
				}
				mockChecker = linters.NewMockRuffChecker(mockCtrl)
				v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.py"
				ctx.ToolInput.Content = "import os"
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should respect UseRuff config when enabled", func() {
				useRuff := true
				cfg := &config.PythonValidatorConfig{
					UseRuff: &useRuff,
				}
				mockChecker = linters.NewMockRuffChecker(mockCtrl)
				v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.py"
				ctx.ToolInput.Content = "print('hello')"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("timeout", func() {
			It("should use custom timeout from config", func() {
				cfg := &config.PythonValidatorConfig{
					Timeout: config.Duration(30 * time.Second),
				}
				mockChecker = linters.NewMockRuffChecker(mockCtrl)
				v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.py"
				ctx.ToolInput.Content = "print('hello')"

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
				cfg := &config.PythonValidatorConfig{
					ContextLines: &contextLines,
				}
				mockChecker = linters.NewMockRuffChecker(mockCtrl)
				v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.py"
				ctx.ToolInput.Content = "print('hello')"

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("ruff config path", func() {
			It("should accept custom ruff config path", func() {
				cfg := &config.PythonValidatorConfig{
					RuffConfig: "/path/to/ruff.toml",
				}
				mockChecker = linters.NewMockRuffChecker(mockCtrl)
				v = file.NewPythonValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

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
			tempFile = filepath.Join(tmpDir, "test.py")
		})

		Context("when file exists", func() {
			It("should read and validate file content", func() {
				content := `print("Hello, World!")
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
				ctx.ToolInput.FilePath = filepath.Join(GinkgoT().TempDir(), "nonexistent.py")
				ctx.ToolInput.Content = ""

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when edit operation has missing strings", func() {
			It("should pass when old_string is missing", func() {
				content := `print("Hello, World!")
`
				err := os.WriteFile(tempFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = tempFile
				ctx.ToolInput.OldString = ""
				ctx.ToolInput.NewString = "print('Hi')"

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass when new_string is missing", func() {
				content := `print("Hello, World!")
`
				err := os.WriteFile(tempFile, []byte(content), 0o600)
				Expect(err).NotTo(HaveOccurred())

				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = tempFile
				ctx.ToolInput.OldString = "print('Hello')"
				ctx.ToolInput.NewString = ""

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when edit file cannot be read", func() {
			It("should pass gracefully", func() {
				ctx.EventType = hook.EventTypePreToolUse
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = filepath.Join(GinkgoT().TempDir(), "nonexistent.py")
				ctx.ToolInput.OldString = "old"
				ctx.ToolInput.NewString = "new"

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when fragment cannot be extracted", func() {
			It("should pass gracefully", func() {
				content := `print("Hello, World!")
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
