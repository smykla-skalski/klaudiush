package main

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("unresolved session findings", func() {
	const sessionID = "sess-unresolved"

	var (
		store *hooksession.Store
		log   logger.Logger
		dir   string
	)

	afterWrite := func(path, agentID string) *hook.Context {
		return &hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "PostToolUse",
			SessionID:    sessionID,
			AgentID:      agentID,
			WorkingDir:   dir,
			ToolName:     hook.ToolTypeWrite,
			ToolFamily:   hook.ToolFamilyWrite,
			ToolInput:    hook.ToolInput{FilePath: path},
		}
	}

	failure := func(hookCtx *hook.Context) []*dispatcher.ValidationError {
		return []*dispatcher.ValidationError{{
			Validator:   "file.markdown",
			Message:     hookCtx.GetFilePath() + ": bad heading",
			ShouldBlock: true,
			Resource:    hookCtx.Resource(),
		}}
	}

	check := func(hookCtx *hook.Context) []dispatcher.Check {
		return []dispatcher.Check{{Validator: "file.markdown", Resource: hookCtx.Resource()}}
	}

	record := func(
		hookCtx *hook.Context,
		errs []*dispatcher.ValidationError,
		checks []dispatcher.Check,
	) {
		_, cleanup := applyHookSessionLifecycle(store, hookCtx, errs, checks, log)
		cleanup()
	}

	stop := func(active bool) []*dispatcher.ValidationError {
		hookCtx := gateCtx(hook.ProviderCodex, "Stop", sessionID, active)
		errs, cleanup := applyHookSessionLifecycle(store, hookCtx, nil, nil, log)
		errs, _ = applyCompletionGate(store, hookCtx, errs, log)

		cleanup()

		return errs
	}

	subagentStop := func(agentID string) []*dispatcher.ValidationError {
		hookCtx := gateCtx(hook.ProviderCodex, "SubagentStop", sessionID, false)
		hookCtx.AgentID = agentID
		errs, cleanup := applyHookSessionLifecycle(store, hookCtx, nil, nil, log)

		cleanup()

		return errs
	}

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(dir, "state", "state.json")),
		)
		log = logger.NewNoOpLogger()
	})

	It("stops blocking completion once the file is repaired", func() {
		broken := afterWrite("README.md", "")
		record(broken, failure(broken), check(broken))
		Expect(dispatcher.ShouldBlock(stop(false))).To(BeTrue())

		repaired := afterWrite(filepath.Join(dir, "README.md"), "")
		record(repaired, nil, check(repaired))

		Expect(stop(false)).To(BeEmpty())
	})

	It("keeps an unresolved finding through a denied Stop", func() {
		broken := afterWrite("README.md", "")
		record(broken, failure(broken), check(broken))

		Expect(dispatcher.ShouldBlock(stop(false))).To(BeTrue())
		Expect(dispatcher.ShouldBlock(stop(true))).To(BeTrue())

		combined, err := store.CombinedErrors(hook.ProviderCodex, sessionID)
		Expect(err).NotTo(HaveOccurred())
		Expect(combined).To(HaveLen(1))
	})

	It("keeps findings unresolved after the gate releases the turn", func() {
		broken := afterWrite("README.md", "")
		record(broken, failure(broken), check(broken))

		for attempt := 1; attempt <= maxCompletionBlocks; attempt++ {
			Expect(dispatcher.ShouldBlock(stop(attempt > 1))).To(BeTrue())
		}

		Expect(dispatcher.ShouldBlock(stop(true))).To(BeFalse())
		Expect(dispatcher.ShouldBlock(stop(false))).To(BeTrue())
	})

	It("resolves only the resource that was checked again", func() {
		first := afterWrite("a.md", "")
		second := afterWrite("b.md", "")

		record(first, failure(first), check(first))
		record(second, failure(second), check(second))

		record(first, nil, check(first))

		errs := stop(false)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Resource).To(Equal(second.Resource()))
	})

	It("does not resolve a finding when a different validator passes", func() {
		broken := afterWrite("README.md", "")
		record(broken, failure(broken), check(broken))

		record(broken, nil, []dispatcher.Check{
			{Validator: "file.shellscript", Resource: broken.Resource()},
		})

		Expect(stop(false)).To(HaveLen(1))
	})

	It("rechecks unresolved files after a tool", func() {
		broken := afterWrite("README.md", "")
		record(broken, failure(broken), check(broken))

		next := afterWrite("other.md", "")
		prepareRecheck(store, next, log)
		Expect(next.RecheckFiles).To(ConsistOf(hook.CanonicalFilePath(dir, "README.md")))
		Expect(next.NeedsRecheck(filepath.Join(dir, "README.md"))).To(BeTrue())
		Expect(next.NeedsRecheck("other.md")).To(BeFalse())

		before := gateCtx(hook.ProviderCodex, "PreToolUse", sessionID, false)
		prepareRecheck(store, before, log)
		Expect(before.RecheckFiles).To(BeEmpty())
	})

	It("keeps parent and subagent work apart", func() {
		parent := afterWrite("parent.md", "")
		child := afterWrite("child.md", "agent-1")

		record(parent, failure(parent), check(parent))
		record(child, failure(child), check(child))

		childErrs := subagentStop("agent-1")
		Expect(childErrs).To(HaveLen(1))
		Expect(childErrs[0].Resource).To(Equal(child.Resource()))
		Expect(subagentStop("agent-2")).To(BeEmpty())
		Expect(subagentStop("")).To(BeEmpty())

		subagentEnd := gateCtx(hook.ProviderCodex, "SessionEnd", sessionID, false)
		subagentEnd.AgentID = "agent-1"
		record(subagentEnd, nil, nil)

		Expect(stop(false)).To(HaveLen(2))
		Expect(subagentStop("agent-1")).To(HaveLen(1))

		record(gateCtx(hook.ProviderCodex, "SessionStart", sessionID, false), nil, nil)
		Expect(stop(false)).To(HaveLen(2))

		record(gateCtx(hook.ProviderCodex, "SessionEnd", sessionID, false), nil, nil)
		Expect(stop(false)).To(BeEmpty())
	})

	It("keeps going when the store cannot be read", func() {
		stateFile := filepath.Join(dir, "broken.json")
		Expect(os.WriteFile(stateFile, []byte("{"), 0o600)).To(Succeed())

		store = hooksession.NewStore(hooksession.WithStateFile(stateFile))

		hookCtx := afterWrite("README.md", "")
		prepareRecheck(store, hookCtx, log)
		Expect(hookCtx.RecheckFiles).To(BeEmpty())

		original := failure(hookCtx)

		Expect(stop(false)).To(BeEmpty())

		subErrs, cleanup := applyHookSessionLifecycle(
			store,
			&hook.Context{
				Provider:     hook.ProviderCodex,
				Event:        hook.CanonicalEventSubagentStop,
				RawEventName: "SubagentStop",
				SessionID:    sessionID,
				AgentID:      "agent-1",
			},
			original,
			nil,
			log,
		)
		cleanup()
		Expect(subErrs).To(Equal(original))
	})
})
