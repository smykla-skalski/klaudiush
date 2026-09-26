package parser

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

const (
	// maxAliasDepth bounds how many git aliases are expanded in a chain.
	maxAliasDepth = 5
	// maxTypoDistance is how far a mistyped subcommand may be from the one
	// git's autocorrect runs.
	maxTypoDistance = 2
	// maxBraceWords caps how many words a brace expansion in a program word
	// yields.
	maxBraceWords = 64
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
	// unsupportedPositional matches positional forms substitutePositional
	// does not handle: slices, defaults, two-digit indexes and shift.
	unsupportedPositional = regexp.MustCompile(
		`\$\{(?:[@*][^}]|[0-9]+[^0-9}]|[0-9]{2,}\})|\bshift\b`,
	)
	// configParameter matches one 'key'='value' pair in GIT_CONFIG_PARAMETERS.
	configParameter = regexp.MustCompile(`'([^'=]+)'?=?'([^']*)'`)
	// lookupCommands print the program named by their last operand.
	lookupCommands = nameSet("command echo printf readlink realpath type which whereis")
)

// newAstWalker returns a walker ready to record commands.
func newAstWalker(resolver Resolver) *astWalker {
	return &astWalker{
		commands:        make([]Command, 0),
		fileWrites:      make([]FileWrite, 0),
		stdinByCall:     make(map[*syntax.CallExpr]string),
		stdinFileByCall: make(map[*syntax.CallExpr]string),
		assignments:     make(map[string]string),
		resolver:        resolver,
		aliases:         make(map[string]string),
		funcs:           make(map[string]string),
		scriptFiles:     make(map[string]string),
		state:           &parseState{work: maxParseWork},
		expanding:       make(map[string]bool),
	}
}

// child returns a walker for a script run by a command at depth. It sees the
// variables, aliases and functions defined so far without leaking its own,
// and shares the parse's work budget and outcome.
func (w *astWalker) child(dir string, depth int) *astWalker {
	child := newAstWalker(w.resolver)
	child.currentDir = dir
	child.dirUnknown = w.dirUnknown
	child.depth = depth
	child.scriptFiles = w.scriptFiles
	child.state = w.state

	maps.Copy(child.assignments, w.assignments)
	maps.Copy(child.aliases, w.aliases)
	maps.Copy(child.funcs, w.funcs)
	maps.Copy(child.expanding, w.expanding)

	return child
}

// expandName substitutes the variables in a command word: first those
// assigned earlier on the line, then the environment.
func (w *astWalker) expandName(word string) string {
	return expandVars(word, func(name string) (string, bool) {
		if value, ok := w.assignments[name]; ok {
			return value, true
		}

		return w.resolver.LookupEnv(name)
	})
}

// commandWord renders a command word, resolving a command substitution to
// the program it prints, as in $(which git) or "$(command -v git)".
func commandWord(word *syntax.Word) string {
	return commandWordParts(word.Parts)
}

// commandWordParts renders the parts of a command word, looking inside
// double quotes for substitutions too.
func commandWordParts(parts []syntax.WordPart) string {
	var b strings.Builder

	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.CmdSubst:
			b.WriteString(substitutedProgram(p))
		case *syntax.DblQuoted:
			b.WriteString(commandWordParts(p.Parts))
		default:
			b.WriteString(argWord(&syntax.Word{Parts: []syntax.WordPart{part}}))
		}
	}

	return b.String()
}

// braceWords returns the words a brace expansion in word produces, or nil
// when it has none. It works on a copy: splitting braces rewrites the word,
// and the walker still has to visit the original.
func braceWords(word *syntax.Word) []string {
	if !strings.Contains(wordToString(word), "{") {
		return nil
	}

	clone := &syntax.Word{Parts: slices.Clone(word.Parts)}
	if !syntax.SplitBraces(clone) {
		return nil
	}

	// Only the program and its leading arguments matter, so a huge sequence
	// such as {1..99999999} stops early instead of allocating it all.
	var words []string

	for expanded, err := range expand.BracesSeq(nil, clone) {
		if err != nil || len(words) == maxBraceWords {
			break
		}

		words = append(words, argWord(expanded))
	}

	return words
}

