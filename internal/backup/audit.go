package backup

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
)

const (
	// AuditLogFile is the name of the audit log file.
	AuditLogFile = "audit.jsonl"

	// AuditLogPerms is the file permissions for the audit log.
	AuditLogPerms = 0o600

	// AuditLogDirPerms is the directory permissions for the audit log directory.
	AuditLogDirPerms = 0o700
)

// Operation types for audit logging.
const (
	OperationCreate  = "create"
	OperationRestore = "restore"
	OperationDelete  = "delete"
	OperationPrune   = "prune"
	OperationList    = "list"
	OperationGet     = "get"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	// Timestamp is when the operation occurred.
	Timestamp time.Time `json:"timestamp"`

	// Operation is the type of operation (create, restore, delete, prune).
	Operation string `json:"operation"`

	// ConfigPath is the path to the config file involved.
	ConfigPath string `json:"config_path,omitempty"`

	// SnapshotID is the ID of the snapshot involved.
	SnapshotID string `json:"snapshot_id,omitempty"`

	// User is the username who performed the operation.
	User string `json:"user,omitempty"`

	// Hostname is the machine hostname.
	Hostname string `json:"hostname,omitempty"`

	// Success indicates whether the operation succeeded.
	Success bool `json:"success"`

	// Error contains error message if operation failed.
	Error string `json:"error,omitempty"`

	// Extra contains additional operation-specific data.
	Extra map[string]any `json:"extra,omitempty"`
}

// AuditFilter defines criteria for querying audit entries.
type AuditFilter struct {
	// Operation filters by operation type.
	Operation string

	// Since filters entries after this time.
	Since time.Time

	// SnapshotID filters by snapshot ID.
	SnapshotID string

	// Success filters by success/failure (nil = all).
	Success *bool

	// Limit limits the number of entries returned (0 = all).
	Limit int
}

// AuditLogger provides audit logging functionality.
type AuditLogger interface {
	// Log writes an audit entry.
	Log(entry AuditEntry) error

	// Query retrieves audit entries matching the filter.
	Query(filter AuditFilter) ([]AuditEntry, error)

	// Close closes the logger and releases resources.
	Close() error
}

// JSONLAuditLogger implements AuditLogger using JSONL format.
type JSONLAuditLogger struct {
	// logPath is the path to the audit log file.
	logPath string

	// mu protects concurrent writes.
	mu sync.Mutex
}

// NewJSONLAuditLogger creates a new JSONL audit logger.
func NewJSONLAuditLogger(baseDir string) (*JSONLAuditLogger, error) {
	if baseDir == "" {
		return nil, errors.New("baseDir cannot be empty")
	}

	logPath := filepath.Join(baseDir, AuditLogFile)

	return &JSONLAuditLogger{
		logPath: logPath,
	}, nil
}

// Log writes an audit entry to the log file.
func (l *JSONLAuditLogger) Log(entry AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(l.logPath)
	if err := os.MkdirAll(dir, AuditLogDirPerms); err != nil {
		return errors.Wrap(err, "failed to create audit log directory")
	}

	// Open file in append mode
	file, err := os.OpenFile(l.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, AuditLogPerms)
	if err != nil {
		return errors.Wrap(err, "failed to open audit log")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Best-effort close, already returning or continuing
			_ = closeErr
		}
	}()

	// Encode entry as JSON
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(entry); err != nil {
		return errors.Wrap(err, "failed to encode audit entry")
	}

	return nil
}

// Query retrieves audit entries matching the filter.
func (l *JSONLAuditLogger) Query(filter AuditFilter) ([]AuditEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(l.logPath); os.IsNotExist(err) {
		return []AuditEntry{}, nil
	}

	// Open file for reading
	file, err := os.Open(l.logPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open audit log")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Best-effort close, already returning
			_ = closeErr
		}
	}()

	// Read entries line by line
	var entries []AuditEntry

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var entry AuditEntry

		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			// Skip invalid entries
			continue
		}

		// Apply filters
		if !matchesFilter(entry, filter) {
			continue
		}

		entries = append(entries, entry)

		// Apply limit
		if filter.Limit > 0 && len(entries) >= filter.Limit {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "failed to read audit log")
	}

	return entries, nil
}

// Close closes the logger.
func (*JSONLAuditLogger) Close() error {
	// No resources to close for JSONL logger
	return nil
}

// matchesFilter checks if an entry matches the filter criteria.
func matchesFilter(entry AuditEntry, filter AuditFilter) bool {
	// Filter by operation
	if filter.Operation != "" && entry.Operation != filter.Operation {
		return false
	}

	// Filter by time
	if !filter.Since.IsZero() && entry.Timestamp.Before(filter.Since) {
		return false
	}

	// Filter by snapshot ID
	if filter.SnapshotID != "" && entry.SnapshotID != filter.SnapshotID {
		return false
	}

	// Filter by success
	if filter.Success != nil && entry.Success != *filter.Success {
		return false
	}

	return true
}
