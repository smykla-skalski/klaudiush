package parser

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// astWalker walks the AST and extracts commands and file operations.
type astWalker struct {
	commands   []Command
	fileWrites []FileWrite
	currentDir string // Tracks the effective working directory from cd commands
	// stdinByCall maps a CallExpr to the content fed to its stdin (heredoc or
	// piped echo/printf). Populated when a Stmt or pipeline is visited, then
	// consumed when the corresponding CallExpr is extracted into a Command.
	stdinByCall map[*syntax.CallExpr]string
	// stdinFileByCall maps a CallExpr to the file redirected to its stdin (<).
	stdinFileByCall map[*syntax.CallExpr]string
	// assignments records literal NAME=value assignments, both standalone and
	// as a prefix on a command, so consumers can resolve a variable used later
	// in the same command line.
	assignments map[string]string
	// depth counts the launchers, scripts and aliases that led here.
	depth int
	// resolver answers what the command text cannot: environment, script
	// files, program identity and git aliases.
	resolver Resolver
	// aliases and funcs hold the aliases and functions defined earlier on the
	// line, so a later call is followed into what it runs.
	aliases map[string]string
	funcs   map[string]string
	// scriptFiles holds the content of process substitutions by stand-in path.
	scriptFiles map[string]string
	// truncated records that something past maxLaunchDepth was not followed.
	truncated bool
}

// visit is called for each node in the AST.
func (w *astWalker) visit(node syntax.Node) bool {
	switch n := node.(type) {
	case *syntax.BinaryCmd:
		w.extractPipedStdin(n)
	case *syntax.CallExpr:
		w.extractCommand(n)
	case *syntax.FuncDecl:
		w.defineFunc(n)
	case *syntax.Stmt:
		w.extractRedirect(n)
	case *syntax.Subshell:
		// Subshells are handled recursively by syntax.Walk
		return true
	case *syntax.CmdSubst:
		// Command substitution is handled recursively
		return true
	}

	return true
}

// recordStdin associates stdin content with a CallExpr so it can be attached
// to the Command when that CallExpr is later extracted.
func (w *astWalker) recordStdin(call *syntax.CallExpr, content string) {
	w.stdinByCall[call] = content
}

// extractPipedStdin handles "producer | consumer" pipelines, capturing the
// producer's literal output (echo/printf) as the consumer's stdin. This lets
// validators inspect messages fed via "git commit -F -".
func (w *astWalker) extractPipedStdin(bin *syntax.BinaryCmd) {
	if bin.Op != syntax.Pipe && bin.Op != syntax.PipeAll {
		return
	}

	producer := callExprOf(bin.X)
	consumer := callExprOf(bin.Y)

	if producer == nil || consumer == nil {
		return
	}

	if content, ok := literalCommandOutput(producer); ok {
		w.recordStdin(consumer, content)

		return
	}

	// cat passes on a heredoc or a file unchanged, so "cat <<EOF | bash" and
	// "cat x.sh | bash" hand bash that script.
	if !isCommand(producer, "cat") {
		return
	}

	info := collectRedirs(bin.X)

	switch {
	case info.hasHeredoc && copiesStdinVerbatim(producer):
		w.recordStdin(consumer, info.heredocContent)
	case info.inputPath != "" && copiesStdinVerbatim(producer):
		w.stdinFileByCall[consumer] = info.inputPath
	case len(producer.Args) == 2 && isLiteralWord(producer.Args[1]):
		if file := wordToString(producer.Args[1]); file != "-" && !strings.HasPrefix(file, "-") {
			w.stdinFileByCall[consumer] = file
		}
	}
}

// isCommand reports whether call runs the named program.
func isCommand(call *syntax.CallExpr, name string) bool {
	return call != nil && len(call.Args) > 0 && commandName(wordToString(call.Args[0])) == name
}

// callExprOf returns the CallExpr a statement runs, or nil if the statement is
// not a simple command.
func callExprOf(stmt *syntax.Stmt) *syntax.CallExpr {
	if stmt == nil {
		return nil
	}

	if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
		return call
	}

	return nil
}

