package parser

import (
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// bashEnvVar names the file bash reads before a non-interactive script.
const bashEnvVar = "BASH_ENV"

// envVar names the file an interactive POSIX shell reads first.
const envVar = "ENV"

// rcfileLabel names the file bash --rcfile or --init-file reads.
const rcfileLabel = "--rcfile"

// devNull is the usual way to give a shell no startup file.
const devNull = "/dev/null"

// startupPrefix marks a startup file in expanding, so a shell it starts does
// not read it again.
const startupPrefix = "startup:"

// startupVars are the variables naming a file a new shell runs first, or
// the directory it reads its own startup files from.
var startupVars = nameSet(anyStartupVar)

// startupMention matches a startup variable named anywhere in a loop body.
var startupMention = regexp.MustCompile(
	`(^|[^A-Za-z0-9_])(BASH_ENV|ENV|HOME|ZDOTDIR)([^A-Za-z0-9_]|$)`,
)

// specialBuiltins keep prefix assignments in the shell in POSIX mode.
var specialBuiltins = nameSet(`: . break continue eval exec exit export readonly
	return set shift source times trap unset`)

// startupValue is a startup variable set for one command alone, as a prefix
// assignment or an env operand.
type startupValue struct {
	value    string
	dynamic  bool
	anyName  bool
	deferred bool
	literal  bool
	unset    bool
}

// startupScript is a file a shell runs before anything else.
// A lazy one is a zsh file after .zshenv, read again when .zshenv moves
// ZDOTDIR or HOME; one with no key only marks where that happens. A lenient
// one sits on disk where the line never touched it, so what cannot be
// inspected in it does not block.
type startupScript struct {
	label   string
	path    string
	key     string
	text    string
	lazy    bool
	logout  bool
	touched bool
	lenient bool
}

// prefixStartup returns the startup variables a call sets for its command
// alone.
func prefixStartup(call *syntax.CallExpr) map[string]startupValue {
	var vars map[string]startupValue

	for _, assign := range call.Assigns {
		if assign.Name == nil || !startupVars[assign.Name.Value] {
			continue
		}

		if vars == nil {
			vars = make(map[string]startupValue)
		}

		vars[assign.Name.Value] = startupValue{
			value: wordToString(assign.Value),
			dynamic: assign.Append || assign.Naked || assign.Index != nil ||
				assign.Array != nil || wordDynamic(assign.Value),
			deferred: deferredWord(assign.Value),
		}
	}

	return vars
}

// keepsPrefix reports whether a prefix assignment on a call to name may
// outlive the call: a special builtin or a function in POSIX mode keeps it,
// and so may a name klaudiush cannot resolve. Any other command gets it for
// itself alone.
func (w *astWalker) keepsPrefix(word string) bool {
	invoked := w.expandName(word)

	return specialBuiltins[commandName(invoked)] || w.defined(invoked) ||
		HasUnresolvedVars(invoked) || strings.Contains(invoked, unresolvedProgram)
}

// withEnvOperands adds the startup variables env sets in its NAME=value
// operands to the command it runs. A dynamic word on the line may be one of
// them, rendered without the part that comes from command output.
func withEnvOperands(child, parent Command, operands []string) Command {
	set := func(name string, v startupValue) {
		vars := maps.Clone(child.startup)
		if vars == nil {
			vars = make(map[string]startupValue)
		}

		vars[name] = v
		child.startup = vars
	}

	for _, arg := range append(slices.Clone(operands), child.Name) {
		name, value, ok := strings.Cut(arg, "=")

		switch {
		case unknownOperand(name, ok, arg):
			for startupVar := range startupVars {
				set(startupVar, startupValue{dynamic: true, anyName: true})
			}
		case ok && startupVars[name]:
			set(
				name,
				startupValue{
					value: value,
					dynamic: marked(value) ||
						parent.dynamicWords[strings.ReplaceAll(arg, unresolvedWord, "")],
				},
			)
		}
	}

	return child
}

// unknownOperand reports an env operand, or the word env runs, that may
// expand to NAME=value for any name: one from command output or a variable.
func unknownOperand(name string, hasValue bool, arg string) bool {
	if hasValue && variableName.MatchString(name) {
		return false
	}

	return marked(arg) || HasUnresolvedVars(arg) || strings.Contains(arg, unresolvedProgram)
}

// startupScripts returns the files a new shell started by cmd runs first:
// BASH_ENV for any program that may start bash, ENV for an interactive shell
// and the --rcfile of bash. A file whose path or content cannot be known is
// recorded as opaque instead.
func (w *astWalker) startupScripts(cmd Command, args []string) []startupScript {
	if loginProgram(cmd.Name, args) {
		w.opaque(OpacityStartupFile, cmd.Name, DetailLoginAccount)

		return nil
	}

	_, isLauncher := launchers[cmd.Name]
	if shellBuiltins[cmd.Name] || dataCommands[cmd.Name] || isLauncher ||
		w.defined(cmd.Invoked) {
		return nil
	}

	var scripts []startupScript

	add := func(label string, v startupValue, set bool) {
		if script, ok := w.startupScript(cmd, label, v, set); ok {
			scripts = append(scripts, script)
		}
	}

	if !shells[cmd.Name] {
		v, set := w.startupSetting(cmd, bashEnvVar)
		add(bashEnvVar, v, set)

		return scripts
	}

	mode := shellOptions(cmd.Name, w.optionWords(args))
	mode.login = mode.login || cmd.loginArgv0
	named := rcfiles(args)

	var after, systemAfter []string

	if cmd.Name == zshShell && mode.emulate == "" {
		scripts = append(scripts, w.zshStartupScripts(cmd, mode)...)
	} else {
		before, homeAfter := homeStartupFiles(cmd.Name, mode, len(named) > 0)
		after = homeAfter

		systemName := cmd.Name
		if cmd.Name == zshShell {
			systemName = zshEmulation(mode.emulate)
		}

		systemBefore, lateSystem := systemStartupFiles(systemName, mode, len(named) > 0)
		systemAfter = lateSystem

		scripts = append(scripts, w.systemScripts(cmd, systemBefore)...)
		scripts = append(scripts, w.homeScripts(cmd, before, false)...)
	}

	if cmd.Name == fishShell && !mode.noRC {
		w.checkFishConfScripts(cmd)
	}

	v, set := w.startupSetting(cmd, bashEnvVar)
	add(bashEnvVar, v, set)

	if mode.interactive {
		v, set = w.startupSetting(cmd, envVar)
		add(envVar, v, set)
	}

	if mode.interactive && bashShells[cmd.Name] {
		for _, rcfile := range named {
			add(rcfileLabel, literalRCFile(rcfile), true)
		}
	}

	scripts = append(scripts, w.systemScripts(cmd, systemAfter)...)

	return append(scripts, w.homeScripts(cmd, after, false)...)
}

// bashShells read --rcfile and --init-file.
var bashShells = nameSet("bash rbash")

// rcfiles returns the files bash --rcfile and --init-file name among its
// options. Anything after the first operand, which -c takes as the command,
// is a positional parameter.
func rcfiles(args []string) []string {
	var files []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions:
			return files
		case arg == rcfileLabel || arg == "--init-file":
			if i+1 < len(args) {
				files = append(files, args[i+1])
			}

			i++
		case slices.Contains(shellValueFlags, arg):
			i++
		case strings.HasPrefix(arg, "-"), strings.HasPrefix(arg, "+"):
		default:
			return files
		}
	}

	return files
}

