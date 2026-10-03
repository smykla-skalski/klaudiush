package parser

import (
	"fmt"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// OpacitySourcedStream means source or . runs code klaudiush cannot see: the
// output of a process substitution, stdin fed by a pipe or redirect it does
// not capture, another file descriptor or device, or a file whose path comes
// from command output.
const OpacitySourcedStream OpacityCause = "sourced-stream"

// Fixed reasons, set as Opacity.Detail, that what source or . reads is
// opaque.
const (
	DetailSourceSubstitution = "it reads the output of a process substitution klaudiush cannot see"
	DetailSourceStdin        = "it reads stdin, fed by a command or redirect klaudiush cannot see"
	DetailSourceDescriptor   = "it reads a file descriptor or device klaudiush cannot follow"
	DetailSourceOutput       = "it reads a file whose path comes from command output"
)

// unseenInfix marks the stand-in path of a process substitution whose
// output is unknown.
const unseenInfix = "unseen-"

// stdinPaths are the files that read the current process's stdin.
var stdinPaths = nameSet(devStdin + " /dev/fd/0 /proc/self/fd/0")

// soleProcSubst returns the input process substitution that is all of
// word.
func soleProcSubst(word *syntax.Word) *syntax.ProcSubst {
	if word == nil || len(word.Parts) != 1 {
		return nil
	}

	sub, ok := word.Parts[0].(*syntax.ProcSubst)
	if !ok || sub.Op != syntax.CmdIn {
		return nil
	}

	return sub
}

// unseenSubst returns a stand-in path for a process substitution whose
// output is unknown, remembering the setup tool it runs, if any.
func (w *astWalker) unseenSubst(sub *syntax.ProcSubst) string {
	if w.state.unseenSubsts == nil {
		w.state.unseenSubsts = make(map[string]string)
	}

	path := fmt.Sprintf("%s%s%d", procSubstPrefix, unseenInfix, len(w.state.unseenSubsts))
	w.state.unseenSubsts[path] = w.knownSetupTool(sub.Stmts)

	return path
}

// markPiped marks every command stmt runs as reading a stdin the walker
// does not capture, fed by tool when it is a known setup tool. Commands
// inside a group or loop, and in substitutions, share that stdin.
func (w *astWalker) markPiped(stmt *syntax.Stmt, tool string) {
	syntax.Walk(stmt, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok {
			w.setPiped(call, tool)
		}

		return true
	})
}

// sourcedFile renders the operand of source or . (also run through builtin
// or command) when it holds a substitution. A process substitution with
// unknown output becomes a stand-in path, so source can name it. A path
// built from command substitutions is rendered when each one is the
// directory of the running script (dirname "$0") or an allowed lookup (see
// AllowedLookups), so source "$(dirname "$0")/lib.sh" is followed; any
// other leaves the operand to fail closed as command output. Other
// commands keep their arguments as they were.
func (w *astWalker) sourcedFile(name string, before []string, word *syntax.Word) (string, bool) {
	if !sourceOperandNext(name, before) || !hasCmdSubst(word.Parts) {
		return "", false
	}

	if sub := soleProcSubst(word); sub != nil {
		if _, literal := procSubstOutput(word); literal {
			return "", false
		}

		return w.unseenSubst(sub), true
	}

	return w.sourcedParts(word.Parts)
}

// sourceOperandNext reports whether the next word of a command named name,
// after the arguments before, is the file source or . reads.
func sourceOperandNext(name string, before []string) bool {
	if name == builtinCommand || name == commandBuiltin {
		if len(before) == 0 || (before[0] != sourceBuiltin && before[0] != dotBuiltin) {
			return false
		}

		name, before = before[0], before[1:]
	}

	return (name == sourceBuiltin || name == dotBuiltin) &&
		(len(before) == 0 || (len(before) == 1 && before[0] == endOfOptions))
}

func (w *astWalker) sourcedParts(parts []syntax.WordPart) (string, bool) {
	var b strings.Builder

	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.CmdSubst:
			dir, ok := w.lookedUpDir(p)
			if !ok {
				return "", false
			}

			b.WriteString(dir)
		case *syntax.DblQuoted:
			inner, ok := w.sourcedParts(p.Parts)
			if !ok {
				return "", false
			}

			b.WriteString(inner)
		case *syntax.ParamExp:
			b.WriteString(w.scriptDirParam(p))
		case *syntax.ProcSubst, *syntax.ArithmExp, *syntax.ExtGlob:
			return "", false
		default:
			b.WriteString(argWord(&syntax.Word{Parts: []syntax.WordPart{part}}))
		}
	}

	return b.String(), true
}

// lookedUpDir returns the directory a command substitution prints when it is
// dirname "$0" of a script file or an allowed lookup.
func (w *astWalker) lookedUpDir(sub *syntax.CmdSubst) (string, bool) {
	call := plainCall(sub)
	if call == nil {
		return "", false
	}

	if dir, ok := w.scriptDir(call.Args); ok {
		return dir, true
	}

	return w.lookupOutput(call.Args)
}

func (w *astWalker) setPiped(call *syntax.CallExpr, tool string) {
	if w.pipedByCall == nil {
		w.pipedByCall = make(map[*syntax.CallExpr]string)
	}

	w.pipedByCall[call] = tool
}

