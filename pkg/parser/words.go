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

// markSubstituted appends unresolvedWord to an argument with a command or
// process substitution in it.
func markSubstituted(word *syntax.Word, rendered string) string {
	if word != nil && hasCmdSubst(word.Parts) {
		return rendered + unresolvedWord
	}

	return rendered
}

func hasCmdSubst(parts []syntax.WordPart) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp:
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

// combinedTakesValue reports combined short flags (-sSF) whose last flag
// takes the next argument as its value.
func combinedTakesValue(arg, sub string) bool {
	if len(arg) < len("-sF") || arg[0] != '-' || arg[1] == '-' {
		return false
	}

	return flagTakesValue("-"+arg[len(arg)-1:], sub)
}

// storedArgs removes the marks from cmd's arguments. A quoted empty word
// stays, as in the shell. An argument that was all substitution is dropped,
// as the parser always did, unless it is the value of a git global option
// or of a flag: keeping it empty there stops the flag from taking the next
// argument (git -C "$(pwd)" push). The second result marks, per stored
// argument, whether it was substituted.
func storedArgs(cmd Command) ([]string, []bool) {
	if !slices.ContainsFunc(cmd.Args, marked) {
		return cmd.Args, nil
	}

	sub, idx := "", -1
	if cmd.Name == gitProgram {
		if idx = gitSubcommandIndex(cmd.Args); idx >= 0 {
			sub = cmd.Args[idx]
		}
	}

	args := make([]string, 0, len(cmd.Args))
	substituted := make([]bool, 0, len(cmd.Args))

	for i, arg := range cmd.Args {
		value := strings.ReplaceAll(arg, unresolvedWord, "")
		if gluedToFlag(arg, value) {
			args = append(args, value, "")
			substituted = append(substituted, true, true)

			continue
		}

		if value != "" || !marked(arg) || keepsEmpty(cmd, i, idx, sub) {
			args = append(args, value)
			substituted = append(substituted, marked(arg))
		}
	}

	return args, substituted
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
		return flagTakesValue(cmd.Args[i-1], sub) || combinedTakesValue(cmd.Args[i-1], sub)
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
	markSafeAssigns(stmt, w.safeAssigns, w.chainAssigns)
	markSafeNamerefs(stmt, w.safeNamerefs)
	markCertainStmts(stmt, w.certain)
	w.stmtFuncs = funcBodies(stmt)

	syntax.Walk(stmt, func(node syntax.Node) bool {
		w.noteArithmetic(node)

		switch n := node.(type) {
		case *syntax.WhileClause, *syntax.ForClause:
			walkLoop(n, func(inner syntax.Node, param bool) {
				w.noteLoopStartup(inner, param)

				if call, ok := inner.(*syntax.CallExpr); ok {
					w.loopCalls[call] = true
				}
			})

			return false
		case *syntax.ParamExp:
			w.forgetAssigned(n)

			return true
		default:
			return true
		}
	})
}

// markSafeNamerefs records nameref assignments made by a plain declaration.
// A declaration nested in control flow may not run, so its target is unknown.
func markSafeNamerefs(stmt *syntax.Stmt, safe map[*syntax.Assign]bool) {
	if stmt == nil || stmt.Background || stmt.Coprocess || stmt.Negated {
		return
	}

	decl, ok := stmt.Cmd.(*syntax.DeclClause)
	if !ok || !declHasNameref(decl) {
		return
	}

	for _, assign := range decl.Args {
		if assign.Name != nil {
			safe[assign] = true
		}
	}
}

// elementAssign reports an assignment to one element (a[1]=x) or an array
// with indexed elements (a=([1]=x)), whose joined value the parser does not
// model, so the variable is unknown after it.
func elementAssign(assign *syntax.Assign) bool {
	if assign.Index != nil {
		return true
	}

	return assign.Array != nil && slices.ContainsFunc(
		assign.Array.Elems,
		func(e *syntax.ArrayElem) bool { return e.Index != nil },
	)
}

