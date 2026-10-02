package hookresponse_test

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// fakeToken matches the GitHub PAT pattern; it is not a real credential.
var fakeToken = "ghp_" + strings.Repeat("A1b2", 9)

func agentText(resp any) string {
	fields := responseFields(resp)

	var b strings.Builder

	for _, key := range []string{"reason", "stopReason"} {
		if v, ok := fields[key].(string); ok {
			b.WriteString(v + "\n")
		}
	}

	if out, ok := fields["hookSpecificOutput"].(map[string]any); ok {
		for _, key := range []string{"permissionDecisionReason", "additionalContext"} {
			if v, ok := out[key].(string); ok {
				b.WriteString(v + "\n")
			}
		}

		if decision, ok := out["decision"].(map[string]any); ok {
			if v, ok := decision["message"].(string); ok {
				b.WriteString(v + "\n")
			}
		}
	}

	return b.String()
}

func systemText(resp any) string {
	v, _ := responseFields(resp)["systemMessage"].(string)

	return v
}

func commitErrors(cfg *config.CommitValidatorConfig, message string) []*dispatcher.ValidationError {
	fakeGit := gitpkg.NewFakeRunner()
	fakeGit.StagedFiles = []string{"file.txt"}

	v := git.NewCommitValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)
	result := v.Validate(context.Background(), &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: "git commit -sS -m '" + message + "'"},
	})
	Expect(result.Passed).To(BeFalse())

	return []*dispatcher.ValidationError{{
		Validator:   v.Name(),
		Message:     result.Message,
		Details:     result.Details,
		ShouldBlock: result.ShouldBlock,
		Reference:   result.Reference,
		FixHint:     result.FixHint,
		Findings:    result.Findings,
	}}
}

