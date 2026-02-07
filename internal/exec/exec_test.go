package exec_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/exec"
)

func TestExec(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exec Suite")
}

var _ = Describe("CommandRunner", func() {
	var runner exec.CommandRunner

	BeforeEach(func() {
		runner = exec.NewCommandRunner(5 * time.Second)
	})

	Describe("Run", func() {
		It("should execute a simple command", func() {
			ctx := context.Background()
			result := runner.Run(ctx, "echo", "hello")

			Expect(result.Err).ToNot(HaveOccurred())
			Expect(result.Stdout).To(Equal("hello\n"))
			Expect(result.ExitCode).To(Equal(0))
			Expect(result.Success()).To(BeTrue())
		})

		It("should capture stderr", func() {
			ctx := context.Background()
			// sh -c to write to stderr
			result := runner.Run(ctx, "sh", "-c", "echo error >&2")

			Expect(result.Err).ToNot(HaveOccurred())
			Expect(result.Stderr).To(Equal("error\n"))
		})

		It("should handle command failures", func() {
			ctx := context.Background()
			result := runner.Run(ctx, "sh", "-c", "exit 42")

			Expect(result.Err).To(HaveOccurred())
			Expect(result.ExitCode).To(Equal(42))
			Expect(result.Failed()).To(BeTrue())
		})

		It("should respect context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			result := runner.Run(ctx, "sleep", "10")
			Expect(result.Err).To(HaveOccurred())
		})
	})

	Describe("RunWithStdin", func() {
		It("should pass stdin to command", func() {
			ctx := context.Background()
			stdin := strings.NewReader("test input")

			result := runner.RunWithStdin(ctx, stdin, "cat")

			Expect(result.Err).ToNot(HaveOccurred())
			Expect(result.Stdout).To(Equal("test input"))
		})
	})

	Describe("RunWithTimeout", func() {
		It("should execute command with timeout", func() {
			result := runner.RunWithTimeout(5*time.Second, "echo", "test")

			Expect(result.Err).ToNot(HaveOccurred())
			Expect(result.Stdout).To(Equal("test\n"))
		})

		It("should timeout long-running commands", func() {
			result := runner.RunWithTimeout(100*time.Millisecond, "sleep", "10")
			Expect(result.Err).To(HaveOccurred())
		})
	})
})

var _ = Describe("ToolChecker", func() {
	var checker exec.ToolChecker

	BeforeEach(func() {
		checker = exec.NewToolChecker()
	})

	Describe("IsAvailable", func() {
		It("should return true for available tools", func() {
			Expect(checker.IsAvailable("sh")).To(BeTrue())
			Expect(checker.IsAvailable("echo")).To(BeTrue())
		})

		It("should return false for unavailable tools", func() {
			Expect(checker.IsAvailable("nonexistent-tool-xyz")).To(BeFalse())
		})
	})

	Describe("RequireTool", func() {
		It("should not error for available tools", func() {
			err := checker.RequireTool("sh")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should error for unavailable tools", func() {
			err := checker.RequireTool("nonexistent-tool-xyz")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("FindTool", func() {
		It("should return first available tool", func() {
			tool := checker.FindTool("nonexistent-tool", "sh", "bash")
			Expect(tool).To(Equal("sh"))
		})

		It("should return empty string if none available", func() {
			tool := checker.FindTool("nonexistent-tool-1", "nonexistent-tool-2")
			Expect(tool).To(Equal(""))
		})

		It("should handle empty list", func() {
			tool := checker.FindTool()
			Expect(tool).To(Equal(""))
		})
	})
})

var _ = Describe("TempFileManager", func() {
	var manager exec.TempFileManager

	BeforeEach(func() {
		manager = exec.NewTempFileManager()
	})

	Describe("Create", func() {
		It("should create temp file with content", func() {
			path, cleanup, err := manager.Create("test-*.txt", "test content")
			defer cleanup()

			Expect(err).ToNot(HaveOccurred())
			Expect(path).ToNot(BeEmpty())

			// Read file to verify content
			content, err := os.ReadFile(path)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(content)).To(Equal("test content"))
		})

		It("should cleanup file when called", func() {
			path, cleanup, err := manager.Create("test-*.txt", "test")
			Expect(err).ToNot(HaveOccurred())

			// File should exist
			_, err = os.Stat(path)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			cleanup()

			// File should not exist
			_, err = os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should handle empty content", func() {
			path, cleanup, err := manager.Create("test-*.txt", "")
			defer cleanup()

			Expect(err).ToNot(HaveOccurred())
			Expect(path).ToNot(BeEmpty())
		})
	})
})
