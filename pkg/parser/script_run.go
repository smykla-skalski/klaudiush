package parser

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// scriptRun is what a script file knows about how it was run: $0, and its
// positional parameters when withArgs is set. Both are unknown at the top
// level and in new shells started with -c. file is the script file being
// read, run or sourced, which ${BASH_SOURCE[0]} names.
type scriptRun struct {
	zero     string
	args     []string
	withArgs bool
	file     string
}

// OutputResolver is an optional Resolver extension that prints the output of
// a fixed lookup command (see AllowedLookups) run in dir, as the shell would
// substitute it. A Resolver without it leaves every substitution opaque.
type OutputResolver interface {
	CommandOutput(dir string, argv []string) (string, bool)
}

// AllowedLookups are the only command substitutions whose output a program
// word is built from: they print a fixed directory and change nothing.
var AllowedLookups = [][]string{
	{brewProgram, "--prefix"},
	{poetryProgram, "env", "info", "--path"},
	{condaProgram, "info", "--base"},
	{"go", goEnv, "GOPATH"},
	{"go", goEnv, "GOBIN"},
	revParseToplevel,
}

const (
	brewProgram     = "brew"
	brewFormulaArgs = 3
	poetryProgram   = "poetry"
	condaProgram    = "conda"
	goEnv           = "env"
	revParse        = "rev-parse"
)

var revParseToplevel = []string{gitProgram, revParse, "--show-toplevel"}

