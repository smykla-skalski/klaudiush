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
			}, 1, scriptWalk{label: target.label})
		}
	}
}

func (w *astWalker) startupWriteTargets(fw FileWrite) []startupTarget {
	path, known := w.writtenPath(fw)
	if !known {
		label, foreign, ok := startupPathSuffix(fw.Path)
		if !ok {
			return nil
		}

		return []startupTarget{{label: label, foreign: foreign}}
	}

	if label, foreign, ok := w.classifyStartupPath(fw, path); ok {
		return []startupTarget{{path: path, label: label, foreign: foreign}}
	}

	_, sources, isCopy := w.startupCopySources(fw)
	if !isCopy {
		return nil
	}

	targets := make([]startupTarget, 0, len(sources))

	for _, source := range sources {
		sourcePath, sourceKnown := w.startupCopySourcePath(fw, source)
		if !sourceKnown {
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
		if candidate, set := w.startupWriteVariable(fw, variable); set && candidate == path {
			return variable, false, true
		}
	}

	if home, set := w.startupWriteVariable(fw, homeVar); set {
		if relative, inside := relativeStartupPath(home, path); inside {
			if foreign, startup := homeStartupPath(relative); startup {
				return filepath.Base(path), foreign, true
			}
		}
	}

	if zdot, set := w.startupWriteVariable(fw, zdotdirVar); set {
		if relative, inside := relativeStartupPath(zdot, path); inside && zshFiles[relative] {
			return filepath.Base(path), false, true
		}
	}

	return "", false, false
}

func (w *astWalker) startupWriteVariable(fw FileWrite, name string) (string, bool) {
	if fw.Vars.IsDynamic(name) || fw.Vars.unknownName(name) {
		return "", false
	}

	value, set := "${"+name+"}", false
	if fw.Vars != nil {
		_, set = fw.Vars.Assignments[name]
	}

	if !set {
		_, set = w.resolver.LookupEnv(name)
	}

	if !set {
		return "", false
	}

	candidate := fw
	candidate.Path = value
	candidate.Dynamic = false
	candidate.TargetUnknown = false

	return w.writtenPath(candidate)
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

	cmd, sources, ok := w.startupCopySources(fw)
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
		path, sourceKnown := w.startupCopySourcePath(fw, source)
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
	for p := w; p != nil; p = p.parent {
		writes := make([]FileWrite, 0, len(p.fileWrites))

		for _, candidate := range p.fileWrites {
			path, known := w.writtenPath(candidate)
			if !known {
				continue
			}

			candidate.Path, candidate.WorkingDirectory = path, ""
			writes = append(writes, candidate)
		}

		if content, found, captured := lastWrite(writes, target, &fw.Location); found {
			return content, captured
		}
	}

	text, status := w.resolver.ReadScript(target)

	return text, status == ScriptText || status == ScriptMissing
}

func (w *astWalker) startupCopySources(fw FileWrite) (Command, []string, bool) {
	if fw.Operation != WriteOpCopy || fw.Source != cpProgram {
		return Command{}, nil, false
	}

	if w.defined(cpProgram) || w.state.pathChanged {
		return Command{}, nil, false
	}

	cmdIndex := slices.IndexFunc(w.commands, func(cmd Command) bool {
		return cmd.Name == cpProgram && cmd.Location == fw.Location
	})
	if cmdIndex < 0 {
		return Command{}, nil, false
	}

	var dirs []string

	cmd := w.commands[cmdIndex]

	operands := scanArgs(cmd.Args, copySpec, func(name, value string) {
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

func (w *astWalker) startupCopySourcePath(fw FileWrite, word string) (string, bool) {
	source := fw
	source.Path = word
	source.Dynamic = marked(word) || HasUnresolvedVars(word)

	return w.writtenPath(source)
}
