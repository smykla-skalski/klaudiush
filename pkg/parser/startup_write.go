package parser

import (
	"path/filepath"
	"slices"
	"strings"
)

var supportedHomeStartupPaths = nameSet(`.zshenv .zprofile .zshrc .zlogin .zlogout
	.bash_profile .bash_login .profile .bash_logout .bashrc .kshrc .mkshrc
	.yash_profile .yashrc`)

var foreignHomeStartupPaths = nameSet(`.tcshrc .cshrc .login .logout
	.config/fish/config.fish .config/nushell/env.nu .config/nushell/config.nu
	.config/nushell/login.nu .config/elvish/rc.elv .config/xonsh/rc.xsh .xonshrc`)

var supportedSystemStartupPaths = nameSet(`/etc/zshenv /etc/zprofile /etc/zshrc
	/etc/zlogin /etc/zlogout /etc/profile /etc/bash.bashrc`)

var foreignSystemStartupPaths = nameSet(`/etc/csh.cshrc /etc/csh.login
	/etc/fish/config.fish /etc/xonsh/xonshrc`)

type startupTarget struct {
	path    string
	label   string
	foreign bool
	opaque  bool
}

// validateStartupWrites follows content planted in startup files even when
// this command line does not start the shell that will read them later.
func (w *astWalker) validateStartupWrites() {
	seen := make(map[string]bool)

	for i := 0; ; i++ {
		if i >= len(w.fileWrites) {
			break
		}

		fw := w.fileWrites[i]

		for _, target := range w.startupWriteTargets(fw) {
			if target.opaque {
				w.opaque(OpacityStartupFile, target.label, DetailScriptWritten)

				continue
			}

			if target.foreign {
				w.opaque(OpacityScriptSyntax, target.label, "")

				continue
			}

			text, ok := w.startupWriteContent(fw, target.path)
			if !ok {
				w.opaque(OpacityStartupFile, target.label, DetailScriptWritten)

				continue
			}

			key := target.path + "\x00" + text
			if seen[key] {
				continue
			}

			seen[key] = true

			w.walkScript(text, Command{
				Name:             target.label,
				Invoked:          target.label,
				Location:         fw.Location,
				WorkingDirectory: fw.WorkingDirectory,
				DirUnknown:       fw.DirUnknown,
				Vars:             fw.Vars,
			}, 1, scriptWalk{label: target.label, future: true})
		}
	}
}

func (w *astWalker) startupWriteTargets(fw FileWrite) []startupTarget {
	target := fw
	if fw.targetFromSubstitution && fw.Path != "" {
		target.Dynamic = false
	}

	path, known := w.writtenPath(target)
	if !known {
		label, foreign, ok := startupPathSuffix(fw.Path)
		if ok {
			return []startupTarget{{label: label, foreign: foreign}}
		}

		if fw.TargetUnknown && fw.targetFromSubstitution {
			return []startupTarget{{label: "dynamic redirect", opaque: true}}
		}

		return nil
	}

	if label, foreign, ok := w.classifyStartupPath(fw, path); ok {
		return []startupTarget{{path: path, label: label, foreign: foreign}}
	}

	_, sources, isTransfer := w.startupTransferSources(fw)
	if !isTransfer {
		return nil
	}

	targets := make([]startupTarget, 0, len(sources))
	directoryLabel, startupDirectory := w.startupDirectoryLabel(fw, path)

	for _, source := range sources {
		sourcePath, sourceKnown := w.startupTransferSourcePath(fw, source)
		if !sourceKnown {
			if startupDirectory {
				targets = append(targets, startupTarget{label: directoryLabel, opaque: true})
			}

			continue
		}

		candidate := filepath.Join(path, filepath.Base(sourcePath))
		if label, foreign, ok := w.classifyStartupPath(fw, candidate); ok {
			targets = append(targets, startupTarget{
				path: candidate, label: label, foreign: foreign,
			})
		}
	}

	return targets
}

