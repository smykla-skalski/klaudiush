package hookresponse

import (
	"strings"
	"unicode"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
)

const (
	// maxReasonCharsPerError caps each error's contribution to permissionDecisionReason.
	maxReasonCharsPerError = 200

	// reasonSeparator joins multiple error reasons.
	reasonSeparator = " | "

	// maxSummaryParagraphs limits how many non-supplementary paragraphs
	// are kept in the concise summary.
	maxSummaryParagraphs = 2

	// variationSelector16 is the Unicode variation selector that forces
	// emoji presentation (U+FE0F).
	variationSelector16 = 0xFE0F

	// zeroWidthJoiner combines emoji sequences (U+200D).
	zeroWidthJoiner = 0x200D
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

	b.WriteString(summarizeMessage(e.Message))

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

// maxTableSuggestionLines limits how many lines of a table suggestion
// are included in additionalContext to avoid bloating the context.
const maxTableSuggestionLines = 15

// formatAdditionalContext builds behavioral framing for Claude.
func formatAdditionalContext(
	blocking, warnings, bypassed []*dispatcher.ValidationError,
	patternWarnings []string,
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
					"Fix ALL reported errors at once and retry. "+
					"Fixing one issue can introduce another "+
					"(e.g., adding type(scope): prefix makes title exceed 50 chars).")
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

	// Include table suggestions in context so Claude can see the correctly
	// formatted table. Check both blocking and warning errors.
	allErrs := make([]*dispatcher.ValidationError, 0, len(blocking)+len(warnings))
	allErrs = append(allErrs, blocking...)
	allErrs = append(allErrs, warnings...)

	for _, e := range allErrs {
		if suggestion, ok := e.Details["suggested_table"]; ok && suggestion != "" {
			parts = append(parts, truncateTableSuggestion(suggestion))

			break // Only include first suggestion
		}
	}

	parts = append(parts, patternWarnings...)

	return strings.Join(parts, " ")
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
// This replaces the old FormatErrors function in the dispatcher package.
func FormatSystemMessage(errs []*dispatcher.ValidationError) string {
	if len(errs) == 0 {
		return ""
	}

	var b strings.Builder

	for _, e := range errs {
		formatSingleError(&b, e)
	}

	// Append disable hint for blocking error codes
	var codes []string

	seen := make(map[string]bool)

	for _, e := range errs {
		if !e.ShouldBlock {
			continue
		}

		code := extractCode(e.Reference)
		if code != "" && !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}

	b.WriteString(FormatDisableHint(codes))

	return b.String()
}

// formatSingleError writes one error entry with compact, non-duplicating format.
func formatSingleError(b *strings.Builder, e *dispatcher.ValidationError) {
	code := extractCode(e.Reference)
	emoji := "\u274c"

	if !e.ShouldBlock {
		emoji = "\u26a0\ufe0f"
	}

	// Header: emoji CODE: message
	b.WriteString(emoji)
	b.WriteString(" ")

	if code != "" {
		b.WriteString(code)
		b.WriteString(": ")
	}

	b.WriteString(stripEmoji(e.Message))
	b.WriteString("\n")

	// Fix hint
	if e.FixHint != "" {
		b.WriteString("  Fix: ")
		b.WriteString(e.FixHint)
		b.WriteString("\n")
	}

	// Reference
	if e.Reference != "" {
		b.WriteString("  Ref: ")
		b.WriteString(string(e.Reference))
		b.WriteString("\n")
	}

	// Details (supplementary only - skip keys rendered elsewhere)
	if len(e.Details) > 0 {
		for k, v := range e.Details {
			if k == "suggested_table" || k == "commit_preview" {
				continue
			}

			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				b.WriteString("\n")
				b.WriteString(trimmed)
				b.WriteString("\n")
			}
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

// isSessionPoisoned checks if any blocking error is a SESS001 session poison error.
func isSessionPoisoned(blocking []*dispatcher.ValidationError) bool {
	for _, e := range blocking {
		if e.Reference == validator.RefSessionPoisoned {
			return true
		}
	}

	return false
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
