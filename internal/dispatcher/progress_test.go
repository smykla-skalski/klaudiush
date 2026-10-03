package dispatcher_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// published keeps the latest findings a dispatcher published.
type published struct {
	mu   sync.Mutex
	errs []*dispatcher.ValidationError
	n    int
}

func (p *published) store(errs []*dispatcher.ValidationError) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.errs = errs
	p.n++
}

func (p *published) latest() []*dispatcher.ValidationError {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.errs
}

func (p *published) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.n
}

// runOnly hides the observing side of an executor.
type runOnly struct{ dispatcher.Executor }

// hanging blocks until release closes, ignoring its context.
func hanging(name string, release <-chan struct{}) *funcValidator {
	return &funcValidator{name: name, fn: func(context.Context) *validator.Result {
		<-release

		return validator.Pass()
	}}
}

var _ = Describe("Progress publishing", func() {
	var (
		log     logger.Logger
		hookCtx *hook.Context
		found   *published
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		found = &published{}
		hookCtx = &hook.Context{
			Event:     hook.CanonicalEventBeforeTool,
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: "ls"},
		}
	})

	dispatcherFor := func(executor dispatcher.Executor, vs ...validator.Validator) *dispatcher.Dispatcher {
		reg := validator.NewRegistry()
		for _, v := range vs {
			reg.Register(v, validator.EventTypeIs(hook.EventTypePreToolUse))
		}

		return dispatcher.NewDispatcherWithOptions(reg, log, executor,
			dispatcher.WithProgress(found.store))
	}

	executors := map[string]func() dispatcher.Executor{
		"sequential": func() dispatcher.Executor { return dispatcher.NewSequentialExecutor(log) },
		"parallel": func() dispatcher.Executor {
			return dispatcher.NewParallelExecutor(log, nil)
		},
	}

	for name, newExecutor := range executors {
		Context(name+" executor", func() {
			It("publishes a deny while a later validator never returns", func() {
				release := make(chan struct{})
				finished := make(chan struct{})

				disp := dispatcherFor(newExecutor(),
					returning("validate-deny", validator.Fail("bad command")),
					hanging("validate-stuck", release),
				)

				go func() {
					defer close(finished)

					disp.Dispatch(context.Background(), hookCtx)
				}()

				Eventually(found.latest).Should(ConsistOf(
					HaveField("Message", "bad command"),
				))
				Expect(found.latest()[0].ShouldBlock).To(BeTrue())
				Expect(found.latest()[0].Resource).To(Equal(hookCtx.Resource()))
				Consistently(finished).ShouldNot(BeClosed())

				close(release)
				Eventually(finished).Should(BeClosed())
			})

			It("applies the failure policy to what it publishes", func() {
				disp := dispatcherFor(newExecutor(),
					returning("validate-missing", validator.Unavailable(
						validator.ReasonMissingTool, "tool is not installed",
					)),
					returning("validate-warn", validator.Warn("heads up")),
				)

				errs := disp.Dispatch(context.Background(), hookCtx)

				Expect(errs).To(ConsistOf(HaveField("Message", "heads up")))
				Expect(found.latest()).To(ConsistOf(HaveField("Message", "heads up")))
			})
		})
	}

	It("publishes the final findings when the executor cannot report progress", func() {
		disp := dispatcherFor(runOnly{dispatcher.NewSequentialExecutor(log)},
			returning("validate-deny", validator.Fail("bad command")),
			returning("validate-ok", validator.Pass()),
		)

		errs := disp.Dispatch(context.Background(), hookCtx)

		Expect(found.count()).To(Equal(1))
		Expect(found.latest()).To(Equal(errs))
	})

	It("publishes findings about a file a tool changed as advisory", func() {
		after := &hook.Context{
			Event:     hook.CanonicalEventAfterTool,
			EventType: hook.EventTypePostToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{FilePath: "/tmp/x.sh", Content: "echo"},
		}

		reg := validator.NewRegistry()
		reg.Register(
			returning("validate-deny", validator.Fail("bad script")),
			validator.EventTypeIs(hook.EventTypePostToolUse),
		)

		disp := dispatcher.NewDispatcherWithOptions(reg, log,
			dispatcher.NewSequentialExecutor(log), dispatcher.WithProgress(found.store))

		disp.Dispatch(context.Background(), after)

		Expect(found.latest()).To(ConsistOf(SatisfyAll(
			HaveField("Message", "/tmp/x.sh: bad script"),
			HaveField("ShouldBlock", false),
		)))
	})

	It("does not publish without a publisher", func() {
		reg := validator.NewRegistry()
		reg.Register(
			returning("validate-deny", validator.Fail("bad command")),
			validator.EventTypeIs(hook.EventTypePreToolUse),
		)

		disp := dispatcher.NewDispatcherWithOptions(reg, log, dispatcher.NewSequentialExecutor(log))

		Expect(disp.Dispatch(context.Background(), hookCtx)).
			To(ConsistOf(HaveField("ShouldBlock", true)))
	})
})
