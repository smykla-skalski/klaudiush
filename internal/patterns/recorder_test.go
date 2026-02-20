package patterns_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Recorder", func() {
	var (
		tmpDir   string
		cfg      *config.PatternsConfig
		store    *patterns.FilePatternStore
		recorder *patterns.Recorder
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		cfg = &config.PatternsConfig{
			ProjectDataFile: "patterns.json",
			GlobalDataDir:   filepath.Join(tmpDir, "global"),
		}
		store = patterns.NewFilePatternStore(cfg, tmpDir)
		recorder = patterns.NewRecorder(store)
	})

	It("does nothing on first observation", func() {
		recorder.Observe("sess1", []string{"GIT013"})
		Expect(store.GetAllPatterns()).To(BeEmpty())
	})

	It("records sequences on second observation", func() {
		recorder.Observe("sess1", []string{"GIT013"})
		recorder.Observe("sess1", []string{"GIT004"})

		all := store.GetAllPatterns()
		Expect(all).To(HaveLen(1))
		Expect(all[0].SourceCode).To(Equal("GIT013"))
		Expect(all[0].TargetCode).To(Equal("GIT004"))
	})

	It("records all pairs from multiple previous codes", func() {
		recorder.Observe("sess1", []string{"GIT013", "GIT006"})
		recorder.Observe("sess1", []string{"GIT004"})

		all := store.GetAllPatterns()
		Expect(all).To(HaveLen(2))

		sources := make(map[string]bool)
		for _, p := range all {
			sources[p.SourceCode] = true
			Expect(p.TargetCode).To(Equal("GIT004"))
		}

		Expect(sources).To(HaveKey("GIT013"))
		Expect(sources).To(HaveKey("GIT006"))
	})

	It("clears session on pass (empty codes)", func() {
		recorder.Observe("sess1", []string{"GIT013"})
		recorder.Observe("sess1", []string{})

		// Next observation should not record any sequence
		recorder.Observe("sess1", []string{"GIT005"})
		Expect(store.GetAllPatterns()).To(BeEmpty())
	})

	It("tracks sessions independently", func() {
		recorder.Observe("sess1", []string{"GIT013"})
		recorder.Observe("sess2", []string{"GIT005"})

		recorder.Observe("sess1", []string{"GIT004"})
		recorder.Observe("sess2", []string{"GIT016"})

		all := store.GetAllPatterns()
		Expect(all).To(HaveLen(2))
	})

	It("skips self-referencing pairs", func() {
		recorder.Observe("sess1", []string{"GIT013"})
		recorder.Observe("sess1", []string{"GIT013"})

		Expect(store.GetAllPatterns()).To(BeEmpty())
	})

	It("ClearSession removes session state", func() {
		recorder.Observe("sess1", []string{"GIT013"})
		recorder.ClearSession("sess1")
		recorder.Observe("sess1", []string{"GIT004"})

		Expect(store.GetAllPatterns()).To(BeEmpty())
	})

	It("persists session state across save/load cycles", func() {
		// First invocation: observe GIT013
		recorder.Observe("sess1", []string{"GIT013"})
		Expect(store.Save()).To(Succeed())

		// Simulate new process: create fresh store and recorder
		store2 := patterns.NewFilePatternStore(cfg, tmpDir)
		Expect(store2.Load()).To(Succeed())
		recorder2 := patterns.NewRecorder(store2)

		// Second invocation: observe GIT004 -> should record GIT013->GIT004
		recorder2.Observe("sess1", []string{"GIT004"})

		all := store2.GetAllPatterns()
		Expect(all).To(HaveLen(1))
		Expect(all[0].SourceCode).To(Equal("GIT013"))
		Expect(all[0].TargetCode).To(Equal("GIT004"))
	})

	It("sets LastSeen on session entries during Observe", func() {
		before := time.Now()

		recorder.Observe("sess1", []string{"GIT013"})

		after := time.Now()

		sessions := store.GetSessions()
		Expect(sessions).To(HaveKey("sess1"))
		Expect(sessions["sess1"].LastSeen).To(BeTemporally(">=", before))
		Expect(sessions["sess1"].LastSeen).To(BeTemporally("<=", after))
	})

	It("session cleanup does not break recording flow", func() {
		recorder.Observe("sess1", []string{"GIT013"})

		// Cleanup with a long maxAge keeps the session
		removed := store.CleanupSessions(time.Hour)
		Expect(removed).To(Equal(0))

		// Recording still works
		recorder.Observe("sess1", []string{"GIT004"})

		all := store.GetAllPatterns()
		Expect(all).To(HaveLen(1))
		Expect(all[0].SourceCode).To(Equal("GIT013"))
		Expect(all[0].TargetCode).To(Equal("GIT004"))
	})

	It("clears persisted session state on pass", func() {
		recorder.Observe("sess1", []string{"GIT013"})
		Expect(store.Save()).To(Succeed())

		// New process: pass (empty codes)
		store2 := patterns.NewFilePatternStore(cfg, tmpDir)
		Expect(store2.Load()).To(Succeed())
		recorder2 := patterns.NewRecorder(store2)
		recorder2.Observe("sess1", []string{})
		Expect(store2.Save()).To(Succeed())

		// Third process: new error should not link to GIT013
		store3 := patterns.NewFilePatternStore(cfg, tmpDir)
		Expect(store3.Load()).To(Succeed())
		recorder3 := patterns.NewRecorder(store3)
		recorder3.Observe("sess1", []string{"GIT005"})

		Expect(store3.GetAllPatterns()).To(BeEmpty())
	})
})
