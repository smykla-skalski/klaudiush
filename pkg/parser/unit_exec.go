package parser

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// othersRun reports whether a command other than carrier may run a file the
// carrier's code writes: one of the line, or of the scripts that launched it,
// that runs files or commands (see runsFiles). Text it cannot read counts as
// holding one. A program that is none of those is trusted not to execute the
// file, the same way the arguments of such a program are not read as a
// command line.
func (w *astWalker) othersRun(carrier Command) bool {
	skip := carrier.Location

	for unit := w; unit != nil; unit = unit.parent {
		if unit.unit == "" || unitRuns(unit.unit, skip) {
			return true
		}

		skip = unit.launchedAt
	}

	return false
}

// unitRuns reports whether the shell text holds a command, other than the
// one at skip, that runs files or commands.
func unitRuns(text string, skip Location) bool {
	file, err := syntax.NewParser().Parse(strings.NewReader(text), "")
	if err != nil {
		return true
	}

	runs := false

	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || runs || len(call.Args) == 0 {
			return !runs
		}

		at := Location{Line: call.Pos().Line(), Column: call.Pos().Col()}
		if at.Line == skip.Line && at.Column == skip.Column {
			return true
		}

		name, literal := literalWord(call.Args[0])
		runs = !literal || strings.Contains(name, "/") || runsFiles(commandName(name))

		return !runs
	})

	return runs
}

// runsFiles reports whether a program runs a file or a command it is handed:
// a shell, an interpreter, a launcher, a runner, make or a builtin that
// reads code.
func runsFiles(name string) bool {
	_, isInterpreter := interpreters[name]
	_, isLauncher := launchers[name]

	return shells[name] || isInterpreter || isLauncher || makers[name] || argRunners[name] ||
		isContainerRunner(name) || codeBuiltins[name]
}

// codeBuiltins are the shell builtins that run code from their operands or
// from a file.
var codeBuiltins = nameSet("source . eval exec trap")

// literalWord returns the text of a word made only of literal parts.
func literalWord(word *syntax.Word) (string, bool) {
	var text strings.Builder

	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			text.WriteString(p.Value)
		case *syntax.SglQuoted:
			text.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}

				text.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}

	return text.String(), true
}
