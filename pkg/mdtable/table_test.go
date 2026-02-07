package mdtable_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/mdtable"
)

var errMarkdownlintFailed = errors.New("markdownlint validation failed")

func TestMdtable(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mdtable Suite")
}

var _ = Describe("Table", func() {
	Describe("New", func() {
		It("creates a table with headers", func() {
			table := mdtable.New("Name", "Age", "City")
			result := table.String()

			Expect(result).To(ContainSubstring("| Name"))
			Expect(result).To(ContainSubstring("| Age"))
			Expect(result).To(ContainSubstring("| City"))
		})

		It("returns empty string for no headers", func() {
			table := mdtable.New()
			Expect(table.String()).To(BeEmpty())
		})
	})

	Describe("AddRow", func() {
		It("adds rows to the table", func() {
			table := mdtable.New("Name", "Age").
				AddRow("Alice", "30").
				AddRow("Bob", "25")

			result := table.String()

			Expect(result).To(ContainSubstring("Alice"))
			Expect(result).To(ContainSubstring("Bob"))
		})

		It("pads missing cells", func() {
			table := mdtable.New("A", "B", "C").AddRow("1")
			result := table.String()

			lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
			Expect(lines).To(HaveLen(3)) // header, separator, 1 row
		})

		It("truncates extra cells", func() {
			table := mdtable.New("A", "B").AddRow("1", "2", "3", "4")
			result := table.String()

			Expect(result).NotTo(ContainSubstring("3"))
			Expect(result).NotTo(ContainSubstring("4"))
		})
	})

	Describe("SetAlignment", func() {
		It("sets left alignment", func() {
			table := mdtable.New("Name").
				SetAlignment(0, mdtable.AlignLeft).
				AddRow("Test")

			result := table.String()

			Expect(result).To(ContainSubstring("|:"))
		})

		It("sets center alignment", func() {
			table := mdtable.New("Name").
				SetAlignment(0, mdtable.AlignCenter).
				AddRow("Test")

			result := table.String()
			lines := strings.Split(result, "\n")

			Expect(lines[1]).To(MatchRegexp(`\|:-+:\|`))
		})

		It("sets right alignment", func() {
			table := mdtable.New("Name").
				SetAlignment(0, mdtable.AlignRight).
				AddRow("Test")

			result := table.String()
			lines := strings.Split(result, "\n")

			Expect(lines[1]).To(MatchRegexp(`\|-+:\|`))
		})
	})

	Describe("SetAlignments", func() {
		It("sets multiple alignments", func() {
			table := mdtable.New("A", "B", "C").
				SetAlignments(mdtable.AlignLeft, mdtable.AlignCenter, mdtable.AlignRight).
				AddRow("1", "2", "3")

			result := table.String()
			lines := strings.Split(result, "\n")

			Expect(lines[1]).To(ContainSubstring(":"))
		})
	})

	Describe("String", func() {
		It("formats a complete table", func() {
			table := mdtable.New("Name", "Age", "City").
				AddRow("John", "30", "NYC").
				AddRow("Jane", "25", "SF")

			result := table.String()
			lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")

			Expect(lines).To(HaveLen(4))

			// Verify structure
			for _, line := range lines {
				Expect(line).To(HavePrefix("|"))
				Expect(line).To(HaveSuffix("|"))
			}
		})

		It("handles empty table", func() {
			table := mdtable.New("A", "B")
			result := table.String()

			lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
			Expect(lines).To(HaveLen(2)) // header + separator
		})
	})

	Describe("sanitization", func() {
		It("escapes pipe characters", func() {
			table := mdtable.New("Name", "Data").
				AddRow("Test", "A|B|C")

			result := table.String()

			Expect(result).To(ContainSubstring(`A\|B\|C`))
		})

		It("preserves already escaped pipes", func() {
			table := mdtable.New("Name", "Data").
				AddRow("Test", `A\|B`)

			result := table.String()

			// Should not double-escape
			Expect(result).NotTo(ContainSubstring(`\\|`))
		})

		It("trims whitespace", func() {
			table := mdtable.New("  Name  ", "  Age  ").
				AddRow("  Alice  ", "  30  ")

			result := table.String()

			Expect(result).NotTo(ContainSubstring("  Name"))
			Expect(result).NotTo(ContainSubstring("Alice  "))
		})

		It("replaces newlines with spaces", func() {
			table := mdtable.New("Name", "Note").
				AddRow("Test", "Line1\nLine2")

			result := table.String()

			Expect(result).To(ContainSubstring("Line1 Line2"))
		})

		It("collapses multiple spaces", func() {
			table := mdtable.New("Name", "Data").
				AddRow("Test", "A    B    C")

			result := table.String()

			Expect(result).To(ContainSubstring("A B C"))
		})
	})

	Describe("width calculation", func() {
		It("pads cells to match widest content", func() {
			table := mdtable.New("N", "Name").
				AddRow("X", "Alice")

			result := table.String()
			lines := strings.Split(result, "\n")

			// Header and data should have same column widths
			headerParts := strings.Split(lines[0], "|")
			dataParts := strings.Split(lines[2], "|")

			Expect(len(headerParts[1])).To(Equal(len(dataParts[1])))
		})

		It("ensures minimum separator width of 3", func() {
			table := mdtable.New("A").AddRow("X")
			result := table.String()
			lines := strings.Split(result, "\n")

			// Separator should have at least 3 dashes
			Expect(lines[1]).To(MatchRegexp(`-{3,}`))
		})
	})

	Describe("Format", func() {
		It("creates a formatted table from headers and rows", func() {
			result := mdtable.Format(
				[]string{"Name", "Age"},
				[][]string{{"Alice", "30"}, {"Bob", "25"}},
			)

			Expect(result).To(ContainSubstring("Alice"))
			Expect(result).To(ContainSubstring("Bob"))
		})

		It("applies alignments", func() {
			result := mdtable.Format(
				[]string{"Name", "Amount"},
				[][]string{{"Item", "100"}},
				mdtable.AlignLeft, mdtable.AlignRight,
			)

			lines := strings.Split(result, "\n")

			Expect(lines[1]).To(ContainSubstring("-:"))
		})
	})

	Describe("FormatSimple", func() {
		It("creates a left-aligned table", func() {
			result := mdtable.FormatSimple(
				[]string{"A", "B"},
				[][]string{{"1", "2"}},
			)

			Expect(result).To(ContainSubstring("|"))
		})
	})
})

