package parser

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// trapBuiltin runs its action as shell code when a signal arrives.
const trapBuiltin = "trap"

// maxDefinitionScans bounds the same-line definitions and trap actions one
// loop call may lead the scan through; past it the call may set any.
const maxDefinitionScans = 64

// walkLoop visits every node under root, telling visit whether a literal is
// the name in a parameter expansion, which reads a variable rather than
// setting it.
func walkLoop(root syntax.Node, visit func(node syntax.Node, param bool)) {
	params := make(map[*syntax.Lit]bool)

	syntax.Walk(root, func(node syntax.Node) bool {
		if pe, ok := node.(*syntax.ParamExp); ok && pe.Param != nil && !assignsDefault(pe) {
			params[pe.Param] = true
		}

		lit, isLit := node.(*syntax.Lit)
		visit(node, isLit && params[lit])

		return true
	})
}

// loopStartupNames returns the startup variables node may set on a later
// pass of a loop: one it names, or every one when it runs code the loop
// cannot see or writes through a name it computes (a nameref, ${!v:=x}).
// Reading $HOME or $ZDOTDIR (param) sets neither; a loop reads HOME often.
// seen holds the same-line definitions already scanned.
func (w *astWalker) loopStartupNames(
	node syntax.Node,
	param bool,
	seen map[string]bool,
) []string {
	var text string

	switch n := node.(type) {
	case *syntax.Lit:
		text = n.Value
	case *syntax.SglQuoted:
		text = n.Value
	case *syntax.CallExpr:
		text = w.loopWrites(n, seen)
	case *syntax.DeclClause:
		if slices.ContainsFunc(n.Args, computedOperand) ||
			slices.ContainsFunc(n.Args, namerefOption) {
			text = anyStartupVar
		}
	case *syntax.ParamExp:
		if n.Excl && assignsDefault(n) {
			text = anyStartupVar
		}
	}

	var names []string

	for _, m := range startupMention.FindAllStringSubmatch(text, -1) {
		if param && (m[2] == homeVar || m[2] == zdotdirVar) {
			continue
		}

		names = append(names, m[2])
	}

	return names
}

// namerefOption reports a declaration option making a nameref, through
// which a later plain assignment writes the variable it names.
func namerefOption(assign *syntax.Assign) bool {
	if assign.Name != nil || assign.Value == nil {
		return false
	}

	option := wordToString(assign.Value)

	return isNamerefOption(option)
}

// isNamerefOption reports an option cluster with n in it, such as -n or -gn.
func isNamerefOption(option string) bool {
	return (strings.HasPrefix(option, "-") || strings.HasPrefix(option, "+")) &&
		strings.Contains(option[1:], "n")
}

// loopWrites returns, as text naming them, the startup variables a call in
// a loop may set on a later pass in text the loop does not show: what the
// builtin it names may set, and what a same-line function or alias of that
// name may set. builtin and command skip the definitions.
func (w *astWalker) loopWrites(call *syntax.CallExpr, seen map[string]bool) string {
	args := runWrapped(call.Args)
	if len(args) == 0 {
		return ""
	}

	text := w.commandWrites(commandName(wordToString(args[0])), args[1:], seen)

	if len(args) == len(call.Args) {
		text += " " + w.definitionWrites(args[0], args[1:], seen)
	}

	return text
}

// commandWrites returns, as text, the startup variables a builtin may set:
// every one for source, eval, a mapfile callback or a computed target, what
// a trap action may set, else the targets a variable writer names.
func (w *astWalker) commandWrites(name string, args []*syntax.Word, seen map[string]bool) string {
	switch {
	case name == sourceBuiltin || name == dotBuiltin || name == evalBuiltin:
		return anyStartupVar
	case name == trapBuiltin:
		return w.trapWrites(args, seen)
	case mapfiles[name] && slices.ContainsFunc(args, func(arg *syntax.Word) bool {
		return callbackFlag(argWord(arg))
	}):
		return anyStartupVar
	case varWriters[name]:
		return writerTargets(name, args)
	case declWriters[name] && slices.ContainsFunc(args, func(arg *syntax.Word) bool {
		return computedWord(arg) || isNamerefOption(argWord(arg))
	}):
		return anyStartupVar
	default:
		return ""
	}
}

// trapWrites returns, as text, the startup variables a trap action may set
// when it runs, scanned as a definition is.
func (w *astWalker) trapWrites(args []*syntax.Word, seen map[string]bool) string {
	for i, arg := range args {
		if computedWord(arg) {
			return anyStartupVar
		}

		value := argWord(arg)

		switch {
		case value == endOfOptions:
			if i+1 < len(args) {
				return w.trapWrites(args[i+1:i+2], seen)
			}

			return ""
		case strings.HasPrefix(value, "-") && value != "-":
			continue
		default:
			return w.textWrites(value, seen)
		}
	}

	return ""
}

