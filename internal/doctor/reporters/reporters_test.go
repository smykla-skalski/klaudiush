package reporters_test

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/reporters"
)

func TestReporters(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reporters Suite")
}

func sampleResults() []doctor.CheckResult {
	return []doctor.CheckResult{
		{
			Name:     "Binary available",
			Category: doctor.CategoryBinary,
			Severity: doctor.SeverityInfo,
			Status:   doctor.StatusPass,
			Message:  "Found at /usr/local/bin/klaudiush",
			Details:  []string{},
		},
		{
			Name:     "Permissions",
			Category: doctor.CategoryBinary,
			Severity: doctor.SeverityError,
			Status:   doctor.StatusFail,
			Message:  "Expected 0755, got 0644",
			Details:  []string{"chmod 755 recommended"},
		},
		{
			Name:     "Hook registered",
			Category: doctor.CategoryHook,
			Severity: doctor.SeverityInfo,
			Status:   doctor.StatusPass,
			Message:  "Registered",
			Details:  []string{},
		},
		{
			Name:     "shellcheck",
			Category: doctor.CategoryTools,
			Severity: doctor.SeverityWarning,
			Status:   doctor.StatusFail,
			Message:  "Not found",
			Details:  []string{},
		},
		{
			Name:     "tflint",
			Category: doctor.CategoryTools,
			Severity: doctor.SeverityInfo,
			Status:   doctor.StatusSkipped,
			Message:  "No terraform files",
			Details:  []string{},
		},
	}
}

var _ = Describe("StatusIcon", func() {
	It("returns check for pass", func() {
		r := doctor.Pass("test", "ok")
		Expect(reporters.StatusIcon(r)).To(Equal("✓"))
	})

	It("returns x for error", func() {
		r := doctor.FailError("test", "bad")
		Expect(reporters.StatusIcon(r)).To(Equal("✗"))
	})

	It("returns bang for warning", func() {
		r := doctor.FailWarning("test", "meh")
		Expect(reporters.StatusIcon(r)).To(Equal("!"))
	})

	It("returns dash for skipped", func() {
		r := doctor.Skip("test", "skipped")
		Expect(reporters.StatusIcon(r)).To(Equal("-"))
	})
})

var _ = Describe("RenderTable", func() {
	It("renders a table with results", func() {
		theme := color.NewTheme(false)
		output := reporters.RenderTable(sampleResults(), false, theme)
		Expect(output).To(ContainSubstring("Binary available"))
		Expect(output).To(ContainSubstring("Permissions"))
		Expect(output).To(ContainSubstring("shellcheck"))
	})

	It("includes details column in verbose mode", func() {
		theme := color.NewTheme(false)
		output := reporters.RenderTable(sampleResults(), true, theme)
		Expect(output).To(ContainSubstring("DETAILS"))
		Expect(output).To(ContainSubstring("chmod 755 recommended"))
	})

	It("returns empty string for empty results", func() {
		theme := color.NewTheme(false)
		Expect(reporters.RenderTable(nil, false, theme)).To(BeEmpty())
	})

	It("renders with color theme without error", func() {
		// lipgloss strips ANSI codes in non-TTY test environments,
		// so we just verify it renders without error and contains content
		theme := color.NewTheme(true)
		output := reporters.RenderTable(sampleResults(), false, theme)
		Expect(output).To(ContainSubstring("Binary available"))
		Expect(output).To(ContainSubstring("Permissions"))
	})
})

var _ = Describe("RenderSummary", func() {
	It("counts errors, warnings, and passed", func() {
		theme := color.NewTheme(false)
		summary := reporters.RenderSummary(sampleResults(), theme)
		Expect(summary).To(ContainSubstring("1 error(s)"))
		Expect(summary).To(ContainSubstring("1 warning(s)"))
		Expect(summary).To(ContainSubstring("2 passed"))
		Expect(summary).To(ContainSubstring("1 skipped"))
	})
})

var _ = Describe("GroupResultsByCategory", func() {
	It("groups results by category in order", func() {
		groups := reporters.GroupResultsByCategory(sampleResults())
		Expect(groups).To(HaveLen(3))
		Expect(groups[0].Category).To(Equal(doctor.CategoryBinary))
		Expect(groups[1].Category).To(Equal(doctor.CategoryHook))
		Expect(groups[2].Category).To(Equal(doctor.CategoryTools))
	})

	It("returns empty for empty input", func() {
		groups := reporters.GroupResultsByCategory(nil)
		Expect(groups).To(BeEmpty())
	})
})

