package parser

import (
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	homeVar    = "HOME"
	zdotdirVar = "ZDOTDIR"
	zshShell   = "zsh"
	zshenvFile = ".zshenv"
	profile    = ".profile"
	yashShell  = "yash"
	emulateOpt = "--emulate"
	execArgv0  = "-a"
	fishShell  = "fish"
	loginOpt   = "login"
	shShell    = "sh"
	kshShell   = "ksh"
	xonshShell = "xonsh"
)

// zshFiles are the startup files zsh reads from ZDOTDIR, HOME when unset.
var zshFiles = nameSet(zshenvFile + " .zprofile .zshrc .zlogin .zlogout")

// logoutFiles run when a login shell exits, after its script.
var logoutFiles = nameSet(".zlogout .bash_logout .logout")

// cshShells read .cshrc on every start unless given -f.
var cshShells = nameSet("csh tcsh")

// foreignStartupFiles use shell syntaxes the bash parser cannot interpret.
var foreignStartupFiles = nameSet(`.tcshrc .cshrc .login .logout config.fish
	env.nu config.nu login.nu rc.elv .xonshrc rc.xsh xonshrc`)

// shellMode is how its options make a shell start: posix is bash --posix,
// which reads ENV alone, noRCs is zsh or csh -f or NO_RCS, which reads no
// home file, and emulate is the shell zsh --emulate makes it act as.
type shellMode struct {
	login       bool
	interactive bool
	posix       bool
	noRC        bool
	noProfile   bool
	noRCs       bool
	emulate     string
}

// shellOptions reads the options a shell named name is given before its
// first operand. Anything after it, -c's command included, is an operand. A
// word klaudiush cannot resolve that is followed by an option may itself be
// -l or -i, so the shell is taken as both; one followed by none is the
// script it runs.
func shellOptions(name string, args []string) shellMode {
	var mode shellMode

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions || arg == "-" || arg == "+":
			return mode
		case mayBeDynamic(arg) && !literalLead.MatchString(arg) &&
			i+1 < len(args) && strings.HasPrefix(args[i+1], "-"):
			mode.login, mode.interactive = true, true

			return mode
		case arg == emulateOpt:
			if i+1 < len(args) {
				mode.emulate = args[i+1]
			}

			i++
		case arg == rcfileLabel || arg == "--init-file":
			i++
		case strings.HasPrefix(arg, "--"):
			mode.longOption(name, arg[2:])
		case strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+"):
			if takesOptionName(arg) {
				if i+1 < len(args) && strings.Contains(arg[1:], "o") {
					mode.namedOption(name, args[i+1], arg[0] == '-')
				}

				i++
			}

			mode.flags(name, arg[1:], arg[0] == '-')
		default:
			return mode
		}
	}

	return mode
}

// optionWords expands the variables in a shell's arguments that the line
// sets literally, so a -l held in a variable still makes a login shell.
func (w *astWalker) optionWords(args []string) []string {
	words := make([]string, len(args))
	for i, arg := range args {
		words[i] = w.expandName(arg)
	}

	return words
}

// literalLead matches a word that starts with literal text other than an
// option dash, which no expansion after it can turn into an option.
var literalLead = regexp.MustCompile(`^[A-Za-z0-9_./~+:@%,=]`)

// takesOptionName reports an option cluster whose -o or -O takes the next
// argument as an option name: -o, -eo, +O.
func takesOptionName(arg string) bool {
	return strings.ContainsAny(arg[1:], "oO")
}

// flags applies the single-letter options of a cluster. Only -f turns off
// with +f: turning login or interactive off reads fewer files, which a
// misread option must never cause.
func (m *shellMode) flags(name, cluster string, on bool) {
	for _, flag := range cluster {
		switch {
		case flag == 'l' && on:
			m.login = true
		case flag == 'i' && on:
			m.interactive = true
		case flag == 'f' && (name == zshShell || cshShells[name]):
			m.noRCs = on
		case flag == 'N' && name == fishShell && on:
			m.noRC = true
		}
	}
}

