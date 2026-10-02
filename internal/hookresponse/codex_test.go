package hookresponse_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func codexFields(event hook.CanonicalEvent, errs []*dispatcher.ValidationError) map[string]any {
	resp := hookresponse.BuildForContext(&hook.Context{
		Provider: hook.ProviderCodex,
		Event:    event,
	}, errs, nil)

	_, ok := resp.(*hookresponse.CodexCommandResponse)
	Expect(ok).To(BeTrue())

	data, err := json.Marshal(resp)
	Expect(err).NotTo(HaveOccurred())

	var fields map[string]any
	Expect(json.Unmarshal(data, &fields)).To(Succeed())

	return fields
}

func codexBlocking() []*dispatcher.ValidationError {
	return []*dispatcher.ValidationError{{
		Validator:   "git.commit",
		Message:     "missing signoff",
		ShouldBlock: true,
		Reference:   validator.RefGitNoSignoff,
	}}
}

func codexWarning() []*dispatcher.ValidationError {
	return []*dispatcher.ValidationError{{
		Validator: "markdown",
		Message:   "heading style",
	}}
}

var _ = Describe("BuildCodex", func() {
	lifecycleFields := []string{"continue", "stopReason", "suppressOutput"}

	It("returns nothing without findings", func() {
		Expect(hookresponse.BuildCodex(&hook.Context{
			Provider: hook.ProviderCodex,
			Event:    hook.CanonicalEventBeforeTool,
		}, nil, nil)).To(BeNil())
	})

	It("denies PreToolUse through hookSpecificOutput only", func() {
		fields := codexFields(hook.CanonicalEventBeforeTool, codexBlocking())

		for _, key := range lifecycleFields {
			Expect(fields).NotTo(HaveKey(key))
		}

		Expect(fields).NotTo(HaveKey("decision"))
		Expect(fields).To(HaveKey("systemMessage"))

		out := fields["hookSpecificOutput"].(map[string]any)
		Expect(out).To(HaveKeyWithValue("hookEventName", "PreToolUse"))
		Expect(out).To(HaveKeyWithValue("permissionDecision", "deny"))
		Expect(out["permissionDecisionReason"]).To(ContainSubstring("[GIT001]"))
		Expect(out["additionalContext"]).To(ContainSubstring("Fix ALL"))
	})

	It("keeps PreToolUse warnings advisory without a permission decision", func() {
		fields := codexFields(hook.CanonicalEventBeforeTool, codexWarning())

		for _, key := range lifecycleFields {
			Expect(fields).NotTo(HaveKey(key))
		}

		out := fields["hookSpecificOutput"].(map[string]any)
		Expect(out).NotTo(HaveKey("permissionDecision"))
		Expect(out).NotTo(HaveKey("permissionDecisionReason"))
		Expect(out["additionalContext"]).To(ContainSubstring("warning"))
	})

	It("stops SessionStart with continue=false and stopReason", func() {
		fields := codexFields(hook.CanonicalEventSessionStart, codexBlocking())

		Expect(fields).To(HaveKeyWithValue("continue", false))
		Expect(fields["stopReason"]).To(ContainSubstring("[GIT001]"))
		Expect(fields).NotTo(HaveKey("hookSpecificOutput"))
	})

	It("omits continue on advisory SessionStart responses", func() {
		fields := codexFields(hook.CanonicalEventSessionStart, codexWarning())

		Expect(fields).NotTo(HaveKey("continue"))
		Expect(fields["hookSpecificOutput"]).To(HaveKeyWithValue("hookEventName", "SessionStart"))
	})

	It("blocks Stop with decision and reason, without additionalContext", func() {
		fields := codexFields(hook.CanonicalEventTurnStop, codexBlocking())

		Expect(fields).To(HaveKeyWithValue("decision", "block"))
		Expect(fields["reason"]).To(ContainSubstring("[GIT001]"))
		Expect(fields).NotTo(HaveKey("continue"))
		Expect(fields).NotTo(HaveKey("hookSpecificOutput"))
	})

	It("drops additionalContext Stop does not accept", func() {
		fields := codexFields(hook.CanonicalEventTurnStop, codexWarning())

		Expect(fields).NotTo(HaveKey("hookSpecificOutput"))
		Expect(fields).To(HaveKey("systemMessage"))
	})

	It("keeps PostToolUse advisory even for blocking findings", func() {
		fields := codexFields(hook.CanonicalEventAfterTool, codexBlocking())

		Expect(fields).NotTo(HaveKey("decision"))
		Expect(fields).NotTo(HaveKey("continue"))

		out := fields["hookSpecificOutput"].(map[string]any)
		Expect(out).To(HaveKeyWithValue("hookEventName", "PostToolUse"))
		Expect(out).NotTo(HaveKey("permissionDecision"))
	})

	It("blocks UserPromptSubmit with decision rather than continue", func() {
		fields := codexFields(hook.CanonicalEventUserPromptSubmit, codexBlocking())

		Expect(fields).To(HaveKeyWithValue("decision", "block"))
		Expect(fields).NotTo(HaveKey("continue"))
		Expect(fields).NotTo(HaveKey("stopReason"))
	})

	It("emits only systemMessage for events Codex does not document", func() {
		fields := codexFields(hook.CanonicalEventNotification, codexBlocking())

		Expect(fields).To(HaveLen(1))
		Expect(fields).To(HaveKey("systemMessage"))
	})

	It("emits only systemMessage for Codex events aliased onto another contract", func() {
		resp := hookresponse.BuildForContext(&hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventSessionStart,
			RawEventName: "SubagentStart",
		}, codexBlocking(), nil)

		data, err := json.Marshal(resp)
		Expect(err).NotTo(HaveOccurred())

		var fields map[string]any
		Expect(json.Unmarshal(data, &fields)).To(Succeed())
		Expect(fields).To(HaveLen(1))
		Expect(fields).To(HaveKey("systemMessage"))
	})
})