// allElements matches ${NAME[@]} or ${NAME[*]}, which eval cannot take as
// text: quoted, it still splits into one word per element.
var allElements = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\[[@*]\]\}`)

// dynamicElements reports an array with an element from command output.
func dynamicElements(array *syntax.ArrayExpr) bool {
	return array != nil && slices.ContainsFunc(array.Elems, func(e *syntax.ArrayElem) bool {
		return e.Value != nil && wordDynamic(e.Value)
	})
}

// markArray records that name holds an array: $name is then only its first
// element, which the joined value kept for it does not show.
func (w *astWalker) markArray(name string) {
	if w.state.arrays == nil {
		w.state.arrays = make(map[string]bool)
	}

	w.state.arrays[name] = true
}

// arrayDecl reports a declaration that makes its names arrays (-a, -A).
func arrayDecl(decl *syntax.DeclClause) bool {
	return slices.ContainsFunc(decl.Args, func(a *syntax.Assign) bool {
		if a.Name != nil || a.Value == nil {
			return false
		}

		option := wordToString(a.Value)

		return (strings.HasPrefix(option, "-") || strings.HasPrefix(option, "+")) &&
			strings.ContainsAny(option, "aA")
	})
}

// plainArrayRef reports a $name or ${name} reference to an array in word.
func (w *astWalker) plainArrayRef(word string) bool {
	for _, m := range varRefPattern.FindAllStringSubmatch(word, -1) {
		if w.state.arrays[m[1]] && !strings.HasSuffix(m[0], "]}") {
			return true
		}
	}

	return false
}

// forgetAssigned forgets a variable that ${NAME:=word} or ${NAME=word}
// assigns as a side effect of being expanded.
func (w *astWalker) forgetAssigned(exp *syntax.ParamExp) {
	if exp.Param == nil || exp.Exp == nil {
		return
	}

	if exp.Exp.Op == syntax.AssignUnset || exp.Exp.Op == syntax.AssignUnsetOrNull {
		w.forgetName(exp.Param.Value)
	}
}

// markSafeAssigns records the assignments stmt always makes in the current
// shell: a statement of plain assignments, export or declare without -n,
// and either of those leading an && chain.
func markSafeAssigns(stmt *syntax.Stmt, safe, chained map[*syntax.Assign]bool) {
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
			markSafeAssigns(c.X, safe, chained)
			markChainedAssigns(c.Y, chained)
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
	if assign.Name != nil && w.chainAssigns[assign] {
		w.chained = append(w.chained, assign.Name.Value)

		return
	}

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
		value, set, trusted := w.trustedValue(name)
		if !trusted {
			known = false

			return "", false
		}

		if set {
			w.noteExpanded(value)
		}

		return value, set
	})

	return expanded, known && !HasUnresolvedVars(expanded)
}

// trustedValue returns name's value from assignments made earlier on the
// line, then the environment, and whether it is set. It reports untrusted
// when the variable may hold something else by the time it is used.
func (w *astWalker) trustedValue(name string) (value string, set, trusted bool) {
	if w.inLoop || w.distrust || w.state.untrusted || w.state.dynamicVars[name] ||
		w.unknownVars[name] {
		return "", false, false
	}

	value, set = w.assignments[name]
	if !set {
		value, set = w.resolver.LookupEnv(name)
	}

	return value, set, true
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
		w.addOpacity(Opacity{
			Cause:     OpacityUnresolvedWord,
			Operation: cmd.Name,
			Detail:    DetailWordOutput,
			Tool:      w.state.evalSetups[cmd.Location.Seq],
		})

		return line, false
	}

	if !HasUnresolvedVars(line) {
		return line, true
	}

	expanded, ok := w.resolveWord(line)
	if !ok || allElements.MatchString(line) {
		w.opaque(OpacityUnresolvedWord, cmd.Name, DetailWordVariable)

		return line, false
	}

	return expanded, true
}

// launchedFrom returns what cmd runs. The commands it launches keep the
// stand-ins of followed, so env git $(...) is still seen; scripts and files
// are found as before. Eval runs its line with known variables substituted
// (a line it cannot know is already opaque and is not walked), and a
// container runner runs the program its exec names, its --entrypoint and
// the program after its image, and parallel runs its command lines.
func (w *astWalker) launchedFrom(cmd, followed Command, function bool) launch {
	launches := launched
	if function {
		launches = functionLaunch
	}

	l := launches(cmd)
	if shells[cmd.Name] {
		l = w.shellStreamLaunch(followed, l)
	}

	if l.stdinProgram {
		l = w.stdinProgramLaunch(followed, l)
	}

	if slices.ContainsFunc(followed.Args, marked) {
		l.commands = launches(followed).commands
	}

	if cmds, replace := w.containerExecCommands(followed); replace {
		l.commands, l.scripts = cmds, nil
	} else {
		l.commands = append(l.commands, cmds...)
		opaque := len(w.state.opacities)
		l.entrypoints = w.entrypointCommands(followed)

		// An --entrypoint already found opaque is not reported twice.
		if len(w.state.opacities) == opaque {
			l.commands = append(l.commands, w.containerRunCommands(followed)...)
		}
	}

	if launchers[cmd.Name].stdinArgs {
		l.commands = w.resolveXargsReplace(followed, l.commands)
	}

	if parallelPrograms[cmd.Name] {
		l.commands, l.scripts = nil, nil
		l.scripts, l.code = w.parallelScripts(followed)
	}

	if cmd.Name == sourceBuiltin || cmd.Name == dotBuiltin {
		l.files = w.sourceLaunch(followed)
	}

	if cmd.Name == evalBuiltin {
		l.scripts = nil

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
	w.distrustSplitting(name)

	for p := w; p != nil; p = p.parent {
		p.unknownVars[name] = true
		p.scope = nil
		delete(p.startupUnset, name)
	}
}

// varWriters are the builtins that set variables named in their arguments.
var varWriters = nameSet(
	strings.Join([]string{"read mapfile readarray unset", getoptsBuiltin, printfBuiltin}, " "),
)

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

// sameShellRunners run a command line in the current shell, which sees the
// variables as the parser tracked them. Any other runner starts a new
// process, which sees only exported variables and may first source a file
// named by BASH_ENV or ENV, so no variable resolves inside it.
var sameShellRunners = nameSet(strings.Join(
	[]string{evalBuiltin, sourceBuiltin, dotBuiltin, trapBuiltin}, " ",
))

// runsInShell reports whether the script parent runs shares its shell: eval,
// source, a trap, or a same-line alias or function (not a git or gh alias).
func runsInShell(parent Command, sw scriptWalk) bool {
	if sameShellRunners[parent.Name] {
		return true
	}

	return sw.name != "" && !strings.HasPrefix(sw.name, "git:") &&
		!strings.HasPrefix(sw.name, "gh:")
}

// forgetDeclared forgets every variable a declaration run as a command sets,
// and stops trusting any after an option that changes values or makes
// references, or an operand that is not literal.
func (w *astWalker) forgetDeclared(cmd Command) {
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+") {
			if cmd.Name == exportBuiltin && isNamerefOption(arg) {
				continue
			}

			if strings.ContainsAny(arg[1:], "lucn") {
				w.distrustOption(arg)
			}

			continue
		}

		name, _, _ := strings.Cut(arg, "=")
		if !variableName.MatchString(name) {
			w.distrustNames()

			continue
		}

		w.forgetName(name)
	}
}

func (w *astWalker) forgetRemovedNamerefs(cmd Command) bool {
	remove := false
	options := true

	var names []string

	for _, arg := range cmd.Args {
		switch {
		case options && arg == endOfOptions:
			options = false
		case options && strings.HasPrefix(arg, "-"):
			remove = remove || strings.Contains(arg[1:], "n")
		default:
			options = false

			names = append(names, arg)
		}
	}

	if !remove {
		return false
	}

	if !cmd.unconditional {
		w.distrustNames()

		return true
	}

	for _, name := range names {
		if !variableName.MatchString(name) {
			w.distrustNames()

			continue
		}

		delete(w.namerefs, name)
		w.forget(name)
	}

	return true
}

// defaultVars are the variables a writer sets when it names none.
var defaultVars = map[string]string{"read": "REPLY", "mapfile": "MAPFILE", "readarray": "MAPFILE"}

// forgetWritten forgets the variables cmd sets other than by assignment.
func (w *astWalker) forgetWritten(cmd Command) {
	if assignmentPattern.MatchString(cmd.Invoked) ||
		(mapfiles[cmd.Name] && slices.ContainsFunc(cmd.Args, callbackFlag)) {
		w.distrustNames()

		return
	}

	if HasUnresolvedVars(cmd.Invoked) || strings.Contains(cmd.Invoked, unresolvedProgram) ||
		cmd.Name == sourceBuiltin || cmd.Name == dotBuiltin {
		w.state.untrusted = true

		return
	}

	if cmd.Name == unsetBuiltin && w.forgetRemovedNamerefs(cmd) {
		return
	}

	if declWriters[cmd.Name] {
		w.forgetDeclared(cmd)
		w.caseChanged = false

		return
	}

	if !varWriters[cmd.Name] {
		return
	}

	names := writtenVars(cmd)
	if cmd.Name == getoptsBuiltin {
		names = append(names, "OPTARG", "OPTIND")
	}

	if len(names) == 0 && defaultVars[cmd.Name] != "" {
		names = []string{defaultVars[cmd.Name]}
	}

	wasUnknown := make(map[string]bool, len(startupVars))
	for name := range startupVars {
		wasUnknown[name] = w.startupUnknown(name)
	}

	for _, name := range names {
		w.forgetName(name)
	}

	w.noteUnset(cmd, wasUnknown)
}

// forgetName forgets a written variable. A target built from an expansion
// or naming an element ("$v", x[0]) could be any variable, so after it none
// is trusted. Other words (a prompt, a timeout) name no variable.
func (w *astWalker) forgetName(name string) {
	switch {
	case variableName.MatchString(name):
		if target, ok := w.namerefTarget(name); ok {
			w.forget(target)
		} else {
			w.forget(name)
		}
	case strings.ContainsAny(name, "$[") || marked(name):
		w.distrustNames()
	}
}

// printfConversionN matches a %n conversion, with any flags, width or
// precision, which makes printf assign to the variable its argument names.
var printfConversionN = regexp.MustCompile(`%[-+ #0-9.*]*n`)

// printfAssigns reports a printf format that may assign through %n: one
// that has it, or one holding an expansion klaudiush cannot read.
func printfAssigns(format string) bool {
	return printfConversionN.MatchString(format) || strings.Contains(format, "$") || marked(format)
}

// variableName matches a plain shell variable name.
var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// distrustDecl stops trusting any variable after a declare whose operand is
// not a literal assignment (declare "$v", export $v=x) or whose options
// change values or make references (-l, -u, -c, -n).
func (w *astWalker) distrustDecl(decl *syntax.DeclClause) {
	tracked := w.trackNamerefs(decl)

	for _, a := range decl.Args {
		if a.Name != nil || a.Value == nil {
			continue
		}

		option := wordToString(a.Value)
		if decl.Variant != nil && decl.Variant.Value == exportBuiltin && isNamerefOption(option) {
			continue
		}

		if isNamerefOption(option) && tracked {
			continue
		}

		w.distrustOption(option)
	}
}

// trackNamerefs records literal targets from a plain nameref declaration.
// Reads through a nameref stay unknown; writes can still be attributed.
func (w *astWalker) trackNamerefs(decl *syntax.DeclClause) bool {
	if !declHasNameref(decl) {
		return false
	}

	remove := slices.ContainsFunc(decl.Args, func(a *syntax.Assign) bool {
		if a.Name != nil || a.Value == nil {
			return false
		}

		option := wordToString(a.Value)

		return strings.HasPrefix(option, "+") && isNamerefOption(option)
	})

	tracked := false

	for _, assign := range decl.Args {
		if assign.Name == nil {
			continue
		}

		if !w.safeNamerefs[assign] {
			return false
		}

		if remove {
			delete(w.namerefs, assign.Name.Value)

			tracked = true

			continue
		}

		if assign.Append || assign.Value == nil || !isLiteralWord(assign.Value) {
			return false
		}

		target := wordToString(assign.Value)
		if !variableName.MatchString(target) {
			return false
		}

		w.namerefs[assign.Name.Value] = target
		tracked = true
	}

	return tracked
}

// namerefTarget resolves chained namerefs and rejects cycles.
func (w *astWalker) namerefTarget(name string) (string, bool) {
	seen := make(map[string]bool)

	for {
		target, ok := w.namerefs[name]
		if !ok {
			return name, len(seen) > 0
		}

		if seen[name] {
			w.distrustNames()

			return "", false
		}

		seen[name] = true
		name = target
	}
}

// forgetNamerefAssign attributes an assignment through a known nameref to
// its target. The value stays unknown because nameref reads are not modeled.
func (w *astWalker) forgetNamerefAssign(assign *syntax.Assign) bool {
	if assign.Name == nil {
		return false
	}

	target, ok := w.namerefTarget(assign.Name.Value)
	if !ok {
		return false
	}

	if target == pathVar {
		w.state.pathChanged = true
	}

	w.forget(target)

	return true
}

// writtenVars returns the words cmd may write to: every operand and flag
// value, since an empty value such as read -d ” leaves no word behind, and
// for printf the value of -v, given apart or attached, and the arguments a
// format with %n assigns. A value in
// quotes for an option of read or mapfile that takes one names nothing.
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
		case strings.HasPrefix(arg, "-") && quotedValue(cmd, i):
			i++
		case strings.HasPrefix(arg, "-"):
		case cmd.Name == printfBuiltin && printfAssigns(arg):
			return append(names, cmd.Args[i+1:]...)
		case cmd.Name == printfBuiltin:
			return names
		default:
			names = append(names, arg)
		}
	}

	return names
}

