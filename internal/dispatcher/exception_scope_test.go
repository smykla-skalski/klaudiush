package dispatcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

type codeExceptionChecker struct {
	codes   map[string]bool
	dropped map[string]bool
}

func (c *codeExceptionChecker) CheckException(
	_ *hook.Context,
	verr *dispatcher.ValidationError,
) (*dispatcher.ValidationError, bool) {
	if c.dropped[verr.Reference.Code()] {
		return nil, false
	}

	if !verr.ShouldBlock || !c.codes[verr.Reference.Code()] {
		return verr, false
	}

	waived := *verr
	waived.ShouldBlock = false
	waived.Bypassed = true

	return &waived, true
}

func (*codeExceptionChecker) IsEnabled() bool { return true }

type combinedValidator struct{}

func (combinedValidator) Name() string { return "git.commit" }

func (combinedValidator) Category() validator.ValidatorCategory { return validator.CategoryCPU }

func (combinedValidator) Validate(_ context.Context, _ *hook.Context) *validator.Result {
	return validator.FailWithRef(validator.RefGitBadTitle, "Title too long").
		AddFinding(
			validator.Finding{Reference: validator.RefGitBadTitle, Location: "title", Repair: "a"},
			validator.Finding{
				Reference: validator.RefGitBadBody,
				Location:  "message line 3",
				Repair:    "b",
			},
			validator.Finding{Reference: validator.RefGitPRRef, Location: "message", Repair: "c"},
		)
}

var _ = Describe("exception scope", func() {
	dispatchWith := func(checker *codeExceptionChecker) []*dispatcher.ValidationError {
		reg := validator.NewRegistry()
		reg.Register(combinedValidator{}, validator.EventTypeIs(hook.EventTypePreToolUse))

		log := logger.NewNoOpLogger()
		disp := dispatcher.NewDispatcherWithOptions(
			reg, log, dispatcher.NewSequentialExecutor(log),
			dispatcher.WithExceptionChecker(checker),
		)

		return disp.Dispatch(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			Event:     hook.CanonicalEventBeforeTool,
			ToolName:  hook.ToolTypeBash,
		})
	}

	dispatch := func(codes ...string) []*dispatcher.ValidationError {
		allowed := make(map[string]bool, len(codes))
		for _, c := range codes {
			allowed[c] = true
		}

		return dispatchWith(&codeExceptionChecker{codes: allowed})
	}

	It("waives only the findings with the excepted code", func() {
		errs := dispatch("GIT004")

		Expect(errs).To(HaveLen(2))
		Expect(errs[0].Bypassed).To(BeTrue())
		Expect(errs[0].Findings).To(HaveLen(1))
		Expect(errs[1].ShouldBlock).To(BeTrue())
		Expect(errs[1].Reference).To(Equal(validator.RefGitBadBody))
		Expect(errs[1].Findings).To(HaveLen(2))
	})

	It("applies further exceptions to the remaining findings", func() {
		errs := dispatch("GIT004", "GIT005", "GIT011")

		Expect(errs).To(HaveLen(3))

		for _, e := range errs {
			Expect(e.Bypassed).To(BeTrue())
			Expect(e.Findings).To(HaveLen(1))
		}
	})

	It("leaves an error without exceptions whole", func() {
		errs := dispatch()

		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeTrue())
		Expect(errs[0].Findings).To(HaveLen(3))
	})

	It("waives a secondary code without an exception for the primary one", func() {
		errs := dispatch("GIT005")

		Expect(errs).To(HaveLen(2))
		Expect(errs[0].Bypassed).To(BeTrue())
		Expect(errs[0].Reference).To(Equal(validator.RefGitBadBody))
		Expect(errs[0].Findings).To(HaveLen(1))
		Expect(errs[1].ShouldBlock).To(BeTrue())
		Expect(errs[1].Reference).To(Equal(validator.RefGitBadTitle))
		Expect(errs[1].Message).To(Equal("Title too long"))
		Expect(errs[1].Findings).To(HaveLen(2))
		Expect(errs[1].Findings[0].Reference).To(Equal(validator.RefGitBadTitle))
		Expect(errs[1].Findings[1].Reference).To(Equal(validator.RefGitPRRef))
	})

	It("drops the findings of a code the checker removes", func() {
		errs := dispatchWith(&codeExceptionChecker{
			dropped: map[string]bool{"GIT004": true, "GIT005": true, "GIT011": true},
		})

		Expect(errs).To(BeEmpty())
	})
})