var _ = Describe("StyledIcon", func() {
	It("returns plain icon when no color", func() {
		theme := color.NewTheme(false)
		r := doctor.Pass("test", "ok")
		Expect(reporters.StyledIcon(r, theme)).To(Equal("✓"))
	})

	It("returns icon with color theme applied", func() {
		// lipgloss may strip ANSI in non-TTY, just verify icon content
		theme := color.NewTheme(true)
		r := doctor.FailError("test", "bad")
		icon := reporters.StyledIcon(r, theme)
		Expect(icon).To(ContainSubstring("✗"))
	})

	It("returns warning styled icon for warnings", func() {
		theme := color.NewTheme(false)
		r := doctor.FailWarning("test", "meh")
		icon := reporters.StyledIcon(r, theme)
		Expect(icon).To(Equal("!"))
	})

	It("returns skip styled icon", func() {
		theme := color.NewTheme(false)
		r := doctor.Skip("test", "skipped")
		icon := reporters.StyledIcon(r, theme)
		Expect(icon).To(Equal("-"))
	})

	It("returns plain icon for unknown status", func() {
		theme := color.NewTheme(false)
		r := doctor.CheckResult{Status: "unknown"}
		icon := reporters.StyledIcon(r, theme)
		Expect(icon).To(Equal("?"))
	})
})

var _ = Describe("StatusIcon default", func() {
	It("returns ? for unknown status", func() {
		r := doctor.CheckResult{Status: "something-else"}
		Expect(reporters.StatusIcon(r)).To(Equal("?"))
	})

	It("returns i for fail with info severity", func() {
		r := doctor.CheckResult{
			Status:   doctor.StatusFail,
			Severity: doctor.SeverityInfo,
		}
		Expect(reporters.StatusIcon(r)).To(Equal("i"))
	})
})

var _ = Describe("padToWidth", func() {
	It("pads short text to target width", func() {
		result := reporters.PadToWidth("hi", 10)
		Expect(result).To(Equal("hi        "))
		Expect(len(result)).To(Equal(10))
	})

	It("returns text unchanged when already at width", func() {
		result := reporters.PadToWidth("hello", 5)
		Expect(result).To(Equal("hello"))
	})

	It("returns text unchanged when wider than target", func() {
		result := reporters.PadToWidth("hello world", 5)
		Expect(result).To(Equal("hello world"))
	})

	It("handles empty string", func() {
		result := reporters.PadToWidth("", 5)
		Expect(result).To(Equal("     "))
	})

	It("handles ANSI-styled text by padding based on visible width", func() {
		// \x1b[1mhi\x1b[0m = "hi" with bold, visible width 2
		styled := "\x1b[1mhi\x1b[0m"
		result := reporters.PadToWidth(styled, 10)
		// Should add 8 spaces (10 - 2 visible chars)
		Expect(result).To(HavePrefix(styled))
		Expect(strings.TrimLeft(result[len(styled):], " ")).To(BeEmpty())
	})
})

var _ = Describe("toCellWidths", func() {
	It("adds padding to each content width", func() {
		content := map[int]int{0: 1, 1: 10, 2: 30}
		cell := reporters.ToCellWidths(content)
		Expect(cell[0]).To(Equal(3))  // 1 + 2
		Expect(cell[1]).To(Equal(12)) // 10 + 2
		Expect(cell[2]).To(Equal(32)) // 30 + 2
	})

	It("returns empty map for empty input", func() {
		cell := reporters.ToCellWidths(map[int]int{})
		Expect(cell).To(BeEmpty())
	})
})

var _ = Describe("calcColumnWidthsFor", func() {
	It("returns nil when width is below minimum", func() {
		widths := reporters.CalcColumnWidthsFor(30, sampleResults(), false)
		Expect(widths).To(BeNil())
	})

	It("returns nil when width is zero", func() {
		widths := reporters.CalcColumnWidthsFor(0, sampleResults(), false)
		Expect(widths).To(BeNil())
	})

	It("returns column widths for 80-char terminal", func() {
		widths := reporters.CalcColumnWidthsFor(80, sampleResults(), false)
		Expect(widths).NotTo(BeNil())
		Expect(widths).To(HaveLen(3))
		Expect(widths[0]).To(Equal(1)) // icon
		// Check + message should sum to available space
		Expect(widths[1] + widths[2] + widths[0]).
			To(Equal(80 - (3*3 + 1))) // width - overhead
	})

	It("returns 4 columns in verbose mode", func() {
		widths := reporters.CalcColumnWidthsFor(120, sampleResults(), true)
		Expect(widths).NotTo(BeNil())
		Expect(widths).To(HaveLen(4))
		Expect(widths[0]).To(Equal(1)) // icon
		// Message gets 60% of remaining, details gets 40%
		remaining := widths[2] + widths[3]
		Expect(widths[2]).To(Equal(remaining * 60 / 100))
	})

	It("caps check name width on narrow terminals", func() {
		results := []doctor.CheckResult{
			{Name: "a very long check name that would dominate narrow terminals"},
		}
		widths := reporters.CalcColumnWidthsFor(60, results, false)
		Expect(widths).NotTo(BeNil())
		// Message column should get at least 20 chars
		Expect(widths[2]).To(BeNumerically(">=", 20))
	})

	It("returns nil when available space is too small", func() {
		// 3 cols * 3 overhead + 1 border + 1 icon = 11
		// available = 45 - 11 = 34, needs minMsgW(20) + minCheckW(5) = 25
		// With width 36: available = 36 - 11 = 25 - exactly at boundary
		// With width 35: available = 35 - 11 = 24 - below threshold
		widths := reporters.CalcColumnWidthsFor(35, sampleResults(), false)
		Expect(widths).To(BeNil())
	})
})

