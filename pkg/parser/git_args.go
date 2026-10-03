package parser

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Fixed reasons, set as Opacity.Detail, that a git argument is opaque, beside
// the ones command words share.
const (
	DetailWordSplit = "it comes from a variable whose value the shell splits " +
		"into several words"
	DetailWordSecret = "it comes from an environment variable whose value looks " +
		"like a secret, which klaudiush does not substitute"
)

// argumentSuffix ends the Opacity.Operation of a git argument.
const argumentSuffix = " argument"

// ArgumentOperation is the Opacity.Operation of an argument of git sub that
// comes from a variable, command output, a glob or word splitting.
func ArgumentOperation(sub string) string {
	return gitProgram + " " + sub + argumentSuffix
}

// IsArgumentOperation reports an Opacity.Operation made by ArgumentOperation.
func IsArgumentOperation(operation string) bool {
	return strings.HasPrefix(operation, gitProgram+" ") &&
		strings.HasSuffix(operation, argumentSuffix)
}

const subcmdPush = "push"

// ifsBlanks are what the default IFS splits on; any other IFS stops every
// variable from resolving.
const ifsBlanks = " \t\n"

// splitMark stands in a view for an unquoted command substitution, whose
// output the shell splits and globs.
const splitMark = ""

// optionStarts are the first characters of a glob or brace expansion that
// can expand to an option.
const optionStarts = "*?[{-"

// argValueFlags are, per checked subcommand, the options that take the next
// argument as their value. Only push's --repo, --receive-pack and --exec
// values are read; the others need only stay one word.
var argValueFlags = map[string]map[string]bool{
	subcmdPush: nameSet("-o --push-option --repo --receive-pack --exec"),
	subcmdCommit: nameSet(`-m --message -F --file -c -C --reedit-message --reuse-message
		--author --date --fixup --squash -t --template --cleanup --trailer --pathspec-from-file`),
}

// gluedValueFlags are the short options that take a value attached to them
// (-mtext, -S<keyid>), per checked subcommand.
var gluedValueFlags = map[string]string{
	subcmdPush:   "o",
	subcmdCommit: "mFcCtSu",
}

// uninspectedPushValues are the push options whose value no validator reads.
var uninspectedPushValues = nameSet("-o --push-option")

// writtenArg is how an argument was written. prefix is its literal text
// before the first expansion; literal reports no expansion at all. view is
// the argument with quoted glob, brace and blank characters neutralized,
// quoted expansions standing for nothing, unquoted variables kept as
// references and unquoted substitutions as splitMark, so splitting and
// globbing are found in it.
type writtenArg struct {
	prefix    string
	view      string
	literal   bool
	ambiguous bool
}

// writtenArgs records how each argument word was written, keyed by the
// argument it renders to. Two words that render alike but were written
// differently are marked ambiguous and checked as if unquoted.
func writtenArgs(words []*syntax.Word) map[string]writtenArg {
	written := make(map[string]writtenArg, len(words))

	for _, word := range words {
		key := markSubstituted(word, argWord(word))
		arg := writeArg(word)

		if prev, ok := written[key]; ok && prev != arg {
			arg = writtenArg{ambiguous: true}
		}

		written[key] = arg
	}

	return written
}

func writeArg(word *syntax.Word) writtenArg {
	var prefix, view strings.Builder

	arg := writtenArg{literal: true}
	open := true

	text := func(rendered, neutral string) {
		view.WriteString(neutral)

		if open {
			prefix.WriteString(rendered)
		}
	}

	expansion := func(shown string) {
		open, arg.literal = false, false

		view.WriteString(shown)
	}

	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			text(renderLit(p.Value, true, allEscapable), unescapedArg(p.Value))
		case *syntax.SglQuoted:
			value := p.Value
			if p.Dollar {
				value = decodeANSIC(p.Value)
			}

			text(value, neutralArg(value))
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				if lit, ok := inner.(*syntax.Lit); ok {
					value := renderLit(lit.Value, true, doubleQuoteEscapable)
					text(value, neutralArg(value))

					continue
				}

				expansion(neutralGlob)
			}
		case *syntax.ParamExp:
			expansion(paramExpToString(p))
		default:
			expansion(splitMark)
		}
	}

	arg.prefix, arg.view = prefix.String(), view.String()

	return arg
}

