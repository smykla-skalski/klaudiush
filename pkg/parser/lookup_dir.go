package parser

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// lookupDir returns the directory a cd or pushd to the output of an
// allowed lookup moves to, as in cd "$(git rev-parse --show-toplevel)/sub",
// by running the lookup the way the shell would. It returns "" for
// anything else, which leaves a computed directory unknown.
func (w *astWalker) lookupDir(name string, words []*syntax.Word) string {
	if name != cdBuiltin && name != "pushd" {
		return ""
	}

	var operand *syntax.Word

	for _, word := range words[1:] {
		if isLiteralWord(word) && strings.HasPrefix(wordToString(word), "-") {
			continue
		}

		if operand != nil {
			return ""
		}

		operand = word
	}

	if operand == nil || !wordDynamic(operand) {
		return ""
	}

	dir, ok := w.lookupParts(operand.Parts)
	if !ok || !filepath.IsAbs(dir) {
		return ""
	}

	return filepath.Clean(dir)
}

// lookupParts renders word parts made only of literal text and allowed
// lookups.
func (w *astWalker) lookupParts(parts []syntax.WordPart) (string, bool) {
	var b strings.Builder

	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}

			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			text, ok := w.lookupParts(p.Parts)
			if !ok {
				return "", false
			}

			b.WriteString(text)
		case *syntax.CmdSubst:
			call := plainCall(p)
			if call == nil {
				return "", false
			}

			out, ok := w.lookupOutput(call.Args)
			if !ok {
				return "", false
			}

			b.WriteString(out)
		default:
			return "", false
		}
	}

	return b.String(), true
}
