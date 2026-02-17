// Package session provides session state tracking for Claude Code hooks.
package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// Audit file permission constants.
const (
	// auditFilePermissions is the permission mode for the audit log file.
	auditFilePermissions = 0o600

	// auditDirPermissions is the permission mode for the audit directory.
	auditDirPermissions = 0o700

	// bytesPerMB is the number of bytes per megabyte.
	bytesPerMB = 1024 * 1024

	// hoursPerDay is the number of hours in a day.
	hoursPerDay = 24

	// timestampLength is the length of the backup timestamp format (YYYYMMDD-HHMMSS).
	timestampLength = 15

	// timestampDashPos is the position of the dash in the timestamp format.
	timestampDashPos = 8
)

//go:generate enumer -type=AuditAction -trimprefix=AuditAction -json -text -yaml -sql -output=audit_action_enumer.go
//go:generate go run github.com/smykla-skalski/klaudiush/tools/enumerfix audit_action_enumer.go

// AuditAction represents the type of session action being audited.
type AuditAction int

const (
	// AuditActionPoison indicates a session was poisoned.
	AuditActionPoison AuditAction = iota

	// AuditActionUnpoison indicates a session was unpoisoned.
	AuditActionUnpoison
)

// AuditEntry represents an audit log entry for session operations.
type AuditEntry struct {
	// Timestamp is when the action occurred.
	Timestamp time.Time `json:"timestamp"`

	// Action is the type of operation (poison/unpoison).
	Action AuditAction `json:"action"`

	// SessionID is the Claude Code session identifier.
	SessionID string `json:"session_id"`

	// PoisonCodes are the error codes involved in the action.
	// For poison: codes that caused poisoning.
	// For unpoison: codes that were acknowledged.
	PoisonCodes []string `json:"poison_codes"`

	// Source indicates where the unpoison token was found (env_var/comment).
	// Only populated for unpoison actions.
	Source string `json:"source,omitempty"`

	// Command is the command that triggered the action.
	// Truncated to prevent sensitive data leakage.
	Command string `json:"command,omitempty"`

	// PoisonMessage is the original error message (for poison actions).
	PoisonMessage string `json:"poison_message,omitempty"`

	// WorkingDir is the working directory when the action occurred.
	WorkingDir string `json:"working_dir,omitempty"`
}

// AuditLogger manages audit logging for session operations.
// It writes audit entries to a JSONL file with support for
// rotation and retention policies.
type AuditLogger struct {
	mu     sync.Mutex
	config *config.SessionAuditConfig
	logger logger.Logger

	// logFile is the resolved path for the audit log file.
	logFile string

	// now is a function that returns the current time.
	// Used for testing to control time.
	now func() time.Time
}

// AuditLoggerOption configures the AuditLogger.
type AuditLoggerOption func(*AuditLogger)

// WithAuditLoggerLogger sets the logger.
func WithAuditLoggerLogger(log logger.Logger) AuditLoggerOption {
	return func(a *AuditLogger) {
		if log != nil {
			a.logger = log
		}
	}
}

// WithAuditFile sets a custom audit log file path.
func WithAuditFile(path string) AuditLoggerOption {
	return func(a *AuditLogger) {
		a.logFile = path
	}
}

// WithAuditTimeFunc sets a custom time function for testing.
func WithAuditTimeFunc(fn func() time.Time) AuditLoggerOption {
	return func(a *AuditLogger) {
		if fn != nil {
			a.now = fn
		}
	}
}

