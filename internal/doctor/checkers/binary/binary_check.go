// Package binary provides checkers for klaudiush binary validation.
package binary

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
)

const (
	binaryName          = "klaudiush"
	expectedPermissions = 0o755
)

// ExistsChecker checks if the klaudiush binary exists and is accessible in PATH
type ExistsChecker struct{}

// NewExistsChecker creates a new binary exists checker
func NewExistsChecker() *ExistsChecker {
	return &ExistsChecker{}
}

// Name returns the name of the check
func (*ExistsChecker) Name() string {
	return "Binary available and executable"
}

// Category returns the category of the check
func (*ExistsChecker) Category() doctor.Category {
	return doctor.CategoryBinary
}

// Check performs the binary existence check
func (*ExistsChecker) Check(_ context.Context) doctor.CheckResult {
	path, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.FailError("Binary available and executable", "klaudiush binary not found in PATH").
			WithDetails(
				"The klaudiush binary must be installed and available in your PATH",
				"Install with: mise run install",
			).
			WithFixID("install_binary")
	}

	return doctor.Pass("Binary available and executable", "Found at "+path)
}

// PermissionsChecker checks if the klaudiush binary has correct permissions
type PermissionsChecker struct{}

// NewPermissionsChecker creates a new permissions checker
func NewPermissionsChecker() *PermissionsChecker {
	return &PermissionsChecker{}
}

// Name returns the name of the check
func (*PermissionsChecker) Name() string {
	return "Correct permissions"
}

// Category returns the category of the check
func (*PermissionsChecker) Category() doctor.Category {
	return doctor.CategoryBinary
}

// Check performs the permissions check
func (*PermissionsChecker) Check(_ context.Context) doctor.CheckResult {
	path, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip("Correct permissions", "Binary not found")
	}

	info, err := os.Stat(path)
	if err != nil {
		return doctor.FailError("Correct permissions", fmt.Sprintf("Cannot stat binary: %v", err))
	}

	actualPerms := info.Mode().Perm()
	if actualPerms != expectedPermissions {
		return doctor.FailWarning(
			"Correct permissions",
			fmt.Sprintf(
				"Binary has permissions %04o, expected %04o",
				actualPerms,
				expectedPermissions,
			),
		).
			WithDetails(
				"Binary location: "+path,
				fmt.Sprintf("Fix with: chmod %04o %s", expectedPermissions, path),
			).
			WithFixID("fix_permissions")
	}

	return doctor.Pass("Correct permissions", fmt.Sprintf("%04o", actualPerms))
}

// LocationChecker checks the location of the klaudiush binary
type LocationChecker struct{}

// NewLocationChecker creates a new location checker
func NewLocationChecker() *LocationChecker {
	return &LocationChecker{}
}

// Name returns the name of the check
func (*LocationChecker) Name() string {
	return "Binary location"
}

// Category returns the category of the check
func (*LocationChecker) Category() doctor.Category {
	return doctor.CategoryBinary
}

// Check performs the location check
func (*LocationChecker) Check(_ context.Context) doctor.CheckResult {
	path, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip("Binary location", "Binary not found")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// Check if it's in a standard location
	homeDir, _ := os.UserHomeDir()
	standardLocations := []string{
		filepath.Join(homeDir, ".local", "bin", binaryName),
		filepath.Join(homeDir, ".claude", "hooks", binaryName),
		filepath.Join("/usr", "local", "bin", binaryName),
		filepath.Join("/usr", "bin", binaryName),
	}

	isStandard := slices.Contains(standardLocations, absPath)

	result := doctor.Pass("Binary location", absPath)
	if !isStandard {
		result = result.WithDetails("Note: Binary is not in a standard location")
	}

	return result
}
