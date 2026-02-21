// Package config provides internal configuration loading and processing.
package config

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/pelletier/go-toml/v2"

	"github.com/smykla-skalski/klaudiush/internal/backup"
	"github.com/smykla-skalski/klaudiush/internal/schema"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	// ConfigFileMode is the file mode for configuration files (user read/write only).
	ConfigFileMode = 0o600

	// ConfigDirMode is the file mode for configuration directories (user rwx only).
	ConfigDirMode = 0o700
)

// Writer handles writing configuration to TOML files.
type Writer struct {
	// homeDir is the user's home directory (for testing).
	homeDir string

	// workDir is the current working directory (for testing).
	workDir string

	// backupManager handles automatic backups before config changes (optional).
	backupManager *backup.Manager
}

// NewWriter creates a new Writer with default directories.
func NewWriter() *Writer {
	return &Writer{
		homeDir: os.Getenv("HOME"),
		workDir: mustGetwd(),
	}
}

// NewWriterWithBackup creates a new Writer with default directories and a backup manager.
func NewWriterWithBackup(backupMgr *backup.Manager) *Writer {
	return &Writer{
		homeDir:       os.Getenv("HOME"),
		workDir:       mustGetwd(),
		backupManager: backupMgr,
	}
}

// NewWriterWithDirs creates a new Writer with custom directories (for testing).
func NewWriterWithDirs(homeDir, workDir string) *Writer {
	return &Writer{
		homeDir: homeDir,
		workDir: workDir,
	}
}

// NewWriterWithDirsAndBackup creates a new Writer with custom directories and backup manager (for testing).
func NewWriterWithDirsAndBackup(homeDir, workDir string, backupMgr *backup.Manager) *Writer {
	return &Writer{
		homeDir:       homeDir,
		workDir:       workDir,
		backupManager: backupMgr,
	}
}

// WriteGlobal writes the configuration to the global config file.
func (w *Writer) WriteGlobal(cfg *config.Config) error {
	path := w.GlobalConfigPath()

	return w.WriteFile(path, cfg)
}

// WriteProject writes the configuration to the project config file.
// Uses the primary location (.klaudiush/config.toml).
func (w *Writer) WriteProject(cfg *config.Config) error {
	path := w.ProjectConfigPath()

	return w.WriteFile(path, cfg)
}

// WriteFile writes the configuration to the given path.
func (w *Writer) WriteFile(path string, cfg *config.Config) error {
	if cfg == nil {
		return errors.Wrap(ErrInvalidConfig, "config is nil")
	}

	// Backup existing config if enabled
	if err := w.backupBeforeWrite(path, cfg); err != nil {
		return errors.Wrap(err, "failed to backup config before write")
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, ConfigDirMode); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", dir)
	}

	// Marshal to TOML with indentation
	var buf bytes.Buffer

	// Prepend Taplo schema directive
	buf.WriteString(schema.SchemaDirective())
	buf.WriteByte('\n')

	encoder := toml.NewEncoder(&buf)
	encoder.SetIndentTables(true)

	if err := encoder.Encode(cfg); err != nil {
		return errors.Wrap(err, "failed to encode config to TOML")
	}

	// Write to file with secure permissions
	if err := os.WriteFile(path, buf.Bytes(), ConfigFileMode); err != nil {
		return errors.Wrapf(err, "failed to write config file %s", path)
	}

	return nil
}

// backupBeforeWrite creates a backup of the config file before writing if enabled.
func (w *Writer) backupBeforeWrite(path string, cfg *config.Config) error {
	// Skip if no backup manager
	if w.backupManager == nil {
		return nil
	}

	// Skip if backup is disabled or auto_backup is disabled
	if cfg.Backup == nil || !cfg.Backup.IsEnabled() || !cfg.Backup.IsAutoBackupEnabled() {
		return nil
	}

	// Skip if file doesn't exist (nothing to backup)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// Prepare backup options
	opts := backup.CreateBackupOptions{
		ConfigPath: path,
		Trigger:    backup.TriggerAutomatic,
		Metadata:   backup.SnapshotMetadata{},
	}

	// Create backup (async or sync based on config)
	if cfg.Backup.IsAsyncBackupEnabled() {
		// Async: run in background, don't wait for completion
		go func() {
			_, _ = w.backupManager.CreateBackup(opts)
		}()
	} else {
		// Sync: wait for completion
		if _, err := w.backupManager.CreateBackup(opts); err != nil {
			return errors.Wrap(err, "backup failed")
		}
	}

	return nil
}

// GlobalConfigPath returns the path to the global configuration file.
func (w *Writer) GlobalConfigPath() string {
	return filepath.Join(w.homeDir, GlobalConfigDir, GlobalConfigFile)
}

// ProjectConfigPath returns the path to the primary project configuration file.
func (w *Writer) ProjectConfigPath() string {
	return filepath.Join(w.workDir, ProjectConfigDir, ProjectConfigFile)
}

// EnsureGlobalConfigDir ensures the global config directory exists.
func (w *Writer) EnsureGlobalConfigDir() error {
	dir := filepath.Join(w.homeDir, GlobalConfigDir)

	if err := os.MkdirAll(dir, ConfigDirMode); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", dir)
	}

	return nil
}

// EnsureProjectConfigDir ensures the project config directory exists.
func (w *Writer) EnsureProjectConfigDir() error {
	dir := filepath.Join(w.workDir, ProjectConfigDir)

	if err := os.MkdirAll(dir, ConfigDirMode); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", dir)
	}

	return nil
}

// IsGlobalConfigExists checks if the global config file exists.
func (w *Writer) IsGlobalConfigExists() bool {
	path := w.GlobalConfigPath()
	_, err := os.Stat(path)

	return err == nil
}

// IsProjectConfigExists checks if the project config file exists.
func (w *Writer) IsProjectConfigExists() bool {
	path := w.ProjectConfigPath()
	_, err := os.Stat(path)

	return err == nil
}

// GlobalConfigDir returns the global config directory path.
func (w *Writer) GlobalConfigDir() string {
	return filepath.Join(w.homeDir, GlobalConfigDir)
}

// ProjectConfigDir returns the project config directory path.
func (w *Writer) ProjectConfigDir() string {
	return filepath.Join(w.workDir, ProjectConfigDir)
}
