package harness

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

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
)

const (
	shimQueryTimeout = 30 * time.Second
	shimHeaderSize   = 512
	miseName         = "mise"
	asdfName         = "asdf"
)

// ResolveBinary finds a harness executable: the override variable, then
// PATH. Symlinks are resolved so the sandbox PATH does not need the caller's
// PATH entries. Version-manager shims (mise, asdf) cannot run in the empty
// sandbox environment, so they are resolved to the real executable here, in
// the caller's environment. A shim whose tool is not active is skipped the
// way the shim itself falls through to the next PATH entry. It returns ""
// and no error when the harness is not installed.
func ResolveBinary(envVar, name string) (string, error) {
	if override := os.Getenv(envVar); override != "" {
		path, err := resolveExecutable(override)
		if err != nil {
			return "", errors.Wrapf(err, "%s=%s", envVar, override)
		}

		return path, nil
	}

	var shimErr error

	for _, candidate := range pathCandidates(name) {
		path, err := resolveExecutable(candidate)
		if err == nil {
			return path, nil
		}

		if shimErr == nil {
			shimErr = err
		}
	}

	if shimErr != nil {
		return "", errors.Wrapf(shimErr,
			"cannot resolve %s on PATH to a real executable; set %s to it", name, envVar)
	}

	return "", nil
}

// pathCandidates lists every executable named name on PATH, in order.
// Relative PATH entries are left out, like exec.LookPath.
func pathCandidates(name string) []string {
	var found []string

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}

		path := filepath.Join(dir, name)
		if _, err := exec.LookPath(path); err == nil {
			found = append(found, path)
		}
	}

	return found
}

// resolveExecutable follows symlinks and resolves a version-manager shim to
// the executable it would run.
func resolveExecutable(path string) (string, error) {
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
		return executable(target)
	}

	shimmed, err := shimTarget(manager, tool)
	if err != nil {
		return "", errors.Wrapf(err, "%s is a %s shim", path, filepath.Base(manager))
	}

	resolved, err := filepath.EvalSymlinks(shimmed)
	if err != nil {
		return "", errors.Wrapf(err, "%s which %s returned %s",
			filepath.Base(manager), tool, shimmed)
	}

	if isMiseShim(shimmed, resolved) || isAsdfShim(resolved) {
		return "", errors.Newf("%s which %s returned another shim: %s",
			filepath.Base(manager), tool, shimmed)
	}

	return executable(resolved)
}

func executable(path string) (string, error) {
	if _, err := exec.LookPath(path); err != nil {
		return "", errors.Wrapf(err, "%s is not an executable file", path)
	}

	return path, nil
}

// isMiseShim reports a mise shim: a link to the mise binary under the tool's
// name.
func isMiseShim(path, target string) bool {
	return filepath.Base(target) == miseName && filepath.Base(path) != miseName
}

// isAsdfShim reports an asdf shim: a script that runs `asdf exec`.
func isAsdfShim(path string) bool {
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return false
	}

	defer func() { _ = dir.Close() }()

	file, err := dir.Open(filepath.Base(path))
	if err != nil {
		return false
	}

	defer func() { _ = file.Close() }()

	header := make([]byte, shimHeaderSize)
	n, _ := io.ReadFull(file, header)

	return bytes.HasPrefix(header[:n], []byte("#!")) &&
		bytes.Contains(header[:n], []byte("asdf exec"))
}

// shimTarget asks the version manager which executable its shim runs. It
// runs in the caller's environment and directory, where the shim would.
func shimTarget(manager, tool string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shimQueryTimeout)
	defer cancel()

	result := execpkg.NewCommandRunner(shimQueryTimeout).Run(ctx, manager, "which", tool)
	if result.Err != nil {
		return "", errors.Wrapf(result.Err, "%s which %s: %s",
			filepath.Base(manager), tool, strings.TrimSpace(result.Stderr))
	}

	shimmed, _, _ := strings.Cut(strings.TrimSpace(result.Stdout), "\n")
	if !filepath.IsAbs(shimmed) {
		return "", errors.Newf("%s which %s printed no absolute path: %q",
			filepath.Base(manager), tool, result.Stdout)
	}

	return shimmed, nil
}
