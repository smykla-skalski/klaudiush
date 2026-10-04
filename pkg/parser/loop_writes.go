package parser

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

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
// cannot see. Reading $HOME or $ZDOTDIR (param) sets neither; a loop reads
// HOME often. seen holds the same-line definitions already scanned.
func (w *astWalker) loopStartupNames(node syntax.Node, param bool, seen map[string]bool) []string {
	var text string

	switch n := node.(type) {
	case *syntax.Lit:
		text = n.Value
	case *syntax.SglQuoted:
		text = n.Value
	case *syntax.CallExpr:
		text = w.loopWrites(n, seen)
	case *syntax.DeclClause:
		if slices.ContainsFunc(n.Args, computedOperand) {
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

// loopWrites returns, as text naming them, the startup variables a call in
// a loop may set on a later pass in text the loop does not show: every one
// for source, eval or a computed target, the targets a variable writer
// names, and what a same-line function or alias it calls may set. builtin
// and command run the command they name, skipping definitions.
func (w *astWalker) loopWrites(call *syntax.CallExpr, seen map[string]bool) string {
	args := runWrapped(call.Args)
	if len(args) == 0 {
		return ""
	}

	word := wordToString(args[0])
	name := commandName(word)

	switch {
	case name == sourceBuiltin || name == dotBuiltin || name == evalBuiltin:
		return anyStartupVar
	case len(args) == len(call.Args) && w.defined(word):
		return w.definitionWrites(word, args[1:], seen)
	case varWriters[name]:
		return writerTargets(name, args[1:])
	case declWriters[name] && slices.ContainsFunc(args[1:], computedWord):
		return anyStartupVar
	default:
		return ""
	}
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
			for len(args) > 0 && isLiteralWord(args[0]) && strings.HasPrefix(argWord(args[0]), "-") {
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

// writerTargets returns, as text, the variables a variable writer sets, or
// every startup variable when one is computed. printf sets one only with -v
// before its format, so a computed word after the format sets nothing.
func writerTargets(name string, args []*syntax.Word) string {
	words := make([]string, 0, len(args))

	for i, arg := range args {
		if computedWord(arg) {
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
// function or alias may set when a loop calls it with args. Its text is
// scanned as the loop is. Text that does not parse, runs a command whose
// name it computes (such as "$@"), or is an alias ending in a blank (which
// makes the next word an alias too) may set any. Each definition is scanned
// once, so a recursive one ends.
func (w *astWalker) definitionWrites(
	word string,
	args []*syntax.Word,
	seen map[string]bool,
) string {
	if seen[word] {
		return ""
	}

	seen[word] = true

	var texts []string

	if value, ok := w.aliases[word]; ok {
		text, ok := aliasLine(value, args)
		if !ok {
			return anyStartupVar
		}

		texts = append(texts, text)
	}

	if body, ok := w.funcs[word]; ok {
		texts = append(texts, body)
	}

	var names []string

	for _, text := range texts {
		file, err := syntax.NewParser().Parse(strings.NewReader(text), "")
		if err != nil {
			return anyStartupVar
		}

		computed := false

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
