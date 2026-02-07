package mdtable_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/mdtable"
)

var _ = Describe("Parser", func() {
	Describe("Parse", func() {
		It("parses a simple table", func() {
			content := `| Name | Age |
| ---- | --- |
| John | 30  |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Tables[0].Headers).To(Equal([]string{"Name", "Age"}))
			Expect(result.Tables[0].Rows).To(HaveLen(1))
			Expect(result.Tables[0].Rows[0]).To(Equal([]string{"John", "30"}))
		})

		It("parses table with alignment markers", func() {
			content := `| Left | Center | Right |
|:-----|:------:|------:|
| A    | B      | C     |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Tables[0].Alignments).To(Equal([]mdtable.Alignment{
				mdtable.AlignLeft,
				mdtable.AlignCenter,
				mdtable.AlignRight,
			}))
		})

		It("parses multiple tables", func() {
			content := `# Section 1

| A | B |
|---|---|
| 1 | 2 |

# Section 2

| X | Y |
|---|---|
| 3 | 4 |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(2))
			Expect(result.Tables[0].Headers).To(Equal([]string{"A", "B"}))
			Expect(result.Tables[1].Headers).To(Equal([]string{"X", "Y"}))
		})

		It("handles table with no data rows", func() {
			content := `| Header |
|--------|`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Tables[0].Rows).To(BeEmpty())
		})

		It("handles escaped pipes in cells", func() {
			content := `| Name | Data |
|------|------|
| Test | A\|B |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Tables[0].Rows[0][1]).To(Equal("A|B"))
		})

		It("returns empty result for content without tables", func() {
			content := `# Just a heading

Some paragraph text.

- A list item`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(BeEmpty())
		})

		It("ignores invalid table-like content", func() {
			content := `| This looks like a table
But it's not complete`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(BeEmpty())
		})

		It("records table line numbers", func() {
			content := `Line 1
Line 2
| A | B |
|---|---|
| 1 | 2 |
Line 6`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Tables[0].StartLine).To(Equal(3))
			Expect(result.Tables[0].EndLine).To(Equal(5))
		})
	})

	Describe("FormatTable", func() {
		It("formats a parsed table", func() {
			content := `|Name|Age|
|---|---|
|John|30|`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))

			formatted := mdtable.FormatTable(&result.Tables[0])

			// Should have proper spacing
			Expect(formatted).To(ContainSubstring("| Name |"))
			Expect(formatted).To(ContainSubstring("| John |"))
		})

		It("preserves alignments when formatting", func() {
			content := `|Left|Right|
|:---|---:|
|A|B|`

			result := mdtable.Parse(content)
			formatted := mdtable.FormatTable(&result.Tables[0])

			Expect(formatted).To(ContainSubstring(":"))
		})
	})

	Describe("FindAndFormatTables", func() {
		It("returns formatted tables by start line", func() {
			content := `| A | B |
|---|---|
| 1 | 2 |`

			formatted := mdtable.FindAndFormatTables(content)

			Expect(formatted).To(HaveKey(1))
			Expect(formatted[1]).To(ContainSubstring("| A"))
			Expect(formatted[1]).To(ContainSubstring("| B"))
		})

		It("returns multiple formatted tables", func() {
			content := `| A | B |
|---|---|
| 1 | 2 |

| X | Y |
|---|---|
| 3 | 4 |`

			formatted := mdtable.FindAndFormatTables(content)

			Expect(formatted).To(HaveLen(2))
		})
	})
})

var _ = Describe("Table issue detection", func() {
	Describe("column count mismatch", func() {
		It("detects when data row has fewer columns", func() {
			content := `| A | B | C |
|---|---|---|
| 1 | 2 |`

			result := mdtable.Parse(content)

			Expect(result.Issues).NotTo(BeEmpty())

			var found bool

			for _, issue := range result.Issues {
				if issue.Message == "Row column count doesn't match header" {
					found = true

					break
				}
			}

			Expect(found).To(BeTrue())
		})
	})
})

