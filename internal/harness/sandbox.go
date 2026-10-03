package harness

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
)

const (
	dirPerm  = 0o750
	filePerm = 0o600
	execPerm = 0o700

	placeholderWork = "{{WORK}}"
	placeholderHome = "{{HOME}}"
	placeholderRoot = "{{ROOT}}"

	placeholderWorkSlug = "{{WORK_SLUG}}"

	placeholdersPerRoot = 4
)

// capturePattern matches the per-hook base name mktemp gives the shim; the
// .in, .out and other parts share it with a suffix.
var capturePattern = regexp.MustCompile(`^hook\.[A-Za-z0-9]+$`)

// systemPath is appended to the sandbox PATH so harnesses find git, sh and
// the usual tools without inheriting the caller's PATH entries.
var systemPath = []string{
	"/opt/homebrew/bin",
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
	"/usr/sbin",
	"/sbin",
}

// Sandbox is a disposable home, project and bin directory for one harness
// run. Its environment is built from scratch: nothing from the caller's
// environment leaks in, so a harness cannot read the real HOME, XDG config,
// Codex or Claude config, or session sockets of the agent running the suite.
type Sandbox struct {
	Root     string
	Home     string
	Work     string
	Bin      string
	Captures string

	extraEnv map[string]string
	aliases  []string
	redact   map[string]string

	mu       sync.Mutex
	sessions map[int]struct{}
	known    map[int]int64
	keeper   *keeper

	keeperClosed bool

	scanMu sync.Mutex
}

// NewSandbox creates the sandbox directories under base (the system temp
// directory when empty).
func NewSandbox(base string) (*Sandbox, error) {
	root, err := os.MkdirTemp(base, "klaudiush-harness-")
	if err != nil {
		return nil, errors.Wrap(err, "creating sandbox")
	}

	s := &Sandbox{
		Root:     root,
		Home:     filepath.Join(root, "home"),
		Work:     filepath.Join(root, "work"),
		Bin:      filepath.Join(root, "bin"),
		Captures: filepath.Join(root, "captures"),
		extraEnv: map[string]string{},
		sessions: map[int]struct{}{},
		known:    map[int]int64{},
	}

	dirs := []string{
		s.Work, s.Bin, s.Captures, filepath.Join(root, "tmp"),
		s.ConfigHome(), s.DataHome(), s.StateHome(), s.CacheHome(), s.CodexHome(), s.ClaudeHome(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			_ = os.RemoveAll(root)

			return nil, errors.Wrapf(err, "creating %s", dir)
		}
	}

	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		s.aliases = append(s.aliases, resolved)
	}

	return s, nil
}

// ConfigHome is the sandbox XDG_CONFIG_HOME.
func (s *Sandbox) ConfigHome() string { return filepath.Join(s.Home, ".config") }

// DataHome is the sandbox XDG_DATA_HOME.
func (s *Sandbox) DataHome() string { return filepath.Join(s.Home, ".local", "share") }

// StateHome is the sandbox XDG_STATE_HOME.
func (s *Sandbox) StateHome() string { return filepath.Join(s.Home, ".local", "state") }

// CacheHome is the sandbox XDG_CACHE_HOME.
func (s *Sandbox) CacheHome() string { return filepath.Join(s.Home, ".cache") }

// CodexHome is the sandbox CODEX_HOME.
func (s *Sandbox) CodexHome() string { return filepath.Join(s.Home, ".codex") }

// ClaudeHome is the sandbox CLAUDE_CONFIG_DIR.
func (s *Sandbox) ClaudeHome() string { return filepath.Join(s.Home, ".claude") }

// SetEnv adds a variable to the sandbox environment.
func (s *Sandbox) SetEnv(key, value string) {
	s.extraEnv[key] = value
}

// Env is the complete environment for processes run in the sandbox.
func (s *Sandbox) Env() []string {
	path := append([]string{s.Bin}, systemPath...)
	tmp := filepath.Join(s.Root, "tmp")

	// Claude Code keeps per-project task files under /tmp/claude-<uid>
	// whatever TMPDIR says, unless CLAUDE_CODE_TMPDIR moves them.
	env := map[string]string{
		"PATH":                strings.Join(path, string(os.PathListSeparator)),
		"HOME":                s.Home,
		"XDG_CONFIG_HOME":     s.ConfigHome(),
		"XDG_DATA_HOME":       s.DataHome(),
		"XDG_STATE_HOME":      s.StateHome(),
		"XDG_CACHE_HOME":      s.CacheHome(),
		"CODEX_HOME":          s.CodexHome(),
		"CLAUDE_CONFIG_DIR":   s.ClaudeHome(),
		"TMPDIR":              tmp,
		"CLAUDE_CODE_TMPDIR":  tmp,
		"LANG":                "en_US.UTF-8",
		"TERM":                "dumb",
		"NO_COLOR":            "1",
		"SHELL":               "/bin/sh",
		"USER":                harnessIdentity,
		"LOGNAME":             harnessIdentity,
		"GIT_CONFIG_NOSYSTEM": "1",
	}

	maps.Copy(env, s.extraEnv)

	out := make([]string, 0, len(env))
	for _, key := range slices.Sorted(maps.Keys(env)) {
		out = append(out, key+"="+env[key])
	}

	return out
}

