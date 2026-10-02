package hookresponse

import (
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	// maxSummaryParagraphs limits how many non-supplementary paragraphs
	// are kept in the concise summary.
	maxSummaryParagraphs = 2

	// variationSelector16 is the Unicode variation selector that forces
	// emoji presentation (U+FE0F).
	variationSelector16 = 0xFE0F

	// zeroWidthJoiner combines emoji sequences (U+200D).
	zeroWidthJoiner = 0x200D
)

// formatDecisionReason builds the decision reason shown to the agent with the
// default budget. Every blocking error is listed with every finding and repair.
func formatDecisionReason(blocking []*dispatcher.ValidationError) string {
	return formatDecisionReasonWithin(blocking, defaultAgentBudget)
}

// formatDecisionReasonWithin builds the decision reason within budget bytes,
// trimming supplementary detail before any repair.
func formatDecisionReasonWithin(blocking []*dispatcher.ValidationError, budget int) string {
	if len(blocking) == 0 {
		return ""
	}

	return renderWithin(budget, func(level detail) string {
		return agentEntries(blocking, level)
	})
}

// blockingContextLead opens additionalContext when a finding stopped the action.
const blockingContextLead = "Automated klaudiush validation check. " +
	"Fix ALL reported errors at once and retry. " +
	"Fixing one issue can introduce another " +
	"(e.g., adding a type(scope): prefix can push the title over its length limit)."

// maxTableSuggestionLines limits how many lines of a table suggestion
// are included in additionalContext to avoid bloating the context.
const maxTableSuggestionLines = 15

// contextRequest is the input for one additionalContext rendering.
// withFindings lists the blocking findings too, for responses that carry no
// decision reason to the agent.
type contextRequest struct {
	hookCtx                      *hook.Context
	blocking, warnings, bypassed []*dispatcher.ValidationError
	patternWarnings              []string
	withFindings                 bool
	budget                       int
}

// formatAdditionalContext builds behavioral framing for Claude.
func formatAdditionalContext(
	blocking, warnings, bypassed []*dispatcher.ValidationError,
	patternWarnings []string,
) string {
	return buildContext(contextRequest{
		blocking:        blocking,
		warnings:        warnings,
		bypassed:        bypassed,
		patternWarnings: patternWarnings,
		budget:          defaultAgentBudget,
	})
}

// buildContext renders additionalContext: the lead, the findings the agent
// must act on, accepted exceptions, a table suggestion and pattern warnings.
// Findings get whatever budget the fixed parts leave.
func buildContext(req contextRequest) string {
	var lead string
	if len(req.blocking) > 0 {
		lead = blockingContextLead
	}

	exceptionOutcome := "Validation waived for this action; " +
		"normal permission checks still apply."
	if len(req.blocking) > 0 {
		exceptionOutcome = "Validation waived for this finding; " +
			"the remaining errors still block the action."
	}

	exceptions := make([]string, 0, len(req.bypassed))

	for _, e := range req.bypassed {
		code := extractCode(e.Reference)

		reason := e.BypassReason
		if reason == "" {
			reason = "no reason provided"
		}

		exceptions = append(exceptions, sanitizeText(
			"klaudiush: Exception EXC:"+code+" accepted (reason: "+reason+"). "+
				exceptionOutcome))
	}

	var trailing []string

	if table := firstTableSuggestion(req.blocking, req.warnings); table != "" {
		trailing = append(trailing, table)
	}

	trailing = append(trailing, req.patternWarnings...)

	fixed := len(lead) + len(strings.Join(exceptions, " ")) + len(strings.Join(trailing, " "))
	findingsBudget := max(req.budget-contextReserve-fixed, 0)

	findings := renderWithin(findingsBudget, func(level detail) string {
		return contextFindings(req, level)
	})

	parts := make([]string, 0, 3+len(exceptions)+len(trailing))
	if lead != "" {
		parts = append(parts, lead)
	}

	if findings != "" {
		parts = append(parts, findings)
	}

	parts = append(parts, exceptions...)
	parts = append(parts, trailing...)

	return strings.Join(parts, " ")
}

