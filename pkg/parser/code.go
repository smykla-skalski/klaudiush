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
	// listLiteral matches a list of quoted strings, such as an argv array.
	listLiteral = regexp.MustCompile(`\[((?:\s*(?:"[^"]*"|'[^']*')\s*,?)+)\]`)
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

// commandLines returns the command lines program source may run: its string
// literals, argv-style lists, program-and-list calls and Perl or Ruby command
// strings. Only candidates naming git, gh or a shell are kept.
func commandLines(code string) []string {
	literals := quotedLiteral.FindAllStringSubmatch(code, -1)
	lists := listLiteral.FindAllStringSubmatch(code, -1)
	calls := programThenList.FindAllStringSubmatch(code, -1)
	execs := quotedExec.FindAllStringSubmatch(code, -1)
	lines := make([]string, 0, len(literals)+len(lists)+len(calls)+len(execs))

	for _, m := range literals {
		lines = append(lines, literalEscapes.Replace(m[1]+m[2]+m[3]))
	}

	for _, m := range lists {
		lines = append(lines, joinListItems(m[1]))
	}

	for _, m := range calls {
		lines = append(lines, shellQuote(m[1])+" "+joinListItems(m[2]))
	}

	for _, m := range execs {
		lines = append(lines, m[1])
	}

	return slices.DeleteFunc(lines, func(line string) bool {
		return !mentionsCommand.MatchString(line)
	})
}

// joinListItems turns the quoted items of a list literal into a command line.
func joinListItems(list string) string {
	items := listItem.FindAllStringSubmatch(list, -1)
	words := make([]string, 0, len(items))

	for _, item := range items {
		words = append(words, shellQuote(item[1]+item[2]))
	}

	return strings.Join(words, " ")
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
