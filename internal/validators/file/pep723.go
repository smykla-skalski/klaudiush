package file

import (
	"regexp"
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
// first well-formed PEP 723 script block; the spec allows only one. As in the
// spec's reference regex, a block opens with "# /// script", holds at least one line that is "#" or
// "# ...", and closes on the last "# ///" of that unbroken run of comment
// lines. Every line must be a top-level comment starting in column 0, so an
// unterminated block, or one broken by code, exempts nothing.
func pep723Lines(lines []string, state stringState, scan commentScan) []bool {
	topLevel := make([]bool, len(lines))

	for i, line := range lines {
		var idx int

		start := state
		idx, state = findCommentStart(line, state, scan.syntax)
		state = scan.lineStart(state)
		topLevel[i] = idx == 0 && start == stateCode
	}

	inBlock := make([]bool, len(lines))

	for i := range lines {
		if !topLevel[i] || trimCR(lines[i]) != pep723Start {
			continue
		}

		end := -1

		for j := i + 1; j < len(lines) && topLevel[j] && pep723Body.MatchString(trimCR(lines[j])); j++ {
			if trimCR(lines[j]) == pep723End {
				end = j
			}
		}

		if end <= i+1 {
			continue
		}

		for j := i; j <= end; j++ {
			inBlock[j] = true
		}

		break
	}

	return inBlock
}

func trimCR(line string) string {
	return strings.TrimSuffix(line, "\r")
}