// startupSetting returns what a startup variable holds for cmd: the value
// set for it alone, or the one assigned on the line. set is false when the
// line never sets it; the environment klaudiush runs in is not consulted.
// A command inside the value of an assignment runs before it.
func (w *astWalker) startupSetting(cmd Command, name string) (startupValue, bool) {
	if v, ok := cmd.startup[name]; ok && v.unset {
		return startupValue{}, false
	}

	if v, ok := cmd.startup[name]; ok && (!v.anyName || startsShell(cmd)) {
		return v, true
	}

	if w.inLoop && w.loopStartup[name] {
		return startupValue{dynamic: true}, true
	}

	if end, ok := w.startupPending[name]; ok && runsBefore(cmd.Location, end) {
		return startupValue{}, false
	}

	if w.startupUnknown(name) {
		return startupValue{dynamic: true}, true
	}

	if w.state.namesUnknown && startsShell(cmd) {
		return startupValue{dynamic: true}, true
	}

	value, set := w.assignments[name]

	return startupValue{value: value, deferred: w.startupDeferred[name]}, set
}

// startsShell reports whether cmd is itself a shell or a script run by
// path. After a write to a name klaudiush cannot read only these are
// blocked: any program may start bash, but blocking every one would block
// export $(cat .env) followed by anything.
func startsShell(cmd Command) bool {
	return shells[cmd.Name] || strings.Contains(cmd.Invoked, "/")
}

