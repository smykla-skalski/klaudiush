package parser

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// ScriptStatus says what reading a script file found.
type ScriptStatus int

const (
	// ScriptMissing means there is no such file.
	ScriptMissing ScriptStatus = iota
	// ScriptText means the file was read in full.
	ScriptText
	// ScriptBinary means the file is a compiled program, not a script.
	ScriptBinary
	// ScriptOpaque means the file is a script that could not be read in full.
	ScriptOpaque
)

const (
	// maxScriptBytes caps how much of a script file is read.
	maxScriptBytes = 256 << 10
	// maxCompareBytes caps the size of a program compared byte for byte.
	maxCompareBytes = 128 << 20
	// compareBlockBytes is how much of each program is compared at a time.
	compareBlockBytes = 64 << 10
	// binarySniffBytes is how much of a script is read before the rest.
	binarySniffBytes = 8 << 10
	// lookupTimeout bounds one git or xcrun lookup.
	lookupTimeout = 2 * time.Second
	// maxAliasDirs caps how many repositories have their git aliases read.
	maxAliasDirs = 8
)

// Resolver answers what the command text alone cannot: the environment, the
// scripts a command runs, what a program name really is, and aliases.
type Resolver interface {
	// LookupEnv returns the value of an environment variable.
	LookupEnv(name string) (string, bool)
	// ReadScript returns the text of a script file and what reading found.
	ReadScript(path string) (string, ScriptStatus)
	// Program identifies what word runs, with relative paths taken from dir.
	Program(word, dir string) Program
	// LookPath returns the file a bare program name runs.
	LookPath(name string) (string, bool)
	// GitAlias returns the value of a git alias as seen from dir.
	GitAlias(dir, name string) (string, bool)
	GitCommand(name string) bool
	// GHAlias returns the expansion of a gh alias.
	GHAlias(name string) (string, bool)
}

// OSResolver answers from the running system. It remembers what it learns,
// so each lookup runs at most once for the parse it serves.
type OSResolver struct {
	mu        sync.Mutex
	aliases   map[string]map[string]string // git aliases by config directory
	programs  map[string]Program           // identities by word and directory
	targets   map[string]string            // real program files by name
	execPath  *string                      // git --exec-path
	ghAliases map[string]string            // gh aliases
	paths     map[string]string            // LookPath results, "" when not found
}

// LookupEnv returns the value of an environment variable.
func (*OSResolver) LookupEnv(name string) (string, bool) {
	return os.LookupEnv(name)
}

// ReadScript reads a script file. A NUL in the first line of a file without
// a shebang marks a compiled program, as bash sees it; NULs later on are
// dropped. A script larger than maxScriptBytes, or one that cannot be read,
// is opaque.
func (*OSResolver) ReadScript(path string) (string, ScriptStatus) {
	path = xdg.ExpandPathSilent(path)

	// The stat keeps a FIFO or device from blocking the read.
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ScriptMissing
	}

	file, err := os.Open(path) //nolint:gosec // reads a script the command itself runs
	if err != nil {
		return "", ScriptOpaque
	}
	defer file.Close() //nolint:errcheck // read-only file

	head := make([]byte, binarySniffBytes)

	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", ScriptOpaque
	}

	head = head[:n]

	firstLine, _, _ := bytes.Cut(head, []byte("\n"))
	if !bytes.HasPrefix(head, []byte("#!")) && bytes.IndexByte(firstLine, 0) >= 0 {
		return "", ScriptBinary
	}

	if info.Size() > maxScriptBytes {
		return "", ScriptOpaque
	}

	rest, err := io.ReadAll(io.LimitReader(file, maxScriptBytes))
	if err != nil {
		return "", ScriptOpaque
	}

	return strings.ReplaceAll(string(head)+string(rest), "\x00", ""), ScriptText
}

// Program identifies what word runs: git or gh under any name, some other
// program, or nothing on disk at all.
func (r *OSResolver) Program(word, dir string) Program {
	key := word + "\x00" + dir

	r.mu.Lock()
	program, cached := r.programs[key]
	r.mu.Unlock()

	if cached {
		return program
	}

	program = ProgramMissing
	if path, ok := r.programPath(word, dir); ok {
		program = r.identifyProgram(path)
	}

	r.mu.Lock()
	if r.programs == nil {
		r.programs = make(map[string]Program)
	}

	r.programs[key] = program
	r.mu.Unlock()

	return program
}

// fallbackBinDirs are where tools usually live when a hook runs with a
// shorter PATH than the shell, as a GUI-launched host does. A name found
// here is a real program, not a missing one.
var fallbackBinDirs = strings.Fields(`/opt/homebrew/bin /usr/local/bin /usr/bin /bin
	/usr/sbin /sbin /run/current-system/sw/bin /nix/var/nix/profiles/default/bin
	~/.nix-profile/bin ~/.local/bin ~/bin ~/.cargo/bin ~/go/bin
	~/.local/share/mise/shims ~/.asdf/shims ~/.volta/bin`)

// LookPath returns the file a bare program name runs, looking beyond PATH in
// the usual install directories.
func (r *OSResolver) LookPath(name string) (string, bool) {
	r.mu.Lock()
	path, cached := r.paths[name]
	r.mu.Unlock()

	if !cached {
		path = lookPath(name)

		r.mu.Lock()
		if r.paths == nil {
			r.paths = make(map[string]string)
		}

		r.paths[name] = path
		r.mu.Unlock()
	}

	return path, path != ""
}

// lookPath searches PATH, then fallbackBinDirs.
func lookPath(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	for _, dir := range fallbackBinDirs {
		path := filepath.Join(xdg.ExpandPathSilent(dir), name)
		if info, err := os.Stat(
			path,
		); err == nil && info.Mode().IsRegular() &&
			info.Mode()&0o111 != 0 {
			return path
		}
	}

	return ""
}