// ProgramWordOperation is the Opacity.Operation of a program word that comes
// from a variable, command output or a glob.
const ProgramWordOperation = "program"

// findPath is the found path find -exec and xargs -I{} substitute into the
// command they run, so a program word holding it names an unknown file.
const findPath = "{}"

// programWord resolves the word that names cmd's program. Its expansion
// splits into words, so x="git commit"; $x runs git, and an expansion to
// nothing leaves the next argument as the program, so x=; $x git push runs
// git. It also says why the word cannot be known, or returns "" when every
// expansion in it resolves to literal text.
func (w *astWalker) programWord(cmd Command, view string) (Command, string) {
	detail := ""

	for range len(cmd.Args) + 1 {
		raw := w.applyDefaults(strings.ReplaceAll(cmd.Name, unresolvedWord, unresolvedProgram))
		cmd.Invoked = w.expandName(raw)

		if detail == "" {
			detail = w.programWordDetail(raw, view)
		}

		view = ""

		if !HasUnresolvedVars(raw) {
			break
		}

		w.noteExpanded(cmd.Invoked)
		w.noteExpanded(commandName(cmd.Invoked))

		fields := strings.Fields(cmd.Invoked)
		if len(fields) == 0 && len(cmd.Args) > 0 {
			cmd.Name, cmd.Args = cmd.Args[0], cmd.Args[1:]

			continue
		}

		if len(fields) > 1 {
			cmd.Invoked, cmd.Args = fields[0], slices.Concat(fields[1:], cmd.Args)
		}

		break
	}

	cmd.Name = commandName(cmd.Invoked)

	return cmd, detail
}

