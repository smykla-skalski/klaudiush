package parser

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

const (
	// maxAliasDepth bounds how many git aliases are expanded in a chain.
	maxAliasDepth = 5
	// unresolvedProgram stands for a command substitution whose output cannot
	// be known. No program has this name, so it resolves as missing.
	unresolvedProgram = "$(...)"
	// procSubstPrefix names the files process substitutions stand for.
	procSubstPrefix = "/dev/fd/klaudiush-"
	// lookupWords is a lookup command plus the operand it names.
	lookupWords = 2
)

var (
	// gitAliasName matches a name git accepts as an alias.
	gitAliasName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// positionalParam matches the positional parameters a function body uses.
	positionalParam = regexp.MustCompile(`"?\$(?:\{([@*1-9])\}|([@*1-9]))"?`)
	// lookupCommands print the program named by their last operand.
	lookupCommands = nameSet("command echo printf readlink realpath type which whereis")
)

// newAstWalker returns a walker ready to record commands.
func newAstWalker(resolver Resolver) *astWalker {
	return &astWalker{
		commands:    make([]Command, 0),
		fileWrites:  make([]FileWrite, 0),
		assignments: make(map[string]string),
		resolver:    resolver,
		aliases:     make(map[string]string),
		funcs:       make(map[string]string),
		scriptFiles: make(map[string]string),
	}
}

// child returns a walker for a script run by a command at depth. It sees the
// variables, aliases and functions defined so far without leaking its own.
func (w *astWalker) child(dir string, depth int) *astWalker {
	child := newAstWalker(w.resolver)
	child.currentDir = dir
	child.depth = depth

	maps.Copy(child.assignments, w.assignments)
	maps.Copy(child.aliases, w.aliases)
	maps.Copy(child.funcs, w.funcs)

	if w.scriptFiles != nil {
		child.scriptFiles = w.scriptFiles
	}

	return child
}

// res returns the walker's resolver, or one that knows nothing.
func (w *astWalker) res() Resolver {
	if w.resolver == nil {
		return nopResolver{}
	}

	return w.resolver
}

// expandName substitutes the variables in a command word: first those
// assigned earlier on the line, then the environment.
func (w *astWalker) expandName(word string) string {
	word = expandVars(word, w.assignments)
	if !HasUnresolvedVars(word) {
		return word
	}

	return varRefPattern.ReplaceAllStringFunc(word, func(ref string) string {
		if value, ok := w.res().LookupEnv(ref[2 : len(ref)-1]); ok {
			return value
		}

		return ref
	})
}

// commandWord renders a command word, resolving a command substitution to
// the program it prints, as in $(which git) or "$(command -v git)".
func commandWord(word *syntax.Word) string {
	var b strings.Builder

	for _, part := range word.Parts {
		b.WriteString(commandWordPart(part))
	}

	return b.String()
}

// commandWordPart renders one part of a command word.
func commandWordPart(part syntax.WordPart) string {
	switch p := part.(type) {
	case *syntax.CmdSubst:
		return substitutedProgram(p)
	case *syntax.DblQuoted:
		var b strings.Builder

		for _, inner := range p.Parts {
			b.WriteString(commandWordPart(inner))
		}

		return b.String()
	default:
		return wordToString(&syntax.Word{Parts: []syntax.WordPart{part}})
	}
}

// substitutedProgram returns the program a command substitution prints.
func substitutedProgram(sub *syntax.CmdSubst) string {
	if len(sub.Stmts) != 1 {
		return unresolvedProgram
	}

	call := callExprOf(sub.Stmts[0])
	if call == nil || len(call.Args) < lookupWords {
		return unresolvedProgram
	}

	args := wordsToStrings(call.Args)
	if len(args) < lookupWords {
		return unresolvedProgram
	}

	name, operand := commandName(args[0]), args[len(args)-1]

	switch {
	case lookupCommands[name] && !strings.HasPrefix(operand, "-"):
		return operand
	case name == gitProgram && slices.Contains(args[1:], "--exec-path"):
		return "/git-core"
	default:
		return unresolvedProgram
	}
}

// resolveProgram names git or gh however they were reached: as a git-<sub>
// binary, through hub, under another file name, or behind a name nothing on
// disk explains. It then expands git aliases, returning the command line of a
// shell alias for the walker to follow.
func (w *astWalker) resolveProgram(cmd Command) (Command, []string) {
	if sub, ok := strings.CutPrefix(cmd.Name, "git-"); ok && sub != "" {
		cmd.Name, cmd.Args = gitProgram, slices.Concat([]string{sub}, cmd.Args)
	}

	if cmd.Name == "hub" {
		cmd.Name = gitProgram
	}

	if cmd.Name != gitProgram && cmd.Name != ghCLI && !shellBuiltins[cmd.Name] &&
		!w.defined(cmd.Invoked) {
		cmd.Name = w.programBehind(cmd)
	}

	if cmd.Name != gitProgram {
		return cmd, nil
	}

	return w.expandGitAlias(cmd)
}