// longOption applies a --name option. zsh takes any option name that way.
func (m *shellMode) longOption(name, option string) {
	switch option {
	case loginOpt:
		m.login = true
	case "interactive":
		m.interactive = true
	case "posix":
		m.posix = bashShells[name]
	case "norc":
		m.noRC = true
	case "no-rc":
		m.noRC = name == xonshShell
	case "no-config", "no-config-file":
		m.noRC = true
	case "noprofile":
		m.noProfile = true
	default:
		if name == zshShell {
			m.namedOption(name, strings.ReplaceAll(option, "-", ""), true)
		}
	}
}

// namedOption applies -o name or +o name. zsh ignores case and
// underscores, and a "no" prefix negates the option.
func (m *shellMode) namedOption(name, option string, on bool) {
	option = strings.ToLower(strings.ReplaceAll(option, "_", ""))
	if name == zshShell && strings.HasPrefix(option, "no") {
		option, on = option[2:], !on
	}

	switch option {
	case loginOpt:
		m.login = m.login || on
	case "interactive":
		m.interactive = m.interactive || on
	case "posix":
		if bashShells[name] {
			m.posix = on
		}
	case "rcs":
		if name == zshShell {
			m.noRCs = !on
		}
	}
}

// homeStartupFiles returns the startup files a shell reads from its home
// directory (ZDOTDIR for zsh): before runs ahead of BASH_ENV, ENV and
// --rcfile, after behind them, logout files included since a login shell
// runs them when it exits. A login bash reads the first of its profiles
// that exists; all are returned, since one may be removed on the line.
func homeStartupFiles(name string, mode shellMode, rcfile bool) (before, after []string) {
	if name == zshShell && mode.emulate != "" {
		name = zshEmulation(mode.emulate)
	}

	if name == fishShell || name == "nu" || name == "elvish" || name == xonshShell {
		return foreignHomeStartupFiles(name, mode)
	}

	switch name {
	case zshShell:
		return zshStartupFiles(mode)
	case "bash", "rbash":
		switch {
		case mode.posix:
		case mode.login:
			if !mode.noProfile {
				before = []string{".bash_profile", ".bash_login", profile}
			}

			after = []string{".bash_logout"}
		case mode.interactive && !mode.noRC && !rcfile:
			after = []string{".bashrc"}
		}
	case shShell, "dash", "ash", "posh":
		if mode.login {
			before = []string{profile}
		}
	case kshShell, "mksh", "oksh", "loksh", yashShell:
		before, after = kshStartupFiles(name, mode)
	case "csh", "tcsh":
		before, after = cshStartupFiles(mode)
	}

	return before, after
}

func foreignHomeStartupFiles(name string, mode shellMode) (before, after []string) {
	if mode.noRC {
		return nil, nil
	}

	switch name {
	case fishShell:
		before = []string{".config/fish/config.fish"}
	case "nu":
		before = []string{".config/nushell/env.nu", ".config/nushell/config.nu"}
		if mode.login {
			before = append(before, ".config/nushell/login.nu")
		}
	case "elvish":
		before = []string{".config/elvish/rc.elv"}
	case xonshShell:
		before = []string{".config/xonsh/rc.xsh"}
		if mode.interactive {
			before = append(before, ".xonshrc")
		}
	}

	return before, nil
}

// zshEmulation returns the shell whose startup files zsh --emulate reads:
// sh and ksh emulation read .profile and ENV instead of the z-files.
func zshEmulation(emulate string) string {
	switch emulate {
	case shShell, kshShell:
		return emulate
	default:
		return zshShell
	}
}

// zshStartupFiles returns the files zsh reads from ZDOTDIR, in order.
func zshStartupFiles(mode shellMode) (before, after []string) {
	if mode.noRCs {
		return nil, nil
	}

	before = []string{zshenvFile}
	if mode.login {
		before = append(before, ".zprofile")
	}

	if mode.interactive {
		after = append(after, ".zshrc")
	}

	if mode.login {
		after = append(after, ".zlogin", ".zlogout")
	}

	return before, after
}

// kshStartupFiles returns the home startup files of ksh, mksh and yash:
// .profile for a login shell, and the rc file an interactive one reads when
// ENV is unset.
func kshStartupFiles(name string, mode shellMode) (before, after []string) {
	if mode.login {
		before = []string{profile}
		if name == yashShell {
			before = []string{".yash_profile", profile}
		}
	}

	if mode.interactive {
		after = []string{".kshrc", ".mkshrc"}
		if name == yashShell {
			after = []string{".yashrc"}
		}
	}

	return before, after
}

