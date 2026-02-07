package mdtable_test

import (
	"testing"

	"github.com/smykla-labs/klaudiush/pkg/mdtable"
)

func FuzzMdtableParse(f *testing.F) {
	// Seed corpus from existing tests
	f.Add("| Name | Age |\n| ---- | --- |\n| John | 30  |")
	f.Add("| Left | Center | Right |\n|:-----|:------:|------:|\n| A | B | C |")
	f.Add("| Name | Data |\n|------|------|\n| Test | A\\|B |")
	f.Add("| Status | Name |\n|--------|------|\n| ✅ | Done |")
	f.Add("")
	f.Add("| A | B | C |\n|---|---|---|\n| 1 | 2 |") // column mismatch
	f.Add("not a table")
	f.Add("| single cell |")
	f.Add("| | |\n|-|-|\n| | |") // empty cells
	f.Add("| 日本語 | 中文 |\n|--------|------|\n| テスト | 测试 |")
	f.Add("| Col1 |\n|------|\n| Row1 |\n| Row2 |\n| Row3 |")
	f.Add("Some text\n\n| Name | Age |\n| ---- | --- |\n| John | 30  |\n\nMore text")

	f.Fuzz(func(t *testing.T, content string) {
		result := mdtable.Parse(content)
		if result == nil {
			t.Error("nil result")
			return
		}

		// Access all fields - should not panic
		_ = result.Tables
		_ = result.Issues

		for _, table := range result.Tables {
			_ = table.StartLine
			_ = table.EndLine
			_ = table.Headers
			_ = table.Rows
			_ = table.Alignments
			_ = table.RawLines

			// Exercise formatting functions
			_ = mdtable.FormatTable(&table)
		}
	})
}
