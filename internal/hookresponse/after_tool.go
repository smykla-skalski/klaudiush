package hookresponse

import (
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	afterToolContextLead = "Automated klaudiush validation check after the tool ran. " +
		"The tool was not stopped and its changes are already applied. " +
		"Repair required: fix ALL reported errors in the affected files at once."

	failedToolContextLead = "Automated klaudiush validation check after the tool failed. " +
		"The failed tool may have left partial changes on disk. " +
		"Repair required: fix ALL reported errors in the affected files at once."

	afterToolWarningLead = "klaudiush checked the files after the tool ran. " +
		"The changes are already applied. Repair every finding below that your change caused."

	failedToolWarningLead = "klaudiush checked the files the failed tool may have " +
		"partly changed. Repair every finding below that your change caused."

	afterToolReasonPrefix  = "Repair required, the change is already applied: "
	failedToolReasonPrefix = "Repair required, the failed tool may have left partial changes: "

	afterToolUserNotice = "klaudiush checked the result after the tool ran. " +
		"The change was not stopped; the files below need repair."

	failedToolUserNotice = "klaudiush checked the files a failed tool may have " +
		"partly changed. The files below need repair."

	afterToolUnavailableLead = "Automated klaudiush validation check after the tool ran. " +
		"A required check could not run, so the result was not validated. " +
		"Do not edit files for it; tell the user what failed."
)

// formatContextFor builds additionalContext for the event the hook received.
// After a tool ran nothing can be prevented, so blocking findings ask for a
// repair instead of a retry. withFindings lists the blocking findings in the
// context, for responses that give the agent no decision reason.
func formatContextFor(
	hookCtx *hook.Context,
	blocking, warnings, bypassed []*dispatcher.ValidationError,
	patternWarnings []string,
	withFindings bool,
) string {
	text := buildContext(contextRequest{
		hookCtx:         hookCtx,
		blocking:        blocking,
		warnings:        warnings,
		bypassed:        bypassed,
		patternWarnings: patternWarnings,
		withFindings:    withFindings,
		budget:          agentBudgetFor(hookCtx),
	})
	if hookCtx == nil || !hookCtx.IsAfterTool() {
		return text
	}

	if len(blocking) > 0 {
		text = strings.Replace(text, unavailableContextLead, afterToolUnavailableLead, 1)

		return strings.Replace(text, blockingContextLead, afterToolLead(hookCtx), 1)
	}

	if !needsRepair(warnings) {
		return text
	}

	return warningLeadFor(hookCtx) + " " + text
}

// formatReasonFor builds the decision reason for the event the hook received.
func formatReasonFor(hookCtx *hook.Context, blocking []*dispatcher.ValidationError) string {
	prefix := ""

	if hookCtx != nil && hookCtx.IsAfterTool() && needsRepair(blocking) {
		prefix = afterToolReasonPrefix
		if hookCtx.ToolFailed() {
			prefix = failedToolReasonPrefix
		}
	}

	return prefix + formatDecisionReasonWithin(blocking, agentBudgetFor(hookCtx)-len(prefix))
}

func warningLeadFor(hookCtx *hook.Context) string {
	if hookCtx.ToolFailed() {
		return failedToolWarningLead
	}

	return afterToolWarningLead
}

func afterToolLead(hookCtx *hook.Context) string {
	if hookCtx.ToolFailed() {
		return failedToolContextLead
	}

	return afterToolContextLead
}

// noteAfterToolRepair tells the user that blocking findings from a check
// after the tool ran describe changes already made, not a stopped action.
func noteAfterToolRepair(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	resp any,
) {
	blocking, _, _ := categorize(errs)
	if hookCtx == nil || !hookCtx.IsAfterTool() || !needsRepair(blocking) {
		return
	}

	p := systemMessage(resp)
	if p == nil || *p == "" {
		return
	}

	notice := afterToolUserNotice
	if hookCtx.ToolFailed() {
		notice = failedToolUserNotice
	}

	*p = notice + "\n\n" + *p
}

// needsRepair reports whether any finding asks for a file change. A check that
// could not run says nothing about the files.
func needsRepair(errs []*dispatcher.ValidationError) bool {
	for _, e := range errs {
		if !e.Unavailable {
			return true
		}
	}

	return false
}