// programWordDetail says why a program word cannot be known: a variable the
// rules of resolveWord do not trust, command output, or a glob. view is the
// word with quoted glob characters neutralized, when the word came from the
// command line; globs are looked for in it after expansion.
func (w *astWalker) programWordDetail(word, view string) string {
	expanded := word

	if HasUnresolvedVars(word) {
		resolved, ok := w.resolveWord(word)
		if !ok || w.plainArrayRef(word) {
			return w.variableDetail()
		}

		expanded = resolved
	}

	if strings.Contains(expanded, unresolvedProgram) || strings.Contains(expanded, findPath) {
		return DetailWordOutput
	}

	globbed := expanded
	if view != "" {
		globbed = w.expandName(w.applyDefaults(view))
	}

	if slices.ContainsFunc(strings.Fields(globbed), globWord) {
		return DetailWordOutput
	}

	return ""
}

// variableDetail says why a variable in a program word is not trusted, so
// the repair can match: inside a loop or a new shell none is.
func (w *astWalker) variableDetail() string {
	switch {
	case w.inLoop:
		return DetailWordLoop
	case w.distrust:
		return DetailWordNewShell
	case w.state.untrusted:
		return DetailWordUntrusted
	default:
		return DetailWordVariable
	}
}

// neutralGlob stands in for a quoted or escaped glob character, which
// matches nothing.
const neutralGlob = "\uE002"

