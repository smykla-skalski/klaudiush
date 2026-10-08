package parser

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// positionalContext is where a function body expands a positional
// parameter, which decides how a call's argument is put in its place:
// unquoted ($1, split and globbed), a double-quoted string holding only the
// parameter ("$1"), a parameter inside a longer double-quoted string
// ("[$1]"), a parameter in the operand of an expansion inside double quotes
// ("${x:-$1}"), the body of an unquoted heredoc, or single-quoted code that
// eval or trap runs later, where the function's parameters still apply.
type positionalContext int

const (
	positionalUnquoted positionalContext = iota
	positionalQuotedWord
	positionalQuotedPart
	positionalQuotedOperand
	positionalHeredoc
	positionalEvalCode
)

// evalCodeParam matches a positional parameter in code eval or trap runs.
var evalCodeParam = regexp.MustCompile(`\$(?:\{([@*1-9])\}|([@*1-9]))`)

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
	return r.context != positionalUnquoted && r.context != positionalEvalCode
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

		switch n := node.(type) {
		case *syntax.ParamExp:
			if plainPositional(n) {
				refs = append(refs, positionalRefAt(n, stack, heredocs))
			}
		case *syntax.SglQuoted:
			if evalCode(stack) {
				refs = append(refs, evalCodeRefs(n)...)
			}
		}

		stack = append(stack, node)

		return true
	})

	// A heredoc body comes after the line that opens it but is walked with
	// its redirect.
	slices.SortFunc(refs, func(a, b positionalRef) int { return cmp.Compare(a.start, b.start) })

	return refs, true
}

// evalCode reports a single-quoted argument of eval or trap, whose text is
// run later as code in the function, with its positional parameters.
func evalCode(stack []syntax.Node) bool {
	word, ok := lastNode(stack).(*syntax.Word)
	if !ok {
		return false
	}

	call, ok := lastNode(stack[:len(stack)-1]).(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 || call.Args[0] == word {
		return false
	}

	name := call.Args[0].Lit()

	return name == evalBuiltin || name == trapBuiltin
}

// lastNode returns the innermost node of stack, or nil.
func lastNode(stack []syntax.Node) syntax.Node {
	if len(stack) == 0 {
		return nil
	}

	return stack[len(stack)-1]
}

// evalCodeRefs returns the positional parameters in single-quoted code.
func evalCodeRefs(quoted *syntax.SglQuoted) []positionalRef {
	if quoted.Dollar {
		return nil
	}

	base := int(quoted.Pos().Offset()) + 1

	var refs []positionalRef

	for _, m := range evalCodeParam.FindAllStringSubmatchIndex(quoted.Value, -1) {
		group := 2
		if m[group] < 0 {
			group = 4
		}

		param := quoted.Value[m[group]:m[group+1]]

		refs = append(refs, positionalRef{
			param:   param,
			start:   base + m[0],
			end:     base + m[1],
			context: positionalEvalCode,
		})
	}

	return refs
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

	operand := false

	for _, node := range slices.Backward(stack) {
		switch n := node.(type) {
		case *syntax.ParamExp:
			operand = true
		case *syntax.DblQuoted:
			switch {
			case operand:
				ref.context = positionalQuotedOperand
			case len(n.Parts) == 1 && n.Parts[0] == exp:
				ref.context = positionalQuotedWord
				ref.start, ref.end = int(n.Pos().Offset()), int(n.End().Offset())
			default:
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

// doubleQuoteEscape keeps a value literal inside double quotes, where a
// nested quote would start a new string rather than end the value.
func doubleQuoteEscape(value string) string {
	return strings.NewReplacer(`\`, `\\`, "$", `\$`, "`", "\\`", `"`, `\"`).Replace(value)
}