// funcBodies returns the bodies of the functions a statement defines, which
// a loop in it may call before the walker records them.
func funcBodies(stmt *syntax.Stmt) map[string][]string {
	var bodies map[string][]string

	syntax.Walk(stmt, func(node syntax.Node) bool {
		fn, ok := node.(*syntax.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			return true
		}

		var body strings.Builder
		if err := syntax.NewPrinter().Print(&body, fn.Body); err != nil {
			body.Reset()
			body.WriteString(evalBuiltin)
		}

		if bodies == nil {
			bodies = make(map[string][]string)
		}

		bodies[fn.Name.Value] = append(bodies[fn.Name.Value], body.String())

		return true
	})

	return bodies
}

// runWrapped drops builtin and command, with the options of command, from
// the front of a call, leaving the command they run.
func runWrapped(args []*syntax.Word) []*syntax.Word {
	for len(args) > 0 && isLiteralWord(args[0]) {
		switch commandName(argWord(args[0])) {
		case builtinCommand:
			args = args[1:]
		case commandBuiltin:
			args = args[1:]
			for len(args) > 0 && isLiteralWord(args[0]) &&
				strings.HasPrefix(argWord(args[0]), "-") {
				args = args[1:]
			}
		default:
			return args
		}
	}

	return args
}

// computedWord reports a word that is not literal text.
func computedWord(word *syntax.Word) bool {
	return !isLiteralWord(word)
}

// expandingWord reports a literal word with an unquoted brace or glob
// character, which the shell may turn into other words.
func expandingWord(word *syntax.Word) bool {
	return slices.ContainsFunc(word.Parts, func(part syntax.WordPart) bool {
		lit, ok := part.(*syntax.Lit)

		return ok && strings.ContainsAny(lit.Value, "{[*?")
	})
}

// writerTargets returns, as text, the variables a variable writer sets, or
// every startup variable when one is computed or expands. printf sets one
// only with -v before its format, so such a word after the format sets
// nothing.
func writerTargets(name string, args []*syntax.Word) string {
	words := make([]string, 0, len(args))

	for i, arg := range args {
		if computedWord(arg) || expandingWord(arg) {
			if name != printfBuiltin || printfOptionAt(args[:i]) {
				return anyStartupVar
			}

			break
		}

		words = append(words, argWord(arg))
	}

	return strings.Join(writtenVars(Command{Name: name, Args: words}), " ")
}

// printfOptionAt reports whether the word after the literal words before is
// still an option of printf or the variable -v names, not the format or an
// argument for it.
func printfOptionAt(before []*syntax.Word) bool {
	for i := 0; i < len(before); i++ {
		arg := argWord(before[i])

		switch {
		case arg == endOfOptions:
			return false
		case arg == "-v":
			i++
		case strings.HasPrefix(arg, "-"):
		default:
			return false
		}
	}

	return true
}

// definitionWrites returns, as text, the startup variables a same-line
// function or alias named by word may set when a loop calls it with args.
// A function is found by the word with quotes and backslashes removed, an
// alias by the word as written. Each function is scanned once and each
// alias once per line it expands to, so recursion ends.
func (w *astWalker) definitionWrites(
	word *syntax.Word,
	args []*syntax.Word,
	seen map[string]bool,
) string {
	var texts []string

	if value, ok := w.aliases[wordToString(word)]; ok {
		text, ok := aliasLine(value, args)
		if !ok {
			return anyStartupVar
		}

		if key := "alias\x00" + text; !seen[key] {
			seen[key] = true

			texts = append(texts, text)
		}
	}

	name := argWord(word)
	if key := "func\x00" + name; !seen[key] {
		seen[key] = true

		if body, ok := w.funcs[name]; ok {
			texts = append(texts, body)
		}

		texts = append(texts, w.stmtFuncs[name]...)
	}

	var names []string

	for _, text := range texts {
		names = append(names, w.textWrites(text, seen))
	}

	return strings.Join(names, " ")
}

// textWrites returns, as text, the startup variables shell code may set,
// scanned as a loop body is. Code that does not parse, runs a command whose
// name it computes (such as "$@"), or leads the scan through too many
// definitions may set any.
func (w *astWalker) textWrites(text string, seen map[string]bool) string {
	if len(seen) > maxDefinitionScans {
		return anyStartupVar
	}

	file, err := syntax.NewParser().Parse(strings.NewReader(text), "")
	if err != nil {
		return anyStartupVar
	}

	computed := false

	var names []string

	walkLoop(file, func(node syntax.Node, param bool) {
		if call, ok := node.(*syntax.CallExpr); ok {
			inner := runWrapped(call.Args)
			computed = computed || (len(inner) > 0 && computedWord(inner[0]))
		}

		names = append(names, w.loopStartupNames(node, param, seen)...)
	})

	if computed {
		return anyStartupVar
	}

	return strings.Join(names, " ")
}

// aliasLine returns the line a call to an alias with args runs. It fails
// for a value ending in a blank, after which the shell expands the next
// word as an alias too.
func aliasLine(value string, args []*syntax.Word) (string, bool) {
	if strings.TrimRight(value, " \t") != value {
		return "", false
	}

	var line strings.Builder

	line.WriteString(value)

	for _, arg := range args {
		line.WriteString(" ")

		if err := syntax.NewPrinter().Print(&line, arg); err != nil {
			return "", false
		}
	}

	return line.String(), true
}