// neutralArg replaces the characters the shell would glob, brace-expand or
// split on in quoted text.
func neutralArg(text string) string {
	return strings.NewReplacer(
		"*", neutralGlob, "?", neutralGlob, "[", neutralGlob, "{", neutralGlob,
		" ", neutralGlob, "\t", neutralGlob, "\n", neutralGlob,
	).Replace(text)
}

// unescapedArg neutralizes the characters a backslash escapes in unquoted
// text, keeping the others.
func unescapedArg(text string) string {
	var b strings.Builder

	escaped := false

	for _, r := range text {
		switch {
		case escaped:
			escaped = false

			b.WriteString(neutralArg(string(r)))
		case r == '\\':
			escaped = true
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}

// writtenAs returns how arg was written, or, when that is unknown (an
// argument passed on by a function or a launcher that rendered it), a view
// that treats every expansion in it as unquoted.
func (c Command) writtenAs(arg string) writtenArg {
	if written, ok := c.written[arg]; ok && !written.ambiguous {
		return written
	}

	view := strings.ReplaceAll(arg, unresolvedWord, splitMark)

	return writtenArg{view: view, literal: !marked(arg) && !HasUnresolvedVars(arg)}
}

// resolveGitArgs checks the arguments of the git subcommands whose argument
// values validators read: every argument of push (remote, refspecs, options
// and the global options before it) and the options of commit. An argument
// built from command output, a variable klaudiush cannot resolve, a glob, a
// brace expansion or word splitting is opaque. A variable it can resolve is
// replaced by its value, so B=main; git push origin $B is checked as a push
// to main.
func (w *astWalker) resolveGitArgs(cmd Command) Command {
	idx := gitSubcommandIndex(cmd.Args)
	if idx < 0 {
		return cmd
	}

	sub := cmd.Args[idx]
	if _, checked := argValueFlags[sub]; !checked {
		return cmd
	}

	args := make([]string, 0, len(cmd.Args))

	for i := range len(cmd.Args) {
		if i == idx || (i < idx && sub == subcmdCommit) {
			args = append(args, cmd.Args[i])

			continue
		}

		value, keep, detail := w.gitArg(cmd, i, idx, sub)
		if detail != "" {
			w.opaque(OpacityUnresolvedWord, ArgumentOperation(sub), detail)

			return cmd
		}

		if keep {
			args = append(args, value)
		}

		if sub == subcmdCommit && i > idx && endsOptions(cmd, i, sub) {
			args = append(args, cmd.Args[i+1:]...)

			break
		}
	}

	cmd.Args = args

	return cmd
}

// endsOptions reports a literal -- that is not an option's value: what
// follows it are paths.
func endsOptions(cmd Command, i int, sub string) bool {
	return cmd.Args[i] == endOfOptions && cmd.writtenAs(endOfOptions).literal &&
		!valueSlot(cmd.Args, i, sub)
}

// gitArg checks the argument at i of a git command whose subcommand is at
// idx. It returns the argument to keep in its place, whether to keep one,
// or why the argument cannot be known.
func (w *astWalker) gitArg(cmd Command, i, idx int, sub string) (string, bool, string) {
	arg := cmd.Args[i]
	written := cmd.writtenAs(arg)

	expanded, detail := w.splitView(written.view)
	if detail != "" {
		return arg, true, detail
	}

	if i > idx && valueSlot(cmd.Args, i, sub) {
		if sub == subcmdPush && !uninspectedPushValues[cmd.Args[i-1]] {
			return w.pushArg(arg, written, expanded)
		}

		return arg, true, ""
	}

	if sub == subcmdPush {
		return w.pushArg(arg, written, expanded)
	}

	return w.commitOption(arg, written, expanded)
}

// valueSlot reports an argument that is the value of the option before it,
// written literally.
func valueSlot(args []string, i int, sub string) bool {
	prev := args[i-1]
	if marked(prev) || HasUnresolvedVars(prev) {
		return false
	}

	if argValueFlags[sub][prev] {
		return true
	}

	if len(prev) < len("-xy") || prev[0] != '-' || prev[1] == '-' {
		return false
	}

	return argValueFlags[sub]["-"+prev[len(prev)-1:]]
}

// pushArg checks an argument of push, all of which the push validator reads.
func (w *astWalker) pushArg(
	arg string,
	written writtenArg,
	expanded string,
) (string, bool, string) {
	if globbed(expanded, "") {
		return arg, true, DetailWordOutput
	}

	return w.resolvedArg(arg, written, expanded)
}

// commitOption checks a commit argument that is not an option's value: an
// expansion klaudiush cannot see may hide an option such as --no-verify, so
// it is opaque unless the literal text before it shows that it is a path
// or an option's attached value.
func (w *astWalker) commitOption(
	arg string,
	written writtenArg,
	expanded string,
) (string, bool, string) {
	if globbed(expanded, optionStarts) {
		return arg, true, DetailWordOutput
	}

	value, keep, detail := w.resolvedArg(arg, written, expanded)
	if detail != "" && notOption(written.prefix, subcmdCommit) {
		return arg, true, ""
	}

	return value, keep, detail
}

// notOption reports literal text that starts an argument which cannot be
// an option: a path, or an option whose value starts within the text.
func notOption(prefix, sub string) bool {
	switch {
	case prefix == "":
		return false
	case !strings.HasPrefix(prefix, "-"):
		return true
	case strings.HasPrefix(prefix, "--"):
		return strings.Contains(prefix, "=")
	}

	return strings.ContainsAny(prefix[1:], gluedValueFlags[sub])
}

// resolvedArg returns an argument with its variables replaced by their
// values. An unquoted variable that expands to nothing leaves no argument.
func (w *astWalker) resolvedArg(
	arg string,
	written writtenArg,
	expanded string,
) (string, bool, string) {
	if marked(arg) {
		return arg, true, DetailWordOutput
	}

	if written.literal {
		return arg, true, ""
	}

	if w.plainArrayRef(arg) {
		return arg, true, w.variableDetail()
	}

	if w.secretRef(arg) {
		return arg, true, DetailWordSecret
	}

	value, ok := w.resolveWord(w.applyDefaults(arg))
	if !ok {
		return arg, true, w.variableDetail()
	}

	if value == "" && strings.TrimSpace(expanded) == "" {
		return "", false, ""
	}

	return value, true, ""
}

// splitView expands the unquoted variables of a view. It says why the
// argument cannot be known when the shell would split it: an unquoted
// command substitution, or an unquoted variable klaudiush cannot resolve or
// whose value holds blanks.
func (w *astWalker) splitView(view string) (string, string) {
	if strings.Contains(view, splitMark) {
		return view, DetailWordOutput
	}

	if !HasUnresolvedVars(view) {
		return view, ""
	}

	detail := ""

	expanded := expandVars(w.applyDefaults(view), func(name string) (string, bool) {
		value, set, trusted := w.trustedValue(name)

		switch {
		case !trusted || !set:
			return "", false
		case strings.ContainsAny(value, ifsBlanks):
			detail = DetailWordSplit
		}

		return value, true
	})

	switch {
	case HasUnresolvedVars(expanded) || w.plainArrayRef(view):
		return expanded, w.variableDetail()
	default:
		return expanded, detail
	}
}

// secretRef reports a variable in arg whose value comes from the
// environment rather than the line and looks like a key or password, which
// a validator's message would show once substituted.
func (w *astWalker) secretRef(arg string) bool {
	for _, m := range varRefPattern.FindAllStringSubmatch(arg, -1) {
		if _, assigned := w.assignments[m[1]]; assigned {
			continue
		}

		value, set, trusted := w.trustedValue(m[1])
		if trusted && set &&
			slices.ContainsFunc(strings.FieldsFunc(value, urlSeparator), tokenLike) {
			return true
		}
	}

	return false
}

func urlSeparator(r rune) bool {
	return strings.ContainsRune(":/@=", r)
}

// globbed reports a view the shell would expand against file names or as a
// brace expansion. With starts set, only a word that could expand to one
// starting with an option dash counts.
func globbed(view, starts string) bool {
	for word := range strings.FieldsSeq(view) {
		if !globWord(word) && !bracesExpand(word) {
			continue
		}

		if starts == "" || strings.ContainsAny(word[:1], starts) {
			return true
		}
	}

	return false
}
