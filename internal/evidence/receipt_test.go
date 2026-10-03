package evidence_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

var _ = Describe("Receipt", func() {
	var (
		check *evidence.Check
		now   time.Time
	)

	alive := func(alive bool) func(int) bool {
		return func(int) bool { return alive }
	}

	receipt := func(status evidence.Status) *evidence.Receipt {
		return &evidence.Receipt{
			RunID:     evidence.NewRunID(),
			CheckID:   check.ID(),
			Check:     check.Name,
			Kind:      check.Kind,
			Status:    status,
			Digest:    "sha256:current",
			Source:    "claude",
			Command:   "make test",
			StartedAt: now.Add(-time.Minute),
		}
	}

	BeforeEach(func() {
		now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
		check = compileOne(&config.EvidenceCheckConfig{
			Name:     "tests",
			Commands: []string{"make test"},
			Timeout:  config.Duration(10 * time.Minute),
		})
	})

	It("is satisfied only by a pass on the current digest", func() {
		verdict := evidence.Judge(check, receipt(evidence.StatusPassed), "sha256:current", now, nil)
		Expect(verdict.Satisfied()).To(BeTrue())
		Expect(verdict.Reason).To(ContainSubstring("passed"))
	})

	DescribeTable("rejects everything else",
		func(mutate func(*evidence.Receipt), digest string, status evidence.Status, reason string) {
			item := receipt(evidence.StatusPassed)
			mutate(item)

			verdict := evidence.Judge(check, item, digest, now, alive(true))
			Expect(verdict.Satisfied()).To(BeFalse())
			Expect(verdict.Status).To(Equal(status))
			Expect(verdict.Reason).To(ContainSubstring(reason))
		},
		Entry("stale pass", func(*evidence.Receipt) {}, "sha256:other",
			evidence.StatusStale, "changed since"),
		Entry("failed", func(r *evidence.Receipt) {
			r.Status = evidence.StatusFailed
			code := 2
			r.ExitCode = &code
		}, "sha256:current", evidence.StatusFailed, "exit status 2"),
		Entry("failed without status", func(r *evidence.Receipt) {
			r.Status = evidence.StatusFailed
			r.Detail = "boom"
		}, "sha256:current", evidence.StatusFailed, "failed: boom"),
		Entry("running", func(r *evidence.Receipt) { r.Status = evidence.StatusRunning },
			"sha256:current", evidence.StatusRunning, "still running"),
		Entry("running past the timeout", func(r *evidence.Receipt) {
			r.Status = evidence.StatusRunning
			r.StartedAt = now.Add(-time.Hour)
		}, "sha256:current", evidence.StatusCanceled, "no result within"),
		Entry("canceled", func(r *evidence.Receipt) { r.Status = evidence.StatusCanceled },
			"sha256:current", evidence.StatusCanceled, "canceled"),
		Entry("unverified", func(r *evidence.Receipt) {
			r.Status = evidence.StatusUnverified
			r.Detail = "background"
		}, "sha256:current", evidence.StatusUnverified, "background"),
		Entry("stored stale", func(r *evidence.Receipt) { r.Status = evidence.StatusStale },
			"sha256:current", evidence.StatusStale, "stale"),
		Entry("unknown status", func(r *evidence.Receipt) { r.Status = "weird" },
			"sha256:current", evidence.StatusUnverified, "unknown result status"),
		Entry("old definition", func(r *evidence.Receipt) { r.CheckID = "tests@old" },
			"sha256:current", evidence.StatusMissing, "predates"),
	)

	It("reports a missing result", func() {
		verdict := evidence.Judge(check, nil, "sha256:current", now, nil)
		Expect(verdict.Status).To(Equal(evidence.StatusMissing))
		Expect(verdict.Receipt).To(BeNil())
	})

	It("treats a verifier that died as canceled", func() {
		item := receipt(evidence.StatusRunning)
		item.Source = evidence.SourceVerifier
		item.PID = 4242

		verdict := evidence.Judge(check, item, "sha256:current", now, alive(false))
		Expect(verdict.Status).To(Equal(evidence.StatusCanceled))
		Expect(verdict.Reason).To(ContainSubstring("stopped without reporting"))

		verdict = evidence.Judge(check, item, "sha256:current", now, alive(true))
		Expect(verdict.Status).To(Equal(evidence.StatusRunning))
	})

	It("ends stale when the files changed during the run", func() {
		item := receipt(evidence.StatusRunning)
		code := 0
		item.Finish(evidence.StatusPassed, &code, "sha256:after", "", now)

		Expect(item.Status).To(Equal(evidence.StatusStale))
		Expect(item.Detail).To(ContainSubstring("changed while the check ran"))
		Expect(item.EndDigest).To(Equal("sha256:after"))
		Expect(*item.FinishedAt).To(Equal(now))
	})

	It("keeps the outcome when the files did not change", func() {
		item := receipt(evidence.StatusRunning)
		item.Finish(evidence.StatusFailed, nil, "sha256:current", "boom", now)

		Expect(item.Status).To(Equal(evidence.StatusFailed))
		Expect(item.Detail).To(Equal("boom"))
	})

	It("keeps an unverified outcome without an end digest", func() {
		item := receipt(evidence.StatusRunning)
		item.Finish(evidence.StatusUnverified, nil, "", "no fingerprint", now)

		Expect(item.Status).To(Equal(evidence.StatusUnverified))
	})

	It("summarizes test and review receipts", func() {
		item := receipt(evidence.StatusPassed)
		item.Files = 3
		Expect(item.Summary()).To(ContainSubstring("tests passed"))
		Expect(item.Summary()).To(ContainSubstring("3 file(s)"))

		item.Base = "abc"
		item.Changed = []string{"a.go"}
		Expect(item.Summary()).To(ContainSubstring("base: abc, 1 changed file(s)"))
	})

	It("creates distinct run IDs", func() {
		Expect(evidence.NewRunID()).NotTo(Equal(evidence.NewRunID()))
	})

	DescribeTable("JudgeKept falls back to the kept pass",
		func(latestStatus evidence.Status, kept bool, status evidence.Status, fromKept bool) {
			var latest, pass *evidence.Receipt
			if latestStatus != "" {
				latest = receipt(latestStatus)
			}

			if kept {
				pass = receipt(evidence.StatusPassed)
			}

			verdict := evidence.JudgeKept(check, latest, pass, "sha256:current", now, alive(true))
			Expect(verdict.Status).To(Equal(status))

			if fromKept {
				Expect(verdict.Receipt).To(BeIdenticalTo(pass))
			} else {
				Expect(verdict.Receipt).To(BeIdenticalTo(latest))
			}
		},
		Entry("latest pass wins", evidence.StatusPassed, true, evidence.StatusPassed, false),
		Entry("running run keeps the pass",
			evidence.StatusRunning, true, evidence.StatusPassed, true),
		Entry("canceled run keeps the pass",
			evidence.StatusCanceled, true, evidence.StatusPassed, true),
		Entry("no latest uses the pass", evidence.Status(""), true, evidence.StatusPassed, true),
		Entry("failure on the same content wins",
			evidence.StatusFailed, true, evidence.StatusFailed, false),
		Entry("no kept pass reports the latest",
			evidence.StatusUnverified, false, evidence.StatusUnverified, false),
	)

	It("JudgeKept ignores a kept pass for other content", func() {
		pass := receipt(evidence.StatusPassed)
		pass.Digest = "sha256:old"

		latest := receipt(evidence.StatusCanceled)

		verdict := evidence.JudgeKept(check, latest, pass, "sha256:current", now, alive(true))
		Expect(verdict.Status).To(Equal(evidence.StatusCanceled))
		Expect(verdict.Receipt).To(BeIdenticalTo(latest))
	})
})

var _ = Describe("Coverage", func() {
	It("describes what each provider supports", func() {
		Expect(evidence.Coverage(hook.ProviderClaude)).To(ContainSubstring("both count"))
		Expect(
			evidence.Coverage(hook.ProviderCodex),
		).To(ContainSubstring("only 'klaudiush evidence run'"))
		Expect(
			evidence.Coverage(hook.ProviderGemini),
		).To(ContainSubstring("only 'klaudiush evidence run'"))
		Expect(evidence.Coverage(hook.ProviderOpenCode)).To(ContainSubstring("not gated"))
		Expect(evidence.CoverageLines()).To(HaveLen(len(evidence.Providers)))
	})
})

var _ = Describe("ProcessAlive", func() {
	It("knows this process runs", func() {
		Expect(evidence.ProcessAlive(os.Getpid())).To(BeTrue())
		Expect(evidence.ProcessAlive(0)).To(BeFalse())
	})
})
