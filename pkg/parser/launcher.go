package parser

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	// maxLaunchDepth bounds how many launchers, scripts and aliases are
	// followed from one command, so pathological nesting cannot recurse
	// without limit. Anything deeper marks the parse truncated, which fails
	// closed.
	maxLaunchDepth = 8
	// maxParseWork caps the commands and scripts one parse follows, so fan-out
	// cannot push the hook past its timeout. Anything past it fails closed.
	maxParseWork = 2000
	// endOfOptions ends a command's options; what follows is an operand.
	endOfOptions = "--"
	// pathVar is the variable that decides what a bare program name runs.
	pathVar = "PATH"
)

// launch lists what a command runs besides itself.
type launch struct {
	commands []Command    // run directly (env git, find -exec git)
	scripts  []string     // shell command lines (bash -c, eval, su -c)
	files    []scriptFile // files run as scripts (bash x.sh, ./x.sh, source x)
	code     []string     // interpreter source scanned for commands (python -c)
}

// empty reports whether the command launches nothing to follow.
func (l launch) empty() bool {
	return len(l.commands) == 0 && len(l.scripts) == 0 && len(l.files) == 0 && len(l.code) == 0
}

// scriptFile is a file a command runs.
type scriptFile struct {
	path        string
	interpreter bool // run by a language interpreter rather than a shell
	// explicit marks a file the command itself names to run. One that cannot
	// be read then fails closed; a script only found on PATH does not.
	explicit bool
}

// launcher describes how a command that runs another command lays out its
// arguments, so the launched command can be found without running anything.
type launcher struct {
	valueFlags  []string // take the next argument as their value
	scriptFlags []string // take a shell command line as their value (su -c)
	stopFlags   []string // make the launcher run nothing (command -v, sudo -l)
	operands    int      // operands before the command (timeout's duration)
	assignments bool     // NAME=value operands before the command (env)
	noCommand   bool     // runs only what a script flag hands it (su)
	joined      bool     // also runs its operands as one command line (watch)
	stdinArgs   bool     // appends or substitutes stdin into the command (xargs)
}

// launchers are the commands that run another command named in their
// arguments. Each would otherwise hide what it runs from every validator.
var launchers = map[string]launcher{
	"builtin":    {},
	"busybox":    {},
	"caffeinate": {valueFlags: strings.Fields("-t -w")},
	"chronic":    {},
	"command":    {stopFlags: strings.Fields("-v -V")},
	"doas":       {valueFlags: strings.Fields("-u -C"), stopFlags: strings.Fields("-L")},
	"env": {
		valueFlags:  strings.Fields("-u -C --unset --chdir"),
		scriptFlags: strings.Fields("-S --split-string"),
		assignments: true,
	},
	"exec": {valueFlags: strings.Fields("-a")},
	"flock": {
		valueFlags:  strings.Fields("-w -E --timeout --conflict-exit-code"),
		scriptFlags: strings.Fields("-c --command"),
		operands:    1,
	},
	"ionice": {
		valueFlags: strings.Fields("-c -n --class --classdata"),
		stopFlags:  strings.Fields("-p -P -u --pid --pgid --uid"),
	},
	"nice":   {valueFlags: strings.Fields("-n --adjustment")},
	"nohup":  {},
	"setsid": {},
	"stdbuf": {valueFlags: strings.Fields("-i -o -e --input --output --error")},
	"su": {
		valueFlags:  strings.Fields("-s -g -G --shell --group --supp-group"),
		scriptFlags: strings.Fields("-c --command --session-command"),
		noCommand:   true,
	},
	"sudo": {
		valueFlags: strings.Fields(`-u -g -C -D -h -p -r -t -T -U -R --user --group
			--close-from --chdir --host --prompt --role --type --command-timeout
			--other-user --chroot`),
		stopFlags: strings.Fields(
			"-e -l -v -V -K --edit --list --validate --version --remove-timestamp",
		),
	},
	"time":     {valueFlags: strings.Fields("-o -f --output --format")},
	"timeout":  {valueFlags: strings.Fields("-s -k --signal --kill-after"), operands: 1},
	"unbuffer": {},
	"watch":    {valueFlags: strings.Fields("-n --interval"), joined: true},
	"xargs": {
		valueFlags: strings.Fields(`-n -I -L -P -d -E -s -a --max-args --replace --max-lines
			--max-procs --delimiter --eof --max-chars --arg-file`),
		stdinArgs: true,
	},
}