// NewAuditLogger creates a new session audit logger.
func NewAuditLogger(cfg *config.SessionAuditConfig, opts ...AuditLoggerOption) *AuditLogger {
	a := &AuditLogger{
		config: cfg,
		logger: logger.NewNoOpLogger(),
		now:    time.Now,
	}

	// Set default log file from config
	if cfg != nil {
		a.logFile = cfg.GetLogFile()
	} else {
		a.logFile = (&config.SessionAuditConfig{}).GetLogFile()
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Log writes an audit entry to the log file.
// It automatically handles rotation if the file exceeds the max size.
func (a *AuditLogger) Log(entry *AuditEntry) error {
	if entry == nil {
		return nil
	}

	// Check if audit logging is enabled
	if a.config != nil && !a.config.IsAuditEnabled() {
		a.logger.Debug("session audit logging disabled, skipping entry",
			"action", entry.Action.String(),
			"session_id", entry.SessionID,
		)

		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check and perform rotation if needed
	if rotateErr := a.rotateIfNeededLocked(); rotateErr != nil {
		a.logger.Error("failed to rotate session audit log",
			"error", rotateErr.Error(),
		)
		// Continue to log even if rotation fails
	}

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return errors.Wrap(err, "marshaling session audit entry")
	}

	return a.writeEntryLocked(data)
}

// writeEntryLocked writes the JSON data to the log file.
// Must be called with mu held.
func (a *AuditLogger) writeEntryLocked(data []byte) error {
	path := a.resolveLogPath()

	// Ensure directory exists
	dir := filepath.Dir(path)

	if mkdirErr := os.MkdirAll(dir, auditDirPermissions); mkdirErr != nil {
		return errors.Wrap(mkdirErr, "creating session audit directory")
	}

	// Open file for append
	// Path comes from trusted configuration, not user input.
	//nolint:gosec // G304: path is from config
	file, err := os.OpenFile(
		path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		auditFilePermissions,
	)
	if err != nil {
		return errors.Wrap(err, "opening session audit file")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			a.logger.Error("failed to close session audit file",
				"error", closeErr.Error(),
			)
		}
	}()

	// Write entry with newline
	if _, writeErr := file.Write(append(data, '\n')); writeErr != nil {
		return errors.Wrap(writeErr, "writing session audit entry")
	}

	a.logger.Debug("session audit entry logged",
		"path", path,
	)

	return nil
}

// Read reads all audit entries from the log file.
// Returns an empty slice if the file does not exist.
func (a *AuditLogger) Read() ([]*AuditEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := a.resolveLogPath()

	// Path comes from trusted configuration, not user input.
	file, err := os.Open(path) //nolint:gosec // G304: path is from config
	if err != nil {
		if os.IsNotExist(err) {
			return []*AuditEntry{}, nil
		}

		return nil, errors.Wrap(err, "opening session audit file")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			a.logger.Error("failed to close session audit file",
				"error", closeErr.Error(),
			)
		}
	}()

	return a.readEntriesFromFile(file)
}

// readEntriesFromFile reads audit entries from an open file.
func (a *AuditLogger) readEntriesFromFile(file *os.File) ([]*AuditEntry, error) {
	var entries []*AuditEntry

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry AuditEntry

		if unmarshalErr := json.Unmarshal([]byte(line), &entry); unmarshalErr != nil {
			a.logger.Debug("skipping malformed session audit entry",
				"error", unmarshalErr.Error(),
				"line", line,
			)

			continue
		}

		entries = append(entries, &entry)
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return nil, errors.Wrap(scanErr, "scanning session audit file")
	}

	return entries, nil
}

// Rotate forces rotation of the audit log file.
func (a *AuditLogger) Rotate() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.rotateLocked()
}

// Cleanup removes old backup files and entries exceeding retention.
func (a *AuditLogger) Cleanup() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Clean up old backup files
	if err := a.cleanupBackupsLocked(); err != nil {
		return err
	}

	// Clean up old entries from current file
	return a.cleanupOldEntriesLocked()
}

// GetLogPath returns the resolved log file path.
func (a *AuditLogger) GetLogPath() string {
	return a.resolveLogPath()
}

// rotateIfNeededLocked checks if rotation is needed and performs it.
// Must be called with mu held.
func (a *AuditLogger) rotateIfNeededLocked() error {
	path := a.resolveLogPath()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return errors.Wrap(err, "checking session audit file size")
	}

	maxSizeBytes := int64(a.getMaxSizeMB()) * bytesPerMB
	if info.Size() < maxSizeBytes {
		return nil
	}

	a.logger.Debug("session audit log exceeds max size, rotating",
		"size", info.Size(),
		"max_size", maxSizeBytes,
	)

	return a.rotateLocked()
}

// rotateLocked rotates the audit log file.
// Must be called with mu held.
func (a *AuditLogger) rotateLocked() error {
	path := a.resolveLogPath()

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// Generate backup filename with timestamp
	timestamp := a.now().Format("20060102-150405")
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	backupPath := base + "." + timestamp + ext

	// Rename current file to backup
	if err := os.Rename(path, backupPath); err != nil {
		return errors.Wrap(err, "rotating session audit file")
	}

	a.logger.Debug("rotated session audit log",
		"from", path,
		"to", backupPath,
	)

	// Clean up excess backups
	return a.cleanupBackupsLocked()
}

