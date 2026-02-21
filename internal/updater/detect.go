package updater

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/exec"
)

// Detector discovers klaudiush binaries and determines their install method.
type Detector struct {
	runner exec.CommandRunner
}

// NewDetector creates a new Detector.
func NewDetector(runner exec.CommandRunner) *Detector {
	return &Detector{runner: runner}
}

// isHomebrewPath returns true if the resolved path looks like a homebrew-managed binary.
func isHomebrewPath(resolved string) bool {
	// Homebrew installs live under Cellar or the homebrew/linuxbrew prefix.
	// Check for common patterns:
	//   /opt/homebrew/Cellar/...         (macOS ARM)
	//   /usr/local/Cellar/...            (macOS Intel)
	//   /home/linuxbrew/.linuxbrew/Cellar/... (Linuxbrew)
	//   ~/.linuxbrew/Cellar/...          (user Linuxbrew)
	lower := strings.ToLower(resolved)

	return strings.Contains(lower, "/cellar/") ||
		strings.Contains(lower, "/homebrew/") ||
		strings.Contains(lower, "/linuxbrew/")
}

// DetectMethod determines the install method from a resolved binary path.
func (*Detector) DetectMethod(binaryPath string) InstallMethod {
	resolved, err := filepath.EvalSymlinks(binaryPath)
	if err != nil {
		// Can't resolve - assume direct.
		return InstallMethodDirect
	}

	if isHomebrewPath(resolved) {
		return InstallMethodHomebrew
	}

	return InstallMethodDirect
}

// DetectCurrent returns install info for the currently running binary.
func (*Detector) DetectCurrent() (*InstallInfo, error) {
	resolved, err := CurrentBinaryPath()
	if err != nil {
		return nil, err
	}

	exe, exeErr := os.Executable()
	if exeErr != nil {
		return nil, errors.Wrap(exeErr, "getting executable path")
	}

	info := &InstallInfo{
		Path:   resolved,
		Method: InstallMethodDirect,
	}

	if isHomebrewPath(resolved) {
		info.Method = InstallMethodHomebrew
	}

	// If the original path differs from resolved, it was a symlink.
	if exe != resolved {
		info.SymlinkPath = exe
	}

	return info, nil
}

// FindAll discovers all klaudiush binaries in PATH.
// Deduplicates by resolved (real) path.
func (d *Detector) FindAll(ctx context.Context) ([]InstallInfo, error) {
	infos, err := d.findViaWhich(ctx)
	if err != nil {
		// Fall back to manual PATH scan.
		infos, err = d.scanPATH()
		if err != nil {
			return nil, errors.Wrap(err, "scanning PATH for binaries")
		}
	}

	return deduplicateByPath(infos), nil
}

// findViaWhich uses "which -a" to find all klaudiush binaries.
func (d *Detector) findViaWhich(ctx context.Context) ([]InstallInfo, error) {
	result := d.runner.Run(ctx, "which", "-a", BinaryName)
	if result.Failed() {
		return nil, errors.Wrap(result.Err, "which -a")
	}

	return d.parsePathList(result.Stdout), nil
}

// scanPATH manually scans $PATH directories for the binary.
func (d *Detector) scanPATH() ([]InstallInfo, error) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil, errors.New("PATH is empty")
	}

	var infos []InstallInfo

	for _, dir := range filepath.SplitList(pathEnv) {
		candidate := filepath.Join(dir, BinaryName)

		//nolint:gosec // G703: candidate is from PATH + known binary name
		fi, err := os.Stat(candidate)
		if err != nil {
			continue
		}

		if fi.IsDir() {
			continue
		}

		info := d.buildInstallInfo(candidate)
		infos = append(infos, info)
	}

	if len(infos) == 0 {
		return nil, errors.New("no klaudiush binary found in PATH")
	}

	return infos, nil
}

// parsePathList turns newline-separated paths into InstallInfo entries.
func (d *Detector) parsePathList(output string) []InstallInfo {
	var infos []InstallInfo

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		infos = append(infos, d.buildInstallInfo(line))
	}

	return infos
}

// buildInstallInfo resolves a binary path and determines its install method.
func (*Detector) buildInstallInfo(path string) InstallInfo {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// Can't resolve symlink - use path as-is.
		return InstallInfo{
			Path:   path,
			Method: InstallMethodDirect,
		}
	}

	info := InstallInfo{
		Path:   resolved,
		Method: InstallMethodDirect,
	}

	if isHomebrewPath(resolved) {
		info.Method = InstallMethodHomebrew
	}

	if resolved != path {
		info.SymlinkPath = path
	}

	return info
}

// deduplicateByPath removes entries that resolve to the same real path.
// Keeps the first occurrence.
func deduplicateByPath(infos []InstallInfo) []InstallInfo {
	seen := make(map[string]bool, len(infos))
	result := make([]InstallInfo, 0, len(infos))

	for _, info := range infos {
		if seen[info.Path] {
			continue
		}

		seen[info.Path] = true

		result = append(result, info)
	}

	return result
}