// shells run a command line handed to them after -c, a script file, or stdin.
var shells = nameSet(`ash bash dash ksh mksh sh zsh csh tcsh fish rbash yash posh
	oksh loksh nu elvish xonsh`)

// shellValueFlags are shell options that take the next argument as their value.
var shellValueFlags = strings.Fields("-o +o -O +O --rcfile --init-file")

// interpreter describes how a language interpreter takes inline code.
type interpreter struct {
	codeFlags  []string // take the program source as their value (python -c)
	valueFlags []string // take the next argument as their value
	stopFlags  []string // run something other than a script file (python -m)
	fileFlags  []string // take a script file as their value (awk -f)
	shellLike  bool     // the source is itself a command line (pwsh)
	codeFirst  bool     // the first operand is source, not a file (awk)
}

// awkInterpreter runs its first operand as a program, or the file given to
// -f. system(), print | "cmd" and getline from a command all run commands.
var awkInterpreter = interpreter{
	valueFlags: strings.Fields("-F -v"),
	fileFlags:  strings.Fields("-f"),
	codeFirst:  true,
}

var (
	pythonInterpreter = interpreter{
		codeFlags:  strings.Fields("-c"),
		valueFlags: strings.Fields("-W -X"),
		stopFlags:  strings.Fields("-m"),
	}
	nodeInterpreter = interpreter{
		codeFlags:  strings.Fields("-e -p --eval --print"),
		valueFlags: strings.Fields("-r --require --import --loader"),
	}
	pwshInterpreter = interpreter{codeFlags: strings.Fields("-c -command"), shellLike: true}
)

// interpreters run program source that can start a git command itself.
var interpreters = map[string]interpreter{
	"awk":        awkInterpreter,
	"bun":        {codeFlags: strings.Fields("-e --eval -p --print")},
	"gawk":       awkInterpreter,
	"mawk":       awkInterpreter,
	"nawk":       awkInterpreter,
	"deno":       {codeFlags: strings.Fields("eval")},
	"lua":        {codeFlags: strings.Fields("-e")},
	"node":       nodeInterpreter,
	"nodejs":     nodeInterpreter,
	"osascript":  {codeFlags: strings.Fields("-e")},
	"perl":       {codeFlags: strings.Fields("-e -E"), valueFlags: strings.Fields("-M -I")},
	"php":        {codeFlags: strings.Fields("-r")},
	"powershell": pwshInterpreter,
	"pwsh":       pwshInterpreter,
	"python":     pythonInterpreter,
	"python2":    pythonInterpreter,
	"python3":    pythonInterpreter,
	"rscript":    {codeFlags: strings.Fields("-e")},
	"ruby":       {codeFlags: strings.Fields("-e"), valueFlags: strings.Fields("-r -I")},
	"ts-node":    {codeFlags: strings.Fields("-e --eval")},
	"tsx":        {codeFlags: strings.Fields("-e --eval")},
}

