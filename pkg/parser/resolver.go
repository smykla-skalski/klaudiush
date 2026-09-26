package parser

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

// Program identifies what a command word runs, as far as the system can tell.
type Program int

const (
	// ProgramOther is some other program found on disk.
	ProgramOther Program = iota
	// ProgramGit is git, under its own name or another (symlink, hard link, copy).
	ProgramGit
	// ProgramGH is the GitHub CLI, under its own name or another.
	ProgramGH
	// ProgramMissing means nothing on disk runs the word: a shell alias or
	// function, a variable that could not be resolved, or a typo.
	ProgramMissing
)

const (
	// maxScriptBytes caps how much of a script file is read.
	maxScriptBytes = 256 << 10
	// maxCompareBytes caps the size of a program compared byte for byte.
	maxCompareBytes = 128 << 20
	// gitAliasTimeout bounds one git config lookup.
	gitAliasTimeout = 2 * time.Second
)

// Resolver answers what the command text alone cannot: the environment, the
// scripts a command runs, what a program name really is, and git aliases.
type Resolver interface {
	// LookupEnv returns the value of an environment variable.
	LookupEnv(name string) (string, bool)
	// ReadScript returns the text of a script file, refusing binaries.
	ReadScript(path string) (string, bool)
	// Program identifies what word runs, with relative paths taken from dir.
	Program(word, dir string) Program
	// GitAlias returns the value of a git alias as seen from dir.
	GitAlias(dir, name string) (string, bool)
}

// OSResolver answers from the running system.
type OSResolver struct{}

// LookupEnv returns the value of an environment variable.
func (OSResolver) LookupEnv(name string) (string, bool) {
	return os.LookupEnv(name)
}

// ReadScript returns the text of a regular file of bounded size, refusing
// anything that looks binary.
func (OSResolver) ReadScript(path string) (string, bool) {
	path = xdg.ExpandPathSilent(path)

	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScriptBytes {
		return "", false
	}

	data, err := os.ReadFile(path) //nolint:gosec // reads a script the command itself runs
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return "", false
	}

	return string(data), true
}

// Program identifies what word runs: git or gh under any name, some other
// program, or nothing on disk at all.
func (OSResolver) Program(word, dir string) Program {
	path, ok := programPath(word, dir)
	if !ok {
		return ProgramMissing
	}

	return identifyProgram(path)
}

// GitAlias returns the value of alias.<name> as git would see it from dir,
// falling back to the global configuration when dir is not usable. Git runs
// an external git-<name> command before it considers aliases, so one found on
// PATH (git lfs, git flow) rules the alias out without running git config.
func (OSResolver) GitAlias(dir, name string) (string, bool) {
	if _, err := exec.LookPath("git-" + name); err == nil {
		return "", false
	}

	if dir != "" {
		if value, ok := gitConfigAlias(xdg.ExpandPathSilent(dir), name); ok {
			return value, true
		}
	}

	return gitConfigAlias("", name)
}

// gitConfigAlias runs git config to read one alias.
func gitConfigAlias(dir, name string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitAliasTimeout)
	defer cancel()

	args := []string{"config", "--get", "alias." + name}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}

	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // fixed program, no shell

	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	value := strings.TrimSpace(string(out))

	return value, value != ""
}

// programPath finds the file a command word runs.
func programPath(word, dir string) (string, bool) {
	if !strings.Contains(word, "/") {
		path, err := exec.LookPath(word)

		return path, err == nil
	}

	path := xdg.ExpandPathSilent(word)
	if !filepath.IsAbs(path) && dir != "" {
		path = filepath.Join(xdg.ExpandPathSilent(dir), path)
	}

	info, err := os.Stat(path)

	return path, err == nil && !info.IsDir()
}

// knownPrograms are the programs identified under any name.
var knownPrograms = []struct {
	name    string
	program Program
}{
	{name: "git", program: ProgramGit},
	{name: "gh", program: ProgramGH},
}

// identifyProgram tells whether path is git or gh: by the name its symlinks
// lead to, or by being the same file or an identical copy.
func identifyProgram(path string) Program {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}

	base := strings.ToLower(filepath.Base(resolved))

	for _, known := range knownPrograms {
		if base == known.name || sameProgram(resolved, known.name) {
			return known.program
		}
	}

	return ProgramOther
}

// sameProgram reports whether path is the named program under another name.
// A named program that is itself a dispatcher (a version manager shim) is
// not compared, since every other shim would match it.
func sameProgram(path, name string) bool {
	target, err := exec.LookPath(name)
	if err != nil {
		return false
	}

	if target, err = filepath.EvalSymlinks(target); err != nil {
		return false
	}

	if strings.ToLower(filepath.Base(target)) != name {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	targetInfo, err := os.Stat(target)
	if err != nil {
		return false
	}

	if os.SameFile(info, targetInfo) {
		return true
	}

	return info.Size() == targetInfo.Size() && info.Size() <= maxCompareBytes &&
		sameContent(path, target)
}

// sameContent reports whether two files hold identical bytes.
func sameContent(a, b string) bool {
	dataA, err := os.ReadFile(a) //nolint:gosec // compares a program the command runs
	if err != nil {
		return false
	}

	dataB, err := os.ReadFile(b) //nolint:gosec // compares a program the command runs
	if err != nil {
		return false
	}

	return bytes.Equal(dataA, dataB)
}

// nopResolver knows nothing beyond the command text.
type nopResolver struct{}

func (nopResolver) LookupEnv(string) (string, bool)        { return "", false }
func (nopResolver) ReadScript(string) (string, bool)       { return "", false }
func (nopResolver) Program(string, string) Program         { return ProgramOther }
func (nopResolver) GitAlias(string, string) (string, bool) { return "", false }
