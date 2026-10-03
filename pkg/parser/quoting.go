package parser

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// renderedSpecial are the characters that make a literal word render like
// an expansion or a glob, so a quoted one is recorded as quoted.
const renderedSpecial = globChars + "{$"

// quotedArgs returns the rendered arguments whose expansions and glob
// characters are all quoted, so the shell keeps each one a single word
// ("$X", "$(pwd)":/w, '*.go'). An argument rendered the same as one that
// is not quoted is left out, so the shared rendering fails closed.
func quotedArgs(words []*syntax.Word) map[string]bool {
	var quoted map[string]bool

	for _, word := range words {
		expands, splits := wordSplitting(word)
		if !expands {
			continue
		}

		if quoted == nil {
			quoted = make(map[string]bool)
		}

		key := argWord(word)
		if seen, ok := quoted[key]; !ok || seen {
			quoted[key] = !splits
		}
	}

	return quoted
}

// wordSplitting reports whether word expands at all (a variable, command
// output, arithmetic, a glob or a brace expansion), and whether the shell
// may split it into several words, drop it, or match it against files.
func wordSplitting(word *syntax.Word) (expands, splits bool) {
	if word == nil {
		return false, false
	}

	clone := &syntax.Word{Parts: append([]syntax.WordPart(nil), word.Parts...)}
	if syntax.SplitBraces(clone) {
		return true, true
	}

	return partsSplitting(word.Parts, false)
}

func partsSplitting(parts []syntax.WordPart, quoted bool) (expands, splits bool) {
	for _, part := range parts {
		e, s := partSplitting(part, quoted)
		expands, splits = expands || e, splits || s
	}

	return expands, splits
}

func partSplitting(part syntax.WordPart, quoted bool) (expands, splits bool) {
	switch p := part.(type) {
	case *syntax.Lit:
		special := strings.ContainsAny(p.Value, renderedSpecial)
		if quoted {
			return special, false
		}

		return special, globWord(unescapedGlobs(p.Value))
	case *syntax.SglQuoted:
		return strings.ContainsAny(p.Value, renderedSpecial), false
	case *syntax.DblQuoted:
		return partsSplitting(p.Parts, true)
	case *syntax.ParamExp:
		return true, !quoted || allElementsParam(p)
	case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp, *syntax.ExtGlob:
		return true, !quoted
	default:
		return true, true
	}
}

// allElementsParam reports "$@" or an array subscript ("${a[@]}"), which
// expand to one word per element even when quoted.
func allElementsParam(p *syntax.ParamExp) bool {
	return p.Index != nil || (p.Param != nil && p.Param.Value == "@")
}

// mayShift reports whether arg may stand for more or fewer words than one,
// or for words klaudiush cannot see: an unquoted variable, command output, a
// glob or a brace expansion. A word reading options and operands one by
// one cannot know where it is past such an argument.
func (c Command) mayShift(arg string) bool {
	return mayBeDynamic(arg) && !c.quotedWords[strings.ReplaceAll(arg, unresolvedWord, "")]
}

// mayBeDynamic reports a word that comes from a variable, command output, a
// glob or a brace expansion.
func mayBeDynamic(word string) bool {
	return dynamicWord(word) != "" || globWord(word)
}