// neutralize replaces the glob characters in quoted text.
func neutralize(text string) string {
	return strings.NewReplacer("*", neutralGlob, "?", neutralGlob, "[", neutralGlob).Replace(text)
}

// globView renders a program word for finding globs: quoted and escaped
// glob characters are neutralized, an unquoted plain variable stays a
// reference whose value may glob, and a quoted one cannot.
func globView(word *syntax.Word) string {
	return globParts(word.Parts, false)
}

func globParts(parts []syntax.WordPart, quoted bool) string {
	var b strings.Builder

	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if quoted {
				b.WriteString(neutralize(p.Value))
			} else {
				b.WriteString(unescapedGlobs(p.Value))
			}
		case *syntax.SglQuoted:
			b.WriteString(neutralize(p.Value))
		case *syntax.DblQuoted:
			b.WriteString(globParts(p.Parts, true))
		case *syntax.ParamExp:
			if quoted {
				b.WriteString(neutralGlob)
			} else {
				b.WriteString(neutralize(paramExpToString(p)))
			}
		default:
			b.WriteString(unresolvedProgram)
		}
	}

	return b.String()
}

// unescapedGlobs neutralizes the glob characters a backslash escapes in
// unquoted text, keeping the others.
func unescapedGlobs(text string) string {
	var b strings.Builder

	escaped := false

	for _, r := range text {
		switch {
		case escaped:
			escaped = false

			b.WriteString(neutralize(string(r)))
		case r == '\\':
			escaped = true
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}

// globWord reports a word the shell expands against file names.
func globWord(word string) bool {
	if strings.ContainsAny(word, "*?") {
		return true
	}

	_, after, found := strings.Cut(word, "[")

	return found && strings.Contains(after, "]")
}

// defaultValue matches ${NAME:-word} and ${NAME-word} with a plain literal
// word.
var defaultValue = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:?)-([A-Za-z0-9_./+@%,-]*)\}`)

// applyDefaults resolves the default-value forms in a program word, as in
// ${EDITOR:-vi}, to a plain reference when the variable is set (and, for :-,
// not empty) or to the default otherwise. A variable whose value is not
// trusted keeps its form and stays unresolved.
func (w *astWalker) applyDefaults(word string) string {
	if !strings.Contains(word, "-") {
		return word
	}

	return defaultValue.ReplaceAllStringFunc(word, func(ref string) string {
		m := defaultValue.FindStringSubmatch(ref)

		value, set, trusted := w.trustedValue(m[1])

		switch {
		case !trusted:
			return ref
		case set && (value != "" || m[2] == ""):
			return "${" + m[1] + "}"
		default:
			return m[3]
		}
	})
}

// distrustSplitting stops trusting any variable once IFS changes: the shell
// then splits expansions on other characters, so IFS=,; x=git,push; $x
// runs git.
func (w *astWalker) distrustSplitting(name string) {
	if name == "IFS" {
		w.state.untrusted = true
	}
}

// withoutProgramFile drops the script file a program given by an opaque path
// would run, which would otherwise be reported a second time.
func withoutProgramFile(files []scriptFile, invoked string) []scriptFile {
	return slices.DeleteFunc(files, func(f scriptFile) bool {
		return f.path == invoked
	})
}
