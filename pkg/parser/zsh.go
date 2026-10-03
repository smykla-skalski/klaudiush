package parser

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"mvdan.cc/sh/v3/syntax"
)

// ZshSyntaxError reports a command that does not parse as bash but uses zsh
// syntax. Commands are inspected as bash, so zsh syntax stays opaque even
// though a zsh login shell may run it. errors.Is matches it as
// ErrParseFailed.
type ZshSyntaxError struct {
	// Construct names the zsh syntax, such as "parameter expansion flags".
	// It is empty when the bash error does not name it.
	Construct string
	// Possible reports that the zsh grammar rejects the command too, but it
	// holds a zsh loop form that grammar does not know. The command may be
	// valid zsh or broken, and the bash error may be the real one.
	Possible bool
	cause    error
}

func (e *ZshSyntaxError) Error() string {
	return e.cause.Error()
}

// Is matches ErrParseFailed, so callers that only check for a parse failure
// still fail closed.
func (*ZshSyntaxError) Is(target error) bool {
	return target == ErrParseFailed
}

func (e *ZshSyntaxError) Unwrap() error {
	return e.cause
}

// parseFailure wraps a bash syntax error, telling zsh syntax apart from
// a command no shell klaudiush knows can parse.
func parseFailure(command string, err error) error {
	file, zshErr := syntax.NewParser(syntax.Variant(syntax.LangZsh)).
		Parse(strings.NewReader(command), "")
	if zshErr != nil {
		if construct := unknownZshForm(command, zshErr); construct != "" {
			return &ZshSyntaxError{Construct: construct, Possible: true, cause: err}
		}

		// The zsh grammar got further, so its error is the real break.
		if errorOffset(zshErr) > errorOffset(err) {
			return errors.Wrap(ErrParseFailed, zshErr.Error())
		}

		return errors.Wrap(ErrParseFailed, err.Error())
	}

	// The zsh grammar of mvdan.cc/sh reads foreach as a plain call, so a
	// foreach without its end parses although zsh rejects it.
	if hasZshCall(file, "foreach") && !hasZshCall(file, "end") {
		return errors.Wrap(ErrParseFailed, err.Error())
	}

	construct := zshFeature(err)
	if construct == "" {
		construct = zshConstruct(file, errorOffset(err))
	}

	return &ZshSyntaxError{Construct: construct, cause: err}
}

// errorOffset returns the byte offset of a parse error, or -1.
func errorOffset(err error) int {
	var parseErr syntax.ParseError
	if errors.As(err, &parseErr) {
		return int(parseErr.Pos.Offset())
	}

	var langErr syntax.LangError
	if errors.As(err, &langErr) {
		return int(langErr.Pos.Offset())
	}

	return -1
}

// zshFeature returns the zsh feature a bash LangError names, or "".
func zshFeature(err error) string {
	var langErr syntax.LangError
	if errors.As(err, &langErr) && slices.Contains(langErr.Langs, syntax.LangZsh) {
		return langErr.Feature
	}

	return ""
}

// hasZshCall reports whether a call in the file starts with the word name.
func hasZshCall(file *syntax.File, name string) bool {
	found := false

	syntax.Walk(file, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok && len(call.Args) > 0 &&
			call.Args[0].Lit() == name {
			found = true
		}

		return !found
	})

	return found
}

// zshConstruct names the first zsh-only construct in a file parsed as zsh
// that reaches the bash error offset, for zsh syntax the bash error does not
// name. Earlier constructs parsed as bash, so they are not what bash
// rejected. It returns "" when it finds none it knows.
func zshConstruct(file *syntax.File, bashOffset int) string {
	return zshConstructIn(file, bashOffset)
}

const zshDisown = "`&|` and `&!` disowning"

func zshConstructIn(root syntax.Node, bashOffset int) string {
	construct := ""

	syntax.Walk(root, func(node syntax.Node) bool {
		if construct != "" || node == nil {
			return false
		}

		if int(node.End().Offset()) <= bashOffset {
			return false
		}

		// A statement's disown marker trails its words, so name it only
		// when nothing inside the statement is zsh syntax.
		if stmt, ok := node.(*syntax.Stmt); ok && stmt.Disown && node != root {
			construct = zshConstructIn(stmt, bashOffset)
			if construct == "" {
				construct = zshDisown
			}

			return false
		}

		construct = zshNodeConstruct(node)

		return construct == ""
	})

	return construct
}

