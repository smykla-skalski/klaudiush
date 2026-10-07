package parser

import (
	"fmt"
	"iter"
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
	procSubstPrefix  = "/dev/fd/klaudiush-"
	substitutedInput = procSubstPrefix + "substituted"
	devStdin         = "/dev/stdin"
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
	// lookupCommands print the path of the one program they name. echo,
	// printf and command without -v print any text, so their output is
	// unknown.
	lookupCommands = nameSet("readlink realpath which")
)

// newAstWalker returns a walker ready to record commands.
func newAstWalker(resolver Resolver) *astWalker {
	return &astWalker{
		commands:        make([]Command, 0),
		fileWrites:      make([]FileWrite, 0),
		stdinByCall:     make(map[*syntax.CallExpr]string),
		stdinTextByCall: make(map[*syntax.CallExpr]*ShellText),
		stdinFileByCall: make(map[*syntax.CallExpr]string),
		assignments:     make(map[string]string),
		unknownVars:     make(map[string]bool),
		safeAssigns:     make(map[*syntax.Assign]bool),
		safeNamerefs:    make(map[*syntax.Assign]bool),
		chainAssigns:    make(map[*syntax.Assign]bool),
		namerefs:        make(map[string]string),
		certain:         make(map[*syntax.Stmt]certainty),
		loopCalls:       make(map[*syntax.CallExpr]bool),
		resolver:        resolver,
		aliases:         make(map[string]string),
		funcs:           make(map[string]string),
		scriptFiles:     make(map[string]string),
		startupUnset:    make(map[string]bool),
		state: &parseState{
			work:        maxParseWork,
			lenientWork: maxParseWork,
			distinct:    make(map[string]bool),
			repeated:    make(map[string]bool),
		},
		expanding: make(map[string]bool),
	}
}

// child returns a walker for a script run by a command at depth. It sees the
// variables, aliases and functions defined so far without leaking its own,
// and shares the parse's work budget and outcome.
func (w *astWalker) child(dir string, depth int, inheritNamerefs bool) *astWalker {
	child := newAstWalker(w.resolver)
	child.currentDir = dir
	child.dirUnknown = w.dirUnknown
	child.dirComputed = w.dirComputed
	child.lenient = w.lenient
	child.depth = depth
	child.scriptFiles = w.scriptFiles
	child.state = w.state
	child.parent = w
	child.via = slices.Clone(w.via)

	maps.Copy(child.assignments, w.assignments)
	maps.Copy(child.unknownVars, w.unknownVars)

	if inheritNamerefs {
		maps.Copy(child.namerefs, w.namerefs)
	}

	child.outerLoop = w.inLoop
	child.loopStartup = maps.Clone(w.loopStartup)
	child.startupUnset = maps.Clone(w.startupUnset)
	child.startupDeferred = maps.Clone(w.startupDeferred)
	maps.Copy(child.aliases, w.aliases)
	maps.Copy(child.funcs, w.funcs)
	maps.Copy(child.expanding, w.expanding)

	return child
}

// earlierCommands yields the commands recorded so far on the line, latest
// first, including those of the scripts that run this one.
func (w *astWalker) earlierCommands() iter.Seq[Command] {
	return func(yield func(Command) bool) {
		for p := w; p != nil; p = p.parent {
			for _, cmd := range slices.Backward(p.commands) {
				if !yield(cmd) {
					return
				}
			}
		}
	}
}

// lastLineWrite is lastWrite over every write recorded so far on the line,
// including those of the scripts that run this one.
func (w *astWalker) lastLineWrite(target string) (content string, found, captured bool) {
	for p := w; p != nil; p = p.parent {
		writes := make([]FileWrite, 0, len(p.fileWrites))

		for _, fw := range p.fileWrites {
			path, known := w.writtenPath(fw)
			if !known {
				continue
			}

			fw.Path, fw.WorkingDirectory = path, ""
			writes = append(writes, fw)
		}

		if content, found, captured = lastWrite(writes, target, nil); found {
			return content, found, captured
		}
	}

	return "", false, false
}