// startupUnknown reports whether a startup variable may hold a value the
// parser cannot know: one from command output or an untracked write, one a
// later loop iteration may change. A plain unset leaves only the earlier
// value or nothing.
func (w *astWalker) startupUnknown(name string) bool {
	return (w.unknownVars[name] && !w.startupUnset[name]) || w.state.dynamicVars[name] ||
		(w.inLoop && w.loopStartup[name])
}

// startupScript reads the startup file a setting names, recording why when
// it cannot. The shell opens "-" as a file, not stdin.
func (w *astWalker) startupScript(
	cmd Command,
	label string,
	v startupValue,
	set bool,
) (startupScript, bool) {
	if !set {
		return startupScript{}, false
	}

	if v.dynamic {
		w.opaque(OpacityStartupFile, label, DetailStartupValue)

		return startupScript{}, false
	}

	path, detail := w.startupPath(v)
	if path == "-" {
		path = "./-"
	}

	clean := resolvePath(cmd.WorkingDirectory, path)

	switch {
	case detail != "":
	case path == "":
		return startupScript{}, false
	case clean == devNull:
		return startupScript{}, false
	case clean == devStdin && w.expanding[startupPrefix+devStdin] && cmd.Stdin == "" && cmd.StdinFile == "":
		return startupScript{}, false
	case clean == devStdin:
		path, detail = startupStdin(cmd)
		if detail == "" && path != devStdin {
			path, detail = redirectedStdin(cmd, path)
		}

		if detail == "" && path == "" {
			return startupScript{}, false
		}
	case specialPath(clean) || (!filepath.IsAbs(clean) && specialPath(filepath.Join("/", clean))):
		detail = DetailScriptRead
	}

	if detail != "" {
		w.opaque(OpacityStartupFile, label, detail)

		return startupScript{}, false
	}

	text, status, detail := w.scriptSource(path, cmd)

	switch status {
	case ScriptText:
		key := label + "\x00" + clean
		if w.expanding[startupPrefix+key+"\x00"+text] {
			return startupScript{}, false
		}

		return startupScript{label: label, key: key, text: text}, true
	case ScriptOpaque:
		w.opaque(OpacityStartupFile, label, detail)
	case ScriptMissing, ScriptBinary:
	}

	return startupScript{}, false
}

// startupPath expands the variables a startup file's path names, as the
// shell does when it starts, and says why the path cannot be known.
func (w *astWalker) startupPath(v startupValue) (path, detail string) {
	if v.deferred {
		return "", DetailStartupExpansion
	}

	if v.literal && strings.ContainsAny(v.value, "$`") {
		return "", DetailScriptVariable
	}

	if v.literal {
		return v.value, ""
	}

	unknown := false

	path = expandVars(v.value, func(name string) (string, bool) {
		if w.inLoop || w.state.namesUnknown || w.state.dynamicVars[name] || w.unknownVars[name] ||
			(movedDir[name] && (w.currentDir != "" || w.dirUnknown)) {
			unknown = true

			return "", false
		}

		if value, ok := w.assignments[name]; ok {
			return value, true
		}

		return w.resolver.LookupEnv(name)
	})

	switch {
	case unknown || HasUnresolvedVars(path) || (strings.HasPrefix(path, "~") && w.homeChanged()):
		return "", DetailScriptVariable
	case strings.ContainsAny(path, "$`") || marked(path) || bracesExpand(path):
		return "", DetailStartupExpansion
	default:
		return path, ""
	}
}

// walkPrelude walks the startup files in the shell the walker stands for, so
// what they define is in place for the script that follows. It reports
// whether only lenient files moved HOME or ZDOTDIR.
func (w *astWalker) walkPrelude(prelude []startupScript, parent Command) bool {
	moved := homeMove{}

	for _, part := range prelude {
		if part.logout {
			continue
		}

		parts := []startupScript{part}
		if part.lazy {
			parts = w.lazyStartup(part, parent, moved.lenient)
		}

		for _, part := range parts {
			if part.key == "" {
				continue
			}

			before := w.currentHomeState()

			w.walkStartupPart(part)

			if w.currentHomeState() != before {
				moved.note(part.lenient)
			}
		}
	}

	return moved.lenient
}

// homeMove records whether the startup files that moved HOME or ZDOTDIR
// were all lenient ones.
type homeMove struct {
	lenient bool
	strict  bool
}

// note records one startup file that moved HOME or ZDOTDIR.
func (m *homeMove) note(lenient bool) {
	m.strict = m.strict || !lenient
	m.lenient = lenient && !m.strict
}