// substitutedProgram returns the program a command substitution prints.
func substitutedProgram(sub *syntax.CmdSubst) string {
	if len(sub.Stmts) != 1 {
		return unresolvedProgram
	}

	call := callExprOf(sub.Stmts[0])
	if call == nil {
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
func (w *astWalker) resolveProgram(cmd Command) (Command, []nestedScript) {
	if sub, ok := strings.CutPrefix(cmd.Name, "git-"); ok && sub != "" {
		cmd.Name, cmd.Args = gitProgram, slices.Concat([]string{sub}, cmd.Args)
	}

	if cmd.Name == hubCLI {
		cmd.Name = gitProgram
	}

	if cmd.Name != gitProgram && cmd.Name != ghCLI && !shellBuiltins[cmd.Name] &&
		!w.defined(cmd.Invoked) {
		cmd.Name = w.programBehind(cmd)
	}

	switch cmd.Name {
	case gitProgram:
		return w.expandGitAlias(cmd)
	case ghCLI:
		return w.expandGHAlias(ghCommandFirst(cmd))
	default:
		return cmd, nil
	}
}

// ghCommandFirst moves gh options given before the command (gh -R o/r pr
// create) after it, so every check finds the command in the same place.
func ghCommandFirst(cmd Command) Command {
	i := 0

	for i < len(cmd.Args) && strings.HasPrefix(cmd.Args[i], "-") {
		if ghGlobalValueFlags[cmd.Args[i]] {
			i++
		}

		i++
	}

	if i == 0 || i >= len(cmd.Args) {
		return cmd
	}

	end := i + 1
	if end < len(cmd.Args) && !strings.HasPrefix(cmd.Args[end], "-") {
		end++ // the action, as in "pr create"
	}

	cmd.Args = slices.Concat(cmd.Args[i:end], cmd.Args[:i], cmd.Args[end:])

	return cmd
}

// expandGHAlias replaces a gh alias with what it stands for, from gh alias
// set earlier on the line or from gh's configuration. A shell alias ("!...")
// is returned as a command line to follow.
func (w *astWalker) expandGHAlias(cmd Command) (Command, []nestedScript) {
	for range maxAliasDepth {
		if len(cmd.Args) == 0 || strings.HasPrefix(cmd.Args[0], "-") || ghBuiltins[cmd.Args[0]] {
			return cmd, nil
		}

		name, rest := cmd.Args[0], cmd.Args[1:]
		if w.expanding["gh:"+name] {
			return cmd, nil
		}

		value, ok := w.lineGHAlias(name)
		if !ok {
			value, ok = w.resolver.GHAlias(name)
		}

		if !ok {
			return cmd, nil
		}

		if line, shell := strings.CutPrefix(value, "!"); shell {
			return cmd, []nestedScript{{name: "gh:" + name, text: line + " " + quoteArgs(rest)}}
		}

		cmd.Args = slices.Concat(strings.Fields(value), rest)
		cmd = ghCommandFirst(cmd)
	}

	return cmd, nil
}

// lineGHAlias returns an alias set with gh alias set earlier on the line.
// A --shell alias comes back with gh's own "!" prefix.
func (w *astWalker) lineGHAlias(name string) (string, bool) {
	for _, cmd := range slices.Backward(w.commands) {
		if cmd.Name != ghCLI || len(cmd.Args) < 2 || cmd.Args[0] != "alias" ||
			cmd.Args[1] != "set" {
			continue
		}

		shell := false

		var operands []string

		for _, arg := range cmd.Args[2:] {
			switch {
			case arg == "--shell" || arg == "-s":
				shell = true
			case strings.HasPrefix(arg, "-"):
			default:
				operands = append(operands, arg)
			}
		}

		if len(operands) < 2 || operands[0] != name {
			continue
		}

		if shell && !strings.HasPrefix(operands[1], "!") {
			return "!" + operands[1], true
		}

		return operands[1], true
	}

	return "", false
}

// nestedScript is a command line run through a definition: a same-line
// alias or function, or a git or gh shell alias. The name keeps the
// definition from being expanded inside itself.
type nestedScript struct {
	name string
	text string
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
	// A PATH or hash change on the line can point a bare name anywhere.
	bareAfterPathChange := w.state.pathChanged && !strings.Contains(cmd.Invoked, "/")

	program := ProgramMissing
	if !HasUnresolvedVars(cmd.Invoked) && !strings.Contains(cmd.Invoked, unresolvedProgram) &&
		!bareAfterPathChange {
		program = w.resolver.Program(cmd.Invoked, cmd.WorkingDirectory)
	}

	if program == want || program == ProgramMissing {
		return name
	}

	return cmd.Name
}

// expandGitAlias replaces a git alias with what it stands for, from -c
// options on the command or from git config. A shell alias ("!...") is
// returned as a command line to follow.
func (w *astWalker) expandGitAlias(cmd Command) (Command, []nestedScript) {
	for range maxAliasDepth {
		idx := gitSubcommandIndex(cmd.Args)
		if idx < 0 || gitBuiltins[cmd.Args[idx]] || !gitAliasName.MatchString(cmd.Args[idx]) {
			return cmd, nil
		}

		name, rest := cmd.Args[idx], cmd.Args[idx+1:]

		// An alias that runs itself was already followed once.
		if w.expanding["git:"+name] {
			return cmd, nil
		}

		value, ok := inlineGitAlias(cmd.Args[:idx], name)
		if !ok {
			value, ok = w.lineGitAlias(name)
		}

		if !ok {
			value, ok = w.envGitAlias(cmd.Args[:idx], name)
		}

		if !ok {
			value, ok = w.resolver.GitAlias(gitDir(cmd, idx), name)
		}

		if !ok {
			return w.unknownGitCommand(cmd, idx), nil
		}

		if line, shell := strings.CutPrefix(value, "!"); shell {
			return cmd, []nestedScript{{name: "git:" + name, text: line + " " + quoteArgs(rest)}}
		}

		cmd.Args = slices.Concat(cmd.Args[:idx], strings.Fields(value), rest)
	}

	return cmd, nil
}

// gitConfigVars move where git reads its configuration from, so an alias
// lookup made outside the command no longer sees what git will.
var gitConfigVars = strings.Fields(`HOME XDG_CONFIG_HOME GIT_DIR GIT_CONFIG GIT_CONFIG_GLOBAL
	GIT_CONFIG_SYSTEM GIT_CONFIG_NOSYSTEM`)

// unknownGitCommand handles a git subcommand that is neither a builtin nor
// a known alias. A typo git would autocorrect becomes the command it runs.
// When the command points git at other configuration, the alias could be
// defined there, so the parse fails closed.
func (w *astWalker) unknownGitCommand(cmd Command, idx int) Command {
	name, globals := cmd.Args[idx], cmd.Args[:idx]

	if corrected, found := w.autocorrect(name); found {
		cmd.Args = slices.Concat(globals, []string{corrected}, cmd.Args[idx+1:])

		return cmd
	}

	movesConfig := slices.ContainsFunc(gitConfigVars, func(name string) bool {
		_, set := w.assignments[name]

		return set
	}) || slices.ContainsFunc(globals, func(arg string) bool {
		return arg == "--git-dir" || strings.HasPrefix(arg, "--git-dir=")
	})

	if movesConfig && w.resolver.Program("git-"+name, "") == ProgramMissing {
		w.state.truncated = true
	}

	return cmd
}

// lineGitAlias returns an alias set with git config earlier on the line,
// which is in place by the time the later command runs.
func (w *astWalker) lineGitAlias(name string) (string, bool) {
	for _, cmd := range slices.Backward(w.commands) {
		if cmd.Name != gitProgram {
			continue
		}

		gitCmd, err := ParseGitCommand(cmd)
		if err != nil || gitCmd.Subcommand != "config" {
			continue
		}

		args := gitCmd.Args
		if len(args) > 0 && args[0] == "set" {
			args = args[1:] // git config set <key> <value>
		}

		if len(args) >= 2 && strings.EqualFold(args[0], "alias."+name) {
			return args[1], true
		}
	}

	return "", false
}

// envGitAlias returns an alias set through the environment: GIT_CONFIG_COUNT
// with GIT_CONFIG_KEY_n and GIT_CONFIG_VALUE_n, GIT_CONFIG_PARAMETERS, or a
// --config-env option naming a variable.
func (w *astWalker) envGitAlias(globals []string, name string) (string, bool) {
	key := "alias." + name

	if count, err := strconv.Atoi(w.lookupVar("GIT_CONFIG_COUNT")); err == nil {
		for i := range count {
			if strings.EqualFold(w.lookupVar(fmt.Sprintf("GIT_CONFIG_KEY_%d", i)), key) {
				return w.lookupVar(fmt.Sprintf("GIT_CONFIG_VALUE_%d", i)), true
			}
		}
	}

	for _, m := range configParameter.FindAllStringSubmatch(w.lookupVar("GIT_CONFIG_PARAMETERS"), -1) {
		if strings.EqualFold(m[1], key) {
			return m[2], true
		}
	}

	for i, arg := range globals {
		setting, found := strings.CutPrefix(arg, "--config-env=")
		if !found && arg == "--config-env" && i+1 < len(globals) {
			setting, found = globals[i+1], true
		}

		if k, variable, ok := strings.Cut(setting, "="); found && ok && strings.EqualFold(k, key) {
			return w.lookupVar(variable), true
		}
	}

	return "", false
}

// lookupVar returns a variable assigned on the line, or else from the
// environment.
func (w *astWalker) lookupVar(name string) string {
	if value, ok := w.assignments[name]; ok {
		return value
	}

	value, _ := w.resolver.LookupEnv(name)

	return value
}

// autocorrect returns the validated subcommand git's help.autocorrect would
// run for a mistyped name: the one builtin closest to it, within git's
// distance, when no git-<name> command exists.
func (w *astWalker) autocorrect(name string) (string, bool) {
	if w.resolver.Program("git-"+name, "") != ProgramMissing {
		return "", false
	}

	best, bestDistance, unique := "", maxTypoDistance+1, false

	for builtin := range gitBuiltins {
		switch distance := editDistance(name, builtin); {
		case distance < bestDistance:
			best, bestDistance, unique = builtin, distance, true
		case distance == bestDistance:
			unique = false
		}
	}

	if !unique || !validatedGitSubcommands[best] {
		return "", false
	}

	return best, true
}

// editDistance counts the insertions, deletions, substitutions and adjacent
// swaps that turn a into b.
func editDistance(a, b string) int {
	prev2, prev, curr := make([]int, len(b)+1), make([]int, len(b)+1), make([]int, len(b)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)

			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				curr[j] = min(curr[j], prev2[j-2]+1)
			}
		}

		prev2, prev, curr = prev, curr, prev2
	}

	return prev[len(b)]
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
func (w *astWalker) definitionScripts(cmd Command) []nestedScript {
	// A definition that calls itself was already followed once.
	if w.expanding[cmd.Invoked] {
		return nil
	}

	var scripts []nestedScript

	if value, ok := w.aliases[cmd.Invoked]; ok {
		scripts = append(
			scripts,
			nestedScript{name: cmd.Invoked, text: value + " " + quoteArgs(cmd.Args)},
		)
	}

	if body, ok := w.funcs[cmd.Invoked]; ok {
		// Positional forms that are not substituted leave the call unknown.
		if unsupportedPositional.MatchString(body) {
			w.state.truncated = true

			return scripts
		}

		scripts = append(
			scripts,
			nestedScript{name: cmd.Invoked, text: substitutePositional(body, cmd.Args)},
		)
	}

	return scripts
}