var brewFormula = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9@+_.-]*(/[A-Za-z0-9][A-Za-z0-9@+_.-]*){0,2}$`,
)

// AllowedLookup reports whether argv is one of AllowedLookups.
func AllowedLookup(argv []string) bool {
	if slices.ContainsFunc(AllowedLookups, func(allowed []string) bool {
		return slices.Equal(allowed, argv)
	}) {
		return true
	}

	return len(argv) == brewFormulaArgs && argv[0] == brewProgram && argv[1] == "--prefix" &&
		brewFormula.MatchString(argv[2])
}

// plainPath matches lookup output safe to put in a program word: one
// absolute path with nothing the shell would split, glob or expand.
var plainPath = regexp.MustCompile(`^/[A-Za-z0-9_./@+:,=-]*$`)

// setupCommands may run earlier on the line without changing what a lookup
// prints.
var setupCommands = nameSet("cd pushd popd pwd set true : echo printf test [")

// lookupEnvPrefixes and lookupEnvNames are variables allowed lookups read,
// which an assignment on the line would change for the shell but not for
// klaudiush's own lookup.
var (
	lookupEnvPrefixes = []string{
		"GIT_", "GO", "CGO_", "HOMEBREW_", "POETRY_", "CONDA_", "PYTHON", "RUBY", "_CE_",
	}
	lookupEnvNames = nameSet(
		"HOME XDG_CONFIG_HOME XDG_DATA_HOME XDG_CACHE_HOME PATH BASH_ENV ENV CDPATH VIRTUAL_ENV",
	)
)

// substitutedProgram returns what a command substitution in a program word
// prints: a lookup of a program's path, the directory of the running script,
// or the output of an allowed lookup when nothing on the line changes it.
func (w *astWalker) substitutedProgram(sub *syntax.CmdSubst) string {
	call := plainCall(sub)
	if call == nil {
		return lookupProgram(sub)
	}

	if dir, ok := w.scriptDir(call.Args); ok {
		return dir
	}

	if out, ok := w.lookupOutput(call.Args); ok {
		return out
	}

	return lookupProgram(sub)
}

// plainCall returns the one simple command a substitution runs, with no
// assignments, redirects or background.
func plainCall(sub *syntax.CmdSubst) *syntax.CallExpr {
	if len(sub.Stmts) != 1 {
		return nil
	}

	stmt := sub.Stmts[0]
	if stmt.Negated || stmt.Background || stmt.Coprocess || len(stmt.Redirs) > 0 {
		return nil
	}

	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return nil
	}

	return call
}

// lookupOutput runs an allowed lookup written in literal words through the
// resolver.
func (w *astWalker) lookupOutput(words []*syntax.Word) (string, bool) {
	if !slices.ContainsFunc(words, func(word *syntax.Word) bool { return !isLiteralWord(word) }) {
		argv := wordsToStrings(words)

		resolver, ok := w.resolver.(OutputResolver)
		if !ok || !AllowedLookup(argv) || !w.lookupUnchanged(argv[0]) {
			return "", false
		}

		out, ok := resolver.CommandOutput(w.currentDir, argv)
		if ok && plainPath.MatchString(out) {
			return out, true
		}
	}

	return "", false
}

// lookupUnchanged reports whether a lookup klaudiush runs now prints what
// the shell's would: the directory is known, no loop repeats it, and nothing
// earlier on the line changes the environment, PATH, lookup program, or files
// a lookup reads. Commands that launched this script are part of running it.
func (w *astWalker) lookupUnchanged(program string) bool {
	if w.inLoop || w.dirUnknown || w.dirComputed || w.state.pathChanged || w.state.untrusted ||
		w.defined(program) || w.lookupEnvChanged() {
		return false
	}

	seq := -1

	for p := w; p != nil; p = p.parent {
		if len(p.fileWrites) > 0 || p.dynamicWrites > 0 {
			return false
		}

		if slices.ContainsFunc(p.commands, func(cmd Command) bool {
			return cmd.Location.Seq != seq && !setupCommands[cmd.Name]
		}) {
			return false
		}

		seq = p.launchSeq
	}

	return true
}

// lookupEnvChanged reports an environment the lookup may not share with the
// shell: CDPATH sending a cd elsewhere, a new shell sourcing BASH_ENV or ENV
// first, or a variable an allowed lookup reads assigned on the line.
func (w *astWalker) lookupEnvChanged() bool {
	if cdpath, set := w.resolver.LookupEnv("CDPATH"); set && cdpath != "" && w.currentDir != "" {
		return true
	}

	if w.distrust && slices.ContainsFunc([]string{"BASH_ENV", "ENV"}, func(name string) bool {
		_, set := w.resolver.LookupEnv(name)

		return set
	}) {
		return true
	}

	return slices.ContainsFunc(w.touchedVars(), lookupVar)
}

// touchedVars names every variable assigned, forgotten or made dynamic on
// the line so far.
func (w *astWalker) touchedVars() []string {
	var names []string

	for p := w; p != nil; p = p.parent {
		for name := range p.assignments {
			names = append(names, name)
		}

		for name := range p.unknownVars {
			names = append(names, name)
		}
	}

	for name := range w.state.dynamicVars {
		names = append(names, name)
	}

	return names
}

func lookupVar(name string) bool {
	return lookupEnvNames[name] || slices.ContainsFunc(lookupEnvPrefixes, func(prefix string) bool {
		return strings.HasPrefix(name, prefix)
	})
}

// scriptDir returns the directory dirname "$0" prints in a script file
// whose $0 is known, or dirname "${BASH_SOURCE[0]}" in a script file read
// by path.
func (w *astWalker) scriptDir(words []*syntax.Word) (string, bool) {
	if len(words) < 2 || len(words) > 3 || w.defined(dirnameProgram) ||
		!isLiteralWord(words[0]) || wordToString(words[0]) != dirnameProgram {
		return "", false
	}

	if len(words) == 3 && (!isLiteralWord(words[1]) || wordToString(words[1]) != endOfOptions) {
		return "", false
	}

	_, pe := singleParam(words[len(words)-1])

	switch {
	case pe == nil:
		return "", false
	case simpleParam(pe, "0") && w.scriptRun.zero != "":
		return dirname(w.scriptRun.zero), true
	case bashSourceParam(pe) && w.scriptRun.file != "":
		return dirname(w.scriptRun.file), true
	default:
		return "", false
	}
}

// bashSourceParam reports $BASH_SOURCE or ${BASH_SOURCE[0]}, the file the
// current code was read from.
func bashSourceParam(pe *syntax.ParamExp) bool {
	rendered := paramExpToString(pe)

	return pe.Param.Value == "BASH_SOURCE" &&
		(rendered == "${BASH_SOURCE}" || rendered == "${BASH_SOURCE[0]}")
}

// scriptDirParam renders a parameter in a program word, giving ${0%/*} its
// value in a script file whose $0 is known.
func (w *astWalker) scriptDirParam(pe *syntax.ParamExp) string {
	rendered := paramExpToString(pe)
	if w.scriptRun.zero == "" || rendered != "${0%/*}" {
		return rendered
	}

	if i := strings.LastIndexByte(w.scriptRun.zero, '/'); i >= 0 {
		return w.scriptRun.zero[:i]
	}

	return w.scriptRun.zero
}

// dirname returns what dirname prints for path.
func dirname(path string) string {
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" {
		return "/"
	}

	before, _, found := strings.CutLast(trimmed, "/")
	if !found {
		return "."
	}

	if dir := strings.TrimRight(before, "/"); dir != "" {
		return dir
	}

	return "/"
}

// singleParam returns the parameter expansion that makes up all of word,
// and whether it is double-quoted.
func singleParam(word *syntax.Word) (bool, *syntax.ParamExp) {
	if word == nil || len(word.Parts) != 1 {
		return false, nil
	}

	part, quoted := word.Parts[0], false
	if dq, ok := part.(*syntax.DblQuoted); ok {
		if len(dq.Parts) != 1 {
			return false, nil
		}

		part, quoted = dq.Parts[0], true
	}

	pe, ok := part.(*syntax.ParamExp)
	if !ok || pe.Param == nil {
		return false, nil
	}

	return quoted, pe
}

// simpleParam reports a plain $name or ${name} reference to name.
func simpleParam(pe *syntax.ParamExp, name string) bool {
	return pe.Param.Value == name && paramExpToString(pe) == "${"+name+"}"
}

// knownZero reports whether a script file path is what $0 holds in it.
func knownZero(path string) bool {
	return path != "" && path != "-" && path != devStdin &&
		!strings.HasPrefix(path, procSubstPrefix) && !HasUnresolvedVars(path) &&
		!strings.Contains(path, unresolvedProgram) && !marked(path)
}

// fileRun returns $0 and the positional parameters of a shell script file
// cmd runs. An executed script's $0 is its path and its positional
// parameters are the arguments after it, known only when cmd ran directly
// (xargs, for one, adds more). A sourced script keeps the caller's, unless
// source passes arguments.
func (w *astWalker) fileRun(cmd Command, file scriptFile, depth int) scriptRun {
	if cmd.Name == sourceBuiltin || cmd.Name == dotBuiltin {
		run := w.scriptRun
		if file.withArgs {
			run.args, run.withArgs = file.args, true
		}

		run.file = ""
		if knownZero(file.path) {
			run.file = file.path
		}

		return run
	}

	var run scriptRun

	if knownZero(file.path) {
		run.zero, run.file = file.path, file.path
	}

	if file.withArgs && depth-1 == w.depth {
		run.args, run.withArgs = file.args, true
	}

	return run
}

// childRun returns what a script parent runs knows of $0 and the positional
// parameters. Functions and aliases have their arguments substituted before
// they are walked, so they keep only $0; eval and trap share the caller's.
func (w *astWalker) childRun(parent Command, sw scriptWalk) scriptRun {
	switch {
	case sw.file:
		return sw.run
	case sw.literal, strings.HasPrefix(sw.name, "git:"), strings.HasPrefix(sw.name, "gh:"):
		return scriptRun{}
	case sw.name != "":
		return scriptRun{zero: w.scriptRun.zero}
	case runsInShell(parent, sw):
		return w.scriptRun
	default:
		return scriptRun{}
	}
}

// withoutPartialArgs drops the positional parameters of the scripts a
// command runs when one of its arguments was all or part command output,
// which the stored arguments leave out.
func withoutPartialArgs(files []scriptFile, followed Command) []scriptFile {
	if !slices.ContainsFunc(followed.Args, marked) {
		return files
	}

	for i := range files {
		files[i].args, files[i].withArgs = nil, false
	}

	return files
}

// argsAfter returns the arguments after a shell's script operand.
func argsAfter(args []string, operand string) []string {
	for i, arg := range args {
		if arg != operand {
			continue
		}

		if found, isScript, ok := shellOperand(args[:i+1]); ok && !isScript && found == operand {
			return args[i+1:]
		}
	}

	return nil
}

// trackPositional forgets the positional parameters after set or shift
// changes them, here and in the scripts that ran this one.
func (w *astWalker) trackPositional(cmd Command) {
	if cmd.Name == setBuiltin && !setsPositional(cmd.Args) {
		return
	}

	for p := w; p != nil; p = p.parent {
		p.scriptRun.args, p.scriptRun.withArgs = nil, false
	}
}

// setsPositional reports set arguments that replace the positional
// parameters: --, -, or an operand that is not an -o option name.
func setsPositional(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions || arg == "-":
			return true
		case strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+"):
			if strings.HasSuffix(arg, "o") {
				i++
			}
		default:
			return true
		}
	}

	return false
}

// positionalWords returns the words "$@", "$*" or "$1" to "$9" expand to
// when the positional parameters are known and the word is only that
// reference. Unquoted, they split on spaces; after an IFS change, or in a
// loop that may shift them, they are not known.
func (w *astWalker) positionalWords(word *syntax.Word) ([]string, bool) {
	if !w.scriptRun.withArgs || w.inLoop || w.state.untrusted {
		return nil, false
	}

	quoted, pe := singleParam(word)
	if pe == nil || !simpleParam(pe, pe.Param.Value) {
		return nil, false
	}

	args := w.scriptRun.args

	var values []string

	switch name := pe.Param.Value; {
	case name == "@":
		values = slices.Clone(args)
	case name == "*" && quoted:
		values = []string{strings.Join(args, " ")}
	case name == "*":
		values = slices.Clone(args)
	case len(name) == 1 && name >= "1" && name <= "9":
		n, _ := strconv.Atoi(name)

		values = []string{""}
		if n <= len(args) {
			values = []string{args[n-1]}
		}
	default:
		return nil, false
	}

	if quoted {
		return values, true
	}

	var fields []string
	for _, value := range values {
		fields = append(fields, strings.Fields(value)...)
	}

	return fields, true
}

// withoutEmptyPositional drops leading words that expand to no words, so
// the next one is the program, as with "$@" and no arguments.
func (w *astWalker) withoutEmptyPositional(words []*syntax.Word) []*syntax.Word {
	if !w.scriptRun.withArgs {
		return words
	}

	for len(words) > 0 {
		values, ok := w.positionalWords(words[0])
		if !ok || len(values) > 0 {
			break
		}

		words = words[1:]
	}

	return words
}

// positionalWord reports a word positionalWords expands.
func (w *astWalker) positionalWord(word *syntax.Word) bool {
	_, ok := w.positionalWords(word)

	return ok
}

// callWords renders a call's program and arguments, putting the known
// positional parameters in place of the words that reference them.
func (w *astWalker) callWords(words []*syntax.Word) (string, []string, string) {
	var name, view string

	var args []string

	if values, ok := w.positionalWords(words[0]); ok {
		name, args, view = values[0], slices.Clone(values[1:]), values[0]
		if quoted, _ := singleParam(words[0]); quoted {
			view = neutralize(view)
		}
	} else {
		name, view = w.commandWord(words[0]), globView(words[0])
	}

	for _, word := range words[1:] {
		if values, ok := w.positionalWords(word); ok {
			args = append(args, values...)

			continue
		}

		if path, ok := w.sourcedFile(name, args, word); ok {
			args = append(args, path)

			continue
		}

		args = append(args, w.argStrings([]*syntax.Word{word})...)
	}

	return name, args, view
}