// contextFindings renders blocking findings (when asked) and every warning,
// each labeled with what it means for the action.
func contextFindings(req contextRequest, level detail) string {
	var parts []string

	if req.withFindings && len(req.blocking) > 0 {
		parts = append(parts, "Findings:\n"+agentEntries(req.blocking, level))
	}

	for _, e := range req.warnings {
		parts = append(parts, warningPrefix(outcomeOf(req.hookCtx, e))+agentEntry(e, level))
	}

	return strings.Join(parts, "\n")
}

func warningPrefix(o outcome) string {
	switch o {
	case outcomeRepairRequired:
		return "Repair required: "
	case outcomeUnavailable:
		return "klaudiush could not validate this action, so it was not checked: "
	case outcomeBlocked, outcomeExceptionAccepted, outcomeWarning:
		return "klaudiush warning: Not blocking. "
	default:
		return "klaudiush warning: Not blocking. "
	}
}

// firstTableSuggestion returns the first table suggestion, so the agent can
// see the correctly formatted table.
func firstTableSuggestion(blocking, warnings []*dispatcher.ValidationError) string {
	for _, list := range [][]*dispatcher.ValidationError{blocking, warnings} {
		for _, e := range list {
			if suggestion, ok := e.Details["suggested_table"]; ok && suggestion != "" {
				return sanitizeText(truncateTableSuggestion(suggestion))
			}
		}
	}

	return ""
}

// truncateTableSuggestion caps a table suggestion to maxTableSuggestionLines.
func truncateTableSuggestion(suggestion string) string {
	lines := strings.Split(suggestion, "\n")
	if len(lines) <= maxTableSuggestionLines {
		return suggestion
	}

	return strings.Join(lines[:maxTableSuggestionLines], "\n") + "\n..."
}

// FormatSystemMessage builds the human-readable message shown in the UI.
func FormatSystemMessage(errs []*dispatcher.ValidationError) string {
	return formatSystemMessageFor(nil, errs)
}

// formatSystemMessageFor builds the human message for the event the hook
// received, labeling each error with its outcome. Disable guidance is left to
// `klaudiush disable --help` so a block never advertises turning checks off.
func formatSystemMessageFor(hookCtx *hook.Context, errs []*dispatcher.ValidationError) string {
	if len(errs) == 0 {
		return ""
	}

	var b strings.Builder

	for _, e := range errs {
		formatSingleError(&b, outcomeOf(hookCtx, e), e)
	}

	return fitBudget(sanitizeText(b.String()), humanBudget)
}

// hiddenDetailKeys are details rendered elsewhere or kept for tooling.
var hiddenDetailKeys = map[string]bool{
	"suggested_table": true,
	"commit_preview":  true,
	"all_codes":       true,
}

