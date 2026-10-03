package file

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// stringState is the multi-line string literal a line starts inside of.
// Single and double quoted strings are line-local and not tracked here.
type stringState uint8

const (
	stateCode stringState = iota
	stateBacktick
	stateTripleDouble
	stateTripleSingle
)

// tripleQuoteTail is how many bytes of a triple quote follow its first byte.
const tripleQuoteTail = 2

// tripleKind says whether a language has a triple-quoted string delimiter and
// whether a backslash escapes the next byte inside it.
type tripleKind uint8

const (
	tripleNone tripleKind = iota
	tripleEscaped
	tripleRaw
)

// commentStyle is the line-comment marker a language uses. commentLoose, for
// files whose language is not known, accepts both "//" and "#" but only at line
// start or after whitespace. commentHash accepts "#" and commentSlash accepts
// "//" anywhere outside a string. commentHashSpaced accepts "#" only at line
// start or after whitespace, for languages that also use "#" in sigils or
// character literals.
type commentStyle uint8

const (
	commentLoose commentStyle = iota
	commentHash
	commentSlash
	commentHashSpaced
)

// langSyntax is the comment and string syntax the scanner applies to a file.
// The zero value keeps the language-agnostic behaviour: no triple-quoted
// strings and loose comment markers.
type langSyntax struct {
	double  tripleKind
	single  tripleKind
	comment commentStyle
}

// langSyntaxByExt maps file extensions of languages with triple-quoted
// multi-line strings to their syntax. Elsewhere `"""` is an empty string plus
// a quote.
var langSyntaxByExt = map[string]langSyntax{
	".py":     {double: tripleEscaped, single: tripleEscaped, comment: commentHash},
	".pyi":    {double: tripleEscaped, single: tripleEscaped, comment: commentHash},
	".pyw":    {double: tripleEscaped, single: tripleEscaped, comment: commentHash},
	".toml":   {double: tripleEscaped, single: tripleRaw, comment: commentHash},
	".ex":     {double: tripleEscaped, single: tripleEscaped, comment: commentHashSpaced},
	".exs":    {double: tripleEscaped, single: tripleEscaped, comment: commentHashSpaced},
	".jl":     {double: tripleEscaped, comment: commentHashSpaced},
	".groovy": {double: tripleEscaped, single: tripleEscaped, comment: commentSlash},
	".gradle": {double: tripleEscaped, single: tripleEscaped, comment: commentSlash},
	".java":   {double: tripleEscaped, comment: commentSlash},
	".kt":     {double: tripleRaw, comment: commentSlash},
	".kts":    {double: tripleRaw, comment: commentSlash},
	".scala":  {double: tripleRaw, comment: commentSlash},
	".sc":     {double: tripleRaw, comment: commentSlash},
}

// langSyntaxForPath returns the comment and string syntax for path.
func langSyntaxForPath(path string) langSyntax {
	return langSyntaxByExt[strings.ToLower(filepath.Ext(path))]
}

// hasTripleQuote reports whether line holds three q bytes starting at i.
func hasTripleQuote(line string, i int, q byte) bool {
	return i+tripleQuoteTail < len(line) &&
		line[i] == q && line[i+1] == q && line[i+tripleQuoteTail] == q
}

// opensTripleQuote returns the state entered when a triple-quoted string
// opens at line[i], or stateCode when none opens there.
func opensTripleQuote(line string, i int, syntax langSyntax) stringState {
	switch {
	case syntax.double != tripleNone && hasTripleQuote(line, i, '"'):
		return stateTripleDouble
	case syntax.single != tripleNone && hasTripleQuote(line, i, '\''):
		return stateTripleSingle
	default:
		return stateCode
	}
}

// scanMultiLineString advances over line[i] while inside a multi-line string
// and returns the index of the last byte consumed and the resulting state. A
// raw triple-quoted string closes on the last three quotes of a longer run, so
// """a"""" holds a" in Kotlin and Scala.
func scanMultiLineString(
	line string,
	i int,
	state stringState,
	syntax langSyntax,
) (int, stringState) {
	c := line[i]

	if state == stateBacktick {
		if c == '`' {
			return i, stateCode
		}

		return i, state
	}

	q, kind := byte('"'), syntax.double
	if state == stateTripleSingle {
		q, kind = '\'', syntax.single
	}

	switch {
	case c == '\\' && kind == tripleEscaped:
		return i + 1, state
	case hasTripleQuote(line, i, q):
		for kind == tripleRaw && i+tripleQuoteTail+1 < len(line) &&
			line[i+tripleQuoteTail+1] == q {
			i++
		}

		return i + tripleQuoteTail, stateCode
	default:
		return i, state
	}
}