func (w *astWalker) classifyStartupPath(
	fw FileWrite,
	path string,
) (label string, foreign, ok bool) {
	if supportedSystemStartupPaths[path] {
		return filepath.Base(path), false, true
	}

	if foreignSystemStartupPaths[path] || fishConfPath(path, "/etc/fish/conf.d") {
		return filepath.Base(path), true, true
	}

	for _, variable := range []string{bashEnvVar, envVar} {
		if slices.Contains(w.startupWriteVariables(fw, variable), path) {
			return variable, false, true
		}
	}

	for _, home := range w.startupWriteVariables(fw, homeVar) {
		if relative, inside := relativeStartupPath(home, path); inside {
			if foreign, startup := homeStartupPath(relative); startup {
				return filepath.Base(path), foreign, true
			}
		}
	}

	for _, zdot := range w.startupWriteVariables(fw, zdotdirVar) {
		if relative, inside := relativeStartupPath(zdot, path); inside && zshFiles[relative] {
			return filepath.Base(path), false, true
		}
	}

	return "", false, false
}

func (w *astWalker) startupWriteVariables(fw FileWrite, name string) []string {
	var values []string

	if !fw.Vars.IsDynamic(name) && !fw.Vars.unknownName(name) && fw.Vars != nil {
		if _, set := fw.Vars.Assignments[name]; set {
			candidate := fw
			candidate.Path = "${" + name + "}"
			candidate.Dynamic = false
			candidate.TargetUnknown = false

			if path, known := w.writtenPath(candidate); known {
				values = append(values, path)
			}
		}
	}

	if value, set := w.resolver.LookupEnv(name); set {
		candidate := fw
		candidate.Path = value
		candidate.Dynamic = false
		candidate.TargetUnknown = false

		if path, known := w.writtenPath(candidate); known && !slices.Contains(values, path) {
			values = append(values, path)
		}
	}

	return values
}

func (w *astWalker) startupDirectoryLabel(fw FileWrite, path string) (string, bool) {
	path = filepath.Clean(path)

	if systemStartupDirectory(path) {
		return path, true
	}

	if variable, ok := w.envStartupDirectory(fw, path); ok {
		return variable, true
	}

	if w.homeStartupDirectory(fw, path) {
		return homeVar, true
	}

	if slices.Contains(w.startupWriteVariables(fw, zdotdirVar), path) {
		return zdotdirVar, true
	}

	return "", false
}

func systemStartupDirectory(path string) bool {
	for candidate := range supportedSystemStartupPaths {
		if filepath.Dir(candidate) == path {
			return true
		}
	}

	for candidate := range foreignSystemStartupPaths {
		if filepath.Dir(candidate) == path {
			return true
		}
	}

	return path == "/etc/fish/conf.d"
}

func (w *astWalker) envStartupDirectory(fw FileWrite, path string) (string, bool) {
	for _, variable := range []string{bashEnvVar, envVar} {
		for _, candidate := range w.startupWriteVariables(fw, variable) {
			if filepath.Dir(candidate) == path {
				return variable, true
			}
		}
	}

	return "", false
}

func (w *astWalker) homeStartupDirectory(fw FileWrite, path string) bool {
	for _, home := range w.startupWriteVariables(fw, homeVar) {
		if homeStartupDirectoryAt(home, path) {
			return true
		}
	}

	return false
}

func homeStartupDirectoryAt(home, path string) bool {
	for candidate := range supportedHomeStartupPaths {
		if filepath.Dir(filepath.Join(home, filepath.FromSlash(candidate))) == path {
			return true
		}
	}

	for candidate := range foreignHomeStartupPaths {
		if filepath.Dir(filepath.Join(home, filepath.FromSlash(candidate))) == path {
			return true
		}
	}

	return filepath.Join(home, ".config/fish/conf.d") == path
}

func relativeStartupPath(root, path string) (string, bool) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}

	return filepath.ToSlash(relative), true
}

func homeStartupPath(path string) (foreign, ok bool) {
	switch {
	case supportedHomeStartupPaths[path]:
		return false, true
	case foreignHomeStartupPaths[path]:
		return true, true
	case fishConfPath(path, ".config/fish/conf.d"):
		return true, true
	default:
		return false, false
	}
}

func fishConfPath(path, dir string) bool {
	return filepath.ToSlash(filepath.Dir(path)) == dir && strings.HasSuffix(path, ".fish")
}

