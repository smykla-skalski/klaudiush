package harness

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
)

const (
	shimQueryTimeout = 30 * time.Second
	shimHeaderSize   = 512
	miseName         = "mise"
	asdfName         = "asdf"
	envName          = "env"
	miseInactive     = "not currently active"
)

var errInactiveShim = errors.New("shim tool is not active")

// harnessBinary resolves a harness executable on first use, so building a
// driver runs no version manager.
type harnessBinary struct {
	envVar string
	name   string
	once   sync.Once
	path   string
	err    error
}

func newHarnessBinary(envVar, name string) *harnessBinary {
	return &harnessBinary{envVar: envVar, name: name}
}

func (b *harnessBinary) resolve(ctx context.Context) string {
	b.once.Do(func() { b.path, b.err = ResolveBinary(ctx, b.envVar, b.name) })

	return b.path
}

// Binary is the resolved executable, or "" when it is missing or could not
// be resolved.
func (b *harnessBinary) Binary() string {
	return b.resolve(context.Background())
}

// BinaryError says why a harness that is installed could not be resolved.
func (b *harnessBinary) BinaryError() error {
	b.resolve(context.Background())

	return b.err
}

// ResolveBinary finds a harness executable: the override variable, then
// PATH. Symlinks are resolved so the sandbox PATH does not need the caller's
// PATH entries. Version-manager shims (mise, asdf) cannot run in the empty
// sandbox environment, so they are resolved to the real executable here, in
// the caller's environment. A mise shim whose tool is not active is skipped
// the way the shim itself falls through to the next PATH entry; any other
// resolution error is returned. A script whose interpreter is not on the
// sandbox PATH is rejected. It returns "" and no error when the harness is
// not installed.
func ResolveBinary(ctx context.Context, envVar, name string) (string, error) {
	if override := os.Getenv(envVar); override != "" {
		path, err := resolveOverride(ctx, override)
		if err != nil {
			return "", errors.Wrapf(err, "%s=%s", envVar, override)
		}

		return path, nil
	}

	var inactive error

	for _, candidate := range pathCandidates(name) {
		path, err := resolveExecutable(ctx, candidate)

		switch {
		case err == nil:
			return path, nil
		case errors.Is(err, errInactiveShim):
			if inactive == nil {
				inactive = err
			}
		default:
			return "", errors.Wrapf(err,
				"cannot resolve %s on PATH to a real executable; set %s to it", name, envVar)
		}
	}

	if inactive != nil {
		return "", errors.Wrapf(inactive,
			"cannot resolve %s on PATH to a real executable; set %s to it", name, envVar)
	}

	return "", nil
}

// SelectBinary resolves the driver's binary only when only (a
// comma-separated list of harness names, "" for all) selects it, so an
// excluded harness never runs a version manager.
func SelectBinary(driver Driver, only string) (binary string, selected bool, err error) {
	if only != "" && !slices.Contains(strings.Split(only, ","), driver.Name()) {
		return "", false, nil
	}

	return driver.Binary(), true, driver.BinaryError()
}

func resolveOverride(ctx context.Context, override string) (string, error) {
	abs, err := filepath.Abs(override)
	if err != nil {
		return "", errors.Wrap(err, "making the path absolute")
	}

	return resolveExecutable(ctx, abs)
}

// pathCandidates lists every executable named name on PATH, in order.
// Relative and repeated PATH entries are left out.
func pathCandidates(name string) []string {
	var found []string

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}

		path := filepath.Join(dir, name)
		if slices.Contains(found, path) {
			continue
		}

		if _, err := exec.LookPath(path); err == nil {
			found = append(found, path)
		}
	}

	return found
}

// resolveExecutable follows symlinks and resolves a version-manager shim to
// the executable it would run.
func resolveExecutable(ctx context.Context, path string) (string, error) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.Wrap(err, "resolving symlinks")
	}

	tool := filepath.Base(path)

	var manager string

	switch {
	case isMiseShim(path, target):
		manager = target
	case isAsdfShim(target):
		manager = asdfName
	default:
		return runnable(target)
	}

	shimmed, err := shimTarget(ctx, manager, tool)
	if err != nil {
		return "", errors.Wrapf(err, "%s is a %s shim", path, filepath.Base(manager))
	}

	resolved, err := filepath.EvalSymlinks(shimmed)
	if err != nil {
		return "", errors.Wrapf(err, "%s which %s returned %s",
			filepath.Base(manager), tool, shimmed)
	}

	if filepath.Base(resolved) == miseName || isMiseShim(shimmed, resolved) ||
		isAsdfShim(resolved) {
		return "", errors.Newf("%s which %s returned another shim: %s",
			filepath.Base(manager), tool, shimmed)
	}

	return runnable(resolved)
}

