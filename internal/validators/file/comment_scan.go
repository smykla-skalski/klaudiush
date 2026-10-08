package file

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
// that is not reported, and scanning of the line stops there. commentSlash
// accepts only "//", at line start or after whitespace, for languages where
// "#" starts code such as a Rust attribute or a C preprocessor directive.
type commentStyle uint8

const (
	commentLoose commentStyle = iota
	commentHash
	commentSlash
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

// slashCommentSyntax is the syntax of languages whose only line comment is
// "//" and that have no triple-quoted strings the scanner follows.
var slashCommentSyntax = langSyntax{comment: commentSlash}

// slashCommentExts lists extensions of languages whose only line comment is
// "//". In them "#" starts code: Rust attributes, C, C++, Objective-C, shader
// and C# preprocessor directives, F# and Swift compiler directives,
// JavaScript private members, CSS selectors and colours, and Vue slot
// shorthands. ".m" is read as Objective-C, not Octave.
var slashCommentExts = map[string]bool{
	".rs": true,
	".c":  true, ".h": true, ".cc": true, ".cpp": true, ".cxx": true, ".c++": true,
	".hh": true, ".hpp": true, ".hxx": true, ".h++": true, ".inl": true, ".ipp": true,
	".tpp": true, ".cppm": true, ".ixx": true, ".ino": true, ".m": true, ".mm": true,
	".cu": true, ".cuh": true, ".glsl": true, ".vert": true, ".frag": true, ".hlsl": true,
	".metal": true, ".fs": true, ".fsi": true, ".fsx": true, ".csx": true,
	".cs": true, ".swift": true, ".go": true, ".java": true, ".kt": true, ".kts": true,
	".scala": true, ".dart": true,
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
	".css": true, ".scss": true, ".sass": true, ".less": true,
	".vue": true, ".svelte": true, ".astro": true,
}

// langSyntaxForPath returns the comment and string syntax for path.
func langSyntaxForPath(path string) langSyntax {
	ext := strings.ToLower(filepath.Ext(path))
	if slashCommentExts[ext] {
		return slashCommentSyntax
	}

	return langSyntaxByExt[ext]
}

// followsFileStrings reports whether the language has triple-quoted strings,
// whose state at an Edit's position is read from the file on disk.
func (s langSyntax) followsFileStrings() bool {
	return s.python || s.double != tripleNone || s.single != tripleNone
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

// isCommentMarker reports whether a line-comment marker of style starts at
// line[i]: "//", or with commentLoose also "#". Markers must sit at line start
// or after whitespace.
func isCommentMarker(line string, i int, style commentStyle) bool {
	isHash := style == commentLoose && line[i] == '#'
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
		case isCommentMarker(line, i, syntax.comment):
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

// Python frames. A string frame is pyFrame plus pyDouble, pyTriple,
// pyFString and pyContinued flags; pyContinued marks a single-quoted string
// whose line ends in a backslash continuation. pyField is an f-string replacement field, pyBracket a
// bracket opened inside one, and pySpec its format spec. All have the high
// bit set so a Python state never equals a non-Python one.
const (
	pyFrame     byte = 0x80
	pyDouble    byte = 0x01
	pyTriple    byte = 0x02
	pyFString   byte = 0x04
	pyContinued byte = 0x08
	pyField     byte = 0x90
	pyBracket   byte = 0xA0
	pySpec      byte = 0xB0
	pyKindMask  byte = 0xF0
)

// maxPythonDepth bounds the frame stack. An opener past it replaces the
// stack with pyBroken for the rest of the scan: every "#" after whitespace is
// then reported, so nesting the scanner cannot follow never hides a comment.
const maxPythonDepth = 64

// pyBroken is the only frame left once the frame stack overflows.
const pyBroken byte = 0xC0

func isPyString(frame byte) bool { return frame&pyKindMask == pyFrame }

func pyTop(stack []byte) byte {
	if len(stack) == 0 {
		return 0
	}

	return stack[len(stack)-1]
}

func pyPush(stack []byte, frame byte) []byte {
	if len(stack) >= maxPythonDepth {
		return append(stack[:0], pyBroken)
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

		if line[i] == '#' && !isPyString(top) && top != pySpec {
			idx, _, unspaced := hashComment(line, i, stateCode)
			if idx >= 0 || top != pyBroken {
				return idx, stringState(stack), unspaced
			}

			continue
		}

		switch {
		case top == pyBroken:
		case isPyString(top):
			i, stack = scanPythonString(line, i, stack, top)
		case top == pySpec:
			stack = scanPythonSpec(line[i], stack)
		default:
			i, stack = scanPythonCode(line, i, stack, top)
		}
	}

	return -1, stringState(stack), false
}

// scanPythonCode advances over line[i], a byte of code or of a replacement
// field expression other than "#".
func scanPythonCode(line string, i int, stack []byte, top byte) (int, []byte) {
	c := line[i]

	switch {
	case c == '\\':
		return i + 1, stack
	case c == '\'' || c == '"':
		return openPythonString(line, i, stack)
	case top == 0:
	case c == '(' || c == '[' || c == '{':
		return i, pyPush(stack, pyBracket)
	case c == ')' || c == ']' || c == '}':
		if top == pyBracket || (top == pyField && c == '}') {
			return i, stack[:len(stack)-1]
		}
	case c == ':' && top == pyField:
		return i, pyPush(stack, pySpec)
	}

	return i, stack
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

	if top&pyContinued != 0 {
		top &^= pyContinued
		stack[len(stack)-1] = top
	}

	q := byte('\'')
	if top&pyDouble != 0 {
		q = '"'
	}

	hasNext := i+1 < len(line)

	switch {
	case c == '\\':
		if strings.TrimSuffix(line[i+1:], "\r") == "" {
			stack[len(stack)-1] |= pyContinued

			return len(line) - 1, stack
		}

		if line[i+1] != '{' && line[i+1] != '}' {
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

// endPythonLine drops single-quoted string text left open at a line break
// without a backslash continuation. Continued strings, triple-quoted strings
// and replacement fields carry on to the next line.
func endPythonLine(state stringState) stringState {
	stack := []byte(state)
	for len(stack) > 0 && isPyString(pyTop(stack)) && pyTop(stack)&pyTriple == 0 {
		if top := pyTop(stack); top&pyContinued != 0 {
			stack[len(stack)-1] = top &^ pyContinued

			break
		}

		stack = stack[:len(stack)-1]
	}

	return stringState(stack)
}

// commentScan is how a Write or Edit payload is scanned: the language
// syntax, the leads it is scanned from (none means once from code), and
// whether triple-quoted state is dropped at each line break because the
// payload's lines are not contiguous in the file.
type commentScan struct {
	syntax          langSyntax
	leads           []editLead
	lineLocalTriple bool
}

// editLead is one place an Edit's new_string lands: the multi-line string
// state its line starts in, the file text before it on that line, and the
// file text after it, only used to find the declaration a comment documents.
// before holds the file lines above it, starting in beforeState, only used to
// find a PEP 723 metadata block the Edit lands in. noMetadata is set when a
// block the Edit adds could be a second one: the file has a script block the
// replaced text does not touch, or several occurrences are replaced and not
// all of them inside the file's block.
type editLead struct {
	state       stringState
	prefix      string
	suffix      string
	before      []string
	beforeState stringState
	noMetadata  bool
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
// is scanned correctly; with several matches it is scanned from each. The
// file text after old_string lets a comment the fragment touches find the
// declaration it documents. Only languages with triple-quoted strings carry
// the file's string state; elsewhere block comments are not tracked, so a
// backtick inside one would open a string that hides every later comment,
// and the line starts in code. CRLF line endings are matched as LF. An Edit with no
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

	if detectShebang {
		if scan.syntax = shebangSyntax(hookCtx.ToolInput.NewString); scan.syntax.python {
			detectShebang = false
		}
	}

	if hookCtx.ToolInput.OldString == "" {
		scan.lineLocalTriple = true

		if !detectShebang {
			return scan
		}
	}

	data, ok := readRegularFile(hook.CanonicalFilePath(hookCtx.WorkingDir, path))
	if !ok || len(data) == 0 {
		return scan
	}

	original := strings.ReplaceAll(string(data), "\r\n", "\n")

	if detectShebang {
		scan.syntax = shebangSyntax(original)
	}

	old := strings.ReplaceAll(hookCtx.ToolInput.OldString, "\r\n", "\n")
	if old == "" {
		return scan
	}

	scan.leads = editLeads(original, old, scan.syntax, toolEdits(hookCtx)[0].ReplaceAll)

	if !scan.syntax.followsFileStrings() {
		for i := range scan.leads {
			scan.leads[i].state = stateCode
		}
	}

	return scan
}

// pythonShebang matches a first line that runs the file with Python, or
// with uv as a script, directly or through env.
var pythonShebang = regexp.MustCompile(
	`^#!.*\bpython[0-9.]*(\s|$)` +
		`|^#!\s*(\S*/)?(env\s+(-S\s+)?)?uv\s+run\s(.*\s)?--script(\s|$)`,
)

// shebangSyntax returns the Python syntax when text starts with a Python
// shebang, for scripts without an extension, and the default syntax otherwise.
func shebangSyntax(text string) langSyntax {
	firstLine, _, _ := strings.Cut(text, "\n")
	if pythonShebang.MatchString(firstLine) {
		return pythonSyntax
	}

	return langSyntax{}
}

// maxEditSourceBytes bounds the file an Edit reads to find its start state;
// a larger file is scanned from code.
const maxEditSourceBytes = 4 << 20

// readRegularFile reads path when it is a regular file of at most
// maxEditSourceBytes; a FIFO or device would block or never end.
func readRegularFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxEditSourceBytes {
		return nil, false
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, false
	}

	return data, true
}

// maxStartStateOccurrences bounds the old_string matches editLeads checks;
// each is scanned separately, so many matches would multiply the work.
const maxStartStateOccurrences = 32

// editLeads returns a lead for the first occurrence of old in content, or
// for every occurrence when all is set, or none (scan once from code) when
// there are none or too many.
func editLeads(content, old string, syntax langSyntax, all bool) []editLead {
	lines := strings.Split(content, "\n")
	lineStates := make([]stringState, len(lines))
	lineOffsets := make([]int, len(lines))
	topLevel := make([]bool, len(lines))

	state, offset := stateCode, 0

	for i, line := range lines {
		var idx int

		lineStates[i], lineOffsets[i] = state, offset
		idx, state = findCommentStart(line, state, syntax)
		topLevel[i] = idx == 0 && lineStates[i] == stateCode
		offset += len(line) + 1
	}

	metadataStart, metadataEnd := -1, -1

	if syntax.python {
		block := pep723Block(lines, topLevel)
		metadataStart, metadataEnd = slices.Index(block, true), lastMarked(block)
	}

	var leads []editLead

	for from := 0; ; {
		rel := strings.Index(content[from:], old)
		if rel < 0 {
			break
		}

		if len(leads) == maxStartStateOccurrences {
			return nil
		}

		pos := from + rel
		li := sort.SearchInts(lineOffsets, pos+1) - 1
		last := sort.SearchInts(lineOffsets, pos+len(old)) - 1
		from = pos + len(old)
		first := commentRunStart(lines, lineStates, max(0, li-maxDocContextLines))

		leads = append(leads, editLead{
			state:  lineStates[li],
			prefix: content[lineOffsets[li]:pos],
			suffix: throughCommentRun(
				firstLines(content[from:], maxDocContextLines),
				content[from:],
			),
			before:      lines[first:li],
			beforeState: lineStates[first],
			noMetadata: metadataStart >= 0 &&
				(last < metadataStart || li > metadataEnd),
		})

		if !all {
			break
		}
	}

	if len(leads) > 1 && (metadataStart < 0 || slices.ContainsFunc(leads, func(l editLead) bool {
		return l.noMetadata
	})) {
		for i := range leads {
			leads[i].noMetadata = true
		}
	}

	return leads
}

// commentRunStart moves first back to the start of the run of top-level
// "#" lines it sits in, so a PEP 723 block longer than the lookback above an
// Edit is still seen from its opening line.
func commentRunStart(lines []string, states []stringState, first int) int {
	for first > 0 && strings.HasPrefix(lines[first], "#") &&
		states[first-1] == stateCode && strings.HasPrefix(lines[first-1], "#") {
		first--
	}

	return first
}

// throughCommentRun extends head, a prefix of s ending at a line break,
// while the lines after it continue its run of "#" lines, so a PEP 723 block
// whose closing line is past the lookahead below an Edit is still closed.
func throughCommentRun(head, s string) string {
	end := len(head)

	lastStart := strings.LastIndexByte(strings.TrimSuffix(head, "\n"), '\n') + 1
	if end == len(s) || !strings.HasPrefix(head[lastStart:], "#") {
		return head
	}

	for end < len(s) && s[end] == '#' {
		next := strings.IndexByte(s[end:], '\n')
		if next < 0 {
			return s
		}

		end += next + 1
	}

	return s[:end]
}

// firstLines returns the first n lines of s.
func firstLines(s string, n int) string {
	end := 0

	for range n {
		next := strings.IndexByte(s[end:], '\n')
		if next < 0 {
			return s
		}

		end += next + 1
	}

	return s[:end]
}
