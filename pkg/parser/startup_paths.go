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

// printfMayWriteAny reports a printf that may write a computed name. Only
// -v names a variable, so a printf without it writes nothing but what its
// values assign. With -v, a target that is not a plain name, a startup
// variable, or any computed word may write one. A word before the format
// that expands or globs may itself become -v.
func printfMayWriteAny(args []*syntax.Word) bool {
	for i, word := range args {
		if computedPrintfWord(word) {
			return true
		}

		arg := argWord(word)

		switch {
		case strings.HasPrefix(arg, "-v"):
			target := strings.TrimPrefix(arg, "-v")
			if target == "" && i+1 < len(args) {
				target = argWord(args[i+1])
			}

			return !variableName.MatchString(target) || startupVars[target] ||
				slices.ContainsFunc(args, computedPrintfWord)
		case arg == endOfOptions || !strings.HasPrefix(arg, "-"):
			return slices.ContainsFunc(args[i:], assigningWord)
		}
	}

	return false
}

// assigningWord reports a word whose expansion may assign a variable it
// names only at run time: arithmetic, a computed index or slice, an
// indirect ${!n}, whose target may be an element with a computed index, a
// default assignment ${x:=v}, whose x may be a nameref, or ${x@P}, which
// expands the value again. A command substitution runs in a
// subshell, so what it assigns does not reach the loop; ${ cmd;} and
// ${|cmd;} run in the current shell and count as assigning.
func assigningWord(word *syntax.Word) bool {
	found := false

	syntax.Walk(word, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CmdSubst:
			found = found || n.TempFile || n.ReplyVar

			return false
		case *syntax.ProcSubst:
			return false
		case *syntax.ArithmExp:
			found = true
		case *syntax.ParamExp:
			found = found || !literalIndex(n.Index) || n.Slice != nil ||
				indirect(n) || assignsDefault(n) || promptExpansion(n)
		}

		return !found
	})

	return found
}

// indirect reports ${!n}, which expands the variable n names, as opposed to
// listing names with ${!pre@} or keys with ${!a[@]}.
func indirect(pe *syntax.ParamExp) bool {
	return pe.Excl && pe.Names == 0 && !allIndex(pe.Index)
}

// allIndex reports the [@] or [*] index of ${!a[@]}, which lists keys.
func allIndex(index syntax.ArithmExpr) bool {
	word, ok := index.(*syntax.Word)
	if !ok {
		return false
	}

	lit := word.Lit()

	return lit == "@" || lit == "*"
}

// tildeWord reports a word with a leading unquoted ~, which expands to a
// directory from HOME, PWD, OLDPWD or the user database.
func tildeWord(word *syntax.Word) bool {
	if len(word.Parts) == 0 {
		return false
	}

	lit, ok := word.Parts[0].(*syntax.Lit)

	return ok && strings.HasPrefix(lit.Value, "~")
}

// promptExpansion reports ${x@P}, which expands x as a prompt string.
func promptExpansion(pe *syntax.ParamExp) bool {
	return pe.Exp != nil && pe.Exp.Op == syntax.OtherParamOps &&
		pe.Exp.Word != nil && pe.Exp.Word.Lit() == "P"
}

// computedPrintfWord reports a printf word the shell may turn into other
// words: an expansion, a brace expansion, a tilde or a file name glob.
func computedPrintfWord(word *syntax.Word) bool {
	if !isLiteralWord(word) || globWord(globView(word)) || tildeWord(word) {
		return true
	}

	clone := &syntax.Word{Parts: slices.Clone(word.Parts)}

	return syntax.SplitBraces(clone)
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