// literalCommandOutput returns the text a simple echo/printf command writes to
// stdout, when it can be determined from literal arguments alone.
func literalCommandOutput(call *syntax.CallExpr) (string, bool) {
	// callExprOf returns nil for statements that are not a simple command (e.g.
	// a redirected block or subshell like "{ echo hi; } > out"), so guard
	// against a nil call to avoid a panic.
	if call == nil || len(call.Args) == 0 {
		return "", false
	}

	name := wordToString(call.Args[0])
	if name != "echo" && name != "printf" {
		return "", false
	}

	// Only capture when every argument is strictly literal. Expansions
	// (parameters, command/arithmetic substitution, globs) cannot be
	// reproduced here, so capturing would yield a message different from what
	// the command actually emits.
	args, ok := literalArgs(call.Args[1:])
	if !ok {
		return "", false
	}

	if name == "echo" {
		return echoOutput(args)
	}

	return printfOutput(args) // name == "printf"
}

// literalArgs converts words to strings only when every word is strictly
// literal. It returns false as soon as any word contains a shell expansion.
func literalArgs(words []*syntax.Word) ([]string, bool) {
	args := make([]string, 0, len(words))

	for _, word := range words {
		if !isLiteralWord(word) {
			return nil, false
		}

		args = append(args, wordToString(word))
	}

	return args, true
}

