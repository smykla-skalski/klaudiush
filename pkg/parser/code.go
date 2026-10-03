package parser

import (
	"regexp"
	"slices"
	"strings"
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
// output piped or redirected into a program (popen, a process's stdin, a
// replaced stdout, dup2 onto a pipe after fork) or captured as a value (print
// or a log stream into a buffer, redirect_stdout). PATH counts only as a key
// or assignment, since messages name it ("not found on PATH"). Reading a
// result's stdout or printing to sys.stderr does not count.
var proseUnsafe = regexp.MustCompile(
	`(?i)alias\.|\[alias|\[include|include(?:if)?\.|gitconfig|git/config|` +
		`GIT_CONFIG|GIT_DIR|GIT_COMMON_DIR|GIT_WORK_TREE|GIT_EXEC_PATH|XDG_CONFIG_HOME|` +
		`chdir|\bcwd\b|popen|open3|\bstdin\b|\$stdout\s*=|\bstdout\s*=[^=]|` +
		`dup2|\bfork\b|\bpipe\s*\(|fdopen|redirect_std|StringIO|BytesIO|` +
		`\b(?:file|stream)\s*=\s*(?:[^s\s]|s[^y])|` +
		`\|\s*["'\x60]|["'\x60]\s*\||` +
		`(?-i:\bHOME\b|["']PATH["']|\bPATH\s*=|\.PATH\b|\{PATH\})`,
)

// messageCalls name calls that show their argument to a person: printing,
// logging, failing and exiting. Any other call may run it, so the list is
// closed: an exec function or wrapper missing from it fails closed.
var messageCalls = nameSet(
	"print println printf eprint eprintln puts fail die warn warning error info debug " +
		"critical exception log notice exit abort echo alert",
)

// raisedError matches an exception or error raised or thrown right where it
// is built (raise ValueError(, throw new Error(), whose message is shown
// rather than handed on as a value.
var raisedError = regexp.MustCompile(
	`(?:^|[^\w.])(?:raise|throw\s+new|throw)\s+(?:[A-Za-z_$][\w$]*\.)*` +
		`[\w$]*(?:Error|Exception|Warning)\s*$`,
)

// trailingName matches the identifier that ends a piece of code.
var trailingName = regexp.MustCompile(`([A-Za-z_$][\w$]*)$`)

// docstringOwner matches the line a Python docstring follows: a def or class.
var docstringOwner = regexp.MustCompile(`^\s*(?:async\s+)?(?:def|class)\b`)

// maxStringPrefix is the longest string prefix before a quote (rb, f, u).
const maxStringPrefix = 2

// docRead matches code that reads docstrings back as values.
var docRead = regexp.MustCompile(`__doc__|getdoc`)

// errorCaught matches code that catches an error into a name or reads the
// current one back, so its message can be handed on as a value.
var errorCaught = regexp.MustCompile(
	`\bexcept\b[^:]*\bas\s|\bcatch\s*\(|exc_info|format_exc|format_exception|\.args\b|` +
		`sys\.exception|excepthook|uncaughtException|unhandledRejection|\.then\s*\(`,
)

// textReuse says which kinds of prose the code reads back as values.
type textReuse struct {
	docs   bool
	errors bool
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
		if trimmed := strings.TrimRight(before, "rRbBuUfF"); len(before)-len(trimmed) <= 1 {
			before = trimmed
		}
	}

	prev := significantCode(before)

	if triple && prev == "" {
		return !reuse.docs
	}

	if triple && strings.HasSuffix(prev, ":") {
		header := prev[strings.LastIndexByte(prev, '\n')+1:]

		return !reuse.docs && docstringOwner.MatchString(header) && bracketsClosed(header) &&
			strings.Contains(before[len(prev):], "\n")
	}

	callee, isCall := strings.CutSuffix(prev, "(")
	if !isCall || strings.ContainsAny(callee[strings.LastIndexByte(callee, '\n')+1:], "#/") {
		return false
	}

	callee = strings.TrimRight(callee, " \t")

	return messageCalls[trailingName.FindString(callee)] ||
		!reuse.errors && raisedError.MatchString(callee)
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
// strings. Only candidates naming git, gh or a shell are kept. A literal
// counts as prose only where it is text (see proseLiteral) and the code
// cannot change git configuration, which would make an unknown git word run.
func commandLines(code string) []codeLine {
	literals := quotedLiteral.FindAllStringSubmatchIndex(code, -1)
	lists := listLiteral.FindAllStringSubmatch(code, -1)
	calls := programThenList.FindAllStringSubmatch(code, -1)
	execs := quotedExec.FindAllStringSubmatch(code, -1)
	lines := make([]codeLine, 0, len(literals)+len(lists)+len(calls)+len(execs))
	unsafe := proseUnsafe.MatchString(code)
	reuse := textReuse{docs: docRead.MatchString(code), errors: errorCaught.MatchString(code)}

	for _, m := range literals {
		text := literalEscapes.Replace(submatchText(code, m))
		if !mentionsCommand.MatchString(text) {
			continue
		}

		lines = append(lines, codeLine{
			text:  text,
			prose: !unsafe && proseLiteral(code, m[0], m[1], reuse),
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
