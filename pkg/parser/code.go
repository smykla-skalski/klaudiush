package parser

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	// quotedLiteral matches a double, single or backtick quoted string.
	quotedLiteral = regexp.MustCompile(
		`"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'|` + "`((?:[^`\\\\]|\\\\.)*)`",
	)
	// listLiteral matches quoted strings in brackets or parentheses: an argv
	// array or tuple, or the separate arguments of system("git", "commit").
	listLiteral = regexp.MustCompile(`[\[(]((?:\s*(?:"[^"]*"|'[^']*')\s*,?)+)[\])]`)
	// listItem matches one quoted string inside a list literal.
	listItem = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	// programThenList matches a program name followed by its argument list,
	// the form spawn("git", ["commit"]) and execFile take.
	programThenList = regexp.MustCompile(
		`["']([^"']+)["']\s*,\s*\[((?:\s*(?:"[^"]*"|'[^']*')\s*,?)*)\]`,
	)
	// quotedExec matches Perl qx{} and Ruby %x() command strings.
	quotedExec = regexp.MustCompile(`(?:qx|%x)\s*[{(\[]([^})\]]*)[})\]]`)
	// mentionsCommand keeps only candidates that name something tracked.
	mentionsCommand = regexp.MustCompile(
		`(^|[^\w.-])(git|gh|hub|git-[\w-]+|sh|bash|zsh)([^\w.-]|$)`,
	)
	// literalEscapes undoes the escapes common to these languages.
	literalEscapes = strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\'`, `'`, "\\`", "`", `\n`, "\n")
)

// proseUnsafe matches code where a message may still run or an unknown git
// word may run something klaudiush cannot see: git config, config files, the
// variables that move or extend the configuration git reads, a directory
// change to another repository, a PATH that finds other git commands, or
// output piped or redirected into a program (print to a process's stdin, a
// replaced stdout, dup2 onto a pipe after fork) or captured as a value (print
// or a log stream into a buffer, redirect_stdout). Code that can run a shell
// string at all (os.system, os.popen, shell=True, execSync, exec, shlex) or
// rebind names dynamically (setattr, builtins, globals) reads no prose: any
// message it builds could reach that call. HOME and PATH count only as a key
// or assignment, since code reads them and messages name them ("not found on
// PATH"). Argv-list calls (Popen, execFileSync, create_subprocess_exec) with
// cwd, stdout or stdin options, reading a result's stdout, and printing to
// stderr do not count.
var proseUnsafe = regexp.MustCompile(
	`(?i)os\.system|os\.popen|shell\s*[=:]\s*true|getoutput|execsync|os\.exec|\bexec\s*\(|` +
		`\beval\s*\(|shlex|spawn|setattr|builtins|globals\s*\(|locals\s*\(|__dict__|` +
		`\bvars\s*\(|__import__|importlib|putenv|environ\.setdefault|create_subprocess_shell|` +
		`alias\.|\[alias|\[include|include(?:if)?\.|gitconfig|git/config|` +
		`GIT_CONFIG|GIT_DIR|GIT_COMMON_DIR|GIT_WORK_TREE|GIT_EXEC_PATH|XDG_CONFIG_HOME|` +
		`chdir|open3|\$stdout\s*=|\bsys\.stdout\s*=[^=]|` +
		`\bstd(?:out|err)\.write\s*=[^=]|` +
		`dup2|\bfork\b|\bpipe\s*\(|fdopen|redirect_std|StringIO|BytesIO|` +
		`\b(?:file|stream)\s*=\s*(?:[^s\s]|s[^ty]|sy[^s]|st[^d])|` +
		`\|\s*["'\x60]|["'\x60]\s*\||` +
		`(?-i:\b(?:HOME|PATH)\b["'\]]*\s*[:=][^=]|\.PATH\s*=[^=]|\{PATH\})`,
)

// unsafeWords are the plain proseUnsafe alternatives, lower case.
var unsafeWords = strings.Fields(
	"os.system os.popen getoutput execsync os.exec shlex spawn setattr builtins __dict__ " +
		"__import__ importlib putenv environ.setdefault create_subprocess_shell alias. [alias " +
		"[include include. includeif. gitconfig git/config git_config git_dir git_common_dir " +
		"git_work_tree git_exec_path xdg_config_home chdir open3 dup2 fdopen redirect_std " +
		"stringio bytesio",
)

// unsafePattern is a proseUnsafe alternative with a literal prefix, so the
// regexp engine jumps between occurrences of it. wordStart stands in for a
// leading \b, which would hide that prefix from the engine.
type unsafePattern struct {
	re        *regexp.Regexp
	wordStart bool
	exactCase bool
}

// unsafePatterns are the remaining proseUnsafe alternatives. Unless exactCase
// is set they run on lowered code.
var unsafePatterns = []unsafePattern{
	{re: regexp.MustCompile(`shell\s*[=:]\s*true`)},
	{re: regexp.MustCompile(`exec\s*\(`), wordStart: true},
	{re: regexp.MustCompile(`eval\s*\(`), wordStart: true},
	{re: regexp.MustCompile(`globals\s*\(`)},
	{re: regexp.MustCompile(`locals\s*\(`)},
	{re: regexp.MustCompile(`vars\s*\(`), wordStart: true},
	{re: regexp.MustCompile(`\$stdout\s*=`)},
	{re: regexp.MustCompile(`sys\.stdout\s*=[^=]`), wordStart: true},
	{re: regexp.MustCompile(`std(?:out|err)\.write\s*=[^=]`), wordStart: true},
	{re: regexp.MustCompile(`fork\b`), wordStart: true},
	{re: regexp.MustCompile(`pipe\s*\(`), wordStart: true},
	{re: regexp.MustCompile(`file\s*=\s*(?:[^s\s]|s[^ty]|sy[^s]|st[^d])`), wordStart: true},
	{re: regexp.MustCompile(`stream\s*=\s*(?:[^s\s]|s[^ty]|sy[^s]|st[^d])`), wordStart: true},
	{re: regexp.MustCompile(`HOME\b["'\]]*\s*[:=][^=]`), wordStart: true, exactCase: true},
	{re: regexp.MustCompile(`PATH\b["'\]]*\s*[:=][^=]`), wordStart: true, exactCase: true},
	{re: regexp.MustCompile(`\.PATH\s*=[^=]`), exactCase: true},
	{re: regexp.MustCompile(`\{PATH\}`), exactCase: true},
}

// codeUnsafe reports whether proseUnsafe matches code. One regexp with this
// many case-folded alternatives costs about a microsecond per byte, so ASCII
// code is checked by substring search and prefixed patterns instead, giving
// the same answer far faster on a large script. Other code, where case
// folding reaches beyond ASCII, keeps the regexp.
func codeUnsafe(code string) bool {
	if !isASCII(code) {
		return proseUnsafe.MatchString(code)
	}

	lower := strings.ToLower(code)

	for _, word := range unsafeWords {
		if strings.Contains(lower, word) {
			return true
		}
	}

	for _, p := range unsafePatterns {
		text := lower
		if p.exactCase {
			text = code
		}

		if patternAt(text, p) {
			return true
		}
	}

	return quotePipe(code)
}

// patternAt reports whether p matches text, at a word start when p asks.
func patternAt(text string, p unsafePattern) bool {
	if !p.wordStart {
		return p.re.MatchString(text)
	}

	for start := 0; start < len(text); {
		loc := p.re.FindStringIndex(text[start:])
		if loc == nil {
			return false
		}

		at := start + loc[0]
		if at == 0 || !isWordByte(text[at-1]) {
			return true
		}

		// A rejected match may overlap the next one (0PATH=PATH=x).
		start = at + 1
	}

	return false
}

// quotePipe reports whether a pipe character sits next to a quote, with only
// whitespace between: a shell pipeline built in a string.
func quotePipe(code string) bool {
	for i := range len(code) {
		if code[i] != '|' {
			continue
		}

		before := strings.TrimRight(code[:i], regexSpace)
		after := strings.TrimLeft(code[i+1:], regexSpace)

		if before != "" && strings.ContainsRune(quoteBytes, rune(before[len(before)-1])) ||
			after != "" && strings.ContainsRune(quoteBytes, rune(after[0])) {
			return true
		}
	}

	return false
}

// regexSpace is what \s matches in a Go regexp.
const regexSpace = "\t\n\f\r "

// quoteBytes are the quote characters a string literal opens with.
const quoteBytes = "\"'`"

// isASCII reports whether s holds only ASCII bytes.
func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}

	return true
}

// isWordByte reports whether b is an ASCII word character, as \w matches.
func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// messageCallNames name calls that show their argument to a person: printing,
// logging, failing and exiting. Any other call may run it, so the list is
// closed: an exec function or wrapper missing from it fails closed.
const messageCallNames = "print println printf eprint eprintln puts fail die warn warning " +
	"error info debug critical exception log notice exit abort echo alert"

// messageCalls is messageCallNames as a set.
var messageCalls = nameSet(messageCallNames)

// messageAlternation matches any one of messageCallNames as a whole word.
var messageAlternation = `\b(` + strings.ReplaceAll(messageCallNames, " ", "|") + `)\b`

// messageRebound matches code that gives a message-call name another meaning:
// an import (from os import system as echo), an assignment (print =
// os.system, fail: object = run, warn := os.system), a JavaScript function
// or binding, or a destructured name ({execSync: log}).
var messageRebound = regexp.MustCompile(
	`(?:\bimport\b[^\n;]*|\bas\s+|\bfunction\s*\*?\s*|\b(?:const|let|var)\s+|[{,][ \t]*|` +
		`\{[^{}]*:[ \t]*|\blambda\b[^:\n]*|\bfor\b[^\n:]*)` + messageAlternation + `|` +
		messageAlternation + `\s*(?::[^=\n;]*)?=[^=>]`,
)

// pythonDef matches a Python def line, capturing its indent and name.
var pythonDef = regexp.MustCompile(`(?m)^([ \t]*)(?:async[ \t]+)?def[ \t]+([A-Za-z_]\w*)[ \t]*\(`)

// runsCommands matches code that may run a command line.
var runsCommands = regexp.MustCompile(
	`system|popen|subprocess|\bexec|spawn|eval|getoutput|check_output|\brun\s*\(|\bcall\s*\(`,
)

// untrustedMessages returns the message-call names the code redefines in a
// way that may run their argument: rebound, imported or destructured, or a
// Python def whose body may run commands. A def that only prints and exits
// (fail writing to stderr) keeps its name trusted.
func untrustedMessages(code string) map[string]bool {
	untrusted := make(map[string]bool)

	for _, m := range messageRebound.FindAllStringSubmatch(code, -1) {
		untrusted[m[1]+m[2]] = true
	}

	for _, m := range pythonDef.FindAllStringSubmatchIndex(code, -1) {
		name := code[m[4]:m[5]]
		if messageCalls[name] && runsCommands.MatchString(defBody(code, m[1], m[3]-m[2])) {
			untrusted[name] = true
		}
	}

	return untrusted
}

// defBody returns the lines after a def header at headerEnd that are indented
// deeper than indent columns, or blank: the body of that def.
func defBody(code string, headerEnd, indent int) string {
	rest := code[headerEnd:]

	_, body, found := strings.Cut(rest, "\n")
	if !found {
		return rest
	}

	end := 0

	for line := range strings.Lines(body) {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.TrimSpace(line) != "" && len(line)-len(trimmed) <= indent {
			break
		}

		end += len(line)
	}

	return rest[:len(rest)-len(body)+end]
}

// raisedError matches an exception or error raised or thrown right where it
// is built (raise ValueError(, throw new Error(), whose message is shown
// rather than handed on as a value.
var raisedError = regexp.MustCompile(
	`(?:^|[^\w.])(?:raise|throw\s+new|throw)\s+(?:[A-Za-z_$][\w$]*\.)*` +
		`[\w$]*(?:Error|Exception|Warning|Exit)\s*$`,
)

// streamWrite matches a write to the process's own stderr or stdout
// (sys.stderr.write, process.stderr.write), which shows its argument.
var streamWrite = regexp.MustCompile(`(?:^|[^\w.])(?:sys|process)\.std(?:err|out)\.write$`)

// trustedReceivers are the objects whose message-named methods show their
// argument: loggers, the console and the stdlib. A method on another object,
// such as a helper module klaudiush does not follow, may do anything.
var trustedReceivers = nameSet("logging log logger LOGGER LOG console warnings sys self cls click")

// messageReceiver matches the object a method call is made on (log in
// log.info), when the call is a method call.
var messageReceiver = regexp.MustCompile(`([A-Za-z_$][\w$]*)\.[A-Za-z_$][\w$]*$`)

// trailingName matches the identifier that ends a piece of code.
var trailingName = regexp.MustCompile(`([A-Za-z_$][\w$]*)$`)

// docstringOwner matches the line a Python docstring follows: a def or class.
var docstringOwner = regexp.MustCompile(`^\s*(?:async\s+)?(?:def|class)\b`)

// maxStringPrefix is the longest string prefix before a quote (rb, f, u).
const maxStringPrefix = 2

// stringPrefixes are the letters a string prefix is made of.
const stringPrefixes = "rRbBuUfF"

// docRead matches code that reads docstrings back as values.
var docRead = regexp.MustCompile(`__doc__|getdoc|get_docstring`)

// errorCaught matches code that catches an error into a name or reads the
// current one back, so its message can be handed on as a value.
var errorCaught = regexp.MustCompile(
	`\bexcept\b[^:]*\bas\s|\bcatch\s*\(|exc_info|format_exc|format_exception|\.args\b|` +
		`sys\.exception|excepthook|uncaughtException|unhandledRejection|\.then\s*\(`,
)

// textReuse says which kinds of prose the code reads back as values, and
// which message-call names it gave another meaning. Each scans the whole
// source, so it runs on first use: most literals are ruled out before.
type textReuse struct {
	docs      func() bool
	errors    func() bool
	untrusted func() map[string]bool
}

// newTextReuse returns the textReuse of code.
func newTextReuse(code string) textReuse {
	return textReuse{
		docs:      sync.OnceValue(func() bool { return docRead.MatchString(code) }),
		errors:    sync.OnceValue(func() bool { return errorCaught.MatchString(code) }),
		untrusted: sync.OnceValue(func() map[string]bool { return untrustedMessages(code) }),
	}
}

// proseLiteral reports whether the string literal opening at start in code is
// text rather than a command line: a docstring at the top of a module, def
// or class, or the first argument of a message call (print, fail, raise
// ValueError). A string anywhere else may run, and a backtick string runs in
// Ruby and Perl, so neither is prose; nor is one that interpolates a command
// or expression (`...`, #{...}, @{[...]}, $(...)). Docstrings and error
// messages the code reads back (__doc__, except ... as e) are values too.
func proseLiteral(code string, start, end int, reuse textReuse) bool {
	if code[start] == '`' || strings.Contains(code[start+1:end], "`") ||
		strings.Contains(code[start:end], "#{") || strings.Contains(code[start:end], "@{") ||
		strings.Contains(code[start:end], "$(") {
		return false
	}

	before := code[:start]

	triple := strings.HasSuffix(before, `""`) || strings.HasSuffix(before, `''`)
	if triple {
		before = before[:len(before)-2]
	}

	for range maxStringPrefix {
		if before != "" && strings.IndexByte(stringPrefixes, before[len(before)-1]) >= 0 {
			before = before[:len(before)-1]
		}
	}

	prev := significantCode(before)

	if triple && prev == "" {
		return !reuse.docs()
	}

	if triple && strings.HasSuffix(prev, ":") {
		header := prev[strings.LastIndexByte(prev, '\n')+1:]

		return !reuse.docs() && docstringOwner.MatchString(header) && bracketsClosed(header) &&
			strings.Contains(before[len(prev):], "\n")
	}

	callee, isCall := strings.CutSuffix(prev, "(")
	callee = strings.TrimRight(callee[strings.LastIndexByte(callee, '\n')+1:], " \t")

	if !isCall || strings.ContainsAny(callee, "#/") {
		return false
	}

	name := trailingName.FindString(callee)
	receiver := messageReceiver.FindStringSubmatch(callee)
	trustedCall := receiver == nil || trustedReceivers[receiver[1]]

	return messageCalls[name] && !reuse.untrusted()[name] && trustedCall ||
		streamWrite.MatchString(callee) ||
		!reuse.errors() && raisedError.MatchString(callee)
}

// bracketsClosed reports whether every bracket opened in line is closed in
// it, so a colon at its end ends a def or class header rather than a dict key
// in a default argument.
func bracketsClosed(line string) bool {
	return strings.Count(line, "(") == strings.Count(line, ")") &&
		strings.Count(line, "[") == strings.Count(line, "]") &&
		strings.Count(line, "{") == strings.Count(line, "}")
}

// maxCommentSkip bounds how many comment lines significantCode skips, so a
// long comment block costs each literal a fixed amount of work. Past it the
// comment itself is returned, which no prose rule accepts.
const maxCommentSkip = 32

// significantCode trims trailing whitespace and whole comment lines (a
// shebang included) from code, leaving what last precedes a literal.
func significantCode(code string) string {
	for range maxCommentSkip {
		code = strings.TrimRight(code, " \t\r\n")

		lineStart := strings.LastIndexByte(code, '\n') + 1
		if !strings.HasPrefix(strings.TrimLeft(code[lineStart:], " \t"), "#") {
			return code
		}

		code = code[:lineStart]
	}

	return code
}

// codeLine is a command line found in program source. prose marks a plain
// string literal, which is mostly messages and docs rather than commands.
type codeLine struct {
	text  string
	prose bool
}

// commandLines returns the command lines program source may run: its string
// literals, argv-style lists, program-and-list calls and Perl or Ruby command
// strings. Only candidates naming git, gh or a shell are kept. Unless plain
// is set, only backtick strings are taken from the literals, since nothing
// in the code can run any other string. A literal counts as prose only where
// it is text (see proseLiteral) and the code cannot change git
// configuration, which would make an unknown git word run.
func commandLines(code string, plain bool) []codeLine {
	literals := quotedLiteral.FindAllStringSubmatchIndex(code, -1)
	lists := listLiteral.FindAllStringSubmatch(code, -1)
	calls := programThenList.FindAllStringSubmatch(code, -1)
	execs := quotedExec.FindAllStringSubmatch(code, -1)
	lines := make([]codeLine, 0, len(literals)+len(lists)+len(calls)+len(execs))
	unsafe := sync.OnceValue(func() bool { return codeUnsafe(code) })
	reuse := newTextReuse(code)

	for _, m := range literals {
		if !plain && code[m[0]] != '`' {
			continue
		}

		text := literalEscapes.Replace(submatchText(code, m))
		if !mentionsCommand.MatchString(text) {
			continue
		}

		lines = append(lines, codeLine{
			text:  text,
			prose: proseLiteral(code, m[0], m[1], reuse) && !unsafe(),
		})
	}

	for _, m := range lists {
		lines = append(lines, codeLine{text: joinListItems(m[1])})
	}

	for _, m := range calls {
		lines = append(lines, codeLine{text: shellQuote(m[1]) + " " + joinListItems(m[2])})
	}

	for _, m := range execs {
		lines = append(lines, codeLine{text: m[1]})
	}

	return slices.DeleteFunc(lines, func(line codeLine) bool {
		return !mentionsCommand.MatchString(line.text)
	})
}

// joinListItems turns the quoted items of a list literal into a command line.
func joinListItems(list string) string {
	matches := listItem.FindAllStringSubmatch(list, -1)
	items := make([]string, 0, len(matches))

	for _, match := range matches {
		items = append(items, match[1]+match[2])
	}

	return quoteArgs(items)
}

// interpreterShebang reports whether a script's shebang names a language
// interpreter rather than a shell.
func interpreterShebang(text string) bool {
	line, _, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(line, "#!") {
		return false
	}

	for word := range strings.FieldsSeq(strings.TrimPrefix(line, "#!")) {
		if _, ok := interpreters[commandName(word)]; ok {
			return true
		}
	}

	return false
}

// shellQuote wraps s in single quotes so the shell reads it as one word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// submatchText joins the captured groups of an index match, the one quote
// style that matched.
func submatchText(code string, match []int) string {
	var text strings.Builder

	for i := 2; i+1 < len(match); i += 2 {
		if match[i] >= 0 {
			text.WriteString(code[match[i]:match[i+1]])
		}
	}

	return text.String()
}

// quoteArgs quotes each argument and joins them into one command line tail.
func quoteArgs(args []string) string {
	quoted := make([]string, 0, len(args))

	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}

	return strings.Join(quoted, " ")
}
