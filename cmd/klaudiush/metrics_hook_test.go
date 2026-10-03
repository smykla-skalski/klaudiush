package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("hook metrics", func() {
	var (
		home        string
		savedMode   string
		savedGrace  time.Duration
		savedConfig *config.Config
	)

	BeforeEach(func() {
		home = isolateHome()

		savedMode, savedGrace, savedConfig = failureMode, watchdogGrace, crashConfig
		failureMode = ""
		crashConfig = &config.Config{CrashDump: &config.CrashDumpConfig{Enabled: new(false)}}
	})

	AfterEach(func() {
		failureMode, watchdogGrace, crashConfig = savedMode, savedGrace, savedConfig
	})

	newRun := func(provider hook.Provider, event string) *hookRun {
		return newHookRun(
			provider,
			hook.ResolveLegacyEventType(provider, event, hook.EventTypeUnknown),
			event,
			logger.NewNoOpLogger(),
		)
	}

	load := func() []metrics.Record {
		GinkgoHelper()

		records, skipped, err := metrics.NewStore(nil).Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(skipped).To(BeZero())

		return records
	}

	It("records one prevented outcome when the watchdog answers for an overrun", func() {
		watchdogGrace = 10 * time.Millisecond

		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.setPolicy(failpolicy.New(&config.FailurePolicyConfig{
			Mode:     "block",
			Deadline: config.Duration(20 * time.Millisecond),
		}))

		release := make(chan struct{})

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				<-release

				return nil
			})).To(Succeed())
		})

		close(release)

		Expect(out).To(ContainSubstring(`"permissionDecision":"deny"`))

		records := load()
		Expect(records).To(HaveLen(1))
		Expect(records[0].Outcome).To(Equal(metrics.ClassPrevented))
		Expect(records[0].Findings).To(HaveLen(1))
		Expect(records[0].Findings[0].Unavailable).To(Equal("timeout"))
		Expect(records[0].Micros).To(BeNumerically(">", 0))
	})

	It("records a warned failure as unavailable, not enforced", func() {
		h := newRun(hook.ProviderCodex, "PreToolUse")

		captureStdout(func() {
			Expect(h.supervise(func() error {
				return failHook(validator.ReasonMalformedInput, errors.New("invalid JSON"))
			})).To(Succeed())
		})

		records := load()
		Expect(records).To(HaveLen(1))
		Expect(records[0].Provider).To(Equal("codex"))
		Expect(records[0].Outcome).To(Equal(metrics.ClassUnavailable))
		Expect(records[0].Findings[0].Unavailable).To(Equal("malformed_input"))
	})

	It("honors metrics.enabled from a configuration that failed to load", func() {
		configDir := filepath.Join(home, ".config", "klaudiush")
		Expect(os.MkdirAll(configDir, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
			"[metrics]\nenabled = false\n\n[validators.git.commit.message]\ntitle_max_length = -5\n",
		), 0o600)).To(Succeed())

		h := newRun(hook.ProviderClaude, "PreToolUse")

		captureStdout(func() {
			Expect(h.supervise(func() error {
				return failHook(validator.ReasonConfig, errors.New("broken config"))
			})).To(Succeed())
		})

		Expect(xdg.MetricsFile()).NotTo(BeAnExistingFile())
	})

	It("records what validation found, with checks and timings", func() {
		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.metrics.Store(&config.MetricsConfig{})
		h.outcome.Store(&dispatcher.Outcome{
			Checks:  []dispatcher.Check{{Validator: "validate-commit", Resource: "command"}},
			Timings: []dispatcher.Timing{{Validator: "validate-commit", Elapsed: time.Millisecond}},
		})

		hookCtx := &hook.Context{
			Provider:     hook.ProviderClaude,
			Event:        hook.CanonicalEventBeforeTool,
			RawEventName: "PreToolUse",
			ToolName:     hook.ToolTypeBash,
			SessionID:    "s",
			ToolInput:    hook.ToolInput{Command: "git commit"},
		}

		h.recordMetrics(hookCtx, []*dispatcher.ValidationError{{
			Validator:   "validate-commit",
			Reference:   validator.RefGitMissingFlags,
			ShouldBlock: true,
			Resource:    "command",
		}}, true)

		h.recordSelection(&hook.Context{
			Provider:     hook.ProviderGemini,
			Event:        hook.CanonicalEventToolSelection,
			RawEventName: "BeforeToolSelection",
		}, true)

		h.recordMetrics(nil, nil, false)

		records := load()
		Expect(records).To(HaveLen(2))
		Expect(records[0].Outcome).To(Equal(metrics.ClassPrevented))
		Expect(records[0].Checked).To(ConsistOf("commit"))
		Expect(records[0].Timings).To(HaveKeyWithValue("commit", int64(1000)))
		Expect(records[1].Outcome).To(Equal(metrics.ClassAdvisory))
	})

	It("records nothing when metrics are disabled", func() {
		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.metrics.Store(&config.MetricsConfig{Enabled: new(false)})

		h.recordMetrics(&hook.Context{Provider: hook.ProviderClaude}, nil, false)

		Expect(xdg.MetricsFile()).NotTo(BeAnExistingFile())
	})

	It("loses the sample, not the hook, when metrics cannot be written", func() {
		Expect(os.MkdirAll(filepath.Dir(filepath.Dir(xdg.MetricsFile())), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Dir(xdg.MetricsFile()), []byte("not a dir"), 0o600)).
			To(Succeed())

		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.metrics.Store(&config.MetricsConfig{})

		Expect(func() {
			h.recordMetrics(&hook.Context{Provider: hook.ProviderClaude}, nil, false)
		}).NotTo(Panic())
	})

	DescribeTable("parses report windows",
		func(value string, expected time.Duration, ok bool) {
			got, err := parseSince(value)
			if !ok {
				Expect(err).To(MatchError(errInvalidSince))

				return
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(expected))
		},
		Entry("days", "7d", 7*24*time.Hour, true),
		Entry("duration", "90m", 90*time.Minute, true),
		Entry("zero days", "0d", time.Duration(0), false),
		Entry("bad days", "xd", time.Duration(0), false),
		Entry("negative", "-1h", time.Duration(0), false),
		Entry("garbage", "soon", time.Duration(0), false),
	)
})