// cleanupBackupsLocked removes excess backup files.
// Must be called with mu held.
func (a *AuditLogger) cleanupBackupsLocked() error {
	path := a.resolveLogPath()
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := filepath.Base(strings.TrimSuffix(path, ext))

	// List backup files
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return errors.Wrap(err, "reading session audit directory")
	}

	backups := a.findBackupFiles(entries, base, ext, dir)

	// Sort backups by name (timestamp) in descending order (newest first)
	slices.Sort(backups)
	slices.Reverse(backups)

	// Remove excess backups
	a.removeExcessBackups(backups)

	return nil
}

// findBackupFiles finds backup files matching the pattern base.YYYYMMDD-HHMMSS.ext.
func (*AuditLogger) findBackupFiles(
	entries []os.DirEntry,
	base, ext, dir string,
) []string {
	var backups []string

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base+".") || !strings.HasSuffix(name, ext) {
			continue
		}

		// Check if it has a timestamp in the middle
		middle := strings.TrimPrefix(name, base+".")
		middle = strings.TrimSuffix(middle, ext)

		if len(middle) == timestampLength && middle[timestampDashPos] == '-' {
			backups = append(backups, filepath.Join(dir, name))
		}
	}

	return backups
}

// removeExcessBackups removes backups exceeding the max count.
func (a *AuditLogger) removeExcessBackups(backups []string) {
	maxBackups := a.getMaxBackups()

	for i := maxBackups; i < len(backups); i++ {
		if err := os.Remove(backups[i]); err != nil {
			a.logger.Error("failed to remove old session audit backup",
				"path", backups[i],
				"error", err.Error(),
			)

			continue
		}

		a.logger.Debug("removed old session audit backup",
			"path", backups[i],
		)
	}
}

// cleanupOldEntriesLocked removes entries older than max age.
// Must be called with mu held.
func (a *AuditLogger) cleanupOldEntriesLocked() error {
	path := a.resolveLogPath()

	validEntries, originalCount, err := a.readAndFilterEntries(path)
	if err != nil {
		return err
	}

	removedCount := originalCount - len(validEntries)
	if removedCount <= 0 {
		return nil
	}

	return a.writeFilteredEntries(path, validEntries, removedCount)
}

// readAndFilterEntries reads entries and filters out those older than max age.
func (a *AuditLogger) readAndFilterEntries(path string) ([][]byte, int, error) {
	// Path comes from trusted configuration, not user input.
	file, err := os.Open(path) //nolint:gosec // G304: path is from config
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}

		return nil, 0, errors.Wrap(err, "opening session audit file for cleanup")
	}

	defer func() {
		_ = file.Close()
	}()

	maxAge := time.Duration(a.getMaxAgeDays()) * hoursPerDay * time.Hour
	cutoff := a.now().Add(-maxAge)

	validEntries, originalCount := a.filterEntries(file, cutoff)

	return validEntries, originalCount, nil
}

// filterEntries scans the file and returns valid entries and original count.
func (*AuditLogger) filterEntries(file *os.File, cutoff time.Time) ([][]byte, int) {
	var validEntries [][]byte

	originalCount := 0
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		originalCount++

		var entry AuditEntry

		if unmarshalErr := json.Unmarshal(line, &entry); unmarshalErr != nil {
			// Keep malformed entries to avoid data loss
			validEntries = append(validEntries, slices.Clone(line))

			continue
		}

		if entry.Timestamp.After(cutoff) {
			validEntries = append(validEntries, slices.Clone(line))
		}
	}

	return validEntries, originalCount
}