// cshStartupFiles returns the files csh and tcsh read: .tcshrc or .cshrc on
// every start, then .login, and .logout on exit, for a login shell.
func cshStartupFiles(mode shellMode) (before, after []string) {
	if mode.noRCs {
		return nil, nil
	}

	before = []string{".tcshrc", ".cshrc"}
	if mode.login {
		before = append(before, ".login")
		after = []string{".logout"}
	}

	return before, after
}

// homeScripts reads the startup files a shell started by cmd reads from its
// home directory. Unless now, a zsh file after .zshenv and a logout file
// only get a placeholder: the shell reads them once the files before them,
// or its script, have run, which may move ZDOTDIR, cd or write them.
func (w *astWalker) homeScripts(cmd Command, files []string, now bool) []startupScript {
	var scripts []startupScript

	for _, file := range files {
		zdot := zshFiles[file]

		dirs, ok := w.homeDirs(cmd, zdot)
		if !ok {
			continue
		}

		if !now && ((zdot && file != zshenvFile) || logoutFiles[file]) {
			scripts = append(scripts, startupScript{
				label: file, lazy: true, logout: logoutFiles[file],
			})

			continue
		}

		for _, dir := range dirs {
			if script, found := w.homeScript(cmd, file, dir); found {
				scripts = append(scripts, script)
			}
		}
	}

	return scripts
}

// homeDirs returns the directories a shell started by cmd reads its startup
// files from, recording why when they cannot be known. A ZDOTDIR assigned on
// the line may not be exported, so zsh may read HOME's files instead.
func (w *astWalker) homeDirs(cmd Command, zdot bool) ([]homeDir, bool) {
	home, ok := w.startupDir(cmd, homeVar)
	if !ok {
		return nil, false
	}

	var dirs []homeDir

	if zdot {
		dir, known := w.startupDir(cmd, zdotdirVar)
		if !known {
			return nil, false
		}

		_, prefixed := cmd.startup[zdotdirVar]
		_, assigned := w.assignments[zdotdirVar]

		if dir.set {
			dirs = append(dirs, dir)
		}

		if dir.set && (prefixed || (!assigned && !w.startupUnset[zdotdirVar])) {
			return dirs, true
		}
	}

	if home.set && !slices.ContainsFunc(dirs, func(d homeDir) bool { return d.path == home.path }) {
		dirs = append(dirs, home)
	}

	return dirs, true
}

// homeDir is a directory a shell reads startup files from. inherited marks
// one taken from the environment klaudiush runs in rather than the line.
type homeDir struct {
	path      string
	set       bool
	inherited bool
}

// startupDir returns the directory a startup directory variable holds for
// cmd, and whether it is set. A shell whose HOME is unset (unset, env -u,
// env -i) takes it from the user database, which klaudiush does not read.
func (w *astWalker) startupDir(cmd Command, name string) (homeDir, bool) {
	inherited := func() (homeDir, bool) {
		path, set := w.resolver.LookupEnv(name)

		return homeDir{path: path, set: set, inherited: true}, true
	}

	if v, ok := cmd.startup[name]; ok && v.unset {
		if name != homeVar {
			return homeDir{}, true
		}

		w.opaque(OpacityStartupFile, name, DetailStartupValue)

		return homeDir{}, false
	}

	v, set := w.startupSetting(cmd, name)
	_, prefixed := cmd.startup[name]

	switch {
	case name == homeVar && w.startupUnset[name] && !prefixed:
		w.opaque(OpacityStartupFile, name, DetailStartupValue)

		return homeDir{}, false
	case !set:
		return inherited()
	case v.dynamic:
		w.opaque(OpacityStartupFile, name, DetailStartupValue)

		return homeDir{}, false
	}

	dir, detail := w.startupPath(v)
	if detail == "" && strings.HasPrefix(dir, "~") {
		dir = ExpandHome(dir, w.resolver)
		if strings.HasPrefix(dir, "~") {
			detail = DetailScriptVariable
		}
	}

	if detail != "" {
		w.opaque(OpacityStartupFile, name, detail)

		return homeDir{}, false
	}

	return homeDir{path: dir, set: true}, true
}

