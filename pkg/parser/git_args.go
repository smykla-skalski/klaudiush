package parser

import (
	"maps"
	"regexp"
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

const (
	pwdBuiltin      = "pwd"
	lookupSeparator = "\x00"
)

var (
	branchShowCurrent  = []string{gitProgram, "branch", "--show-current"}
	revParseAbbrevHead = []string{gitProgram, revParse, "--abbrev-ref", "HEAD"}
)

// ArgumentLookups are the command substitutions whose output a git push
// argument is built from: the current branch, and the directory -C names.
// Each changes nothing, and runs in the command's directory.
var ArgumentLookups = [][]string{
	branchShowCurrent,
	revParseAbbrevHead,
	revParseToplevel,
	{pwdBuiltin},
}

// ArgumentLookup reports whether argv is one of ArgumentLookups.
func ArgumentLookup(argv []string) bool {
	return slices.ContainsFunc(ArgumentLookups, func(allowed []string) bool {
		return slices.Equal(allowed, argv)
	})
}

// plainBranch matches lookup output safe to use as a branch: one word with
// nothing the shell would split, glob or expand, and no option dash.
var plainBranch = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./+@-]*$`)

// lookupArgv returns the literal words of the one command a word that is a
// single command substitution ("$(git branch --show-current)") runs.
func lookupArgv(word *syntax.Word) []string {
	parts := word.Parts
	if len(parts) == 1 {
		if quoted, ok := parts[0].(*syntax.DblQuoted); ok {
			parts = quoted.Parts
		}
	}

	if len(parts) != 1 {
		return nil
	}

	sub, ok := parts[0].(*syntax.CmdSubst)
	if !ok {
		return nil
	}

	call := plainCall(sub)
	if call == nil || slices.ContainsFunc(call.Args, func(w *syntax.Word) bool {
		return !isLiteralWord(w)
	}) {
		return nil
	}

	return wordsToStrings(call.Args)
}

// argumentLookup returns what an allowed lookup prints when nothing on the
// line can change it, as lookupOutput does for program words.
func (w *astWalker) argumentLookup(argv []string) (string, bool) {
	resolver, ok := w.resolver.(OutputResolver)
	if !ok || !ArgumentLookup(argv) || !w.lookupUnchanged() {
		return "", false
	}

	out, ok := resolver.CommandOutput(w.currentDir, argv)
	if !ok {
		return "", false
	}

	if slices.Equal(argv, branchShowCurrent) || slices.Equal(argv, revParseAbbrevHead) {
		return out, plainBranch.MatchString(out) && !strings.Contains(out, "..")
	}

	return out, plainPath.MatchString(out)
}

// writtenArg is how an argument was written. prefix is its literal text
// before the first expansion; literal reports no expansion at all. view is
// the argument with quoted glob, brace and blank characters neutralized,
// quoted expansions standing for nothing, unquoted variables kept as
// references and unquoted substitutions as splitMark, so splitting and
// globbing are found in it.
type writtenArg struct {
	prefix  string
	view    string
	literal bool
	tilde   bool
	splits  bool
	lookup  string
}

// writtenArgs records how each argument word was written, keyed by the
// argument it renders to, in the order the words appear, so two words that
// render alike ("$(pwd)" and "$(git branch --show-current)") stay apart.
func writtenArgs(words []*syntax.Word) map[string][]writtenArg {
	written := make(map[string][]writtenArg, len(words))

	for _, word := range words {
		key := markSubstituted(word, argWord(word))
		arg := writeArg(word)

		if argv := lookupArgv(word); ArgumentLookup(argv) {
			arg.lookup = strings.Join(argv, lookupSeparator)
		}

		written[key] = append(written[key], arg)
	}

	return written
}

// argWriter builds a writtenArg part by part.
type argWriter struct {
	prefix, view strings.Builder
	arg          writtenArg
	closed       bool
}

func (a *argWriter) text(rendered, neutral string) {
	a.view.WriteString(neutral)

	if !a.closed {
		a.prefix.WriteString(rendered)
	}
}

func (a *argWriter) expansion(shown string) {
	a.closed, a.arg.literal = true, false

	a.view.WriteString(shown)
}

func (a *argWriter) quoted(parts []syntax.WordPart) {
	a.view.WriteString(neutralGlob)

	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			value := renderLit(p.Value, true, doubleQuoteEscapable)
			a.text(value, neutralArg(value))
		case *syntax.ParamExp:
			if splitsQuoted(p) {
				a.expansion(paramExpToString(p))
			} else {
				a.expansion(neutralGlob)
			}
		default:
			a.expansion(neutralGlob)
		}
	}
}

func writeArg(word *syntax.Word) writtenArg {
	a := argWriter{arg: writtenArg{literal: true}}

	for i, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if i == 0 && strings.HasPrefix(p.Value, "~") {
				a.arg.tilde = true
				a.expansion(unescapedArg(p.Value))

				continue
			}

			a.text(renderLit(p.Value, true, allEscapable), unescapedArg(p.Value))
		case *syntax.SglQuoted:
			value := p.Value
			if p.Dollar {
				value = decodeANSIC(p.Value)
			}

			a.text(value, neutralArg(value)+neutralGlob)
		case *syntax.DblQuoted:
			a.quoted(p.Parts)
		case *syntax.ParamExp:
			a.expansion(paramExpToString(p))
		default:
			a.expansion(splitMark)
		}
	}

	a.arg.prefix, a.arg.view = a.prefix.String(), a.view.String()

	return a.arg
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

// splitsQuoted reports an expansion that gives several words even when
// quoted: "$@", "${a[@]}" and "${!prefix@}".
func splitsQuoted(exp *syntax.ParamExp) bool {
	return exp.Param != nil && exp.Param.Value == "@" || exp.Names != 0 ||
		strings.Contains(paramExpToString(exp), "[@]")
}

// writtenAs returns how arg was written: as the caller wrote it, for an
// argument an alias or function call re-quoted into the text it runs, or as
// written here. When that is unknown (an argument a launcher rendered, or
// literal text holding a substitution only re-quoting makes), every
// expansion and glob character in arg counts as unquoted.
// The words that render to arg are matched to its places in order, and only
// when there are as many of each.
func (w *astWalker) writtenAs(cmd Command, i int) writtenArg {
	arg := cmd.Args[i]
	if forwarded, ok := w.forwarded[arg]; ok {
		return forwarded
	}

	written := cmd.written[arg]
	if len(written) != countOf(cmd.Args, arg) {
		return unknownArg(arg)
	}

	return written[countOf(cmd.Args[:i], arg)]
}

func countOf(args []string, arg string) int {
	n := 0

	for _, a := range args {
		if a == arg {
			n++
		}
	}

	return n
}

func unknownArg(arg string) writtenArg {
	return writtenArg{
		view:    strings.ReplaceAll(arg, unresolvedWord, splitMark),
		literal: !marked(arg) && !HasUnresolvedVars(arg) && !strings.HasPrefix(arg, "~"),
		tilde:   strings.HasPrefix(arg, "~"),
	}
}

// forwardQuoted records how cmd's caller wrote the arguments it passes on
// in words the callee does not split again: an alias's arguments, or ones a
// function body quotes ("$1", "$@").
// The forwarded arguments are cmd's from start on.
func (w *astWalker) forwardQuoted(cmd Command, start int) map[string]writtenArg {
	indexes := make([]int, 0, len(cmd.Args)-start)
	for i := start; i < len(cmd.Args); i++ {
		indexes = append(indexes, i)
	}

	return w.forwardAt(cmd, indexes)
}

// forwardAt records how cmd's arguments at indexes were written. Two
// arguments with the same text written differently count as unknown.
func (w *astWalker) forwardAt(cmd Command, indexes []int) map[string]writtenArg {
	forward := make(map[string]writtenArg, len(indexes))

	for _, i := range indexes {
		arg, written := cmd.Args[i], w.writtenAs(cmd, i)
		if prev, ok := forward[arg]; ok && prev != written {
			written = unknownArg(arg)
		}

		forward[arg] = written
	}

	return forward
}

// forwardPositional records how a function body passes on the arguments of
// cmd. A positional parameter the body leaves unquoted ($1, $@) is split
// and globbed again, so the argument it holds counts as unquoted text.
func (w *astWalker) forwardPositional(cmd Command, body string) map[string]writtenArg {
	var quoted []int

	unquoted := make(map[string]bool)

	for _, ref := range positionalParam.FindAllString(body, -1) {
		first, last := 0, len(cmd.Args)

		param := strings.Trim(ref, `"${}`)
		if param != "@" && param != "*" {
			n := int(param[0] - '0')
			if n > len(cmd.Args) {
				continue
			}

			first, last = n-1, n
		}

		for i := first; i < last; i++ {
			if len(ref) > 1 && strings.HasPrefix(ref, `"`) && strings.HasSuffix(ref, `"`) {
				quoted = append(quoted, i)
			} else {
				unquoted[cmd.Args[i]] = true
			}
		}
	}

	forward := w.forwardAt(cmd, quoted)

	for arg := range unquoted {
		split := unknownArg(arg)
		split.splits = true
		forward[arg] = split
	}

	return forward
}