// mixedLineWrite reports an earlier write that may name target through a mix
// of absolute and relative spellings when the command's starting directory is
// unavailable.
func (w *astWalker) mixedLineWrite(target string) bool {
	if _, known := w.startPWD(); known {
		return false
	}

	for p := w; p != nil; p = p.parent {
		for _, fw := range p.fileWrites {
			path, known := w.writtenPath(fw)
			if !known || filepath.IsAbs(path) == filepath.IsAbs(target) {
				continue
			}

			absolute, relative := path, target
			if !filepath.IsAbs(absolute) {
				absolute, relative = relative, absolute
			}

			if strings.HasSuffix(absolute, string(filepath.Separator)+filepath.Clean(relative)) {
				return true
			}
		}
	}

	return false
}

// expandName substitutes the variables in a command word: first those
// assigned earlier on the line, then the environment.
func (w *astWalker) expandName(word string) string {
	return expandVars(word, func(name string) (string, bool) {
		if w.unknownVars[name] {
			return "", false
		}

		if value, ok := w.assignments[name]; ok {
			return value, true
		}

		return w.resolver.LookupEnv(name)
	})
}

// commandWord renders a command word, resolving a command substitution to
// the program it prints, as in $(which git) or "$(command -v git)".
func (w *astWalker) commandWord(word *syntax.Word) string {
	return w.commandWordParts(word.Parts)
}

// commandWordParts renders the parts of a command word, looking inside
// double quotes for substitutions too.
func (w *astWalker) commandWordParts(parts []syntax.WordPart) string {
	var b strings.Builder

	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.CmdSubst:
			b.WriteString(w.substitutedProgram(p))
		case *syntax.ParamExp:
			b.WriteString(w.scriptDirParam(p))
		case *syntax.ExtGlob, *syntax.ProcSubst, *syntax.ArithmExp:
			b.WriteString(unresolvedProgram)
		case *syntax.DblQuoted:
			b.WriteString(w.commandWordParts(p.Parts))
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

// lookupProgram returns the program a lookup command substitution prints.
func lookupProgram(sub *syntax.CmdSubst) string {
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

	name := commandName(args[0])
	if name == gitProgram && slices.Contains(args[1:], "--exec-path") {
		return "/git-core"
	}

	flags, operands := splitLookup(args[1:])
	if len(operands) != 1 || !lookupFlags(name, flags) {
		return unresolvedProgram
	}

	return operands[0]
}

// splitLookup separates a lookup's flags from its operands.
func splitLookup(args []string) (flags, operands []string) {
	for i, arg := range args {
		if arg == endOfOptions {
			return flags, append(operands, args[i+1:]...)
		}

		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
		} else {
			operands = append(operands, arg)
		}
	}

	return flags, operands
}

// lookupFlags reports whether name with flags prints only the path of the
// program it names: command and type print other text without -v or -p.
func lookupFlags(name string, flags []string) bool {
	switch name {
	case "command":
		return slices.Contains(flags, "-v")
	case "type":
		return slices.ContainsFunc(flags, func(f string) bool {
			return f == "-p" || f == "-P"
		})
	default:
		return lookupCommands[name]
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
		resolved, ok := w.resolveGitSubcommand(cmd)
		if !ok {
			return resolved, nil
		}

		return w.expandGitAlias(resolved)
	case ghCLI:
		resolved, ok := w.resolveGHCommand(ghCommandFirst(cmd))
		if !ok {
			return resolved, nil
		}

		expanded, nested := w.expandGHAlias(resolved)
		if len(nested) == 0 {
			expanded, _ = w.resolveGHCommand(expanded)
		}

		return expanded, nested
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
			return cmd, []nestedScript{
				{
					name:    "gh:" + name,
					text:    line + " " + quoteArgs(rest),
					forward: w.forwardQuoted(cmd, len(cmd.Args)-len(rest)),
				},
			}
		}

		cmd.Args = slices.Concat(strings.Fields(value), rest)
		cmd = ghCommandFirst(cmd)
	}

	return cmd, nil
}