// homeScript reads the startup file named file in dir, as the shell joins
// them, recording why when it cannot. A file the line may have changed is
// checked strictly; one in an inherited directory that the line never
// touched is lenient, and skipped when it cannot be read.
func (w *astWalker) homeScript(cmd Command, file string, dir homeDir) (startupScript, bool) {
	path := dir.path + "/" + file
	clean := resolvePath(cmd.WorkingDirectory, path)
	relative := !filepath.IsAbs(path)
	touched := w.homeTouched(clean, dir.path, cmd)

	if dir.inherited && !touched && !w.lenient {
		defer w.enterLenient()()
	}

	detail := ""

	switch {
	case relative && (cmd.DirUnknown || w.dirUnknown || cmd.DirComputed || w.dirComputed):
		detail = DetailScriptDirectory
	case specialPath(clean) || (!filepath.IsAbs(clean) && specialPath(filepath.Join("/", clean))):
		detail = DetailScriptRead
	}

	if detail != "" {
		w.opaque(OpacityStartupFile, file, detail)

		return startupScript{}, false
	}

	return w.startupPathScript(file, clean, touched, dir.inherited && !touched)
}

// startupPathScript reads a resolved startup path. Unsupported syntaxes are
// opaque only when the command line touched them; untouched files are lenient.
func (w *astWalker) startupPathScript(
	label string,
	path string,
	touched bool,
	lenient bool,
) (startupScript, bool) {
	text, status, detail := w.homeFileSource(path)
	if foreignStartupFiles[filepath.Base(path)] {
		if status != ScriptMissing && !lenient {
			w.opaque(OpacityScriptSyntax, filepath.Base(label), "")
		}

		return startupScript{}, false
	}

	switch status {
	case ScriptText:
		key := label + "\x00" + path
		if w.expanding[startupPrefix+key+"\x00"+text] {
			return startupScript{}, false
		}

		return startupScript{
			label: label, key: key, text: text,
			touched: touched, lenient: lenient,
		}, true
	case ScriptOpaque:
		w.opaque(OpacityStartupFile, label, detail)
	case ScriptMissing, ScriptBinary:
	}

	return startupScript{}, false
}

// systemScripts reads fixed system startup files leniently until the line
// touches them, matching inherited home startup files.
func (w *astWalker) systemScripts(cmd Command, files []string) []startupScript {
	var scripts []startupScript

	for _, path := range files {
		if script, found := w.systemScript(cmd, path); found {
			scripts = append(scripts, script)
		}
	}

	return scripts
}

func (w *astWalker) systemScript(cmd Command, path string) (startupScript, bool) {
	touched := w.homeTouched(path, filepath.Dir(path), cmd)
	if !touched && !w.lenient {
		defer w.enterLenient()()
	}

	return w.startupPathScript(filepath.Base(path), path, touched, !touched)
}

// checkFishConfScripts finds same-line writes to fish conf.d files. Untouched
// foreign-syntax files need no directory enumeration because they are lenient.
func (w *astWalker) checkFishConfScripts(cmd Command) {
	dirs, ok := w.homeDirs(cmd, false)
	if !ok {
		return
	}

	seen := make(map[string]bool)

	for _, dir := range dirs {
		confDir := resolvePath(cmd.WorkingDirectory, dir.path+"/.config/fish/conf.d")

		for p := w; p != nil; p = p.parent {
			for _, fw := range p.fileWrites {
				path, known := w.writtenPath(fw)
				if !known || filepath.Dir(path) != confDir ||
					!strings.HasSuffix(filepath.Base(path), ".fish") || seen[path] {
					continue
				}

				seen[path] = true
				if _, status, _ := w.homeFileSource(path); status != ScriptMissing {
					w.opaque(OpacityScriptSyntax, filepath.Base(path), "")
				}
			}
		}
	}
}

// homeFileSource returns the text of a home startup file: the content
// written to it earlier on the line, or the file on disk. A write whose
// target may be the file but cannot be resolved makes it opaque.
func (w *astWalker) homeFileSource(target string) (string, ScriptStatus, string) {
	text, found, captured, unsure := w.homeWrite(target)

	switch {
	case unsure || (found && !captured):
		return "", ScriptOpaque, DetailScriptWritten
	case found:
		return text, ScriptText, ""
	}

	if stored, ok := w.scriptFiles[target]; ok {
		return stored, ScriptText, ""
	}

	text, status := w.resolver.ReadScript(target)
	if status == ScriptOpaque {
		return "", ScriptOpaque, DetailScriptRead
	}

	return text, status, ""
}

