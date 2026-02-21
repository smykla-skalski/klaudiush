package updater

// InstallMethod represents how klaudiush was installed.
type InstallMethod int

const (
	// InstallMethodDirect means the binary was installed directly (GitHub release, manual copy).
	InstallMethodDirect InstallMethod = iota
	// InstallMethodHomebrew means the binary was installed via homebrew.
	InstallMethodHomebrew
)

// String returns a human-readable name for the install method.
func (m InstallMethod) String() string {
	switch m {
	case InstallMethodHomebrew:
		return "homebrew"
	default:
		return "direct"
	}
}

// InstallInfo describes a discovered klaudiush binary.
type InstallInfo struct {
	// Path is the resolved (real) path to the binary.
	Path string
	// SymlinkPath is the original path before symlink resolution.
	// Empty if the path is not a symlink.
	SymlinkPath string
	// Method is how the binary was installed.
	Method InstallMethod
}

// DisplayPath returns SymlinkPath if set, otherwise Path.
func (i InstallInfo) DisplayPath() string {
	if i.SymlinkPath != "" {
		return i.SymlinkPath
	}

	return i.Path
}

// InstallStatus extends InstallInfo with version information.
type InstallStatus struct {
	InstallInfo
	CurrentVersion string
	LatestVersion  string
	Outdated       bool
}

// UpdateAllResult holds the outcome of updating a single binary in an UpdateAll operation.
type UpdateAllResult struct {
	InstallInfo
	Result  *UpdateResult
	Err     error
	Skipped bool
}
