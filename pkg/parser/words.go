package parser

import (
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// unresolvedWord marks an argument that takes part of its value from a
// command substitution, which renders partially or not at all. The mark
// keeps the argument's place and shows that a git or gh command word comes
// from command output. It uses private-use characters so that no word a user
// types is mistaken for it, and it is removed before the command is recorded.
const unresolvedWord = "\uE000$(...)\uE001"

// globChars expand against files when unquoted.
const globChars = "*?["

// ghActionCommands take an action as their second word (gh pr create).
var ghActionCommands = nameSet("issue pr")

// markSubstituted appends unresolvedWord to an argument with a command
// substitution in it.
func markSubstituted(word *syntax.Word, rendered string) string {
	if word != nil && hasCmdSubst(word.Parts) {
		return rendered + unresolvedWord
	}

	return rendered
}

func hasCmdSubst(parts []syntax.WordPart) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.CmdSubst:
			return true
		case *syntax.DblQuoted:
			if hasCmdSubst(p.Parts) {
				return true
			}
		}
	}

	return false
}

// bracesExpand reports a word with a brace expansion ({a,b} or {1..3}); find
// -exec's {} has none.
func bracesExpand(word string) bool {
	_, after, found := strings.Cut(word, "{")

	return found && (strings.Contains(after, ",") || strings.Contains(after, ".."))
}

func marked(arg string) bool {
	return strings.Contains(arg, unresolvedWord)
}

// gluedToFlag reports a short flag whose attached value was all
// substitution (-C$(pwd)). It is stored as the flag and an empty value, so
// the bare flag does not take the next argument as its value.
func gluedToFlag(arg, value string) bool {
	return strings.HasSuffix(arg, unresolvedWord) && strings.HasPrefix(value, "-") &&
		!strings.HasPrefix(value, "--")
}

// storedArgs removes the marks from cmd's arguments. An argument that was
// all substitution is dropped, as the parser always did, unless it is the
// value of a git global option or of a flag: keeping it empty there stops
// the flag from taking the next argument (git -C "$(pwd)" push).
func storedArgs(cmd Command) []string {
	if !slices.ContainsFunc(cmd.Args, marked) {
		return cmd.Args
	}

	sub, idx := "", -1
	if cmd.Name == gitProgram {
		if idx = gitSubcommandIndex(cmd.Args); idx >= 0 {
			sub = cmd.Args[idx]
		}
	}

	args := make([]string, 0, len(cmd.Args))

	for i, arg := range cmd.Args {
		value := strings.ReplaceAll(arg, unresolvedWord, "")
		if gluedToFlag(arg, value) {
			args = append(args, value, "")

			continue
		}

		if value != "" || (marked(arg) && keepsEmpty(cmd, i, idx, sub)) {
			args = append(args, value)
		}
	}

	return args
}

// keepsEmpty reports whether the substituted argument at i fills a value
// slot that must stay in place.
func keepsEmpty(cmd Command, i, idx int, sub string) bool {
	switch {
	case cmd.Name == gitProgram && (idx < 0 || i < idx):
		return true
	case i == 0:
		return false
	case cmd.Name == gitProgram:
		return flagTakesValue(cmd.Args[i-1], sub)
	case cmd.Name == ghCLI:
		return strings.HasPrefix(cmd.Args[i-1], "-") && cmd.Args[i-1] != endOfOptions
	default:
		return false
	}
}

// prepare looks at a top-level statement before it is walked. A variable
// assigned anywhere but as a plain statement of its own (in a subshell,
// pipeline, condition, loop, function, background job or as a command
// prefix) may or may not hold that value later, so it is forgotten rather
// than resolved. Commands in a loop see variables a later iteration may
// change, so they resolve none.
func (w *astWalker) prepare(stmt *syntax.Stmt) {
	markSafeAssigns(stmt, w.safeAssigns)

	syntax.Walk(stmt, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.WhileClause, *syntax.ForClause:
			syntax.Walk(n, func(inner syntax.Node) bool {
				if call, ok := inner.(*syntax.CallExpr); ok {
					w.loopCalls[call] = true
				}

				return true
			})

			return false
		default:
			return true
		}
	})
}