// lineGHAlias returns an alias set with gh alias set earlier on the line.
// A --shell alias comes back with gh's own "!" prefix.
func (w *astWalker) lineGHAlias(name string) (string, bool) {
	for cmd := range w.earlierCommands() {
		if cmd.Name != ghCLI || len(cmd.Args) < 2 || cmd.Args[0] != ghAliasCommand ||
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
	name      string
	text      string
	splitArgs bool
	scoped    bool
	forward   map[string]writtenArg
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
		if idx < 0 || gitBuiltins[cmd.Args[idx]] {
			return cmd, nil
		}

		if !gitAliasName.MatchString(cmd.Args[idx]) {
			return w.unknownGitCommand(cmd, idx), nil
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
			return cmd, []nestedScript{
				{
					name:    "git:" + name,
					text:    line + " " + quoteArgs(rest),
					forward: w.forwardQuoted(cmd, len(cmd.Args)-len(rest)),
				},
			}
		}

		cmd.Args = slices.Concat(cmd.Args[:idx], strings.Fields(value), rest)
	}

	return cmd, nil
}

// proseGit reports whether a git command written directly in a plain string
// of interpreter code names no subcommand git could run ("git executable not
// found"): no builtin, installed command, typo git would correct or alias
// klaudiush can see, with nothing on the line moving git's configuration or
// directory. Such a string is a message; run as a command, git would refuse
// it. Only a plain alias-shaped word qualifies: an option, a brace or glob
// the shell may expand ("{push,}") or any other form stays opaque.
func (w *astWalker) proseGit(cmd Command, depth int) bool {
	if !w.prose || depth != w.depth || cmd.Name != gitProgram || cmd.Invoked != gitProgram ||
		cmd.Dynamic || len(cmd.Args) == 0 || w.dirUnknown || w.dirComputed ||
		w.state.pathChanged || w.lookupEnvChanged() {
		return false
	}

	name := cmd.Args[0]
	if gitBuiltins[name] || !gitAliasName.MatchString(name) {
		return false
	}

	if _, found := w.autocorrect(name); found || w.resolver.GitCommand(name) {
		return false
	}

	if _, ok := w.lineGitAlias(name); ok {
		return false
	}

	if _, ok := w.envGitAlias(nil, name); ok {
		return false
	}

	_, ok := w.resolver.GitAlias(cmd.WorkingDirectory, name)

	return !ok
}

// unknownGitCommand handles a git subcommand that is neither a builtin nor
// a known alias. A typo git would autocorrect becomes the command it runs.
// Anything else either fails in git or runs an alias from configuration
// klaudiush cannot see (written earlier on the line, moved by HOME or
// GIT_DIR, included from another file), so the parse fails closed.
func (w *astWalker) unknownGitCommand(cmd Command, idx int) Command {
	name, globals := cmd.Args[idx], cmd.Args[:idx]

	if corrected, found := w.autocorrect(name); found {
		cmd.Args = slices.Concat(globals, []string{corrected}, cmd.Args[idx+1:])

		return cmd
	}

	if !gitAliasName.MatchString(name) || !w.resolver.GitCommand(name) {
		w.opaque(OpacityUnresolvedProgram, gitProgram+" "+w.shownWord(name), "")
	}

	return cmd
}

// lineGitAlias returns an alias set with git config earlier on the line,
// which is in place by the time the later command runs.
func (w *astWalker) lineGitAlias(name string) (string, bool) {
	for cmd := range w.earlierCommands() {
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
	if w.resolver.GitCommand(name) {
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
			nestedScript{
				name:    cmd.Invoked,
				text:    value + " " + quoteArgs(cmd.Args),
				forward: w.forwardQuoted(cmd, 0),
			},
		)
	}

	if body, ok := w.funcs[cmd.Invoked]; ok {
		// Positional forms that are not substituted leave the call unknown.
		if unsupportedPositional.MatchString(body) {
			w.opaque(OpacityUnresolvedArgs, w.shownWord(cmd.Invoked), "")

			return scripts
		}

		text, split := substitutePositional(body, cmd.Args)
		scripts = append(scripts, nestedScript{
			name:      cmd.Invoked,
			text:      text,
			splitArgs: split,
			scoped:    true,
			forward:   w.forwardPositional(cmd, body),
		})
	}

	return scripts
}