func startupPathSuffix(path string) (label string, foreign, ok bool) {
	path = filepath.ToSlash(filepath.Clean(path))

	for candidate := range supportedHomeStartupPaths {
		if path == candidate || strings.HasSuffix(path, "/"+candidate) {
			return filepath.Base(candidate), false, true
		}
	}

	for candidate := range foreignHomeStartupPaths {
		if path == candidate || strings.HasSuffix(path, "/"+candidate) {
			return filepath.Base(candidate), true, true
		}
	}

	if strings.Contains(path, "/.config/fish/conf.d/") && strings.HasSuffix(path, ".fish") {
		return filepath.Base(path), true, true
	}

	return "", false, false
}

func (w *astWalker) startupWriteContent(fw FileWrite, target string) (string, bool) {
	if fw.emittedContentCaptured {
		if fw.emittedContentAppended {
			before, ok := w.startupContentBefore(fw, target)
			if !ok {
				return "", false
			}

			return before + fw.emittedContent, true
		}

		return fw.emittedContent, true
	}

	cmd, sources, ok := w.startupTransferSources(fw)
	if !ok {
		return "", false
	}

	destination, known := w.writtenPath(fw)
	if !known {
		return "", false
	}

	if destination == target && len(sources) != 1 {
		return "", false
	}

	for _, source := range sources {
		path, sourceKnown := w.startupTransferSourcePath(fw, source)
		if !sourceKnown {
			continue
		}

		if destination != target && filepath.Join(destination, filepath.Base(path)) != target {
			continue
		}

		text, status, _ := w.scriptSource(path, cmd)

		return text, status == ScriptText
	}

	return "", false
}

func (w *astWalker) startupContentBefore(fw FileWrite, target string) (string, bool) {
	text, status := w.resolver.ReadScript(target)
	captured := status == ScriptText || status == ScriptMissing

	var chain []*astWalker
	for p := w; p != nil; p = p.parent {
		chain = append(chain, p)
	}

	for _, walker := range slices.Backward(chain) {
		text, captured = w.replayStartupWrites(
			walker.fileWrites,
			target,
			fw.Location,
			text,
			captured,
		)
	}

	return text, captured
}

func (w *astWalker) replayStartupWrites(
	writes []FileWrite,
	target string,
	before Location,
	text string,
	captured bool,
) (string, bool) {
	for _, candidate := range writes {
		if !locationBefore(candidate.Location, before) {
			continue
		}

		path, known := w.writtenPath(candidate)
		if !known || path != target {
			continue
		}

		text, captured = applyStartupWrite(candidate, text, captured)
	}

	return text, captured
}

func applyStartupWrite(candidate FileWrite, text string, captured bool) (string, bool) {
	if !candidate.emittedContentCaptured {
		return "", false
	}

	if !candidate.emittedContentAppended {
		return candidate.emittedContent, true
	}

	if captured {
		text += candidate.emittedContent
	}

	return text, captured
}

func (w *astWalker) startupTransferSources(fw FileWrite) (Command, []string, bool) {
	var spec optionSpec

	switch fw.Source {
	case cpProgram, "copy", mvProgram, "move":
		spec = copySpec
	case "rsync":
		spec = rsyncSpec
	case installProgram:
		spec = installSpec
	default:
		return Command{}, nil, false
	}

	if w.defined(fw.Source) || w.state.pathChanged {
		return Command{}, nil, false
	}

	cmdIndex := slices.IndexFunc(w.commands, func(cmd Command) bool {
		return cmd.Name == fw.Source && cmd.Location == fw.Location
	})
	if cmdIndex < 0 {
		return Command{}, nil, false
	}

	var dirs []string

	cmd := w.commands[cmdIndex]

	operands := scanArgs(fw.sourceArgs, spec, func(name, value string) {
		if targetDirOpts[name] {
			dirs = append(dirs, value)
		}
	})
	if len(dirs) == 1 && len(operands) > 0 {
		return cmd, operands, true
	}

	if len(dirs) > 0 || len(operands) < minDestOperands {
		return Command{}, nil, false
	}

	return cmd, operands[:len(operands)-1], true
}

func (w *astWalker) startupTransferSourcePath(fw FileWrite, word string) (string, bool) {
	source := fw
	source.Path = word
	source.Dynamic = marked(word) || HasUnresolvedVars(word)

	return w.writtenPath(source)
}
