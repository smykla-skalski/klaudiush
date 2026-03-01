// Package xdgchecker provides health checkers for XDG base directory compliance.
package xdgchecker

import (
	"context"
	"fmt"
	"os"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

const dirPerm = 0o700

// LegacyPathChecker detects unmigrated files in ~/.klaudiush/.
type LegacyPathChecker struct{}

// NewLegacyPathChecker creates a new legacy path checker.
func NewLegacyPathChecker() *LegacyPathChecker {
	return &LegacyPathChecker{}
}

// Name returns the name of the check.
func (*LegacyPathChecker) Name() string {
	return "XDG migration status"
}

// Category returns the category of the check.
func (*LegacyPathChecker) Category() doctor.Category {
	return doctor.CategoryXDG
}

// Check detects whether legacy paths still need migration.
func (*LegacyPathChecker) Check(_ context.Context) doctor.CheckResult {
	if !xdg.NeedsMigration() {
		return doctor.Pass("XDG migration status", "Migration complete or fresh install")
	}

	return doctor.FailWarning("XDG migration status", "Legacy paths detected").
		WithDetails(
			"Legacy directory: "+xdg.LegacyDir(),
			"Files should be migrated to XDG locations",
			"Run: klaudiush doctor --fix --category xdg",
		).
		WithFixID("migrate_xdg")
}

// DirChecker verifies XDG directories exist with correct permissions.
type DirChecker struct{}

// NewDirChecker creates a new XDG directory checker.
func NewDirChecker() *DirChecker {
	return &DirChecker{}
}

// Name returns the name of the check.
func (*DirChecker) Name() string {
	return "XDG directories"
}

// Category returns the category of the check.
func (*DirChecker) Category() doctor.Category {
	return doctor.CategoryXDG
}

// Check verifies XDG directories exist and have 0700 permissions.
func (*DirChecker) Check(_ context.Context) doctor.CheckResult {
	dirs := []struct {
		name string
		path string
	}{
		{"config", xdg.ConfigDir()},
		{"data", xdg.DataDir()},
		{"state", xdg.StateDir()},
	}

	var missing []string

	var badPerms []string

	for _, d := range dirs {
		info, err := os.Stat(d.path)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, d.name+": "+d.path)

				continue
			}

			return doctor.FailError("XDG directories",
				fmt.Sprintf("Failed to stat %s: %v", d.path, err))
		}

		if !info.IsDir() {
			return doctor.FailError("XDG directories", d.path+" exists but is not a directory")
		}

		perm := info.Mode().Perm()
		if perm != dirPerm {
			badPerms = append(badPerms,
				fmt.Sprintf("%s: %04o (expected %04o)", d.path, perm, dirPerm))
		}
	}

	if len(missing) > 0 || len(badPerms) > 0 {
		var details []string

		if len(missing) > 0 {
			details = append(details, "Missing directories:")
			details = append(details, missing...)
		}

		if len(badPerms) > 0 {
			details = append(details, "Permission issues:")
			details = append(details, badPerms...)
		}

		details = append(details, "Run: klaudiush doctor --fix --category xdg")

		msg := "Some XDG directories missing"
		if len(missing) == 0 {
			msg = "Directory permissions not secure"
		} else if len(badPerms) > 0 {
			msg = "XDG directory issues found"
		}

		return doctor.FailWarning("XDG directories", msg).
			WithDetails(details...).
			WithFixID("create_xdg_dirs")
	}

	return doctor.Pass("XDG directories", "All directories present with secure permissions")
}