// substitutePositional puts a call's arguments in place of the positional
// parameters a function body uses. An unquoted reference splits its value
// into words and an empty one leaves none, so f() { $1 git push; }; f ""
// runs git push. It also reports whether an unquoted reference split a
// value, which only holds while IFS keeps its default.
func substitutePositional(body string, args []string) (string, bool) {
	split := false

	text := positionalParam.ReplaceAllStringFunc(body, func(ref string) string {
		param := strings.Trim(ref, `"${}`)
		quoted := strings.HasPrefix(ref, `"`) || strings.HasSuffix(ref, `"`)

		var values []string

		switch n := int(param[0] - '0'); {
		case param == "@" || param == "*":
			values = args
		case n <= len(args):
			values = args[n-1 : n]
		case quoted:
			return "''"
		default:
			return ""
		}

		if quoted {
			return quoteArgs(values)
		}

		split = split || slices.ContainsFunc(values, func(v string) bool { return v != "" })

		return quoteArgs(splitFields(values))
	})

	return text, split
}

// splitFields splits values into the words an unquoted expansion gives.
func splitFields(values []string) []string {
	fields := make([]string, 0, len(values))

	for _, value := range values {
		fields = append(fields, strings.Fields(value)...)
	}

	return fields
}

// follow records everything a command launches. A shell, or a script run
// by path, runs its startup files first in the shell that then runs its
// script; any other program may start bash, which reads them on its own.
func (w *astWalker) follow(cmd Command, l launch, depth int, startup []startupScript) {
	var prelude []startupScript
	if shells[cmd.Name] {
		prelude = startup
	}

	for _, launchedCmd := range l.commands {
		w.recordCommand(launchedCmd, depth)
	}

	for _, entrypoint := range l.entrypoints {
		w.record(entrypoint, depth, neutralize(entrypoint.Name))
	}

	for _, script := range l.scripts {
		w.walkScript(script, cmd, depth, scriptWalk{prelude: prelude})
	}

	ranStartup := len(prelude) > 0 && len(l.scripts) > 0

	for _, file := range l.files {
		ranStartup = w.followFile(cmd, file, depth, startup) || ranStartup
	}

	for _, code := range l.code {
		w.followCode(cmd, code, depth, scriptWalk{literal: true})
	}

	if !ranStartup {
		w.walkStartup(cmd, startup, depth)
	}
}

// walkStartup records the commands of the startup files a program's own
// shells read. Each run is walked on its own, since the directory and
// variables it sees may differ; the work budget bounds repetition.
func (w *astWalker) walkStartup(cmd Command, startup []startupScript, depth int) {
	if shells[cmd.Name] && len(startup) > 0 {
		w.walkScript("", cmd, depth, scriptWalk{prelude: startup})

		return
	}

	for _, script := range startup {
		w.walkScript("", cmd, depth, scriptWalk{prelude: []startupScript{script}})
	}
}

// followFile records the commands of a script file a command runs, after the
// startup files the shell running it reads, and reports whether it ran them.
// A file handed to a shell that cannot be read (too large, written on the
// line but not captured, under an unknown directory) fails closed; a
// compiled program is an accepted limit.
func (w *astWalker) followFile(
	cmd Command,
	file scriptFile,
	depth int,
	startup []startupScript,
) bool {
	text, status, detail := w.scriptSource(file.path, cmd)

	switch status {
	case ScriptText:
		literal := file.interpreter || interpreterShebang(text)

		src := scriptSourceText{path: file.path, text: text, literal: literal}
		if !literal {
			src.run = w.fileRun(cmd, file, depth)
			src.prelude = w.shebangStartup(cmd, file, text, startup)
		}

		key := w.sourceKey(cmd, src)
		if w.followingKey(key) {
			w.state.repeated[key] = true

			return false
		}

		w.walkSource(cmd, src, depth, key)

		if w.state.repeated[key] && !w.followingKey(key) {
			w.confirmRepeat(cmd, src, depth, key)
		}

		return len(src.prelude) > 0
	case ScriptOpaque:
		if file.explicit {
			w.opaque(OpacityUnreadableScript, scriptName(file.path), detail)
		}
	case ScriptMissing, ScriptBinary:
	}

	return false
}