// substitutePositional puts a call's arguments in place of the positional
// parameters a function body uses.
func substitutePositional(body string, args []string) string {
	return positionalParam.ReplaceAllStringFunc(body, func(ref string) string {
		param := strings.Trim(ref, `"${}`)

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
		w.walkScript(script, cmd, depth, scriptWalk{})
	}

	for _, file := range l.files {
		w.followFile(cmd, file, depth)
	}

	for _, code := range l.code {
		w.followCode(cmd, code, depth)
	}
}

// followFile records the commands of a script file a command runs. A file
// handed to a shell that cannot be read (too large, written on the line but
// not captured, under an unknown directory) fails closed; a compiled program
// is an accepted limit.
func (w *astWalker) followFile(cmd Command, file scriptFile, depth int) {
	text, status := w.scriptSource(file.path, cmd)

	switch status {
	case ScriptText:
		if file.interpreter || interpreterShebang(text) {
			w.followCode(cmd, text, depth)
		} else {
			w.walkScript(text, cmd, depth, scriptWalk{})
		}
	case ScriptOpaque:
		if file.explicit {
			w.state.truncated = true
		}
	case ScriptMissing, ScriptBinary:
	}
}

// followCode records the command lines found in program source.
func (w *astWalker) followCode(cmd Command, code string, depth int) {
	for _, line := range commandLines(code) {
		w.walkScript(line, cmd, depth, scriptWalk{literal: true})
	}
}

// scriptSource returns the text of a script a command runs: stdin, a process
// substitution, a file written earlier on the same line, or the file on disk.
func (w *astWalker) scriptSource(path string, cmd Command) (string, ScriptStatus) {
	if path == "-" || path == "/dev/stdin" {
		if cmd.Stdin == "" {
			return "", ScriptMissing
		}

		return cmd.Stdin, ScriptText
	}

	if text, ok := w.scriptFiles[path]; ok {
		return text, ScriptText
	}

	path = w.expandName(path)

	relative := !filepath.IsAbs(path) && !strings.HasPrefix(path, "~")
	if HasUnresolvedVars(path) || (relative && w.dirUnknown) {
		return "", ScriptOpaque
	}

	target := resolvePath(cmd.WorkingDirectory, path)
	if text, found, captured := lastWrite(w.fileWrites, target, nil); found {
		if !captured {
			return "", ScriptOpaque
		}

		return text, ScriptText
	}

	return w.resolver.ReadScript(target)
}

// scriptWalk says how walkScript treats a script.
type scriptWalk struct {
	// name is the definition being expanded, kept from expanding in itself.
	name string
	// literal marks a string from interpreter code, where prose is expected.
	literal bool
}

// walkScript records the commands of a script that parent runs. A cd inside
// the script moves only the script, and its definitions stay inside it. A
// shell runs the commands before a syntax error and what follows is unknown,
// so a script that fails to parse fails closed; interpreter strings, which
// are mostly prose, do not.
func (w *astWalker) walkScript(script string, parent Command, depth int, sw scriptWalk) {
	if !w.state.spend() {
		w.state.truncated = true

		return
	}

	child := w.child(parent.WorkingDirectory, depth)
	child.literal = sw.literal

	if sw.name != "" {
		child.expanding[sw.name] = true
	}

	for stmt, err := range syntax.NewParser().StmtsSeq(strings.NewReader(script)) {
		if err != nil {
			w.state.truncated = w.state.truncated || !sw.literal

			break
		}

		syntax.Walk(stmt, child.visit)
	}

	// Commands report the line of the command that ran the script, keeping
	// their own place in execution order.
	for _, cmd := range child.commands {
		cmd.Location.Line, cmd.Location.Column = parent.Location.Line, parent.Location.Column
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
		if text, ok := procSubstOutput(word); ok {
			path := fmt.Sprintf("%s%d", procSubstPrefix, len(w.scriptFiles))
			w.scriptFiles[path] = text
			args = append(args, path)

			continue
		}

		if s := argWord(word); s != "" {
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
