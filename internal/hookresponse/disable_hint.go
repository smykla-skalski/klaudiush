package hookresponse

import "strings"

// FormatDisableHint renders the disable hint for blocking error codes.
// This is the single point of change for the disable hint format.
func FormatDisableHint(codes []string) string {
	if len(codes) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("Wrong for your workflow? klaudiush disable ")
	b.WriteString(strings.Join(codes, " "))
	b.WriteString("\n")

	return b.String()
}