// isLiteralWord reports whether a word consists solely of literal text - plain
// literals and single/double-quoted literals - with no shell expansions such as
// parameters, command/arithmetic substitution, or globbing.
func isLiteralWord(word *syntax.Word) bool {
	if word == nil {
		return false
	}

	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit, *syntax.SglQuoted:
			// Literal text, no expansion.
		case *syntax.DblQuoted:
			if !allLiteralParts(p.Parts) {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// allLiteralParts reports whether every part is a plain literal (no expansions),
// as found inside a double-quoted string.
func allLiteralParts(parts []syntax.WordPart) bool {
	for _, p := range parts {
		if _, ok := p.(*syntax.Lit); !ok {
			return false
		}
	}

	return true
}

// echoOutput returns the literal text content of an "echo args..." call. Only
// the leading run of echo flags (-n, -e, -E and combinations) is stripped; once
// a non-flag word appears, every remaining argument is literal, even if it
// starts with "-". It returns false when -e is present, since that enables
// backslash-escape interpretation which this helper does not perform - capturing
// the raw text would mis-validate the message. This is a best-effort capture for
// validation: it joins the arguments with spaces and omits the trailing newline
// echo would normally emit (callers trim surrounding whitespace anyway).
func echoOutput(args []string) (string, bool) {
	i := 0

	for i < len(args) {
		flag, ok := echoFlag(args[i])
		if !ok {
			break
		}

		if strings.ContainsRune(flag, 'e') {
			return "", false
		}

		i++
	}

	return strings.Join(args[i:], " "), true
}

// echoFlag reports whether arg is an echo option like -n, -e, -E, or -ne and
// returns its letters (without the leading dash) when so.
func echoFlag(arg string) (string, bool) {
	if len(arg) < 2 || arg[0] != '-' {
		return "", false
	}

	for _, c := range arg[1:] {
		if c != 'n' && c != 'e' && c != 'E' {
			return "", false
		}
	}

	return arg[1:], true
}

// printfOutput returns the text a simple "printf" call writes, for the forms
// used to build commit messages: a bare literal format, or a format whose only
// directives are %s (one string argument each) and %%. Backslash escapes in the
// format (\n, \t, \r, \\, \a, \b, \f, \v) are interpreted, so a multi-line
// message like "printf '%s\n\n%s\n' title body" is reconstructed correctly. A
// single trailing newline is dropped (callers trim surrounding whitespace
// anyway). It returns false for any other directive (e.g. %d, %q), an unknown
// escape, or a %s/argument count mismatch, so an uncertain capture falls back to
// a disk read rather than validating reconstructed-wrong bytes.
func printfOutput(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}

	out, ok := expandPrintf(args[0], args[1:])
	if !ok {
		return "", false
	}

	return strings.TrimSuffix(out, "\n"), true
}

// expandPrintf reproduces a printf call's output for formats that use only %s
// and %% directives plus known backslash escapes. It returns false when the
// format contains any other directive, an unknown or trailing escape, a dangling
// "%", or when the number of %s directives does not equal len(args) - printf's
// format recycling and missing-argument behaviour are intentionally not
// reproduced, so such calls decline capture instead of guessing.
func expandPrintf(format string, args []string) (string, bool) {
	var b strings.Builder

	used := 0

	for i := 0; i < len(format); i++ {
		switch format[i] {
		case '\\':
			i++
			if i >= len(format) {
				return "", false
			}

			esc, ok := printfEscape(format[i])
			if !ok {
				return "", false
			}

			b.WriteByte(esc)
		case '%':
			i++
			if i >= len(format) {
				return "", false
			}

			ok := writePrintfVerb(&b, format[i], args, &used)
			if !ok {
				return "", false
			}
		default:
			b.WriteByte(format[i])
		}
	}

	if used != len(args) {
		return "", false
	}

	return b.String(), true
}

// writePrintfVerb handles the character after a "%" in a printf format. It
// supports "%%" (a literal percent) and "%s" (consuming the next argument),
// returning false for any other verb or when no argument remains for "%s".
func writePrintfVerb(b *strings.Builder, verb byte, args []string, used *int) bool {
	switch verb {
	case '%':
		b.WriteByte('%')

		return true
	case 's':
		if *used >= len(args) {
			return false
		}

		b.WriteString(args[*used])
		*used++

		return true
	default:
		return false
	}
}

// printfEscape interprets a backslash escape in a printf format string. It
// returns false for escapes this best-effort reproducer does not handle (e.g.
// octal or hex), so the caller can decline to capture rather than guess.
func printfEscape(c byte) (byte, bool) {
	switch c {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case '\\':
		return '\\', true
	case 'a':
		return '\a', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'v':
		return '\v', true
	default:
		return 0, false
	}
}

// extractCommand extracts a command from a CallExpr node.
func (w *astWalker) extractCommand(call *syntax.CallExpr) {
	w.extractAssigns(call)

	if len(call.Args) == 0 {
		return
	}

	// First word is the command name
	name := commandWord(call.Args[0])
	args := w.argStrings(call.Args[1:])

	// The shell expands {git,commit,-m,x} into words before running anything.
	if words := braceWords(call.Args[0]); len(words) > 0 {
		name, args = words[0], slices.Concat(words[1:], args)
	}

	if name == "" {
		return
	}

	w.recordCommand(Command{
		Name:             name,
		Args:             args,
		Location:         Location{Line: call.Pos().Line(), Column: call.Pos().Col()},
		Type:             CmdTypeSimple,
		WorkingDirectory: w.currentDir,
		Stdin:            w.stdinByCall[call],
		StdinFile:        w.stdinFileByCall[call],
	}, w.depth)
}

// recordCommand stores cmd under the program it really runs, then follows
// what it launches: the command behind a launcher, a script handed to a shell,
// eval or an interpreter, a script file, a same-line alias or function, or a
// git alias. Without this, /usr/bin/git, env git, bash -c "git ...", ./x.sh
// or an alias would each hide a git command from every validator.
func (w *astWalker) recordCommand(cmd Command, depth int) {
	cmd.Invoked = w.expandName(cmd.Name)

	// An expansion splits into words, so x="git commit"; $x runs git.
	if strings.Contains(cmd.Name, "${") {
		if fields := strings.Fields(cmd.Invoked); len(fields) > 1 {
			cmd.Invoked, cmd.Args = fields[0], slices.Concat(fields[1:], cmd.Args)
		}
	}

	cmd.Name = commandName(cmd.Invoked)

	cmd, aliasScripts := w.resolveProgram(cmd)

	w.defineAliases(cmd)
	w.commands = append(w.commands, cmd)

	if cmd.Name == "cd" && len(cmd.Args) > 0 {
		w.currentDir = cmd.Args[0]
	}

	w.extractFileWriteCommand(cmd)

	l := launched(cmd)
	l.scripts = slices.Concat(l.scripts, aliasScripts, w.definitionScripts(cmd))

	if l.empty() {
		return
	}

	// Past the cap nothing more is followed, and the command fails closed:
	// what it launches cannot be shown to be safe.
	if depth >= maxLaunchDepth {
		w.truncated = true

		return
	}

	w.follow(cmd, l, depth+1)
}

// redirInfo holds the output redirection and heredoc found on a statement.
type redirInfo struct {
	outputPath     string
	outputOp       WriteOp
	outputLoc      Location
	heredocContent string
	heredocLoc     Location
	inputPath      string // file redirected to stdin (<)
	hasOutput      bool
	hasHeredoc     bool
}

// collectRedirs gathers output redirection and heredoc details from a statement.
func collectRedirs(stmt *syntax.Stmt) redirInfo {
	var info redirInfo

	for _, redir := range stmt.Redirs {
		switch redir.Op {
		case syntax.RdrOut, syntax.AppOut:
			path := wordToString(redir.Word)
			if path == "" {
				continue
			}

			info.outputPath = path

			info.outputOp = WriteOpRedirect
			if redir.Op == syntax.AppOut {
				info.outputOp = WriteOpAppend
			}

			info.outputLoc = Location{Line: redir.Pos().Line(), Column: redir.Pos().Col()}
			info.hasOutput = true
		case syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			switch {
			case redir.Op == syntax.WordHdoc:
				// A here-string feeds its word, plus a newline, to stdin.
				info.heredocContent = wordToString(redir.Word) + "\n"
			case redir.Hdoc != nil:
				// Extract heredoc content from Hdoc field (may be empty).
				info.heredocContent = wordToString(redir.Hdoc)
			}
			// Mark as heredoc even if content is empty.
			info.heredocLoc = Location{Line: redir.Pos().Line(), Column: redir.Pos().Col()}
			info.hasHeredoc = true
		case syntax.RdrIn:
			info.inputPath = wordToString(redir.Word)
		default:
			// Other redirection operators are not relevant here.
		}
	}

	return info
}

// extractRedirect extracts file write operations and stdin content from redirections.
func (w *astWalker) extractRedirect(stmt *syntax.Stmt) {
	if stmt.Redirs == nil {
		return
	}

	info := collectRedirs(stmt)

	// A heredoc always feeds the command's stdin, regardless of any output
	// redirection on the same statement. Record it so validators can inspect
	// stdin-fed content (e.g. "git commit -F - <<EOF ... EOF >/dev/null").
	if info.hasHeredoc {
		if call := callExprOf(stmt); call != nil {
			w.recordStdin(call, info.heredocContent)
		}
	}

	if info.inputPath != "" {
		if call := callExprOf(stmt); call != nil {
			w.stdinFileByCall[call] = info.inputPath
		}
	}

	switch {
	case info.hasOutput && info.hasHeredoc:
		// Output redirection combined with a heredoc. The heredoc body equals the
		// file bytes only when it is an overwrite (">", not ">>") and the command
		// copies stdin to stdout verbatim (cat). A transforming command such as
		// "grep foo > f <<EOF" writes filtered output, not the heredoc body, so
		// that content must not be treated as captured.
		captured := info.outputOp == WriteOpRedirect && copiesStdinVerbatim(callExprOf(stmt))
		w.fileWrites = append(w.fileWrites, FileWrite{
			Path:             info.outputPath,
			Operation:        WriteOpHeredoc,
			Content:          info.heredocContent,
			ContentCaptured:  captured,
			Location:         info.heredocLoc,
			WorkingDirectory: w.currentDir,
		})
	case info.hasOutput:
		// Just output redirection without heredoc. A literal overwrite's output
		// is captured into RedirectContent for inline commit-message recovery
		// (e.g. printf '%s' msg > "$MSG"; git commit -F "$MSG"), but never into
		// Content: Content flows to the dispatcher's synthetic file-write
		// validation, which would lint partial bytes (e.g. "echo 'x' > foo.go"
		// tripping gofumpt).
		fw := FileWrite{
			Path:             info.outputPath,
			Operation:        info.outputOp,
			Location:         info.outputLoc,
			WorkingDirectory: w.currentDir,
		}

		if info.outputOp == WriteOpRedirect {
			if content, ok := literalCommandOutput(callExprOf(stmt)); ok {
				fw.RedirectContent = content
				fw.RedirectContentCaptured = true
			}
		}

		w.fileWrites = append(w.fileWrites, fw)
	}
}

// copiesStdinVerbatim reports whether the command copies its stdin to stdout
// unchanged, so that a heredoc fed to it becomes the exact redirected output.
// Only "cat" (reading stdin implicitly) or "cat -" (reading stdin once) qualify.
// "cat file", "cat -n", "cat - -" (stdin emitted more than once), and
// transforming commands (grep, sed, ...) do not.
func copiesStdinVerbatim(call *syntax.CallExpr) bool {
	if call == nil || len(call.Args) == 0 {
		return false
	}

	if wordToString(call.Args[0]) != "cat" {
		return false
	}

	switch rest := call.Args[1:]; len(rest) {
	case 0:
		return true // "cat"
	case 1:
		return wordToString(rest[0]) == "-" // "cat -"
	default:
		return false // "cat - -", "cat file", ...
	}
}

// extractAssigns records NAME=value assignments carried by a call, whether the
// call is a bare assignment or a command with assignment prefixes. Appends
// (NAME+=value) and naked assignments carry no complete value, so they are
// skipped rather than recorded with a partial one.
func (w *astWalker) extractAssigns(call *syntax.CallExpr) {
	for _, assign := range call.Assigns {
		if assign.Name == nil || assign.Append || assign.Naked {
			continue
		}

		switch {
		case assign.Value != nil:
			w.assignments[assign.Name.Value] = wordToString(assign.Value)
		case assign.Array != nil:
			// An array is kept as its elements joined by spaces, which is what
			// "${NAME[@]}" expands to as separate words.
			elems := make([]string, 0, len(assign.Array.Elems))
			for _, elem := range assign.Array.Elems {
				elems = append(elems, wordToString(elem.Value))
			}

			w.assignments[assign.Name.Value] = strings.Join(elems, " ")
		}
	}
}

// extractFileWriteCommand detects file write commands (tee, cp, mv).
func (w *astWalker) extractFileWriteCommand(cmd Command) {
	op, targets := getFileWriteOperation(cmd)
	if op == WriteOpNone {
		return
	}

	for _, target := range targets {
		fw := FileWrite{
			Path:             target,
			Operation:        op,
			Source:           cmd.Name,
			Location:         cmd.Location,
			WorkingDirectory: cmd.WorkingDirectory,
		}

		w.fileWrites = append(w.fileWrites, fw)
	}
}

// getFileWriteOperation determines if a command writes to files.
func getFileWriteOperation(cmd Command) (WriteOp, []string) {
	switch cmd.Name {
	case "tee":
		// tee writes to all file arguments
		return WriteOpTee, extractTeeTargets(cmd.Args)

	case "cp", "copy":
		// cp writes to the last argument
		if len(cmd.Args) >= 2 { //nolint:mnd // Trivial check for minimum args (source + dest)
			return WriteOpCopy, []string{cmd.Args[len(cmd.Args)-1]}
		}

	case "mv", "move":
		// mv writes to the last argument
		if len(cmd.Args) >= 2 { //nolint:mnd // Trivial check for minimum args (source + dest)
			return WriteOpMove, []string{cmd.Args[len(cmd.Args)-1]}
		}
	}

	return WriteOpNone, nil
}

// extractTeeTargets extracts file targets from tee command arguments.
func extractTeeTargets(args []string) []string {
	targets := make([]string, 0)

	// Skip flags (starting with -)
	for _, arg := range args {
		if len(arg) > 0 && arg[0] != '-' {
			targets = append(targets, arg)
		}
	}

	return targets
}
