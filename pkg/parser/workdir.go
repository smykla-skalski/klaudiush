package parser

import (
	"path/filepath"
	"slices"
	"strings"
)

const (
	pwdVar    = "PWD"
	oldPWDVar = "OLDPWD"
)

// moveDir follows cd or pushd. A directory built from command output, or
// from a variable holding it, is unknown: the walker cannot see the output,
// and rendering it partially would send relative paths to the wrong place.
func (w *astWalker) moveDir(cmd Command) {
	if cmd.Dynamic || slices.ContainsFunc(cmd.Args, w.namesDynamicVar) {
		w.dirComputed, w.dirUnknown = true, true

		return
	}

	w.changeDirTo(cmd, cmd.Args)
}

// namesDynamicVar reports a word that uses a variable holding command
// output, arithmetic or an append.
func (w *astWalker) namesDynamicVar(word string) bool {
	for _, ref := range varRefPattern.FindAllStringSubmatch(word, -1) {
		if w.state.dynamicVars[ref[1]] {
			return true
		}
	}

	return false
}

// syncDirVars captures $PWD before a directory change and returns the
// function that sets PWD and OLDPWD after it, as the shell does. Without
// it a later $PWD would be read from klaudiush's own environment.
func (w *astWalker) syncDirVars() func() {
	old, oldKnown := w.pwdValue()

	return func() {
		w.setDirVar(oldPWDVar, old, oldKnown)

		pwd, known := w.pwdValue()
		w.setDirVar(pwdVar, pwd, known)
	}
}

// pwdValue returns $PWD as the shell holds it now, and whether it is known.
// A relative tracked directory is relative to the directory the line
// started in, which is the PWD klaudiush runs in.
func (w *astWalker) pwdValue() (string, bool) {
	if w.dirUnknown {
		return "", false
	}

	if w.currentDir == "" {
		if w.unknownVars[pwdVar] {
			return "", false
		}

		if value, ok := w.assignments[pwdVar]; ok {
			return value, true
		}

		return w.startPWD()
	}

	dir := w.currentDir

	if strings.HasPrefix(dir, "~") {
		if w.homeChanged() {
			return "", false
		}

		dir = ExpandHome(dir, w.resolver)
	}

	if strings.HasPrefix(dir, "~") || HasUnresolvedVars(dir) {
		return "", false
	}

	if !filepath.IsAbs(dir) {
		start, ok := w.startPWD()
		if !ok {
			return "", false
		}

		dir = filepath.Join(start, dir)
	}

	return filepath.Clean(dir), true
}

// startPWD returns the directory the line starts in.
func (w *astWalker) startPWD() (string, bool) {
	start, ok := w.resolver.LookupEnv(pwdVar)
	if !ok || !filepath.IsAbs(start) {
		return "", false
	}

	return start, true
}

// unknownDirVars adds PWD and OLDPWD to dynamic when the walker cannot
// tell them, so validators reading a VarScope do not fall back to the
// environment klaudiush runs in.
func (w *astWalker) unknownDirVars(dynamic map[string]bool) map[string]bool {
	for _, name := range []string{pwdVar, oldPWDVar} {
		if !w.unknownVars[name] {
			continue
		}

		if dynamic == nil {
			dynamic = make(map[string]bool)
		}

		dynamic[name] = true
	}

	return dynamic
}

// setDirVar sets PWD or OLDPWD for this walker only: a cd in a nested
// script does not move the shell that runs it.
func (w *astWalker) setDirVar(name, value string, known bool) {
	w.scope = nil

	if known {
		w.assignments[name] = value
		delete(w.unknownVars, name)

		return
	}

	delete(w.assignments, name)
	w.unknownVars[name] = true
}
