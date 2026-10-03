package parser

import (
	"regexp"
	"slices"
	"strings"

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

		// Bash stopped at zsh syntax, so the zsh error is the real break.
		if zshFeature(err) != "" {
			return errors.Wrap(ErrParseFailed, zshErr.Error())
		}

		return errors.Wrap(ErrParseFailed, err.Error())
	}

	construct := zshFeature(err)
	if construct == "" {
		construct = zshConstruct(file)
	}

	return &ZshSyntaxError{Construct: construct, cause: err}
}

// zshFeature returns the zsh feature a bash LangError names, or "".
func zshFeature(err error) string {
	var langErr syntax.LangError
	if errors.As(err, &langErr) && slices.Contains(langErr.Langs, syntax.LangZsh) {
		return langErr.Feature
	}

	return ""
}

// zshConstruct names the first zsh-only construct in a file parsed as zsh,
// for zsh syntax the bash error does not name. It returns "" when it finds
// none it knows.
func zshConstruct(file *syntax.File) string {
	construct := ""

	syntax.Walk(file, func(node syntax.Node) bool {
		if construct != "" {
			return false
		}

		construct = zshNodeConstruct(node)

		return construct == ""
	})

	return construct
}

func zshNodeConstruct(node syntax.Node) string {
	switch n := node.(type) {
	case *syntax.Stmt:
		if n.Disown {
			return "`&|` and `&!` disowning"
		}
	case *syntax.CallExpr:
		// The zsh grammar of mvdan.cc/sh reads a foreach loop as a call.
		if len(n.Args) > 0 && n.Args[0].Lit() == "foreach" {
			return "foreach loops"
		}
	case *syntax.ParamExp:
		return zshParamConstruct(n)
	case *syntax.Word:
		// Only unquoted text: zsh glob qualifiers like *.go(N) stay literal.
		for _, part := range n.Parts {
			if lit, ok := part.(*syntax.Lit); ok && strings.Contains(lit.Value, "(") {
				return "glob qualifiers"
			}
		}
	}

	return ""
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
