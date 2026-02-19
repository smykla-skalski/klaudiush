package hookresponse

import (
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
)

const (
	// maxReasonCharsPerError caps each error's contribution to permissionDecisionReason.
	maxReasonCharsPerError = 200

	// reasonSeparator joins multiple error reasons.
	reasonSeparator = " | "
)

// formatDecisionReason builds the permissionDecisionReason string shown to Claude.
// Format per error: [CODE] message. Fix hint.
func formatDecisionReason(blocking []*dispatcher.ValidationError) string {
	if len(blocking) == 0 {
		return ""
	}

	parts := make([]string, 0, len(blocking))

	for _, e := range blocking {
		parts = append(parts, formatSingleReason(e))
	}

	return strings.Join(parts, reasonSeparator)
}

// formatSingleReason formats one error for the decision reason.
func formatSingleReason(e *dispatcher.ValidationError) string {
	var b strings.Builder

	code := extractCode(e.Reference)
	if code != "" {
		b.WriteString("[")
		b.WriteString(code)
		b.WriteString("] ")
	}

	b.WriteString(e.Message)

	if e.FixHint != "" {
		if !strings.HasSuffix(e.Message, ".") {
			b.WriteString(".")
		}

		b.WriteString(" ")
		b.WriteString(e.FixHint)
	}

	s := b.String()
	if len(s) > maxReasonCharsPerError {
		return s[:maxReasonCharsPerError-3] + "..."
	}

	return s
}

// formatAdditionalContext builds behavioral framing for Claude.
func formatAdditionalContext(
	blocking, warnings, bypassed []*dispatcher.ValidationError,
) string {
	var parts []string

	if len(blocking) > 0 {
		if isSessionPoisoned(blocking) {
			parts = append(parts,
				"Automated klaudiush session check. "+
					"A previous command was blocked. "+
					"Acknowledge the error codes to unpoison the session, then retry.")
		} else {
			parts = append(parts,
				"Automated klaudiush validation check. "+
					"Fix the reported errors and retry the same command.")
		}
	}

	for _, e := range bypassed {
		code := extractCode(e.Reference)

		reason := e.BypassReason
		if reason == "" {
			reason = "no reason provided"
		}

		parts = append(parts,
			"klaudiush: Exception EXC:"+code+" accepted (reason: "+reason+"). "+
				"Proceeding despite validation failure.")
	}

	for _, e := range warnings {
		parts = append(parts,
			"klaudiush warning: "+e.Message+". Not blocking.")
	}

	return strings.Join(parts, " ")
}

// FormatSystemMessage builds the human-readable message shown in the UI.
// This replaces the old FormatErrors function in the dispatcher package.
func FormatSystemMessage(errs []*dispatcher.ValidationError) string {
	if len(errs) == 0 {
		return ""
	}

	blockingErrs := make([]*dispatcher.ValidationError, 0)
	warningErrs := make([]*dispatcher.ValidationError, 0)

	for _, e := range errs {
		if e.ShouldBlock {
			blockingErrs = append(blockingErrs, e)
		} else {
			warningErrs = append(warningErrs, e)
		}
	}

	var b strings.Builder

	if len(blockingErrs) > 0 {
		formatErrorList(&b, "\u274c Validation Failed:", blockingErrs)
	}

	if len(warningErrs) > 0 {
		formatErrorList(&b, "\u26a0\ufe0f  Warnings:", warningErrs)
	}

	return b.String()
}

// formatErrorList writes a categorized list of errors.
func formatErrorList(b *strings.Builder, header string, errs []*dispatcher.ValidationError) {
	if len(errs) == 0 {
		return
	}

	b.WriteString("\n")
	b.WriteString(header)

	for _, e := range errs {
		b.WriteString(" ")
		b.WriteString(shortName(e.Validator))
	}

	b.WriteString("\n\n")

	for _, e := range errs {
		formatSingleError(b, e)
	}
}

// formatSingleError writes one error entry.
func formatSingleError(b *strings.Builder, e *dispatcher.ValidationError) {
	b.WriteString(e.Message)
	b.WriteString("\n")

	if e.FixHint != "" {
		b.WriteString("   Fix: ")
		b.WriteString(e.FixHint)
		b.WriteString("\n")
	}

	if e.Reference != "" {
		b.WriteString("   Reference: ")
		b.WriteString(string(e.Reference))
		b.WriteString("\n")
	}

	if len(e.Details) > 0 {
		b.WriteString("\n")

		for _, v := range e.Details {
			lines := strings.SplitSeq(strings.TrimSpace(v), "\n")
			for line := range lines {
				if line != "" {
					b.WriteString(line)
					b.WriteString("\n")
				}
			}
		}
	}

	b.WriteString("\n")
}

// shortName strips the "validate-" prefix from a validator name.
func shortName(name string) string {
	return strings.TrimPrefix(name, "validate-")
}

// extractCode gets the error code from a Reference.
func extractCode(ref validator.Reference) string {
	if ref == "" {
		return ""
	}

	return ref.Code()
}

// isSessionPoisoned checks if any blocking error is a SESS001 session poison error.
func isSessionPoisoned(blocking []*dispatcher.ValidationError) bool {
	for _, e := range blocking {
		if e.Reference == validator.RefSessionPoisoned {
			return true
		}
	}

	return false
}
