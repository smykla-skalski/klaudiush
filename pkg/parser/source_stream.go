package parser

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
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
	DetailSourceOption       = "it takes an option klaudiush does not follow"
	DetailSourcePath         = "it searches after PATH or source lookup changed earlier on the line"
	DetailShellOperand       = "it runs a script whose path comes from command output or a process substitution"
	DetailShellCommand       = "it runs a command line that comes from command output or a process substitution"
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
		if _, literal := procSubstOutput(word); literal && !stmtHeredocExpands(sub.Stmts[0]) {
			return "", false
		}

		return w.unseenSubst(sub), true
	}

	return w.sourcedParts(word.Parts)
}

// sourceOperandNext reports whether the next word of a command named name,
// after the arguments before, is the file source or . reads.
func sourceOperandNext(name string, before []string) bool {
	for name == builtinCommand || name == commandBuiltin {
		for len(before) > 0 && strings.HasPrefix(before[0], "-") {
			before = before[1:]
		}

		if len(before) == 0 {
			return false
		}

		name, before = before[0], before[1:]
	}

	if name != sourceBuiltin && name != dotBuiltin {
		return false
	}

	rest, _, ok := sourceOptions(before)

	return ok && len(rest) == 0
}

// sourcePathOption is bash 5.3's source -p PATH, which searches PATH for the
// file instead of $PATH.
const sourcePathOption = "-p"

// sourceOptions skips the options of source: -p PATH and --. It reports
// whether -p was given, and fails on an option it does not know.
func sourceOptions(args []string) (rest []string, searched, ok bool) {
	for len(args) > 0 {
		switch arg := args[0]; {
		case arg == endOfOptions:
			return args[1:], searched, true
		case arg == sourcePathOption && len(args) > 1:
			args, searched = args[2:], true
		case len(arg) > 1 && strings.HasPrefix(arg, "-"):
			return nil, searched, false
		default:
			return args, searched, true
		}
	}

	return nil, searched, true
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
// dirname "$0" of a script file, an allowed lookup, or cd to one of those
// followed by pwd (the usual SCRIPT_DIR idiom).
func (w *astWalker) lookedUpDir(sub *syntax.CmdSubst) (string, bool) {
	if dir, ok := w.changedDir(sub); ok {
		return dir, true
	}

	call := plainCall(sub)
	if call == nil {
		return "", false
	}

	if dir, ok := w.scriptDir(call.Args); ok {
		return dir, true
	}

	return w.lookupOutput(call.Args)
}

// markOutputSubst marks the commands of an output process substitution,
// >(cmd), as reading a pipe: they read what the command around it writes.
func (w *astWalker) markOutputSubst(sub *syntax.ProcSubst) {
	if sub.Op != syntax.CmdOut {
		return
	}

	for _, stmt := range sub.Stmts {
		w.markPiped(stmt, "")
	}
}

// changedDir returns the directory $(cd DIR && pwd) prints when DIR is one
// sourcedParts can render. pwd prints an absolute path, so a relative DIR
// is joined to the walker's directory when it is known.
func (w *astWalker) changedDir(sub *syntax.CmdSubst) (string, bool) {
	if len(sub.Stmts) != 1 || len(sub.Stmts[0].Redirs) > 0 {
		return "", false
	}

	and, ok := sub.Stmts[0].Cmd.(*syntax.BinaryCmd)
	if !ok || and.Op != syntax.AndStmt {
		return "", false
	}

	cd, pwd := callExprOf(and.X), callExprOf(and.Y)
	if cd == nil || pwd == nil || len(cd.Args) != 2 || len(and.X.Redirs) > 0 ||
		len(and.Y.Redirs) > 0 || !isLiteralWord(cd.Args[0]) || argWord(cd.Args[0]) != "cd" ||
		!printsDir(pwd) || w.defined("cd") || w.defined("pwd") {
		return "", false
	}

	dir, ok := w.sourcedParts(cd.Args[1].Parts)
	if !ok || dir == "" || HasUnresolvedVars(dir) || (searchesCDPath(dir) && !w.cdpathEmpty()) {
		return "", false
	}

	return resolvePath(w.currentDir, dir), true
}

// searchesCDPath reports a cd target the shell looks up in CDPATH: a
// relative path not starting with . or ..
func searchesCDPath(dir string) bool {
	return !filepath.IsAbs(dir) && dir != "." && dir != ".." &&
		!strings.HasPrefix(dir, "./") && !strings.HasPrefix(dir, "../")
}

// cdpathEmpty reports CDPATH known to be unset or empty, so cd goes to the
// directory it is given.
func (w *astWalker) cdpathEmpty() bool {
	const name = "CDPATH"

	if w.unknownVars[name] || w.state.dynamicVars[name] || w.state.namesUnknown {
		return false
	}

	if value, assigned := w.assignments[name]; assigned {
		return value == ""
	}

	value, set := w.resolver.LookupEnv(name)

	return !set || value == ""
}

// printsDir reports pwd, with -P or -L at most.
func printsDir(call *syntax.CallExpr) bool {
	args, literal := literalArgs(call.Args)
	if !literal || len(args) == 0 || args[0] != "pwd" {
		return false
	}

	for _, arg := range args[1:] {
		if arg != "-P" && arg != "-L" {
			return false
		}
	}

	return true
}

func (w *astWalker) setPiped(call *syntax.CallExpr, tool string) {
	if w.pipedByCall == nil {
		w.pipedByCall = make(map[*syntax.CallExpr]string)
	}

	w.pipedByCall[call] = tool
}

// notePiped carries the piped and untrusted stdin of call over to the
// command recorded at seq.
func (w *astWalker) notePiped(call *syntax.CallExpr, seq int) {
	if tool, ok := w.pipedByCall[call]; ok {
		if w.state.pipedStdin == nil {
			w.state.pipedStdin = make(map[int]string)
		}

		w.state.pipedStdin[seq] = tool
	}

	if tool, ok := w.untrustedByCall[call]; ok {
		if w.state.untrustedStdin == nil {
			w.state.untrustedStdin = make(map[int]string)
		}

		w.state.untrustedStdin[seq] = tool
	}
}

// noteStdinRedirects marks the commands whose stdin a redirect of stmt
// feeds in a way the walker does not capture: a redirect on a group or
// loop, more than one stdin redirect (the shell takes the last), a
// duplicated descriptor, a process substitution, or a heredoc or
// here-string with an expansion. exec changes the stdin of every later
// command in the shell, so it marks the whole parse.
func (w *astWalker) noteStdinRedirects(stmt *syntax.Stmt) {
	var redirs []*syntax.Redirect

	for _, redir := range stmt.Redirs {
		if redirectsStdin(redir) {
			redirs = append(redirs, redir)
		}
	}

	if len(redirs) == 0 {
		return
	}

	call := callExprOf(stmt)

	switch last := redirs[len(redirs)-1]; {
	case call == nil:
		w.markPiped(stmt, "")
	case runsExec(call):
		w.stdinReplaced = true
	case len(redirs) > 1, last.Op == syntax.DplIn, last.Op == syntax.RdrInOut,
		heredocExpands(last):
		w.setUntrusted(call, "")
	case last.Op == syntax.RdrIn:
		if sub := soleProcSubst(last.Word); sub != nil {
			w.setUntrusted(call, w.knownSetupTool(sub.Stmts))
		}
	}
}

// runsExec reports a call of exec, also through builtin or command, which
// replaces the shell's own stdin when given only redirects.
func runsExec(call *syntax.CallExpr) bool {
	for _, word := range call.Args {
		if !isLiteralWord(word) {
			return false
		}

		switch name := argWord(word); {
		case name == execBuiltin:
			return true
		case name != builtinCommand && name != commandBuiltin && !strings.HasPrefix(name, "-"):
			return false
		}
	}

	return false
}

// heredocExpands reports a heredoc or here-string whose text holds an
// expansion, which the shell fills in with output or values the parser
// does not see.
func heredocExpands(redir *syntax.Redirect) bool {
	word := redir.Hdoc
	if redir.Op == syntax.WordHdoc {
		word = redir.Word
	}

	return word != nil && expands(word.Parts)
}

func expands(parts []syntax.WordPart) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp, *syntax.ParamExp:
			return true
		case *syntax.DblQuoted:
			if expands(p.Parts) {
				return true
			}
		}
	}

	return false
}