// notePiped carries the piped stdin of call over to the command recorded
// at seq.
func (w *astWalker) notePiped(call *syntax.CallExpr, seq int) {
	tool, ok := w.pipedByCall[call]
	if !ok {
		return
	}

	if w.state.pipedStdin == nil {
		w.state.pipedStdin = make(map[int]string)
	}

	w.state.pipedStdin[seq] = tool
}

// noteStdinRedirects marks the commands whose stdin a redirect of stmt
// feeds in a way the walker does not capture: a redirect on a group or
// loop, a duplicated descriptor, or a process substitution. exec with only
// redirects changes the stdin of every later command.
func (w *astWalker) noteStdinRedirects(stmt *syntax.Stmt) {
	call := callExprOf(stmt)

	for _, redir := range stmt.Redirs {
		if !redirectsStdin(redir) {
			continue
		}

		switch {
		case call == nil:
			w.markPiped(stmt, "")
		case len(call.Args) == 1 && isLiteralWord(call.Args[0]) && argWord(call.Args[0]) == execBuiltin:
			w.stdinFed = true
		case redir.Op == syntax.DplIn || redir.Op == syntax.RdrInOut:
			w.setPiped(call, "")
		case redir.Op == syntax.RdrIn:
			if sub := soleProcSubst(redir.Word); sub != nil {
				w.setPiped(call, w.knownSetupTool(sub.Stmts))
			}
		}
	}
}

// redirectsStdin reports a redirect that replaces stdin.
func redirectsStdin(redir *syntax.Redirect) bool {
	if redir.N != nil && redir.N.Value != "0" {
		return false
	}

	switch redir.Op {
	case syntax.RdrIn, syntax.RdrInOut, syntax.DplIn,
		syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
		return true
	default:
		return false
	}
}

// feedsStdin reports whether cmd runs with a stdin the scripts it runs
// inherit: literal text, a file, a pipe, or the stdin it inherited itself.
func (w *astWalker) feedsStdin(cmd Command) bool {
	_, piped := w.state.pipedStdin[cmd.Location.Seq]

	return w.stdinFed || piped || cmd.Stdin != "" || cmd.StdinFile != ""
}

// sourceLaunch returns the file source or . reads, given the arguments
// before command output is dropped from them. What klaudiush cannot see,
// such as source <(curl ...), fails closed.
func (w *astWalker) sourceLaunch(cmd Command) []scriptFile {
	args := cmd.Args
	if len(args) > 0 && args[0] == endOfOptions {
		args = args[1:]
	}

	if len(args) == 0 {
		return nil
	}

	path, o := w.sourcePath(cmd, args[0])
	if o.Cause != "" {
		w.addOpacity(o)

		return nil
	}

	if path == "" {
		return nil
	}

	return []scriptFile{{path: path, explicit: true, args: args[1:], withArgs: len(args) > 1}}
}

// sourcePath returns the path of the script source reads for operand, ""
// when it reads nothing, or why what it reads cannot be seen. A path with
// a variable klaudiush cannot resolve is left for scriptSource to report.
func (w *astWalker) sourcePath(cmd Command, operand string) (string, Opacity) {
	if tool, unseen := w.state.unseenSubsts[operand]; unseen {
		return "", sourceOpacity(cmd, DetailSourceSubstitution, tool)
	}

	if marked(operand) {
		return "", sourceOpacity(cmd, DetailSourceOutput, "")
	}

	if _, captured := w.scriptFiles[operand]; captured {
		return operand, Opacity{}
	}

	path := w.expandName(operand)
	if HasUnresolvedVars(path) {
		return operand, Opacity{}
	}

	clean := resolvePath(cmd.WorkingDirectory, path)

	switch {
	case path == "-" || stdinPaths[clean]:
		return w.sourceStdin(cmd)
	case clean == devNull:
		return "", Opacity{}
	case specialPath(clean) || (!filepath.IsAbs(clean) && specialPath(filepath.Join("/", clean))):
		return "", sourceOpacity(cmd, DetailSourceDescriptor, "")
	default:
		return operand, Opacity{}
	}
}

// sourceStdin returns what source of stdin reads: literal text, or a regular
// file redirected to it. Stdin from a pipe, a process substitution or a
// descriptor, or inherited from the command running the script, is opaque.
// With nothing on stdin it reads nothing.
func (w *astWalker) sourceStdin(cmd Command) (string, Opacity) {
	tool, piped := w.state.pipedStdin[cmd.Location.Seq]

	switch {
	case cmd.Stdin != "":
		return devStdin, Opacity{}
	case cmd.StdinFile != "":
		path, detail := redirectedStdin(cmd, cmd.StdinFile)
		if detail != "" {
			return "", sourceOpacity(cmd, DetailSourceStdin, tool)
		}

		return path, Opacity{}
	case piped || w.stdinFed:
		return "", sourceOpacity(cmd, DetailSourceStdin, tool)
	default:
		return "", Opacity{}
	}
}

func sourceOpacity(cmd Command, detail, tool string) Opacity {
	return Opacity{Cause: OpacitySourcedStream, Operation: cmd.Name, Detail: detail, Tool: tool}
}
