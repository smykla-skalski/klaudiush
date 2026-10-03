package parser

import (
	"maps"
	"path/filepath"
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
)

// zshFiles are the startup files zsh reads from ZDOTDIR, HOME when unset.
var zshFiles = nameSet(zshenvFile + " .zprofile .zshrc .zlogin .zlogout")

// cshShells read .cshrc on every start unless given -f.
var cshShells = nameSet("csh tcsh")

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
// first operand. Anything after it, -c's command included, is an operand.
func shellOptions(name string, args []string) shellMode {
	var mode shellMode

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions || arg == "-" || arg == "+":
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
		}
	}
}

// longOption applies a --name option. zsh takes any option name that way.
func (m *shellMode) longOption(name, option string) {
	switch option {
	case "login":
		m.login = true
	case "interactive":
		m.interactive = true
	case "posix":
		m.posix = bashShells[name]
	case "norc":
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
	case "login":
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
	case "sh", "dash", "ash", "posh":
		if mode.login {
			before = []string{profile}
		}
	case "ksh", "mksh", "oksh", "loksh", yashShell:
		before, after = kshStartupFiles(name, mode)
	case "csh", "tcsh":
		before, after = cshStartupFiles(mode)
	}

	return before, after
}

// zshEmulation returns the shell whose startup files zsh --emulate reads:
// sh and ksh emulation read .profile and ENV instead of the z-files.
func zshEmulation(emulate string) string {
	switch emulate {
	case "sh", "ksh":
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
// home directory. A zsh file after .zshenv is preceded by a placeholder, so
// it is read again from the directory .zshenv may have moved ZDOTDIR to.
func (w *astWalker) homeScripts(cmd Command, files []string) []startupScript {
	var scripts []startupScript

	for _, file := range files {
		zdot := zshFiles[file]
		lazy := zdot && file != zshenvFile

		dirs, ok := w.homeDirs(cmd, zdot)
		if !ok {
			continue
		}

		if lazy {
			scripts = append(scripts, startupScript{label: file, lazy: true})
		}

		for _, dir := range dirs {
			if script, found := w.homeScript(cmd, file, dir); found {
				script.lazy = lazy
				scripts = append(scripts, script)
			}
		}
	}

	return scripts
}

// homeDirs returns the directories a shell started by cmd reads its startup
// files from, recording why when they cannot be known. A ZDOTDIR assigned on
// the line may not be exported, so zsh may read HOME's files instead.
func (w *astWalker) homeDirs(cmd Command, zdot bool) ([]string, bool) {
	home, homeSet, ok := w.startupDir(cmd, homeVar)
	if !ok {
		return nil, false
	}

	var dirs []string

	if zdot {
		dir, set, known := w.startupDir(cmd, zdotdirVar)
		if !known {
			return nil, false
		}

		_, prefixed := cmd.startup[zdotdirVar]
		_, assigned := w.assignments[zdotdirVar]

		if set {
			dirs = append(dirs, dir)
		}

		if set && (prefixed || (!assigned && !w.startupUnset[zdotdirVar])) {
			return dirs, true
		}
	}

	if homeSet && !slices.Contains(dirs, home) {
		dirs = append(dirs, home)
	}

	return dirs, true
}

// startupDir returns the directory a startup directory variable holds for
// cmd, and whether it is set. A shell whose HOME is unset takes it from the
// user database, which klaudiush does not read.
func (w *astWalker) startupDir(cmd Command, name string) (dir string, set, known bool) {
	if v, ok := cmd.startup[name]; ok && v.unset {
		if name != homeVar {
			return "", false, true
		}

		dir, set = w.resolver.LookupEnv(name)

		return dir, set, true
	}

	v, set := w.startupSetting(cmd, name)
	_, prefixed := cmd.startup[name]

	switch {
	case name == homeVar && w.startupUnset[name] && !prefixed:
		w.opaque(OpacityStartupFile, name, DetailStartupValue)

		return "", false, false
	case !set:
		dir, set = w.resolver.LookupEnv(name)

		return dir, set, true
	case v.dynamic:
		w.opaque(OpacityStartupFile, name, DetailStartupValue)

		return "", false, false
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

		return "", false, false
	}

	return dir, true, true
}

// homeScript reads the startup file named file in dir, as the shell joins
// them, recording why when it cannot.
func (w *astWalker) homeScript(cmd Command, file, dir string) (startupScript, bool) {
	clean := resolvePath(cmd.WorkingDirectory, dir+"/"+file)
	relative := !filepath.IsAbs(clean)

	detail := ""

	switch {
	case relative && (cmd.DirUnknown || w.dirUnknown || cmd.DirComputed || w.dirComputed):
		detail = DetailScriptDirectory
	case specialPath(clean) || (relative && specialPath(filepath.Join("/", clean))):
		detail = DetailScriptRead
	}

	if detail != "" {
		w.opaque(OpacityStartupFile, file, detail)

		return startupScript{}, false
	}

	text, status, detail := w.homeFileSource(clean, cmd)

	switch status {
	case ScriptText:
		key := file + "\x00" + clean
		if w.expanding[startupPrefix+key+"\x00"+text] {
			return startupScript{}, false
		}

		return startupScript{label: file, key: key, text: text}, true
	case ScriptOpaque:
		w.opaque(OpacityStartupFile, file, detail)
	case ScriptMissing, ScriptBinary:
	}

	return startupScript{}, false
}

// homeFileSource returns the text of a home startup file: the content
// written to it earlier on the line, or the file on disk. A write whose
// target may be the file but cannot be resolved makes it opaque.
func (w *astWalker) homeFileSource(target string, cmd Command) (string, ScriptStatus, string) {
	text, found, captured, unsure := w.homeWrite(target)

	switch {
	case unsure || (found && !captured):
		return "", ScriptOpaque, DetailScriptWritten
	case found:
		return text, ScriptText, ""
	default:
		return w.scriptSource(target, cmd)
	}
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
		if fw.Vars.IsDynamic(name) {
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

	name, args, ok := shebangShell(text)
	if !ok {
		return startup
	}

	before, after := homeStartupFiles(name, shellOptions(name, args), false)
	if len(before)+len(after) == 0 {
		return startup
	}

	scripts := w.homeScripts(cmd, before)
	scripts = append(scripts, startup...)

	return append(scripts, w.homeScripts(cmd, after)...)
}

// shebangShell returns the shell a script's shebang runs and the words
// after it.
func shebangShell(text string) (string, []string, bool) {
	line, _, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(line, "#!") {
		return "", nil, false
	}

	words := strings.Fields(strings.TrimPrefix(line, "#!"))
	for i, word := range words {
		if name := commandName(word); shells[name] {
			return name, words[i+1:], true
		}
	}

	return "", nil, false
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

// lazyStartup returns the parts to walk for a zsh startup file after
// .zshenv: those read up front while HOME and ZDOTDIR stand as they did,
// otherwise the file read again from where they point now.
func (w *astWalker) lazyStartup(
	part startupScript,
	parent Command,
	start homeState,
) []startupScript {
	switch changed := w.currentHomeState() != start; {
	case !changed:
		return []startupScript{part}
	case part.key != "":
		return nil
	default:
		parent.startup = nil
		parent.WorkingDirectory = w.currentDir
		parent.DirUnknown, parent.DirComputed = w.dirUnknown, w.dirComputed

		return w.homeScripts(parent, []string{part.label})
	}
}

// assignsDefault reports ${X:=word} or ${X=word}, which sets X.
func assignsDefault(pe *syntax.ParamExp) bool {
	return pe.Exp != nil &&
		(pe.Exp.Op == syntax.AssignUnset || pe.Exp.Op == syntax.AssignUnsetOrNull)
}

// refersToOutput reports a word naming a variable that holds command
// output, which expandName renders as the text around it alone.
func (w *astWalker) refersToOutput(word string) bool {
	for _, m := range varRefPattern.FindAllStringSubmatch(word, -1) {
		if w.state.dynamicVars[m[1]] {
			return true
		}
	}

	return false
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