// homeWrite finds the last write on the line to target, matching each
// write's path as it resolved when the write ran: ~, $HOME and a cd to ~
// all name the home directory. unsure reports a write to a file of the same
// name under a directory klaudiush cannot resolve, a relative one after a
// cd to a computed directory included.
func (w *astWalker) homeWrite(target string) (content string, found, captured, unsure bool) {
	base := filepath.Base(target)

	for p := w; p != nil; p = p.parent {
		var writes []FileWrite

		for _, fw := range p.fileWrites {
			path, known := w.writtenPath(fw)
			if p.dirComputed && !filepath.IsAbs(fw.Path) && !strings.HasPrefix(fw.Path, "~") {
				known = false
			}

			switch {
			case !known:
				unsure = unsure || filepath.Base(fw.Path) == base
			case path == target:
				fw.Path, fw.WorkingDirectory = path, ""
				writes = append(writes, fw)
			}
		}

		if content, found, captured = lastWrite(writes, target, nil); found {
			return content, found, captured, unsure
		}
	}

	return "", false, false, unsure
}

// writtenPath resolves the path a write went to, with the variables as they
// stood then.
func (w *astWalker) writtenPath(fw FileWrite) (string, bool) {
	if fw.Dynamic {
		return "", false
	}

	dir, ok := w.writtenWord(fw, fw.WorkingDirectory)
	if !ok {
		return "", false
	}

	path, ok := w.writtenWord(fw, fw.Path)
	if !ok || (!filepath.IsAbs(path) && fw.DirUnknown) {
		return "", false
	}

	return resolvePath(dir, path), true
}

// writtenWord expands the variables and leading ~ of a write's path or
// directory.
func (w *astWalker) writtenWord(fw FileWrite, word string) (string, bool) {
	known := true

	lookup := func(name string) (string, bool) {
		if fw.Vars.IsDynamic(name) || fw.Vars.unknownName(name) {
			known = false

			return "", false
		}

		if fw.Vars != nil {
			if value, ok := fw.Vars.Assignments[name]; ok {
				return value, true
			}
		}

		return w.resolver.LookupEnv(name)
	}

	word = expandVars(word, lookup)
	if !known || HasUnresolvedVars(word) || marked(word) {
		return "", false
	}

	if !strings.HasPrefix(word, "~") {
		return word, true
	}

	home, ok := lookup(homeVar)
	if !ok || !known || (word != "~" && !strings.HasPrefix(word, "~/")) {
		return "", false
	}

	return home + word[1:], true
}

// shebangStartup adds the home startup files of the shell a script's
// shebang names, for a script the program runs rather than sources:
// #!/bin/zsh reads .zshenv, #!/bin/bash -l the login profiles.
func (w *astWalker) shebangStartup(
	cmd Command,
	file scriptFile,
	text string,
	startup []startupScript,
) []startupScript {
	if shells[cmd.Name] || sameShellRunners[cmd.Name] || file.interpreter {
		return startup
	}

	name, launcherArgs, args, ok := shebangShell(text)
	if !ok {
		return startup
	}

	before, after := homeStartupFiles(name, shellOptions(name, args), false)
	if len(before)+len(after) == 0 {
		return startup
	}

	cmd = withEnvUnset(cmd, launcherArgs)
	cmd = withEnvOperands(cmd, Command{}, launcherArgs)

	scripts := w.homeScripts(cmd, before, false)
	scripts = append(scripts, startup...)

	return append(scripts, w.homeScripts(cmd, after, false)...)
}

// shebangShell returns the shell a script's shebang runs, the words before
// it (env's options and NAME=value operands) and the words after it.
func shebangShell(text string) (name string, launcher, args []string, ok bool) {
	line, _, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(line, "#!") {
		return "", nil, nil, false
	}

	words := strings.Fields(strings.TrimPrefix(line, "#!"))
	for i, word := range words {
		if name := commandName(word); shells[name] {
			return name, words[min(1, i):i], words[i+1:], true
		}
	}

	return "", nil, nil, false
}