// followCode records the command lines found in program source, walking
// each as sw describes. Prose is read only in Python and JavaScript, whose
// print and log calls cannot pipe their own output into a program, and only
// when nothing on the line routes output to another command.
func (w *astWalker) followCode(cmd Command, code string, depth int, sw scriptWalk) {
	proseAllowed := !w.state.outputRouted && proseLanguage(cmd, code)

	for _, line := range commandLines(code) {
		lineWalk := sw
		lineWalk.prose = proseAllowed && line.prose

		w.walkScript(line.text, cmd, depth, lineWalk)
	}
}

// proseLanguage reports whether program source is Python or JavaScript, by
// the interpreter running it or, for a script run by path, its shebang. A
// named interpreter (awk -f x.py) ignores the shebang, and a script's own
// name (./python-tool) says nothing about its language.
func proseLanguage(cmd Command, code string) bool {
	name := commandName(cmd.Name)
	if _, named := interpreters[name]; named {
		return proseInterpreter(name)
	}

	line, _, _ := strings.Cut(code, "\n")
	if !strings.HasPrefix(line, "#!") {
		return false
	}

	return slices.ContainsFunc(
		strings.Fields(strings.TrimPrefix(line, "#!")),
		func(word string) bool {
			return proseInterpreter(commandName(word))
		},
	)
}

// proseInterpreter reports whether an interpreter name runs Python or
// JavaScript.
func proseInterpreter(name string) bool {
	return strings.HasPrefix(name, "python") || name == "node" || name == "nodejs" ||
		name == "deno" || name == "bun"
}

// scriptSource returns the text of a script a command runs: stdin, a process
// substitution, a file written earlier on the same line, or the file on disk.
// For an opaque script it also says why.
func (w *astWalker) scriptSource(path string, cmd Command) (string, ScriptStatus, string) {
	if path == "-" || path == devStdin {
		if cmd.Stdin == "" {
			return "", ScriptMissing, ""
		}

		return cmd.Stdin, ScriptText, ""
	}

	if text, ok := w.scriptFiles[path]; ok {
		return text, ScriptText, ""
	}

	path = w.expandName(path)

	if HasUnresolvedVars(path) {
		return "", ScriptOpaque, DetailScriptVariable
	}

	relative := !filepath.IsAbs(path) && !strings.HasPrefix(path, "~")
	if relative && w.dirUnknown {
		return "", ScriptOpaque, DetailScriptDirectory
	}

	target := w.trackedPath(cmd.WorkingDirectory, path)
	if w.mixedLineWrite(target) {
		return "", ScriptOpaque, DetailScriptDirectory
	}

	if w.unplacedWriteBefore(cmd, target) {
		return "", ScriptOpaque, DetailScriptUnplacedWrite
	}

	if w.lineWriteAbove(target) {
		return "", ScriptOpaque, DetailScriptWritten
	}

	if text, found, captured := w.lastLineWrite(target); found {
		if !captured {
			return "", ScriptOpaque, DetailScriptWritten
		}

		return text, ScriptText, ""
	}

	text, status := w.resolver.ReadScript(target)
	if status == ScriptOpaque {
		return "", ScriptOpaque, DetailScriptRead
	}

	return text, status, ""
}

// scriptWalk says how walkScript treats a script. prelude holds the startup
// files the shell runs before it.
type scriptWalk struct {
	// name is the definition being expanded, kept from expanding in itself.
	name string
	// literal marks a string from interpreter code, where prose is expected.
	literal bool
	// prose marks a plain string literal from interpreter code, where an
	// unknown git word is a message rather than a command.
	prose bool
	// scoped marks a function body, whose local declarations do not escape.
	scoped bool
	// label names the script in diagnostics.
	label string
	// run is the $0 and positional parameters of a script file, set when
	// file is.
	run  scriptRun
	file bool
	// source is the state a script file is followed in, kept from being
	// followed inside itself in the same state.
	source string

	prelude   []startupScript
	forwarded map[string]writtenArg
}

