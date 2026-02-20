package file_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("ExtractEditFragment", func() {
	var log logger.Logger

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
	})

	Context("single-line edits", func() {
		It("extracts fragment with full context", func() {
			content := `line 1
line 2
line 3
line 4 to change
line 5
line 6
line 7`

			result := file.ExtractEditFragment(
				content,
				"line 4 to change",
				"line 4 changed",
				2,
				log,
			)

			expected := `line 2
line 3
line 4 changed
line 5
line 6`

			Expect(result).To(Equal(expected))
		})

		It("handles edits at the beginning with limited context before", func() {
			content := `line 1 to change
line 2
line 3
line 4
line 5`

			result := file.ExtractEditFragment(
				content,
				"line 1 to change",
				"line 1 changed",
				2,
				log,
			)

			expected := `line 1 changed
line 2
line 3`

			Expect(result).To(Equal(expected))
		})

		It("handles edits at the end with limited context after", func() {
			content := `line 1
line 2
line 3
line 4
line 5 to change`

			result := file.ExtractEditFragment(
				content,
				"line 5 to change",
				"line 5 changed",
				2,
				log,
			)

			expected := `line 3
line 4
line 5 changed`

			Expect(result).To(Equal(expected))
		})

		It("handles single line file", func() {
			content := `only line to change`

			result := file.ExtractEditFragment(
				content,
				"only line to change",
				"only line changed",
				2,
				log,
			)

			expected := `only line changed`

			Expect(result).To(Equal(expected))
		})

		It("handles partial line replacement", func() {
			content := `line 1
function foo() {
  return bar
}
line 5`

			result := file.ExtractEditFragment(
				content,
				"bar",
				"baz",
				2,
				log,
			)

			// Includes 2 lines before ("line 1" and "function foo() {")
			// and 2 lines after ("}" and "line 5")
			expected := `line 1
function foo() {
  return baz
}
line 5`

			Expect(result).To(Equal(expected))
		})
	})

	Context("multi-line edits", func() {
		It("extracts fragment for multi-line replacement", func() {
			content := `line 1
line 2
old line A
old line B
old line C
line 6
line 7`

			result := file.ExtractEditFragment(
				content,
				`old line A
old line B
old line C`,
				`new line A
new line B`,
				2,
				log,
			)

			expected := `line 1
line 2
new line A
new line B
line 6
line 7`

			Expect(result).To(Equal(expected))
		})

		It("handles multi-line edit at file beginning", func() {
			content := `old line 1
old line 2
line 3
line 4
line 5`

			result := file.ExtractEditFragment(
				content,
				`old line 1
old line 2`,
				`new line 1
new line 2`,
				2,
				log,
			)

			expected := `new line 1
new line 2
line 3
line 4`

			Expect(result).To(Equal(expected))
		})

		It("handles multi-line edit at file end", func() {
			content := `line 1
line 2
line 3
old line 4
old line 5`

			result := file.ExtractEditFragment(
				content,
				`old line 4
old line 5`,
				`new line 4
new line 5`,
				2,
				log,
			)

			expected := `line 2
line 3
new line 4
new line 5`

			Expect(result).To(Equal(expected))
		})
	})

	Context("context lines with function boundaries", func() {
		It("includes partial functions in context", func() {
			content := `func before() {
  doSomething()
}

func target() {
  old code
}

func after() {
  doOtherThing()
}`

			result := file.ExtractEditFragment(
				content,
				"  old code",
				"  new code",
				2,
				log,
			)

			// Includes 2 lines before and 2 lines after the changed line
			expected := `
func target() {
  new code
}
`

			Expect(result).To(Equal(expected))
		})

		It("handles edits within nested structures", func() {
			content := `type Config struct {
  Name string
  Value int
  OldField string
  Extra bool
}`

			result := file.ExtractEditFragment(
				content,
				"  OldField string",
				"  NewField string",
				2,
				log,
			)

			// Includes 2 lines before and 2 lines after
			expected := `  Name string
  Value int
  NewField string
  Extra bool
}`

			Expect(result).To(Equal(expected))
		})
	})

	Context("edge cases", func() {
		It("returns empty string when old string not found", func() {
			content := `line 1
line 2
line 3`

			result := file.ExtractEditFragment(
				content,
				"non-existent",
				"replacement",
				2,
				log,
			)

			Expect(result).To(BeEmpty())
		})

		It("handles empty lines in context", func() {
			content := `line 1

line 3
old content
line 5

line 7`

			result := file.ExtractEditFragment(
				content,
				"old content",
				"new content",
				2,
				log,
			)

			expected := `
line 3
new content
line 5
`

			Expect(result).To(Equal(expected))
		})

		It("handles indented content", func() {
			content := "  line 1\n    line 2\n      old line\n    4\n  line"

			result := file.ExtractEditFragment(
				content,
				"      old line",
				"      new line",
				2,
				log,
			)

			// Includes 2 lines before and 2 lines after
			expected := "  line 1\n    line 2\n      new line\n    4\n  line"

			Expect(result).To(Equal(expected))
		})

		It("handles content with special characters", func() {
			content := `line 1
line 2: old $value
line 3`

			result := file.ExtractEditFragment(
				content,
				"line 2: old $value",
				"line 2: new $value",
				1,
				log,
			)

			expected := `line 1
line 2: new $value
line 3`

			Expect(result).To(Equal(expected))
		})

		It("handles zero context lines", func() {
			content := "line 1\nline 2\nold line\n4\nline"

			result := file.ExtractEditFragment(
				content,
				"old line",
				"new line",
				0,
				log,
			)

			expected := `new line`

			Expect(result).To(Equal(expected))
		})

		It("handles context larger than file", func() {
			content := "line 1\nold line\n"

			result := file.ExtractEditFragment(
				content,
				"old line",
				"new line",
				10,
				log,
			)

			expected := "line 1\nnew line\n"

			Expect(result).To(Equal(expected))
		})
	})

	Context("markdown-specific scenarios", func() {
		It("includes context for heading spacing validation", func() {
			content := `# Heading 1

Some text
## Old Heading
More text

# Heading 2`

			result := file.ExtractEditFragment(
				content,
				"## Old Heading",
				"## New Heading",
				2,
				log,
			)

			expected := `
Some text
## New Heading
More text
`

			Expect(result).To(Equal(expected))
		})

		It("includes context for list validation", func() {
			content := `- Item 1
- Item 2
- Old item
- Item 4
- Item 5`

			result := file.ExtractEditFragment(
				content,
				"- Old item",
				"- New item",
				2,
				log,
			)

			expected := `- Item 1
- Item 2
- New item
- Item 4
- Item 5`

			Expect(result).To(Equal(expected))
		})
	})

	Context("shell script scenarios", func() {
		It("includes context for function validation", func() {
			content := `#!/bin/bash

function before() {
  echo "before"
}

function target() {
  old_command
}

function after() {
  echo "after"
}`

			result := file.ExtractEditFragment(
				content,
				"  old_command",
				"  new_command",
				2,
				log,
			)

			// Includes 2 lines before and 2 lines after
			expected := `
function target() {
  new_command
}
`

			Expect(result).To(Equal(expected))
		})

		It("includes context for variable assignment", func() {
			content := `VAR1="value1"
VAR2="value2"
OLD_VAR="old"
VAR3="value3"
VAR4="value4"`

			result := file.ExtractEditFragment(
				content,
				`OLD_VAR="old"`,
				`NEW_VAR="new"`,
				2,
				log,
			)

			expected := `VAR1="value1"
VAR2="value2"
NEW_VAR="new"
VAR3="value3"
VAR4="value4"`

			Expect(result).To(Equal(expected))
		})
	})
})

