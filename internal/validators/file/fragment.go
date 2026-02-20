package file

import (
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/validators"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// FragmentResult contains the extracted fragment content and edit range metadata.
type FragmentResult struct {
	Content   string
	EditRange validators.FragmentRange
}

// ExtractEditFragment extracts the edit region with surrounding context lines.
// It finds the oldStr in content, replaces it with newStr, and returns a fragment
// containing the edit plus contextLines before and after for proper linting context.
func ExtractEditFragment(
	content string,
	oldStr string,
	newStr string,
	contextLines int,
	log logger.Logger,
) string {
	result := ExtractEditFragmentWithRange(content, oldStr, newStr, contextLines, log)

	return result.Content
}

// ExtractEditFragmentWithRange extracts the edit region with surrounding context lines
// and returns both the fragment content and the range of edited lines within the fragment.
func ExtractEditFragmentWithRange(
	content string,
	oldStr string,
	newStr string,
	contextLines int,
	log logger.Logger,
) FragmentResult {
	// Find the position of oldStr in content
	idx := strings.Index(content, oldStr)
	if idx == -1 {
		log.Debug("old_string not found in file content")

		return FragmentResult{}
	}

	// Split content into lines
	lines := strings.Split(content, "\n")

	// Find which line contains the start and end of oldStr
	startLine := findLineNumber(lines, idx)
	endLine := findEndLineNumber(lines, idx+len(oldStr))

	// Extract lines with context
	contextStart := max(0, startLine-contextLines)
	contextEnd := min(endLine+contextLines, len(lines)-1)

	// Build fragment with the edit applied
	fragmentLines := make([]string, 0, contextEnd-contextStart+1)

	for i := contextStart; i <= contextEnd; i++ {
		fragmentLines = append(fragmentLines, lines[i])
	}

	// Strip trailing empty lines to avoid false positives from files with trailing blank lines.
	// These trailing blanks, when combined with preamble context, can create consecutive
	// blank lines that trigger MD012 (no-multiple-blanks) errors.
	fragmentLines = trimTrailingEmptyLines(fragmentLines)

	// Apply the replacement at exact character offset instead of strings.Replace
	// to avoid hitting wrong substring in context lines
	fragment := strings.Join(fragmentLines, "\n")
	contextCharOffset := sumCharCounts(lines[:contextStart])
	fragmentOffset := idx - contextCharOffset

	if fragmentOffset >= 0 && fragmentOffset+len(oldStr) <= len(fragment) {
		fragment = fragment[:fragmentOffset] + newStr + fragment[fragmentOffset+len(oldStr):]
	} else {
		// Fallback: should not happen, but be safe
		fragment = strings.Replace(fragment, oldStr, newStr, 1)
	}

	// Compute edit range within the fragment (1-indexed)
	contextBefore := startLine - contextStart
	editStart := contextBefore + 1
	newStrLineCount := strings.Count(newStr, "\n") + 1
	editEnd := editStart + newStrLineCount - 1

	return FragmentResult{
		Content: fragment,
		EditRange: validators.FragmentRange{
			EditStart: editStart,
			EditEnd:   editEnd,
		},
	}
}

// trimTrailingEmptyLines removes excess trailing empty strings from a slice.
// Keeps at most one trailing empty string (preserving normal trailing newline)
// but removes additional ones (blank lines) that would cause MD012 errors
// when combined with preamble context.
func trimTrailingEmptyLines(lines []string) []string {
	// Count trailing empty lines
	trailingCount := 0

	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			break
		}

		trailingCount++
	}

	// Keep at most one trailing empty line (normal trailing newline)
	if trailingCount > 1 {
		lines = lines[:len(lines)-(trailingCount-1)]
	}

	return lines
}

// getFragmentStartLine returns the line number where the fragment starts (0-indexed).
// This accounts for context lines added before the actual edit location.
func getFragmentStartLine(content, oldStr string, contextLines int) int {
	idx := strings.Index(content, oldStr)
	if idx == -1 {
		return 0
	}

	lines := strings.Split(content, "\n")
	startLine := findLineNumber(lines, idx)

	return max(0, startLine-contextLines)
}

// findLineNumber returns the 0-indexed line number containing the character at position idx.
// Used for finding the start of a match.
func findLineNumber(lines []string, idx int) int {
	charCount := 0

	for i, line := range lines {
		if charCount+len(line)+1 > idx { // +1 for newline
			return i
		}

		charCount += len(line) + 1
	}

	return len(lines) - 1
}

// findEndLineNumber returns the 0-indexed line number containing the character at position endIdx.
// Used for finding the end of a match.
func findEndLineNumber(lines []string, endIdx int) int {
	charCount := 0

	for i, line := range lines {
		charCount += len(line) + 1

		if charCount >= endIdx {
			return i
		}
	}

	return len(lines) - 1
}

// sumCharCounts returns the total character count of lines (including newline separators).
func sumCharCounts(lines []string) int {
	total := 0

	for _, line := range lines {
		total += len(line) + 1 // +1 for newline
	}

	return total
}

// EditReachesEOF determines if an edit operation reaches the end of file.
// Returns true if the new_string will end at or near EOF after the replacement.
//
// The logic:
//  1. Find where old_string ends in the original content
//  2. Check what remains after old_string (the "tail")
//  3. If tail is empty or only whitespace/newlines, the edit reaches EOF
//
// This is used to determine whether MD047 (single-trailing-newline) should be checked.
// For mid-file edits, we don't want MD047 to complain about fragments not ending with newline.
func EditReachesEOF(content, oldStr string) bool {
	idx := strings.Index(content, oldStr)
	if idx == -1 {
		return false
	}

	// Get everything after old_string
	endIdx := idx + len(oldStr)
	tail := content[endIdx:]

	// If there's nothing after old_string, or only whitespace/newlines, the edit reaches EOF
	trimmed := strings.TrimSpace(tail)

	return trimmed == ""
}
