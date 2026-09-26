package parser

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

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
	// compareBlockBytes is how much of each program is compared at a time.
	compareBlockBytes = 64 << 10
	// binarySniffBytes is how much of a script is checked for binary content
	// before the rest is read.
	binarySniffBytes = 8 << 10
	// lookupTimeout bounds one git config or xcrun lookup.
	lookupTimeout = 2 * time.Second
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
// anything that looks binary. The stat keeps a FIFO from blocking the read,
// and only the first block is read before a binary is turned away.
func (OSResolver) ReadScript(path string) (string, bool) {
	path = xdg.ExpandPathSilent(path)

	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScriptBytes {
		return "", false
	}

	file, err := os.Open(path) //nolint:gosec // reads a script the command itself runs
	if err != nil {
		return "", false
	}
	defer file.Close() //nolint:errcheck // read-only file

	head := make([]byte, binarySniffBytes)

	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", false
	}

	if bytes.IndexByte(head[:n], 0) >= 0 {
		return "", false
	}

	rest, err := io.ReadAll(io.LimitReader(file, maxScriptBytes-int64(n)))
	if err != nil {
		return "", false
	}

	return string(head[:n]) + string(rest), true
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

// GitAlias returns the value of alias.<name> as git would see it from dir.
// Git runs an external git-<name> command before it considers aliases, so one
// found on PATH (git lfs, git flow) rules the alias out without running git
// config. A dir that is not a directory falls back to the global config; git
// reads system and global config from any directory, so one lookup suffices.
func (OSResolver) GitAlias(dir, name string) (string, bool) {
	if _, err := exec.LookPath("git-" + name); err == nil {
		return "", false
	}

	if dir != "" {
		dir = xdg.ExpandPathSilent(dir)

		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = ""
		}
	}

	return gitConfigAlias(dir, name)
}

// gitConfigAlias runs git config to read one alias.
func gitConfigAlias(dir, name string) (string, bool) {
	args := []string{"config", "--get", "alias." + name}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}

	return commandOutput(gitProgram, args...)
}

// commandOutput runs a program without a shell and returns its trimmed
// output, or false when it fails or prints nothing.
func commandOutput(name string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // no shell
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

// identifyProgram tells whether path is git or gh: by the name its symlinks
// lead to, or by being the same file or an identical copy.
func identifyProgram(path string) Program {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}

	base := strings.ToLower(filepath.Base(resolved))

	switch {
	case base == gitProgram || sameProgram(resolved, gitProgram):
		return ProgramGit
	case base == ghCLI || sameProgram(resolved, ghCLI):
		return ProgramGH
	default:
		return ProgramOther
	}
}

// sameProgram reports whether path is the named program under another name:
// the same file as the real program, or an identical copy of it.
func sameProgram(path, name string) bool {
	target, ok := realProgram(name)
	if !ok {
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

// realProgram returns the file that really runs for name. A version manager
// shim is not a program of its own, so it yields nothing. The macOS /usr/bin
// launcher, one file shared by git, make, python3 and dozens more, is looked
// through with xcrun, since comparing against it would make every tool git.
func realProgram(name string) (string, bool) {
	target, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}

	if target, err = filepath.EvalSymlinks(target); err != nil {
		return "", false
	}

	if strings.ToLower(filepath.Base(target)) != name {
		return "", false
	}

	if isLauncherStub(target) {
		return commandOutput("xcrun", "--find", name)
	}

	return target, true
}

// launcherSiblings are tools that share one file with the macOS launcher.
var launcherSiblings = strings.Fields("make cc clang python3 swift xcrun")

// isLauncherStub reports whether path is one file shared by unrelated tools,
// which picks the tool to run from the name it was started under.
func isLauncherStub(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	for _, sibling := range launcherSiblings {
		siblingPath, err := exec.LookPath(sibling)
		if err != nil {
			continue
		}

		if siblingInfo, err := os.Stat(siblingPath); err == nil && os.SameFile(info, siblingInfo) {
			return true
		}
	}

	return false
}

// sameContent reports whether two files hold identical bytes.
// The files are read in step, one block at a time, stopping at the first
// difference, so a large program is never held in memory.
func sameContent(a, b string) bool {
	fileA, err := os.Open(a) //nolint:gosec // compares a program the command runs
	if err != nil {
		return false
	}
	defer fileA.Close() //nolint:errcheck // read-only file

	fileB, err := os.Open(b) //nolint:gosec // compares a program the command runs
	if err != nil {
		return false
	}
	defer fileB.Close() //nolint:errcheck // read-only file

	blockA := make([]byte, compareBlockBytes)
	blockB := make([]byte, compareBlockBytes)

	for {
		nA, errA := io.ReadFull(fileA, blockA)
		nB, errB := io.ReadFull(fileB, blockB)

		if nA != nB || !bytes.Equal(blockA[:nA], blockB[:nB]) {
			return false
		}

		doneA := errors.Is(errA, io.EOF) || errors.Is(errA, io.ErrUnexpectedEOF)
		doneB := errors.Is(errB, io.EOF) || errors.Is(errB, io.ErrUnexpectedEOF)

		switch {
		case doneA && doneB:
			return true
		case doneA || doneB || errA != nil || errB != nil:
			return false
		}
	}
}