var _ = Describe("ExtractEditFragmentWithRange", func() {
	var log logger.Logger

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
	})

	Context("edit range calculation", func() {
		It("computes correct range for single-line edit with context", func() {
			content := "line 1\nline 2\nline 3\nline 4 to change\nline 5\nline 6\nline 7"
			result := file.ExtractEditFragmentWithRange(
				content, "line 4 to change", "line 4 changed", 2, log,
			)
			// Fragment: line 2, line 3, line 4 changed, line 5, line 6
			// Context before: 2 lines, edit: 1 line
			Expect(result.EditRange.EditStart).To(Equal(3))
			Expect(result.EditRange.EditEnd).To(Equal(3))
			Expect(result.Content).To(ContainSubstring("line 4 changed"))
		})

		It("computes correct range for edit at file start", func() {
			content := "line 1 to change\nline 2\nline 3\nline 4\nline 5"
			result := file.ExtractEditFragmentWithRange(
				content, "line 1 to change", "line 1 changed", 2, log,
			)
			// No context before, edit: 1 line
			Expect(result.EditRange.EditStart).To(Equal(1))
			Expect(result.EditRange.EditEnd).To(Equal(1))
		})

		It("computes correct range for multi-line replacement", func() {
			content := "line 1\nline 2\nold A\nold B\nold C\nline 6\nline 7"
			result := file.ExtractEditFragmentWithRange(
				content, "old A\nold B\nold C", "new A\nnew B", 2, log,
			)
			// Context before: line 1, line 2 (2 lines)
			// Edit: "new A\nnew B" = 2 lines
			Expect(result.EditRange.EditStart).To(Equal(3))
			Expect(result.EditRange.EditEnd).To(Equal(4))
		})

		It("handles line count expansion correctly", func() {
			content := "line 1\nline 2\nold line\n4\nline"
			result := file.ExtractEditFragmentWithRange(
				content, "old line", "new A\nnew B\nnew C\nnew D\nnew E", 2, log,
			)
			// newStr has 5 lines
			Expect(result.EditRange.EditStart).To(Equal(3))
			Expect(result.EditRange.EditEnd).To(Equal(7))
		})

		It("handles empty newStr (deletion within line)", func() {
			content := "line 1\nsome words to remove\nline 3"
			result := file.ExtractEditFragmentWithRange(
				content, "words to remove", "", 1, log,
			)
			// Empty string counts as 1 line (no newlines)
			Expect(result.EditRange.EditStart).To(Equal(2))
			Expect(result.EditRange.EditEnd).To(Equal(2))
		})
	})

	Context("cmp.Or fix - contextLines=0 on first line", func() {
		It("returns only the edit line with zero context", func() {
			content := "line 1\nline 2\nline 3\nline 4\nline 5"
			result := file.ExtractEditFragmentWithRange(
				content, "line 1", "changed 1", 0, log,
			)
			Expect(result.Content).To(Equal("changed 1"))
			Expect(result.EditRange.EditStart).To(Equal(1))
			Expect(result.EditRange.EditEnd).To(Equal(1))
		})

		It("does not include entire file for line-0 edit with contextLines=0", func() {
			// This was the cmp.Or bug: cmp.Or(min(0+0, 4), 4) = 4 (entire file)
			lines := make([]string, 100)
			for i := range lines {
				lines[i] = "line"
			}

			lines[0] = "target"
			content := strings.Join(lines, "\n")

			result := file.ExtractEditFragmentWithRange(
				content, "target", "changed", 0, log,
			)
			// Should be just 1 line, not 100
			lineCount := strings.Count(result.Content, "\n") + 1
			Expect(lineCount).To(Equal(1))
		})
	})

	Context("offset-based replacement", func() {
		It("replaces at exact position when oldStr appears in context", func() {
			// "fix" appears in context line AND in edit line
			content := "The word fix appears here.\n\nSome text with fix in it.\n\nMore content."
			result := file.ExtractEditFragmentWithRange(
				content, "fix in it", "repair in it", 2, log,
			)
			// Context should show "fix" unchanged, only "fix in it" replaced
			Expect(result.Content).To(ContainSubstring("fix appears here"))
			Expect(result.Content).To(ContainSubstring("repair in it"))
			Expect(result.Content).NotTo(ContainSubstring("fix in it"))
		})
	})

	Context("returns empty result when not found", func() {
		It("returns zero FragmentResult", func() {
			result := file.ExtractEditFragmentWithRange(
				"content", "not found", "replacement", 2, log,
			)
			Expect(result.Content).To(BeEmpty())
			Expect(result.EditRange.EditStart).To(Equal(0))
			Expect(result.EditRange.EditEnd).To(Equal(0))
		})
	})
})

