package reporters

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"golang.org/x/term"

	"github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
)

// StatusIcon returns a single-width character icon for a check result.
// These are used inside tables where emoji would break column alignment.
func StatusIcon(result doctor.CheckResult) string {
	switch result.Status {
	case doctor.StatusPass:
		return "✓"
	case doctor.StatusFail:
		switch result.Severity {
		case doctor.SeverityError:
			return "✗"
		case doctor.SeverityWarning:
			return "!"
		default:
			return "i"
		}
	case doctor.StatusSkipped:
		return "-"
	default:
		return "?"
	}
}

// StyledIcon returns a StatusIcon colored by the theme.
func StyledIcon(result doctor.CheckResult, theme color.Theme) string {
	icon := StatusIcon(result)

	switch result.Status {
	case doctor.StatusPass:
		return theme.Pass.Render(icon)
	case doctor.StatusFail:
		if result.Severity == doctor.SeverityError {
			return theme.Fail.Render(icon)
		}

		return theme.Warning.Render(icon)
	case doctor.StatusSkipped:
		return theme.Skip.Render(icon)
	default:
		return icon
	}
}

// RenderTable builds a table from check results using tablewriter.
// Category headers span columns 2+ via horizontal merge, keeping the icon
// column narrow. Long text wraps within cells.
func RenderTable(results []doctor.CheckResult, verbose bool, theme color.Theme) string {
	if len(results) == 0 {
		return ""
	}

	grouped := GroupResultsByCategory(results)
	if len(grouped) == 0 {
		return ""
	}

	headers := []string{"", "Check", "Message"}
	if verbose {
		headers = append(headers, "Details")
	}

	colWidths := calcColumnWidths(results, verbose)

	var buf bytes.Buffer

	opts := []tablewriter.Option{
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Symbols: tw.NewSymbols(tw.StyleRounded),
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenRows: tw.On,
				},
			},
		})),
		tablewriter.WithPadding(tw.Padding{Left: " ", Right: " "}),
		tablewriter.WithConfig(tablewriter.NewConfigBuilder().
			WithTrimSpace(tw.Off).
			Row().Merging().WithMode(tw.MergeHorizontal).Build().
			Formatting().WithAutoWrap(tw.WrapNormal).Build().
			Build().Build()),
	}

	if colWidths != nil {
		opts = append(opts, tablewriter.WithColumnWidths(toCellWidths(colWidths)))
	}

	t := tablewriter.NewTable(&buf, opts...)

	t.Header(headers)

	for _, g := range grouped {
		appendCategoryRows(t, g, headers, verbose, colWidths, theme)
	}

	_ = t.Render()

	output := strings.TrimRight(buf.String(), "\n")

	return dimBorders(output, theme)
}

// appendCategoryRows adds a category header row and all result rows for a
// group to the table. Rows are sorted by severity and padded to colWidths.
func appendCategoryRows(
	t *tablewriter.Table,
	g categoryGroup,
	headers []string,
	verbose bool,
	colWidths map[int]int,
	theme color.Theme,
) {
	catName := theme.Header.Render(getCategoryName(g.Category))

	catRow := []string{""}
	for i := 1; i < len(headers); i++ {
		catRow = append(catRow, catName)
	}

	_ = t.Append(catRow)

	sorted := slices.Clone(g.Results)
	slices.SortFunc(sorted, func(a, b doctor.CheckResult) int {
		return severityRank(a) - severityRank(b)
	})

	for _, r := range sorted {
		row := buildResultRow(r, verbose, colWidths, theme)
		_ = t.Append(row)
	}
}

// buildResultRow creates a table row for a single check result, padding cells
// to the target column widths when set.
func buildResultRow(
	r doctor.CheckResult,
	verbose bool,
	colWidths map[int]int,
	theme color.Theme,
) []string {
	icon := StyledIcon(r, theme)
	name := theme.CheckName.Render(r.Name)
	msg := shortenPath(r.Message)
	row := []string{icon, name, msg}

	if verbose {
		row = append(row, shortenPath(strings.Join(r.Details, "; ")))
	}

	if colWidths != nil {
		for i, cell := range row {
			if w, ok := colWidths[i]; ok {
				row[i] = padToWidth(cell, w)
			}
		}
	}

	return row
}

// toCellWidths converts content widths to cell widths (content + left/right
// padding) for WithColumnWidths. Tablewriter subtracts padding from these
// values to get the effective content wrapping width.
func toCellWidths(contentWidths map[int]int) tw.Mapper[int, int] {
	const padW = 2 // " " left + " " right

	m := make(tw.Mapper[int, int], len(contentWidths))
	for col, w := range contentWidths {
		m[col] = w + padW
	}

	return m
}

// padToWidth right-pads s with spaces so its display width reaches w.
// ANSI escape codes are excluded from width calculation.
func padToWidth(s string, w int) string {
	visible := runewidth.StringWidth(ansi.Strip(s))
	if visible >= w {
		return s
	}

	return s + strings.Repeat(" ", w-visible)
}