// markSafeAssigns records the assignments stmt always makes in the current
// shell: a statement of plain assignments, export or declare without -n,
// and either of those leading an && chain.
func markSafeAssigns(stmt *syntax.Stmt, safe map[*syntax.Assign]bool) {
	if stmt == nil || stmt.Background || stmt.Coprocess {
		return
	}

	switch c := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		if len(c.Args) == 0 {
			for _, a := range c.Assigns {
				safe[a] = true
			}
		}
	case *syntax.DeclClause:
		if slices.ContainsFunc(c.Args, namesReference) {
			return
		}

		for _, a := range c.Args {
			safe[a] = true
		}
	case *syntax.BinaryCmd:
		if c.Op == syntax.AndStmt {
			markSafeAssigns(c.X, safe)
		}
	}
}

// namesReference reports a declare option that makes a variable a reference
// to another (-n), or one klaudiush cannot read.
func namesReference(a *syntax.Assign) bool {
	if a.Name != nil || a.Value == nil {
		return false
	}

	option := wordToString(a.Value)

	return !strings.HasPrefix(option, "-") && !strings.HasPrefix(option, "+") ||
		strings.Contains(option, "n")
}

// forgetUnlessSafe forgets a variable assigned somewhere its value may not
// hold.
func (w *astWalker) forgetUnlessSafe(assign *syntax.Assign) {
	if assign.Name != nil && !w.safeAssigns[assign] {
		w.forget(assign.Name.Value)
	}
}

// resolveWord expands the variables in a rendered word from assignments made
// earlier on the line, then the environment. It reports false when a
// reference stays unknown or holds command output.
func (w *astWalker) resolveWord(word string) (string, bool) {
	known := true

	expanded := expandVars(word, func(name string) (string, bool) {
		if w.inLoop || w.state.untrusted || w.state.dynamicVars[name] || w.unknownVars[name] {
			known = false

			return "", false
		}

		value, ok := w.assignments[name]
		if !ok {
			value, ok = w.resolver.LookupEnv(name)
		}

		if ok {
			w.noteExpanded(value)
		}

		return value, ok
	})

	return expanded, known && !HasUnresolvedVars(expanded)
}

// commandWordDetail says why a git or gh command word cannot be known, or
// returns "" when it is literal.
func commandWordDetail(word string) string {
	switch {
	case HasUnresolvedVars(word):
		return DetailWordVariable
	case marked(word) || strings.ContainsAny(word, globChars) || bracesExpand(word):
		return DetailWordOutput
	default:
		return ""
	}
}

// resolveGitSubcommand substitutes variables in git's subcommand word, so
// x=status; git $x is checked as git status. A subcommand from an unknown
// variable, command output or a glob is opaque: it could be any subcommand.
// Each pass replaces one dynamic word with literal ones, so it ends.
func (w *astWalker) resolveGitSubcommand(cmd Command) (Command, bool) {
	for {
		idx := gitSubcommandIndex(cmd.Args)
		if idx < 0 {
			return cmd, true
		}

		resolved, ok := w.resolveCommandWord(cmd.Args[idx], gitProgram)
		if !ok || resolved == nil {
			return cmd, ok
		}

		cmd.Args = slices.Concat(cmd.Args[:idx], resolved, cmd.Args[idx+1:])
	}
}