var _ = Describe("EditReachesEOF", func() {
	Context("when old_string is at EOF", func() {
		It("returns true when old_string is at exact end of file", func() {
			content := "start\nmiddle\nlast line"
			Expect(file.EditReachesEOF(content, "last line")).To(BeTrue())
		})

		It("returns true when old_string ends with trailing newline at EOF", func() {
			content := "start\nmiddle\nlast line\n"
			Expect(file.EditReachesEOF(content, "last line\n")).To(BeTrue())
		})

		It("returns true when only whitespace follows old_string", func() {
			content := "start\nmiddle\nlast line\n   \n\n"
			Expect(file.EditReachesEOF(content, "last line")).To(BeTrue())
		})

		It("returns true for single-line file", func() {
			content := "only line"
			Expect(file.EditReachesEOF(content, "only line")).To(BeTrue())
		})
	})

	Context("when old_string is mid-file", func() {
		It("returns false when content follows old_string", func() {
			content := "start\nmiddle line\nthird\nfourth"
			Expect(file.EditReachesEOF(content, "middle line")).To(BeFalse())
		})

		It("returns false when old_string is at beginning", func() {
			content := "first line\nsecond\nthird"
			Expect(file.EditReachesEOF(content, "first line")).To(BeFalse())
		})

		It("returns false when editing mid-file session log entry", func() {
			// This is the real-world false positive case from the log
			content := `## Session Log

### Session 1
- Did thing A

### Session 2
- Did thing B

---

## Notes
- Some note`

			// Editing the session log section (mid-file)
			oldStr := `### Session 2
- Did thing B`

			Expect(file.EditReachesEOF(content, oldStr)).To(BeFalse())
		})
	})

	Context("edge cases", func() {
		It("returns false when old_string not found", func() {
			content := "start\nsecond"
			Expect(file.EditReachesEOF(content, "not found")).To(BeFalse())
		})

		It("returns true for empty content", func() {
			content := ""
			// Empty string matches at position 0, tail is empty
			Expect(file.EditReachesEOF(content, "")).To(BeTrue())
		})

		It("handles partial line matches correctly", func() {
			content := "line with word\nmore content"
			// "word" is mid-file, not at EOF
			Expect(file.EditReachesEOF(content, "word")).To(BeFalse())
		})

		It("handles edit that spans multiple lines at EOF", func() {
			content := "start\nsecond\nlast\nline"
			oldStr := "last\nline"
			Expect(file.EditReachesEOF(content, oldStr)).To(BeTrue())
		})
	})
})
