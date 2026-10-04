package parser

import (
	"regexp"
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
	case *syntax.Word:
		if isLiteralWord(n) {
			text = argWord(n)
		}
	case *syntax.CallExpr:
		text = w.loopWrites(n, seen)
	case *syntax.DeclClause:
		if slices.ContainsFunc(n.Args, computedOperand) ||
			slices.ContainsFunc(n.Args, namerefOption) {
			text = anyStartupVar
		}
	case *syntax.ParamExp:
		if (n.Excl && assignsDefault(n)) || promptExpansion(n) {
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

// promptExpansion reports ${x@P}, which expands the value of x again as a
// prompt string.
func promptExpansion(pe *syntax.ParamExp) bool {
	return pe.Exp != nil && pe.Exp.Op == syntax.OtherParamOps &&
		pe.Exp.Word != nil && pe.Exp.Word.Lit() == "P"
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
// a loop may set on a later pass in text the loop does not show: what a
// same-line function or alias named by its first word may set, and what
// the builtin it runs, past builtin and command, may set.
func (w *astWalker) loopWrites(call *syntax.CallExpr, seen map[string]bool) string {
	if len(call.Args) == 0 {
		return ""
	}

	text := w.definitionWrites(call.Args[0], call.Args[1:], seen)

	if args := runWrapped(call.Args); len(args) > 0 {
		text += " " + w.commandWrites(commandName(wordToString(args[0])), args[1:], seen)
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
// the front of a call, leaving the command they run. command -v and -V only
// look a name up, so they run nothing.
func runWrapped(args []*syntax.Word) []*syntax.Word {
	for len(args) > 0 && isLiteralWord(args[0]) {
		switch commandName(argWord(args[0])) {
		case builtinCommand:
			args = args[1:]
		case commandBuiltin:
			var runs bool

			if args, runs = commandOptions(args[1:]); !runs {
				return nil
			}
		default:
			return args
		}
	}

	return args
}

// commandOptions drops the options of command, reporting false when one of
// them makes it look the name up instead of running it.
func commandOptions(args []*syntax.Word) ([]*syntax.Word, bool) {
	for len(args) > 0 && isLiteralWord(args[0]) {
		option := argWord(args[0])

		switch {
		case option == endOfOptions:
			return args[1:], true
		case !strings.HasPrefix(option, "-") || option == "-":
			return args, true
		case strings.ContainsAny(option[1:], "vV"):
			return nil, false
		}

		args = args[1:]
	}

	return args, true
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

// literalElement matches an element with a literal numeric index. Any
// other subscript may be arithmetic that assigns a variable it computes.
var literalElement = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\[[0-9]+\]$`)

// joinTargets returns the targets as text, or every startup variable when
// one is an element whose subscript is not a literal number.
func joinTargets(names []string) string {
	for _, name := range names {
		if strings.Contains(name, "[") && !literalElement.MatchString(name) {
			return anyStartupVar
		}
	}

	return strings.Join(names, " ")
}

// writerSpec describes the options of a variable writer: the letters taking
// a value, the letters taking a target name, and how many operands are
// targets (all when negative).
type writerSpec struct {
	values   string
	targets  string
	operands int
}

// writerSpecs are the writers whose option values the loop scan tells apart
// from their targets.
var writerSpecs = map[string]writerSpec{
	"read":        {values: "dinNptu", targets: "a", operands: -1},
	"mapfile":     {values: "dnOsuCc", operands: 1},
	"readarray":   {values: "dnOsuCc", operands: 1},
	printfBuiltin: {targets: "v", operands: 1},
}

// writerTargets returns, as text, the variables a variable writer sets, or
// every startup variable when a target or an option word is computed or
// expands. A computed option value that stays one word sets nothing.
func writerTargets(name string, args []*syntax.Word) string {
	spec, ok := writerSpecs[name]
	if !ok {
		if slices.ContainsFunc(args, unsureWord) {
			return anyStartupVar
		}

		words := make([]string, 0, len(args))
		for _, arg := range args {
			words = append(words, argWord(arg))
		}

		return joinTargets(writtenVars(Command{Name: name, Args: words}))
	}

	names, rest, ok := spec.optionTargets(args)
	if !ok {
		return anyStartupVar
	}

	if name == printfBuiltin {
		return printfTargets(names, rest)
	}

	for i, arg := range rest {
		if spec.operands >= 0 && i >= spec.operands {
			break
		}

		if unsureWord(arg) {
			return anyStartupVar
		}

		names = append(names, argWord(arg))
	}

	return joinTargets(names)
}

// printfTargets adds the arguments a printf format assigns through %n to
// the -v target in names. A format the scan cannot read may hold %n, which
// assigns nothing only when it stays one word with no arguments after it.
func printfTargets(names []string, rest []*syntax.Word) string {
	if len(rest) == 0 {
		return joinTargets(names)
	}

	if unsureWord(rest[0]) && (len(rest) > 1 || !singleWord(rest[0])) {
		return anyStartupVar
	}

	if !printfAssigns(argWord(rest[0])) {
		return joinTargets(names)
	}

	for _, arg := range rest[1:] {
		if unsureWord(arg) {
			return anyStartupVar
		}

		names = append(names, argWord(arg))
	}

	return joinTargets(names)
}

// optionTargets returns the targets the options of a writer name and the
// operands after them. It fails when an option word, or a target, is
// computed or expands, or a value may not stay one word.
func (spec writerSpec) optionTargets(args []*syntax.Word) ([]string, []*syntax.Word, bool) {
	var names []string

	for i := 0; i < len(args); i++ {
		// A leading ~ is left to the operands: alone it never becomes an
		// option the writer acts on.
		if computedWord(args[i]) || expandingWord(args[i]) {
			return nil, nil, false
		}

		option := argWord(args[i])

		switch {
		case option == endOfOptions:
			return names, args[i+1:], true
		case !strings.HasPrefix(option, "-") || option == "-":
			return names, args[i:], true
		}

		for j := 1; j < len(option); j++ {
			letter := option[j : j+1]
			takesTarget := strings.Contains(spec.targets, letter)

			if !takesTarget && !strings.Contains(spec.values, letter) {
				continue
			}

			switch {
			case j+1 < len(option) && takesTarget:
				names = append(names, option[j+1:])
			case j+1 < len(option):
			case i+1 >= len(args):
			case takesTarget && unsureWord(args[i+1]):
				return nil, nil, false
			case takesTarget:
				i++
				names = append(names, argWord(args[i]))
			case !singleWord(args[i+1]):
				return nil, nil, false
			default:
				i++
			}

			break
		}
	}

	return names, nil, true
}

// quotedValue reports whether the argument after the option at i is the
// value that option of read or mapfile takes, written in quotes so it stays
// one word whatever it expands to.
func quotedValue(cmd Command, i int) bool {
	spec, ok := writerSpecs[cmd.Name]
	if !ok || spec.values == "" || i+1 >= len(cmd.Args) {
		return false
	}

	option := cmd.Args[i]
	if len(option) < 2 || strings.ContainsAny(option[1:len(option)-1], spec.values+spec.targets) ||
		!strings.Contains(spec.values, option[len(option)-1:]) {
		return false
	}

	return cmd.quoting[cmd.Args[i+1]] == quotedWord
}

// unsureWord reports a word whose value or number of words the scan cannot
// know: computed, holding a brace or glob character, or starting with an
// unquoted ~, which expands to a directory that may read as an option.
func unsureWord(word *syntax.Word) bool {
	return computedWord(word) || expandingWord(word) || tildeWord(word)
}

// tildeWord reports a word the shell tilde-expands: one whose tilde prefix,
// the text from a leading ~ up to the first unquoted slash, has no quoted
// character.
func tildeWord(word *syntax.Word) bool {
	if len(word.Parts) == 0 {
		return false
	}

	lit, ok := word.Parts[0].(*syntax.Lit)
	if !ok || !strings.HasPrefix(lit.Value, "~") {
		return false
	}

	prefix, _, slash := strings.Cut(lit.Value, "/")

	return !strings.Contains(prefix, `\`) && (slash || len(word.Parts) == 1)
}

// singleWord reports a word that stays exactly one word: literal, or with
// its expansions inside double quotes, apart from "$@" and array elements,
// which may split.
func singleWord(word *syntax.Word) bool {
	if expandingWord(word) {
		return false
	}

	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit, *syntax.SglQuoted:
		case *syntax.DblQuoted:
			if slices.ContainsFunc(p.Parts, splitsInQuotes) {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// splitsInQuotes reports a part of a double-quoted word that may still
// expand to several words: "$@", an array element list or a name list.
func splitsInQuotes(part syntax.WordPart) bool {
	pe, ok := part.(*syntax.ParamExp)

	return ok && ((pe.Param != nil && pe.Param.Value == "@") || pe.Index != nil || pe.Names != 0)
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
