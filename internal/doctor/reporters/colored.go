package reporters

import (
	"fmt"

	"github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
)

// ColoredReporter outputs a static colored table to stdout.
// Used for non-TTY output when colors are still enabled (e.g. piped to a pager
// that supports ANSI).
type ColoredReporter struct {
	theme color.Theme
}

// NewColoredReporter creates a ColoredReporter with the given theme.
func NewColoredReporter(theme color.Theme) *ColoredReporter {
	return &ColoredReporter{theme: theme}
}

// Report renders results as a colored table without interactive spinners.
func (r *ColoredReporter) Report(results []doctor.CheckResult, verbose bool) {
	fmt.Println("Checking klaudiush health...")
	fmt.Println()

	tbl := RenderTable(results, verbose, r.theme)
	if tbl != "" {
		fmt.Println(tbl)
		fmt.Println()
	}

	fmt.Println(RenderSummary(results, r.theme))
}
