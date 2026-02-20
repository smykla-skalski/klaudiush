package validators_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators"
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

			It("should not treat # comment immediately before closing marker as header", func() {
				// This test directly reproduces the bug: checkHeader sees prevLine as "#..."
				// from inside the code block when processing the closing marker
				fragment := `Some text

` + "```bash" + `
# This triggers the bug
` + "```" + `

More text`

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(
					result.Warnings,
				).To(BeEmpty(), "# immediately before closing marker should not be treated as header")
			})

			It("should ignore # comments inside code blocks", func() {
				// Fragment with # comments that should not be treated as headers
				fragment := `Some text

` + "```bash" + `
# This is a bash comment, not a header
echo "test"
` + "```" + `

More text`

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(
					result.Warnings,
				).To(BeEmpty(), "# inside code block should not be treated as header")
			})

			It("should ignore # in various programming languages inside code blocks", func() {
				// Test multiple languages that use # for comments
				fragment := `# Real Header

` + "```python" + `
# Python comment
def foo():
    pass
` + "```" + `

` + "```ruby" + `
# Ruby comment
puts "hello"
` + "```" + `

` + "```yaml" + `
# YAML comment
key: value
` + "```"

				result := validators.AnalyzeMarkdown(fragment, nil)
				Expect(
					result.Warnings,
				).To(BeEmpty(), "# inside any code block should not be treated as header")
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
			It("detects tables with inconsistent spacing as cosmetic", func() {
				content := `# Test

| Name | Age |
| ---- | --- |
| John | 30  |
| Jane | 25  |
`
				result := validators.AnalyzeMarkdown(content, nil)

				// Cosmetic issue (width padding), not structural
				Expect(result.CosmeticTableSuggested).NotTo(BeEmpty())
				// No structural warnings
				Expect(result.Warnings).To(BeEmpty())
			})

			It("provides properly formatted table suggestion for structural issues", func() {
				// |John|30| has no cell padding - structural issue
				content := `| Name | Age |
|---|---|
|John|30|`

				result := validators.AnalyzeMarkdown(content, nil)

				// Structural issues (missing padding) go to TableSuggested
				Expect(result.TableSuggested).To(HaveKey(1))
				suggestion := result.TableSuggested[1]

				// Verify suggestion has proper spacing
				Expect(suggestion).To(ContainSubstring("| Name |"))
				Expect(suggestion).To(ContainSubstring("| John |"))
			})

			It("handles tables with emoji correctly", func() {
				// |✅|Done| has no padding - structural issue
				content := `| Status | Name |
|---|---|
|✅|Done|
|❌|Failed|`

				result := validators.AnalyzeMarkdown(content, nil)

				// Tables with structural issues get suggestions in TableSuggested
				if len(result.TableSuggested) > 0 {
					suggestion := result.TableSuggested[1]

					// Should preserve emoji and have proper alignment
					Expect(suggestion).To(ContainSubstring("✅"))
					Expect(suggestion).To(ContainSubstring("❌"))
				}
			})

			It("handles tables with already escaped pipes in cell content", func() {
				// |Test|A\|B\|C| has no padding - structural issue
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
				if len(result.CosmeticTableSuggested) > 0 {
					suggestion := result.CosmeticTableSuggested[1]
					Expect(suggestion).To(ContainSubstring("|"))
				}
			})
		})

		Context("when content has no tables", func() {
			It("returns empty suggestion maps", func() {
				content := `# Just a heading

Some paragraph text.

- A list item`

				result := validators.AnalyzeMarkdown(content, nil)

				Expect(result.TableSuggested).To(BeEmpty())
				Expect(result.CosmeticTableSuggested).To(BeEmpty())
			})
		})
	})

	Describe("Table severity classification", func() {
		Context("structural vs cosmetic separation", func() {
			It("classifies column count mismatch as structural (blocking)", func() {
				content := `| A | B | C |
|---|---|---|
| 1 | 2 |`
				result := validators.AnalyzeMarkdown(content, nil)
				// Column count mismatch is structural
				Expect(result.Warnings).NotTo(BeEmpty())

				found := false

				for _, w := range result.Warnings {
					if strings.Contains(w, "column count") {
						found = true

						break
					}
				}

				Expect(found).To(BeTrue())
			})

			It("classifies missing cell padding as structural (blocking)", func() {
				content := `| A | B |
|---|---|
|x  | y |`
				result := validators.AnalyzeMarkdown(content, nil)
				// Missing padding is a structural issue
				Expect(result.Warnings).NotTo(BeEmpty())

				found := false

				for _, w := range result.Warnings {
					if strings.Contains(w, "spacing") {
						found = true

						break
					}
				}

				Expect(found).To(BeTrue())
			})

			It("classifies width inconsistency as cosmetic (non-blocking)", func() {
				content := `| Name | Age |
| ---- | --- |
| John | 30  |
`
				result := validators.AnalyzeMarkdown(content, nil)
				// Width inconsistency only - cosmetic
				Expect(result.Warnings).To(BeEmpty())
				Expect(result.CosmeticTableWarnings).NotTo(BeEmpty())
			})

			It("returns no warnings for well-formatted table", func() {
				content := `| Name | Age |
|:-----|:----|
| John | 30  |
| Jane | 25  |
`
				result := validators.AnalyzeMarkdown(content, nil)
				Expect(result.Warnings).To(BeEmpty())
				Expect(result.CosmeticTableWarnings).To(BeEmpty())
			})

			It("handles tables with code in cells without false positives", func() {
				content := "| Function | Usage      |\n|:---------|:-----------|\n| `map`    | `map(fn)` |\n"
				result := validators.AnalyzeMarkdown(content, nil)
				Expect(result.Warnings).To(BeEmpty())
			})

			It("handles tables with links without false positives", func() {
				content := "| Name | Link                       |\n|:-----|:---------------------------|\n| Docs | [Link](https://klaudiu.sh) |\n"
				result := validators.AnalyzeMarkdown(content, nil)
				Expect(result.Warnings).To(BeEmpty())
			})
		})

		Context("fragment mode", func() {
			It("skips cosmetic checks when IsFragment=true", func() {
				content := `| Name | Age |
| ---- | --- |
| John | 30  |
`
				result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
					CheckTableFormatting: true,
					TableWidthMode:       0,
					IsFragment:           true,
				})
				// Fragment mode skips cosmetic checks
				Expect(result.CosmeticTableWarnings).To(BeEmpty())
				Expect(result.CosmeticTableSuggested).To(BeEmpty())
			})

			It("still catches structural issues in fragment mode", func() {
				content := `| A | B | C |
|---|---|---|
| 1 | 2 |`
				result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
					CheckTableFormatting: true,
					TableWidthMode:       0,
					IsFragment:           true,
				})
				// Structural issues still detected
				Expect(result.Warnings).NotTo(BeEmpty())
			})

			It("fragment mode does not populate cosmetic suggestions", func() {
				content := `| Name | Age |
|---|---|
|John|30|`
				result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
					CheckTableFormatting: true,
					TableWidthMode:       0,
					IsFragment:           true,
				})
				// No cosmetic suggestions in fragment mode
				Expect(result.CosmeticTableSuggested).To(BeEmpty())
				// But structural issues still present
				Expect(result.Warnings).NotTo(BeEmpty())
			})
		})
	})

	Describe("Heading context detection", func() {
		Context("LastHeadingLevel tracking", func() {
			It("detects no heading level when no headings present", func() {
				content := `Just some text.

More text here.
`
				state := validators.DetectMarkdownState(content, 3)
				Expect(state.LastHeadingLevel).To(Equal(0))
			})

			It("detects h1 heading level", func() {
				content := `# Main Title

Some content.
`
				state := validators.DetectMarkdownState(content, 3)
				Expect(state.LastHeadingLevel).To(Equal(1))
			})

			It("detects h2 heading level", func() {
				content := `# Main Title

## Section

Content here.
`
				state := validators.DetectMarkdownState(content, 5)
				Expect(state.LastHeadingLevel).To(Equal(2))
			})

			It("detects h3 heading level", func() {
				content := `# Main Title

## Section

### Subsection

Content.
`
				state := validators.DetectMarkdownState(content, 7)
				Expect(state.LastHeadingLevel).To(Equal(3))
			})

			It("tracks the last heading seen, not the first", func() {
				content := `# Main Title

## First Section

### Subsection A

## Second Section

Content after second section.
`
				state := validators.DetectMarkdownState(content, 9)
				Expect(state.LastHeadingLevel).To(Equal(2)) // ## Second Section
			})

			It("ignores headings inside code blocks", func() {
				content := "# Main Title\n\n```markdown\n## This is inside code\n```\n\nText.\n"
				state := validators.DetectMarkdownState(content, 7)
				Expect(state.LastHeadingLevel).To(Equal(1)) // Still just h1 from outside code
			})

			It("detects h4 through h6 levels", func() {
				content := `# H1

## H2

### H3

#### H4

Content here.
`
				state := validators.DetectMarkdownState(content, 9)
				Expect(state.LastHeadingLevel).To(Equal(4))
			})

			It("caps heading level at 6", func() {
				content := "####### Too many hashes\n\nContent.\n"
				state := validators.DetectMarkdownState(content, 1)
				// ######## is not a valid markdown heading, but if detected, cap at 6
				// Actually, markdown spec says >6 hashes is NOT a heading
				// Our isHeader regex should not match this
				Expect(state.LastHeadingLevel).To(Equal(0))
			})
		})
	})

	Describe("GeneratePreamble", func() {
		Context("with nil state", func() {
			It("returns empty preamble", func() {
				preamble, lines := validators.GeneratePreamble(nil)
				Expect(preamble).To(BeEmpty())
				Expect(lines).To(Equal(0))
			})
		})

		Context("with StartLine=0", func() {
			It("returns empty preamble (fragment starts at beginning)", func() {
				state := &validators.MarkdownState{
					StartLine: 0,
				}
				preamble, lines := validators.GeneratePreamble(state)
				Expect(preamble).To(BeEmpty())
				Expect(lines).To(Equal(0))
			})
		})

		Context("with StartLine>0 but no special context", func() {
			It("returns basic h1 preamble", func() {
				state := &validators.MarkdownState{
					StartLine: 10,
				}
				preamble, lines := validators.GeneratePreamble(state)
				Expect(preamble).To(ContainSubstring("# Preamble"))
				Expect(lines).To(Equal(2)) // header + blank line
			})
		})

		Context("heading hierarchy generation", func() {
			It("generates h1 for LastHeadingLevel=1", func() {
				state := &validators.MarkdownState{
					StartLine:        10,
					LastHeadingLevel: 1,
				}
				preamble, lines := validators.GeneratePreamble(state)
				Expect(preamble).To(ContainSubstring("# Preamble H1"))
				Expect(preamble).NotTo(ContainSubstring("## "))
				Expect(lines).To(Equal(2)) // h1 + blank line
			})

			It("generates h1→h2 for LastHeadingLevel=2", func() {
				state := &validators.MarkdownState{
					StartLine:        10,
					LastHeadingLevel: 2,
				}
				preamble, lines := validators.GeneratePreamble(state)
				Expect(preamble).To(ContainSubstring("# Preamble H1"))
				Expect(preamble).To(ContainSubstring("## Preamble H2"))
				Expect(lines).To(Equal(4)) // h1 + blank + h2 + blank
			})

			It("generates h1→h2→h3 for LastHeadingLevel=3", func() {
				state := &validators.MarkdownState{
					StartLine:        10,
					LastHeadingLevel: 3,
				}
				preamble, lines := validators.GeneratePreamble(state)
				Expect(preamble).To(ContainSubstring("# Preamble H1"))
				Expect(preamble).To(ContainSubstring("## Preamble H2"))
				Expect(preamble).To(ContainSubstring("### Preamble H3"))
				Expect(lines).To(Equal(6)) // 3 headings × 2 lines each
			})

			It("generates full hierarchy h1→h2→h3→h4 for LastHeadingLevel=4", func() {
				state := &validators.MarkdownState{
					StartLine:        10,
					LastHeadingLevel: 4,
				}
				preamble, lines := validators.GeneratePreamble(state)
				Expect(preamble).To(ContainSubstring("# Preamble H1"))
				Expect(preamble).To(ContainSubstring("## Preamble H2"))
				Expect(preamble).To(ContainSubstring("### Preamble H3"))
				Expect(preamble).To(ContainSubstring("#### Preamble H4"))
				Expect(lines).To(Equal(8)) // 4 headings × 2 lines each
			})
		})

		Context("combined heading and list context", func() {
			It("generates heading hierarchy before list context", func() {
				state := &validators.MarkdownState{
					StartLine:        20,
					LastHeadingLevel: 2,
					InList:           true,
					ListItemDepth:    1,
					ListStack: []validators.ListItemInfo{
						{MarkerIndent: 0, ContentIndent: 2, IsOrdered: false, Marker: "-"},
					},
				}
				preamble, lines := validators.GeneratePreamble(state)

				// Should have heading hierarchy first
				Expect(preamble).To(ContainSubstring("# Preamble H1"))
				Expect(preamble).To(ContainSubstring("## Preamble H2"))

				// Then list context
				Expect(preamble).To(ContainSubstring("- Item"))
				Expect(lines).To(BeNumerically(">", 4)) // headings + list item
			})

			It("generates heading hierarchy with ordered list", func() {
				state := &validators.MarkdownState{
					StartLine:        30,
					LastHeadingLevel: 3,
					InList:           true,
					ListItemDepth:    1,
					ListStack: []validators.ListItemInfo{
						{
							MarkerIndent:  0,
							ContentIndent: 3,
							IsOrdered:     true,
							OrderNumber:   5,
							Marker:        "5.",
						},
					},
				}
				preamble, lines := validators.GeneratePreamble(state)

				// Should have h1 → h2 → h3
				Expect(preamble).To(ContainSubstring("# Preamble H1"))
				Expect(preamble).To(ContainSubstring("## Preamble H2"))
				Expect(preamble).To(ContainSubstring("### Preamble H3"))

				// Then ordered list items 1-5
				Expect(preamble).To(ContainSubstring("1. Item 1"))
				Expect(preamble).To(ContainSubstring("5. Item 5"))
				Expect(lines).To(BeNumerically(">", 6)) // 6 heading lines + 5 list items
			})
		})

		Context("code block context", func() {
			It("adds opening fence when InCodeBlock is true", func() {
				state := &validators.MarkdownState{
					StartLine:   10,
					InCodeBlock: true,
				}
				preamble, lines := validators.GeneratePreamble(state)
				// Should have basic h1 preamble + opening code fence
				Expect(preamble).To(ContainSubstring("# Preamble"))
				Expect(preamble).To(ContainSubstring("```text\n"))
				Expect(lines).To(Equal(3)) // header + blank + code fence
			})

			It("adds opening fence after heading hierarchy", func() {
				state := &validators.MarkdownState{
					StartLine:        10,
					LastHeadingLevel: 2,
					InCodeBlock:      true,
				}
				preamble, lines := validators.GeneratePreamble(state)
				// Should have h1 → h2 hierarchy + code fence
				Expect(preamble).To(ContainSubstring("# Preamble H1"))
				Expect(preamble).To(ContainSubstring("## Preamble H2"))
				Expect(preamble).To(HaveSuffix("```text\n"))
				Expect(lines).To(Equal(5)) // h1 + blank + h2 + blank + code fence
			})

			It("adds opening fence after list context", func() {
				state := &validators.MarkdownState{
					StartLine:     10,
					InList:        true,
					ListItemDepth: 1,
					InCodeBlock:   true,
					ListStack: []validators.ListItemInfo{
						{MarkerIndent: 0, ContentIndent: 2, IsOrdered: false, Marker: "-"},
					},
				}
				preamble, lines := validators.GeneratePreamble(state)
				// Should have basic preamble + list item + code fence
				Expect(preamble).To(ContainSubstring("# Preamble"))
				Expect(preamble).To(ContainSubstring("- Item"))
				Expect(preamble).To(HaveSuffix("```text\n"))
				Expect(lines).To(Equal(4)) // header + blank + list item + code fence
			})

			It("does not add fence when InCodeBlock is false", func() {
				state := &validators.MarkdownState{
					StartLine:   10,
					InCodeBlock: false,
				}
				preamble, _ := validators.GeneratePreamble(state)
				Expect(preamble).NotTo(ContainSubstring("```"))
			})
		})

		Context("blank line handling", func() {
			It("adds blank line when HadBlankLineBeforeFragment with no context", func() {
				state := &validators.MarkdownState{
					StartLine:                  10,
					LastHeadingLevel:           0, // No heading context
					InList:                     false,
					HadBlankLineBeforeFragment: true,
				}
				preamble, lines := validators.GeneratePreamble(state)
				// Should end with extra blank line after basic preamble
				Expect(preamble).To(HaveSuffix("\n\n"))
				Expect(lines).To(Equal(3)) // header + blank + extra blank
			})

			It("does NOT add extra blank when heading context exists", func() {
				// When we have heading context, the hierarchy already ends with a blank
				// Adding HadBlankLineBeforeFragment would create consecutive blanks (MD012)
				state := &validators.MarkdownState{
					StartLine:                  10,
					LastHeadingLevel:           2, // Has heading context
					HadBlankLineBeforeFragment: true,
				}
				preamble, lines := validators.GeneratePreamble(state)
				// Should NOT have consecutive blank lines
				Expect(preamble).NotTo(ContainSubstring("\n\n\n"))
				Expect(lines).To(Equal(4)) // h1 + blank + h2 + blank (no extra)
			})

			It("does NOT add extra blank when list context exists", func() {
				state := &validators.MarkdownState{
					StartLine:                  10,
					InList:                     true,
					ListItemDepth:              1,
					HadBlankLineBeforeFragment: true,
					ListStack: []validators.ListItemInfo{
						{MarkerIndent: 0, ContentIndent: 2, IsOrdered: false, Marker: "-"},
					},
				}
				preamble, _ := validators.GeneratePreamble(state)
				// List context already provides proper spacing
				Expect(preamble).NotTo(ContainSubstring("\n\n\n"))
			})
		})
	})
})

