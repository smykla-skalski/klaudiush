package parser

import (
	"slices"
	"strings"
)

// XargsReplaceOperation names an xargs replace string (-I, -i, --replace,
// -J) in an Opacity: it comes from a word klaudiush cannot read, so where
// the input goes is unknown.
const XargsReplaceOperation = "xargs replace string"

// literalValue returns an option value with known variables substituted,
// or why it cannot be known: command output, a brace expansion, or a
// variable klaudiush does not resolve.
func (w *astWalker) literalValue(value string) (resolved, detail string) {
	switch {
	case marked(value) || bracesExpand(value):
		return "", DetailWordOutput
	case HasUnresolvedVars(value):
		expanded, ok := w.resolveWord(value)
		if !ok {
			return "", DetailWordVariable
		}

		return expanded, ""
	default:
		return value, ""
	}
}

// resolveXargsReplace returns the commands xargs runs once a replace string
// written as a variable is resolved: R=X; xargs -I "$R" X push replaces X.
// A replace string it cannot resolve is opaque and nothing is followed.
func (w *astWalker) resolveXargsReplace(cmd Command, cmds []Command) []Command {
	idx, ok := commandIndex(launchers[cmd.Name], cmd.Args)
	if !ok {
		return cmds
	}

	replace, _ := xargsInput(cmd.Args[:idx])
	if replace == "" || !mayBeDynamic(replace) {
		return cmds
	}

	value, detail := w.literalValue(replace)
	if detail != "" {
		defer w.enter(cmd)()

		w.opaque(OpacityUnresolvedWord, XargsReplaceOperation, detail)

		return nil
	}

	resolved := cmd
	resolved.Args = slices.Clone(cmd.Args)

	for i, arg := range resolved.Args[:idx] {
		resolved.Args[i] = strings.Replace(arg, replace, value, 1)
	}

	return launched(resolved).commands
}