var _ = Describe("Unicode handling", func() {
	It("handles emoji", func() {
		table := mdtable.New("Status", "Name").
			AddRow("✅", "Done").
			AddRow("❌", "Failed")

		result := table.String()

		Expect(result).To(ContainSubstring("✅"))
		Expect(result).To(ContainSubstring("❌"))
	})

	It("handles CJK characters", func() {
		table := mdtable.New("名前", "都市").
			AddRow("太郎", "東京")

		result := table.String()

		Expect(result).To(ContainSubstring("名前"))
		Expect(result).To(ContainSubstring("太郎"))
	})

	It("handles mixed scripts", func() {
		table := mdtable.New("Name", "Hello").
			AddRow("English", "Hello").
			AddRow("Japanese", "こんにちは").
			AddRow("Arabic", "مرحبا")

		result := table.String()

		Expect(result).To(ContainSubstring("Hello"))
		Expect(result).To(ContainSubstring("こんにちは"))
	})
})

var _ = Describe("Markdownlint validation", func() {
	var tmpDir string

	BeforeEach(func() {
		// Check if markdownlint is available
		_, err := exec.LookPath("markdownlint")
		if err != nil {
			Skip("markdownlint not available, skipping validation tests")
		}

		tmpDir = GinkgoT().TempDir()

		// Create markdownlint config that disables rules inappropriate for tables
		// MD060 is disabled because it doesn't understand display width of
		// CJK characters and emoji (they have width 2 but count as 1 character)
		configContent := `{
  "MD013": false,
  "MD033": false,
  "MD059": false,
  "MD060": false
}`
		configPath := filepath.Join(tmpDir, ".markdownlint.json")
		err = os.WriteFile(configPath, []byte(configContent), 0o644)
		Expect(err).NotTo(HaveOccurred())
	})

	validateWithMarkdownlint := func(tableContent string) error {
		// Wrap table in proper markdown document
		content := "# Test Document\n\n" + tableContent

		// Write to temp file
		filename := filepath.Join(tmpDir, "test.md")
		err := os.WriteFile(filename, []byte(content), 0o644)
		if err != nil {
			return err
		}

		// Run markdownlint with config
		configPath := filepath.Join(tmpDir, ".markdownlint.json")
		cmd := exec.Command("markdownlint", "--config", configPath, filename)
		output, err := cmd.CombinedOutput()
		if err != nil {
			GinkgoWriter.Printf("markdownlint output: %s\n", string(output))

			return errMarkdownlintFailed
		}

		return nil
	}

	It("produces tables that pass markdownlint", func() {
		table := mdtable.New("Name", "Age", "City").
			AddRow("John", "30", "NYC").
			AddRow("Jane", "25", "SF")

		err := validateWithMarkdownlint(table.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("handles emoji correctly for markdownlint", func() {
		table := mdtable.New("Status", "Name", "Progress").
			AddRow("✅", "Task 1", "Complete").
			AddRow("❌", "Task 2", "Failed").
			AddRow("⚠️", "Task 3", "Warning")

		err := validateWithMarkdownlint(table.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("handles CJK characters for markdownlint", func() {
		table := mdtable.New("名前", "年齢", "都市").
			AddRow("太郎", "30", "東京").
			AddRow("花子", "25", "大阪")

		err := validateWithMarkdownlint(table.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("handles mixed content for markdownlint", func() {
		table := mdtable.New("Feature", "Status", "Notes").
			AddRow("Auth", "✅ Done", "Uses OAuth").
			AddRow("日本語", "⚠️ WIP", "Needs review").
			AddRow("API", "❌ Failed", "See PR #123")

		err := validateWithMarkdownlint(table.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("handles code in cells for markdownlint", func() {
		table := mdtable.New("Function", "Usage").
			AddRow("`map`", "`map(arr, fn)`").
			AddRow("`filter`", "`filter(arr, fn)`")

		err := validateWithMarkdownlint(table.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("handles escaped pipes for markdownlint", func() {
		table := mdtable.New("Name", "Formula").
			AddRow("Test", "a|b|c")

		err := validateWithMarkdownlint(table.String())
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Width mode", func() {
	Describe("WidthModeDisplay (default)", func() {
		It("uses display width for CJK characters", func() {
			table := mdtable.New("Header", "Value").
				AddRow("ASCII", "Test").
				AddRow("CJK", "日本語")

			result := table.String()
			lines := strings.Split(result, "\n")

			// CJK characters have display width 2, so "日本語" is 6 display units
			// "Header" is 6 characters, so both columns should be aligned
			Expect(lines[0]).To(ContainSubstring("| Header |"))
		})

		It("uses display width for emoji", func() {
			table := mdtable.New("Status", "Name").
				AddRow("✅", "Done").
				AddRow("❌", "Failed")

			result := table.String()

			// Emoji should be properly aligned
			Expect(result).To(ContainSubstring("| Status |"))
		})

		It("matches CJK golden file", func() {
			table := mdtable.New("Header", "Value").
				AddRow("ASCII", "Test").
				AddRow("CJK", "日本語")

			result := table.String()
			golden, err := os.ReadFile("testdata/cjk_display_width.golden")
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(Equal(string(golden)))
		})

		It("matches emoji golden file", func() {
			table := mdtable.New("Status", "Name").
				AddRow("✅", "Done").
				AddRow("❌", "Failed")

			result := table.String()
			golden, err := os.ReadFile("testdata/emoji_display_width.golden")
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(Equal(string(golden)))
		})
	})

	Describe("WidthModeByte", func() {
		It("uses byte width for CJK characters", func() {
			table := mdtable.New("Header", "Value").
				SetWidthMode(mdtable.WidthModeByte).
				AddRow("ASCII", "Test").
				AddRow("CJK", "日本語")

			result := table.String()
			lines := strings.Split(result, "\n")

			// In byte mode, "日本語" is 9 bytes (3 bytes per character)
			// The column width should be based on byte count
			// Line 3 (index 3) is the CJK row
			Expect(lines[3]).To(ContainSubstring("日本語"))
		})

		It("produces different output than display mode for CJK", func() {
			displayTable := mdtable.New("A", "Value").
				SetWidthMode(mdtable.WidthModeDisplay).
				AddRow("X", "日本語")

			byteTable := mdtable.New("A", "Value").
				SetWidthMode(mdtable.WidthModeByte).
				AddRow("X", "日本語")

			displayResult := displayTable.String()
			byteResult := byteTable.String()

			// The outputs should be different because of different width calculations
			Expect(displayResult).NotTo(Equal(byteResult))
		})

		It("uses FormatWithMode correctly", func() {
			displayResult := mdtable.FormatWithMode(
				[]string{"Col", "Data"},
				[][]string{{"A", "日本語"}},
				mdtable.WidthModeDisplay,
			)

			byteResult := mdtable.FormatWithMode(
				[]string{"Col", "Data"},
				[][]string{{"A", "日本語"}},
				mdtable.WidthModeByte,
			)

			Expect(displayResult).NotTo(Equal(byteResult))
		})

		It("matches CJK golden file", func() {
			table := mdtable.New("Header", "Value").
				SetWidthMode(mdtable.WidthModeByte).
				AddRow("ASCII", "Test").
				AddRow("CJK", "日本語")

			result := table.String()
			golden, err := os.ReadFile("testdata/cjk_byte_width.golden")
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(Equal(string(golden)))
		})

		It("matches emoji golden file", func() {
			table := mdtable.New("Status", "Name").
				SetWidthMode(mdtable.WidthModeByte).
				AddRow("✅", "Done").
				AddRow("❌", "Failed")

			result := table.String()
			golden, err := os.ReadFile("testdata/emoji_byte_width.golden")
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(Equal(string(golden)))
		})
	})

	Describe("SetWidthMode", func() {
		It("is chainable", func() {
			table := mdtable.New("A", "B").
				SetWidthMode(mdtable.WidthModeByte).
				SetAlignment(0, mdtable.AlignCenter).
				AddRow("1", "2")

			result := table.String()
			Expect(result).To(ContainSubstring("|"))
		})
	})
})

var _ = Describe("Edge cases", func() {
	It("handles empty rows", func() {
		table := mdtable.New("A", "B").
			AddRow("", "").
			AddRow("X", "Y")

		result := table.String()
		lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")

		Expect(lines).To(HaveLen(4))
	})

	It("handles single column table", func() {
		table := mdtable.New("Value").
			AddRow("A").
			AddRow("B")

		result := table.String()

		Expect(result).To(ContainSubstring("| Value |"))
	})

	It("handles many columns", func() {
		table := mdtable.New("A", "B", "C", "D", "E", "F", "G", "H").
			AddRow("1", "2", "3", "4", "5", "6", "7", "8")

		result := table.String()
		lines := strings.Split(result, "\n")

		// Count pipes in first line
		pipeCount := strings.Count(lines[0], "|")
		Expect(pipeCount).To(Equal(9)) // 8 columns = 9 pipes
	})

	It("handles code in cells", func() {
		table := mdtable.New("Function", "Example").
			AddRow("`map`", "`map(nums, fn)`")

		result := table.String()

		Expect(result).To(ContainSubstring("`map`"))
	})

	It("handles URLs in cells", func() {
		table := mdtable.New("Service", "URL").
			AddRow("API", "https://api.klaudiu.sh")

		result := table.String()

		Expect(result).To(ContainSubstring("https://api.klaudiu.sh"))
	})
})