var _ = Describe("structured findings", func() {
	combined := "add a really long commit title that is far past any limit\n\n" +
		strings.Repeat("long ", 18) + "\n" + strings.Repeat("more ", 18) +
		"\n\nSee #42"

	It("gives the agent every violation and repair of a combined commit failure", func() {
		errs := commitErrors(nil, combined)
		Expect(len(errs[0].Findings)).To(BeNumerically(">=", 5))

		for _, provider := range []hook.Provider{
			hook.ProviderClaude, hook.ProviderCodex, hook.ProviderGemini, hook.ProviderOpenCode,
		} {
			text := agentText(hookresponse.BuildForContext(preToolCtx(provider), errs, nil))

			for _, f := range errs[0].Findings {
				Expect(text).To(ContainSubstring(f.Repair), "provider=%s", provider)
			}

			Expect(text).To(ContainSubstring("[GIT004]"))
			Expect(text).To(ContainSubstring("[GIT005]"))
			Expect(text).To(ContainSubstring("[GIT011]"))
		}
	})

	It("lists blocking findings in context when the event has no reason channel", func() {
		errs := commitErrors(nil, combined)

		for _, ctx := range []*hook.Context{
			eventCtx(hook.ProviderClaude, "SessionStart"),
			{
				Provider: hook.ProviderCodex, Event: hook.CanonicalEventAfterTool,
				RawEventName: "PostToolUse", ToolExecuted: true, ToolSucceeded: true,
			},
		} {
			text := agentText(hookresponse.BuildForContext(ctx, errs, nil))
			Expect(text).To(ContainSubstring("Findings:"), "event=%s", ctx.RawEventName)

			for _, f := range errs[0].Findings {
				Expect(text).To(ContainSubstring(f.Repair), "event=%s", ctx.RawEventName)
			}
		}
	})

	It("quotes nondefault title and body limits in agent and human guidance", func() {
		titleMax, bodyMax, tolerance := 64, 60, 0
		cfg := &config.CommitValidatorConfig{Message: &config.CommitMessageConfig{
			TitleMaxLength:    &titleMax,
			BodyMaxLineLength: &bodyMax,
			BodyLineTolerance: &tolerance,
		}}
		errs := commitErrors(
			cfg,
			"feat(api): "+strings.Repeat("t", 60)+"\n\n"+strings.Repeat("b", 70),
		)

		resp := hookresponse.BuildForContext(preToolCtx(hook.ProviderClaude), errs, nil)
		text := agentText(resp)

		Expect(text).To(ContainSubstring("at most 64 characters"))
		Expect(text).To(ContainSubstring("Wrap line 3 at 60 characters"))
		Expect(text).NotTo(ContainSubstring("50 char"))
		Expect(text).NotTo(ContainSubstring("72 char"))
		Expect(systemText(resp)).To(ContainSubstring("Wrap line 3 at 60 characters"))
	})

	It("never echoes a secret value in agent or human diagnostics", func() {
		errs := commitErrors(nil, "feat(api): use "+fakeToken+" for the client right now please")

		resp := hookresponse.BuildForContext(preToolCtx(hook.ProviderClaude), errs, nil)

		Expect(agentText(resp)).NotTo(ContainSubstring(fakeToken))
		Expect(systemText(resp)).NotTo(ContainSubstring(fakeToken))
		Expect(agentText(resp)).To(ContainSubstring("[REDACTED]"))
	})

	Describe("output budgets", func() {
		manyFindings := func(n int, actual string) []*dispatcher.ValidationError {
			findings := make([]validator.Finding, 0, n)
			for i := range n {
				findings = append(findings, validator.Finding{
					Reference: validator.RefGitBadBody,
					Location:  fmt.Sprintf("message line %d", i+3),
					Message:   "Body line is too long",
					Actual:    actual,
					Required:  "at most 72 characters per body line",
					Repair:    fmt.Sprintf("Wrap line %d at 72 characters", i+3),
				})
			}

			return []*dispatcher.ValidationError{{
				Validator:   "validate-git-commit",
				Message:     "Commit message validation failed",
				ShouldBlock: true,
				Reference:   validator.RefGitBadBody,
				Findings:    findings,
			}}
		}

		It("trims supplementary detail before any repair", func() {
			errs := manyFindings(60, strings.Repeat("żółć ", 60))

			for _, provider := range []hook.Provider{hook.ProviderClaude, hook.ProviderCodex} {
				resp := hookresponse.BuildForContext(preToolCtx(provider), errs, nil)
				text := agentText(resp)

				Expect(utf8.ValidString(text)).To(BeTrue())
				Expect(len(text)).To(BeNumerically("<", 10000))

				for _, f := range errs[0].Findings {
					Expect(text).To(ContainSubstring(f.Repair), "provider=%s", provider)
				}
			}
		})

		It("cuts at a rune boundary when even repairs do not fit", func() {
			errs := manyFindings(2000, "x")

			resp := hookresponse.BuildForContext(preToolCtx(hook.ProviderCodex), errs, nil)
			fields := hookSpecific(responseFields(resp))
			reason, ok := fields["permissionDecisionReason"].(string)
			Expect(ok).To(BeTrue())

			Expect(utf8.ValidString(reason)).To(BeTrue())
			Expect(len(reason)).To(BeNumerically("<=", 6000))
			Expect(reason).To(HaveSuffix("output truncated to fit the hook limit]"))
		})

		It("keeps the human message within the cap with valid UTF-8", func() {
			errs := manyFindings(400, strings.Repeat("ąę", 100))
			msg := hookresponse.FormatSystemMessage(errs)

			Expect(utf8.ValidString(msg)).To(BeTrue())
			Expect(len(msg)).To(BeNumerically("<=", 9000))
		})

		It("replaces invalid UTF-8 from validators", func() {
			errs := []*dispatcher.ValidationError{{
				Message: "bad \xff\xfe byte", ShouldBlock: true,
			}}

			resp := hookresponse.BuildForContext(preToolCtx(hook.ProviderClaude), errs, nil)
			Expect(utf8.ValidString(agentText(resp))).To(BeTrue())
			Expect(utf8.ValidString(systemText(resp))).To(BeTrue())
		})
	})

	Describe("outcomes", func() {
		blocked := &dispatcher.ValidationError{
			Message: "Missing -s flag", ShouldBlock: true, Reference: validator.RefGitNoSignoff,
		}
		accepted := &dispatcher.ValidationError{
			Message: "push blocked [BYPASSED: hotfix]", Reference: validator.RefGitKongOrgPush,
			Bypassed: true, BypassReason: "hotfix",
		}
		unavailable := &dispatcher.ValidationError{
			Message: "Plugin error: timeout", ShouldBlock: true, Unavailable: true,
		}
		unchecked := &dispatcher.ValidationError{
			Message: "linter crashed", Unavailable: true,
		}

		It("labels each outcome for the user", func() {
			msg := hookresponse.FormatSystemMessage(
				[]*dispatcher.ValidationError{blocked, accepted, unavailable},
			)

			Expect(msg).To(ContainSubstring("Blocked GIT001: Missing -s flag"))
			Expect(msg).To(ContainSubstring("Exception accepted GIT022"))
			Expect(msg).To(ContainSubstring("Validation unavailable: Plugin error"))
		})

		It("labels findings after a tool as repairs for the user", func() {
			ctx := &hook.Context{
				Provider: hook.ProviderClaude, Event: hook.CanonicalEventAfterTool,
				RawEventName: "PostToolUse", ToolExecuted: true, ToolSucceeded: true,
			}
			resp := hookresponse.BuildForContext(ctx, []*dispatcher.ValidationError{blocked}, nil)

			Expect(systemText(resp)).To(ContainSubstring("Repair required GIT001"))
			Expect(systemText(resp)).NotTo(ContainSubstring("Blocked GIT001"))
		})

		It("tells the agent a blocking check could not run", func() {
			resp := hookresponse.BuildForContext(
				preToolCtx(hook.ProviderClaude), []*dispatcher.ValidationError{unavailable}, nil,
			)

			Expect(agentText(resp)).To(ContainSubstring("Validation unavailable: Plugin error"))
		})

		It("tells the agent an advisory check did not run", func() {
			resp := hookresponse.BuildForContext(
				preToolCtx(hook.ProviderClaude), []*dispatcher.ValidationError{unchecked}, nil,
			)

			Expect(agentText(resp)).To(ContainSubstring("could not validate this action"))
			Expect(agentText(resp)).NotTo(ContainSubstring("klaudiush warning"))
		})

		It("gives warnings their repair", func() {
			warning := &dispatcher.ValidationError{
				Message: "line too long", Reference: validator.RefMarkdownLint,
				FixHint: "Fix the formatting issue and retry",
			}
			resp := hookresponse.BuildForContext(
				preToolCtx(hook.ProviderClaude), []*dispatcher.ValidationError{warning}, nil,
			)

			Expect(agentText(resp)).To(ContainSubstring(
				"klaudiush warning: Not blocking. [FILE005] line too long. Fix the formatting"))
		})
	})

	It("lists lines a summary leaves out when there are no structured findings", func() {
		errs := []*dispatcher.ValidationError{
			{
				Message:     "Shellcheck found issues:\nLine 1: SC2148 add a shebang\nLine 3: SC2086 quote it",
				ShouldBlock: true,
				Reference:   validator.RefShellcheck,
			},
		}

		resp := hookresponse.BuildForContext(preToolCtx(hook.ProviderClaude), errs, nil)
		text := agentText(resp)

		Expect(text).To(ContainSubstring("SC2148"))
		Expect(text).To(ContainSubstring("SC2086"))
	})
})
