// Package xdg provides centralized path management following XDG Base Directory conventions.
// All global/user-level paths klaudiush touches on disk are defined here.
// Project-local paths (.klaudiush/config.toml, .klaudiush/patterns.json) remain in internal/config.
package xdg

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

const appName = "klaudiush"

func userHome() (string, error) {
	return os.UserHomeDir()
}

// --- XDG base directory functions ---

// ConfigHome returns $XDG_CONFIG_HOME or ~/.config.
func ConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}

	home, err := userHome()
	if err != nil {
		return filepath.Join("~", ".config")
	}

	return filepath.Join(home, ".config")
}

// DataHome returns $XDG_DATA_HOME or ~/.local/share.
func DataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}

	home, err := userHome()
	if err != nil {
		return filepath.Join("~", ".local", "share")
	}

	return filepath.Join(home, ".local", "share")
}

// StateHome returns $XDG_STATE_HOME or ~/.local/state.
func StateHome() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v
	}

	home, err := userHome()
	if err != nil {
		return filepath.Join("~", ".local", "state")
	}

	return filepath.Join(home, ".local", "state")
}

// CacheHome returns $XDG_CACHE_HOME or ~/.cache.
func CacheHome() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}

	home, err := userHome()
	if err != nil {
		return filepath.Join("~", ".cache")
	}

	return filepath.Join(home, ".cache")
}

// LegacyDir returns ~/.klaudiush (the pre-XDG location).
func LegacyDir() string {
	home, err := userHome()
	if err != nil {
		return filepath.Join("~", ".klaudiush")
	}

	return filepath.Join(home, ".klaudiush")
}

// LegacyLogFile returns ~/.claude/hooks/dispatcher.log (the pre-XDG log location).
func LegacyLogFile() string {
	home, err := userHome()
	if err != nil {
		return filepath.Join("~", ".claude", "hooks", "dispatcher.log")
	}

	return filepath.Join(home, ".claude", "hooks", "dispatcher.log")
}

// --- klaudiush-specific directories ---

// ConfigDir returns ConfigHome()/klaudiush.
func ConfigDir() string {
	return filepath.Join(ConfigHome(), appName)
}

// DataDir returns DataHome()/klaudiush.
func DataDir() string {
	return filepath.Join(DataHome(), appName)
}

// StateDir returns StateHome()/klaudiush.
func StateDir() string {
	return filepath.Join(StateHome(), appName)
}

// --- Specific file paths ---

// GlobalConfigFile returns ConfigDir()/config.toml.
func GlobalConfigFile() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

// LogFile returns the log file path.
// Respects KLAUDIUSH_LOG_FILE env var, otherwise StateDir()/dispatcher.log.
func LogFile() string {
	if v := os.Getenv("KLAUDIUSH_LOG_FILE"); v != "" {
		return v
	}

	return filepath.Join(StateDir(), "dispatcher.log")
}

// ExceptionStateFile returns DataDir()/exceptions/state.json.
func ExceptionStateFile() string {
	return filepath.Join(DataDir(), "exceptions", "state.json")
}

// ExceptionAuditFile returns StateDir()/exception_audit.jsonl.
func ExceptionAuditFile() string {
	return filepath.Join(StateDir(), "exception_audit.jsonl")
}

// CrashDumpDir returns DataDir()/crash_dumps.
func CrashDumpDir() string {
	return filepath.Join(DataDir(), "crash_dumps")
}

// PatternsGlobalDir returns DataDir()/patterns.
func PatternsGlobalDir() string {
	return filepath.Join(DataDir(), "patterns")
}

// BackupDir returns DataDir()/backups.
func BackupDir() string {
	return filepath.Join(DataDir(), "backups")
}

// PluginDir returns DataDir()/plugins.
func PluginDir() string {
	return filepath.Join(DataDir(), "plugins")
}

// MigrationMarker returns StateDir()/.migration_v2.
func MigrationMarker() string {
	return filepath.Join(StateDir(), ".migration_v2")
}

// --- Utility functions ---

// ExpandPath resolves ~ prefix to the user's home directory.
// Returns the path unchanged if it doesn't start with ~.
// Returns error for invalid tilde usage like "~foo".
func ExpandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}

	home, err := userHome()
	if err != nil {
		return "", errors.Wrap(err, "failed to get home directory")
	}

	switch {
	case path == "~":
		return home, nil
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:]), nil
	default:
		return "", errors.Newf("paths starting with ~ must be either ~ or ~/subdir, got %q", path)
	}
}

// ExpandPathSilent resolves ~ prefix, returning the original path on error.
// Use this for cases where failing gracefully is preferred over returning an error.
func ExpandPathSilent(path string) string {
	expanded, err := ExpandPath(path)
	if err != nil {
		return path
	}

	return expanded
}

// EnsureDir creates a directory with 0700 permissions if it doesn't exist,
// and fixes permissions on existing directories if they're too open.
func EnsureDir(path string) error {
	const dirMode = 0o700

	if err := os.MkdirAll(path, dirMode); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", path)
	}

	// MkdirAll only sets perms on new dirs. Fix existing ones if too open.
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "failed to stat directory %s", path)
	}

	if info.Mode().Perm() != dirMode {
		if err := os.Chmod(path, dirMode); err != nil {
			return errors.Wrapf(err, "failed to set permissions on %s", path)
		}
	}

	return nil
}