// formatSingleError writes one error entry with compact, non-duplicating format.
func formatSingleError(b *strings.Builder, o outcome, e *dispatcher.ValidationError) {
	code := extractCode(e.Reference)

	b.WriteString(o.icon())
	b.WriteString(" ")
	b.WriteString(o.label())

	if code != "" {
		b.WriteString(" ")
		b.WriteString(code)
	}

	b.WriteString(": ")
	b.WriteString(stripEmoji(e.Message))
	b.WriteString("\n")

	humanFindings(b, e.Findings, code)

	if e.FixHint != "" && len(e.Findings) == 0 {
		b.WriteString("  Fix: ")
		b.WriteString(e.FixHint)
		b.WriteString("\n")
	}

	if e.Reference != "" {
		b.WriteString("  Ref: ")
		b.WriteString(string(e.Reference))
		b.WriteString("\n")
	}

	keys := slices.Sorted(maps.Keys(e.Details))
	for _, k := range keys {
		if hiddenDetailKeys[k] || (k == "errors" && len(e.Findings) > 0) {
			continue
		}

		trimmed := strings.TrimSpace(e.Details[k])
		if trimmed != "" {
			b.WriteString("\n")
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
}

// extractCode gets the error code from a Reference.
func extractCode(ref validator.Reference) string {
	if ref == "" {
		return ""
	}

	return ref.Code()
}

// summarizeMessage extracts a concise one-line summary from a rich multiline message.
// Rich messages from validators may contain emoji headers, available-remotes lists,
// usage examples, etc. This strips those down so the permissionDecisionReason
// stays compact while systemMessage retains all the detail.
func summarizeMessage(msg string) string {
	if !strings.Contains(msg, "\n") {
		return stripEmoji(msg)
	}

	paragraphs := strings.Split(msg, "\n\n")

	var parts []string

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if isSupplementaryContext(p) {
			continue
		}

		line := firstNonEmptyLine(p)
		if line == "" {
			continue
		}

		line = stripEmoji(line)
		if line == "" {
			continue
		}

		parts = append(parts, line)

		if len(parts) >= maxSummaryParagraphs {
			break
		}
	}

	// Fallback: if everything was supplementary, use the first non-empty line.
	if len(parts) == 0 {
		line := firstNonEmptyLine(msg)
		if line != "" {
			return stripEmoji(line)
		}

		return msg
	}

	if len(parts) == 1 {
		return parts[0]
	}

	// Smart join: space after colon, period otherwise.
	if strings.HasSuffix(parts[0], ":") {
		return parts[0] + " " + parts[1]
	}

	return parts[0] + ". " + parts[1]
}

// messageDetailLines returns the lines of a rich message that its summary
// leaves out, skipping supplementary paragraphs. A linter's per-line findings
// live there, and the agent needs them when the validator sent no structured
// findings.
func messageDetailLines(msg string) []string {
	if !strings.Contains(msg, "\n") {
		return nil
	}

	var lines []string

	summarized := 0

	for p := range strings.SplitSeq(msg, "\n\n") {
		if strings.TrimSpace(p) == "" || isSupplementaryContext(p) ||
			stripEmoji(firstNonEmptyLine(p)) == "" {
			continue
		}

		skipFirst := summarized < maxSummaryParagraphs
		summarized++

		for line := range strings.SplitSeq(p, "\n") {
			line = stripEmoji(line)
			if line == "" {
				continue
			}

			if skipFirst {
				skipFirst = false

				continue
			}

			lines = append(lines, line)
		}
	}

	return lines
}

// supplementaryPrefixes are line prefixes that indicate context paragraphs
// (remotes lists, usage hints, file listings) that should be excluded from
// the concise summary.
var supplementaryPrefixes = []string{
	"Available remotes:",
	"Use '",
	"Use \"",
	"Use `",
	"Files being added:",
	"Current status:",
	"Example:",
	"Examples:",
	"Staged files:",
	"Modified files:",
	"Untracked files:",
	"Tip:",
	"Note:",
	"See ",
}

// isSupplementaryContext returns true when a paragraph starts with a prefix
// that signals supplementary detail not suitable for the concise reason.
func isSupplementaryContext(paragraph string) bool {
	line := firstNonEmptyLine(paragraph)
	stripped := stripEmoji(line)

	for _, prefix := range supplementaryPrefixes {
		if strings.HasPrefix(stripped, prefix) {
			return true
		}
	}

	return false
}

// firstNonEmptyLine returns the first non-blank line from text.
func firstNonEmptyLine(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// stripEmoji removes emoji characters and variation selectors, then collapses
// any resulting leading/trailing whitespace.
func stripEmoji(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		if !isEmojiRune(r) {
			b.WriteRune(r)
		}
	}

	return strings.TrimSpace(b.String())
}

// isEmojiRune returns true for common emoji code points and variation selectors.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F600 && r <= 0x1F64F: // emoticons
		return true
	case r >= 0x1F300 && r <= 0x1F5FF: // misc symbols & pictographs
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // transport & map symbols
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // supplemental symbols
		return true
	case r >= 0x2600 && r <= 0x26FF: // misc symbols (⚠ etc.)
		return true
	case r >= 0x2700 && r <= 0x27BF: // dingbats (❌ etc.)
		return true
	case r == variationSelector16: // emoji presentation
		return true
	case r == zeroWidthJoiner: // combines emoji sequences
		return true
	case !unicode.IsPrint(r) && !unicode.IsSpace(r): // other non-printable
		return true
	}

	return false
}