// resolveGHCommand does for gh what resolveGitSubcommand does for git, for
// the command word and, for pr and issue, the action after it.
func (w *astWalker) resolveGHCommand(cmd Command) (Command, bool) {
	for pos := 0; pos < len(cmd.Args) && pos < 2; {
		if pos == 1 && !ghActionCommands[cmd.Args[0]] {
			break
		}

		resolved, ok := w.resolveCommandWord(cmd.Args[pos], ghCLI)
		if !ok {
			return cmd, false
		}

		if resolved == nil {
			pos++

			continue
		}

		cmd.Args = slices.Concat(cmd.Args[:pos], resolved, cmd.Args[pos+1:])
	}

	return cmd, true
}

// resolveCommandWord returns the words a dynamic command word expands to, nil
// for a literal word, or false after recording why it cannot be known. The
// expansion is split into words even when quoted, which only leaves more of
// it checked.
func (w *astWalker) resolveCommandWord(word, program string) ([]string, bool) {
	detail := commandWordDetail(word)
	if detail == "" {
		return nil, true
	}

	if detail == DetailWordVariable {
		if expanded, ok := w.resolveWord(word); ok {
			detail = commandWordDetail(expanded)
			if detail == "" {
				return nonNil(strings.Fields(expanded)), true
			}
		}
	}

	w.opaque(OpacityUnresolvedWord, program, detail)

	return nil, false
}

// nonNil keeps an expansion to no words distinct from a literal word.
func nonNil(words []string) []string {
	if words == nil {
		return []string{}
	}

	return words
}

// noteExpanded remembers the words of a variable's value, which may be a
// secret, so no diagnostic of the parse shows them, even after eval runs a
// line the value was substituted into.
func (w *astWalker) noteExpanded(value string) {
	if w.state.expandedWords == nil {
		w.state.expandedWords = make(map[string]bool)
	}

	for word := range strings.FieldsSeq(value) {
		w.state.expandedWords[word] = true
	}
}

// shownWord is safeName, hiding a word that came from a variable's value.
func (w *astWalker) shownWord(word string) string {
	if w.state.expandedWords[word] {
		return hiddenName
	}

	return safeName(word)
}

// resolveEval returns the command line eval runs, with the variables it
// names substituted. A line built from an unknown variable or from command
// output is opaque.
func (w *astWalker) resolveEval(cmd Command) (string, bool) {
	line := strings.Join(cmd.Args, " ")

	if cmd.Dynamic || strings.Contains(line, unresolvedWord) {
		w.opaque(OpacityUnresolvedWord, cmd.Name, DetailWordOutput)

		return line, false
	}

	if !HasUnresolvedVars(line) {
		return line, true
	}

	expanded, ok := w.resolveWord(line)
	if !ok {
		w.opaque(OpacityUnresolvedWord, cmd.Name, DetailWordVariable)

		return line, false
	}

	return expanded, true
}

// launchedFrom returns what cmd runs. The commands it launches keep the
// stand-ins of followed, so env git $(...) is still seen; scripts and files
// are found as before. Eval runs its line with known variables substituted.
func (w *astWalker) launchedFrom(cmd, followed Command) launch {
	l := launched(cmd)

	if slices.ContainsFunc(followed.Args, marked) {
		l.commands = launched(followed).commands
	}

	if cmd.Name == evalBuiltin {
		if line, ok := w.resolveEval(followed); ok {
			l.scripts = []string{line}
		}
	}

	return l
}

// forget records that name was set in a way the parser does not follow
// (read, printf -v, a loop, or an assignment in a function, eval or sourced
// script), so its literal value here and in the scripts that ran this one,
// or its value in the environment, is stale.
func (w *astWalker) forget(name string) {
	for p := w; p != nil; p = p.parent {
		p.unknownVars[name] = true
	}
}

// varWriters are the builtins that set variables named in their arguments.
var varWriters = nameSet("read mapfile readarray getopts unset " + printfBuiltin)

// declWriters are the declaration builtins. Run through builtin or command
// they are plain commands rather than declarations, so their assignments
// are not followed.
var declWriters = nameSet("declare export typeset readonly local")

