package reporters

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
)

// InteractiveReporter shows live spinners during check execution, then renders
// a colored table when all checks complete. Implements doctor.StreamingReporter.
type InteractiveReporter struct {
	theme color.Theme
}

// NewInteractiveReporter creates an InteractiveReporter with the given theme.
func NewInteractiveReporter(theme color.Theme) *InteractiveReporter {
	return &InteractiveReporter{theme: theme}
}

// Report renders results as a static colored table. Called for re-runs after
// fixes (the runner calls Report directly, not RunAndReport, for post-fix output).
func (r *InteractiveReporter) Report(results []doctor.CheckResult, verbose bool) {
	fmt.Println("Checking klaudiush health...")
	fmt.Println()

	tbl := RenderTable(results, verbose, r.theme)
	if tbl != "" {
		fmt.Println(tbl)
		fmt.Println()
	}

	fmt.Println(RenderSummary(results, r.theme))
}

// RunAndReport launches a BubbleTea program that runs checks with live spinners,
// then displays a table of results. Returns the collected check results.
func (r *InteractiveReporter) RunAndReport(
	ctx context.Context,
	registry *doctor.Registry,
	verbose bool,
	categories []doctor.Category,
) []doctor.CheckResult {
	checkers := registry.CheckersForCategories(categories)
	if len(checkers) == 0 {
		fmt.Println("No checks to run.")

		return nil
	}

	model := newDoctorModel(checkers, verbose, r.theme)
	p := tea.NewProgram(model, tea.WithOutput(os.Stderr))

	finalModel, err := p.Run()
	if err != nil {
		// Fallback: run checks without interactive UI, respecting category filter
		fmt.Fprintf(os.Stderr, "interactive UI failed: %v, falling back to static output\n", err)

		results := runCheckersBatch(ctx, checkers)
		r.Report(results, verbose)

		return results
	}

	m, ok := finalModel.(doctorModel)
	if !ok {
		return nil
	}

	results := m.results()
	r.Report(results, verbose)

	return results
}

// runCheckersBatch runs the given checkers sequentially as a fallback.
func runCheckersBatch(ctx context.Context, checkers []doctor.HealthChecker) []doctor.CheckResult {
	results := make([]doctor.CheckResult, len(checkers))

	for i, c := range checkers {
		result := c.Check(ctx)
		result.Category = c.Category()
		results[i] = result
	}

	return results
}
