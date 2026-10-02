package dispatcher_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

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

var _ = Describe("Dispatcher Bash file writes after the tool ran", func() {
	var (
		rec  *recordingValidator
		repo string
	)

	heredoc := func(next string) string {
		cmd := "cat > " + repo + "/new.go <<'EOF'"
		if next != "" {
			cmd += " && " + next
		}

		return cmd + "\npackage main\nEOF"
	}

	writeFile := func(name, content string) {
		path := filepath.Join(repo, name)
		Expect(os.MkdirAll(filepath.Dir(path), 0o700)).To(Succeed())
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	}

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
			WorkingDir:    repo,
			ToolExecuted:  after,
			ToolSucceeded: after,
			ToolInput:     hook.ToolInput{Command: command},
		}
	}

	BeforeEach(func() {
		rec = &recordingValidator{block: true}

		dir, err := filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())

		repo = dir
	})

	It("passes the parsed content before the command runs", func() {
		errs := dispatch(claudeBash(hook.CanonicalEventBeforeTool, heredoc("")))

		Expect(rec.seen).To(HaveLen(1))
		Expect(rec.seen[0].content).To(Equal("package main\n"))
		Expect(rec.seen[0].derived).To(BeTrue())
		Expect(errs[0].ShouldBlock).To(BeTrue())
	})

	It("reads written and reported files from disk as advisory checks", func() {
		writeFile("new.go", "package main\n// rewritten\n")

		hookCtx := claudeBash(hook.CanonicalEventAfterTool, heredoc("echo x > rel.txt"))
		hookCtx.ChangedFiles = []string{repo + "/new.go", repo + "/gen/other.go"}

		errs := dispatch(hookCtx)

		Expect(rec.paths()).To(ConsistOf(repo+"/new.go", repo+"/rel.txt", repo+"/gen/other.go"))

		for _, s := range rec.seen {
			Expect(s.content).To(BeEmpty())
			Expect(s.derived).To(BeTrue())
		}

		Expect(errs).To(HaveLen(3))

		for _, e := range errs {
			Expect(e.ShouldBlock).To(BeFalse())
			Expect(e.Message).To(HavePrefix(repo + "/"))
		}
	})

	It("skips a file that still holds the bytes pre-tool validation saw", func() {
		writeFile("new.go", "package main\n")

		dispatch(claudeBash(hook.CanonicalEventAfterTool, heredoc("")))

		Expect(rec.seen).To(BeEmpty())
	})

	It("skips unchanged captured content after a failed command too", func() {
		writeFile("new.go", "package main\n")

		hookCtx := claudeBash(hook.CanonicalEventAfterTool, heredoc("false"))
		hookCtx.ToolSucceeded = false

		dispatch(hookCtx)

		Expect(rec.seen).To(BeEmpty())
	})

	It("checks a file written twice in different ways", func() {
		writeFile("new.go", "package main\n")

		dispatch(claudeBash(
			hook.CanonicalEventAfterTool,
			heredoc("echo more >> "+repo+"/new.go"),
		))

		Expect(rec.paths()).To(ConsistOf(repo + "/new.go"))
	})

	It("validates reported changes of a command the parser sees no writes in", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, "make generate")
		hookCtx.ChangedFiles = []string{repo + "/a.go", repo + "/./a.go"}

		dispatch(hookCtx)

		Expect(rec.paths()).To(ConsistOf(repo + "/a.go"))
	})

	It("caps how many reported changes one command gets validated for", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, "make generate")
		for i := range 20 {
			hookCtx.ChangedFiles = append(hookCtx.ChangedFiles, fmt.Sprintf("%s/f%d.go", repo, i))
		}

		dispatch(hookCtx)

		Expect(rec.seen).To(HaveLen(10))
	})

	It("matches a target reached through a symlink to the reported real path", func() {
		writeFile("real/out.txt", "x\n")

		link := filepath.Join(repo, "link")
		Expect(os.Symlink(filepath.Join(repo, "real"), link)).To(Succeed())

		hookCtx := claudeBash(hook.CanonicalEventAfterTool, "echo x > "+link+"/out.txt")
		hookCtx.ChangedFiles = []string{repo + "/real/out.txt"}

		dispatch(hookCtx)

		Expect(rec.paths()).To(ConsistOf(repo + "/real/out.txt"))
	})

	It("resolves relative targets against the directory a cd moved to", func() {
		dispatch(claudeBash(hook.CanonicalEventAfterTool, "cd sub && echo x > out.txt"))

		Expect(rec.paths()).To(ConsistOf(repo + "/sub/out.txt"))
	})

	It("does not read special files such as FIFOs", func() {
		fifo := filepath.Join(repo, "fifo.go")
		Expect(syscall.Mkfifo(fifo, 0o600)).To(Succeed())

		dispatch(claudeBash(
			hook.CanonicalEventAfterTool,
			"cat > "+fifo+" <<'EOF'\npackage main\nEOF",
		))

		Expect(rec.paths()).To(ConsistOf(fifo))
	})

	It("leaves a relative target as is without a working directory", func() {
		hookCtx := claudeBash(hook.CanonicalEventAfterTool, "echo x > out.txt")
		hookCtx.WorkingDir = ""

		dispatch(hookCtx)

		Expect(rec.paths()).To(ConsistOf("out.txt"))
	})
})

var _ = Describe("Dispatcher failed file tools", func() {
	It("reports findings about a failed Write as advisory", func() {
		rec := &recordingValidator{block: true}
		reg := validator.NewRegistry()
		reg.Register(rec, validator.ToolTypeIs(hook.ToolTypeWrite))

		errs := dispatcher.NewDispatcher(reg, logger.NewNoOpLogger()).
			Dispatch(context.Background(), &hook.Context{
				Provider:     hook.ProviderClaude,
				Event:        hook.CanonicalEventAfterTool,
				ToolName:     hook.ToolTypeWrite,
				ToolExecuted: true,
				ToolInput:    hook.ToolInput{FilePath: "/repo/a.go"},
			})

		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeFalse())
	})
})
