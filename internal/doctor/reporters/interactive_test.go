package reporters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/reporters"
)

// instantChecker returns a result immediately.
type instantChecker struct {
	name     string
	category doctor.Category
	result   doctor.CheckResult
}

func (c *instantChecker) Name() string                               { return c.name }
func (c *instantChecker) Category() doctor.Category                  { return c.category }
func (c *instantChecker) Check(_ context.Context) doctor.CheckResult { return c.result }

var _ = Describe("InteractiveReporter", func() {
	Describe("implements StreamingReporter", func() {
		It("satisfies the StreamingReporter interface", func() {
			theme := color.NewTheme(false)
			reporter := reporters.NewInteractiveReporter(theme)

			var _ doctor.StreamingReporter = reporter
		})
	})

	Describe("RunAndReport", func() {
		It("returns results from all checkers", func() {
			theme := color.NewTheme(false)
			reporter := reporters.NewInteractiveReporter(theme)

			registry := doctor.NewRegistry()
			registry.RegisterChecker(&instantChecker{
				name:     "test-check",
				category: doctor.CategoryBinary,
				result:   doctor.Pass("test-check", "ok"),
			})
			registry.RegisterChecker(&instantChecker{
				name:     "test-fail",
				category: doctor.CategoryBinary,
				result:   doctor.FailError("test-fail", "bad"),
			})

			ctx := context.Background()
			results := reporter.RunAndReport(ctx, registry, false, nil)

			Expect(results).To(HaveLen(2))

			// Find results by name
			var passResult, failResult doctor.CheckResult

			for _, r := range results {
				switch r.Name {
				case "test-check":
					passResult = r
				case "test-fail":
					failResult = r
				}
			}

			Expect(passResult.Status).To(Equal(doctor.StatusPass))
			Expect(failResult.Status).To(Equal(doctor.StatusFail))
		})

		It("returns nil for empty registry", func() {
			theme := color.NewTheme(false)
			reporter := reporters.NewInteractiveReporter(theme)
			registry := doctor.NewRegistry()

			ctx := context.Background()
			results := reporter.RunAndReport(ctx, registry, false, nil)
			Expect(results).To(BeNil())
		})

		It("filters by category", func() {
			theme := color.NewTheme(false)
			reporter := reporters.NewInteractiveReporter(theme)

			registry := doctor.NewRegistry()
			registry.RegisterChecker(&instantChecker{
				name:     "bin-check",
				category: doctor.CategoryBinary,
				result:   doctor.Pass("bin-check", "ok"),
			})
			registry.RegisterChecker(&instantChecker{
				name:     "hook-check",
				category: doctor.CategoryHook,
				result:   doctor.Pass("hook-check", "ok"),
			})

			ctx := context.Background()
			results := reporter.RunAndReport(
				ctx,
				registry,
				false,
				[]doctor.Category{doctor.CategoryBinary},
			)
			Expect(results).To(HaveLen(1))
			Expect(results[0].Name).To(Equal("bin-check"))
		})
	})
})

var _ = Describe("ColoredReporter", func() {
	It("implements Reporter interface", func() {
		theme := color.NewTheme(false)
		reporter := reporters.NewColoredReporter(theme)

		var _ doctor.Reporter = reporter
	})
})