// writeFilteredEntries writes the filtered entries back to the file.
func (a *AuditLogger) writeFilteredEntries(
	path string,
	validEntries [][]byte,
	removedCount int,
) error {
	tmpPath := path + ".tmp"

	// tmpPath is derived from path which comes from trusted configuration.
	//nolint:gosec // G304: tmpPath derived from config path
	tmpFile, err := os.OpenFile(
		tmpPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		auditFilePermissions,
	)
	if err != nil {
		return errors.Wrap(err, "creating temp file for session audit cleanup")
	}

	for _, entry := range validEntries {
		if _, writeErr := tmpFile.Write(append(entry, '\n')); writeErr != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)

			return errors.Wrap(writeErr, "writing cleaned session audit entries")
		}
	}

	if closeErr := tmpFile.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(closeErr, "closing temp file")
	}

	// Atomic rename
	if renameErr := os.Rename(tmpPath, path); renameErr != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(renameErr, "replacing session audit file after cleanup")
	}

	a.logger.Debug("cleaned up old session audit entries",
		"removed", removedCount,
		"remaining", len(validEntries),
	)

	return nil
}

// getMaxSizeMB returns the max file size in MB.
func (a *AuditLogger) getMaxSizeMB() int {
	if a.config == nil {
		return config.DefaultSessionAuditMaxSizeMB
	}

	return a.config.GetMaxSizeMB()
}

// getMaxAgeDays returns the max age in days.
func (a *AuditLogger) getMaxAgeDays() int {
	if a.config == nil {
		return config.DefaultSessionAuditMaxAgeDays
	}

	return a.config.GetMaxAgeDays()
}

// getMaxBackups returns the max number of backup files.
func (a *AuditLogger) getMaxBackups() int {
	if a.config == nil {
		return config.DefaultSessionAuditMaxBackups
	}

	return a.config.GetMaxBackups()
}

// resolveLogPath expands ~ in the log file path.
func (a *AuditLogger) resolveLogPath() string {
	path := a.logFile
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	return path
}

// Stats returns statistics about the session audit log.
func (a *AuditLogger) Stats() (*AuditStats, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	path := a.resolveLogPath()

	stats := &AuditStats{
		LogFile: path,
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}

		return nil, errors.Wrap(err, "getting session audit file info")
	}

	stats.SizeBytes = info.Size()
	stats.ModTime = info.ModTime()

	if countErr := a.countEntriesAndBackups(stats, path); countErr != nil {
		return nil, countErr
	}

	return stats, nil
}

// countEntriesAndBackups populates entry and backup counts in stats.
func (a *AuditLogger) countEntriesAndBackups(stats *AuditStats, path string) error {
	// Count entries
	// Path comes from trusted configuration, not user input.
	file, err := os.Open(path) //nolint:gosec // G304: path is from config
	if err != nil {
		return errors.Wrap(err, "opening session audit file")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			a.logger.Error("failed to close session audit file",
				"error", closeErr.Error(),
			)
		}
	}()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			stats.EntryCount++
		}
	}

	// Count backups
	stats.BackupCount = a.countBackupFiles(path)

	return nil
}

// countBackupFiles counts the number of backup files for the given log path.
func (*AuditLogger) countBackupFiles(path string) int {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := filepath.Base(strings.TrimSuffix(path, ext))

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	count := 0

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base+".") || !strings.HasSuffix(name, ext) {
			continue
		}

		middle := strings.TrimPrefix(name, base+".")
		middle = strings.TrimSuffix(middle, ext)

		if len(middle) == timestampLength && middle[timestampDashPos] == '-' {
			count++
		}
	}

	return count
}

// AuditStats contains statistics about the session audit log.
type AuditStats struct {
	// LogFile is the path to the audit log file.
	LogFile string `json:"log_file"`

	// SizeBytes is the current size of the audit log file.
	SizeBytes int64 `json:"size_bytes"`

	// SizeMB is the size in megabytes (for display).
	SizeMB string `json:"size_mb"`

	// EntryCount is the number of entries in the log file.
	EntryCount int `json:"entry_count"`

	// BackupCount is the number of backup files.
	BackupCount int `json:"backup_count"`

	// ModTime is the last modification time of the log file.
	ModTime time.Time `json:"mod_time"`
}

// FormatSize formats the size in human-readable form.
func (s *AuditStats) FormatSize() string {
	mb := float64(s.SizeBytes) / float64(bytesPerMB)

	return strconv.FormatFloat(mb, 'f', 2, 64) + " MB"
}

// IsEnabled returns true if session audit logging is enabled.
func (a *AuditLogger) IsEnabled() bool {
	if a.config == nil {
		return true // Enabled by default
	}

	return a.config.IsAuditEnabled()
}
