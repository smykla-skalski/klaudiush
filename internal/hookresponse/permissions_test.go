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

func permissionBypassed() []*dispatcher.ValidationError {
	return []*dispatcher.ValidationError{{
		Validator:    "git.push",
		Message:      "cannot push to protected branch [BYPASSED: Emergency hotfix]",
		Reference:    validator.RefGitKongOrgPush,
		Bypassed:     true,
		BypassReason: "Emergency hotfix",
	}}
}

func permissionMixedAdvisory() []*dispatcher.ValidationError {
	return append(semanticsWarning(), permissionBypassed()...)
}

func preToolCtx(provider hook.Provider) *hook.Context {
	return &hook.Context{
		Provider: provider,
		Event:    hook.CanonicalEventBeforeTool,
		RawEventName: hook.DisplayEventName(
			provider,
			hook.CanonicalEventBeforeTool,
			hook.EventTypeUnknown,
		),
	}
}

// approvalValues lists every spelling a supported harness reads as approval.
var approvalValues = []string{"allow", "approve"}

// collectApprovals walks a marshalled response and returns each field whose
// value approves the action.
func collectApprovals(node any, path string, found *[]string) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			collectApprovals(child, path+"."+key, found)
		}
	case []any:
		for _, child := range v {
			collectApprovals(child, path+"[]", found)
		}
	case string:
		for _, approval := range approvalValues {
			if v == approval {
				*found = append(*found, path+"="+v)
			}
		}
	}
}

func approvals(resp any) []string {
	if hookresponse.IsEmpty(resp) {
		return nil
	}

	data, err := json.Marshal(resp)
	Expect(err).NotTo(HaveOccurred())

	var decoded any
	Expect(json.Unmarshal(data, &decoded)).To(Succeed())

	var found []string
	collectApprovals(decoded, "", &found)

	return found
}

var _ = Describe("permission preservation", func() {
	providers := []hook.Provider{
		hook.ProviderClaude,
		hook.ProviderUnknown,
		hook.ProviderCodex,
		hook.ProviderGemini,
		hook.ProviderOpenCode,
	}

	advisory := map[string]func() []*dispatcher.ValidationError{
		"warning":            semanticsWarning,
		"accepted exception": permissionBypassed,
		"warning+exception":  permissionMixedAdvisory,
	}

	It("never approves a pre-tool action on advisory findings", func() {
		for _, provider := range providers {
			for name, errs := range advisory {
				resp := hookresponse.BuildForContext(preToolCtx(provider), errs(), nil)

				Expect(approvals(resp)).To(BeEmpty(), "provider=%s findings=%s", provider, name)
			}
		}
	})

	It("never approves a permission request on advisory findings", func() {
		for _, provider := range []hook.Provider{hook.ProviderClaude, hook.ProviderCodex} {
			for name, errs := range advisory {
				resp := hookresponse.BuildForContext(
					eventCtx(provider, "PermissionRequest"), errs(), nil,
				)

				Expect(approvals(resp)).To(BeEmpty(), "provider=%s findings=%s", provider, name)
			}
		}
	})

	It("still denies blocking findings on every provider", func() {
		for _, provider := range providers {
			resp := hookresponse.BuildForContext(preToolCtx(provider), semanticsBlocking(), nil)

			data, err := json.Marshal(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`"deny"`), "provider=%s", provider)
			Expect(approvals(resp)).To(BeEmpty(), "provider=%s", provider)
		}
	})

	It("stays silent on a clean pass", func() {
		for _, provider := range providers {
			Expect(hookresponse.IsEmpty(
				hookresponse.BuildForContext(preToolCtx(provider), nil, nil),
			)).To(BeTrue(), "provider=%s", provider)
		}
	})

	DescribeTable("Claude PreToolUse defers to the permission flow",
		func(errs func() []*dispatcher.ValidationError, contextSubstring string) {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderClaude, "PreToolUse"), errs(), nil,
			))

			out := hookSpecific(fields)
			Expect(out).To(HaveKeyWithValue("hookEventName", "PreToolUse"))
			Expect(out).NotTo(HaveKey("permissionDecision"))
			Expect(out).NotTo(HaveKey("permissionDecisionReason"))
			Expect(out["additionalContext"]).To(ContainSubstring(contextSubstring))
			Expect(fields).To(HaveKey("systemMessage"))
			Expect(fields).NotTo(HaveKey("decision"))
		},
		Entry("warning", semanticsWarning, "Not blocking"),
		Entry("accepted exception", permissionBypassed, "normal permission checks still apply"),
		Entry("warning and exception", permissionMixedAdvisory, "Emergency hotfix"),
	)

	It("says remaining blocking findings still block next to an exception", func() {
		errs := append(semanticsBlocking(), permissionBypassed()...)
		out := hookSpecific(responseFields(hookresponse.BuildForContext(
			eventCtx(hook.ProviderClaude, "PreToolUse"), errs, nil,
		)))

		Expect(out).To(HaveKeyWithValue("permissionDecision", "deny"))
		Expect(out["additionalContext"]).
			To(ContainSubstring("remaining errors still block the action"))
		Expect(out["additionalContext"]).
			NotTo(ContainSubstring("normal permission checks still apply"))
	})

	It("does not ask the agent to explain a block that did not happen", func() {
		resp := hookresponse.BuildForContext(
			eventCtx(hook.ProviderClaude, "PreToolUse"), permissionBypassed(), nil,
		)
		before := hookSpecific(responseFields(resp))["additionalContext"]

		hookresponse.AppendAgentSummary(resp)

		Expect(hookSpecific(responseFields(resp))["additionalContext"]).To(Equal(before))
	})
})