var _ = Describe("buildResultRow", func() {
	theme := color.NewTheme(false)

	It("builds row without padding when colWidths is nil", func() {
		r := doctor.Pass("my-check", "all good")
		row := reporters.BuildResultRow(r, false, nil, theme)
		Expect(row).To(HaveLen(3))
		Expect(row[0]).To(Equal("✓"))
		Expect(row[1]).To(Equal("my-check"))
		Expect(row[2]).To(Equal("all good"))
	})

	It("pads cells when colWidths is set", func() {
		r := doctor.Pass("test", "ok")
		colWidths := map[int]int{0: 1, 1: 10, 2: 20}
		row := reporters.BuildResultRow(r, false, colWidths, theme)
		Expect(row).To(HaveLen(3))
		// Icon should be 1 char wide (no padding needed for "✓" which is already 1)
		// Name "test" padded to 10
		Expect(len(row[1])).To(Equal(10))
		// Message "ok" padded to 20
		Expect(len(row[2])).To(Equal(20))
	})

	It("includes details column in verbose mode", func() {
		r := doctor.CheckResult{
			Name:     "perms",
			Status:   doctor.StatusFail,
			Severity: doctor.SeverityError,
			Message:  "bad perms",
			Details:  []string{"chmod 755", "check owner"},
		}
		row := reporters.BuildResultRow(r, true, nil, theme)
		Expect(row).To(HaveLen(4))
		Expect(row[3]).To(ContainSubstring("chmod 755"))
		Expect(row[3]).To(ContainSubstring("check owner"))
	})
})

var _ = Describe("severityRank", func() {
	It("ranks error as 0", func() {
		r := doctor.FailError("test", "bad")
		Expect(reporters.SeverityRank(r)).To(Equal(0))
	})

	It("ranks warning as 1", func() {
		r := doctor.FailWarning("test", "meh")
		Expect(reporters.SeverityRank(r)).To(Equal(1))
	})

	It("ranks pass as 2", func() {
		r := doctor.Pass("test", "ok")
		Expect(reporters.SeverityRank(r)).To(Equal(2))
	})

	It("ranks skipped as 3", func() {
		r := doctor.Skip("test", "skip")
		Expect(reporters.SeverityRank(r)).To(Equal(3))
	})
})

var _ = Describe("shortenPath", func() {
	var origHome string

	BeforeEach(func() {
		origHome, _ = os.UserHomeDir()
	})

	AfterEach(func() {
		reporters.SetHomeDir(origHome)
	})

	It("replaces home dir prefix with ~", func() {
		reporters.SetHomeDir("/home/user")

		result := reporters.ShortenPath("/home/user/.klaudiush/config.toml")
		Expect(result).To(Equal("~/.klaudiush/config.toml"))
	})

	It("leaves paths without home dir unchanged", func() {
		reporters.SetHomeDir("/home/user")

		result := reporters.ShortenPath("/etc/config")
		Expect(result).To(Equal("/etc/config"))
	})

	It("returns string unchanged when homeDir is empty", func() {
		reporters.SetHomeDir("")

		result := reporters.ShortenPath("/some/path")
		Expect(result).To(Equal("/some/path"))
	})
})

var _ = Describe("dimBorders", func() {
	It("replaces box-drawing chars with styled versions", func() {
		theme := color.NewTheme(false)
		input := "╭──╮\n│hi│\n╰──╯"
		result := reporters.DimBorders(input, theme)
		// With no-color theme, styles are no-op, so output should be same
		Expect(result).To(Equal(input))
	})

	It("leaves non-border content unchanged", func() {
		theme := color.NewTheme(false)
		input := "hello world"
		result := reporters.DimBorders(input, theme)
		Expect(result).To(Equal("hello world"))
	})
})

var _ = Describe("RenderTable with verbose", func() {
	It("renders details column with joined details", func() {
		theme := color.NewTheme(false)
		results := []doctor.CheckResult{
			{
				Name:     "multi-detail",
				Category: doctor.CategoryBinary,
				Status:   doctor.StatusFail,
				Severity: doctor.SeverityError,
				Message:  "failed",
				Details:  []string{"detail one", "detail two"},
			},
		}
		output := reporters.RenderTable(results, true, theme)
		Expect(output).To(ContainSubstring("detail one; detail two"))
	})
})
