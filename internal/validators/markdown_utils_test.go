package validators_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/claude-hooks/internal/validators"
)

var _ = Describe("MarkdownState", func() {
	Describe("DetectMarkdownState", func() {
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
})