// stmtHeredocExpands reports a statement fed a heredoc or here-string with
// an expansion.
func stmtHeredocExpands(stmt *syntax.Stmt) bool {
	return slices.ContainsFunc(stmt.Redirs, func(redir *syntax.Redirect) bool {
		return redirectsStdin(redir) && heredocExpands(redir)
	})
}

func (w *astWalker) setUntrusted(call *syntax.CallExpr, tool string) {
	if w.untrustedByCall == nil {
		w.untrustedByCall = make(map[*syntax.CallExpr]string)
	}

	w.untrustedByCall[call] = tool
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

	return w.stdinFed || w.stdinReplaced || piped || cmd.Stdin != "" || cmd.StdinFile != ""
}

// sourceLaunch returns the file source or . reads, given the arguments
// before command output is dropped from them. What klaudiush cannot see,
// such as source <(curl ...), fails closed.
func (w *astWalker) sourceLaunch(cmd Command) []scriptFile {
	args, searched, ok := sourceOptions(cmd.Args)
	if !ok || (searched && len(args) > 0 && !strings.Contains(args[0], "/")) {
		w.addOpacity(sourceOpacity(cmd, DetailSourceOption, ""))

		return nil
	}

	if len(args) == 0 {
		return nil
	}

	if !strings.Contains(args[0], "/") {
		if w.state.pathChanged {
			w.addOpacity(sourceOpacity(cmd, DetailSourcePath, ""))

			return nil
		}

		args[0] = w.sourceSearchPath(cmd, args[0])
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

// sourceSearchPath returns the first PATH candidate that exists on disk or
// was written earlier on the line, then the cwd spelling when none exists.
func (w *astWalker) sourceSearchPath(cmd Command, name string) string {
	diskPath, diskFound := w.resolver.LookSource(name, cmd.WorkingDirectory)

	pathValue, pathKnown := w.resolver.LookupEnv(pathVar)
	if !pathKnown {
		if diskFound {
			return diskPath
		}

		return name
	}

	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			dir = "."
		}

		candidate := resolvePath(cmd.WorkingDirectory, filepath.Join(dir, name))

		_, written, _ := w.lastLineWrite(candidate)
		if written || w.unplacedWriteBefore(cmd, candidate) {
			return candidate
		}

		if diskFound && filepath.Clean(candidate) == filepath.Clean(diskPath) {
			return diskPath
		}
	}

	if diskFound {
		return diskPath
	}

	return name
}