// assignmentPattern matches a NAME=value operand.
var assignmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// commandName returns the program a command word runs. A shell drops
// unquoted backslashes (\git only skips alias lookup), a path such as
// /usr/bin/git or ./git runs the program named by its last element, a flake
// reference such as nixpkgs#git names its package, and case is folded because
// macOS finds git for GIT on its case-insensitive filesystem.
func commandName(word string) string {
	name := word
	if strings.Contains(name, `\`) {
		name = unescape(name)
	}

	if strings.Contains(name, "/") && name != "/" {
		name = path.Base(name)
	}

	if i := strings.LastIndexByte(name, '#'); i >= 0 && i < len(name)-1 {
		name = name[i+1:]
	}

	return strings.ToLower(name)
}

// unescape drops backslash escapes, keeping the escaped character.
func unescape(s string) string {
	var b strings.Builder

	escaped := false

	for _, r := range s {
		if r == '\\' && !escaped {
			escaped = true

			continue
		}

		escaped = false

		b.WriteRune(r)
	}

	return b.String()
}

// launched returns what cmd runs besides itself.
func launched(cmd Command) launch {
	if l, ok := launchedBy(cmd); ok {
		return l
	}

	if spec, ok := launchers[cmd.Name]; ok {
		return launcherLaunch(cmd, spec)
	}

	if spec, ok := interpreters[cmd.Name]; ok {
		return interpreterLaunch(cmd, spec)
	}

	if dataCommands[cmd.Name] {
		return launch{}
	}

	l := scanLaunch(cmd)

	// A program given by path may be a script whose commands would otherwise
	// run unseen.
	if strings.Contains(cmd.Invoked, "/") {
		l.files = append(l.files, scriptFile{path: cmd.Invoked, explicit: true})
	}

	return l
}

// launchedBy handles the programs with their own way of running commands.
func launchedBy(cmd Command) (launch, bool) {
	switch {
	case cmd.Name == gitProgram:
		return gitLaunch(cmd), true
	case cmd.Name == ghCLI:
		return launch{}, true
	case shells[cmd.Name]:
		return shellLaunch(cmd), true
	case cmd.Name == "eval":
		return launch{scripts: []string{strings.Join(cmd.Args, " ")}}, true
	case cmd.Name == "source" || cmd.Name == ".":
		return sourceLaunch(cmd), true
	case cmd.Name == "find":
		return launch{commands: findExecCommands(cmd)}, true
	case cmd.Name == "trap":
		// trap 'command line' SIGNAL... runs the line when the signal comes.
		if operand := firstOperand(cmd.Args); operand != "" && operand != "-" {
			return launch{scripts: []string{operand}}, true
		}

		return launch{}, true
	case editors[cmd.Name]:
		return launch{scripts: editorShellCommands(cmd.Args)}, true
	case makers[cmd.Name]:
		return launch{scripts: stdinRecipes(cmd)}, true
	default:
		return launch{}, false
	}
}

// editors run a shell command given as "-c '!cmd'" or "+!cmd".
var editors = nameSet("vim vi nvim ex view gvim mvim")

// editorShellCommands returns the command lines an editor's -c, --cmd and
// + arguments run through ! (vim -es -c '!git commit').
func editorShellCommands(args []string) []string {
	var scripts []string

	for i, arg := range args {
		var command string

		switch {
		case (arg == "-c" || arg == "--cmd") && i+1 < len(args):
			command = args[i+1]
		case strings.HasPrefix(arg, "+"):
			command = arg[1:]
		default:
			continue
		}

		if line, ok := strings.CutPrefix(strings.TrimLeft(command, ": "), "!"); ok {
			scripts = append(scripts, line)
		}
	}

	return scripts
}

var (
	// makers run recipe lines as shell commands.
	makers = nameSet("make gmake")
	// makefileFlags name the makefile to read.
	makefileFlags = nameSet("-f --file --makefile")
)

// stdinRecipes returns the recipe lines of a makefile read from stdin
// (make -f -), which run as shell commands.
func stdinRecipes(cmd Command) []string {
	fromStdin := false

	for i, arg := range cmd.Args {
		switch {
		case makefileFlags[arg] && i+1 < len(cmd.Args):
			fromStdin = fromStdin || cmd.Args[i+1] == "-" || cmd.Args[i+1] == "/dev/stdin"
		case arg == "-f-" || arg == "--file=-":
			fromStdin = true
		}
	}

	if !fromStdin {
		return nil
	}

	var recipes []string

	for line := range strings.Lines(cmd.Stdin) {
		if recipe, ok := strings.CutPrefix(line, "\t"); ok {
			recipes = append(recipes, strings.TrimLeft(recipe, "@-+"))
		}
	}

	return recipes
}

// gitShellConfigKeys are git settings whose value git runs as a command.
var gitShellConfigKeys = nameSet(`core.editor core.pager core.sshcommand
	core.fsmonitor sequence.editor diff.external gpg.program credential.helper
	core.askpass`)

// gitLaunch returns the command lines git itself runs: shell-valued -c
// settings, rebase --exec, submodule foreach, bisect run and difftool
// --extcmd.
func gitLaunch(cmd Command) launch {
	var l launch

	idx := gitSubcommandIndex(cmd.Args)
	globals := cmd.Args

	if idx >= 0 {
		globals = cmd.Args[:idx]
	}

	for i, arg := range globals {
		setting, found := strings.CutPrefix(arg, flagLowerC)
		if arg == flagLowerC && i+1 < len(globals) {
			setting, found = globals[i+1], true
		}

		key, value, ok := strings.Cut(setting, "=")
		if found && ok && isGitShellSetting(strings.ToLower(key)) {
			l.scripts = append(l.scripts, strings.TrimPrefix(value, "!"))
		}
	}

	if idx < 0 {
		return l
	}

	sub, rest := cmd.Args[idx], cmd.Args[idx+1:]

	switch sub {
	case "rebase":
		l.scripts = append(l.scripts, optionValues(rest, "-x", "--exec")...)
	case "difftool", "mergetool":
		l.scripts = append(l.scripts, optionValues(rest, "-x", "--extcmd")...)
	case "submodule":
		if i := slices.Index(rest, "foreach"); i >= 0 {
			l.scripts = append(l.scripts, strings.Join(skipOptions(rest[i+1:]), " "))
		}
	case "bisect":
		if len(rest) > 1 && rest[0] == "run" {
			l.commands = append(l.commands, childCommand(cmd, rest[1], rest[2:]))
		}
	}

	return l
}

// isGitShellSetting reports whether a git setting's value runs as a command.
func isGitShellSetting(key string) bool {
	return gitShellConfigKeys[key] || strings.HasSuffix(key, ".textconv") ||
		(strings.HasPrefix(key, "filter.") && !strings.HasSuffix(key, ".required")) ||
		(strings.HasPrefix(key, "merge.") && strings.HasSuffix(key, ".driver"))
}

// optionValues returns the values of an option given as "-x v", "--exec v",
// "-xv" or "--exec=v".
func optionValues(args []string, short, long string) []string {
	var values []string

	for i, arg := range args {
		switch {
		case (arg == short || arg == long) && i+1 < len(args):
			values = append(values, args[i+1])
		case strings.HasPrefix(arg, long+"="):
			values = append(values, strings.TrimPrefix(arg, long+"="))
		case strings.HasPrefix(arg, short) && len(arg) > len(short):
			values = append(values, arg[len(short):])
		}
	}

	return values
}

// skipOptions drops the leading options of an argument list.
func skipOptions(args []string) []string {
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return args[i:]
		}
	}

	return nil
}

// launcherLaunch returns what a known launcher runs.
func launcherLaunch(cmd Command, spec launcher) launch {
	l := launch{scripts: scriptFlagValues(spec, cmd.Args)}

	if spec.noCommand {
		return l
	}

	idx, ok := commandIndex(spec, cmd.Args)
	if !ok {
		return l
	}

	if spec.joined {
		l.scripts = append(l.scripts, strings.Join(cmd.Args[idx:], " "))
	}

	child := childCommand(cmd, cmd.Args[idx], cmd.Args[idx+1:])
	if spec.stdinArgs && cmd.Stdin != "" {
		l.commands = xargsCommands(child, cmd.Stdin, xargsReplace(cmd.Args[:idx]))
	} else {
		l.commands = []Command{child}
	}

	return l
}

// commandIndex finds the launched command among a launcher's arguments.
func commandIndex(spec launcher, args []string) (int, bool) {
	operands := spec.operands

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions:
			return i + 1, i+1 < len(args)
		case slices.Contains(spec.stopFlags, arg):
			return 0, false
		case slices.Contains(spec.valueFlags, arg), slices.Contains(spec.scriptFlags, arg):
			i++
		case strings.HasPrefix(arg, "-"):
			// A flag, with any value attached (-uroot, --user=root).
		case spec.assignments && assignmentPattern.MatchString(arg):
		case operands > 0:
			operands--
		default:
			return i, true
		}
	}

	return 0, false
}

// scriptFlagValues returns the command lines passed to a launcher's script
// flags, as "-c line" or "--command=line".
func scriptFlagValues(spec launcher, args []string) []string {
	var scripts []string

	for i, arg := range args {
		value, attached, ok := flagValue(arg, spec.scriptFlags)

		switch {
		case ok && attached:
			scripts = append(scripts, value)
		case ok && i+1 < len(args):
			scripts = append(scripts, args[i+1])
		}
	}

	return scripts
}

// flagValue matches arg against flags that take a value, in every form a
// program accepts: separate ("-c" "code"), long attached ("--command=code"),
// short attached ("-ccode") and clustered with other flags ("-lc" "code",
// "-pe" "code"). A match with attached false means the value is the next
// argument.
func flagValue(arg string, flags []string) (value string, attached, ok bool) {
	if slices.Contains(flags, arg) || slices.Contains(flags, strings.ToLower(arg)) {
		return "", false, true
	}

	var letters strings.Builder

	for _, flag := range flags {
		if long, isLong := strings.CutPrefix(flag, "--"); isLong {
			if v, found := strings.CutPrefix(arg, "--"+long+"="); found {
				return v, true, true
			}

			continue
		}

		if len(flag) == len("-c") && flag[0] == '-' {
			letters.WriteByte(flag[1])
		}
	}

	if letters.Len() == 0 || len(arg) < len("-cx") || arg[0] != '-' || arg[1] == '-' {
		return "", false, false
	}

	cluster := arg[1:]

	for i := range len(cluster) {
		if !strings.ContainsRune(letters.String(), rune(cluster[i])) {
			continue
		}

		rest := cluster[i+1:]

		switch {
		case rest == "":
			return "", false, true
		case strings.Trim(rest, letters.String()) == "":
			// More value flags follow in the cluster; the last one takes it.
		default:
			return rest, true, true
		}
	}

	return "", false, false
}

// hasAttachedValue reports whether arg is a short value flag carrying its
// value, such as perl's -Mstrict, so it is not read as a cluster.
func hasAttachedValue(arg string, flags []string) bool {
	for _, flag := range flags {
		if len(flag) == len("-c") && len(arg) > len(flag) && strings.HasPrefix(arg, flag) {
			return true
		}
	}

	return false
}

// xargsReplace returns the replace string xargs substitutes stdin into.
func xargsReplace(args []string) string {
	for i, arg := range args {
		switch {
		case arg == "-I" && i+1 < len(args):
			return args[i+1]
		case strings.HasPrefix(arg, "--replace="):
			return strings.TrimPrefix(arg, "--replace=")
		case arg == "-i" || arg == "--replace":
			return "{}"
		case strings.HasPrefix(arg, "-I") && len(arg) > 2:
			return arg[2:]
		}
	}

	return ""
}

// xargsCommands builds the commands xargs runs from literal stdin: one per
// input line with a replace string, otherwise one with the input appended.
func xargsCommands(child Command, stdin, replace string) []Command {
	if replace == "" {
		child.Args = append(slices.Clone(child.Args), strings.Fields(stdin)...)

		return []Command{child}
	}

	var cmds []Command

	for line := range strings.SplitSeq(strings.TrimSpace(stdin), "\n") {
		cmd := child
		cmd.Args = make([]string, len(child.Args))

		for i, arg := range child.Args {
			cmd.Args[i] = strings.ReplaceAll(arg, replace, line)
		}

		cmds = append(cmds, cmd)
	}

	return cmds
}

// shellLaunch returns what a shell runs: the command line after -c, a script
// file, or its stdin.
func shellLaunch(cmd Command) launch {
	operand, isScript, ok := shellOperand(cmd.Args)

	switch {
	case ok && isScript:
		return launch{scripts: []string{operand}}
	case ok:
		return launch{files: []scriptFile{{path: operand, explicit: true}}}
	case cmd.Stdin != "":
		return launch{scripts: []string{cmd.Stdin}}
	case cmd.StdinFile != "":
		return launch{files: []scriptFile{{path: cmd.StdinFile, explicit: true}}}
	default:
		return launch{}
	}
}

// shellOperand returns a shell's first operand: the command line it runs when
// a flag cluster carries -c, otherwise the script file it is given. -s makes
// the operands positional parameters, leaving stdin as the script.
func shellOperand(args []string) (operand string, isScript, ok bool) {
	sawC := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case slices.Contains(shellValueFlags, arg):
			i++
		case arg == endOfOptions:
			if i+1 < len(args) {
				return args[i+1], sawC, true
			}

			return "", false, false
		case strings.HasPrefix(arg, "--"), strings.HasPrefix(arg, "+"):
		case strings.HasPrefix(arg, "-"):
			switch cluster := arg[1:]; {
			case strings.Contains(cluster, "c"):
				sawC = true
			case strings.Contains(cluster, "s"):
				return "", false, false
			}
		default:
			return arg, sawC, true
		}
	}

	return "", false, false
}

// sourceLaunch returns the file source or . reads.
func sourceLaunch(cmd Command) launch {
	if len(cmd.Args) == 0 {
		return launch{}
	}

	return launch{files: []scriptFile{{path: cmd.Args[0], explicit: true}}}
}

// interpreterLaunch returns the source a language interpreter runs: inline
// code, a script file, or stdin.
func interpreterLaunch(cmd Command, spec interpreter) launch {
	var l launch

args:
	for i := 0; i < len(cmd.Args); i++ {
		arg := cmd.Args[i]

		switch {
		case slices.Contains(spec.stopFlags, arg):
			// python -m runs a module; what follows are its arguments.
			break args
		case slices.Contains(spec.fileFlags, arg) && i+1 < len(cmd.Args):
			l.files = append(l.files, scriptFile{path: cmd.Args[i+1], interpreter: true, explicit: true})
			i++
		case slices.Contains(spec.valueFlags, arg):
			i++
		case hasAttachedValue(arg, spec.valueFlags):
		case spec.codeFirst && !strings.HasPrefix(arg, "-") && len(l.code) == 0 && len(l.files) == 0:
			l.code = append(l.code, arg)

			break args
		case strings.HasPrefix(arg, "-"):
			value, attached, ok := flagValue(arg, spec.codeFlags)

			switch {
			case ok && attached:
				l.code = append(l.code, value)
			case ok && i+1 < len(cmd.Args):
				l.code = append(l.code, cmd.Args[i+1])
				i++
			}
		case slices.Contains(spec.codeFlags, arg) && i+1 < len(cmd.Args):
			// A code "flag" without a dash, like deno's eval.
			l.code = append(l.code, cmd.Args[i+1])
			i++
		default:
			// The first operand is the script, unless code came inline.
			if len(l.code) == 0 {
				l.files = append(l.files, scriptFile{path: arg, interpreter: !spec.shellLike, explicit: true})
			}

			break args
		}
	}

	if len(l.code) == 0 && len(l.files) == 0 {
		l = stdinSource(cmd, spec)
	}

	// A shell-like language runs its source as a command line too.
	if spec.shellLike {
		l.scripts = l.code
	}

	return l
}

// stdinSource returns what an interpreter given no code or script reads
// from stdin.
func stdinSource(cmd Command, spec interpreter) launch {
	switch {
	case cmd.Stdin != "":
		return launch{code: []string{cmd.Stdin}}
	case cmd.StdinFile != "":
		return launch{
			files: []scriptFile{
				{path: cmd.StdinFile, interpreter: !spec.shellLike, explicit: true},
			},
		}
	default:
		return launch{}
	}
}

// findExecCommands returns the commands find runs through -exec, -execdir,
// -ok and -okdir, each ending at ";" or "+".
func findExecCommands(cmd Command) []Command {
	var cmds []Command

	for i := 0; i < len(cmd.Args); i++ {
		switch cmd.Args[i] {
		case "-exec", "-execdir", "-ok", "-okdir":
		default:
			continue
		}

		end := i + 1
		for end < len(cmd.Args) && cmd.Args[end] != ";" && cmd.Args[end] != "+" {
			end++
		}

		if end > i+1 {
			cmds = append(cmds, childCommand(cmd, cmd.Args[i+1], cmd.Args[i+2:end]))
		}

		i = end
	}

	return cmds
}

// scanLaunch finds a git, gh or shell invocation among the arguments of a
// command that is not a known launcher, covering runners such as mise exec,
// nix run, docker run and ssh without listing each one.
func scanLaunch(cmd Command) launch {
	for i, arg := range cmd.Args {
		rest := cmd.Args[i+1:]
		if len(rest) > 0 && rest[0] == endOfOptions {
			rest = rest[1:]
		}

		if launchesTracked(arg, rest) {
			return launch{commands: []Command{childCommand(cmd, arg, rest)}}
		}

		// One argument holding a whole command line, as tmux, parallel and
		// script -c take it.
		if fields := strings.Fields(
			arg,
		); len(fields) > 1 &&
			launchesTracked(fields[0], fields[1:]) {
			return launch{scripts: []string{arg}}
		}
	}

	return launch{}
}

// launchesTracked reports whether arg followed by rest runs something worth
// following: a validated git or gh command, a shell given a script, an
// interpreter, a launcher, eval or source, or a shell script given by path.
func launchesTracked(arg string, rest []string) bool {
	name := commandName(arg)
	_, isInterpreter := interpreters[name]
	_, isLauncher := launchers[name]

	switch {
	case name == gitProgram || name == hubCLI:
		idx := gitSubcommandIndex(rest)

		return idx >= 0 && validatedGitSubcommands[rest[idx]]
	case strings.HasPrefix(name, "git-"):
		return validatedGitSubcommands[strings.TrimPrefix(name, "git-")]
	case name == ghCLI:
		return len(rest) > 0 && validatedGHCommands[rest[0]]
	case shells[name]:
		_, _, ok := shellOperand(rest)

		return ok
	case isInterpreter, isLauncher, name == "eval", name == "source":
		return true
	default:
		// Only shell scripts: a test runner given code files that merely
		// mention git must not be read as running it.
		return strings.Contains(arg, "/") && shellScriptExtensions[path.Ext(name)]
	}
}

// shellScriptExtensions mark files a runner executes as shell scripts.
var shellScriptExtensions = nameSet(".sh .bash .zsh")

// gitSubcommandIndex returns the position of git's subcommand, after any
// global options, or -1 when there is none.
func gitSubcommandIndex(args []string) int {
	idx := parseGlobalOptions(args, &GitCommand{GlobalOptions: make(map[string]string)})
	if idx >= len(args) {
		return -1
	}

	return idx
}

// childCommand builds a command launched by parent, keeping its context.
func childCommand(parent Command, name string, args []string) Command {
	return Command{
		Name:             name,
		Args:             args,
		Location:         parent.Location,
		Type:             parent.Type,
		WorkingDirectory: parent.WorkingDirectory,
		Stdin:            parent.Stdin,
		StdinFile:        parent.StdinFile,
	}
}
