package parser

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// positionalContext is where a function body expands a positional
// parameter, which decides how a call's argument is put in its place:
// unquoted ($1, split and globbed), a double-quoted string holding only the
// parameter ("$1"), a parameter inside a longer double-quoted string
// ("[$1]"), or the body of an unquoted heredoc.
type positionalContext int

const (
	positionalUnquoted positionalContext = iota
	positionalQuotedWord
	positionalQuotedPart
	positionalHeredoc
)

// positionalRef is one positional parameter a function body expands. start
// and end are the byte range replaced by the argument: the parameter, or the
// whole double-quoted string for positionalQuotedWord.
type positionalRef struct {
	param      string
	start, end int
	context    positionalContext
}

// quoted reports whether the value is kept as one word, unsplit and unglobbed.
func (r positionalRef) quoted() bool {
	return r.context != positionalUnquoted
}

// positionalRefs returns the positional parameters ($1-$9, $@, $*) a
// function body expands, in source order, read from its syntax tree so that
// text in single quotes and quotes around other words are left alone. It
// reports false when the body does not parse.
func positionalRefs(body string) ([]positionalRef, bool) {
	file, err := syntax.NewParser().Parse(strings.NewReader(body), "")
	if err != nil {
		return nil, false
	}

	heredocs := make(map[*syntax.Word]bool)

	syntax.Walk(file, func(node syntax.Node) bool {
		if redir, ok := node.(*syntax.Redirect); ok && redir.Hdoc != nil {
			heredocs[redir.Hdoc] = true
		}

		return true
	})

	var (
		refs  []positionalRef
		stack []syntax.Node
	)

	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]

			return true
		}

		if exp, ok := node.(*syntax.ParamExp); ok && plainPositional(exp) {
			refs = append(refs, positionalRefAt(exp, stack, heredocs))
		}

		stack = append(stack, node)

		return true
	})

	return refs, true
}

// plainPositional reports a bare $1-$9, $@ or $* (braced or not) with no
// operator, the only forms substitutePositional replaces.
func plainPositional(exp *syntax.ParamExp) bool {
	if exp.Param == nil || exp.Flags != nil || exp.NestedParam != nil ||
		exp.Excl || exp.Length || exp.Width || exp.IsSet || exp.Index != nil ||
		len(exp.Modifiers) > 0 || exp.Slice != nil || exp.Repl != nil ||
		exp.Names != 0 || exp.Exp != nil {
		return false
	}

	value := exp.Param.Value

	return value == "@" || value == "*" || len(value) == 1 && value >= "1" && value <= "9"
}

// positionalRefAt finds how exp is expanded from its nearest enclosing
// quoting: a double-quoted string, a heredoc body, or a command or
// arithmetic substitution, which starts an unquoted context again.
func positionalRefAt(
	exp *syntax.ParamExp,
	stack []syntax.Node,
	heredocs map[*syntax.Word]bool,
) positionalRef {
	ref := positionalRef{
		param: exp.Param.Value,
		start: int(exp.Pos().Offset()),
		end:   int(exp.End().Offset()),
	}

	for _, node := range slices.Backward(stack) {
		switch n := node.(type) {
		case *syntax.DblQuoted:
			if len(n.Parts) == 1 && n.Parts[0] == exp {
				ref.context = positionalQuotedWord
				ref.start, ref.end = int(n.Pos().Offset()), int(n.End().Offset())
			} else {
				ref.context = positionalQuotedPart
			}

			return ref
		case *syntax.Word:
			if heredocs[n] {
				ref.context = positionalHeredoc

				return ref
			}
		case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp, *syntax.ArithmCmd:
			return ref
		}
	}

	return ref
}

// positionalValues returns the call arguments a parameter expands to, and
// whether the parameter is set.
func positionalValues(param string, args []string) ([]string, bool) {
	if param == "@" || param == "*" {
		return args, true
	}

	n := int(param[0] - '0')
	if n > len(args) {
		return nil, false
	}

	return args[n-1 : n], true
}

// heredocEscape keeps a value literal in an unquoted heredoc body, where
// backslash, dollar and backquote are still special.
func heredocEscape(value string) string {
	return strings.NewReplacer(`\`, `\\`, "$", `\$`, "`", "\\`").Replace(value)
}
