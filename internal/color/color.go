// Package color provides color detection and theming for CLI output.
package color

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Profile detects the current color profile based on environment variables and flags.
// Returns true if color output should be enabled.
//
// Color is disabled when any of:
//   - NO_COLOR env is set (any value, per https://no-color.org)
//   - CLICOLOR=0
//   - TERM=dumb
//   - noColorFlag is true (--no-color CLI flag)
func Profile(noColorFlag bool) bool {
	if noColorFlag {
		return false
	}

	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}

	if os.Getenv("CLICOLOR") == "0" {
		return false
	}

	if os.Getenv("TERM") == "dumb" {
		return false
	}

	return true
}

// IsTerminal returns true if the given file descriptor is a terminal.
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

// Theme holds lipgloss styles for doctor output.
type Theme struct {
	Pass      lipgloss.Style
	Fail      lipgloss.Style
	Warning   lipgloss.Style
	Skip      lipgloss.Style
	Info      lipgloss.Style
	Header    lipgloss.Style
	CheckName lipgloss.Style
	Muted     lipgloss.Style
}

// NewTheme creates a Theme. When color is false, all styles are empty (no ANSI codes).
func NewTheme(color bool) Theme {
	if !color {
		return Theme{}
	}

	return Theme{
		Pass:      lipgloss.NewStyle().Foreground(lipgloss.Color("10")), // bright green
		Fail:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		Warning:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")), // bright yellow
		Skip:      lipgloss.NewStyle().Foreground(lipgloss.Color("8")),  // gray
		Info:      lipgloss.NewStyle().Foreground(lipgloss.Color("12")), // bright blue
		Header:    lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true),
		CheckName: lipgloss.NewStyle().Bold(true),
		Muted:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	}
}
