package file

import (
	"regexp"
	"strings"
)

// pep723Start and pep723Body are the opening and inner lines of a PEP 723
// inline script metadata block; pep723End closes it.
var (
	pep723Start = regexp.MustCompile(`^# /// [a-zA-Z0-9-]+$`)
	pep723Body  = regexp.MustCompile(`^#( .*)?$`)
)

const pep723End = "# ///"

// pep723Lines reports which of lines, scanned from state, belong to a
// well-formed PEP 723 metadata block. As in the spec's reference regex, a
// block opens with "# /// <type>", holds at least one line that is "#" or
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

	for i := 0; i < len(lines); i++ {
		if !topLevel[i] || !pep723Start.MatchString(trimCR(lines[i])) {
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

		i = end
	}

	return inBlock
}

func trimCR(line string) string {
	return strings.TrimSuffix(line, "\r")
}