func zshNodeConstruct(node syntax.Node) string {
	switch n := node.(type) {
	case *syntax.CallExpr:
		// The zsh grammar of mvdan.cc/sh reads a foreach loop as a call.
		if len(n.Args) > 0 && n.Args[0].Lit() == "foreach" {
			return "foreach loops"
		}
	case *syntax.ParamExp:
		return zshParamConstruct(n)
	case *syntax.Word:
		if zshGlobQualifier.MatchString(unquotedText(n)) {
			return "glob qualifiers"
		}
	}

	return ""
}

// zshGlobQualifier matches a glob ending in a qualifier, as in *.go(N): a
// glob character, then an unescaped parenthesized suffix at the end.
var zshGlobQualifier = regexp.MustCompile(`[*?\]](?:[^\\()]|\\.)*\([^()]*\)$`)

// unquotedText joins the literal parts of a word, which the zsh grammar keeps
// glob qualifiers in.
func unquotedText(word *syntax.Word) string {
	var sb strings.Builder

	for _, part := range word.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			sb.WriteString(lit.Value)
		}
	}

	return sb.String()
}

func zshParamConstruct(pe *syntax.ParamExp) string {
	switch {
	case pe.Flags != nil:
		return "parameter expansion flags"
	case pe.Split != 0:
		return "`${=var}` word splitting"
	case pe.GlobSubst != 0:
		return "`${~var}` glob substitution"
	case pe.RcExpand != 0:
		return "`${^var}` array expansion"
	case pe.IsSet:
		return "`${+var}` set tests"
	case len(pe.Modifiers) > 0:
		return "parameter expansion modifiers"
	case pe.NestedParam != nil:
		return "nested parameter expansions"
	default:
		return ""
	}
}

// zshShortFor matches the header of a zsh short loop such as
// "for x (a b) cmd" or "for x y (a b c d) cmd" at the start of the text.
var zshShortFor = regexp.MustCompile(`^for(?:\s+[A-Za-z_][A-Za-z0-9_]*)+\s*\(`)

// zshBraceFor is the feature mvdan.cc/sh names when it rejects
// "for x in a; { cmd }" in zsh mode, a loop form zsh itself accepts.
const zshBraceFor = "for loops with braces"

// unknownZshForm names a zsh loop form the zsh grammar of mvdan.cc/sh does
// not know, so a command using one is not reported as plainly broken shell.
// It cannot tell whether the rest of the command is valid zsh.
func unknownZshForm(command string, zshErr error) string {
	var langErr syntax.LangError
	if errors.As(zshErr, &langErr) && langErr.Feature == zshBraceFor {
		return zshBraceFor
	}

	// The zsh grammar reports a short loop at its "for" keyword.
	var parseErr syntax.ParseError
	if errors.As(zshErr, &parseErr) {
		offset := int(parseErr.Pos.Offset())
		if offset < len(command) && zshShortFor.MatchString(command[offset:]) {
			return "short for loops"
		}
	}

	return ""
}

// OpacityZshGlobQualifier means a word bash reads as an extended glob is, in
// a zsh login shell, a glob qualifier that runs shell code for each match,
// or holds a command substitution the parser does not inspect.
const OpacityZshGlobQualifier OpacityCause = "zsh-glob-qualifier"

// Forms of glob qualifier that run code, set as Opacity.Operation. The e form
// covers oe sorting and the +func form covers o+func sorting.
const (
	qualifierEval = "(e)"
	qualifierFunc = "(+func)"
	// qualifierSubscript is a [...] subscript naming a variable.
	qualifierSubscript = "([...])"
)

// GlobCommandSubst is the Opacity.Operation of an extended glob holding a
// command substitution. Bash and zsh both run it, but the parser keeps an
// extended glob as plain text, so the command it runs is never inspected.
const GlobCommandSubst = "$(...)"

// GlobVariable is the Opacity.Operation of an extended glob holding a $
// expansion, whose text zsh may read as glob qualifiers under glob_subst.
const GlobVariable = "$var"

// GlobSubst is the Opacity.Operation of a word holding zsh's $~var, which
// bash reads as plain text but zsh expands as a glob, qualifiers included.
const GlobSubst = "$~var"

// zshQualifierPrefix starts the (#q...) form, which extended_glob allows
// anywhere in a word.
const zshQualifierPrefix = "#q"

