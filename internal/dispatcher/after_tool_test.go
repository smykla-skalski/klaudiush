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
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// homeResolver answers HOME from a fixed value and everything else from the
// running system.
type homeResolver struct {
	parser.OSResolver

	home string
}

func (r *homeResolver) LookupEnv(name string) (string, bool) {
	if name == "HOME" {
		return r.home, r.home != ""
	}

	return r.OSResolver.LookupEnv(name)
}

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

// resultChecking marks a recording validator as one that reads the whole
// file after the tool ran.
type resultChecking struct{ *recordingValidator }

func (resultChecking) ChecksToolResult() bool { return true }

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

	It("rechecks unchanged content of a file with unresolved findings", func() {
		writeFile("new.go", "package main\n")

		hookCtx := claudeBash(hook.CanonicalEventAfterTool, heredoc(""))
		hookCtx.RecheckFiles = []string{repo + "/new.go"}

		reg := validator.NewRegistry()
		reg.Register(resultChecking{rec}, validator.ToolTypeIs(hook.ToolTypeWrite))
		reg.Register(&recordingValidator{}, validator.ToolTypeIs(hook.ToolTypeWrite))

		outcome := dispatcher.NewDispatcher(reg, logger.NewNoOpLogger()).
			DispatchWithChecks(context.Background(), hookCtx)

		Expect(rec.paths()).To(ConsistOf(repo + "/new.go"))
		Expect(outcome.Checks).To(ConsistOf(dispatcher.Check{
			Validator: "recording",
			Resource:  hook.ResourceFilePrefix + repo + "/new.go",
		}))
		Expect(outcome.Errors).To(HaveLen(2))
		Expect(outcome.Errors[0].Resource).To(Equal(hook.ResourceFilePrefix + repo + "/new.go"))
	})

	It("reports no checks for a cancelled dispatch", func() {
		reg := validator.NewRegistry()
		reg.Register(rec, validator.ToolTypeIs(hook.ToolTypeWrite))

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()

		outcome := dispatcher.NewDispatcher(reg, logger.NewNoOpLogger()).
			DispatchWithChecks(cancelled, &hook.Context{
				Provider:  hook.ProviderCodex,
				Event:     hook.CanonicalEventAfterTool,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{FilePath: repo + "/a.md"},
			})

		Expect(outcome.Checks).To(BeEmpty())
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

	DescribeTable("resolves ~ to the home directory, not one under the hook's",
		func(command string, succeeded bool, want string) {
			home := filepath.Join(repo, "home")

			reg := validator.NewRegistry()
			reg.Register(rec, validator.ToolTypeIs(hook.ToolTypeWrite))

			hookCtx := claudeBash(hook.CanonicalEventAfterTool, command)
			hookCtx.ToolSucceeded = succeeded

			dispatcher.NewDispatcherWithOptions(
				reg,
				logger.NewNoOpLogger(),
				dispatcher.NewSequentialExecutor(logger.NewNoOpLogger()),
				dispatcher.WithPathResolver(&homeResolver{home: home}),
			).Dispatch(context.Background(), hookCtx)

			Expect(rec.paths()).To(ConsistOf(filepath.Join(home, want)))
		},
		Entry("cd ~/dir", "cd ~/proj && echo x > out.txt", true, "proj/out.txt"),
		Entry("cd ~/dir after a failed command", "cd ~/proj && echo x > out.txt", false,
			"proj/out.txt"),
		Entry("bare cd", "cd && echo x > out.txt", true, "out.txt"),
		Entry("~ target", "cd sub && echo x > ~/out.txt", true, "out.txt"),
	)

	It("leaves a ~ target unjoined when HOME is unknown", func() {
		reg := validator.NewRegistry()
		reg.Register(rec, validator.ToolTypeIs(hook.ToolTypeWrite))

		dispatcher.NewDispatcherWithOptions(
			reg,
			logger.NewNoOpLogger(),
			dispatcher.NewSequentialExecutor(logger.NewNoOpLogger()),
			dispatcher.WithPathResolver(&homeResolver{}),
		).Dispatch(context.Background(), claudeBash(
			hook.CanonicalEventAfterTool,
			"cd ~/proj && echo x > out.txt && echo y > ~/b.txt",
		))

		Expect(rec.paths()).To(ConsistOf("out.txt", "~/b.txt"))
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

var _ = Describe("Dispatcher file tools after they ran", func() {
	var rec *recordingValidator

	dispatch := func(hookCtx *hook.Context) []*dispatcher.ValidationError {
		reg := validator.NewRegistry()
		reg.Register(rec, validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit))

		return dispatcher.NewDispatcher(reg, logger.NewNoOpLogger()).
			Dispatch(context.Background(), hookCtx)
	}

	write := func(provider hook.Provider, event hook.CanonicalEvent, succeeded bool) *hook.Context {
		return &hook.Context{
			Provider:      provider,
			Event:         event,
			ToolName:      hook.ToolTypeWrite,
			ToolExecuted:  event == hook.CanonicalEventAfterTool,
			ToolSucceeded: succeeded,
			ToolInput:     hook.ToolInput{FilePath: "/repo/a.go"},
		}
	}

	BeforeEach(func() {
		rec = &recordingValidator{block: true}
	})

	DescribeTable("reports findings as advisory and names the file",
		func(provider hook.Provider, succeeded bool) {
			errs := dispatch(write(provider, hook.CanonicalEventAfterTool, succeeded))

			Expect(errs).To(HaveLen(1))
			Expect(errs[0].ShouldBlock).To(BeFalse())
			Expect(errs[0].Message).To(Equal("/repo/a.go: finding in /repo/a.go"))
		},
		Entry("failed Claude Write", hook.ProviderClaude, false),
		Entry("successful Codex Write", hook.ProviderCodex, true),
		Entry("successful Gemini Write", hook.ProviderGemini, true),
		Entry("successful opencode Write", hook.ProviderOpenCode, true),
	)

	It("keeps findings before the tool blocking and unnamed", func() {
		errs := dispatch(write(hook.ProviderCodex, hook.CanonicalEventBeforeTool, false))

		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeTrue())
		Expect(errs[0].Message).To(Equal("finding in /repo/a.go"))
	})

	It("reports each file of an applied patch as advisory and named", func() {
		errs := dispatch(&hook.Context{
			Provider:      hook.ProviderCodex,
			Event:         hook.CanonicalEventAfterTool,
			RawToolName:   "apply_patch",
			ToolName:      hook.ToolTypeEdit,
			ToolFamily:    hook.ToolFamilyEdit,
			ToolExecuted:  true,
			ToolSucceeded: true,
			PatchFiles: []hook.PatchFile{
				{
					ToolName:   hook.ToolTypeWrite,
					ToolFamily: hook.ToolFamilyWrite,
					Input:      hook.ToolInput{FilePath: "/repo/a.go"},
				},
				{
					ToolName:   hook.ToolTypeEdit,
					ToolFamily: hook.ToolFamilyEdit,
					Input:      hook.ToolInput{FilePath: "/repo/b.go"},
				},
			},
		})

		Expect(errs).To(HaveLen(2))

		for _, e := range errs {
			Expect(e.ShouldBlock).To(BeFalse())
		}

		Expect(errs[0].Message).To(Equal("/repo/a.go: finding in /repo/a.go"))
		Expect(errs[1].Message).To(Equal("/repo/b.go: finding in /repo/b.go"))
	})
})
