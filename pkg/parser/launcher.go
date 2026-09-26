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
	// without limit.
	maxLaunchDepth = 5
	// endOfOptions ends a command's options; what follows is an operand.
	endOfOptions = "--"
)

// launch lists what a command runs besides itself.
type launch struct {
	commands []Command    // run directly (env git, find -exec git)
	scripts  []string     // shell command lines (bash -c, eval, su -c)
	files    []scriptFile // files run as scripts (bash x.sh, ./x.sh, source x)
	code     []string     // interpreter source scanned for commands (python -c)
}

// scriptFile is a file a command runs.
type scriptFile struct {
	path        string
	interpreter bool // run by a language interpreter rather than a shell
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
}

// launchers are the commands that run another command named in their
// arguments. Each would otherwise hide what it runs from every validator.
var launchers = map[string]launcher{
	"busybox":    {},
	"caffeinate": {valueFlags: flags("-t -w")},
	"chronic":    {},
	"command":    {stopFlags: flags("-v -V")},
	"doas":       {valueFlags: flags("-u -C"), stopFlags: flags("-L")},
	"env": {
		valueFlags:  flags("-u -C --unset --chdir"),
		scriptFlags: flags("-S --split-string"),
		assignments: true,
	},
	"exec": {valueFlags: flags("-a")},
	"flock": {
		valueFlags:  flags("-w -E --timeout --conflict-exit-code"),
		scriptFlags: flags("-c --command"),
		operands:    1,
	},
	"ionice": {
		valueFlags: flags("-c -n --class --classdata"),
		stopFlags:  flags("-p -P -u --pid --pgid --uid"),
	},
	"nice":   {valueFlags: flags("-n --adjustment")},
	"nohup":  {},
	"setsid": {},
	"stdbuf": {valueFlags: flags("-i -o -e --input --output --error")},
	"su": {
		valueFlags:  flags("-s -g -G --shell --group --supp-group"),
		scriptFlags: flags("-c --command --session-command"),
		noCommand:   true,
	},
	"sudo": {
		valueFlags: flags(`-u -g -C -D -h -p -r -t -T -U -R --user --group
			--close-from --chdir --host --prompt --role --type --command-timeout
			--other-user --chroot`),
		stopFlags: flags("-e -l -v -V -K --edit --list --validate --version --remove-timestamp"),
	},
	"time":     {valueFlags: flags("-o -f --output --format")},
	"timeout":  {valueFlags: flags("-s -k --signal --kill-after"), operands: 1},
	"unbuffer": {},
	"watch":    {valueFlags: flags("-n --interval"), joined: true},
	"xargs": {
		valueFlags: flags(`-n -I -L -P -d -E -s -a --max-args --replace --max-lines
			--max-procs --delimiter --eof --max-chars --arg-file`),
	},
}

// shells run a command line handed to them after -c, a script file, or stdin.
var shells = nameSet("ash bash dash ksh mksh sh zsh")

// shellValueFlags are shell options that take the next argument as their value.
var shellValueFlags = flags("-o +o -O +O --rcfile --init-file")

// interpreter describes how a language interpreter takes inline code.
type interpreter struct {
	codeFlags  []string // take the program source as their value (python -c)
	valueFlags []string // take the next argument as their value
	shellLike  bool     // the source is itself a command line (pwsh)
}

var (
	pythonInterpreter = interpreter{codeFlags: flags("-c"), valueFlags: flags("-W -X -m")}
	nodeInterpreter   = interpreter{
		codeFlags:  flags("-e -p --eval --print"),
		valueFlags: flags("-r --require --import --loader"),
	}
	pwshInterpreter = interpreter{codeFlags: flags("-c -command"), shellLike: true}
)

// interpreters run program source that can start a git command itself.
var interpreters = map[string]interpreter{
	"bun":        {codeFlags: flags("-e --eval -p --print")},
	"deno":       {codeFlags: flags("eval")},
	"lua":        {codeFlags: flags("-e")},
	"node":       nodeInterpreter,
	"nodejs":     nodeInterpreter,
	"osascript":  {codeFlags: flags("-e")},
	"perl":       {codeFlags: flags("-e -E"), valueFlags: flags("-M -I")},
	"php":        {codeFlags: flags("-r")},
	"powershell": pwshInterpreter,
	"pwsh":       pwshInterpreter,
	"python":     pythonInterpreter,
	"python2":    pythonInterpreter,
	"python3":    pythonInterpreter,
	"rscript":    {codeFlags: flags("-e")},
	"ruby":       {codeFlags: flags("-e"), valueFlags: flags("-r -I")},
	"ts-node":    {codeFlags: flags("-e --eval")},
	"tsx":        {codeFlags: flags("-e --eval")},
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
	switch {
	case cmd.Name == gitProgram || cmd.Name == ghCLI:
		return launch{}
	case shells[cmd.Name]:
		return shellLaunch(cmd)
	case cmd.Name == "eval":
		return launch{scripts: []string{strings.Join(cmd.Args, " ")}}
	case cmd.Name == "source" || cmd.Name == ".":
		return sourceLaunch(cmd)
	case cmd.Name == "find":
		return launch{commands: findExecCommands(cmd)}
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
		l.files = append(l.files, scriptFile{path: cmd.Invoked})
	}

	return l
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
	if cmd.Name == "xargs" && cmd.Stdin != "" {
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
		for _, flag := range spec.scriptFlags {
			if arg == flag && i+1 < len(args) {
				scripts = append(scripts, args[i+1])
			}

			value, found := strings.CutPrefix(arg, flag+"=")
			if found && strings.HasPrefix(flag, "--") {
				scripts = append(scripts, value)
			}
		}
	}

	return scripts
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
	script, file, ok := shellOperand(cmd.Args)

	switch {
	case ok && file == "":
		return launch{scripts: []string{script}}
	case ok:
		return launch{files: []scriptFile{{path: file}}}
	case cmd.Stdin != "":
		return launch{scripts: []string{cmd.Stdin}}
	case cmd.StdinFile != "":
		return launch{files: []scriptFile{{path: cmd.StdinFile}}}
	default:
		return launch{}
	}
}

