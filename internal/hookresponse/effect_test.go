package hookresponse_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
)

var _ = Describe("Stops", func() {
	stop := false
	goOn := true

	DescribeTable("reads the stop directive of every response shape",
		func(resp any, expected bool) {
			Expect(hookresponse.Stops(resp)).To(Equal(expected))
		},
		Entry("nil", nil, false),
		Entry("typed nil", (*hookresponse.HookResponse)(nil), false),
		Entry("claude deny", &hookresponse.HookResponse{
			HookSpecificOutput: &hookresponse.HookSpecificOutput{PermissionDecision: "deny"},
		}, true),
		Entry("claude context only", &hookresponse.HookResponse{
			HookSpecificOutput: &hookresponse.HookSpecificOutput{AdditionalContext: "x"},
		}, false),
		Entry("claude block", &hookresponse.HookResponse{Decision: "block"}, true),
		Entry("codex deny", &hookresponse.CodexCommandResponse{
			HookSpecificOutput: &hookresponse.CodexHookSpecificOutput{PermissionDecision: "deny"},
		}, true),
		Entry("codex continue false", &hookresponse.CodexCommandResponse{Continue: &stop}, true),
		Entry("codex continue true", &hookresponse.CodexCommandResponse{Continue: &goOn}, false),
		Entry("codex block", &hookresponse.CodexCommandResponse{Decision: "block"}, true),
		Entry("gemini deny", &hookresponse.GeminiCommandResponse{Decision: "deny"}, true),
		Entry("gemini message", &hookresponse.GeminiCommandResponse{SystemMessage: "x"}, false),
		Entry("opencode deny", &hookresponse.OpenCodeCommandResponse{Decision: "deny"}, true),
		Entry("opencode continue", &hookresponse.OpenCodeCommandResponse{Continue: true}, false),
		Entry("elicitation decline", &hookresponse.ElicitationHookResponse{
			HookSpecificOutput: &hookresponse.ElicitationOutput{Action: "decline"},
		}, true),
		Entry("elicitation accept", &hookresponse.ElicitationHookResponse{
			HookSpecificOutput: &hookresponse.ElicitationOutput{Action: "accept"},
		}, false),
		Entry("permission deny", &hookresponse.PermissionRequestResponse{
			HookSpecificOutput: &hookresponse.PermissionRequestOutput{
				Decision: &hookresponse.PermissionRequestDecision{Behavior: "deny"},
			},
		}, true),
		Entry("permission without decision", &hookresponse.PermissionRequestResponse{
			HookSpecificOutput: &hookresponse.PermissionRequestOutput{},
		}, false),
		Entry("unknown type", "deny", false),
	)
})
