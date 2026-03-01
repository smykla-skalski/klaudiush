package xdg

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// MigrationResult tracks what the migration did.
type MigrationResult struct {
	Moved    int
	Symlinks int
	Skipped  int
	Warnings []string
}

// migrationEntry maps a legacy file/dir to its XDG destination.
type migrationEntry struct {
	legacySuffix string // relative to ~/.klaudiush/
	xdgDest      func() string
	isDir        bool
}

var migrationEntries = []migrationEntry{
	// config -> XDG_CONFIG_HOME
	{legacySuffix: "config.toml", xdgDest: GlobalConfigFile, isDir: false},
	// data -> XDG_DATA_HOME
	{
		legacySuffix: "exceptions",
		xdgDest:      func() string { return filepath.Dir(ExceptionStateFile()) },
		isDir:        true,
	},
	{legacySuffix: "crash_dumps", xdgDest: CrashDumpDir, isDir: true},
	{legacySuffix: "patterns", xdgDest: PatternsGlobalDir, isDir: true},
	{legacySuffix: ".backups", xdgDest: BackupDir, isDir: true},
	{legacySuffix: "plugins", xdgDest: PluginDir, isDir: true},
	// state -> XDG_STATE_HOME
	{legacySuffix: "exception_audit.jsonl", xdgDest: ExceptionAuditFile, isDir: false},
}

// NeedsMigration returns true if ~/.klaudiush exists and the v2 marker is absent.
func NeedsMigration() bool {
	// If v2 marker exists, migration is done
	if fileExists(MigrationMarker()) {
		return false
	}

	// If legacy dir doesn't exist, nothing to migrate
	return dirExists(LegacyDir())
}

// Migrate moves files from ~/.klaudiush/ to XDG directories.
// Idempotent: skips files that already exist at the destination.
// Creates symlinks for config.toml and dispatcher.log after moving.
// Uses file locking to prevent races from parallel invocations.
func Migrate(log logger.Logger) (*MigrationResult, error) {
	result := &MigrationResult{}

	// If marker exists, already done
	if fileExists(MigrationMarker()) {
		return result, nil
	}

	// Acquire lock to prevent parallel migrations (TOCTOU between NeedsMigration and Migrate).
	lockPath := MigrationMarker() + ".lock"

	if err := EnsureDir(filepath.Dir(lockPath)); err != nil {
		return result, errors.Wrap(err, "creating lock directory")
	}

	const lockPerm = 0o600

	lockFlags := os.O_CREATE | os.O_EXCL | os.O_WRONLY

	//nolint:gosec // lockPath is from MigrationMarker()
	lockFile, err := os.OpenFile(lockPath, lockFlags, lockPerm)
	if err != nil {
		// Another process is migrating or lock is stale - skip gracefully
		log.Debug("migration lock already held, skipping", "lock", lockPath)

		return result, nil
	}

	_ = lockFile.Close()

	defer func() { _ = os.Remove(lockPath) }()

	// Re-check marker after acquiring lock (another process may have finished)
	if fileExists(MigrationMarker()) {
		return result, nil
	}

	legacy := LegacyDir()

	// Fresh install - no legacy dir exists
	if !dirExists(legacy) {
		return result, writeMigrationMarker()
	}

	// Create XDG directories
	for _, dir := range []string{ConfigDir(), DataDir(), StateDir()} {
		if err := EnsureDir(dir); err != nil {
			return result, errors.Wrapf(err, "creating XDG directory %s", dir)
		}
	}

	// Move files by category
	for _, entry := range migrationEntries {
		migrateEntry(legacy, entry, result, log)
	}

	// Create symlinks for backward compat
	createBackwardCompatSymlinks(legacy, result, log)

	// Write v2 marker
	if err := writeMigrationMarker(); err != nil {
		return result, errors.Wrap(err, "writing migration marker")
	}

	return result, nil
}

// migrateEntry processes a single migration entry (file or directory).
func migrateEntry(legacy string, entry migrationEntry, result *MigrationResult, log logger.Logger) {
	src := filepath.Join(legacy, entry.legacySuffix)
	dest := entry.xdgDest()

	if entry.isDir {
		migrateDirEntry(src, dest, result, log)

		return
	}

	migrateFileEntry(src, dest, result, log)
}

// migrateDirEntry handles migration of a single directory entry.
func migrateDirEntry(src, dest string, result *MigrationResult, log logger.Logger) {
	moved, skipped, err := migrateDir(src, dest, log)
	result.Moved += moved
	result.Skipped += skipped

	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
}

// migrateFileEntry handles migration of a single file entry.
func migrateFileEntry(src, dest string, result *MigrationResult, log logger.Logger) {
	moved, err := migrateFile(src, dest, log)
	if moved {
		result.Moved++
	} else {
		result.Skipped++
	}

	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
}

