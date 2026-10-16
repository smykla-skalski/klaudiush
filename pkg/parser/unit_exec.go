package parser

import (
	"slices"
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
		if unit.unit == "" || w.unitRuns(unit.unit, skip) {
			return true
		}

		skip = unit.launchedAt
	}

	return false
}

// unitCall is a command of a shell text and whether it runs files.
type unitCall struct {
	at   Location
	runs bool
}

// unitRuns reports whether the shell text holds a command, other than the
// one at skip, that runs files or commands. The calls of each text are read
// once per parse.
func (w *astWalker) unitRuns(text string, skip Location) bool {
	calls, ok := w.state.unitCalls[text]
	if !ok {
		calls = readUnitCalls(text)

		if w.state.unitCalls == nil {
			w.state.unitCalls = make(map[string][]unitCall)
		}

		w.state.unitCalls[text] = calls
	}

	return slices.ContainsFunc(calls, func(call unitCall) bool {
		return call.runs && (call.at.Line != skip.Line || call.at.Column != skip.Column)
	})
}

// readUnitCalls lists the commands of a shell text. Text that does not parse
// is one command that runs files.
func readUnitCalls(text string) []unitCall {
	file, err := syntax.NewParser().Parse(strings.NewReader(text), "")
	if err != nil {
		return []unitCall{{runs: true}}
	}

	var calls []unitCall

	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		calls = append(calls, unitCall{
			at:   Location{Line: call.Pos().Line(), Column: call.Pos().Col()},
			runs: callRunsFiles(call),
		})

		return true
	})

	return calls
}

// callRunsFiles reports whether a call may run a file or a command.
func callRunsFiles(call *syntax.CallExpr) bool {
	name, literal := literalWord(call.Args[0])
	if !literal {
		return true
	}

	base := commandName(name)
	if strings.Contains(name, "/") && dataCommands[base] {
		return false
	}

	if strings.Contains(name, "/") {
		return true
	}

	if skipped := dataSubcommands[base]; skipped != nil && len(call.Args) > 1 {
		if first, ok := literalWord(call.Args[1]); ok && skipped[first] {
			return false
		}
	}

	return runsFiles(base)
}

// runsFiles reports whether a program runs a file or a command it is handed:
// a shell, an interpreter, a launcher, a runner, a build tool, a hook runner
// or a builtin that reads code.
func runsFiles(name string) bool {
	_, isInterpreter := interpreters[name]
	_, isLauncher := launchers[name]

	return shells[name] || isInterpreter || isLauncher || makers[name] || argRunners[name] ||
		packageRunners[name] || taskRunners[name] || isContainerRunner(name) ||
		codeBuiltins[name]
}

// taskRunners run recipes, rules and hooks from files in the project, which
// a script may have written.
var taskRunners = nameSet(`just task rake ninja lefthook pre-commit husky direnv gradle
	gradlew mvn mvnw`)

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
