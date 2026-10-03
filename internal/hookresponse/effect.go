package hookresponse

// Stops reports whether a built response tells the harness to stop
// something: a denied tool call or approval, a block decision, a declined
// elicitation, or continue set to false. It reads the response that is
// written, so it agrees with what the provider is asked to do. Whether that
// prevents anything still depends on the event: after a tool ran, a block
// only asks the agent to repair.
func Stops(resp any) bool {
	switch r := resp.(type) {
	case *HookResponse:
		return r != nil && (r.Decision == decisionBlock ||
			r.HookSpecificOutput != nil && r.HookSpecificOutput.PermissionDecision == decisionDeny)
	case *CodexCommandResponse:
		return r != nil && (r.Decision == decisionBlock ||
			r.Continue != nil && !*r.Continue ||
			r.HookSpecificOutput != nil && r.HookSpecificOutput.PermissionDecision == decisionDeny)
	case *GeminiCommandResponse:
		return r != nil && (r.Decision == decisionDeny || r.Decision == decisionBlock)
	case *OpenCodeCommandResponse:
		return r != nil && r.Decision == decisionDeny
	case *ElicitationHookResponse:
		return r != nil && (r.Decision == decisionBlock || r.HookSpecificOutput != nil &&
			r.HookSpecificOutput.Action == elicitationDecline)
	case *PermissionRequestResponse:
		return r != nil && r.HookSpecificOutput != nil && r.HookSpecificOutput.Decision != nil &&
			r.HookSpecificOutput.Decision.Behavior == decisionDeny
	default:
		return false
	}
}
