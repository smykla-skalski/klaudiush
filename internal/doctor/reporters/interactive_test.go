package reporters_test

import (
	"context"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
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

	It("prints table and summary to stdout", func() {
		theme := color.NewTheme(false)
		reporter := reporters.NewColoredReporter(theme)

		// Capture stdout
		r, w, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		origStdout := os.Stdout
		os.Stdout = w

		results := []doctor.CheckResult{
			doctor.Pass("bin-check", "found"),
			doctor.FailError("perms", "bad perms"),
		}
		reporter.Report(results, false)

		w.Close()

		os.Stdout = origStdout

		output, err := io.ReadAll(r)
		Expect(err).NotTo(HaveOccurred())

		out := string(output)
		Expect(out).To(ContainSubstring("Checking klaudiush health..."))
		Expect(out).To(ContainSubstring("bin-check"))
		Expect(out).To(ContainSubstring("perms"))
		Expect(out).To(ContainSubstring("Summary:"))
		Expect(out).To(ContainSubstring("1 error(s)"))
	})
})

var _ = Describe("doctorModel", func() {
	Describe("newDoctorModel", func() {
		It("creates model with correct initial state", func() {
			checkers := []doctor.HealthChecker{
				&instantChecker{
					name:     "check-1",
					category: doctor.CategoryBinary,
					result:   doctor.Pass("check-1", "ok"),
				},
				&instantChecker{
					name:     "check-2",
					category: doctor.CategoryHook,
					result:   doctor.FailError("check-2", "bad"),
				},
			}

			theme := color.NewTheme(false)
			model := reporters.NewModelForTest(checkers, false, theme)

			// View should show running phase
			view := model.View()
			Expect(view).To(ContainSubstring("Checking klaudiush health..."))
			Expect(view).To(ContainSubstring("check-1"))
			Expect(view).To(ContainSubstring("check-2"))
		})
	})

	Describe("Update with checkDoneMsg", func() {
		It("transitions to phaseTable when all checks complete", func() {
			checkers := []doctor.HealthChecker{
				&instantChecker{
					name:     "only-check",
					category: doctor.CategoryBinary,
					result:   doctor.Pass("only-check", "ok"),
				},
			}
			theme := color.NewTheme(false)
			model := reporters.NewModelForTest(checkers, false, theme)

			// Run Init to get commands
			cmd := model.Init()
			Expect(cmd).NotTo(BeNil())

			// Execute the batch - extract individual commands and run them
			// The first cmd is a Batch of spinner tick + check commands
			// Simulate a checkDoneMsg by running the check command
			result := doctor.Pass("only-check", "ok")
			result.Category = doctor.CategoryBinary

			// Create the message that runCheck would produce
			msg := cmd()
			// The batch returns multiple messages; find our checkDoneMsg
			// by running the model update loop
			if batchMsg, ok := msg.(tea.BatchMsg); ok {
				for _, c := range batchMsg {
					if c == nil {
						continue
					}

					innerMsg := c()

					var newCmd tea.Cmd

					model, newCmd = model.Update(innerMsg)
					_ = newCmd
				}
			}

			// After all checks done, view should be empty (phaseTable)
			view := model.View()
			Expect(view).To(BeEmpty())
		})
	})

	Describe("results extraction", func() {
		It("extracts results from completed model", func() {
			checkers := []doctor.HealthChecker{
				&instantChecker{
					name:     "c1",
					category: doctor.CategoryBinary,
					result:   doctor.Pass("c1", "ok"),
				},
			}
			theme := color.NewTheme(false)
			model := reporters.NewModelForTest(checkers, false, theme)

			// Initially results should all be zero-value
			results := reporters.ModelResultsForTest(model)
			Expect(results).To(HaveLen(1))
			Expect(results[0].Name).To(BeEmpty()) // not yet populated
		})

		It("returns nil for non-doctorModel", func() {
			results := reporters.ModelResultsForTest(nil)
			Expect(results).To(BeNil())
		})
	})

	Describe("View in running phase", func() {
		It("shows category headers and check names", func() {
			checkers := []doctor.HealthChecker{
				&instantChecker{
					name:     "binary-check",
					category: doctor.CategoryBinary,
					result:   doctor.Pass("binary-check", "found"),
				},
				&instantChecker{
					name:     "hook-check",
					category: doctor.CategoryHook,
					result:   doctor.Pass("hook-check", "registered"),
				},
			}
			theme := color.NewTheme(false)
			model := reporters.NewModelForTest(checkers, false, theme)

			view := model.View()
			Expect(view).To(ContainSubstring("Binary"))
			Expect(view).To(ContainSubstring("Hook Registration"))
			Expect(view).To(ContainSubstring("binary-check"))
			Expect(view).To(ContainSubstring("hook-check"))
		})
	})

	Describe("InteractiveReporter.Report", func() {
		It("prints table and summary to stdout", func() {
			theme := color.NewTheme(false)
			reporter := reporters.NewInteractiveReporter(theme)

			r, w, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())

			origStdout := os.Stdout
			os.Stdout = w

			results := []doctor.CheckResult{
				doctor.Pass("test-check", "ok"),
			}
			reporter.Report(results, false)

			w.Close()

			os.Stdout = origStdout

			output, err := io.ReadAll(r)
			Expect(err).NotTo(HaveOccurred())

			out := string(output)
			Expect(out).To(ContainSubstring("Checking klaudiush health..."))
			Expect(out).To(ContainSubstring("test-check"))
			Expect(out).To(ContainSubstring("Summary:"))
		})
	})
})
