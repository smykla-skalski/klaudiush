package parser

import (
	"path/filepath"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// renderedSpecial are the characters that make a literal word render like
// an expansion or a glob, so a quoted one is recorded as quoted.
const renderedSpecial = globChars + "{$"

// unsafeDirChars are the characters that make an unquoted directory split
// under the default IFS or match files.
const unsafeDirChars = " \t\n" + globChars

// pwdVar is the variable the shell keeps set to its current directory.
const pwdVar = "PWD"

// singleWordOutputs are the command lines whose output is one word that
// matches no files: a number or a user name. pwd is handled apart, since
// its output is only known to be one word when the directory is.
var singleWordOutputs = [][]string{
	strings.Fields("id -u"),
	strings.Fields("id -g"),
	strings.Fields("id -un"),
	strings.Fields("nproc"),
	strings.Fields("getconf _NPROCESSORS_ONLN"),
}

// workingDirResolver is a Resolver that knows the directory commands start
// in, so a relative cd and pwd can be taken as an absolute path.
type workingDirResolver interface {
	WorkingDir() (string, bool)
}

// quotedArgs returns the rendered arguments the shell keeps one word each:
// every expansion and glob character is quoted ("$X", "$(pwd)":/w,
// '*.go'), or is one known to expand to a single word that matches no
// files ($(id -u), $(pwd) in a directory without spaces). An argument
// rendered the same as one that may split is left out, so the shared
// rendering fails closed.
//
// The second map holds the arguments that are also an image name with a
// tag from such output (img:$(id -un)): literal text first, so the output
// cannot make it an option.
func (w *astWalker) quotedArgs(words []*syntax.Word) (quoted, tagged map[string]bool) {
	for _, word := range words {
		expands, splits := w.wordSplitting(word)
		if !expands {
			continue
		}

		if quoted == nil {
			quoted, tagged = make(map[string]bool), make(map[string]bool)
		}

		key := argWord(word)
		if seen, ok := quoted[key]; !ok || seen {
			quoted[key] = !splits
		}

		if seen, ok := tagged[key]; !ok || seen {
			tagged[key] = !splits && w.literalThenOutput(word)
		}
	}

	return quoted, tagged
}

// literalThenOutput reports a word that starts with literal text other than
// an option dash and whose expansions are all known one-word output.
func (w *astWalker) literalThenOutput(word *syntax.Word) bool {
	first, ok := word.Parts[0].(*syntax.Lit)
	if !ok || first.Value == "" || strings.HasPrefix(first.Value, "-") {
		return false
	}

	for _, part := range word.Parts[1:] {
		switch p := part.(type) {
		case *syntax.Lit:
		case *syntax.CmdSubst:
			if !w.singleWordOutput(p) {
				return false
			}
		case *syntax.ParamExp:
			if !w.pwdParam(p) {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// wordSplitting reports whether word expands at all (a variable, command
// output, arithmetic, a glob or a brace expansion), and whether the shell
// may split it into several words, drop it, or match it against files.
func (w *astWalker) wordSplitting(word *syntax.Word) (expands, splits bool) {
	if word == nil {
		return false, false
	}

	clone := &syntax.Word{Parts: append([]syntax.WordPart(nil), word.Parts...)}
	if syntax.SplitBraces(clone) {
		return true, true
	}

	return w.partsSplitting(word.Parts, false)
}

func (w *astWalker) partsSplitting(parts []syntax.WordPart, quoted bool) (expands, splits bool) {
	for _, part := range parts {
		e, s := w.partSplitting(part, quoted)
		expands, splits = expands || e, splits || s
	}

	return expands, splits
}

func (w *astWalker) partSplitting(part syntax.WordPart, quoted bool) (expands, splits bool) {
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
		return w.partsSplitting(p.Parts, true)
	case *syntax.ParamExp:
		if quoted {
			return true, allElementsParam(p)
		}

		return true, !w.pwdParam(p)
	case *syntax.CmdSubst:
		return true, !quoted && !w.singleWordOutput(p)
	case *syntax.ProcSubst, *syntax.ArithmExp, *syntax.ExtGlob:
		return true, !quoted
	default:
		return true, true
	}
}

// allElementsParam reports "$@" or an all-elements subscript ("${a[@]}"),
// which expand to one word per element even when quoted; "${a[0]}" and
// "${a[*]}" stay one word.
func allElementsParam(p *syntax.ParamExp) bool {
	if p.Param != nil && p.Param.Value == "@" {
		return true
	}

	index, ok := p.Index.(*syntax.Word)

	return ok && index.Lit() == "@"
}

// pwdParam reports a plain $PWD that the line has not reassigned, in a
// directory known to be one word.
func (w *astWalker) pwdParam(p *syntax.ParamExp) bool {
	if p.Param == nil || p.Param.Value != pwdVar || p.Exp != nil || p.Repl != nil ||
		p.Slice != nil || p.Index != nil || p.Excl || p.Length || p.Width {
		return false
	}

	if _, assigned := w.assignments[pwdVar]; assigned || w.unknownVars[pwdVar] {
		return false
	}

	return w.oneWordDir()
}

// singleWordOutput reports a command substitution that is exactly one of
// singleWordOutputs, or pwd in a directory known to be one word: no other
// arguments, assignments or redirects, and no same-line function or alias
// standing in for the program.
func (w *astWalker) singleWordOutput(sub *syntax.CmdSubst) bool {
	if len(sub.Stmts) != 1 {
		return false
	}

	stmt := sub.Stmts[0]
	if stmt.Negated || stmt.Background || stmt.Coprocess || len(stmt.Redirs) > 0 {
		return false
	}

	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return false
	}

	argv, ok := literalArgs(call.Args)
	if !ok || w.defined(argv[0]) {
		return false
	}

	if slices.Equal(argv, []string{"pwd"}) {
		return w.oneWordDir()
	}

	return slices.ContainsFunc(singleWordOutputs, func(known []string) bool {
		return slices.Equal(argv, known)
	})
}

// oneWordDir reports a current directory known as an absolute path that
// neither splits under the default IFS nor matches files.
func (w *astWalker) oneWordDir() bool {
	if w.dirUnknown || w.dirComputed || w.state.untrusted {
		return false
	}

	dir := ExpandHome(w.currentDir, w.resolver)
	if strings.HasPrefix(dir, "~") {
		return false
	}

	if !filepath.IsAbs(dir) {
		start, ok := w.startDir()
		if !ok {
			return false
		}

		dir = filepath.Join(start, dir)
	}

	return !strings.ContainsAny(dir, unsafeDirChars)
}

// startDir returns the absolute directory the command starts in, when the
// resolver knows it.
func (w *astWalker) startDir() (string, bool) {
	resolver, ok := w.resolver.(workingDirResolver)
	if !ok {
		return "", false
	}

	dir, ok := resolver.WorkingDir()

	return dir, ok && filepath.IsAbs(dir)
}

// mayShift reports whether arg may stand for more or fewer words than one,
// or for words klaudiush cannot see: an unquoted variable, command output, a
// glob or a brace expansion. A word reading options and operands one by
// one cannot know where it is past such an argument.
func (c Command) mayShift(arg string) bool {
	return mayBeDynamic(arg) && !c.quotedWords[strings.ReplaceAll(arg, unresolvedWord, "")]
}

// tagged reports whether arg is an image name tagged with known one-word
// output, which cannot be an option.
func (c Command) tagged(arg string) bool {
	return c.taggedWords[strings.ReplaceAll(arg, unresolvedWord, "")]
}

// mayBeDynamic reports a word that comes from a variable, command output, a
// glob or a brace expansion.
func mayBeDynamic(word string) bool {
	return dynamicWord(word) != "" || globWord(word)
}
