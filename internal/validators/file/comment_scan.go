package file

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// stringState is the multi-line string literal a line starts inside of.
// Single and double quoted strings are line-local and not tracked here.
// stateLineComment marks an Edit fragment that starts inside a line comment:
// the rest of that first line is comment text already in the file.
type stringState uint8

const (
	stateCode stringState = iota
	stateBacktick
	stateTripleDouble
	stateTripleSingle
	stateLineComment
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
// start or after whitespace. commentHash accepts only "#", also only at line
// start or after whitespace; a "#" right after code ends the scan of the line
// instead, since it is either a comment or text in a Python 3.12 f-string field
// that reuses the outer quote, and neither may open a string.
type commentStyle uint8

const (
	commentLoose commentStyle = iota
	commentHash
)

// langSyntax is the comment and string syntax the scanner applies to a file.
// The zero value keeps the language-agnostic behaviour: no triple-quoted
// strings and loose comment markers.
type langSyntax struct {
	double     tripleKind
	single     tripleKind
	comment    commentStyle
	closeOnRun bool
}

// langSyntaxByExt maps file extensions of languages with triple-quoted
// multi-line strings to their syntax. Elsewhere `"""` is an empty string plus
// a quote. closeOnRun closes a triple-quoted string on the last three quotes
// of a longer run, as TOML does. Only languages with no block comments and no use of "#" outside
// comments and strings are listed: a block comment holding `"""` would
// otherwise open a string that hides every later comment.
var langSyntaxByExt = map[string]langSyntax{
	".py":  {double: tripleEscaped, single: tripleEscaped, comment: commentHash},
	".pyi": {double: tripleEscaped, single: tripleEscaped, comment: commentHash},
	".pyw": {double: tripleEscaped, single: tripleEscaped, comment: commentHash},
	".toml": {
		double:     tripleEscaped,
		single:     tripleRaw,
		comment:    commentHash,
		closeOnRun: true,
	},
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
// and returns the index of the last byte consumed and the resulting state.
// With closeOnRun, a string ending in four quotes keeps one in its value.
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
		for syntax.closeOnRun && i+tripleQuoteTail+1 < len(line) &&
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

	if style == commentHash {
		return isHash && afterSpace(line, i)
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
	if state == stateLineComment {
		return -1, stateCode
	}

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
		case syntax.comment == commentHash && c == '#':
			return -1, state
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
// places in different states, it falls back to code. Only languages with
// triple-quoted strings, and extension-less files that may hold a Python
// shebang, read the file; CRLF line endings are matched as LF. An Edit with no
// old_string on a non-empty file joins added lines from several patch hunks
// whose boundaries are lost, so triple-quoted state is not carried between its
// lines.
func newCommentScan(hookCtx *hook.Context) commentScan {
	path := hookCtx.GetFilePath()
	scan := commentScan{syntax: langSyntaxForPath(path)}
	detectShebang := scan.syntax == (langSyntax{}) && filepath.Ext(path) == ""

	if hookCtx.ToolName != hook.ToolTypeEdit || hookCtx.ToolInput.Content != "" {
		if detectShebang {
			scan.syntax = shebangSyntax(hookCtx.ToolInput.Content)
		}

		return scan
	}

	if scan.syntax == (langSyntax{}) && !detectShebang {
		return scan
	}

	data, ok := readRegularFile(hook.CanonicalFilePath(hookCtx.WorkingDir, path))
	if !ok || len(data) == 0 {
		return scan
	}

	original := strings.ReplaceAll(string(data), "\r\n", "\n")

	if detectShebang {
		scan.syntax = shebangSyntax(original)
		if scan.syntax == (langSyntax{}) {
			return scan
		}
	}

	old := strings.ReplaceAll(hookCtx.ToolInput.OldString, "\r\n", "\n")
	if old == "" {
		scan.lineLocalTriple = true

		return scan
	}

	scan.start = stateAtOccurrences(original, old, scan.syntax)

	return scan
}

// pythonShebang matches a first line that runs the file with Python.
var pythonShebang = regexp.MustCompile(`^#!.*\bpython[0-9.]*(\s|$)`)

// shebangSyntax returns the Python syntax when text starts with a Python
// shebang, for scripts without an extension, and the default syntax otherwise.
func shebangSyntax(text string) langSyntax {
	firstLine, _, _ := strings.Cut(text, "\n")
	if pythonShebang.MatchString(firstLine) {
		return langSyntaxByExt[".py"]
	}

	return langSyntax{}
}

// readRegularFile reads path when it is a regular file; a FIFO or device would
// block or never end.
func readRegularFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, false
	}

	return data, true
}

// stateAtOccurrences returns the multi-line string state shared by every
// occurrence of old in content (stateLineComment when it starts inside a line
// comment), or stateCode when there is none or the occurrences disagree.
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

		idx, at := findCommentStart(content[lineOffsets[li]:pos], lineStates[li], syntax)
		if idx >= 0 {
			at = stateLineComment
		}

		if found && at != shared {
			return stateCode
		}

		shared, found = at, true
		from = pos + len(old)
	}

	return shared
}