// programBehind returns git or gh for a program invoked with one of their
// validated subcommands when it is that program under another name, or when
// nothing on disk runs it: a shell alias, a function or an unresolved
// variable is checked rather than trusted.
func (w *astWalker) programBehind(cmd Command) string {
	var (
		want Program
		name string
	)

	idx := gitSubcommandIndex(cmd.Args)

	switch {
	case idx >= 0 && validatedGitSubcommands[cmd.Args[idx]]:
		want, name = ProgramGit, gitProgram
	case len(cmd.Args) > 0 && validatedGHCommands[cmd.Args[0]]:
		want, name = ProgramGH, ghCLI
	default:
		return cmd.Name
	}

	// A name still holding a variable or substitution names nothing yet.
	program := ProgramMissing
	if !HasUnresolvedVars(cmd.Invoked) && !strings.Contains(cmd.Invoked, unresolvedProgram) {
		program = w.res().Program(cmd.Invoked, cmd.WorkingDirectory)
	}

	if program == want || program == ProgramMissing {
		return name
	}

	return cmd.Name
}

// expandGitAlias replaces a git alias with what it stands for, from -c
// options on the command or from git config. A shell alias ("!...") is
// returned as a command line to follow.
func (w *astWalker) expandGitAlias(cmd Command) (Command, []string) {
	for range maxAliasDepth {
		idx := gitSubcommandIndex(cmd.Args)
		if idx < 0 || gitBuiltins[cmd.Args[idx]] || !gitAliasName.MatchString(cmd.Args[idx]) {
			return cmd, nil
		}

		value, ok := inlineGitAlias(cmd.Args[:idx], cmd.Args[idx])
		if !ok {
			value, ok = w.res().GitAlias(gitDir(cmd, idx), cmd.Args[idx])
		}

		if !ok {
			return cmd, nil
		}

		rest := cmd.Args[idx+1:]

		if line, shell := strings.CutPrefix(value, "!"); shell {
			return cmd, []string{line + " " + quoteArgs(rest)}
		}

		cmd.Args = slices.Concat(cmd.Args[:idx], strings.Fields(value), rest)
	}

	return cmd, nil
}

// inlineGitAlias returns an alias set with -c alias.<name>=value, the last
// setting winning as it does in git.
func inlineGitAlias(globals []string, name string) (string, bool) {
	value, found := "", false

	for i, arg := range globals {
		var setting string

		switch {
		case arg == flagLowerC && i+1 < len(globals):
			setting = globals[i+1]
		case strings.HasPrefix(arg, flagLowerC) && len(arg) > len(flagLowerC):
			setting = arg[len(flagLowerC):]
		default:
			continue
		}

		key, v, ok := strings.Cut(setting, "=")
		if ok && strings.EqualFold(key, "alias."+name) {
			value, found = v, true
		}
	}

	return value, found
}

// gitDir returns the directory a git command runs in, after any -C options.
func gitDir(cmd Command, idx int) string {
	dir := cmd.WorkingDirectory
	globals := cmd.Args[:idx]

	for i := 0; i+1 < len(globals); i++ {
		if globals[i] != flagUpperC {
			continue
		}

		next := globals[i+1]
		if filepath.IsAbs(next) || dir == "" {
			dir = next
		} else {
			dir = filepath.Join(dir, next)
		}

		i++
	}

	return dir
}

// defined reports whether a name is an alias or function from this line.
func (w *astWalker) defined(name string) bool {
	_, alias := w.aliases[name]
	_, fn := w.funcs[name]

	return alias || fn
}

// defineAliases records the aliases an alias command defines.
func (w *astWalker) defineAliases(cmd Command) {
	if cmd.Name != "alias" {
		return
	}

	for _, arg := range cmd.Args {
		if name, value, ok := strings.Cut(arg, "="); ok && name != "" {
			w.aliases[name] = value
		}
	}
}

// defineFunc records a function body so a later call can be followed.
func (w *astWalker) defineFunc(fn *syntax.FuncDecl) {
	if fn.Name == nil || fn.Body == nil {
		return
	}

	var body strings.Builder

	if err := syntax.NewPrinter().Print(&body, fn.Body); err != nil {
		return
	}

	w.funcs[fn.Name.Value] = body.String()
}

// definitionScripts returns what calling a same-line alias or function runs.
func (w *astWalker) definitionScripts(cmd Command) []string {
	var scripts []string

	if value, ok := w.aliases[cmd.Invoked]; ok {
		scripts = append(scripts, value+" "+quoteArgs(cmd.Args))
	}

	if body, ok := w.funcs[cmd.Invoked]; ok {
		scripts = append(scripts, substitutePositional(body, cmd.Args))
	}

	return scripts
}

