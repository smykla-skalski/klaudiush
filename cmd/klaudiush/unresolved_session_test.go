package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// diskValidator reads the file as the tool left it and fails while it holds
// the text BAD. A file it cannot read passes without counting as checked.
type diskValidator struct{ validator.BaseValidator }

func (*diskValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	data, err := os.ReadFile(hookCtx.GetFilePath())
	if err != nil {
		return validator.Pass()
	}

	if !strings.Contains(string(data), "BAD") {
		return validator.Pass().MarkInspected()
	}

	return validator.Fail("file holds BAD").MarkInspected()
}

// inputValidator looks only at the tool input, so it cannot prove a file clean.
type inputValidator struct{ validator.BaseValidator }

func (*inputValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	if strings.Contains(hookCtx.ToolInput.Content, "SECRET") {
		return validator.Fail("secret in input")
	}

	return validator.Pass()
}

// commandValidator fails commit commands with a bad message.
type commandValidator struct{ validator.BaseValidator }

func (*commandValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	if strings.Contains(hookCtx.GetCommand(), "bad") {
		return validator.Fail("bad commit message")
	}

	return validator.Pass()
}

var _ = Describe("unresolved session findings", func() {
	const sessionID = "sess-unresolved"

	var (
		store *hooksession.Store
		disp  *dispatcher.Dispatcher
		log   logger.Logger
		dir   string
	)

	path := func(name string) string { return filepath.Join(dir, name) }

	writeFile := func(name, content string) {
		Expect(os.WriteFile(path(name), []byte(content), 0o600)).To(Succeed())
	}

	changeLater := func(name, content string) {
		writeFile(name, content)

		later := time.Now().Add(time.Minute)
		Expect(os.Chtimes(path(name), later, later)).To(Succeed())
	}

	run := func(hookCtx *hook.Context) []*dispatcher.ValidationError {
		hookCtx.Provider = hook.ProviderCodex
		hookCtx.SessionID = sessionID
		hookCtx.WorkingDir = dir

		errs, cleanup, _ := dispatchInSession(disp, store, hookCtx, log)
		cleanup()

		return errs
	}

	write := func(name, content, agentID string) []*dispatcher.ValidationError {
		writeFile(name, content)

		return run(&hook.Context{
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "PostToolUse",
			AgentID:      agentID,
			ToolName:     hook.ToolTypeWrite,
			ToolFamily:   hook.ToolFamilyWrite,
			ToolInput:    hook.ToolInput{FilePath: path(name), Content: content},
		})
	}

	bash := func(command, agentID string) []*dispatcher.ValidationError {
		return run(&hook.Context{
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "PostToolUse",
			AgentID:      agentID,
			ToolName:     hook.ToolTypeBash,
			ToolFamily:   hook.ToolFamilyShell,
			ToolInput:    hook.ToolInput{Command: command},
		})
	}

	lifecycle := func(raw, agentID string, active bool) []*dispatcher.ValidationError {
		hookCtx := gateCtx(hook.ProviderCodex, raw, sessionID, active)
		hookCtx.AgentID = agentID

		return run(hookCtx)
	}

	stop := func(active bool) []*dispatcher.ValidationError {
		return lifecycle("Stop", "", active)
	}

	BeforeEach(func() {
		var err error

		dir, err = filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())

		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(dir, "state", "state.json")),
		)
		log = logger.NewNoOpLogger()

		reg := validator.NewRegistry()
		reg.Register(
			&diskValidator{BaseValidator: *validator.NewBaseValidator("file.disk", log)},
			validator.ToolTypeIs(hook.ToolTypeWrite),
		)
		reg.Register(
			&inputValidator{BaseValidator: *validator.NewBaseValidator("file.input", log)},
			validator.ToolTypeIs(hook.ToolTypeWrite),
		)
		reg.Register(
			&commandValidator{BaseValidator: *validator.NewBaseValidator("git.commit", log)},
			validator.ToolTypeIs(hook.ToolTypeBash),
		)
		disp = dispatcher.NewDispatcher(reg, log)
	})

	It("stops reporting a file once a write repairs it", func() {
		Expect(write("a.md", "BAD", "")).To(HaveLen(1))

		errs := stop(false)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Message).To(ContainSubstring(path("a.md")))
		Expect(dispatcher.ShouldBlock(errs)).To(BeFalse())

		Expect(write("a.md", "good", "")).To(BeEmpty())
		Expect(stop(false)).To(BeEmpty())
	})

	It("resolves a command finding once the same check passes on the fix", func() {
		Expect(dispatcher.ShouldBlock(bash("git commit -m bad", ""))).To(BeTrue())

		Expect(dispatcher.ShouldBlock(stop(false))).To(BeTrue())
		Expect(dispatcher.ShouldBlock(stop(true))).To(BeTrue())

		Expect(bash("git commit --amend -m good", "")).To(BeEmpty())
		Expect(stop(true)).To(BeEmpty())
	})

	It("keeps findings unresolved after the gate releases the turn", func() {
		bash("git commit -m bad", "")

		for attempt := 1; attempt <= maxCompletionBlocks; attempt++ {
			Expect(dispatcher.ShouldBlock(stop(attempt > 1))).To(BeTrue())
		}

		Expect(dispatcher.ShouldBlock(stop(true))).To(BeFalse())
		Expect(dispatcher.ShouldBlock(stop(false))).To(BeTrue())
	})

	It("resolves only the file that was checked again", func() {
		write("a.md", "BAD", "")
		write("b.md", "BAD", "")

		write("a.md", "good", "")

		errs := stop(false)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Resource).To(Equal(hook.ResourceFilePrefix + path("b.md")))
	})

	It("does not let an input-only check clear a file finding", func() {
		Expect(write("a.md", "SECRET", "")).To(HaveLen(1))

		Expect(write("a.md", "plain", "")).To(BeEmpty())
		Expect(stop(false)).To(HaveLen(1))
	})

	It("keeps a file finding when the check could not read the file", func() {
		write("a.md", "BAD", "")

		Expect(os.Chmod(path("a.md"), 0)).To(Succeed())
		DeferCleanup(os.Chmod, path("a.md"), os.FileMode(0o600))

		Expect(run(&hook.Context{
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "PostToolUse",
			ToolName:     hook.ToolTypeWrite,
			ToolFamily:   hook.ToolFamilyWrite,
			ToolInput:    hook.ToolInput{FilePath: path("a.md"), Content: "good"},
		})).To(BeEmpty())

		Expect(stop(false)).To(HaveLen(1))
	})

	It("rechecks a file changed outside a parsed write", func() {
		write("a.md", "BAD", "")

		changeLater("a.md", "good")
		Expect(bash("sed -i '' s/BAD/good/ a.md", "")).To(BeEmpty())

		Expect(stop(false)).To(BeEmpty())
	})

	It("still reports a file changed outside a write that stays broken", func() {
		write("a.md", "BAD", "")

		changeLater("a.md", "still BAD")
		Expect(bash("true", "")).To(HaveLen(1))

		Expect(stop(false)).To(HaveLen(1))
	})

	It("drops findings about a deleted file", func() {
		write("a.md", "BAD", "")
		Expect(os.Remove(path("a.md"))).To(Succeed())

		Expect(stop(false)).To(BeEmpty())
	})

	It("keeps parent and subagent work apart", func() {
		bash("git commit -m bad", "")
		bash("git commit -m bad", "agent-1")

		Expect(dispatcher.ShouldBlock(lifecycle("SubagentStop", "agent-1", false))).To(BeTrue())
		Expect(lifecycle("SubagentStop", "agent-2", false)).To(BeEmpty())
		Expect(lifecycle("SubagentStop", "", false)).To(BeEmpty())

		lifecycle("SessionEnd", "agent-1", false)
		lifecycle("SessionStart", "agent-1", false)
		lifecycle("SessionStart", "", false)

		Expect(stop(false)).To(HaveLen(1))
		Expect(lifecycle("SubagentStop", "agent-1", false)).To(HaveLen(1))

		lifecycle("SessionEnd", "", false)
		Expect(stop(false)).To(BeEmpty())
	})

	It("keeps going when the store cannot be read", func() {
		stateFile := filepath.Join(dir, "broken.json")
		Expect(os.WriteFile(stateFile, []byte("{"), 0o600)).To(Succeed())

		store = hooksession.NewStore(hooksession.WithStateFile(stateFile))

		hookCtx := &hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventAfterTool,
			SessionID:    sessionID,
			RawEventName: "PostToolUse",
		}
		prepareRecheck(store, hookCtx, log)
		Expect(hookCtx.RecheckFiles).To(BeEmpty())

		Expect(stop(false)).To(BeEmpty())

		original := []*dispatcher.ValidationError{{Validator: "v", Message: "m"}}
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