// shellQuoting holds the characters that quote or expand text in a qualifier
// list. zsh removes or expands them before it reads the qualifiers.
const shellQuoting = `'"\$`

// signedQualifiers take a number that may start with +, as in m+3 or
// Lk-10, and unitLetters are the units between the letter and the sign.
// textQualifiers take text or a key that could hide a qualifier letter
// (u, g, f and P delimiters, the e argument, the o sort key, subscripts),
// so after one of them a + is never read as a sign.
const (
	signedQualifiers = "amcLldY"
	unitLetters      = "MwhmskKgGtTpP"
	textQualifiers   = "ugfPeoO["
)

// codeQualifier returns the code-running form in the extended globs of word,
// or "". A [...] subscript only counts in an extended glob ending the word,
// the only place zsh reads one as a qualifier. Bash reads *(e:'cmd':) as an extended glob, but zsh runs cmd for
// every file the glob matches. Every extended glob is checked wherever it
// sits in the word and whatever runs the word, which may flag a bash-only
// command such as bash -c 'ls *(e:x:)'.
func codeQualifier(word *syntax.Word) string {
	if strings.Contains(unquotedText(word), "$~") {
		return GlobSubst
	}

	for i, part := range word.Parts {
		glob, ok := part.(*syntax.ExtGlob)
		if !ok || glob.Pattern == nil {
			continue
		}

		if form := globForm(glob.Pattern.Value, i == len(word.Parts)-1); form != "" {
			return form
		}
	}

	if hasLiteralParen(word) {
		return literalGlobForm(word)
	}

	return ""
}

// numericRange matches the text of a zsh numeric glob such as <0-9> or <->
// between its < and >.
var numericRange = regexp.MustCompile(`^[0-9]*-[0-9]*$`)

// numericGlobQualifier returns the code-running form of qualifiers after a
// zsh numeric glob, or "". Bash reads <0-9>(e:cmd:) as input from the file
// 0-9 and a >(...) process substitution, but zsh reads a numeric glob with
// qualifiers that run cmd for every match.
func numericGlobQualifier(stmt *syntax.Stmt) string {
	for _, redir := range stmt.Redirs {
		if redir.Op != syntax.RdrIn || redir.Word == nil || len(redir.Word.Parts) != 2 {
			continue
		}

		lit, isLit := redir.Word.Parts[0].(*syntax.Lit)
		proc, isProc := redir.Word.Parts[1].(*syntax.ProcSubst)

		if !isLit || !isProc || proc.Op != syntax.CmdOut || !numericRange.MatchString(lit.Value) {
			continue
		}

		var sb strings.Builder

		for i, inner := range proc.Stmts {
			if i > 0 {
				sb.WriteString("; ")
			}

			if err := syntax.NewPrinter().Print(&sb, inner); err != nil {
				return GlobVariable
			}
		}

		if form := globForm(sb.String(), true); form != "" {
			return form
		}
	}

	return ""
}

// globForm returns the code-running form of one extended glob pattern, or
// "". Trailing reports that the glob ends its word.
func globForm(pattern string, trailing bool) string {
	switch {
	case hasCommandSubst(pattern):
		return GlobCommandSubst
	case strings.Contains(pattern, "$"):
		return GlobVariable
	default:
		return qualifierCode(pattern, trailing)
	}
}

// hasLiteralParen reports a literal ( in word, which bash keeps as text only
// where it does not parse extended globs, such as the word of ${x:-word}.
func hasLiteralParen(word *syntax.Word) bool {
	for _, part := range word.Parts {
		if lit, ok := part.(*syntax.Lit); ok && strings.Contains(lit.Value, "(") {
			return true
		}
	}

	return false
}

// literalGlobForm checks the extended glob shapes in the text of a word the
// parser kept as literals. zsh still globs such a word, as in
// ${x:-*(e:cmd:)}, so it is read the way an extended glob would be.
func literalGlobForm(word *syntax.Word) string {
	var sb strings.Builder
	if err := syntax.NewPrinter().Print(&sb, word); err != nil {
		return GlobVariable
	}

	text := sb.String()
	for i := 0; i+1 < len(text); i++ {
		if strings.IndexByte(extGlobOps, text[i]) < 0 || text[i+1] != '(' {
			continue
		}

		end := closingParen(text, i+1)
		if form := globForm(text[i+2:end], end >= len(text)-1); form != "" {
			return form
		}
	}

	return ""
}