// walkScript records the commands of a script that parent runs. A cd inside
// the script moves only the script. Its definitions stay inside it unless it
// is a same-shell function call. A shell runs the commands before a syntax
// error and what follows is unknown, so a script that fails to parse fails
// closed; interpreter strings, which are mostly prose, do not.
func (w *astWalker) walkScript(script string, parent Command, depth int, sw scriptWalk) {
	if !w.state.spend() {
		w.opaque(OpacityWorkBudget, sw.operation(), "")

		return
	}

	sameShell := runsInShell(parent, sw)

	child := w.child(parent.WorkingDirectory, depth, sameShell)
	if sameShell {
		child.restoreDirectory(w.directoryState())
	}

	child.literal = sw.literal
	child.prose = sw.prose
	child.distrust = w.distrust || !sameShell
	child.scriptRun = w.childRun(parent, sw)
	child.launchSeq = parent.Location.Seq
	child.stdinFed = w.feedsStdin(parent)
	child.seedStartup(parent)
	movedLeniently := child.walkPrelude(sw.prelude, parent)

	if sw.name != "" {
		child.expanding[sw.name] = true
	}

	child.forwarded = forwardedArgs(w.forwarded, sw.forwarded)

	child.following = slices.Clone(w.following)
	if sw.source != "" {
		child.following = append(child.following, sw.source)
	}

	if op := sw.operation(); (sw.name != "" || sw.label != "") &&
		(len(child.via) == 0 || child.via[len(child.via)-1] != op) {
		child.via = append(child.via, op)
	}

	for stmt, err := range syntax.NewParser().StmtsSeq(strings.NewReader(script)) {
		if err != nil {
			if !sw.literal {
				w.opaque(OpacityScriptSyntax, sw.operation(), "")
			}

			break
		}

		child.walkStmt(stmt)
	}

	child.walkEpilogue(sw.prelude, parent, movedLeniently)
	w.publishFunctions(child, parent, sw)
	w.publishNamerefs(child, parent, sw)

	if runsInShell(parent, sw) && parent.Name != trapBuiltin {
		w.inheritDirectory(child, parent.unconditional)
	}

	// Commands report the line of the command that ran the script, keeping
	// their own place in execution order.
	for _, cmd := range child.commands {
		cmd.Location.Line, cmd.Location.Column = parent.Location.Line, parent.Location.Column
		w.commands = append(w.commands, cmd)
	}

	w.fileWrites = append(w.fileWrites, child.fileWrites...)
	w.dynamicWrites += child.dynamicWrites
	w.dynamicWriteLocs = append(w.dynamicWriteLocs, child.dynamicWriteLocs...)
	w.stdinReplaced = w.stdinReplaced || (child.stdinReplaced && runsInShell(parent, sw))
}

func (w *astWalker) publishNamerefs(child *astWalker, parent Command, sw scriptWalk) {
	if !runsInShell(parent, sw) || parent.isolated ||
		maps.Equal(w.namerefs, child.namerefs) {
		return
	}

	if sw.scoped || !parent.unconditional {
		w.distrustNames()

		return
	}

	w.namerefs = maps.Clone(child.namerefs)
}

func (w *astWalker) publishFunctions(child *astWalker, parent Command, sw scriptWalk) {
	if runsInShell(parent, sw) && !parent.isolated {
		maps.Copy(w.funcs, child.funcs)
	}
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

		if s := markSubstituted(word, argWord(word)); s != "" || keepsEmptyWord(word) {
			args = append(args, s)
		}
	}

	return args
}

// procSubstOutput returns what an input process substitution produces when
// it is a literal echo, printf or cat heredoc.
func procSubstOutput(word *syntax.Word) (string, bool) {
	sub := soleProcSubst(word)
	if sub == nil || len(sub.Stmts) != 1 {
		return "", false
	}

	stmt := sub.Stmts[0]

	if info := collectRedirs(stmt); info.hasHeredoc && copiesStdinVerbatim(callExprOf(stmt)) {
		return info.heredocContent, true
	}

	return literalCommandOutput(callExprOf(stmt))
}
