package hookresponse

import (
	"strings"
	"unicode/utf8"

	"github.com/smykla-skalski/klaudiush/internal/validators/secrets"
)

const truncationMarker = "\n[klaudiush: output truncated to fit the hook limit]"

// sanitizeText masks secrets and replaces invalid UTF-8.
func sanitizeText(s string) string {
	return secrets.Redact(strings.ToValidUTF8(s, "�"))
}

// truncateUTF8 cuts s to at most maxBytes bytes without splitting a rune.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	if maxBytes <= 0 {
		return ""
	}

	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	return s[:cut]
}

// truncateRunes cuts s to at most n runes, marking the cut with an ellipsis.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}

	runes := []rune(s)

	return string(runes[:n]) + "..."
}

// fitBudget returns text unchanged when it fits, otherwise cuts it at a rune
// boundary and marks the cut.
func fitBudget(text string, budget int) string {
	if len(text) <= budget {
		return text
	}

	if budget < len(truncationMarker) {
		return truncateUTF8(text, budget)
	}

	return truncateUTF8(text, budget-len(truncationMarker)) + truncationMarker
}