// walkStartupPart walks one startup file. What runs after it runs after
// every assignment in it, so positions noted in it are dropped.
func (w *astWalker) walkStartupPart(part startupScript) {
	if part.lenient && !w.lenient {
		defer w.enterLenient()()
	}

	w.expanding[startupPrefix+part.key+"\x00"+part.text] = true
	w.expanding[startupPrefix+devStdin] = w.expanding[startupPrefix+devStdin] ||
		strings.HasSuffix(part.key, "\x00"+devStdin)
	w.via = append(w.via, part.label)

	for stmt, err := range syntax.NewParser().StmtsSeq(strings.NewReader(part.text)) {
		if err != nil {
			w.opaque(OpacityScriptSyntax, part.label, "")

			break
		}

		w.walkStmt(stmt)
	}

	clear(w.startupPending)

	w.via = w.via[:len(w.via)-1]
}

// anyStartupVar names every startup variable, for a loop that may set any.
var anyStartupVar = strings.Join([]string{bashEnvVar, envVar, homeVar, zdotdirVar}, " ")

// noteLoopStartup records a loop that may set a startup variable on a later
// pass: one naming it, or running source, eval or a same-line definition
// that may set it in text the loop does not show.
func (w *astWalker) noteLoopStartup(node syntax.Node, param bool) {
	for _, name := range w.loopStartupNames(node, param, make(map[string]bool)) {
		if w.loopStartup == nil {
			w.loopStartup = make(map[string]bool)
		}

		w.loopStartup[name] = true
	}
}

// seedStartup gives a script the startup variables its command was started
// with, which it inherits in its environment.
func (w *astWalker) seedStartup(parent Command) {
	for name, v := range parent.startup {
		if v.unset {
			delete(w.assignments, name)

			if w.startupUnset == nil {
				w.startupUnset = make(map[string]bool)
			}

			w.startupUnset[name] = true

			continue
		}

		if v.dynamic {
			w.unknownVars[name] = true

			continue
		}

		w.assignments[name] = v.value
		delete(w.unknownVars, name)

		if v.deferred {
			if w.startupDeferred == nil {
				w.startupDeferred = make(map[string]bool)
			}

			w.startupDeferred[name] = true
		}
	}
}

// noteUnset marks the startup variables a plain unset clears, unless their
// value was already unknown: the unset leaves the earlier value or none.
func (w *astWalker) noteUnset(cmd Command, wasUnknown map[string]bool) {
	if cmd.Name != unsetBuiltin {
		return
	}

	for name := range startupVars {
		if slices.Contains(cmd.Args, name) && !wasUnknown[name] {
			if w.startupUnset == nil {
				w.startupUnset = make(map[string]bool)
			}

			w.startupUnset[name] = true
		}
	}
}

// distrustNames stops trusting any variable after a write whose target
// klaudiush cannot name, which may be a startup variable too.
func (w *astWalker) distrustNames() {
	w.state.untrusted = true
	w.state.namesUnknown = true
}

// OpacityStartupFile means BASH_ENV, ENV or --rcfile names a file a new
// shell runs first, and the parser cannot tell which file or read it.
const OpacityStartupFile OpacityCause = "startup-file"

// Fixed reasons, set as Opacity.Detail, that a startup file is opaque, on
// top of the script reasons for its path and content.
const (
	DetailStartupValue     = "its value comes from command output or a write klaudiush cannot follow"
	DetailStartupExpansion = "the shell expands its value when it starts, which may run commands"
	DetailLoginAccount     = "the target account's home directory and shell cannot be resolved"
)

// noteStartupPending remembers where the value of a startup variable
// assigned from command output ends: the commands inside it run before the
// variable changes.
func (w *astWalker) noteStartupPending(assign *syntax.Assign) {
	if !startupVars[assign.Name.Value] || assign.Value == nil {
		return
	}

	if w.startupPending == nil {
		w.startupPending = make(map[string]syntax.Pos)
	}

	w.startupPending[assign.Name.Value] = assign.Value.End()
}

// runsBefore reports whether a command at loc sits before end on the line.
func runsBefore(loc Location, end syntax.Pos) bool {
	if loc.Line != end.Line() {
		return loc.Line < end.Line()
	}

	return loc.Column < end.Col()
}

// noteKeywordMode stops trusting names after set -k, which makes any
// NAME=value argument of a later command an assignment for it.
func (w *astWalker) noteKeywordMode(args []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions || arg == "-" ||
			(!strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "+")):
			return
		case arg == setOption:
			if i+1 < len(args) && args[i+1] == "keyword" {
				w.distrustNames()

				return
			}

			i++
		case arg == "+o":
			i++
		case strings.HasPrefix(arg, "-") && strings.Contains(arg[1:], "k"):
			w.distrustNames()

			return
		}
	}
}
