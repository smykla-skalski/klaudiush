package parser

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// unsafeDirChars are the characters that make an unquoted directory split
// under the default IFS or match files.
const unsafeDirChars = ifsBlanks + globChars

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

// oneWordPart reports an unquoted expansion known to give one word that
// matches no files: one of singleWordOutputs, or $(pwd) or $PWD while the
// current directory is known and plain.
func (w *astWalker) oneWordPart(part syntax.WordPart) bool {
	switch p := part.(type) {
	case *syntax.CmdSubst:
		return w.singleWordOutput(p)
	case *syntax.ParamExp:
		return w.pwdParam(p)
	default:
		return false
	}
}

// pwdParam reports a plain $PWD whose value klaudiush tracks and which
// neither splits nor matches files.
func (w *astWalker) pwdParam(p *syntax.ParamExp) bool {
	if p.Exp != nil || p.Repl != nil || p.Slice != nil || p.Index != nil ||
		p.Param == nil || p.Param.Value != pwdVar || p.Excl || p.Length || p.Width {
		return false
	}

	value, set, trusted := w.trustedValue(pwdVar)

	return set && trusted && plainDir(value)
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

	if slices.Equal(argv, []string{pwdBuiltin}) {
		dir, known := w.pwdValue()

		return known && !w.dirComputed && !w.state.untrusted && plainDir(dir)
	}

	return slices.ContainsFunc(singleWordOutputs, func(known []string) bool {
		return slices.Equal(argv, known)
	})
}

// plainDir reports an absolute directory that the default IFS does not
// split and that matches no files.
func plainDir(dir string) bool {
	return strings.HasPrefix(dir, "/") && !strings.ContainsAny(dir, unsafeDirChars)
}

// argShapes says, for each argument of a command, how the shell reads it,
// from how each was written: whether it may split, vanish or match files
// (shifts), whether it stays one word (whole), and whether it starts with
// literal text that is not an option dash and stays one word (tagged), as
// an image name with a tag from known output does. An argument written
// several ways counts by the riskiest.
type argShapes struct {
	shifts map[string]bool
	whole  map[string]bool
	tagged map[string]bool
}

// argShapes reads the shapes of cmd's arguments.
func (w *astWalker) argShapes(cmd Command) argShapes {
	shapes := argShapes{
		shifts: make(map[string]bool, len(cmd.Args)),
		whole:  make(map[string]bool, len(cmd.Args)),
		tagged: make(map[string]bool, len(cmd.Args)),
	}

	total := make(map[string]int, len(cmd.Args))
	for _, arg := range cmd.Args {
		total[arg]++
	}

	seen := make(map[string]int, len(total))

	for _, arg := range cmd.Args {
		written := w.writtenAt(cmd, arg, total[arg], seen[arg])
		first := seen[arg] == 0
		seen[arg]++
		view := written.view
		whole := !strings.Contains(view, splitMark) && !HasUnresolvedVars(view) &&
			!globbed(view, "")

		expanded, detail := w.splitView(view)
		shifts := !whole && (detail != "" && detail != DetailWordSplit || globbed(expanded, ""))
		tagged := whole && written.prefix != "" && !strings.HasPrefix(written.prefix, "-")

		shapes.shifts[arg] = shapes.shifts[arg] || shifts
		shapes.whole[arg] = (first || shapes.whole[arg]) && whole
		shapes.tagged[arg] = (first || shapes.tagged[arg]) && tagged
	}

	return shapes
}

// writtenAt is writtenAs for the nth of total places arg holds among cmd's
// arguments, counted once for all of them.
func (w *astWalker) writtenAt(cmd Command, arg string, total, nth int) writtenArg {
	if forwarded, ok := w.forwarded[arg]; ok {
		return forwarded
	}

	written := cmd.written[arg]
	if len(written) != total {
		return unknownArg(arg)
	}

	return written[nth]
}

// mayShift reports whether word may stand for more or fewer words than
// one, or for words klaudiush cannot see. A word that is not one of the
// command's arguments came from a variable's value and counts by its text.
func (s argShapes) mayShift(word string) bool {
	if shifts, ok := s.shifts[word]; ok {
		return shifts
	}

	return mayBeDynamic(word)
}

// isTagged reports an image name tagged with known output, which cannot be
// an option.
func (s argShapes) isTagged(word string) bool {
	return s.tagged[word]
}

// mayBeDynamic reports a word that comes from a variable, command output, a
// glob or a brace expansion.
func mayBeDynamic(word string) bool {
	return dynamicWord(word) != "" || globWord(word)
}