// sourcePath returns the path of the script source reads for operand, ""
// when it reads nothing, or why what it reads cannot be seen. A path with
// a variable klaudiush cannot resolve is left for scriptSource to report.
func (w *astWalker) sourcePath(cmd Command, operand string) (string, Opacity) {
	if tool, unseen := w.state.unseenSubsts[operand]; unseen {
		return "", sourceOpacity(cmd, DetailSourceSubstitution, tool)
	}

	if marked(operand) || w.fromOutput(operand) {
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
	case path == "-":
		return "./-", Opacity{}
	case stdinPaths[clean]:
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
	if tool, untrusted := w.state.untrustedStdin[cmd.Location.Seq]; untrusted {
		return "", sourceOpacity(cmd, DetailSourceStdin, tool)
	}

	tool, piped := w.state.pipedStdin[cmd.Location.Seq]

	switch {
	case cmd.Stdin != "":
		return devStdin, Opacity{}
	case cmd.StdinFile != "":
		path, detail := redirectedStdin(cmd, cmd.StdinFile)
		if detail != "" {
			return "", sourceOpacity(cmd, DetailSourceStdin, "")
		}

		return path, Opacity{}
	case piped || w.stdinFed || w.stdinReplaced:
		return "", sourceOpacity(cmd, DetailSourceStdin, tool)
	default:
		return "", Opacity{}
	}
}

// shellStreamLaunch returns what a shell reads after shellLaunch found no
// literal input. Pipes and redirects whose contents are not captured are
// opaque because the shell executes them as code.
func (w *astWalker) shellStreamLaunch(cmd Command, visible launch) launch {
	operand, isScript, ok := shellOperand(cmd.Args)
	if !ok {
		return w.shellStdinLaunch(cmd, visible)
	}

	return w.shellOperandLaunch(cmd, operand, isScript, visible)
}

func (w *astWalker) shellOperandLaunch(
	cmd Command,
	operand string,
	isScript bool,
	visible launch,
) launch {
	if isScript && marked(operand) {
		w.addOpacity(sourceOpacity(cmd, DetailShellCommand, ""))

		return launch{}
	}

	if isScript {
		return visible
	}

	if marked(operand) {
		w.addOpacity(sourceOpacity(cmd, DetailShellOperand, ""))

		return launch{}
	}

	if operand == "-" {
		return w.shellStdinLaunch(cmd, visible)
	}

	path, opacity := w.sourcePath(cmd, operand)
	if opacity.Cause != "" {
		w.addOpacity(opacity)

		return launch{}
	}

	if path == "" {
		return launch{}
	}

	if path != operand {
		if len(visible.files) > 0 {
			visible.files[0].path = path

			return visible
		}

		return launch{files: []scriptFile{{path: path, explicit: true}}}
	}

	return visible
}

func (w *astWalker) shellStdinLaunch(cmd Command, visible launch) launch {
	path, opacity := w.sourceStdin(cmd)
	if opacity.Cause != "" {
		w.addOpacity(opacity)

		return launch{}
	}

	if path == devStdin {
		if len(visible.files) > 0 {
			visible.files[0].path = path

			return visible
		}

		return launch{scripts: []string{cmd.Stdin}}
	}

	if path != "" {
		if len(visible.files) > 0 {
			visible.files[0].path = path

			return visible
		}

		return launch{files: []scriptFile{{path: path, explicit: true}}}
	}

	return visible
}

// referencedVar matches a variable a rendered word refers to.
var referencedVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)`)

// fromOutput reports a word using a variable last assigned command output,
// as in f=$(curl ...); source "$f".
func (w *astWalker) fromOutput(word string) bool {
	for _, match := range referencedVar.FindAllStringSubmatch(word, -1) {
		if w.state.dynamicVars[match[1]] {
			return true
		}
	}

	return false
}

func sourceOpacity(cmd Command, detail, tool string) Opacity {
	return Opacity{Cause: OpacitySourcedStream, Operation: cmd.Name, Detail: detail, Tool: tool}
}
