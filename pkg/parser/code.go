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

// shellPatternChars start a brace expansion or glob in a shell word.
const shellPatternChars = "{}[]*?"

// codeLine is a command line found in program source. prose marks a plain
// string literal, which is mostly messages and docs rather than commands.
type codeLine struct {
	text  string
	prose bool
}

// commandLines returns the command lines program source may run: its string
// literals, argv-style lists, program-and-list calls and Perl or Ruby command
// strings. Only candidates naming git, gh or a shell are kept. String
// literals count as prose unless the code may change git configuration,
// since an alias defined there would make an unknown git word run.
func commandLines(code string) []codeLine {
	literals := quotedLiteral.FindAllStringSubmatch(code, -1)
	lists := listLiteral.FindAllStringSubmatch(code, -1)
	calls := programThenList.FindAllStringSubmatch(code, -1)
	execs := quotedExec.FindAllStringSubmatch(code, -1)
	lines := make([]codeLine, 0, len(literals)+len(lists)+len(calls)+len(execs))
	prose := !gitConfigChange.MatchString(code)

	for _, m := range literals {
		lines = append(
			lines,
			codeLine{text: literalEscapes.Replace(m[1] + m[2] + m[3]), prose: prose},
		)
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

// quoteArgs quotes each argument and joins them into one command line tail.
func quoteArgs(args []string) string {
	quoted := make([]string, 0, len(args))

	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}

	return strings.Join(quoted, " ")
}
