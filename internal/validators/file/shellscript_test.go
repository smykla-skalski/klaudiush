package file_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("ShellScriptValidator", func() {
	var (
		v   *file.ShellScriptValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		// Create a real ShellChecker for integration tests
		runner := execpkg.NewCommandRunner(10 * time.Second)
		checker := linters.NewShellChecker(runner)
		v = file.NewShellScriptValidator(logger.NewNoOpLogger(), checker, nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{},
		}
	})

	Describe("valid shell scripts", func() {
		It("should pass for valid bash script", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/bash
echo "Hello, World!"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for valid sh script", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/sh
echo "Hello, World!"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("invalid shell scripts", func() {
		It("should fail for undefined variable", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/bash
echo $UNDEFINED_VAR
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Shellcheck validation failed"))
		})

		It("should fail for syntax error", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/bash
if [ -f file.txt ]
  echo "File exists"
fi
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Shellcheck validation failed"))
		})
	})

	Describe("Fish scripts", func() {
		It("should skip .fish extension", func() {
			ctx.ToolInput.FilePath = "test.fish"
			ctx.ToolInput.Content = `#!/usr/bin/env fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip fish shebang", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/usr/bin/env fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip /usr/bin/fish shebang", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/usr/bin/fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip /bin/fish shebang", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("edge cases", func() {
		It("should pass when no file path provided", func() {
			ctx.ToolInput.FilePath = ""
			ctx.ToolInput.Content = ""
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for empty content", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = ""
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})
})
