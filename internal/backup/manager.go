package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

var (
	// ErrConfigFileNotFound is returned when config file doesn't exist.
	ErrConfigFileNotFound = errors.New("config file not found")

	// ErrBackupDisabled is returned when backup system is disabled.
	ErrBackupDisabled = errors.New("backup system is disabled")
)

// Manager orchestrates backup operations.
type Manager struct {
	// storage provides persistence for snapshots.
	storage Storage

	// config contains backup configuration.
	config *config.BackupConfig

	// auditLogger logs backup operations (optional).
	auditLogger AuditLogger
}

// NewManager creates a new backup manager.
func NewManager(storage Storage, cfg *config.BackupConfig) (*Manager, error) {
	if storage == nil {
		return nil, errors.New("storage cannot be nil")
	}

	if cfg == nil {
		cfg = &config.BackupConfig{}
	}

	return &Manager{
		storage:     storage,
		config:      cfg,
		auditLogger: nil,
	}, nil
}

// NewManagerWithAudit creates a new backup manager with audit logging.
func NewManagerWithAudit(
	storage Storage,
	cfg *config.BackupConfig,
	auditLogger AuditLogger,
) (*Manager, error) {
	if storage == nil {
		return nil, errors.New("storage cannot be nil")
	}

	if cfg == nil {
		cfg = &config.BackupConfig{}
	}

	return &Manager{
		storage:     storage,
		config:      cfg,
		auditLogger: auditLogger,
	}, nil
}

// CreateBackupOptions contains options for creating a backup.
type CreateBackupOptions struct {
	// ConfigPath is the absolute path to the config file.
	ConfigPath string

	// Trigger indicates what caused this backup.
	Trigger Trigger

	// Metadata provides additional context.
	Metadata SnapshotMetadata
}

// CreateBackup creates a new backup snapshot with deduplication.
func (m *Manager) CreateBackup(opts CreateBackupOptions) (*Snapshot, error) {
	if !m.config.IsEnabled() {
		return nil, ErrBackupDisabled
	}

	// Read config file
	data, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrap(ErrConfigFileNotFound, opts.ConfigPath)
		}

		return nil, errors.Wrap(err, "failed to read config file")
	}

	// Initialize storage if needed
	if !m.storage.Exists() {
		if initErr := m.storage.Initialize(); initErr != nil {
			return nil, errors.Wrap(initErr, "failed to initialize storage")
		}
	}

	// Load index
	index, err := m.storage.LoadIndex()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load index")
	}

	// Compute content hash
	contentHash := ComputeContentHash(data)
	opts.Metadata.ConfigHash = contentHash

	// Deduplication: Check if identical content already exists
	if existing, found := index.FindByHash(contentHash); found {
		return &existing, nil
	}

	// Determine storage type (full vs patch)
	timestamp := time.Now()
	storageType := m.determineStorageType(index)

	// Generate snapshot ID
	snapshotID := GenerateSnapshotID(timestamp, contentHash)

	// Determine sequence number and chain ID
	chainID := m.generateChainID(index)
	seqNum := m.getNextSequenceNumber(index, chainID)

	// For now, only implement full snapshots (patch support in Phase 3)
	var storagePath string

	var size int64

	if storageType != StorageTypeFull {
		// Patch support will be implemented in Phase 3
		return nil, errors.New("patch snapshots not yet implemented")
	}

	storagePath, err = m.storage.Save(snapshotID+".full.toml", data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to save full snapshot")
	}

	size = int64(len(data))

	// Create snapshot
	snapshot := m.createSnapshotRecord(
		snapshotID,
		seqNum,
		timestamp,
		opts.ConfigPath,
		storageType,
		storagePath,
		size,
		contentHash,
		chainID,
		opts,
	)

	// Add to index and save
	if err := m.saveSnapshotToIndex(index, snapshot, opts.ConfigPath); err != nil {
		return nil, err
	}

	// Log successful backup creation
	m.logCreateSuccess(opts.ConfigPath, snapshotID, size, storageType, opts.Trigger)

	return &snapshot, nil
}

// List returns all snapshots in chronological order.
func (m *Manager) List() ([]Snapshot, error) {
	if !m.config.IsEnabled() {
		return nil, ErrBackupDisabled
	}

	if !m.storage.Exists() {
		return []Snapshot{}, nil
	}

	index, err := m.storage.LoadIndex()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load index")
	}

	return index.List(), nil
}

// Get retrieves a snapshot by ID.
func (m *Manager) Get(id string) (*Snapshot, error) {
	if !m.config.IsEnabled() {
		return nil, ErrBackupDisabled
	}

	if !m.storage.Exists() {
		return nil, ErrSnapshotNotFound
	}

	index, err := m.storage.LoadIndex()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load index")
	}

	snapshot, err := index.Get(id)
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// determineStorageType determines whether to create a full or patch snapshot.
//
//nolint:unparam // Will be dynamic when delta logic is implemented in Phase 3
func (*Manager) determineStorageType(index *SnapshotIndex) StorageType {
	snapshots := index.List()
	if len(snapshots) == 0 {
		return StorageTypeFull
	}

	// For Phase 1, always create full snapshots
	// Delta logic will be implemented in Phase 3
	return StorageTypeFull
}

