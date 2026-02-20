package reporters_test

import (
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
})
