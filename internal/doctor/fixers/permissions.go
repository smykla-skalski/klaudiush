package fixers

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/prompt"
)

const (
	binaryPermissions = 0o755
	configPermissions = 0o600
)

// PermissionsFixer fixes file permissions for binaries and config files.
type PermissionsFixer struct {
	prompter prompt.Prompter
}

// NewPermissionsFixer creates a new PermissionsFixer.
func NewPermissionsFixer(prompter prompt.Prompter) *PermissionsFixer {
	return &PermissionsFixer{
		prompter: prompter,
	}
}

// ID returns the fixer identifier.
func (*PermissionsFixer) ID() string {
	return "fix_permissions"
}

// Description returns a human-readable description.
func (*PermissionsFixer) Description() string {
	return "Fix file permissions for binaries and configuration files"
}

// CanFix checks if this fixer can fix the given result.
func (*PermissionsFixer) CanFix(result doctor.CheckResult) bool {
	return (result.FixID == "fix_permissions" || result.FixID == "fix_config_permissions") &&
		result.Status == doctor.StatusFail
}

// Fix corrects file permissions.
func (f *PermissionsFixer) Fix(_ context.Context, interactive bool) error {
	// Try to fix binary permissions first
	if err := f.fixBinaryPermissions(interactive); err != nil {
		return err
	}

	// Then try to fix config permissions
	return f.fixConfigPermissions(interactive)
}

func (f *PermissionsFixer) fixBinaryPermissions(interactive bool) error {
	path, err := exec.LookPath("klaudiush")
	if err != nil {
		// Binary not found, skip fixing permissions
		return nil //nolint:nilerr // Intentionally skip if binary not found
	}

	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "failed to stat binary")
	}

	actualPerms := info.Mode().Perm()
	if actualPerms == binaryPermissions {
		// Already correct
		return nil
	}

	if interactive {
		msg := fmt.Sprintf("Fix binary permissions (%04o -> %04o)?", actualPerms, binaryPermissions)

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	if err := os.Chmod(path, binaryPermissions); err != nil {
		return errors.Wrap(err, "failed to change binary permissions")
	}

	return nil
}

func (f *PermissionsFixer) fixConfigPermissions(interactive bool) error {
	loader, _ := config.NewKoanfLoader()

	// Check and fix global config
	if loader.HasGlobalConfig() {
		path := loader.GlobalConfigPath()
		if err := f.fixSingleConfigFile(path, interactive); err != nil {
			return errors.Wrap(err, "failed to fix global config permissions")
		}
	}

	// Check and fix project config
	if loader.HasProjectConfig() {
		paths := loader.ProjectConfigPaths()
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				if err := f.fixSingleConfigFile(path, interactive); err != nil {
					return errors.Wrap(err, "failed to fix project config permissions")
				}

				break
			}
		}
	}

	return nil
}

func (f *PermissionsFixer) fixSingleConfigFile(path string, interactive bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "failed to stat config file")
	}

	actualPerms := info.Mode().Perm()

	// Check if world-writable
	if actualPerms&0o002 == 0 {
		// Not world-writable, skip
		return nil
	}

	if interactive {
		msg := fmt.Sprintf(
			"Fix config permissions for %s (%04o -> %04o)?",
			path,
			actualPerms,
			configPermissions,
		)

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	if err := os.Chmod(path, configPermissions); err != nil {
		return errors.Wrap(err, "failed to change config permissions")
	}

	return nil
}
