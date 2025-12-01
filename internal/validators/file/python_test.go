package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("PythonValidator", func() {
	var (
		v   *file.PythonValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		// Create a real RuffChecker for integration tests
		runner := execpkg.NewCommandRunner(10 * time.Second)
		checker := linters.NewRuffChecker(runner)
		v = file.NewPythonValidator(logger.NewNoOpLogger(), checker, nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{},
		}
	})

	Describe("valid Python scripts", func() {
		It("should pass for valid Python code", func() {
			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `def greet(name):
    print(f"Hello, {name}!")

if __name__ == "__main__":
    greet("World")
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for simple script", func() {
			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `print("Hello, World!")
`
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
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Ruff validation failed"))
		})

		It("should fail for undefined variable", func() {
			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `def hello():
    print(undefined_var)
`
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
			runner := execpkg.NewCommandRunner(10 * time.Second)
			checker := linters.NewRuffChecker(runner)
			v = file.NewPythonValidator(logger.NewNoOpLogger(), checker, cfg, nil)

			ctx.ToolInput.FilePath = "test.py"
			ctx.ToolInput.Content = `import os

print("Hello, World!")
`
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

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should exclude F401 and F841 for fragments", func() {
			// F401 (unused import) should be excluded for fragments
			ctx.EventType = hook.EventTypePreToolUse
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tempFile
			ctx.ToolInput.OldString = `def greet(name):
    print(f"Hello, {name}!")`
			ctx.ToolInput.NewString = `def greet(name):
    import os  # This would normally trigger F401
    print(f"Hello, {name}!")`

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
})
