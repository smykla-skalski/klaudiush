package file

import (
	"regexp"
	"slices"
	"strings"
)

// pep723Body is an inner line of a PEP 723 inline script metadata block.
// Only the "script" type is defined; other types are reserved, so accepting
// them would let any comment wrapped in "# /// x" skip the check.
var pep723Body = regexp.MustCompile(`^#( .*)?$`)

const (
	pep723Start = "# /// script"
	pep723End   = "# ///"
)

// pep723Lines reports which of lines, scanned from state, belong to the
// first well-formed PEP 723 script block (see pep723Block).
func pep723Lines(lines []string, state stringState, scan commentScan) []bool {
	topLevel := make([]bool, len(lines))

	for i, line := range lines {
		var idx int

		start := state
		idx, state = findCommentStart(line, state, scan.syntax)
		state = scan.lineStart(state)
		topLevel[i] = idx == 0 && start == stateCode
	}

	return pep723Block(lines, topLevel)
}

// pep723Block marks the first well-formed PEP 723 script block in lines; the
// spec allows only one. As in the spec's reference regex, a block opens with
// "# /// script", holds at least one line that is "#" or "# ...", and closes
// on the last "# ///" of that unbroken run of comment lines. topLevel says
// which lines are comments starting in column 0 outside any string, and
// every block line must be one, so an unterminated block, or one broken by
// code, marks nothing.
func pep723Block(lines []string, topLevel []bool) []bool {
	inBlock := make([]bool, len(lines))

	for i := 0; i < len(lines); i++ {
		if !topLevel[i] || trimCR(lines[i]) != pep723Start {
			continue
		}

		end, runEnd := -1, i+1

		for ; runEnd < len(lines) && topLevel[runEnd] &&
			pep723Body.MatchString(trimCR(lines[runEnd])); runEnd++ {
			if trimCR(lines[runEnd]) == pep723End {
				end = runEnd
			}
		}

		if end <= i+1 {
			// Any later start in this run sees the same closing lines, so it
			// cannot close either; skipping the run keeps the scan linear.
			i = runEnd - 1

			continue
		}

		for j := i; j <= end; j++ {
			inBlock[j] = true
		}

		break
	}

	return inBlock
}

// lastMarked returns the index of the last true entry, or -1.
func lastMarked(marks []bool) int {
	for i, mark := range slices.Backward(marks) {
		if mark {
			return i
		}
	}

	return -1
}

func trimCR(line string) string {
	return strings.TrimSuffix(line, "\r")
}
