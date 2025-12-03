package file_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("MarkdownValidator", func() {
	var (
		v   *file.MarkdownValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		runner := execpkg.NewCommandRunner(10 * time.Second)
		linter := linters.NewMarkdownLinter(runner)
		v = file.NewMarkdownValidator(nil, linter, logger.NewNoOpLogger(), nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
		}
	})

	Describe("Name", func() {
		It("returns correct validator name", func() {
			Expect(v.Name()).To(Equal("validate-markdown"))
		})
	})

	Describe("Validate", func() {
		Context("with valid markdown", func() {
			It("passes for empty content", func() {
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for markdown with proper spacing", func() {
				content := `# Header

Some text here.

- List item 1
- List item 2

` + "```" + `bash
code here
` + "```" + `

More text.
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for consecutive list items", func() {
				content := `- Item 1
- Item 2
- Item 3
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for list after header with blank line", func() {
				content := `## Features

- Feature 1
- Feature 2
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("fails for list directly after header without blank line", func() {
				content := `## Features
- Feature 1
- Feature 2
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(
					ContainSubstring("Header should have empty line after it"),
				)
			})

			It("passes for consecutive headers", func() {
				content := `# Title
## Subtitle
### Section
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for header followed by comment", func() {
				content := `# Header
<!-- Comment -->
Text
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("code block validation", func() {
			It("warns when code block has no empty line before", func() {
				content := `Some text
` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 2: Code block should have empty line before it"))
			})

			It("passes when code block has empty line before", func() {
				content := `Some text

` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})

			It("ignores list markers inside code blocks", func() {
				content := `
` + "```" + `bash
- this is not a list
* also not a list
1. still not a list
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})

			It("handles multiple code blocks", func() {
				content := `Text

` + "```" + `
code1
` + "```" + `

More text
` + "```" + `
code2
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 8: Code block should have empty line before it"))
			})
		})

		Context("list item validation", func() {
			It("warns when first list item has no empty line before", func() {
				content := `Some text
- List item
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 2: First list item should have empty line before it"))
			})

			It("passes when first list item has empty line before", func() {
				content := `Some text

- List item
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})

			It("handles different list markers", func() {
				content := `Text
- Dash item
Text
* Star item
Text
+ Plus item
Text
1. Ordered item
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(result.Details["errors"]).To(ContainSubstring("Line 2: First list item"))
				Expect(result.Details["errors"]).To(ContainSubstring("Line 4: First list item"))
				Expect(result.Details["errors"]).To(ContainSubstring("Line 6: First list item"))
				Expect(result.Details["errors"]).To(ContainSubstring("Line 8: First list item"))
			})

			It("handles indented list items", func() {
				content := `Text
  - Indented item
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(result.Details["errors"]).To(ContainSubstring("Line 2: First list item"))
			})

			It("does not warn for consecutive list items", func() {
				content := `
- Item 1
- Item 2
- Item 3
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})
		})

		Context("header validation", func() {
			It("warns when header has no empty line after", func() {
				content := `# Header
Text immediately after
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 1: Header should have empty line after it"))
			})

			It("passes when header has empty line after", func() {
				content := `# Header

Text after empty line
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})

			It("handles different header levels", func() {
				content := `# H1
Text
## H2
Text
### H3
Text
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 1: Header should have empty line after it"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 3: Header should have empty line after it"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 5: Header should have empty line after it"))
			})
		})

		Context("edge cases", func() {
			It("skips validation for Edit operations", func() {
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = "/path/to/file.md"
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("skips validation when no content available", func() {
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles truncation of long lines in warnings", func() {
				longLine := strings.Repeat("x", 100)
				content := longLine + "\n- List item\n"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Previous line: '" + strings.Repeat("x", 60)))
				Expect(result.Details["errors"]).NotTo(ContainSubstring(strings.Repeat("x", 70)))
			})

			It("handles empty lines properly", func() {
				content := `
` + "```" + `
code
` + "```" + `
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})
		})

		Context("complex scenarios", func() {
			It("handles mixed formatting issues", func() {
				content := `# Title
Immediate text
- List without space
` + "```" + `
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 1: Header should have empty line after it"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 3: First list item should have empty line before it"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 4: Code block should have empty line before it"))
			})

			It("handles real-world markdown example", func() {
				content := `# Project Title

## Overview

This is a description.

## Features

- Feature 1
- Feature 2
- Feature 3

## Installation

` + "```" + `bash
npm install
` + "```" + `

## Usage

1. Step one
2. Step two

Done!
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(BeEmpty())
			})
		})

		Context("code block indentation in lists", func() {
			It("warns when code block in numbered list has partial indentation", func() {
				content := `1. First item

 ` + "```" + `bash
 code
 ` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 3: Code block in list item should be indented by at least 3 spaces"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Found: 1 spaces, expected: at least 3 spaces"))
			})

			It("passes when code block in numbered list is properly indented", func() {
				content := `1. First item

   ` + "```" + `bash
   code
   ` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("warns when code block in bulleted list has partial indentation", func() {
				content := `- First item

 ` + "```" + `bash
 code
 ` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 3: Code block in list item should be indented by at least 2 spaces"))
			})

			It("passes when code block in bulleted list is properly indented", func() {
				content := `- First item

  ` + "```" + `bash
  code
  ` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles multi-digit numbered lists with partial indentation", func() {
				content := `10. First item

  ` + "```" + `bash
  code
  ` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 3: Code block in list item should be indented by at least 4 spaces"))
			})

			It("passes when code block has extra indentation", func() {
				content := `1. First item

     ` + "```" + `bash
     code
     ` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("does not warn for code blocks outside lists", func() {
				content := `Some text

` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("does not warn when code block immediately follows list without empty line", func() {
				content := `1. First item
` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				// Should warn about missing empty line before code block, but NOT about indentation
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Code block should have empty line before it"))
				Expect(result.Details["errors"]).NotTo(ContainSubstring("indented"))
			})
		})

		Context("multiple empty lines before code block", func() {
			It("warns when two empty lines before code block", func() {
				content := `Some text


` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("Markdown formatting errors"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Line 4: Code block should have only one empty line before it, not multiple"))
			})

			It("passes when only one empty line before code block", func() {
				content := `Some text

` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("warns when three empty lines before code block", func() {
				content := `Some text



` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Code block should have only one empty line before it, not multiple"))
			})

			It("handles multiple empty lines after header", func() {
				content := `## Header


` + "```" + `bash
code
` + "```"
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Code block should have only one empty line before it, not multiple"))
			})
		})
	})
})
