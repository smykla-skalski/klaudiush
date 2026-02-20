package reporters

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
)

// Export unexported functions for external tests.
var (
	PadToWidth          = padToWidth
	ToCellWidths        = toCellWidths
	CalcColumnWidthsFor = calcColumnWidthsFor
	BuildResultRow      = buildResultRow
	SeverityRank        = severityRank
	ShortenPath         = shortenPath
	DimBorders          = dimBorders
)

//nolint:ireturn // test helper returning tea.Model interface
func NewModelForTest(
	checkers []doctor.HealthChecker,
	verbose bool,
	theme color.Theme,
) tea.Model {
	return newDoctorModel(checkers, verbose, theme)
}

// ModelResultsForTest extracts results from a doctorModel via tea.Model.
func ModelResultsForTest(m tea.Model) []doctor.CheckResult {
	dm, ok := m.(doctorModel)
	if !ok {
		return nil
	}

	return dm.results()
}

// SetHomeDir overrides the homeDir package variable for testing shortenPath.
func SetHomeDir(dir string) {
	homeDir = dir
}