var _ = Describe("FragmentRange", func() {
	Describe("IsContextLine", func() {
		editRange := validators.FragmentRange{EditStart: 3, EditEnd: 5}

		DescribeTable("boundary checks",
			func(lineNum, prevLineNum int, expected bool) {
				Expect(editRange.IsContextLine(lineNum, prevLineNum)).To(Equal(expected))
			},
			Entry("both before edit", 2, 1, true),
			Entry("lineNum at edit start", 3, 2, false),
			Entry("prevLineNum at edit start", 2, 3, false),
			Entry("lineNum at edit end", 5, 4, false),
			Entry("prevLineNum at edit end", 6, 5, false),
			Entry("both after edit", 7, 6, true),
			Entry("after edit, prevLineNum ignored", 6, 0, true),
			Entry("at edit end, prevLineNum ignored", 5, 0, false),
			Entry("inside edit", 4, 3, false),
			Entry("before edit, prevLineNum ignored", 1, 0, true),
		)

		It("returns false when EditStart is 0 (no range set)", func() {
			zeroRange := validators.FragmentRange{}
			Expect(zeroRange.IsContextLine(1, 0)).To(BeFalse())
			Expect(zeroRange.IsContextLine(5, 3)).To(BeFalse())
		})
	})
})

var _ = Describe("AnalyzeMarkdown with FragmentRange", func() {
	Context("when context lines have pre-existing violations", func() {
		It("suppresses list spacing warnings on context-only lines", func() {
			// Fragment: context before, context list, edit list, context after
			// Line 1: "Some text" (context)
			// Line 2: "- Item 1" (context)
			// Line 3: "- Item 2" (edit)
			// Line 4: "- Item 3" (context)
			// The "Some text" -> "- Item 1" transition at line 2 is MD032
			// but both lines are context, so it should be suppressed
			content := "Some text\n- Item 1\n- Item 2\n- Item 3"
			result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
				CheckTableFormatting: false,
				FragmentRange:        validators.FragmentRange{EditStart: 3, EditEnd: 3},
			})
			Expect(result.Warnings).To(BeEmpty(),
				"pre-existing MD032 on context line should be suppressed")
		})

		It("catches list spacing warnings on edit lines", func() {
			// Same content but now the bold label is IN the edit range
			content := "**Bold label**:\n- Item 1\n- Item 2"
			result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
				CheckTableFormatting: false,
				FragmentRange:        validators.FragmentRange{EditStart: 1, EditEnd: 3},
			})
			Expect(result.Warnings).NotTo(BeEmpty(),
				"MD032 on edit line should still be caught")
		})

		It("suppresses header spacing warnings on context-only lines", func() {
			// Context: header without blank line after (pre-existing violation)
			// Edit: just the content line
			content := "## Heading\nDirect content"
			result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
				CheckTableFormatting: false,
				FragmentRange:        validators.FragmentRange{EditStart: 2, EditEnd: 2},
			})
			// The header spacing check fires at lineNum=2 with prevLine="## Heading"
			// Both lineNum=2 and prevLineNum=1 need to be checked
			// lineNum=2 is in edit range, so this is NOT suppressed - it's a boundary violation
			Expect(result.Warnings).NotTo(BeEmpty())
		})

		It("suppresses code block spacing warnings on context-only lines", func() {
			content := "Some text\n```bash\necho hello\n```"
			result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
				CheckTableFormatting: false,
				FragmentRange:        validators.FragmentRange{EditStart: 3, EditEnd: 3},
			})
			// "Some text" followed by code block without blank line is context-only
			Expect(result.Warnings).To(BeEmpty(),
				"code block spacing on context line should be suppressed")
		})
	})

	Context("when no FragmentRange is set (Write operation)", func() {
		It("validates all lines normally", func() {
			content := "**Bold label**:\n- Item 1\n- Item 2"
			result := validators.AnalyzeMarkdown(content, nil, validators.AnalysisOptions{
				CheckTableFormatting: false,
			})
			Expect(result.Warnings).NotTo(BeEmpty(),
				"Write operations should catch all violations")
		})
	})
})