// generateChainID generates a chain ID for the snapshot.
func (*Manager) generateChainID(index *SnapshotIndex) string {
	snapshots := index.List()
	if len(snapshots) == 0 {
		return "chain-1"
	}

	// For Phase 1, each full snapshot is its own chain
	// This allows retention policies to work correctly
	// In Phase 3, when delta is added, multiple snapshots will share chains
	maxChainNum := 0

	for _, snapshot := range snapshots {
		var chainNum int

		_, err := fmt.Sscanf(snapshot.ChainID, "chain-%d", &chainNum)
		if err == nil && chainNum > maxChainNum {
			maxChainNum = chainNum
		}
	}

	return fmt.Sprintf("chain-%d", maxChainNum+1)
}

// getNextSequenceNumber returns the next sequence number for a chain.
func (*Manager) getNextSequenceNumber(index *SnapshotIndex, chainID string) int {
	chain := index.GetChain(chainID)
	if len(chain) == 0 {
		return 1
	}

	maxSeq := 0

	for _, snapshot := range chain {
		if snapshot.SequenceNum > maxSeq {
			maxSeq = snapshot.SequenceNum
		}
	}

	return maxSeq + 1
}

// determineConfigType determines whether a config path is global or project.
func (*Manager) determineConfigType(configPath string) ConfigType {
	// Check if path contains .klaudiush directory (project config)
	// vs ~/.klaudiush (global config)
	// Normalize the path and check if any component is .klaudiush
	cleanPath := filepath.Clean(configPath)

	for part := range strings.SplitSeq(cleanPath, string(filepath.Separator)) {
		if part == ".klaudiush" {
			return ConfigTypeProject
		}
	}

	return ConfigTypeGlobal
}

// RetentionResult contains information about retention operations.
type RetentionResult struct {
	// SnapshotsRemoved is the number of snapshots removed.
	SnapshotsRemoved int

	// ChainsRemoved is the number of chains removed.
	ChainsRemoved int

	// BytesFreed is the number of bytes freed.
	BytesFreed int64

	// RemovedSnapshots contains the IDs of removed snapshots.
	RemovedSnapshots []string
}

// ApplyRetention applies a retention policy and removes snapshots that should not be retained.
func (m *Manager) ApplyRetention(policy RetentionPolicy) (*RetentionResult, error) {
	if !m.config.IsEnabled() {
		return nil, ErrBackupDisabled
	}

	if policy == nil {
		return nil, errors.New("policy cannot be nil")
	}

	if !m.storage.Exists() {
		return &RetentionResult{}, nil
	}

	// Load index
	index, err := m.storage.LoadIndex()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load index")
	}

	allSnapshots := index.List()
	if len(allSnapshots) == 0 {
		return &RetentionResult{}, nil
	}

	// Calculate total size
	var totalSize int64
	for _, snapshot := range allSnapshots {
		totalSize += snapshot.Size
	}

	// Determine which snapshots to remove
	toRemove := make([]Snapshot, 0)
	removedChains := make(map[string]bool)

	for _, snapshot := range allSnapshots {
		chain := index.GetChain(snapshot.ChainID)

		context := RetentionContext{
			AllSnapshots: allSnapshots,
			Chain:        chain,
			TotalSize:    totalSize,
			Now:          time.Now(),
		}

		if !policy.ShouldRetain(snapshot, context) {
			toRemove = append(toRemove, snapshot)
			removedChains[snapshot.ChainID] = true
		}
	}

	// Remove snapshots
	var bytesFreed int64

	removedIDs := make([]string, 0, len(toRemove))

	for _, snapshot := range toRemove {
		// Delete from storage
		if err := m.storage.Delete(snapshot.StoragePath); err != nil {
			// Continue removing other snapshots even if one fails
			// Log error but don't fail the entire operation
			continue
		}

		// Delete from index
		if err := index.Delete(snapshot.ID); err != nil {
			continue
		}

		bytesFreed += snapshot.Size
		removedIDs = append(removedIDs, snapshot.ID)
	}

	// Save updated index
	if len(removedIDs) > 0 {
		if err := m.storage.SaveIndex(index); err != nil {
			m.logAuditEntry(AuditEntry{
				Timestamp: time.Now(),
				Operation: OperationPrune,
				Success:   false,
				Error:     err.Error(),
			})

			return nil, errors.Wrap(err, "failed to save index after retention")
		}
	}

	// Log retention operation
	m.logAuditEntry(AuditEntry{
		Timestamp: time.Now(),
		Operation: OperationPrune,
		Success:   true,
		Extra: map[string]any{
			"snapshots_removed": len(removedIDs),
			"chains_removed":    len(removedChains),
			"bytes_freed":       bytesFreed,
		},
	})

	return &RetentionResult{
		SnapshotsRemoved: len(removedIDs),
		ChainsRemoved:    len(removedChains),
		BytesFreed:       bytesFreed,
		RemovedSnapshots: removedIDs,
	}, nil
}

