package parser

import (
	"path/filepath"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// movedDir are the variables a cd on the line changes, which klaudiush
// would otherwise read from its own process.
var movedDir = nameSet("PWD OLDPWD")

// operandsEnd skips the operands still due after a launcher's "--": its
// fixed operands and, for env, NAME=value assignments, which env reads
// after "--" too.
func operandsEnd(spec launcher, args []string, i, operands int) (int, bool) {
	for ; i < len(args); i++ {
		switch {
		case spec.assignments && (assignmentPattern.MatchString(args[i]) ||
			args[i] == unresolvedWord):
		case operands > 0:
			operands--
		default:
			return i, true
		}
	}

	return 0, false
}

// redirectedStdin checks the file redirected to stdin that a /dev/stdin
// startup file reads: /dev/null gives nothing, and another device or
// descriptor (< /dev/fd/3) is a stream klaudiush cannot read.
func redirectedStdin(cmd Command, path string) (string, string) {
	clean := resolvePath(cmd.WorkingDirectory, path)

	switch {
	case clean == devNull:
		return "", ""
	case specialPath(clean) || (!filepath.IsAbs(clean) && specialPath(filepath.Join("/", clean))):
		return "", DetailScriptRead
	default:
		return path, ""
	}
}

// literalRCFile reports an --rcfile path bash takes as written: the shell
// already expanded the argument, so a $ or backquote left in it is either
// part of the name or a variable klaudiush would resolve differently.
func literalRCFile(path string) startupValue {
	return startupValue{value: path, dynamic: marked(path), literal: true}
}

// RCFileOption is the Opacity.Operation of a startup file named by bash
// --rcfile or --init-file, which takes a path rather than a variable.
const RCFileOption = rcfileLabel

// dynamicArgs returns the rendered arguments whose words take part of their
// value from command output, a process substitution or arithmetic, so an
// env operand is judged by its own word rather than the whole command.
func dynamicArgs(words []*syntax.Word) map[string]bool {
	var args map[string]bool

	for _, word := range words {
		if !wordDynamic(word) {
			continue
		}

		if args == nil {
			args = make(map[string]bool)
		}

		args[argWord(word)] = true
	}

	return args
}

// setBuiltin turns on keyword mode with -k or with setOption keyword.
const setBuiltin = "set"

// setOption names a set option by its long name.
const setOption = "-o"

// loopMayWriteAny reports a call in a loop that may set any variable on a
// later pass in text the loop does not show: source, eval, a same-line
// function or alias, or a variable writer with a computed operand.
func (w *astWalker) loopMayWriteAny(call *syntax.CallExpr) bool {
	word := wordToString(call.Args[0])
	name := commandName(word)

	switch {
	case name == sourceBuiltin || name == dotBuiltin || name == evalBuiltin:
		return true
	case w.defined(word):
		return true
	case name == printfBuiltin:
		return printfMayWriteAny(call.Args[1:])
	case varWriters[name] || declWriters[name]:
		return slices.ContainsFunc(call.Args[1:], func(arg *syntax.Word) bool {
			return !isLiteralWord(arg)
		})
	default:
		return false
	}
}

// printfMayWriteAny reports a printf that may write a computed name: one
// whose -v target or options are not literal. Only -v names a variable, so
// the format and the values after it never do.
func printfMayWriteAny(args []*syntax.Word) bool {
	for i := 0; i < len(args); i++ {
		if !isLiteralWord(args[i]) {
			return true
		}

		arg := argWord(args[i])

		switch {
		case arg == "-v":
			i++

			if i < len(args) && !isLiteralWord(args[i]) {
				return true
			}
		case arg == endOfOptions || !strings.HasPrefix(arg, "-"):
			return false
		}
	}

	return false
}

// computedOperand reports a declaration operand that is not a literal
// assignment, which may write any name.
func computedOperand(assign *syntax.Assign) bool {
	return assign.Name == nil && assign.Value != nil && !isLiteralWord(assign.Value)
}

// homeChanged reports whether the line may have changed HOME, which a
// leading ~ in a startup path expands to.
func (w *astWalker) homeChanged() bool {
	_, assigned := w.assignments["HOME"]

	return assigned || w.unknownVars["HOME"] || w.state.dynamicVars["HOME"] ||
		w.state.namesUnknown
}

// startupStdin returns what a startup file of /dev/stdin reads: the
// command's literal stdin, or the file redirected to it. Stdin klaudiush
// cannot see (a pipe from a program, the hook's own stdin) is opaque.
func startupStdin(cmd Command) (path, detail string) {
	switch {
	case cmd.Stdin != "":
		return devStdin, ""
	case cmd.StdinFile != "":
		return cmd.StdinFile, ""
	default:
		return "", DetailScriptRead
	}
}

// specialPath reports a device or process file, which is no regular
// script: a descriptor, a pipe or a terminal.
func specialPath(path string) bool {
	return strings.HasPrefix(path, "/dev/") || strings.HasPrefix(path, "/proc/")
}

// deferredWord reports a value that keeps a $ or backquote as text, from
// quotes or escapes, which the shell expands when it reads the variable at
// startup rather than when it is assigned.
func deferredWord(word *syntax.Word) bool {
	if word == nil {
		return false
	}

	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.SglQuoted:
			if strings.ContainsAny(p.Value, "$`") {
				return true
			}
		case *syntax.Lit:
			if strings.Contains(p.Value, `\$`) || strings.Contains(p.Value, "\\`") {
				return true
			}
		case *syntax.DblQuoted:
			if deferredWord(&syntax.Word{Parts: p.Parts}) {
				return true
			}
		}
	}

	return false
}

// noteStartupDeferred records whether a startup variable's new value holds
// an expansion the shell runs at startup.
func (w *astWalker) noteStartupDeferred(assign *syntax.Assign) {
	if !startupVars[assign.Name.Value] {
		return
	}

	if w.startupDeferred == nil {
		w.startupDeferred = make(map[string]bool)
	}

	w.startupDeferred[assign.Name.Value] = deferredWord(assign.Value)
}

// distrustOption handles one declare operand that is not an assignment. A
// non-literal operand or -n may write any name; -l, -u and -c only change
// the case of the values of the names given, so those stay known.
func (w *astWalker) distrustOption(option string) {
	switch {
	case !strings.HasPrefix(option, "-") && !strings.HasPrefix(option, "+"),
		strings.Contains(option[1:], "n"):
		w.distrustNames()
	case strings.ContainsAny(option[1:], "luc"):
		w.state.untrusted = true
		w.caseChanged = true
	}
}

// forgetCaseChanged forgets the startup variables a declare changing the
// case of its values assigns, since the file name changes with it.
func (w *astWalker) forgetCaseChanged(decl *syntax.DeclClause) {
	if !w.caseChanged {
		return
	}

	w.caseChanged = false

	for _, assign := range decl.Args {
		if assign.Name != nil && startupVars[assign.Name.Value] {
			w.forget(assign.Name.Value)
		}
	}
}