// shellOperand returns the command line a shell runs after a flag cluster
// carrying -c, or else the script file it is given. -s makes the operands
// positional parameters, leaving stdin as the script.
func shellOperand(args []string) (script, file string, ok bool) {
	sawC := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case slices.Contains(shellValueFlags, arg):
			i++
		case arg == endOfOptions:
			if i+1 < len(args) {
				return shellOperandAt(args[i+1], sawC)
			}

			return "", "", false
		case strings.HasPrefix(arg, "--"):
		case strings.HasPrefix(arg, "-"):
			switch cluster := arg[1:]; {
			case strings.Contains(cluster, "c"):
				sawC = true
			case strings.Contains(cluster, "s"):
				return "", "", false
			}
		case strings.HasPrefix(arg, "+"):
		default:
			return shellOperandAt(arg, sawC)
		}
	}

	return "", "", false
}

// shellOperandAt returns a shell's first operand as a script or a file.
func shellOperandAt(operand string, sawC bool) (script, file string, ok bool) {
	if sawC {
		return operand, "", true
	}

	return "", operand, true
}

// sourceLaunch returns the file source or . reads.
func sourceLaunch(cmd Command) launch {
	if len(cmd.Args) == 0 {
		return launch{}
	}

	return launch{files: []scriptFile{{path: cmd.Args[0]}}}
}

// interpreterLaunch returns the source a language interpreter runs: inline
// code, a script file, or stdin.
func interpreterLaunch(cmd Command, spec interpreter) launch {
	var l launch

	for i := 0; i < len(cmd.Args); i++ {
		arg := cmd.Args[i]

		switch {
		case slices.Contains(spec.codeFlags, strings.ToLower(arg)) && i+1 < len(cmd.Args):
			l.code = append(l.code, cmd.Args[i+1])
			i++
		case slices.Contains(spec.valueFlags, arg):
			i++
		case strings.HasPrefix(arg, "-"):
		default:
			if len(l.code) == 0 {
				l.files = append(l.files, scriptFile{path: arg, interpreter: !spec.shellLike})
			}

			return shellLikeCode(l, spec)
		}
	}

	if len(l.code) == 0 && cmd.Stdin != "" {
		l.code = append(l.code, cmd.Stdin)
	}

	if len(l.code) == 0 && cmd.StdinFile != "" {
		l.files = append(l.files, scriptFile{path: cmd.StdinFile, interpreter: !spec.shellLike})
	}

	return shellLikeCode(l, spec)
}

// shellLikeCode also runs an interpreter's source as a command line when the
// language is itself a shell.
func shellLikeCode(l launch, spec interpreter) launch {
	if spec.shellLike {
		l.scripts = append(l.scripts, l.code...)
	}

	return l
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

		if launchesTracked(commandName(arg), rest) {
			return launch{commands: []Command{childCommand(cmd, arg, rest)}}
		}
	}

	return launch{}
}

// launchesTracked reports whether name followed by rest runs something the
// validators check: a validated git or gh command, or a shell's -c line.
func launchesTracked(name string, rest []string) bool {
	switch {
	case name == gitProgram || name == "hub":
		idx := gitSubcommandIndex(rest)

		return idx >= 0 && validatedGitSubcommands[rest[idx]]
	case strings.HasPrefix(name, "git-"):
		return validatedGitSubcommands[strings.TrimPrefix(name, "git-")]
	case name == ghCLI:
		return len(rest) > 0 && validatedGHCommands[rest[0]]
	case shells[name]:
		script, file, ok := shellOperand(rest)

		return ok && file == "" && script != ""
	default:
		return false
	}
}

// gitSubcommandIndex returns the position of git's subcommand, after any
// global options, or -1 when there is none.
func gitSubcommandIndex(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return i
		}

		if globalOptionsWithValue[arg] {
			i++
		}
	}

	return -1
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