// mapfiles run the code given to -C as a callback, which may assign
// anything. An unresolved program name may be a declaration too, and one
// that expands to NAME=value is an assignment once source or eval reads the
// text it came from.
var mapfiles = nameSet("mapfile readarray")

// callbackFlag reports a mapfile option cluster with -C in it (-C, -tC,
// -C'code'), which runs code that may assign anything.
func callbackFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
		strings.Contains(arg, "C")
}

// forgetDeclared forgets every variable a declaration run as a command sets,
// and stops trusting any after an option that changes values or makes
// references, or an operand that is not literal.
func (w *astWalker) forgetDeclared(cmd Command) {
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+") {
			if strings.ContainsAny(arg[1:], "lucn") {
				w.state.untrusted = true
			}

			continue
		}

		name, _, _ := strings.Cut(arg, "=")
		if !variableName.MatchString(name) {
			w.state.untrusted = true

			continue
		}

		w.forget(name)
	}
}

// defaultVars are the variables a writer sets when it names none.
var defaultVars = map[string]string{"read": "REPLY", "mapfile": "MAPFILE", "readarray": "MAPFILE"}

// forgetWritten forgets the variables cmd sets other than by assignment.
func (w *astWalker) forgetWritten(cmd Command) {
	if HasUnresolvedVars(cmd.Invoked) || strings.Contains(cmd.Invoked, unresolvedProgram) ||
		assignmentPattern.MatchString(
			cmd.Invoked,
		) || cmd.Name == sourceBuiltin || cmd.Name == dotBuiltin ||
		(mapfiles[cmd.Name] && slices.ContainsFunc(cmd.Args, callbackFlag)) {
		w.state.untrusted = true

		return
	}

	if declWriters[cmd.Name] {
		w.forgetDeclared(cmd)

		return
	}

	if !varWriters[cmd.Name] {
		return
	}

	names := writtenVars(cmd)
	if len(names) == 0 && defaultVars[cmd.Name] != "" {
		names = []string{defaultVars[cmd.Name]}
	}

	for _, name := range names {
		w.forgetName(name)
	}
}

// forgetName forgets a written variable. A target built from an expansion
// or naming an element ("$v", x[0]) could be any variable, so after it none
// is trusted. Other words (a prompt, a timeout) name no variable.
func (w *astWalker) forgetName(name string) {
	switch {
	case variableName.MatchString(name):
		w.forget(name)
	case strings.ContainsAny(name, "$[") || marked(name):
		w.state.untrusted = true
	}
}

// variableName matches a plain shell variable name.
var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// distrustDecl stops trusting any variable after a declare whose operand is
// not a literal assignment (declare "$v", export $v=x) or whose options
// change values or make references (-l, -u, -c, -n).
func (w *astWalker) distrustDecl(decl *syntax.DeclClause) {
	for _, a := range decl.Args {
		if a.Name != nil || a.Value == nil {
			continue
		}

		option := wordToString(a.Value)
		if !strings.HasPrefix(option, "-") && !strings.HasPrefix(option, "+") ||
			strings.ContainsAny(option, "lucn") {
			w.state.untrusted = true
		}
	}
}

// writtenVars returns the words cmd may write to: every operand and flag
// value, since an empty value such as read -d ” leaves no word behind, and
// for printf only the value of -v, given apart or attached.
func writtenVars(cmd Command) []string {
	var names []string

	for i := 0; i < len(cmd.Args); i++ {
		arg := cmd.Args[i]

		switch {
		case arg == "-a" || arg == "-v":
			if i+1 < len(cmd.Args) {
				names = append(names, cmd.Args[i+1])
			}

			i++
		case strings.HasPrefix(arg, "-v") || strings.HasPrefix(arg, "-a"):
			names = append(names, arg[2:])
		case strings.HasPrefix(arg, "-"):
		case cmd.Name == printfBuiltin:
			return names
		default:
			names = append(names, arg)
		}
	}

	return names
}