// substitutePositional puts a call's arguments in place of the positional
// parameters a function body uses.
func substitutePositional(body string, args []string) string {
	return positionalParam.ReplaceAllStringFunc(body, func(ref string) string {
		match := positionalParam.FindStringSubmatch(ref)
		param := match[1] + match[2]

		if param == "@" || param == "*" {
			return quoteArgs(args)
		}

		if n := int(param[0] - '0'); n <= len(args) {
			return shellQuote(args[n-1])
		}

		return "''"
	})
}

// follow records everything a command launches.
func (w *astWalker) follow(cmd Command, l launch, depth int) {
	for _, launchedCmd := range l.commands {
		w.recordCommand(launchedCmd, depth)
	}

	for _, script := range l.scripts {
		w.walkScript(script, cmd, depth)
	}

	for _, file := range l.files {
		w.followFile(cmd, file, depth)
	}

	for _, code := range l.code {
		w.followCode(cmd, code, depth)
	}
}

// followFile records the commands of a script file a command runs.
func (w *astWalker) followFile(cmd Command, file scriptFile, depth int) {
	text, ok := w.scriptSource(file.path, cmd)
	if !ok {
		return
	}

	if file.interpreter || interpreterShebang(text) {
		w.followCode(cmd, text, depth)

		return
	}

	w.walkScript(text, cmd, depth)
}

// followCode records the command lines found in program source.
func (w *astWalker) followCode(cmd Command, code string, depth int) {
	for _, line := range commandLines(code) {
		w.walkScript(line, cmd, depth)
	}
}

// scriptSource returns the text of a script a command runs: stdin, a process
// substitution, a file written earlier on the same line, or the file on disk.
func (w *astWalker) scriptSource(path string, cmd Command) (string, bool) {
	if path == "-" || path == "/dev/stdin" {
		return cmd.Stdin, cmd.Stdin != ""
	}

	if text, ok := w.scriptFiles[path]; ok {
		return text, true
	}

	for _, fw := range slices.Backward(w.fileWrites) {
		if filepath.Clean(fw.Path) == filepath.Clean(path) {
			return writtenContent(fw)
		}
	}

	if !filepath.IsAbs(xdg.ExpandPathSilent(path)) && cmd.WorkingDirectory != "" {
		path = filepath.Join(cmd.WorkingDirectory, path)
	}

	return w.res().ReadScript(path)
}

// writtenContent returns the bytes a same-line write puts in a file, when
// they are known exactly.
func writtenContent(fw FileWrite) (string, bool) {
	switch {
	case fw.ContentCaptured:
		return fw.Content, true
	case fw.RedirectContentCaptured:
		return fw.RedirectContent, true
	default:
		return "", false
	}
}

// walkScript records the commands of a script that parent runs. A cd inside
// the script moves only the script, and its definitions stay inside it.
func (w *astWalker) walkScript(script string, parent Command, depth int) {
	file, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return
	}

	child := w.child(parent.WorkingDirectory, depth)

	syntax.Walk(file, child.visit)

	for _, cmd := range child.commands {
		cmd.Location = parent.Location
		w.commands = append(w.commands, cmd)
	}

	w.fileWrites = append(w.fileWrites, child.fileWrites...)
}

// argStrings converts argument words to strings. A process substitution fed
// by literal output becomes a stand-in path whose content is remembered, so
// bash <(echo "...") can be followed like any script file.
func (w *astWalker) argStrings(words []*syntax.Word) []string {
	args := make([]string, 0, len(words))

	for _, word := range words {
		if text, ok := procSubstOutput(word); ok && w.scriptFiles != nil {
			path := fmt.Sprintf("%s%d", procSubstPrefix, len(w.scriptFiles))
			w.scriptFiles[path] = text
			args = append(args, path)

			continue
		}

		if s := wordToString(word); s != "" {
			args = append(args, s)
		}
	}

	return args
}

// procSubstOutput returns what an input process substitution produces when
// it is a literal echo, printf or cat heredoc.
func procSubstOutput(word *syntax.Word) (string, bool) {
	if len(word.Parts) != 1 {
		return "", false
	}

	sub, ok := word.Parts[0].(*syntax.ProcSubst)
	if !ok || sub.Op != syntax.CmdIn || len(sub.Stmts) != 1 {
		return "", false
	}

	stmt := sub.Stmts[0]

	if info := collectRedirs(stmt); info.hasHeredoc && copiesStdinVerbatim(callExprOf(stmt)) {
		return info.heredocContent, true
	}

	return literalCommandOutput(callExprOf(stmt))
}