// WriteFile writes a file below the sandbox root, creating parent directories.
func (*Sandbox) WriteFile(path, content string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return errors.Wrapf(err, "creating directory for %s", path)
	}

	return errors.Wrapf(os.WriteFile(path, []byte(content), perm), "writing %s", path)
}

// InstallShim puts a klaudiush wrapper first on the sandbox PATH. It records
// each hook's arguments, stdin, stdout, stderr and exit status in the
// captures directory, then answers exactly as the real binary did.
func (s *Sandbox) InstallShim(realBinary string) error {
	capture := shellQuote(filepath.Join(s.Captures, "hook.XXXXXXXX"))
	script := strings.Join([]string{
		"#!/bin/sh",
		"capture=$(mktemp " + capture + ") || exit 1",
		`printf '%s\n' "$@" > "$capture.args"`,
		`cat > "$capture.in"`,
		shellQuote(realBinary) + ` "$@" < "$capture.in" > "$capture.out" 2> "$capture.err"`,
		"status=$?",
		`printf '%s\n' "$status" > "$capture.status"`,
		`cat "$capture.out"`,
		`cat "$capture.err" >&2`,
		`exit "$status"`,
		"",
	}, "\n")

	return s.WriteFile(filepath.Join(s.Bin, "klaudiush"), script, execPerm)
}

// Capture is one hook invocation recorded by the shim.
type Capture struct {
	Args   []string
	Input  []byte
	Output []byte
	Stderr []byte
	Status int
	At     time.Time
}

// Event is the hook event the capture was invoked for: the value after
// --hook-type or --event.
func (c Capture) Event() string {
	for i, arg := range c.Args {
		if (arg == "--hook-type" || arg == "--event") && i+1 < len(c.Args) {
			return c.Args[i+1]
		}
	}

	return ""
}

// ReadCaptures returns the recorded hook invocations, oldest first. Hooks
// that have not finished (no status file yet) are skipped.
func (s *Sandbox) ReadCaptures() ([]Capture, error) {
	entries, err := os.ReadDir(s.Captures)
	if err != nil {
		return nil, errors.Wrap(err, "reading captures")
	}

	var captures []Capture

	for _, entry := range entries {
		if !capturePattern.MatchString(entry.Name()) {
			continue
		}

		capture, ok, err := readCapture(filepath.Join(s.Captures, entry.Name()))
		if err != nil {
			return nil, err
		}

		if ok {
			captures = append(captures, capture)
		}
	}

	slices.SortStableFunc(captures, func(a, b Capture) int { return a.At.Compare(b.At) })

	return captures, nil
}

func readCapture(base string) (Capture, bool, error) {
	statusData, err := readFile(base + ".status")
	if errors.Is(err, fs.ErrNotExist) {
		return Capture{}, false, nil
	}

	if err != nil {
		return Capture{}, false, err
	}

	status, err := strconv.Atoi(strings.TrimSpace(string(statusData)))
	if err != nil {
		return Capture{}, false, errors.Wrapf(err, "parsing status of %s", base)
	}

	capture := Capture{Status: status}

	parts := map[string]*[]byte{
		".in":  &capture.Input,
		".out": &capture.Output,
		".err": &capture.Stderr,
	}
	for suffix, target := range parts {
		data, readErr := readFile(base + suffix)
		if readErr != nil {
			return Capture{}, false, readErr
		}

		*target = data
	}

	args, err := readFile(base + ".args")
	if err != nil {
		return Capture{}, false, err
	}

	capture.Args = strings.Split(strings.TrimRight(string(args), "\n"), "\n")

	info, err := os.Stat(base + ".in")
	if err != nil {
		return Capture{}, false, errors.Wrapf(err, "stat %s.in", base)
	}

	capture.At = info.ModTime()

	return capture, true, nil
}

// Redact replaces sandbox paths with placeholders so captured payloads and
// responses can be committed and replayed elsewhere.
func (s *Sandbox) Redact(text string) string {
	type pair struct{ from, to string }

	pairs := make([]pair, 0, len(s.redact)+(1+len(s.aliases))*placeholdersPerRoot)

	for from, to := range s.redact {
		pairs = append(pairs, pair{from, to})
	}

	for _, root := range append([]string{s.Root}, s.aliases...) {
		work := filepath.Join(root, "work")
		pairs = append(pairs,
			pair{work, placeholderWork},
			pair{filepath.Join(root, "home"), placeholderHome},
			pair{root, placeholderRoot},
			pair{slug(work), placeholderWorkSlug},
		)
	}

	slices.SortStableFunc(pairs, func(a, b pair) int { return len(b.from) - len(a.from) })

	for _, p := range pairs {
		text = strings.ReplaceAll(text, p.from, p.to)
	}

	return text
}

// AddRedaction makes Redact replace a machine-specific string, such as the
// path of the klaudiush binary under test, that sits outside the sandbox.
func (s *Sandbox) AddRedaction(from, to string) {
	if from == "" {
		return
	}

	if s.redact == nil {
		s.redact = map[string]string{}
	}

	s.redact[from] = to
}

func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Clean(path))

	return data, errors.Wrapf(err, "reading %s", path)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