// extGlobOps are the characters that open an extended glob before a (.
const extGlobOps = "*?+@!"

// closingParen returns the offset of the ) closing the ( at text[open],
// skipping quoted text and escapes, or len(text) when none closes it.
func closingParen(text string, open int) int {
	depth := 0

	var quote byte

	for i := open; i < len(text); i++ {
		c := text[i]

		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\\':
			i++
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return len(text)
}

// hasCommandSubst reports a $(...) or backtick command substitution in an
// extended glob pattern, quoted or not, arithmetic included.
func hasCommandSubst(pattern string) bool {
	return strings.Contains(pattern, "$(") || strings.Contains(pattern, "`")
}

// qualifierCode reads pattern as a zsh glob qualifier list and returns the
// code-running form it may hold, or "". It errs toward blocking: zsh accepts
// many spellings of a qualifier, so it looks for the shape of one at every
// offset instead of parsing the list. Only a plain pattern, free of quotes,
// backslashes and $, holding | or ( is let through as a zsh pattern group,
// as in @(a|b).
func qualifierCode(pattern string, trailing bool) string {
	list, hashQ := strings.CutPrefix(pattern, zshQualifierPrefix)
	qualifierPlace := trailing || hashQ

	quoted := strings.ContainsAny(list, shellQuoting)
	if !hashQ && !quoted && strings.ContainsAny(list, "|(") {
		return ""
	}

	list = unquote(list)

	for i := range len(list) {
		switch {
		case list[i] == 'e' && closesLater(list, i+1):
			return qualifierEval
		case list[i] == '+' && i+1 < len(list) && isNameChar(list[i+1]) && !isSign(list, i):
			return qualifierFunc
		case list[i] == '[' && qualifierPlace && subscriptHasName(list[i+1:]):
			return qualifierSubscript
		}
	}

	return ""
}

// subscriptHasName reports whether the [...] subscript qualifier starting
// at text names a variable. zsh evaluates the subscript as arithmetic, which
// expands a variable's value, command substitutions included.
func subscriptHasName(text string) bool {
	if end := strings.IndexByte(text, ']'); end >= 0 {
		text = text[:end]
	}

	return strings.ContainsFunc(text, func(r rune) bool {
		return r == '_' || r >= utf8.RuneSelf || unicode.IsLetter(r)
	})
}

// closesLater reports whether the character at list[open] appears again
// after it, or its closing pair for [, {, ( and <, so it can delimit an e
// argument.
func closesLater(list string, open int) bool {
	if open >= len(list) {
		return false
	}

	closing := list[open]
	if pair, ok := delimiterPairs[closing]; ok {
		closing = pair
	}

	return strings.IndexByte(list[open+1:], closing) >= 0
}

var delimiterPairs = map[byte]byte{'[': ']', '{': '}', '(': ')', '<': '>'}

// isSign reports whether the + at list[i] is the sign of a number: it
// comes right after a size, time or count qualifier or its unit, and
// nothing before it takes text. Anything else may be a function call, as
// in *(gdwheeld+fn) or *(om+fn).
func isSign(list string, i int) bool {
	if i == 0 || strings.ContainsAny(list[:i], textQualifiers) {
		return false
	}

	prev := list[i-1]
	if strings.IndexByte(signedQualifiers, prev) >= 0 {
		return true
	}

	return isUnit(list, i-1) && !isUnit(list, i-1-1)
}

// isUnit reports whether list[i] is a unit letter right after a size or
// time qualifier, as the m of am.
func isUnit(list string, i int) bool {
	return i >= 1 && strings.IndexByte(unitLetters, list[i]) >= 0 &&
		strings.IndexByte(signedQualifiers, list[i-1]) >= 0
}

// isNameChar reports a character zsh may accept in a function name after
// +, any byte of a multibyte character included.
func isNameChar(c byte) bool {
	return c == '_' || c >= utf8.RuneSelf || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

// unquote drops quotes and backslashes the way zsh removes them before it
// reads a qualifier list, so *('e':cmd:) reads as *(e:cmd:).
func unquote(list string) string {
	var sb strings.Builder

	for i := 0; i < len(list); i++ {
		switch c := list[i]; {
		case c == '\\' && i+1 < len(list):
			i++
			sb.WriteByte(list[i])
		case strings.IndexByte(shellQuoting, c) >= 0:
		default:
			sb.WriteByte(c)
		}
	}

	return sb.String()
}
