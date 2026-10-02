package dispatcher_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

type seenWrite struct {
	path    string
	content string
	derived bool
}

// recordingValidator records every file context it sees and reports one
// finding per file.
type recordingValidator struct {
	mu    sync.Mutex
	block bool
	seen  []seenWrite
}

func (*recordingValidator) Name() string { return "recording" }

func (*recordingValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

func (v *recordingValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	v.mu.Lock()
	v.seen = append(v.seen, seenWrite{
		path:    hookCtx.GetFilePath(),
		content: hookCtx.ToolInput.Content,
		derived: hookCtx.Derived,
	})
	v.mu.Unlock()

	return &validator.Result{
		Passed:      false,
		Message:     "finding in " + hookCtx.GetFilePath(),
		ShouldBlock: v.block,
	}
}

func (v *recordingValidator) paths() []string {
	v.mu.Lock()
	defer v.mu.Unlock()

	paths := make([]string, 0, len(v.seen))
	for _, s := range v.seen {
		paths = append(paths, s.path)
	}

	return paths
}

func errorMessages(errs []*dispatcher.ValidationError) []string {
	messages := make([]string, 0, len(errs))
	for _, e := range errs {
		messages = append(messages, e.Message)
	}

	return messages
}

var _ = Describe("Dispatcher Bash file writes after the tool ran", func() {
	const heredoc = "cat > /repo/new.go <<'EOF'\npackage main\nEOF"

	heredocThen := func(next string) string {
		return "cat > /repo/new.go <<'EOF' && " + next + "\npackage main\nEOF"
	}

	var rec *recordingValidator

	dispatch := func(hookCtx *hook.Context) []*dispatcher.ValidationError {
		reg := validator.NewRegistry()
		reg.Register(rec, validator.ToolTypeIs(hook.ToolTypeWrite))

		return dispatcher.NewDispatcher(reg, logger.NewNoOpLogger()).
			Dispatch(context.Background(), hookCtx)
	}

	claudeBash := func(event hook.CanonicalEvent, command string) *hook.Context {
		after := event == hook.CanonicalEventAfterTool

		return &hook.Context{
			Provider:      hook.ProviderClaude,
			Event:         event,
			ToolName:      hook.ToolTypeBash,
			ToolFamily:    hook.ToolFamilyShell,
			WorkingDir:    "/repo",
			ToolExecuted:  after,
			ToolSucceeded: after,
			ToolInput:     hook.ToolInput{Command: command},
		}
	}

	BeforeEach(func() {
		rec = &recordingValidator{}
	})

	It("passes the parsed content before the command runs", func() {
		dispatch(claudeBash(hook.CanonicalEventBeforeTool, heredoc))

		Expect(rec.seen).To(HaveLen(1))
		Expect(rec.seen[0].content).To(Equal("package main\n"))
		Expect(rec.seen[0].derived).To(BeTrue())
	})

	It("reads every written and reported file from disk afterwards", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, heredocThen("echo x > rel.txt"))
		hookCtx.ChangedFiles = []string{"/repo/new.go", "/repo/gen/other.go"}

		dispatch(hookCtx)

		Expect(rec.paths()).To(ConsistOf("/repo/new.go", "/repo/rel.txt", "/repo/gen/other.go"))

		for _, s := range rec.seen {
			Expect(s.content).To(BeEmpty())
			Expect(s.derived).To(BeTrue())
		}
	})

	It("drops warnings pre-tool validation already showed for the same bytes", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, heredoc)
		hookCtx.ChangedFiles = []string{"/repo/gen/other.go"}

		errs := dispatch(hookCtx)

		Expect(errorMessages(errs)).To(ConsistOf("finding in /repo/gen/other.go"))
	})

	It("keeps blocking findings for content pre-tool validation saw", func() {
		rec.block = true

		errs := dispatch(claudeBash(hook.CanonicalEventAfterTool, heredoc))

		Expect(errorMessages(errs)).To(ConsistOf("finding in /repo/new.go"))
	})

	It("keeps every finding when the command failed", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, heredoc)
		hookCtx.ToolSucceeded = false

		errs := dispatch(hookCtx)

		Expect(errorMessages(errs)).To(ConsistOf("finding in /repo/new.go"))
	})

	It("keeps findings for content written twice in different ways", func() {
		errs := dispatch(claudeBash(
			hook.CanonicalEventAfterTool,
			heredocThen("echo more >> /repo/new.go"),
		))

		Expect(errorMessages(errs)).To(ConsistOf("finding in /repo/new.go"))
	})

	It("validates reported changes of a command the parser sees no writes in", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, "make generate")
		hookCtx.ChangedFiles = []string{"/repo/a.go", "/repo/./a.go"}

		dispatch(hookCtx)

		Expect(rec.paths()).To(ConsistOf("/repo/a.go"))
	})

	It("caps how many reported changes one command gets validated for", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, "make generate")
		for i := range 60 {
			hookCtx.ChangedFiles = append(hookCtx.ChangedFiles, fmt.Sprintf("/repo/f%d.go", i))
		}

		dispatch(hookCtx)

		Expect(rec.seen).To(HaveLen(50))
	})

	It("resolves relative targets against the directory a cd moved to", func() {
		dispatch(claudeBash(hook.CanonicalEventAfterTool, "cd sub && echo x > out.txt"))

		Expect(rec.paths()).To(ConsistOf("/repo/sub/out.txt"))
	})

	It("leaves a relative target as is without a working directory", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, "echo x > out.txt")
		hookCtx.WorkingDir = ""

		dispatch(hookCtx)

		Expect(rec.paths()).To(ConsistOf("out.txt"))
	})
})