// homeState is what decides the directory zsh reads its later startup
// files from.
type homeState struct {
	home, zdot       string
	homeSet, zdotSet bool
	unknown          bool
}

// currentHomeState returns the HOME and ZDOTDIR the walker sees now.
func (w *astWalker) currentHomeState() homeState {
	home, homeSet := w.assignments[homeVar]
	zdot, zdotSet := w.assignments[zdotdirVar]

	return homeState{
		home: home, zdot: zdot, homeSet: homeSet, zdotSet: zdotSet,
		unknown: w.unknownVars[homeVar] || w.unknownVars[zdotdirVar] ||
			w.state.dynamicVars[homeVar] || w.state.dynamicVars[zdotdirVar] ||
			w.state.namesUnknown,
	}
}

// lazyStartup reads a startup file the shell reaches after others have run,
// from where HOME and ZDOTDIR point now. When only lenient files moved them,
// the file stays lenient unless the line touched it.
func (w *astWalker) lazyStartup(part startupScript, parent Command, lenient bool) []startupScript {
	parent.startup = nil
	parent.WorkingDirectory = w.currentDir
	parent.DirUnknown, parent.DirComputed = w.dirUnknown, w.dirComputed

	if !lenient || w.lenient {
		return w.homeScripts(parent, []string{part.label}, true)
	}

	defer w.enterLenient()()

	scripts := w.homeScripts(parent, []string{part.label}, true)
	for i := range scripts {
		scripts[i].lenient = !scripts[i].touched
	}

	return scripts
}

// walkEpilogue walks the logout files a login shell runs when its script
// ends, read with the directory, variables and writes the script left.
func (w *astWalker) walkEpilogue(prelude []startupScript, parent Command, lenient bool) {
	for _, part := range prelude {
		if !part.logout {
			continue
		}

		for _, script := range w.lazyStartup(part, parent, lenient) {
			if script.key != "" {
				w.walkStartupPart(script)
			}
		}
	}
}

// enterLenient stops recording opacities and switches to the lenient work
// budget, which the line's own does not share, until the returned function
// runs.
func (w *astWalker) enterLenient() func() {
	work := w.state.work
	w.state.work = w.state.lenientWork
	w.lenient = true

	return func() {
		w.state.lenientWork = max(w.state.work, 0)
		w.state.work = work
		w.lenient = false
	}
}

// assignsDefault reports ${X:=word} or ${X=word}, which sets X.
func assignsDefault(pe *syntax.ParamExp) bool {
	return pe.Exp != nil &&
		(pe.Exp.Op == syntax.AssignUnset || pe.Exp.Op == syntax.AssignUnsetOrNull)
}

// withEnvUnset marks the startup variables env removes with -u or --unset,
// or with -i, which starts the command with no environment at all.
func withEnvUnset(child Command, options []string) Command {
	unset := func(name string) {
		if !startupVars[name] {
			return
		}

		vars := maps.Clone(child.startup)
		if vars == nil {
			vars = make(map[string]startupValue)
		}

		vars[name] = startupValue{unset: true}
		child.startup = vars
	}

	for i := 0; i < len(options); i++ {
		arg := options[i]

		switch {
		case arg == "-i" || arg == "--ignore-environment" || arg == "-":
			for name := range startupVars {
				unset(name)
			}
		case arg == "-u" || arg == "--unset":
			if i+1 < len(options) {
				unset(options[i+1])
			}

			i++
		case strings.HasPrefix(arg, "--unset="):
			unset(strings.TrimPrefix(arg, "--unset="))
		case strings.HasPrefix(arg, "-u"):
			unset(arg[2:])
		}
	}

	return child
}

// execLogin reports exec -l, or exec -a with a name starting with -, which
// starts the shell as a login shell. A name klaudiush cannot read counts too.
func execLogin(options []string) bool {
	for i := 0; i < len(options); i++ {
		arg := options[i]

		switch {
		case arg == execArgv0:
			if i+1 < len(options) {
				name := options[i+1]
				if strings.HasPrefix(name, "-") || marked(name) || HasUnresolvedVars(name) {
					return true
				}
			}

			i++
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.Contains(arg, "l"):
			return true
		}
	}

	return false
}

