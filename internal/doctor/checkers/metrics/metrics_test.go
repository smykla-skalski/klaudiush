package metrics_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	checker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/metrics"
	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("RecordChecker", func() {
	var path string

	BeforeEach(func() {
		path = filepath.Join(GinkgoT().TempDir(), "metrics", "outcomes.jsonl")
	})

	at := func(m *config.MetricsConfig) *metrics.Store {
		return metrics.NewStore(m, metrics.WithPath(path))
	}

	It("skips when metrics are disabled", func() {
		disabled := false
		result := checker.NewRecordCheckerWithStore(
			&config.Config{Metrics: &config.MetricsConfig{Enabled: &disabled}}, at,
		).Check(context.Background())

		Expect(result.IsSkipped()).To(BeTrue())
	})

	It("passes when the log is writable", func() {
		c := checker.NewRecordCheckerWithStore(nil, at)
		Expect(c.Category()).To(Equal(doctor.CategoryMetrics))

		result := c.Check(context.Background())
		Expect(result.IsPassed()).To(BeTrue())
		Expect(result.Message).To(ContainSubstring("of 16384 KB"))
		Expect(path).To(BeAnExistingFile())
	})

	It("warns when the log cannot be written", func() {
		Expect(os.WriteFile(filepath.Dir(path), []byte("file"), 0o600)).To(Succeed())

		result := checker.NewRecordCheckerWithStore(&config.Config{}, at).
			Check(context.Background())
		Expect(result.IsWarning()).To(BeTrue())
		Expect(result.Details).To(ContainElement(ContainSubstring(path)))
	})

	It("uses the state directory by default", func() {
		Expect(checker.NewRecordChecker(nil).Name()).To(Equal("Outcome metrics are recorded"))
	})
})