// GitAlias returns the value of alias.<name> as git would see it from dir.
// Git runs an external git-<name> command before it considers aliases, so
// one found on PATH or in git's exec path rules the alias out. All aliases
// of a directory are read with one git config call and remembered.
func (r *OSResolver) GitAlias(dir, name string) (string, bool) {
	if r.GitCommand(name) {
		return "", false
	}

	if dir != "" {
		dir = xdg.ExpandPathSilent(dir)

		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = ""
		}
	}

	value, ok := r.gitAliases(dir)[strings.ToLower(name)]

	return value, ok
}

// GHAlias returns the expansion of a gh alias from gh's configuration.
func (r *OSResolver) GHAlias(name string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ghAliases == nil {
		r.ghAliases = readGHAliases(ghConfigFile())
	}

	value, ok := r.ghAliases[name]

	return value, ok
}

// GitCommand reports whether git runs an external git-<name> command for
// name, from PATH or git's exec path, instead of looking for an alias.
func (r *OSResolver) GitCommand(name string) bool {
	if _, ok := r.LookPath("git-" + name); ok {
		return true
	}

	r.mu.Lock()
	if r.execPath == nil {
		path := commandOutput(gitProgram, "--exec-path")
		r.execPath = &path
	}

	execPath := *r.execPath
	r.mu.Unlock()

	if execPath == "" {
		return false
	}

	_, err := os.Stat(filepath.Join(execPath, "git-"+name))

	return err == nil
}

// gitAliases returns every alias git sees from dir. Past maxAliasDirs
// directories only the global configuration is read, so a command naming
// many repositories cannot make klaudiush run git config for each.
func (r *OSResolver) gitAliases(dir string) map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if aliases, ok := r.aliases[dir]; ok {
		return aliases
	}

	if r.aliases == nil {
		r.aliases = make(map[string]map[string]string)
	}

	if len(r.aliases) >= maxAliasDirs {
		dir = ""

		if aliases, ok := r.aliases[dir]; ok {
			return aliases
		}
	}

	args := []string{"config", "--get-regexp", `^alias\.`}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}

	output := commandOutput(gitProgram, args...)
	aliases := make(map[string]string)

	for line := range strings.Lines(output) {
		key, value, _ := strings.Cut(strings.TrimRight(line, "\n"), " ")
		if name, ok := strings.CutPrefix(key, "alias."); ok {
			aliases[name] = value
		}
	}

	r.aliases[dir] = aliases

	return aliases
}

// ghConfigFile returns the file gh keeps its aliases in.
func ghConfigFile() string {
	if dir := os.Getenv("GH_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.yml")
	}

	return filepath.Join(xdg.ConfigHome(), "gh", "config.yml")
}

// readGHAliases reads the aliases section of gh's config.yml, a flat map of
// name to expansion.
func readGHAliases(path string) map[string]string {
	aliases := make(map[string]string)

	file, err := os.Open(path) //nolint:gosec // gh's own configuration
	if err != nil {
		return aliases
	}
	defer file.Close() //nolint:errcheck // read-only file

	inAliases := false
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "aliases:"):
			inAliases = true
		case inAliases && (line == "" || line[0] == ' ' || line[0] == '\t'):
			name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
			if ok && name != "" {
				aliases[strings.TrimSpace(name)] = strings.Trim(strings.TrimSpace(value), `"'`)
			}
		default:
			inAliases = false
		}
	}

	return aliases
}

// commandOutput runs a program without a shell and returns its trimmed
// output, or "" when it fails.
func commandOutput(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // no shell
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// programPath finds the file a command word runs.
func (r *OSResolver) programPath(word, dir string) (string, bool) {
	if !strings.Contains(word, "/") {
		return r.LookPath(word)
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
func (r *OSResolver) identifyProgram(path string) Program {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}

	base := strings.ToLower(filepath.Base(resolved))

	switch {
	case base == gitProgram || r.sameProgram(resolved, gitProgram):
		return ProgramGit
	case base == ghCLI || r.sameProgram(resolved, ghCLI):
		return ProgramGH
	default:
		return ProgramOther
	}
}

// sameProgram reports whether path is the named program under another name:
// the same file as the real program, or an identical copy of it.
func (r *OSResolver) sameProgram(path, name string) bool {
	target := r.realProgram(name)
	if target == "" {
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

// realProgram returns the file that really runs for name, or "" when there
// is none. A version manager shim is not a program of its own. The macOS
// /usr/bin launcher, one file shared by git, make, python3 and dozens more,
// is looked through with xcrun, since comparing against it would make every
// tool git.
func (r *OSResolver) realProgram(name string) string {
	r.mu.Lock()
	target, cached := r.targets[name]
	r.mu.Unlock()

	if cached {
		return target
	}

	target = findRealProgram(name)

	r.mu.Lock()
	if r.targets == nil {
		r.targets = make(map[string]string)
	}

	r.targets[name] = target
	r.mu.Unlock()

	return target
}

// findRealProgram does the lookup realProgram remembers.
func findRealProgram(name string) string {
	target, err := exec.LookPath(name)
	if err != nil {
		return ""
	}

	if target, err = filepath.EvalSymlinks(target); err != nil {
		return ""
	}

	if strings.ToLower(filepath.Base(target)) != name {
		return ""
	}

	if isLauncherStub(target) {
		target = commandOutput("xcrun", "--find", name)
	}

	return target
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

// sameContent reports whether two files hold identical bytes. The files are
// read in step, one block at a time, stopping at the first difference, so a
// large program is never held in memory.
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