// isCommentMarker reports whether a line-comment marker of the given style
// starts at line[i]. Loose markers must sit at line start or after whitespace.
func isCommentMarker(line string, i int, style commentStyle) bool {
	isHash := line[i] == '#'
	isSlash := line[i] == '/' && i+1 < len(line) && line[i+1] == '/'

	switch style {
	case commentHash:
		return isHash
	case commentSlash:
		return isSlash
	case commentHashSpaced:
		return isHash && afterSpace(line, i)
	case commentLoose:
	}

	return (isHash || isSlash) && afterSpace(line, i)
}

// afterSpace reports whether line[i] is at line start or after whitespace.
func afterSpace(line string, i int) bool {
	return i == 0 || line[i-1] == ' ' || line[i-1] == '\t'
}

// findCommentStart returns the byte index of the first line-comment marker
// that is a real code-level comment, or -1 if the line has none. It tracks
// string state so a marker inside a string or URL literal (the "//" in
// "https://…", a " //" inside "a // b", a "## Heading" inside a Python
// triple-quoted string) is ignored. state is the multi-line string state
// carried in from the previous line (Go raw strings, JS template literals and
// the triple-quoted strings syntax enables span lines); the updated state is
// returned so the caller can thread it. Single/double quotes are line-local.
func findCommentStart(
	line string,
	state stringState,
	syntax langSyntax,
) (idx int, endState stringState) {
	var quote byte

	for i := 0; i < len(line); i++ {
		if state != stateCode {
			i, state = scanMultiLineString(line, i, state, syntax)

			continue
		}

		c := line[i]

		opened := stateCode
		if quote == 0 {
			opened = opensTripleQuote(line, i, syntax)
		}

		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '`':
			state = stateBacktick
		case opened != stateCode:
			state, i = opened, i+tripleQuoteTail
		case c == '\'' || c == '"':
			quote = c
		case isCommentMarker(line, i, syntax.comment):
			return i, state
		}
	}

	return -1, state
}

// commentScan is where scanning a Write or Edit payload starts: the language
// syntax, the multi-line string state the first line opens in, and whether
// triple-quoted state is dropped at each line break because the payload's
// lines are not contiguous in the file.
type commentScan struct {
	syntax          langSyntax
	start           stringState
	lineLocalTriple bool
}

// lineStart returns the state the next line starts in after a line ended in
// state.
func (s commentScan) lineStart(state stringState) stringState {
	if s.lineLocalTriple && state != stateBacktick {
		return stateCode
	}

	return state
}

// newCommentScan works out where scanning the hook payload starts. A full
// Write starts in code. An Edit's new_string starts in the string state found
// at its old_string in the file on disk, so a fragment that begins inside (or
// closes) a docstring is scanned correctly; when old_string occurs at several
// places in different states, it falls back to code. An Edit with no
// old_string on a non-empty file joins added lines from several patch hunks
// whose boundaries are lost, so triple-quoted state is not carried between its
// lines.
func newCommentScan(hookCtx *hook.Context) commentScan {
	path := hookCtx.GetFilePath()
	scan := commentScan{syntax: langSyntaxForPath(path)}

	if hookCtx.ToolName != hook.ToolTypeEdit || hookCtx.ToolInput.Content != "" {
		return scan
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil || len(data) == 0 {
		return scan
	}

	old := hookCtx.ToolInput.OldString
	if old == "" {
		scan.lineLocalTriple = true

		return scan
	}

	scan.start = stateAtOccurrences(string(data), old, scan.syntax)

	return scan
}

// stateAtOccurrences returns the multi-line string state shared by every
// occurrence of old in content, or stateCode when there is none or the
// occurrences disagree.
func stateAtOccurrences(content, old string, syntax langSyntax) stringState {
	lines := strings.Split(content, "\n")
	lineStates := make([]stringState, len(lines))
	lineOffsets := make([]int, len(lines))

	state, offset := stateCode, 0
	for i, line := range lines {
		lineStates[i], lineOffsets[i] = state, offset
		_, state = findCommentStart(line, state, syntax)
		offset += len(line) + 1
	}

	var (
		shared stringState
		found  bool
	)

	for from := 0; ; {
		rel := strings.Index(content[from:], old)
		if rel < 0 {
			break
		}

		pos := from + rel
		li := sort.SearchInts(lineOffsets, pos+1) - 1
		_, at := findCommentStart(content[lineOffsets[li]:pos], lineStates[li], syntax)

		if found && at != shared {
			return stateCode
		}

		shared, found = at, true
		from = pos + len(old)
	}

	return shared
}