// forwardedArgs adds the arguments a caller re-quoted into a script to the
// ones its own callers did; the caller's own word wins.
func forwardedArgs(outer, forward map[string]writtenArg) map[string]writtenArg {
	if len(forward) == 0 {
		return outer
	}

	merged := maps.Clone(outer)
	if merged == nil {
		merged = make(map[string]writtenArg, len(forward))
	}

	maps.Copy(merged, forward)

	return merged
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

	values, end := gitOptionValues(cmd.Args, idx, sub)
	args := make([]string, 0, len(cmd.Args))

	for i, arg := range cmd.Args {
		if i == idx || (i < idx || i >= end) && sub == subcmdCommit {
			args = append(args, arg)

			continue
		}

		value, keep, detail := w.gitArg(cmd, i, sub, values)
		if detail != "" {
			w.opaque(OpacityUnresolvedWord, ArgumentOperation(sub), detail)

			return cmd
		}

		if keep {
			args = append(args, value)
		}
	}

	cmd.Args = args

	return cmd
}

// gitOptionValues reads the options after git's subcommand at idx left to
// right, as git does, and returns which arguments are the value of which
// option, and where -- ends the options.
func gitOptionValues(args []string, idx int, sub string) (map[int]string, int) {
	values := make(map[int]string)

	for i := idx + 1; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions:
			return values, i
		case marked(arg) || HasUnresolvedVars(arg) || !strings.HasPrefix(arg, "-"):
			continue
		}

		if flag := nextValueFlag(arg, sub); flag != "" && i+1 < len(args) {
			values[i+1] = flag
			i++
		}
	}

	return values, len(args)
}

