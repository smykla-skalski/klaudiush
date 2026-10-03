package metrics_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	commitValidator = "validate-commit"
	mdValidator     = "validate-markdown"
	fileResource    = hook.ResourceFilePrefix + "/repo/README.md"
)

var _ = Describe("Summarize", func() {
	var (
		store *metrics.Store
		now   time.Time
	)

	record := func(obs *metrics.Observation) {
		now = now.Add(time.Second)
		obs.Time = now

		if obs.Elapsed == 0 {
			obs.Elapsed = 2 * time.Millisecond
		}

		Expect(store.Record(obs)).To(Succeed())
	}

	summarize := func(filter metrics.Filter) *metrics.Report {
		records, skipped, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(skipped).To(BeZero())

		return metrics.Summarize(records, time.Time{}, now, filter)
	}

	ctxFor := func(provider hook.Provider, event hook.CanonicalEvent, raw string) *hook.Context {
		return &hook.Context{
			Provider:     provider,
			Event:        event,
			RawEventName: raw,
			ToolName:     hook.ToolTypeBash,
			SessionID:    "session",
			ToolInput:    hook.ToolInput{Command: "git commit"},
		}
	}

	preTool := func() *hook.Context {
		return ctxFor(hook.ProviderClaude, hook.CanonicalEventBeforeTool, "PreToolUse")
	}

	afterWrite := func() *hook.Context {
		return &hook.Context{
			Provider:     hook.ProviderClaude,
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "PostToolUse",
			ToolName:     hook.ToolTypeWrite,
			ToolFamily:   hook.ToolFamilyWrite,
			SessionID:    "session",
			ToolInput:    hook.ToolInput{FilePath: "/repo/README.md"},
		}
	}

	selection := func() *hook.Context {
		return ctxFor(hook.ProviderGemini, hook.CanonicalEventToolSelection, "BeforeToolSelection")
	}

	stop := func() *hook.Context {
		return ctxFor(hook.ProviderClaude, hook.CanonicalEventTurnStop, "Stop")
	}

	blocking := func(ref validator.Reference) *dispatcher.ValidationError {
		return &dispatcher.ValidationError{
			Validator:   commitValidator,
			Reference:   ref,
			ShouldBlock: true,
			Resource:    hook.ResourceCommand,
		}
	}

	commitCheck := []dispatcher.Check{{Validator: commitValidator, Resource: hook.ResourceCommand}}

	BeforeEach(func() {
		now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
		store = metrics.NewStore(nil,
			metrics.WithPath(filepath.Join(GinkgoT().TempDir(), "outcomes.jsonl")),
			metrics.WithTimeFunc(func() time.Time { return now }),
		)
	})

	It("counts a deny before the tool as prevented", func() {
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Checks:  commitCheck,
			Stopped: true,
		})

		report := summarize(metrics.Filter{})
		Expect(report.Outcomes.Prevented).To(Equal(1))
		Expect(report.Outcomes.Enforced()).To(Equal(1))
		Expect(report.Codes).To(HaveLen(1))
		Expect(report.Codes[0].Code).To(Equal("GIT010"))
		Expect(report.Codes[0].Outcomes.Prevented).To(Equal(1))
	})

	It("never counts findings after a tool as prevented, even with a block decision", func() {
		record(&metrics.Observation{
			Context: ctxFor(hook.ProviderClaude, hook.CanonicalEventAfterTool, "PostToolUse"),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Stopped: true,
		})
		record(&metrics.Observation{
			Context: afterWrite(),
			Errors: []*dispatcher.ValidationError{{
				Validator: mdValidator,
				Reference: validator.RefGitConventionalCommit,
				Resource:  fileResource,
			}},
		})
		record(&metrics.Observation{
			Context: ctxFor(hook.ProviderCodex, hook.CanonicalEventAfterTool, "PostToolUse"),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Stopped: true,
		})

		report := summarize(metrics.Filter{})
		Expect(report.Outcomes.Prevented).To(BeZero())
		Expect(report.Outcomes.Enforced()).To(BeZero())
		Expect(report.Outcomes.Advisory).To(Equal(3))

		for _, code := range report.Codes {
			Expect(code.Outcomes.Prevented).To(BeZero())
		}
	})

	It("counts a blocking finding the response did not stop as advisory", func() {
		record(&metrics.Observation{
			Context: ctxFor(hook.ProviderClaude, hook.CanonicalEventNotification, "Notification"),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
		})

		Expect(summarize(metrics.Filter{}).Outcomes.Advisory).To(Equal(1))
	})

	It("tells held, released and passed completion gates apart", func() {
		record(&metrics.Observation{
			Context: stop(),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Stopped: true,
		})

		released := blocking(validator.RefGitMissingFlags)
		released.ShouldBlock = false
		record(&metrics.Observation{
			Context:  stop(),
			Errors:   []*dispatcher.ValidationError{released},
			Released: true,
		})
		record(&metrics.Observation{Context: stop()})

		report := summarize(metrics.Filter{})
		Expect(report.Outcomes.Held).To(Equal(1))
		Expect(report.Outcomes.Released).To(Equal(1))
		Expect(report.Outcomes.Passed).To(Equal(1))
		Expect(report.Events).To(HaveLen(1))
		Expect(report.Events[0].Event).To(Equal("Stop"))
		Expect(report.Repairs.Violations).To(Equal(1))
		Expect(report.Repairs.Retries).To(Equal(1))
		Expect(report.Repairs.Recurring).To(Equal(1))
		Expect(report.Repairs.Repaired).To(Equal(1))
		Expect(report.Repairs.FirstTry).To(BeZero())
	})

	It("reports unavailable checks by reason, and blocks they caused", func() {
		unavailable := &dispatcher.ValidationError{
			Validator:         "validate-shellscript",
			Reference:         validator.RefValidationUnavailable,
			Unavailable:       true,
			UnavailableReason: validator.ReasonMissingTool,
		}
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{unavailable},
		})

		blocked := *unavailable
		blocked.ShouldBlock = true
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{&blocked},
			Stopped: true,
		})

		record(&metrics.Observation{
			Context: preTool(),
			Errors: []*dispatcher.ValidationError{{
				Validator:   "klaudiush",
				Unavailable: true,
			}},
		})

		report := summarize(metrics.Filter{})
		Expect(report.Outcomes.Unavailable).To(Equal(2))
		Expect(report.Outcomes.Prevented).To(Equal(1))
		Expect(report.Unavailable).To(ConsistOf(
			metrics.UnavailableStats{
				Reason: "missing_tool", Validator: "shellscript", Count: 2, Blocked: 1,
			},
			metrics.UnavailableStats{Reason: "error", Validator: "klaudiush", Count: 1},
		))
		Expect(report.Repairs.Violations).To(BeZero())
	})

	It("follows a violation through retries to its repair", func() {
		for range 3 {
			record(&metrics.Observation{
				Context: preTool(),
				Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
				Checks:  commitCheck,
				Stopped: true,
			})
		}

		record(&metrics.Observation{Context: preTool(), Checks: commitCheck})

		repairs := summarize(metrics.Filter{}).Repairs
		Expect(repairs.Violations).To(Equal(1))
		Expect(repairs.Retries).To(Equal(2))
		Expect(repairs.Recurring).To(Equal(1))
		Expect(repairs.Repaired).To(Equal(1))
		Expect(repairs.FirstTry).To(BeZero())
		Expect(repairs.MeanRetries).To(Equal(2.0))
		Expect(repairs.Unresolved).To(BeZero())
	})

	It("repairs a code the validator stops reporting while it reports another", func() {
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Checks:  commitCheck,
			Stopped: true,
		})
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitConventionalCommit)},
			Checks:  commitCheck,
			Stopped: true,
		})

		report := summarize(metrics.Filter{})
		Expect(report.Repairs.Violations).To(Equal(2))
		Expect(report.Repairs.Repaired).To(Equal(1))
		Expect(report.Repairs.FirstTry).To(Equal(1))
		Expect(report.Repairs.Unresolved).To(Equal(1))
	})

	It("tracks after-tool findings until a recheck of the file clears them", func() {
		finding := &dispatcher.ValidationError{
			Validator: mdValidator,
			Reference: validator.RefGitConventionalCommit,
			Resource:  fileResource,
		}
		mdCheck := []dispatcher.Check{{Validator: mdValidator, Resource: fileResource}}

		record(&metrics.Observation{
			Context: afterWrite(),
			Errors:  []*dispatcher.ValidationError{finding},
			Checks:  mdCheck,
		})
		record(&metrics.Observation{
			Context: ctxFor(hook.ProviderClaude, hook.CanonicalEventAfterTool, "PostToolUse"),
			Checks:  mdCheck,
		})

		repairs := summarize(metrics.Filter{}).Repairs
		Expect(repairs.Violations).To(Equal(1))
		Expect(repairs.Repaired).To(Equal(1))
		Expect(repairs.FirstTry).To(Equal(1))
	})

	It("closes a violation an exception accepted, and keeps a false-positive signal", func() {
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Stopped: true,
		})

		excepted := blocking(validator.RefGitMissingFlags)
		excepted.ShouldBlock = false
		excepted.Bypassed = true
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{excepted},
		})

		report := summarize(metrics.Filter{})
		Expect(report.Outcomes.Excepted).To(Equal(1))
		Expect(report.Repairs.Exceptions).To(Equal(1))
		Expect(report.Repairs.Unresolved).To(BeZero())
		Expect(report.Codes[0].Outcomes.Excepted).To(Equal(1))
		Expect(report.Codes[0].Exceptions).To(Equal(1))
	})

	It("counts violations without a session as uncorrelated", func() {
		ctx := preTool()
		ctx.SessionID = ""

		record(&metrics.Observation{
			Context: ctx,
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Stopped: true,
		})

		repairs := summarize(metrics.Filter{}).Repairs
		Expect(repairs.Uncorrelated).To(Equal(1))
		Expect(repairs.Violations).To(BeZero())
	})

	It("keeps skipped hooks and tool selections apart from enforcement", func() {
		record(&metrics.Observation{Context: preTool(), Skipped: true})
		record(&metrics.Observation{
			Context: stop(),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Stopped: true,
			Skipped: true,
		})
		record(&metrics.Observation{
			Context:  selection(),
			Filtered: true,
		})
		record(&metrics.Observation{Context: selection()})
		record(&metrics.Observation{
			Context: preTool(),
			Errors: []*dispatcher.ValidationError{{
				Validator: commitValidator,
				Reference: validator.RefGitMissingFlags,
			}},
		})

		report := summarize(metrics.Filter{})
		Expect(report.Outcomes.Skipped).To(Equal(1))
		Expect(report.Outcomes.Advisory).To(Equal(1))
		Expect(report.Outcomes.Passed).To(Equal(1))
		Expect(report.Outcomes.Warned).To(Equal(1))
		Expect(report.Outcomes.Held).To(Equal(1))

		gemini := summarize(metrics.Filter{Provider: "GEMINI", Event: "beforetoolselection"})
		Expect(gemini.Records).To(Equal(2))
	})

	It("summarizes hook and validator latency", func() {
		for i := 1; i <= 20; i++ {
			record(&metrics.Observation{
				Context: preTool(),
				Elapsed: time.Duration(i) * time.Millisecond,
				Timings: []dispatcher.Timing{
					{
						Validator: commitValidator,
						Elapsed:   time.Duration(i) * 100 * time.Microsecond,
					},
					{
						Validator: commitValidator,
						Elapsed:   time.Duration(i) * 100 * time.Microsecond,
					},
					{Validator: mdValidator, Elapsed: time.Microsecond},
				},
			})
		}

		report := summarize(metrics.Filter{})
		Expect(report.Latency).To(Equal(metrics.Latency{Count: 20, P50: 10, P95: 19, Max: 20}))
		Expect(report.Events[0].Latency.P95).To(Equal(19.0))
		Expect(report.Validators).To(HaveLen(2))
		Expect(report.Validators[0].Validator).To(Equal("commit"))
		Expect(report.Validators[0].Latency.Max).To(Equal(4.0))
	})

	It("renders text and JSON", func() {
		record(&metrics.Observation{
			Context: preTool(),
			Errors:  []*dispatcher.ValidationError{blocking(validator.RefGitMissingFlags)},
			Checks:  commitCheck,
			Stopped: true,
			Timings: []dispatcher.Timing{{Validator: commitValidator, Elapsed: time.Millisecond}},
		})

		unavailable := &dispatcher.ValidationError{
			Validator:         "validate-shellscript",
			Unavailable:       true,
			UnavailableReason: validator.ReasonTimeout,
		}
		ctx := preTool()
		ctx.SessionID = ""
		record(&metrics.Observation{
			Context: ctx,
			Errors: []*dispatcher.ValidationError{
				unavailable,
				blocking(validator.RefGitMissingFlags),
			},
		})

		report := summarize(metrics.Filter{})
		report.SkippedLines = 2

		var out bytes.Buffer
		Expect(metrics.Render(&out, report)).To(Succeed())
		Expect(out.String()).To(ContainSubstring("Enforced: 1 (prevented 1"))
		Expect(out.String()).To(ContainSubstring("2 unreadable lines skipped"))
		Expect(out.String()).To(ContainSubstring("claude    PreToolUse"))
		Expect(out.String()).To(ContainSubstring("GIT010"))
		Expect(out.String()).To(ContainSubstring("timeout"))
		Expect(out.String()).To(ContainSubstring("1 reports without a session"))
		Expect(out.String()).To(ContainSubstring("Slowest validators"))

		data, err := json.Marshal(report)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"prevented":1`))

		out.Reset()
		Expect(
			metrics.Render(&out, metrics.Summarize(nil, now, now, metrics.Filter{})),
		).To(Succeed())
		Expect(out.String()).To(ContainSubstring("No hooks recorded"))
	})
})