// fileReaders only read the files they name, so naming a startup file does
// not change it.
var fileReaders = nameSet(`ack ag bat cat diff egrep fgrep file grep head jq less ls
	md5sum more rg sha256sum shasum sort stat tail uniq wc yq`)

// homeTouched reports whether the line may have changed a home startup file
// before the shell reads it: a write to it, a write klaudiush cannot place
// or resolve, a copy into a directory above it, or a command other than a
// reader that names the file, the directory it is in, or a path it cannot
// resolve (rsync, a tool the write tracking does not know). The shell's own
// command and the launchers in front of it run nothing before it starts.
func (w *astWalker) homeTouched(target, dir string, cmd Command) bool {
	if _, found, _, unsure := w.homeWrite(target); found || unsure {
		return true
	}

	if w.unplacedWriteBefore(cmd, target) || w.lineWriteAbove(target) || w.unresolvedWrite() {
		return true
	}

	name := filepath.Base(target)
	home := resolvePath(cmd.WorkingDirectory, dir)

	for earlier := range w.earlierCommands() {
		if shellBuiltins[earlier.Name] || fileReaders[earlier.Name] ||
			earlier.Location == cmd.Location {
			continue
		}

		if slices.ContainsFunc(earlier.Args, func(arg string) bool {
			return w.argTouches(earlier, arg, name, home)
		}) {
			return true
		}
	}

	return false
}

// unresolvedWrite reports a write on the line whose path holds a variable
// klaudiush cannot resolve, which may be any startup file.
func (w *astWalker) unresolvedWrite() bool {
	for p := w; p != nil; p = p.parent {
		for _, fw := range p.fileWrites {
			if _, known := w.writtenPath(fw); !known && !fw.TargetUnknown {
				return true
			}
		}
	}

	return false
}

// argTouches reports whether an argument names a startup file, the
// directory it is in, or a path klaudiush cannot resolve. git and gh are
// validated on their own, so their unresolved arguments do not count.
func (w *astWalker) argTouches(cmd Command, arg, name, dir string) bool {
	switch {
	case strings.Contains(arg, name):
		return true
	case marked(arg) || HasUnresolvedVars(w.expandName(arg)):
		return cmd.Name != gitProgram && cmd.Name != ghCLI
	}

	path := ExpandHome(w.expandName(arg), w.resolver)

	return resolvePath(cmd.WorkingDirectory, path) == dir
}

// systemStartupFiles returns fixed startup files for common Unix layouts.
func systemStartupFiles(name string, mode shellMode, rcfile bool) (before, after []string) {
	switch name {
	case zshShell:
		before = []string{"/etc/zshenv"}
		if mode.login {
			before = append(before, "/etc/zprofile")
			after = append(after, "/etc/zlogin", "/etc/zlogout")
		}

		if mode.interactive {
			after = append([]string{"/etc/zshrc"}, after...)
		}
	case "bash", "rbash":
		switch {
		case mode.posix:
		case mode.login && !mode.noProfile:
			before = []string{"/etc/profile"}
		case mode.interactive && !mode.noRC && !rcfile:
			after = []string{"/etc/bash.bashrc"}
		}
	case shShell, "dash", "ash", "posh", kshShell, "mksh", "oksh", "loksh", yashShell:
		if mode.login {
			before = []string{"/etc/profile"}
		}
	case "csh", "tcsh":
		if !mode.noRCs {
			before = []string{"/etc/csh.cshrc"}
			if mode.login {
				before = append(before, "/etc/csh.login")
			}
		}
	case fishShell:
		if !mode.noRC {
			before = []string{"/etc/fish/config.fish"}
		}
	case xonshShell:
		if !mode.noRC {
			before = []string{"/etc/xonsh/xonshrc"}
		}
	}

	return before, after
}

// loginProgram reports login and the login forms of su.
func loginProgram(name string, args []string) bool {
	if name == loginOpt {
		return true
	}

	if name != "su" {
		return false
	}

	return slices.ContainsFunc(args, func(arg string) bool {
		return arg == "-" || arg == "--login" ||
			strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
				strings.Contains(arg[1:], "l")
	})
}

// loginShell names the shell a login program starts when it is visible.
func loginShell(resolver Resolver) string {
	if value, ok := resolver.LookupEnv("SHELL"); ok {
		if name := commandName(value); shells[name] {
			return name
		}
	}

	return shShell
}
