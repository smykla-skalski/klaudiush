package main

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/filelock"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func gateBlocking() []*dispatcher.ValidationError {
	return []*dispatcher.ValidationError{
		{
			Validator:   "lifecycle.rules",
			Message:     "tests not run",
			ShouldBlock: true,
			Reference:   validator.RefGitNoSignoff,
		},
		{Validator: "markdown", Message: "heading style"},
	}
}

func gateCtx(provider hook.Provider, raw, sessionID string, active bool) *hook.Context {
	return &hook.Context{
		Provider:       provider,
		Event:          hook.NormalizeEventName(raw),
		RawEventName:   raw,
		SessionID:      sessionID,
		StopHookActive: active,
	}
}

var _ = Describe("applyCompletionGate", func() {
	var (
		store *hooksession.Store
		log   logger.Logger
	)

	BeforeEach(func() {
		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(GinkgoT().TempDir(), "state.json")),
		)
		log = logger.NewNoOpLogger()
	})

	DescribeTable("keeps blocking up to the limit, then releases with a diagnostic",
		func(provider hook.Provider, raw string) {
			for attempt := 1; attempt <= maxCompletionBlocks; attempt++ {
				errs, notice := applyCompletionGate(
					store, gateCtx(provider, raw, "sess", attempt > 1), gateBlocking(), log,
				)
				Expect(dispatcher.ShouldBlock(errs)).To(BeTrue(), "attempt %d", attempt)
				Expect(notice).To(BeEmpty())
			}

			errs, notice := applyCompletionGate(
				store, gateCtx(provider, raw, "sess", true), gateBlocking(), log,
			)
			Expect(dispatcher.ShouldBlock(errs)).To(BeFalse())
			Expect(errs).To(HaveLen(2))
			Expect(errs[0].Message).To(Equal("tests not run"))
			Expect(notice).To(ContainSubstring(raw))
			Expect(notice).To(ContainSubstring("unresolved"))

			errs, _ = applyCompletionGate(
				store, gateCtx(provider, raw, "sess", true), gateBlocking(), log,
			)
			Expect(dispatcher.ShouldBlock(errs)).To(BeFalse(), "streak stays exhausted")

			errs, _ = applyCompletionGate(
				store, gateCtx(provider, raw, "sess", false), gateBlocking(), log,
			)
			Expect(dispatcher.ShouldBlock(errs)).To(BeTrue(), "a fresh stop restarts the count")
		},
		Entry("Claude Stop", hook.ProviderClaude, "Stop"),
		Entry("Claude SubagentStop", hook.ProviderClaude, "SubagentStop"),
		Entry("Codex Stop", hook.ProviderCodex, "Stop"),
		Entry("Gemini AfterAgent", hook.ProviderGemini, "AfterAgent"),
	)

	It("does not mutate the caller's findings when releasing", func() {
		original := gateBlocking()

		for range maxCompletionBlocks {
			applyCompletionGate(
				store,
				gateCtx(hook.ProviderClaude, "Stop", "s", true),
				original,
				log,
			)
		}

		applyCompletionGate(store, gateCtx(hook.ProviderClaude, "Stop", "s", true), original, log)
		Expect(original[0].ShouldBlock).To(BeTrue())
	})

	It("starts a new streak when the provider was not continued by a block", func() {
		for range maxCompletionBlocks {
			applyCompletionGate(
				store,
				gateCtx(hook.ProviderClaude, "Stop", "s", true),
				gateBlocking(),
				log,
			)
		}

		errs, notice := applyCompletionGate(
			store, gateCtx(hook.ProviderClaude, "Stop", "s", false), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())
		Expect(notice).To(BeEmpty())
	})

	It("resets the streak when the gate passes", func() {
		for range maxCompletionBlocks {
			applyCompletionGate(
				store,
				gateCtx(hook.ProviderClaude, "Stop", "s", true),
				gateBlocking(),
				log,
			)
		}

		_, notice := applyCompletionGate(
			store,
			gateCtx(hook.ProviderClaude, "Stop", "s", true),
			[]*dispatcher.ValidationError{{Validator: "markdown", Message: "warn"}},
			log,
		)
		Expect(notice).To(BeEmpty())

		errs, _ := applyCompletionGate(
			store, gateCtx(hook.ProviderClaude, "Stop", "s", true), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())
	})

	It("counts gates independently", func() {
		for range maxCompletionBlocks {
			applyCompletionGate(
				store,
				gateCtx(hook.ProviderClaude, "Stop", "s", true),
				gateBlocking(),
				log,
			)
		}

		errs, _ := applyCompletionGate(
			store, gateCtx(hook.ProviderClaude, "SubagentStop", "s", true), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())
	})

	It("counts parallel subagents separately", func() {
		subagent := func(agentID string, active bool) *hook.Context {
			ctx := gateCtx(hook.ProviderClaude, "SubagentStop", "s", active)
			ctx.AgentID = agentID

			return ctx
		}

		applyCompletionGate(store, subagent("a", false), gateBlocking(), log)

		for range maxCompletionBlocks - 1 {
			applyCompletionGate(store, subagent("a", true), gateBlocking(), log)
			applyCompletionGate(store, subagent("b", false), gateBlocking(), log)
		}

		errs, notice := applyCompletionGate(store, subagent("a", true), gateBlocking(), log)
		Expect(dispatcher.ShouldBlock(errs)).To(BeFalse())
		Expect(notice).NotTo(BeEmpty())

		errs, _ = applyCompletionGate(store, subagent("b", true), gateBlocking(), log)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())
	})

	It("allows one continuation without a session id", func() {
		errs, _ := applyCompletionGate(
			store, gateCtx(hook.ProviderClaude, "Stop", "", false), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())

		errs, notice := applyCompletionGate(
			store, gateCtx(hook.ProviderClaude, "Stop", "", true), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeFalse())
		Expect(notice).To(ContainSubstring("after 1 continuation"))
	})

	It("releases on a continued stop when the counter cannot be stored", func() {
		stateFile := filepath.Join(GinkgoT().TempDir(), "state.json")
		locked := hooksession.NewStore(
			hooksession.WithStateFile(stateFile),
			hooksession.WithLockTimeout(10*time.Millisecond),
		)

		held, err := filelock.Acquire(stateFile+".lock", time.Second)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(held.Release)

		errs, _ := applyCompletionGate(
			locked, gateCtx(hook.ProviderClaude, "Stop", "s", false), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())

		errs, notice := applyCompletionGate(
			locked, gateCtx(hook.ProviderClaude, "Stop", "s", true), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeFalse())
		Expect(notice).NotTo(BeEmpty())
	})

	It("recovers from corrupt state and keeps counting", func() {
		dir := GinkgoT().TempDir()
		stateFile := filepath.Join(dir, "state.json")
		Expect(os.WriteFile(stateFile, []byte("{not json"), 0o600)).To(Succeed())
		recovered := hooksession.NewStore(hooksession.WithStateFile(stateFile))

		errs, notice := applyCompletionGate(
			recovered, gateCtx(hook.ProviderClaude, "Stop", "s", false), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())
		Expect(notice).To(BeEmpty())

		quarantined, err := filepath.Glob(stateFile + ".corrupt-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(quarantined).To(HaveLen(1))

		errs, notice = applyCompletionGate(
			recovered, gateCtx(hook.ProviderClaude, "Stop", "s", true), gateBlocking(), log,
		)
		Expect(dispatcher.ShouldBlock(errs)).To(BeTrue(), "count 2 stays under the limit")
		Expect(notice).To(BeEmpty())
	})

	DescribeTable("ignores events that are not completion gates",
		func(provider hook.Provider, raw string) {
			errs, notice := applyCompletionGate(
				store, gateCtx(provider, raw, "s", true), gateBlocking(), log,
			)
			Expect(dispatcher.ShouldBlock(errs)).To(BeTrue())
			Expect(notice).To(BeEmpty())
		},
		Entry("Claude PreToolUse", hook.ProviderClaude, "PreToolUse"),
		Entry("Claude SessionEnd", hook.ProviderClaude, "SessionEnd"),
		Entry("opencode session.idle", hook.ProviderOpenCode, "session.idle"),
	)

	It("ignores a nil context", func() {
		errs, notice := applyCompletionGate(store, nil, gateBlocking(), log)
		Expect(errs).To(HaveLen(2))
		Expect(notice).To(BeEmpty())
	})

	It("names a generic gate when the event has no name", func() {
		Expect(completionReleaseNotice("", 3)).To(ContainSubstring("completion check"))
	})
})
