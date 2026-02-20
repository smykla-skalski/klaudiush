package patterns_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Advisor", func() {
	var (
		store   *patterns.FilePatternStore
		advisor *patterns.Advisor
		cfg     *config.PatternsConfig
	)

	BeforeEach(func() {
		tmpDir := GinkgoT().TempDir()
		cfg = &config.PatternsConfig{
			ProjectDataFile: "patterns.json",
			GlobalDataDir:   filepath.Join(tmpDir, "global"),
		}
		store = patterns.NewFilePatternStore(cfg, tmpDir)

		// Load seed data
		store.SetProjectData(patterns.SeedPatterns())

		advisor = patterns.NewAdvisor(store, cfg)
	})

	It("returns nil for empty codes", func() {
		warnings := advisor.Advise(nil)
		Expect(warnings).To(BeNil())
	})

	It("returns warnings for known patterns", func() {
		warnings := advisor.Advise([]string{"GIT013"})
		Expect(warnings).NotTo(BeEmpty())
		Expect(warnings[0]).To(ContainSubstring("GIT013"))
		Expect(warnings[0]).To(ContainSubstring("Pattern hint"))
	})

	It("includes human-readable code descriptions", func() {
		warnings := advisor.Advise([]string{"GIT013"})
		Expect(warnings).NotTo(BeEmpty())
		Expect(warnings[0]).To(ContainSubstring("conventional format"))
	})

	It("caps warnings per error", func() {
		// GIT013 has two follow-ups (GIT004, GIT006) in seeds
		cfg.MaxWarningsPerError = 1
		advisor = patterns.NewAdvisor(store, cfg)

		warnings := advisor.Advise([]string{"GIT013"})
		Expect(len(warnings)).To(Equal(1))
	})

	It("caps total warnings", func() {
		cfg.MaxWarningsTotal = 1
		advisor = patterns.NewAdvisor(store, cfg)

		warnings := advisor.Advise([]string{"GIT013", "GIT004"})
		Expect(len(warnings)).To(Equal(1))
	})

	It("returns nothing for unknown codes", func() {
		warnings := advisor.Advise([]string{"UNKNOWN001"})
		Expect(warnings).To(BeEmpty())
	})

	It("uses code as fallback description for unknown codes", func() {
		// Record enough to exceed seed counts so CUSTOM099 ranks first
		for range 20 {
			store.RecordSequence("GIT013", "CUSTOM099")
		}

		// Cap at 1 so only the highest count pattern is shown
		cfg.MaxWarningsPerError = 1
		advisor = patterns.NewAdvisor(store, cfg)

		warnings := advisor.Advise([]string{"GIT013"})
		Expect(warnings).To(HaveLen(1))
		// Unknown code uses the code itself as description
		Expect(warnings[0]).To(ContainSubstring("CUSTOM099 (CUSTOM099)"))
	})

	It("shows highest count patterns first when capped", func() {
		// Add a high-count learned pattern
		for range 20 {
			store.RecordSequence("GIT013", "GIT005")
		}

		// GIT013 now has: GIT005 (count=20), GIT004 (seed=5), GIT006 (seed=5)
		// Cap at 1 per error to test which one is picked
		cfg.MaxWarningsPerError = 1
		advisor = patterns.NewAdvisor(store, cfg)

		warnings := advisor.Advise([]string{"GIT013"})
		Expect(warnings).To(HaveLen(1))
		Expect(warnings[0]).To(ContainSubstring("GIT005"))
	})

	It("respects minCount threshold", func() {
		// Record a single observation (below default min_count of 3)
		store.RecordSequence("GIT004", "GIT999")

		warnings := advisor.Advise([]string{"GIT004"})

		// Should have the seed GIT004->GIT005 but not GIT004->GIT999
		for _, w := range warnings {
			Expect(w).NotTo(ContainSubstring("GIT999"))
		}
	})
})
