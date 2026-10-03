package parser

import (
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
		if w.inLoop || w.state.dynamicVars[name] || w.unknownVars[name] {
			known = false

			return "", false
		}

		if value, ok := w.assignments[name]; ok {
			return value, true
		}

		return w.resolver.LookupEnv(name)
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
				fields := nonNil(strings.Fields(expanded))
				w.noteExpanded(fields)

				return fields, true
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

// noteExpanded remembers words that came from a variable's value, which may
// be a secret, so a diagnostic never shows them.
func (w *astWalker) noteExpanded(words []string) {
	if w.expanded == nil {
		w.expanded = make(map[string]bool)
	}

	for _, word := range words {
		w.expanded[word] = true
	}
}

// shownWord is safeName, hiding a word that came from a variable's value.
func (w *astWalker) shownWord(word string) string {
	if w.expanded[word] {
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

// varWriters are the builtins that set variables named in their arguments,
// with the flags of each that take a value.
var varWriters = map[string][]string{
	"read":        strings.Fields("-d -i -n -N -p -t -u"),
	"mapfile":     strings.Fields("-d -n -O -s -u -C -c"),
	"readarray":   strings.Fields("-d -n -O -s -u -C -c"),
	printfBuiltin: nil,
	"getopts":     nil,
	"unset":       nil,
}

// defaultVars are the variables a writer sets when it names none.
var defaultVars = map[string]string{"read": "REPLY", "mapfile": "MAPFILE", "readarray": "MAPFILE"}

// forgetWritten forgets the variables cmd sets other than by assignment.
func (w *astWalker) forgetWritten(cmd Command) {
	valueFlags, ok := varWriters[cmd.Name]
	if !ok {
		return
	}

	names := writtenVars(cmd, valueFlags)
	if len(names) == 0 && defaultVars[cmd.Name] != "" {
		names = []string{defaultVars[cmd.Name]}
	}

	for _, name := range names {
		w.forget(name)
	}
}

// writtenVars returns the variable names cmd writes: its operands, and the
// value of -a (read) or -v (printf), given apart or attached. printf writes
// only through -v.
func writtenVars(cmd Command, valueFlags []string) []string {
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
		case slices.Contains(valueFlags, arg):
			i++
		case strings.HasPrefix(arg, "-"):
		case cmd.Name == printfBuiltin:
			return names
		default:
			names = append(names, arg)
		}
	}

	return names
}
