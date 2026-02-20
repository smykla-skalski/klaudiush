package reporters

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
)

// phase tracks which phase the interactive model is in.
type phase int

const (
	phaseRunning phase = iota
	phaseTable
)

// checkEntry tracks state for a single health checker.
type checkEntry struct {
	checker doctor.HealthChecker
	result  doctor.CheckResult
	done    bool
}

// checkDoneMsg is sent when a single check finishes.
type checkDoneMsg struct {
	index  int
	result doctor.CheckResult
}

// doctorModel is the BubbleTea model for interactive doctor output.
type doctorModel struct {
	entries []checkEntry
	spinner spinner.Model
	phase   phase
	verbose bool
	theme   color.Theme
}

func newDoctorModel(checkers []doctor.HealthChecker, verbose bool, theme color.Theme) doctorModel {
	entries := make([]checkEntry, len(checkers))
	for i, c := range checkers {
		entries[i] = checkEntry{checker: c}
	}

	s := spinner.New(spinner.WithSpinner(spinner.Dot))

	if _, ok := theme.Info.GetForeground().(lipgloss.NoColor); !ok {
		s.Style = theme.Info
	}

	return doctorModel{
		entries: entries,
		spinner: s,
		phase:   phaseRunning,
		verbose: verbose,
		theme:   theme,
	}
}

// runCheck returns a tea.Cmd that executes a single health check.
func runCheck(ctx context.Context, index int, checker doctor.HealthChecker) tea.Cmd {
	return func() tea.Msg {
		result := checker.Check(ctx)
		result.Category = checker.Category()

		return checkDoneMsg{index: index, result: result}
	}
}

func (m doctorModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.entries)+1)
	cmds = append(cmds, m.spinner.Tick)

	ctx := context.Background()

	for i := range m.entries {
		cmds = append(cmds, runCheck(ctx, i, m.entries[i].checker))
	}

	return tea.Batch(cmds...)
}

//nolint:ireturn // tea.Model is required by the bubbletea framework
func (m doctorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case spinner.TickMsg:
		if m.phase == phaseRunning {
			var cmd tea.Cmd

			m.spinner, cmd = m.spinner.Update(msg)

			return m, cmd
		}

	case checkDoneMsg:
		m.entries[msg.index].result = msg.result
		m.entries[msg.index].done = true

		if m.allDone() {
			m.phase = phaseTable

			return m, tea.Quit
		}
	}

	return m, nil
}

func (m doctorModel) View() string {
	if m.phase == phaseRunning {
		return m.viewRunning()
	}

	// phaseTable: return empty so BubbleTea clears the spinner view.
	// The table is printed to stdout after the program exits, allowing
	// it to scroll naturally instead of being clipped to terminal height.
	return ""
}

func (m doctorModel) viewRunning() string {
	var b strings.Builder

	b.WriteString("Checking klaudiush health...\n\n")

	groups := m.groupEntries()

	for _, g := range groups {
		b.WriteString(m.theme.Header.Render(getCategoryName(g.category)))
		b.WriteString(":\n")

		for _, e := range g.entries {
			if e.done {
				icon := StyledIcon(e.result, m.theme)
				name := m.theme.CheckName.Render(e.checker.Name())
				fmt.Fprintf(&b, "  %s %s", icon, name)

				if e.result.Message != "" {
					fmt.Fprintf(&b, " - %s", shortenPath(e.result.Message))
				}
			} else {
				fmt.Fprintf(&b, "  %s %s", m.spinner.View(), e.checker.Name())
			}

			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	return b.String()
}

func (m doctorModel) allDone() bool {
	for _, e := range m.entries {
		if !e.done {
			return false
		}
	}

	return true
}

func (m doctorModel) results() []doctor.CheckResult {
	results := make([]doctor.CheckResult, len(m.entries))
	for i, e := range m.entries {
		results[i] = e.result
	}

	return results
}

type entryGroup struct {
	category doctor.Category
	entries  []checkEntry
}

func (m doctorModel) groupEntries() []entryGroup {
	catMap := make(map[doctor.Category][]checkEntry)

	for _, e := range m.entries {
		cat := e.checker.Category()
		catMap[cat] = append(catMap[cat], e)
	}

	var groups []entryGroup

	for _, cat := range categoryOrder {
		if es, ok := catMap[cat]; ok {
			groups = append(groups, entryGroup{category: cat, entries: es})
			delete(catMap, cat)
		}
	}

	for cat, es := range catMap {
		groups = append(groups, entryGroup{category: cat, entries: es})
	}

	return groups
}
