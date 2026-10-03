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

// gitConfigChange matches code that may make an unknown git word run
// something klaudiush cannot see: git config, config files, the variables
// that move or extend the configuration git reads, a directory change to
// another repository, or a PATH that finds other git commands. PATH counts
// only as a key or assignment, since messages name it ("not found on PATH").
var gitConfigChange = regexp.MustCompile(
	`(?i)alias\.|\[alias|\[include|include(?:if)?\.|gitconfig|git/config|` +
		`GIT_CONFIG|GIT_DIR|GIT_EXEC_PATH|XDG_CONFIG_HOME|chdir|\bcwd\b|` +
		`(?-i:\bHOME\b|["']PATH["']|\bPATH\s*=|\.PATH\b|\{PATH\})`,
)

// execCallName matches the name of a call that may run its argument as a
// command, its own or a wrapper's (check_output, getoutput, execSync).
var execCallName = regexp.MustCompile(
	`(?i)system|popen|output|spawn|exec|shell|command|cmd|script|invoke|process|eval`,
)

// execCallWords are words of a call name (run_git, checkCall) too short to
// match inside other words, and keywords whose operand may still run.
var execCallWords = nameSet(
	"run call sh bash zsh return yield await lambda assert if elif while for in and or not else",
)

// trailingName matches the identifier that ends a piece of code.
var trailingName = regexp.MustCompile(`([A-Za-z_$][\w$]*)$`)

// nameWord matches one word of a snake_case or camelCase name.
var nameWord = regexp.MustCompile(`[A-Z]?[a-z0-9]+|[A-Z]+`)

// shellPatternChars start a brace expansion or glob in a shell word.
const shellPatternChars = "{}[]*?"

// maxStringPrefix is the longest string prefix before a quote (rb, f, u).
const maxStringPrefix = 2

// proseLiteral reports whether the string literal opening at start in code is
// text rather than a command line: a docstring, or the first argument of a
// call that does not run commands (print, fail, console.error). A string
// handed to an exec call, assigned, returned or listed may run, so it is not.
func proseLiteral(code string, start int) bool {
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

	if triple && (prev == "" || strings.HasSuffix(prev, ":")) {
		return prev == "" || strings.Contains(before[len(prev):], "\n")
	}

	callee, isCall := strings.CutSuffix(prev, "(")
	if !isCall {
		return false
	}

	name := trailingName.FindString(strings.TrimRight(callee, " \t"))

	return name != "" && !runsCommands(name)
}

// runsCommands reports whether a call name may run its argument.
func runsCommands(name string) bool {
	if execCallName.MatchString(name) {
		return true
	}

	return slices.ContainsFunc(nameWord.FindAllString(name, -1), func(word string) bool {
		return execCallWords[strings.ToLower(word)]
	})
}

// significantCode trims trailing whitespace and whole comment lines (a
// shebang included) from code, leaving what last precedes a literal.
func significantCode(code string) string {
	for {
		code = strings.TrimRight(code, " \t\r\n")

		lineStart := strings.LastIndexByte(code, '\n') + 1
		if !strings.HasPrefix(strings.TrimLeft(code[lineStart:], " \t"), "#") {
			return code
		}

		code = code[:lineStart]
	}
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
	configChange := gitConfigChange.MatchString(code)

	for _, m := range literals {
		lines = append(lines, codeLine{
			text:  literalEscapes.Replace(submatchText(code, m)),
			prose: !configChange && proseLiteral(code, m[0]),
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
