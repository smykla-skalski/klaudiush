package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/github"
	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("validation after the tool ran", func() {
	var (
		dir     string
		afterFn func(path, input string) *hook.Context
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		afterFn = func(path, input string) *hook.Context {
			return &hook.Context{
				Provider:     hook.ProviderClaude,
				Event:        hook.CanonicalEventAfterTool,
				EventType:    hook.EventTypePostToolUse,
				ToolName:     hook.ToolTypeWrite,
				ToolExecuted: true,
				ToolInput:    hook.ToolInput{FilePath: path, Content: input},
			}
		}
	})

	Describe("ContentExtractor", func() {
		It("returns the file on disk instead of the tool input", func() {
			path := filepath.Join(dir, "script.sh")
			Expect(os.WriteFile(path, []byte("echo partial"), 0o600)).To(Succeed())

			info, err := file.NewContentExtractor(logger.NewNoOpLogger(), 2).
				Extract(afterFn(path, "echo requested"), path)

			Expect(err).NotTo(HaveOccurred())
			Expect(info.Content).To(Equal("echo partial"))
			Expect(info.IsFragment).To(BeFalse())
		})

		It("fails when the tool left no file", func() {
			path := filepath.Join(dir, "missing.sh")

			_, err := file.NewContentExtractor(logger.NewNoOpLogger(), 2).
				Extract(afterFn(path, "echo requested"), path)

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("MarkdownValidator", func() {
		It("lints the written file rather than the requested content", func() {
			path := filepath.Join(dir, "README.md")
			Expect(os.WriteFile(path, []byte("# Title\n```\ncode\n```\n"), 0o600)).To(Succeed())

			runner := execpkg.NewCommandRunner(10 * time.Second)
			v := file.NewMarkdownValidator(
				nil,
				linters.NewMarkdownLinter(runner),
				logger.NewNoOpLogger(),
				nil,
			)

			result := v.Validate(context.Background(), afterFn(path, "# Title\n"))

			Expect(result.Passed).To(BeFalse())
		})
	})

	Describe("TerraformValidator", func() {
		It("skips when the tool left no file", func() {
			runner := execpkg.NewCommandRunner(10 * time.Second)
			v := file.NewTerraformValidator(
				linters.NewTerraformFormatter(runner),
				linters.NewTfLinter(runner),
				logger.NewNoOpLogger(),
				nil,
				nil,
			)

			ctx := afterFn(filepath.Join(dir, "main.tf"), "resource \"x\" \"y\" {}\n")

			Expect(v.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})
	})

	Describe("WorkflowValidator", func() {
		It("skips when the tool left no file", func() {
			runner := execpkg.NewCommandRunner(10 * time.Second)
			v := file.NewWorkflowValidator(
				linters.NewActionLinter(runner),
				func() github.Client { return &mockGitHubClient{} },
				logger.NewNoOpLogger(),
				nil,
				nil,
			)

			path := filepath.Join(dir, ".github", "workflows", "ci.yml")

			Expect(v.Validate(context.Background(), afterFn(path, "on: push\n")).Passed).
				To(BeTrue())
		})
	})
})