// RestoreSnapshot restores a snapshot to a target path.
func (m *Manager) RestoreSnapshot(
	snapshotID string,
	opts RestoreOptions,
) (*RestoreResult, error) {
	if !m.config.IsEnabled() {
		return nil, ErrBackupDisabled
	}

	// Get snapshot
	snapshot, err := m.Get(snapshotID)
	if err != nil {
		return nil, err
	}

	// Create restorer
	restorer, err := NewRestorer(m.storage, m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create restorer")
	}

	// Restore snapshot
	result, err := restorer.RestoreSnapshot(snapshot, opts)
	if err != nil {
		m.logAuditEntry(AuditEntry{
			Timestamp:  time.Now(),
			Operation:  OperationRestore,
			ConfigPath: snapshot.ConfigPath,
			SnapshotID: snapshotID,
			Success:    false,
			Error:      err.Error(),
		})

		return nil, errors.Wrap(err, "failed to restore snapshot")
	}

	// Log successful restore
	m.logAuditEntry(AuditEntry{
		Timestamp:  time.Now(),
		Operation:  OperationRestore,
		ConfigPath: result.RestoredPath,
		SnapshotID: snapshotID,
		Success:    true,
		Extra: map[string]any{
			"bytes_restored":    result.BytesRestored,
			"checksum_verified": result.ChecksumVerified,
			"backup_created":    result.BackupSnapshot != nil,
		},
	})

	return result, nil
}

// ValidateSnapshot validates a snapshot's integrity.
func (m *Manager) ValidateSnapshot(snapshotID string) error {
	if !m.config.IsEnabled() {
		return ErrBackupDisabled
	}

	// Get snapshot
	snapshot, err := m.Get(snapshotID)
	if err != nil {
		return err
	}

	// Create restorer
	restorer, err := NewRestorer(m.storage, m)
	if err != nil {
		return errors.Wrap(err, "failed to create restorer")
	}

	// Validate snapshot
	if err := restorer.ValidateSnapshot(snapshot); err != nil {
		return errors.Wrap(err, "validation failed")
	}

	return nil
}

// createSnapshotRecord creates a snapshot record with the given parameters.
func (m *Manager) createSnapshotRecord(
	snapshotID string,
	seqNum int,
	timestamp time.Time,
	configPath string,
	storageType StorageType,
	storagePath string,
	size int64,
	contentHash string,
	chainID string,
	opts CreateBackupOptions,
) Snapshot {
	configType := m.determineConfigType(configPath)

	return Snapshot{
		ID:             snapshotID,
		SequenceNum:    seqNum,
		Timestamp:      timestamp,
		ConfigPath:     configPath,
		ConfigType:     configType,
		Trigger:        opts.Trigger,
		StorageType:    storageType,
		StoragePath:    storagePath,
		Size:           size,
		Checksum:       contentHash,
		ChainID:        chainID,
		BaseSnapshotID: "",
		PatchFrom:      "",
		Metadata:       opts.Metadata,
	}
}

// saveSnapshotToIndex adds snapshot to index and saves it.
func (m *Manager) saveSnapshotToIndex(
	index *SnapshotIndex,
	snapshot Snapshot,
	configPath string,
) error {
	index.Add(snapshot)

	if err := m.storage.SaveIndex(index); err != nil {
		m.logAuditEntry(AuditEntry{
			Timestamp:  time.Now(),
			Operation:  OperationCreate,
			ConfigPath: configPath,
			SnapshotID: snapshot.ID,
			Success:    false,
			Error:      err.Error(),
		})

		return errors.Wrap(err, "failed to save index")
	}

	return nil
}

// logCreateSuccess logs successful backup creation.
func (m *Manager) logCreateSuccess(
	configPath string,
	snapshotID string,
	size int64,
	storageType StorageType,
	trigger Trigger,
) {
	m.logAuditEntry(AuditEntry{
		Timestamp:  time.Now(),
		Operation:  OperationCreate,
		ConfigPath: configPath,
		SnapshotID: snapshotID,
		Success:    true,
		Extra: map[string]any{
			"size":         size,
			"storage_type": string(storageType),
			"trigger":      string(trigger),
		},
	})
}

// logAuditEntry logs an audit entry if audit logger is configured.
func (m *Manager) logAuditEntry(entry AuditEntry) {
	if m.auditLogger == nil {
		return
	}

	// Fill in user and hostname if not provided
	if entry.User == "" {
		entry.User = getCurrentUser()
	}

	if entry.Hostname == "" {
		entry.Hostname = getHostname()
	}

	// Log entry (ignore errors as audit logging is best-effort)
	_ = m.auditLogger.Log(entry)
}

// getCurrentUser returns the current username.
func getCurrentUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}

	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}

	return "unknown"
}

// getHostname returns the current hostname.
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}

	return hostname
}