// dimBorders applies the muted theme style to all box-drawing border
// characters in the rendered table output.
func dimBorders(s string, theme color.Theme) string {
	for _, ch := range []string{
		"╭", "╮", "╰", "╯", "│", "─", "┬", "┴", "├", "┤", "┼",
	} {
		s = strings.ReplaceAll(s, ch, theme.Muted.Render(ch))
	}

	return s
}

// RenderSummary returns a colored summary line.
func RenderSummary(results []doctor.CheckResult, theme color.Theme) string {
	errors, warnings, passed := countResults(results)
	skipped := 0

	for _, r := range results {
		if r.IsSkipped() {
			skipped++
		}
	}

	parts := []string{
		styleSummaryPart(fmt.Sprintf("%d error(s)", errors), errors > 0, theme.Fail),
		styleSummaryPart(fmt.Sprintf("%d warning(s)", warnings), warnings > 0, theme.Warning),
		theme.Pass.Render(fmt.Sprintf("%d passed", passed)),
	}

	if skipped > 0 {
		parts = append(parts, theme.Skip.Render(fmt.Sprintf("%d skipped", skipped)))
	}

	return "Summary: " + strings.Join(parts, ", ")
}

func styleSummaryPart(text string, active bool, style lipgloss.Style) string {
	if active {
		return style.Render(text)
	}

	return text
}

// GroupResultsByCategory groups results by category, preserving category order.
func GroupResultsByCategory(results []doctor.CheckResult) []categoryGroup {
	catMap := make(map[doctor.Category][]doctor.CheckResult)

	for _, r := range results {
		catMap[r.Category] = append(catMap[r.Category], r)
	}

	var groups []categoryGroup

	for _, cat := range categoryOrder {
		if rs, ok := catMap[cat]; ok {
			groups = append(groups, categoryGroup{Category: cat, Results: rs})
			delete(catMap, cat)
		}
	}

	for cat, rs := range catMap {
		groups = append(groups, categoryGroup{Category: cat, Results: rs})
	}

	return groups
}

type categoryGroup struct {
	Category doctor.Category
	Results  []doctor.CheckResult
}

// calcColumnWidths computes per-column content widths that fill the terminal.
// Returns nil when not a terminal or terminal is too narrow for a table.
// Widths are content-only (padding and borders accounted for separately).
func calcColumnWidths(
	results []doctor.CheckResult,
	verbose bool,
) map[int]int {
	return calcColumnWidthsFor(termWidth(), results, verbose)
}

// calcColumnWidthsFor computes per-column content widths for a given terminal
// width. Extracted from calcColumnWidths to allow testing with injected widths.
func calcColumnWidthsFor(
	w int,
	results []doctor.CheckResult,
	verbose bool,
) map[int]int {
	const minTableW = 40

	if w < minTableW {
		return nil
	}

	// Find widest check name (visible chars only).
	checkW := 5 // min = len("Check")

	for _, r := range results {
		if n := len(r.Name); n > checkW {
			checkW = n
		}
	}

	const iconW = 1

	numCols := 3
	if verbose {
		numCols = 4
	}

	// Each column has: 1 border char + 1 left pad + 1 right pad = 3.
	// Plus 1 trailing border on the right.
	const colOverhead = 3

	overhead := numCols*colOverhead + 1
	available := w - overhead - iconW

	// Ensure the message column always gets at least minMsgW chars.
	// Cap check name width if needed so it doesn't dominate narrow terminals.
	const minMsgW = 20

	const minCheckW = 5

	if available < minMsgW+minCheckW {
		return nil
	}

	if checkW > available-minMsgW {
		checkW = available - minMsgW
	}

	remaining := available - checkW

	widths := map[int]int{
		0: iconW,
		1: checkW,
		2: remaining,
	}

	if verbose {
		// 60/40 split between message and details.
		msgW := remaining * 60 / 100 //nolint:mnd // layout ratio
		widths[2] = msgW
		widths[3] = remaining - msgW
	}

	return widths
}

// termWidth returns the terminal width or 0 if not a terminal.
func termWidth() int {
	if w, _, err := term.GetSize(
		int(os.Stdout.Fd()), //nolint:gosec // fd fits int
	); err == nil && w > 0 {
		return w
	}

	if w, _, err := term.GetSize(
		int(os.Stderr.Fd()), //nolint:gosec // fd fits int
	); err == nil && w > 0 {
		return w
	}

	return 0
}

// homeDir caches the user's home directory for path shortening.
var homeDir string

func init() {
	homeDir, _ = os.UserHomeDir()
}

// shortenPath replaces the user's home directory prefix with ~.
func shortenPath(s string) string {
	if homeDir == "" {
		return s
	}

	return strings.ReplaceAll(s, homeDir, "~")
}

// Severity rank constants for sorting results within a category.
const (
	rankError   = 0
	rankWarning = 1
	rankPass    = 2
	rankSkipped = 3
)

func severityRank(r doctor.CheckResult) int {
	if r.IsError() {
		return rankError
	}

	if r.IsWarning() {
		return rankWarning
	}

	if r.IsSkipped() {
		return rankSkipped
	}

	return rankPass
}