var _ = Describe("Complex table parsing", func() {
	It("handles table with code in cells", func() {
		content := "| Function | Usage |\n|----------|-------|\n| `map` | `map(fn)` |"

		result := mdtable.Parse(content)

		Expect(result.Tables).To(HaveLen(1))
		Expect(result.Tables[0].Rows[0][0]).To(Equal("`map`"))
	})

	It("handles table with links", func() {
		content := `| Name | Link |
|------|------|
| Docs | [Link](https://klaudiu.sh) |`

		result := mdtable.Parse(content)

		Expect(result.Tables).To(HaveLen(1))
		Expect(result.Tables[0].Rows[0][1]).To(Equal("[Link](https://klaudiu.sh)"))
	})

	It("handles table with emoji", func() {
		content := `| Status | Name |
|--------|------|
| ✅ | Done |
| ❌ | Failed |`

		result := mdtable.Parse(content)

		Expect(result.Tables).To(HaveLen(1))
		Expect(result.Tables[0].Rows).To(HaveLen(2))
	})
})

var _ = Describe("hasInconsistentSpacing", func() {
	Describe("separator rows", func() {
		It("does not flag separator rows as having inconsistent spacing", func() {
			content := `| Phase | Item                      | Status         |
|:------|:--------------------------|:---------------|
| 1.1   | enumer                    | ✅ Complete    |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Issues).To(BeEmpty(), "separator row should not trigger spacing warning")
		})

		It("does not flag separator with varying dash lengths", func() {
			content := `| A | B |
|:--|:---------------------------|
| x | y |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Issues).To(BeEmpty())
		})

		It("does not flag right-aligned separator", func() {
			content := `| Num | Name |
|----:|:-----|
| 1   | test |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Issues).To(BeEmpty())
		})

		It("does not flag center-aligned separator", func() {
			content := `| Col |
|:---:|
| val |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(result.Issues).To(BeEmpty())
		})
	})

	Describe("data rows", func() {
		It("flags data rows without leading space", func() {
			content := `| A | B |
|---|---|
|x  | y |`

			result := mdtable.Parse(content)

			var found bool

			for _, issue := range result.Issues {
				if issue.Message == "Inconsistent spacing in table row" {
					found = true

					break
				}
			}

			Expect(found).To(BeTrue(), "should detect missing leading space")
		})

		It("flags data rows without trailing space", func() {
			content := `| A | B |
|---|---|
| x| y |`

			result := mdtable.Parse(content)

			var found bool

			for _, issue := range result.Issues {
				if issue.Message == "Inconsistent spacing in table row" {
					found = true

					break
				}
			}

			Expect(found).To(BeTrue(), "should detect missing trailing space")
		})

		It("accepts properly spaced data rows", func() {
			content := `| A | B |
|---|---|
| x | y |`

			result := mdtable.Parse(content)

			for _, issue := range result.Issues {
				Expect(issue.Message).NotTo(Equal("Inconsistent spacing in table row"))
			}
		})
	})

	Describe("substring cell content edge cases", func() {
		It("handles cells that are substrings of other cells", func() {
			// This was the original bug: ":---------------" is substring of ":--------------------------"
			content := `| Phase | Item                      | Status         |
|:------|:--------------------------|:---------------|
| 1.1   | enumer                    | ✅ Complete    |
| 1.2   | slog                      | ⏳ Pending     |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			Expect(
				result.Issues,
			).To(BeEmpty(), "should not flag false positives from substring matching")
		})

		It("handles data cells that are substrings of other cells", func() {
			content := `| Name | Description |
|------|-------------|
| foo  | foo bar baz |
| bar  | bar         |`

			result := mdtable.Parse(content)

			Expect(result.Tables).To(HaveLen(1))
			// Should not have spacing issues for properly formatted table
			for _, issue := range result.Issues {
				Expect(issue.Message).NotTo(Equal("Inconsistent spacing in table row"))
			}
		})
	})
})
