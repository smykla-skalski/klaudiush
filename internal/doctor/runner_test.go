package doctor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// mockStreamingReporter tracks calls to RunAndReport.
type mockStreamingReporter struct {
	reportCalled       bool
	runAndReportCalled bool
	results            []doctor.CheckResult
}

func (m *mockStreamingReporter) Report(_ []doctor.CheckResult, _ bool) {
	m.reportCalled = true
}

func (m *mockStreamingReporter) RunAndReport(
	_ context.Context,
	_ *doctor.Registry,
	_ bool,
	_ []doctor.Category,
) []doctor.CheckResult {
	m.runAndReportCalled = true

	return m.results
}

// mockBatchReporter only implements Reporter (not StreamingReporter).
type mockBatchReporter struct {
	reportCalled bool
}

func (m *mockBatchReporter) Report(_ []doctor.CheckResult, _ bool) {
	m.reportCalled = true
}

// noopLogger satisfies logger.Logger for testing.
type noopLogger struct{}

func (noopLogger) Debug(_ string, _ ...any) {}
func (noopLogger) Info(_ string, _ ...any)  {}
func (noopLogger) Error(_ string, _ ...any) {}

//nolint:ireturn // test mock
func (noopLogger) With(_ ...any) logger.Logger { return noopLogger{} }

var _ = Describe("Runner", func() {
	Describe("StreamingReporter branch", func() {
		It("uses RunAndReport when reporter implements StreamingReporter", func() {
			registry := doctor.NewRegistry()
			registry.RegisterChecker(&stubChecker{name: "test", category: doctor.CategoryBinary})

			sr := &mockStreamingReporter{
				results: []doctor.CheckResult{
					doctor.Pass("test", "ok"),
				},
			}

			runner := doctor.NewRunner(registry, sr, nil, noopLogger{})

			err := runner.Run(context.Background(), doctor.RunOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(sr.runAndReportCalled).To(BeTrue())
			Expect(sr.reportCalled).To(BeFalse())
		})

		It("uses batch path when reporter only implements Reporter", func() {
			registry := doctor.NewRegistry()
			registry.RegisterChecker(&stubChecker{name: "test", category: doctor.CategoryBinary})

			br := &mockBatchReporter{}
			runner := doctor.NewRunner(registry, br, nil, noopLogger{})

			err := runner.Run(context.Background(), doctor.RunOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(br.reportCalled).To(BeTrue())
		})
	})
})