// nextValueFlag returns the option in arg that takes the next argument as
// its value, or "" when the value is attached or none is taken. In a short
// cluster the first option that takes a value takes the rest of it.
func nextValueFlag(arg, sub string) string {
	if strings.HasPrefix(arg, "--") {
		if argValueFlags[sub][arg] {
			return arg
		}

		return ""
	}

	for j := 1; j < len(arg); j++ {
		flag := "-" + arg[j:j+1]

		switch {
		case argValueFlags[sub][flag] && j == len(arg)-1:
			return flag
		case argValueFlags[sub][flag] || strings.Contains(gluedValueFlags[sub], arg[j:j+1]):
			return ""
		}
	}

	return ""
}

// gitArg checks the argument at i of a git command. It returns the argument
// to keep in its place, whether to keep one, or why the argument cannot be
// known.
func (w *astWalker) gitArg(
	cmd Command,
	i int,
	sub string,
	values map[int]string,
) (string, bool, string) {
	arg := cmd.Args[i]
	written := w.writtenAs(cmd, i)

	if sub == subcmdPush && written.lookup != "" {
		argv := strings.Split(written.lookup, lookupSeparator)
		if value, ok := w.argumentLookup(argv); ok {
			return value, true, ""
		}
	}

	expanded, detail := w.splitView(written.view)
	if detail != "" {
		return arg, true, detail
	}

	if written.splits && strings.ContainsAny(arg, ifsBlanks) {
		return arg, true, DetailWordSplit
	}

	flag, isValue := values[i]

	switch {
	case sub == subcmdPush && attachedPushOption(written.prefix):
		return arg, true, ""
	case sub == subcmdPush && !uninspectedPushValues[flag]:
		return w.pushArg(arg, written, expanded)
	case isValue || sub == subcmdPush:
		return arg, true, ""
	default:
		return w.commitOption(arg, written, expanded)
	}
}

// attachedPushOption reports literal text that starts a push option with its
// value attached (-oci.skip, --push-option=x), a value no validator reads.
func attachedPushOption(prefix string) bool {
	if name, _, found := strings.Cut(prefix, "="); strings.HasPrefix(prefix, "--") {
		return found && uninspectedPushValues[name]
	}

	return strings.HasPrefix(prefix, "-") && strings.Contains(prefix[1:], "o")
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

	if written.tilde {
		home, ok := w.tildeHome(arg)
		if !ok {
			return arg, true, w.variableDetail()
		}

		arg = home + arg[1:]
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

// tildeHome returns the home directory a leading ~ or ~/ expands to. Other
// tilde forms (~user, ~+, ~-) name directories klaudiush does not resolve.
func (w *astWalker) tildeHome(arg string) (string, bool) {
	if len(arg) > 1 && arg[1] != '/' {
		return "", false
	}

	home, set, trusted := w.trustedValue("HOME")

	return home, set && trusted
}

// secretRef reports a variable in arg whose value comes from the
// environment rather than the line and would show in a validator's message
// once substituted: one that looks like a key or password, or any in the
// credentials of a URL.
func (w *astWalker) secretRef(arg string) bool {
	arg = w.applyDefaults(arg)
	userinfo := urlUserinfo(arg)

	for _, m := range varRefPattern.FindAllStringSubmatch(arg, -1) {
		if _, assigned := w.assignments[m[1]]; assigned {
			continue
		}

		value, set, trusted := w.trustedValue(m[1])
		if !trusted || !set {
			continue
		}

		if strings.Contains(userinfo, m[0]) || urlUserinfo(value) != "" ||
			slices.ContainsFunc(strings.FieldsFunc(value, urlSeparator), tokenLike) {
			return true
		}
	}

	return false
}

// urlUserinfo returns the credentials part of a URL in text, or "".
func urlUserinfo(text string) string {
	_, rest, found := strings.Cut(text, "://")
	if !found {
		return ""
	}

	host, _, _ := strings.Cut(rest, "/")

	userinfo, _, found := strings.Cut(host, "@")
	if !found {
		return ""
	}

	return userinfo
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
