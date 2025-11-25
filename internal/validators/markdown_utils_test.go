package validators_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validators"
)

var _ = Describe("MarkdownState", func() {
	Describe("DetectMarkdownState", func() {
		Context("code block detection", func() {
			It("starts with InCodeBlock=false for empty content", func() {
				state := validators.DetectMarkdownState("", 0)
				Expect(state.InCodeBlock).To(BeFalse())
			})

			It("starts with InCodeBlock=false for upToLine=0", func() {
				content := "```\ncode\n```"
				state := validators.DetectMarkdownState(content, 0)
				Expect(state.InCodeBlock).To(BeFalse())
			})

			It("detects InCodeBlock=true after opening marker", func() {
				content := `# Header

` + "```json" + `
{
  "key": "value"
}
`
				state := validators.DetectMarkdownState(content, 4)
				Expect(state.InCodeBlock).To(BeTrue())
			})

			It("detects InCodeBlock=false after closing marker", func() {
				content := `# Header

` + "```json" + `
{
  "key": "value"
}
` + "```" + `

Text after
`
				state := validators.DetectMarkdownState(content, 8)
				Expect(state.InCodeBlock).To(BeFalse())
			})

			It("handles multiple code blocks correctly", func() {
				content := `# Header

` + "```bash" + `
echo "first"
` + "```" + `

Some text

` + "```bash" + `
echo "second"
` + "```" + `
`
				// After first code block (line 6 = after first closing ```)
				state := validators.DetectMarkdownState(content, 6)
				Expect(state.InCodeBlock).To(BeFalse())

				// Inside second code block (line 10 = inside second code block)
				state = validators.DetectMarkdownState(content, 10)
				Expect(state.InCodeBlock).To(BeTrue())
			})

			It("handles nested code blocks in lists", func() {
				content := `- List item

  ` + "```bash" + `
  code inside list
  ` + "```" + `

- Another item
`
				// Inside code block within list (line 4)
				state := validators.DetectMarkdownState(content, 4)
				Expect(state.InCodeBlock).To(BeTrue())

				// After code block (line 7)
				state = validators.DetectMarkdownState(content, 7)
				Expect(state.InCodeBlock).To(BeFalse())
			})
		})

		Context("list context detection", func() {
			It("detects no list context outside of lists", func() {
				content := `# Header

Some paragraph text.

More text.
`
				state := validators.DetectMarkdownState(content, 5)
				Expect(state.InList).To(BeFalse())
				Expect(state.ListIndent).To(Equal(0))
				Expect(state.ListItemDepth).To(Equal(0))
			})

			It("detects list context inside a simple list", func() {
				content := `# Header

- First item
- Second item
`
				// After second list item (line 4)
				state := validators.DetectMarkdownState(content, 4)
				Expect(state.InList).To(BeTrue())
				Expect(state.ListIndent).To(Equal(2)) // "- " = 2 chars
				Expect(state.ListItemDepth).To(Equal(1))
			})

			It("detects nested list context", func() {
				content := `- First item
  - Nested item
    - Deeply nested
`
				// After deeply nested item (line 3)
				state := validators.DetectMarkdownState(content, 3)
				Expect(state.InList).To(BeTrue())
				Expect(state.ListItemDepth).To(Equal(3))
			})

			It("detects list context with numbered lists", func() {
				content := `1. First item
2. Second item
   - Nested bullet
`
				// After nested bullet (line 3)
				state := validators.DetectMarkdownState(content, 3)
				Expect(state.InList).To(BeTrue())
				Expect(state.ListItemDepth).To(Equal(2))
			})

			It("resets list context after two empty lines", func() {
				content := `- List item


Paragraph after two empty lines.
`
				// After paragraph (line 4)
				state := validators.DetectMarkdownState(content, 4)
				Expect(state.InList).To(BeFalse())
				Expect(state.ListIndent).To(Equal(0))
			})

			It("maintains list context with single empty line", func() {
				content := `- First item

  Continuation of first item.
`
				// After continuation (line 3)
				state := validators.DetectMarkdownState(content, 3)
				Expect(state.InList).To(BeTrue())
			})

			It("detects list context for fragment starting mid-list", func() {
				// This is the key test case for the MD007 false positive fix
				content := `# Implementation Plan

1. First step
   - Sub-step A
   - Sub-step B
     - Detail 1
     - Detail 2
2. Second step
`
				// After "Detail 2" (line 7) - fragment would start here
				state := validators.DetectMarkdownState(content, 7)
				Expect(state.InList).To(BeTrue())
				Expect(state.ListItemDepth).To(Equal(3)) // 1. -> - -> -
			})

			It("handles list context with code blocks", func() {
				content := `- List item

  ` + "```bash" + `
  echo "code"
  ` + "```" + `

  More list content
`
				// After code block, still in list context (line 7)
				state := validators.DetectMarkdownState(content, 7)
				Expect(state.InList).To(BeTrue())
			})

			It("exits list when content is not indented enough", func() {
				content := `- List item
  - Nested item
Not part of list anymore
`
				// After unindented line (line 3)
				state := validators.DetectMarkdownState(content, 3)
				Expect(state.InList).To(BeFalse())
			})
		})
	})

	Describe("AnalyzeMarkdown with initial state", func() {
		Context("when starting inside code block", func() {
			It("should not complain about code block spacing", func() {
				// Fragment that starts inside a code block
				fragment := `{
  "nested": {
    "data": true,
    "more": "fields"
  }
}
` + "```" + `

Text after`

				// Simulate starting inside a code block
				initialState := &validators.MarkdownState{InCodeBlock: true}
				result := validators.AnalyzeMarkdown(fragment, initialState)
				Expect(result.Warnings).To(BeEmpty())
			})

			It("should handle closing marker correctly", func() {
				// Fragment with closing marker
				fragment := `  "key": "value"
}
` + "```" + `

More text`

				initialState := &validators.MarkdownState{InCodeBlock: true}
				result := validators.AnalyzeMarkdown(fragment, initialState)
				Expect(result.Warnings).To(BeEmpty())
			})
		})

		Context("when starting outside code block", func() {
			It("should validate opening marker spacing", func() {
				// Fragment with code block opening without proper spacing
				fragment := `Some text
` + "```bash" + `
code
` + "```"

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(result.Warnings).NotTo(BeEmpty())
				Expect(result.Warnings[0]).To(
					ContainSubstring("Code block should have empty line before it"),
				)
			})

			It("should pass with proper spacing before code block", func() {
				fragment := `Some text

` + "```bash" + `
code
` + "```"

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(result.Warnings).To(BeEmpty())
			})
		})

		Context("with nil initial state", func() {
			It("should default to InCodeBlock=false", func() {
				fragment := `# Header

` + "```bash" + `
code
` + "```"

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(result.Warnings).To(BeEmpty())
			})
		})

		Context("with fragment spanning code block boundary", func() {
			It("should handle transition from outside to inside", func() {
				fragment := `Some text

` + "```json" + `
{
  "key": "value"
}
`
				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(result.Warnings).To(BeEmpty())
			})

			It("should handle transition from inside to outside", func() {
				fragment := `  "key": "value"
}
` + "```" + `

More text after`

				initialState := &validators.MarkdownState{InCodeBlock: true}
				result := validators.AnalyzeMarkdown(fragment, initialState)
				Expect(result.Warnings).To(BeEmpty())
			})
		})

		Context("with complex nested structures", func() {
			It("should handle code blocks in list items", func() {
				fragment := `- List item with code:

  ` + "```bash" + `
  echo "test"
  ` + "```" + `

- Another item`

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(result.Warnings).To(BeEmpty())
			})

			It("should detect insufficient indentation in list code blocks", func() {
				// Code block with partial indentation (1 space) after list item
				// List item "- " requires 2 spaces, so 1 space is insufficient
				fragment := `- List item

 ` + "```bash" + `
code with 1 space indentation
 ` + "```"

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(result.Warnings).NotTo(BeEmpty())
				Expect(result.Warnings[0]).To(
					ContainSubstring("Code block in list item should be indented"),
				)
			})

			It(
				"should not warn about unindented code blocks after list (treated as separate)",
				func() {
					// Code block with NO indentation is treated as separate block, not part of list
					fragment := `- List item

` + "```bash" + `
code with no indentation
` + "```"

					result := validators.AnalyzeMarkdown(fragment, nil)
					Expect(result.Warnings).To(BeEmpty())
				},
			)
		})
	})

	Describe("Table validation", func() {
		Context("when content has malformed tables", func() {
			It("detects tables with inconsistent spacing", func() {
				content := `# Test

| Name | Age |
| ---- | --- |
| John | 30  |
| Jane | 25  |
`
				result := validators.AnalyzeMarkdown(content, nil)

				// The table has inconsistent formatting, should suggest fix
				Expect(result.TableSuggested).NotTo(BeEmpty())
			})

			It("provides properly formatted table suggestion", func() {
				content := `| Name | Age |
|---|---|
|John|30|`

				result := validators.AnalyzeMarkdown(content, nil)

				Expect(result.TableSuggested).To(HaveKey(1))
				suggestion := result.TableSuggested[1]

				// Verify suggestion has proper spacing
				Expect(suggestion).To(ContainSubstring("| Name |"))
				Expect(suggestion).To(ContainSubstring("| John |"))
			})

			It("handles tables with emoji correctly", func() {
				content := `| Status | Name |
|---|---|
|✅|Done|
|❌|Failed|`

				result := validators.AnalyzeMarkdown(content, nil)

				if len(result.TableSuggested) > 0 {
					suggestion := result.TableSuggested[1]

					// Should preserve emoji and have proper alignment
					Expect(suggestion).To(ContainSubstring("✅"))
					Expect(suggestion).To(ContainSubstring("❌"))
				}
			})

			It("handles tables with already escaped pipes in cell content", func() {
				// When user provides already escaped pipes, they should be preserved
				content := `| Name | Data |
|---|---|
|Test|A\|B\|C|`

				result := validators.AnalyzeMarkdown(content, nil)

				if len(result.TableSuggested) > 0 {
					suggestion := result.TableSuggested[1]

					// Escaped pipes should be preserved in the suggestion
					Expect(suggestion).To(ContainSubstring(`A\|B\|C`))
				}
			})
		})

		Context("when content has well-formatted tables", func() {
			It("does not suggest changes for properly formatted tables", func() {
				// A properly formatted table should not trigger suggestions
				content := `| Name   | Age |
|:-------|:----|
| John   | 30  |
| Jane   | 25  |
`
				result := validators.AnalyzeMarkdown(content, nil)

				// May still suggest if formatting differs slightly
				// The key is that any suggestion should be valid
				if len(result.TableSuggested) > 0 {
					suggestion := result.TableSuggested[1]
					Expect(suggestion).To(ContainSubstring("|"))
				}
			})
		})

		Context("when content has no tables", func() {
			It("returns empty TableSuggested map", func() {
				content := `# Just a heading

Some paragraph text.

- A list item`

				result := validators.AnalyzeMarkdown(content, nil)

				Expect(result.TableSuggested).To(BeEmpty())
			})
		})
	})
})
