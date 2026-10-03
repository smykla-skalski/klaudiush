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
// Outside Python, single and double quoted strings are line-local and not
// tracked here. In Python it is a stack of frames (see pyFrame) so f-string
// replacement fields, which may span lines and hold comments and nested
// strings, are tracked too.
type stringState string

const (
	stateCode         stringState = ""
	stateBacktick     stringState = "`"
	stateTripleDouble stringState = `"`
	stateTripleSingle stringState = "'"
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
// start or after whitespace; a "#" right after code is an unspaced comment
// that is not reported, and scanning of the line stops there.
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
	python     bool
}

// pythonSyntax is the syntax of Python sources, scanned by scanPython.
var pythonSyntax = langSyntax{comment: commentHash, python: true}

// langSyntaxByExt maps file extensions of languages with triple-quoted
// multi-line strings to their syntax. Elsewhere `"""` is an empty string plus
// a quote. closeOnRun closes a triple-quoted string on the last three quotes
// of a longer run, as TOML does. Only languages with no block comments and no
// use of "#" outside comments and strings are listed: a block comment holding
// `"""` would otherwise open a string that hides every later comment.
var langSyntaxByExt = map[string]langSyntax{
	".py":  pythonSyntax,
	".pyi": pythonSyntax,
	".pyw": pythonSyntax,
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

// isCommentMarker reports whether a loose line-comment marker ("//" or "#")
// starts at line[i]. Loose markers must sit at line start or after whitespace.
func isCommentMarker(line string, i int) bool {
	isHash := line[i] == '#'
	isSlash := line[i] == '/' && i+1 < len(line) && line[i+1] == '/'

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
// carried in from the previous line; the updated state is returned so the
// caller can thread it.
func findCommentStart(
	line string,
	state stringState,
	syntax langSyntax,
) (idx int, endState stringState) {
	idx, endState, _ = scanSegment(line, state, syntax)
	if syntax.python {
		endState = endPythonLine(endState)
	}

	return idx, endState
}

// scanSegment scans line, which may be only the start of a source line, from
// state. It returns the index of a reported comment marker (or -1), the state
// where scanning ended, and whether it stopped at an unspaced "#" comment.
func scanSegment(
	line string,
	state stringState,
	syntax langSyntax,
) (idx int, endState stringState, unspaced bool) {
	if syntax.python {
		return scanPython(line, state)
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
			switch {
			case c == '\\' && (quote != '\'' || syntax.single != tripleRaw):
				i++
			case c == quote:
				quote = 0
			}
		case opened != stateCode:
			state, i = opened, i+tripleQuoteTail
		case c == '\'' || c == '"':
			quote = c
		case syntax.comment == commentHash:
			if c == '#' {
				return hashComment(line, i, state)
			}
		case c == '`':
			state = stateBacktick
		case isCommentMarker(line, i):
			return i, state, false
		}
	}

	return -1, state, false
}

// hashComment reports a "#" comment at line[i] when it follows whitespace;
// an unspaced one ends the scan of the line unreported.
func hashComment(line string, i int, state stringState) (int, stringState, bool) {
	if afterSpace(line, i) {
		return i, state, false
	}

	return -1, state, true
}

// Python frames. A string frame is pyFrame plus pyDouble, pyTriple and
// pyFString flags. pyField is an f-string replacement field, pyBracket a
// bracket opened inside one, and pySpec its format spec. All have the high
// bit set so a Python state never equals a non-Python one.
const (
	pyFrame    byte = 0x80
	pyDouble   byte = 0x01
	pyTriple   byte = 0x02
	pyFString  byte = 0x04
	pyField    byte = 0x90
	pyBracket  byte = 0xA0
	pySpec     byte = 0xB0
	pyKindMask byte = 0xF0
)

// maxPythonDepth bounds the frame stack. Deeper openers are ignored, which
// leaves the scanner in code, where comments are still found.
const maxPythonDepth = 64

func isPyString(frame byte) bool { return frame&pyKindMask == pyFrame }

func pyTop(stack []byte) byte {
	if len(stack) == 0 {
		return 0
	}

	return stack[len(stack)-1]
}

func pyPush(stack []byte, frame byte) []byte {
	if len(stack) >= maxPythonDepth {
		return stack
	}

	return append(stack, frame)
}

// scanPython is scanSegment for Python. It follows strings with their
// prefixes, and the replacement fields of f-strings (and t-strings), whose
// expressions may hold comments, nested strings reusing the outer quote, and
// line breaks.
func scanPython(line string, state stringState) (int, stringState, bool) {
	stack := []byte(state)

	for i := 0; i < len(line); i++ {
		top := pyTop(stack)

		switch {
		case isPyString(top):
			i, stack = scanPythonString(line, i, stack, top)
		case top == pySpec:
			stack = scanPythonSpec(line[i], stack)
		default:
			c := line[i]

			switch {
			case c == '#':
				idx, _, unspaced := hashComment(line, i, stateCode)

				return idx, stringState(stack), unspaced
			case c == '\\':
				i++
			case c == '\'' || c == '"':
				i, stack = openPythonString(line, i, stack)
			case top == 0:
			case c == '(' || c == '[' || c == '{':
				stack = pyPush(stack, pyBracket)
			case c == ')' || c == ']' || c == '}':
				if top == pyBracket || (top == pyField && c == '}') {
					stack = stack[:len(stack)-1]
				}
			case c == ':' && top == pyField:
				stack = pyPush(stack, pySpec)
			}
		}
	}

	return -1, stringState(stack), false
}

// openPythonString pushes the string frame for the quote at line[i] and
// returns the index of its last opening byte.
func openPythonString(line string, i int, stack []byte) (int, []byte) {
	q := line[i]

	frame := pyFrame
	if q == '"' {
		frame |= pyDouble
	}

	if pythonFStringPrefix(line, i) {
		frame |= pyFString
	}

	if hasTripleQuote(line, i, q) {
		frame |= pyTriple
		i += tripleQuoteTail
	}

	return i, pyPush(stack, frame)
}

// pythonFStringPrefix reports whether the identifier right before the quote
// at line[i] is a string prefix that makes it an f-string or t-string.
func pythonFStringPrefix(line string, i int) bool {
	j := i
	for j > 0 && isIdentByte(line[j-1]) {
		j--
	}

	prefix := strings.ToLower(line[j:i])
	if len(prefix) > tripleQuoteTail || strings.Trim(prefix, "rbuft") != "" {
		return false
	}

	return strings.ContainsAny(prefix, "ft")
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' ||
		c >= 'A' && c <= 'Z' || c >= 0x80
}

// scanPythonString advances over line[i] inside the string frame top.
func scanPythonString(line string, i int, stack []byte, top byte) (int, []byte) {
	c := line[i]

	q := byte('\'')
	if top&pyDouble != 0 {
		q = '"'
	}

	hasNext := i+1 < len(line)

	switch {
	case c == '\\':
		if hasNext && line[i+1] != '{' && line[i+1] != '}' {
			return i + 1, stack
		}
	case top&pyTriple != 0 && hasTripleQuote(line, i, q):
		return i + tripleQuoteTail, stack[:len(stack)-1]
	case top&pyTriple == 0 && c == q:
		return i, stack[:len(stack)-1]
	case top&pyFString != 0 && (c == '{' || c == '}'):
		if hasNext && line[i+1] == c {
			return i + 1, stack
		}

		if c == '{' {
			return i, pyPush(stack, pyField)
		}
	}

	return i, stack
}

// scanPythonSpec handles byte c of a replacement field's format spec, which
// may nest fields and ends with the field's closing brace.
func scanPythonSpec(c byte, stack []byte) []byte {
	switch c {
	case '{':
		return pyPush(stack, pyField)
	case '}':
		stack = stack[:len(stack)-1]
		if pyTop(stack) == pyField {
			stack = stack[:len(stack)-1]
		}
	}

	return stack
}

// endPythonLine drops single-quoted string text left open at a line break,
// which only a backslash continuation allows. Triple-quoted strings and
// replacement fields carry on to the next line.
func endPythonLine(state stringState) stringState {
	stack := []byte(state)
	for len(stack) > 0 && isPyString(pyTop(stack)) && pyTop(stack)&pyTriple == 0 {
		stack = stack[:len(stack)-1]
	}

	return stringState(stack)
}

// commentScan is where scanning a Write or Edit payload starts: the language
// syntax, the multi-line string state the first line opens in, text from the
// file that precedes the payload on its first line, and whether triple-quoted
// state is dropped at each line break because the payload's lines are not
// contiguous in the file.
type commentScan struct {
	syntax          langSyntax
	start           stringState
	prefix          string
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
// Write starts in code. An Edit's new_string continues the line of its
// old_string in the file on disk, from the string state that line starts in,
// so a fragment that begins inside (or closes) a docstring or inside a comment
// is scanned correctly; see editStart for several matches. Only languages
// with triple-quoted strings, and extension-less files that may hold a Python
// shebang, read the file; CRLF line endings are matched as LF. An Edit with no
// old_string joins added lines from several patch hunks whose boundaries are
// lost, so triple-quoted state is not carried between its lines.
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

	if hookCtx.ToolInput.OldString == "" {
		scan.lineLocalTriple = true

		if !detectShebang {
			return scan
		}
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
		return scan
	}

	scan.start, scan.prefix = editStart(original, old, scan.syntax)

	return scan
}

// pythonShebang matches a first line that runs the file with Python.
var pythonShebang = regexp.MustCompile(`^#!.*\bpython[0-9.]*(\s|$)`)

// shebangSyntax returns the Python syntax when text starts with a Python
// shebang, for scripts without an extension, and the default syntax otherwise.
func shebangSyntax(text string) langSyntax {
	firstLine, _, _ := strings.Cut(text, "\n")
	if pythonShebang.MatchString(firstLine) {
		return pythonSyntax
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

// maxStartStateOccurrences bounds the old_string matches editStart checks;
// each rescans its line, so many matches on a long line are quadratic.
const maxStartStateOccurrences = 32

// Stand-in prefixes for an Edit whose matches sit on different lines but in
// the same kind of spot: inside a reported comment, or after an unspaced one.
const (
	commentPrefix  = "# "
	unspacedPrefix = "_#"
)

// editLead is the state and first-line prefix an Edit's new_string is
// scanned from.
type editLead struct {
	state  stringState
	prefix string
}

// editStart returns the state and line prefix an Edit's new_string continues
// from. When every occurrence of old in content has the same line start state
// and prefix, those are used as they are. Otherwise each occurrence is reduced
// to the state at its position plus a stand-in prefix for a comment, and those
// must agree. With no match, too many, or disagreement it starts in code.
func editStart(content, old string, syntax langSyntax) (stringState, string) {
	lines := strings.Split(content, "\n")
	lineStates := make([]stringState, len(lines))
	lineOffsets := make([]int, len(lines))

	state, offset := stateCode, 0
	for i, line := range lines {
		lineStates[i], lineOffsets[i] = state, offset
		_, state = findCommentStart(line, state, syntax)
		offset += len(line) + 1
	}

	var exact, reduced []editLead

	for from := 0; ; {
		rel := strings.Index(content[from:], old)
		if rel < 0 {
			break
		}

		if len(exact) == maxStartStateOccurrences {
			return stateCode, ""
		}

		pos := from + rel
		li := sort.SearchInts(lineOffsets, pos+1) - 1
		prefix := content[lineOffsets[li]:pos]
		exact = append(exact, editLead{state: lineStates[li], prefix: prefix})

		idx, at, unspaced := scanSegment(prefix, lineStates[li], syntax)

		lead := editLead{state: at}

		switch {
		case idx >= 0:
			lead.prefix = commentPrefix
		case unspaced:
			lead.prefix = unspacedPrefix
		}

		reduced = append(reduced, lead)
		from = pos + len(old)
	}

	for _, leads := range [][]editLead{exact, reduced} {
		if lead, ok := sharedLead(leads); ok {
			return lead.state, lead.prefix
		}
	}

	return stateCode, ""
}

// sharedLead returns the lead every entry of leads has, if any.
func sharedLead(leads []editLead) (editLead, bool) {
	if len(leads) == 0 {
		return editLead{}, false
	}

	for _, lead := range leads[1:] {
		if lead != leads[0] {
			return editLead{}, false
		}
	}

	return leads[0], true
}
