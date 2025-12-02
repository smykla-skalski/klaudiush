// Package backup provides automatic backup functionality for klaudiush config files.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
)

var (
	// ErrSnapshotNotFound is returned when a snapshot is not found.
	ErrSnapshotNotFound = errors.New("snapshot not found")

	// ErrInvalidStorageType is returned when an invalid storage type is provided.
	ErrInvalidStorageType = errors.New("invalid storage type")

	// ErrInvalidConfigType is returned when an invalid config type is provided.
	ErrInvalidConfigType = errors.New("invalid config type")
)

// StorageType indicates how a snapshot is stored.
type StorageType string

const (
	// StorageTypeFull indicates a complete snapshot.
	StorageTypeFull StorageType = "full"

	// StorageTypePatch indicates a delta/patch snapshot.
	StorageTypePatch StorageType = "patch"
)

// ConfigType indicates which config file is backed up.
type ConfigType string

const (
	// ConfigTypeGlobal indicates a global config (~/.klaudiush/config.toml).
	ConfigTypeGlobal ConfigType = "global"

	// ConfigTypeProject indicates a project config (.klaudiush/config.toml).
	ConfigTypeProject ConfigType = "project"
)

// Trigger indicates what caused the backup to be created.
type Trigger string

const (
	// TriggerManual indicates a user-initiated backup.
	TriggerManual Trigger = "manual"

	// TriggerAutomatic indicates an automatic backup before config change.
	TriggerAutomatic Trigger = "automatic"

	// TriggerBeforeInit indicates a backup before init --force.
	TriggerBeforeInit Trigger = "before_init"

	// TriggerMigration indicates a backup during first-run migration.
	TriggerMigration Trigger = "migration"
)

// Snapshot represents a single backup snapshot of a config file.
type Snapshot struct {
	// ID is the unique identifier for this snapshot.
	ID string `json:"id"`

	// SequenceNum is the sequence number within the chain (1-based).
	SequenceNum int `json:"sequence_num"`

	// Timestamp is when the snapshot was created.
	Timestamp time.Time `json:"timestamp"`

	// ConfigPath is the absolute path to the config file that was backed up.
	ConfigPath string `json:"config_path"`

	// ConfigType indicates whether this is a global or project config.
	ConfigType ConfigType `json:"config_type"`

	// Trigger indicates what caused this backup to be created.
	Trigger Trigger `json:"trigger"`

	// StorageType indicates how this snapshot is stored (full or patch).
	StorageType StorageType `json:"storage_type"`

	// StoragePath is the path where the snapshot data is stored.
	StoragePath string `json:"storage_path"`

	// Size is the size of the stored data in bytes.
	Size int64 `json:"size"`

	// Checksum is the SHA256 checksum of the stored data.
	Checksum string `json:"checksum"`

	// ChainID identifies the chain this snapshot belongs to.
	ChainID string `json:"chain_id"`

	// BaseSnapshotID is the ID of the full snapshot this patch is based on.
	// Empty for full snapshots.
	BaseSnapshotID string `json:"base_snapshot_id,omitempty"`

	// PatchFrom is the ID of the previous snapshot this patch was generated from.
	// Empty for full snapshots.
	PatchFrom string `json:"patch_from,omitempty"`

	// Metadata contains additional context about the snapshot.
	Metadata SnapshotMetadata `json:"metadata"`
}

// SnapshotMetadata contains additional context about a snapshot.
type SnapshotMetadata struct {
	// User is the username who created the backup.
	User string `json:"user,omitempty"`

	// Hostname is the machine hostname.
	Hostname string `json:"hostname,omitempty"`

	// Command is the klaudiush command that triggered the backup.
	Command string `json:"command,omitempty"`

	// ConfigHash is the SHA256 hash of the config content.
	ConfigHash string `json:"config_hash"`

	// Tag is an optional user-provided tag.
	Tag string `json:"tag,omitempty"`

	// Description is an optional user-provided description.
	Description string `json:"description,omitempty"`
}

// SnapshotIndex contains metadata about all snapshots in a directory.
type SnapshotIndex struct {
	// Version is the index schema version.
	Version int `json:"version"`

	// Updated is when the index was last updated.
	Updated time.Time `json:"updated"`

	// Snapshots maps snapshot IDs to their metadata.
	Snapshots map[string]Snapshot `json:"snapshots"`
}

// IsFull returns true if this is a full snapshot.
func (s *Snapshot) IsFull() bool {
	return s.StorageType == StorageTypeFull
}

// IsPatch returns true if this is a patch snapshot.
func (s *Snapshot) IsPatch() bool {
	return s.StorageType == StorageTypePatch
}

// IsGlobal returns true if this is a global config backup.
func (s *Snapshot) IsGlobal() bool {
	return s.ConfigType == ConfigTypeGlobal
}

// IsProject returns true if this is a project config backup.
func (s *Snapshot) IsProject() bool {
	return s.ConfigType == ConfigTypeProject
}

// GenerateSnapshotID generates a unique snapshot ID from timestamp and content hash.
func GenerateSnapshotID(timestamp time.Time, contentHash string) string {
	data := fmt.Sprintf("%d-%s", timestamp.UnixNano(), contentHash)
	hash := sha256.Sum256([]byte(data))

	return hex.EncodeToString(hash[:])[:16]
}

// ComputeContentHash computes the SHA256 hash of content.
func ComputeContentHash(content []byte) string {
	hash := sha256.Sum256(content)

	return hex.EncodeToString(hash[:])
}

// NewSnapshotIndex creates a new empty snapshot index.
func NewSnapshotIndex() *SnapshotIndex {
	return &SnapshotIndex{
		Version:   1,
		Updated:   time.Now(),
		Snapshots: make(map[string]Snapshot),
	}
}

// Add adds a snapshot to the index.
func (idx *SnapshotIndex) Add(snapshot Snapshot) {
	idx.Snapshots[snapshot.ID] = snapshot
	idx.Updated = time.Now()
}

// Get retrieves a snapshot by ID.
func (idx *SnapshotIndex) Get(id string) (Snapshot, error) {
	snapshot, ok := idx.Snapshots[id]
	if !ok {
		return Snapshot{}, errors.Wrapf(ErrSnapshotNotFound, "ID: %s", id)
	}

	return snapshot, nil
}

// Delete removes a snapshot from the index.
func (idx *SnapshotIndex) Delete(id string) error {
	if _, ok := idx.Snapshots[id]; !ok {
		return errors.Wrapf(ErrSnapshotNotFound, "ID: %s", id)
	}

	delete(idx.Snapshots, id)
	idx.Updated = time.Now()

	return nil
}

// List returns all snapshots in chronological order.
func (idx *SnapshotIndex) List() []Snapshot {
	snapshots := make([]Snapshot, 0, len(idx.Snapshots))

	for _, snapshot := range idx.Snapshots {
		snapshots = append(snapshots, snapshot)
	}

	return snapshots
}

// FindByHash returns the first snapshot with matching config hash.
func (idx *SnapshotIndex) FindByHash(hash string) (Snapshot, bool) {
	for _, snapshot := range idx.Snapshots {
		if snapshot.Metadata.ConfigHash == hash {
			return snapshot, true
		}
	}

	return Snapshot{}, false
}

// GetChain returns all snapshots in a chain, ordered by sequence number.
func (idx *SnapshotIndex) GetChain(chainID string) []Snapshot {
	snapshots := make([]Snapshot, 0)

	for _, snapshot := range idx.Snapshots {
		if snapshot.ChainID == chainID {
			snapshots = append(snapshots, snapshot)
		}
	}

	return snapshots
}