// createBackwardCompatSymlinks creates symlinks from legacy locations to XDG paths.
func createBackwardCompatSymlinks(legacy string, result *MigrationResult, log logger.Logger) {
	if err := createSymlink(
		GlobalConfigFile(),
		filepath.Join(legacy, "config.toml"),
		log,
	); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	} else {
		result.Symlinks++
	}

	// Always symlink to the standard XDG log path, not LogFile() which respects KLAUDIUSH_LOG_FILE.
	xdgLogFile := filepath.Join(StateDir(), "dispatcher.log")

	if err := createSymlink(xdgLogFile, LegacyLogFile(), log); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	} else {
		result.Symlinks++
	}
}

// migrateFile moves a single file from src to dest if src exists and dest doesn't.
func migrateFile(src, dest string, log logger.Logger) (moved bool, err error) {
	if !fileExists(src) {
		return false, nil
	}

	if fileExists(dest) {
		log.Debug("skipping migration, destination exists", "src", src, "dest", dest)

		return false, nil
	}

	// Ensure destination directory exists
	if err := EnsureDir(filepath.Dir(dest)); err != nil {
		return false, errors.Wrapf(err, "creating directory for %s", dest)
	}

	if err := os.Rename(src, dest); err != nil {
		// Cross-device: copy + remove
		if copyErr := copyAndRemove(src, dest); copyErr != nil {
			return false, copyErr
		}

		log.Info("migrated file (cross-device)", "from", src, "to", dest)

		return true, nil
	}

	log.Info("migrated file", "from", src, "to", dest)

	return true, nil
}

// migrateDir moves a directory from src to dest if src exists and dest doesn't.
func migrateDir(src, dest string, log logger.Logger) (moved, skipped int, err error) {
	if !dirExists(src) {
		return 0, 1, nil
	}

	if dirExists(dest) {
		log.Debug("skipping directory migration, destination exists", "src", src, "dest", dest)

		return 0, 1, nil
	}

	if err := EnsureDir(filepath.Dir(dest)); err != nil {
		return 0, 0, errors.Wrapf(err, "creating parent directory for %s", dest)
	}

	if err := os.Rename(src, dest); err != nil {
		// Cross-device: copy entries individually + remove source dir
		if copyErr := copyDirAndRemove(src, dest); copyErr != nil {
			return 0, 0, errors.Wrapf(copyErr, "moving directory %s to %s", src, dest)
		}

		log.Info("migrated directory (cross-device)", "from", src, "to", dest)

		return 1, 0, nil
	}

	log.Info("migrated directory", "from", src, "to", dest)

	return 1, 0, nil
}

// createSymlink creates a symlink from legacy location pointing to the XDG target.
// Only creates the symlink if the XDG target exists and legacy location doesn't.
func createSymlink(xdgTarget, legacyPath string, log logger.Logger) error {
	if !fileExists(xdgTarget) && !dirExists(xdgTarget) {
		return nil
	}

	// Don't overwrite existing files/symlinks
	if _, err := os.Lstat(legacyPath); err == nil {
		return nil
	}

	// Ensure parent directory exists
	if err := EnsureDir(filepath.Dir(legacyPath)); err != nil {
		return errors.Wrapf(err, "creating symlink parent directory")
	}

	if err := os.Symlink(xdgTarget, legacyPath); err != nil {
		return errors.Wrapf(err, "creating symlink %s -> %s", legacyPath, xdgTarget)
	}

	log.Info("created symlink", "link", legacyPath, "target", xdgTarget)

	return nil
}

func writeMigrationMarker() error {
	marker := MigrationMarker()

	if err := EnsureDir(filepath.Dir(marker)); err != nil {
		return err
	}

	const markerPerm = 0o600

	return os.WriteFile(marker, []byte("v2"), markerPerm)
}

func copyAndRemove(src, dest string) error {
	data, err := os.ReadFile(src) //nolint:gosec // G304: src is from internal migration paths
	if err != nil {
		return errors.Wrapf(err, "reading %s", src)
	}

	info, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "stat %s", src)
	}

	if err := os.WriteFile(dest, data, info.Mode()); err != nil {
		return errors.Wrapf(err, "writing %s", dest)
	}

	if err := os.Remove(src); err != nil {
		return errors.Wrapf(err, "removing source %s after copy to %s", src, dest)
	}

	return nil
}

// copyDirAndRemove copies a directory tree from src to dest, then removes src.
// Used as cross-device fallback when os.Rename fails.
func copyDirAndRemove(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return errors.Wrapf(err, "reading directory %s", src)
	}

	if err := EnsureDir(dest); err != nil {
		return errors.Wrapf(err, "creating destination directory %s", dest)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := copyDirAndRemove(srcPath, destPath); err != nil {
				return err
			}

			continue
		}

		if err := copyAndRemove(srcPath, destPath); err != nil {
			return err
		}
	}

	return os.Remove(src)
}