// runnable checks that path is an executable whose script interpreter, if
// any, exists on the sandbox PATH.
func runnable(path string) (string, error) {
	if _, err := exec.LookPath(path); err != nil {
		return "", errors.Wrapf(err, "%s is not an executable file", path)
	}

	interpreter := scriptInterpreter(readHeader(path))
	if interpreter == "" {
		return path, nil
	}

	if filepath.IsAbs(interpreter) {
		if _, err := exec.LookPath(interpreter); err != nil {
			return "", errors.Newf("%s needs %s, which is not an executable", path, interpreter)
		}

		return path, nil
	}

	for _, dir := range systemPath {
		if _, err := exec.LookPath(filepath.Join(dir, interpreter)); err == nil {
			return path, nil
		}
	}

	return "", errors.Newf("%s needs %s, which is not on the sandbox PATH (%s)",
		path, interpreter, strings.Join(systemPath, string(os.PathListSeparator)))
}

// scriptInterpreter returns the program a script's shebang runs: the
// absolute interpreter, or the name `env` looks up. "" for non-scripts.
func scriptInterpreter(header []byte) string {
	line, ok := bytes.CutPrefix(header, []byte("#!"))
	if !ok {
		return ""
	}

	line, _, _ = bytes.Cut(line, []byte("\n"))
	fields := strings.Fields(string(line))

	if len(fields) == 0 {
		return ""
	}

	if filepath.Base(fields[0]) != envName {
		return fields[0]
	}

	for _, field := range fields[1:] {
		if !strings.HasPrefix(field, "-") && !strings.Contains(field, "=") {
			return field
		}
	}

	return ""
}

// isMiseShim reports a mise shim: a link to the mise binary under the tool's
// name.
func isMiseShim(path, target string) bool {
	return filepath.Base(target) == miseName && filepath.Base(path) != miseName
}

// isAsdfShim reports an asdf shim: a script whose second line names its
// plugin, or that runs `asdf exec`.
func isAsdfShim(path string) bool {
	header := readHeader(path)
	if !bytes.HasPrefix(header, []byte("#!")) {
		return false
	}

	return bytes.Contains(header, []byte("\n# asdf-plugin: ")) ||
		bytes.Contains(header, []byte("asdf exec"))
}

func readHeader(path string) []byte {
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil
	}

	defer func() { _ = dir.Close() }()

	file, err := dir.Open(filepath.Base(path))
	if err != nil {
		return nil
	}

	defer func() { _ = file.Close() }()

	header := make([]byte, shimHeaderSize)
	n, _ := io.ReadFull(file, header)

	return header[:n]
}

// shimTarget asks the version manager which executable its shim runs. It
// runs in the caller's environment and directory, where the shim would. A
// mise tool that is not active is marked errInactiveShim.
func shimTarget(ctx context.Context, manager, tool string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, shimQueryTimeout)
	defer cancel()

	result := execpkg.NewCommandRunner(shimQueryTimeout).Run(ctx, manager, "which", tool)
	if result.Err != nil {
		err := errors.Wrapf(result.Err, "%s which %s: %s",
			filepath.Base(manager), tool, strings.TrimSpace(result.Stderr))

		if filepath.Base(manager) == miseName && strings.Contains(result.Stderr, miseInactive) {
			return "", errors.Mark(err, errInactiveShim)
		}

		return "", err
	}

	shimmed, _, _ := strings.Cut(strings.TrimSpace(result.Stdout), "\n")
	if !filepath.IsAbs(shimmed) {
		return "", errors.Newf("%s which %s printed no absolute path: %q",
			filepath.Base(manager), tool, result.Stdout)
	}

	return shimmed, nil
}
