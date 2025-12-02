# Backup System Architecture

Automatic configuration backup system with centralized storage, deduplication, retention policies, and audit logging.

## Core Design Philosophy

**Centralized Storage**: All backups in `~/.klaudiush/.backups/{global,projects/*/}` instead of scattered project directories. Simplifies management and prevents project clutter.

**Always-On Deduplication**: SHA256 content hashing prevents duplicate backups. `FindByHash()` checks before creating snapshots.

**Phase 1 Strategy**: Currently only full snapshots. Each full snapshot is its own chain. Delta/patch support designed but not yet implemented.

**Interface-Based**: `Storage` interface allows future backends (S3, database) without manager changes.

## Critical Implementation Details

### Path Sanitization

Project paths become directory names: `/Users/bart/project` → `Users_bart_project`

**macOS Gotcha**: `/var` is symlink to `/private/var`. `os.Getwd()` returns real path. Tests must use actual working directory for comparisons, not the original path passed to `os.Chdir()`.

### Storage Initialization

`Manager.CreateBackup()` automatically initializes storage via `storage.Initialize()` if `!storage.Exists()`. Storage creation is lazy - only happens on first backup.

### Deduplication Flow

```go
hash := SHA256(configData)
if existing := index.FindByHash(hash) {
    return existing  // No new snapshot created
}
// Create new snapshot
```

Always check deduplication BEFORE creating snapshots to avoid wasted storage.

### Chain Design (Phase 1)

Every full snapshot gets unique `ChainID`. No delta chains yet. This simplifies Phase 1 but means:

- Each backup is independent
- No patch reconstruction needed
- Retention deletes entire "chains" (just one snapshot)
- Future phases will build multi-snapshot chains with deltas

### Sequence Numbers

Per-directory sequence counter starting at 1. Used in snapshot filenames: `001_DATE.full.toml`, `002_DATE.full.toml`, etc. Managed by `storage.LoadIndex()` and `manager.getNextSequenceNumber()`.

### Trigger Types

- `TriggerManual` - User-initiated via CLI
- `TriggerAutomatic` - Before config writes (Writer integration)
- `TriggerBeforeInit` - Before `init --force` overwrites
- `TriggerMigration` - First-run migration backup

Tracked in snapshots for audit purposes.

## Integration Points

### Writer Integration

`Writer` accepts optional `BackupManager`. If present:

```go
if backupMgr && exists(path) {
    if async {
        go backupMgr.CreateBackup(...)  // Non-blocking
    } else {
        backupMgr.CreateBackup(...)  // Blocking
    }
}
```

**Backwards Compatibility**: Writer works with `nil` BackupManager. All backup operations check for nil before calling methods.

### Init Command Integration

Before `--force` overwrites existing config:

```go
if force && exists(configPath) {
    snapshot := backupMgr.CreateBackup(opts)
    log("Backed up:", snapshot.ID)
}
```

Ensures user can recover if init destroys their config.

### Migration Integration

First-run detection via marker file `~/.klaudiush/.migration_v1`:

```go
func performFirstRunMigration(homeDir, log) {
    if markerExists() { return }  // Already migrated

    backupIfExists(globalConfig)
    backupIfExists(projectConfig)  // From os.Getwd()

    createMarker()
}
```

**Testability**: `backupConfigIfExists()` accepts `homeDir` parameter so tests can use temp directories instead of real home.

**Error Resilience**: Backup failures logged but don't block migration. Avoids breaking existing installations.

## Retention Policies

### Policy Types

**CountRetentionPolicy**: Keep last N snapshots
**AgeRetentionPolicy**: Keep snapshots younger than duration
**SizeRetentionPolicy**: Keep snapshots under total size limit
**CompositeRetentionPolicy**: Combine multiple policies

### Chain-Aware Cleanup

Retention policies delete entire chains, not individual snapshots:

```go
chains := groupByChain(snapshots)
for chain := range chains {
    if shouldDelete(chain) {
        deleteAllInChain(chain)  // Full snapshot + all patches
    }
}
```

Future-proofs for delta chains while working correctly with Phase 1 single-snapshot chains.

## Restore Operations

### Restore Flow

```go
RestoreSnapshot(snapshotID, opts) {
    if opts.BackupBeforeRestore {
        createBackup(targetPath)  // Safety backup
    }

    data := ReconstructSnapshot(snapshotID)  // Future: apply patches

    if opts.Validate {
        verifyChecksum(data, snapshot.Checksum)
    }

    writeFile(opts.TargetPath, data)
}
```

**Safety First**: Backup-before-restore enabled by default. Prevents data loss if restore fails or user restores wrong version.

**Validation**: Checksum verification optional but recommended for critical restores.

## Audit Logging

### JSONL Format

Append-only log in `~/.klaudiush/.backups/audit.jsonl`:

```json
{"timestamp":"2025-01-02T15:04:05Z","operation":"create","config_path":"/path","snapshot_id":"abc123","user":"bart","hostname":"laptop","success":true,"extra":{"trigger":"automatic"}}
```

**Thread-Safe**: Uses `sync.Mutex` for concurrent writes. Multiple goroutines can log simultaneously without corruption.

**Query Filters**: Operation, time range, snapshot ID, success/failure, limit.

**No Rotation**: Log grows indefinitely. Future enhancement would add log rotation.

## Doctor Integration

### Backup Health Checks

**DirectoryChecker**: Verifies backup directory exists and has correct permissions
**MetadataChecker**: Validates snapshot index integrity and structure
**IntegrityChecker**: Checks snapshot file existence and index consistency

**Auto-Fix**: `--fix` flag creates missing directories, fixes permissions, rebuilds corrupted indexes.

**Category**: `--category backup` filters to only backup-related checks.

## Testing Patterns

### Isolation

Each test gets unique `tempDir` via `os.MkdirTemp()`. Prevents test interference and enables parallel test execution.

### Symlink Handling

macOS tests must resolve symlinks:

```go
os.Chdir(projectDir)
actualWorkDir, _ := os.Getwd()  // Get real path
sanitizedPath := SanitizePath(actualWorkDir)  // Use real path
```

Without this, `/var/...` vs `/private/var/...` path mismatches cause test failures.

### Variable Shadowing

Test file is in `main` package, so global vars from `main.go` are visible. Avoid using variable names like `configPath` that shadow globals. Use `testConfigPath`, `projectConfigPath`, etc.

### Audit Testing

Audit logger must be closed before reading log file:

```go
logger.Close()  // Flush writes
data := readFile(auditPath)
```

Without explicit close, writes may be buffered and not visible to tests.

## Configuration Schema

```toml
[backup]
enabled = true              # Default: true
auto_backup = true          # Default: true
max_backups = 10            # Per-directory limit
max_age = "720h"            # 30 days
max_size = 52428800         # 50MB
async_backup = true         # Non-blocking backups

[backup.delta]
full_snapshot_interval = 10     # Every 10 backups (future)
full_snapshot_max_age = "168h"  # Or every 7 days (future)
```

**Defaults**: When `BackupConfig` is `nil` or fields are `nil`, defaults apply via `IsEnabled()` methods. No config required for basic operation.

**Deduplication**: Always-on, not configurable. No user-facing setting.

## CLI Commands

```bash
klaudiush backup list [--project PATH | --global | --all]
klaudiush backup create [--tag TAG --description DESC]
klaudiush backup restore SNAPSHOT_ID [--dry-run] [--force]
klaudiush backup delete SNAPSHOT_ID...
klaudiush backup prune [--dry-run]
klaudiush backup status
klaudiush backup audit [--operation OP --since TIME --snapshot ID]
```

All implemented in `cmd/klaudiush/backup.go`. Uses `Manager` API directly.

## Error Handling

**Storage Not Initialized**: Manager auto-initializes on first use. No manual initialization required.

**Config File Not Found**: Returns `ErrConfigFileNotFound` wrapped error. Caller decides if this is fatal.

**Backup Disabled**: `CreateBackup()` returns `ErrBackupDisabled` early. Allows graceful degradation.

**Checksum Mismatch**: Restore with validation fails loudly. Without validation, corrupt data silently written.

## Performance Characteristics

**Deduplication Check**: O(1) hash lookup in index map. Very fast.

**Storage Operations**: Direct filesystem I/O. Bounded by disk speed.

**Async Backups**: Spawns goroutine for non-blocking backup. Returns immediately. Errors logged but not propagated to caller.

**Index Loading**: Reads entire JSON file on first access. Cached in memory afterward. Large indexes (1000+ snapshots) may have noticeable load time.

## Security Model

**Permissions**: Files 0o600, directories 0o700. Owner-only access.

**No Encryption**: Relies on filesystem encryption (FileVault, LUKS). Plaintext backups in ~/.klaudiush/.

**Checksums**: SHA256 for integrity, not security. Detects corruption, not tampering.

**Audit Log**: Immutable append-only JSONL. No deletion or modification operations exposed.

## Future Enhancements (Not Yet Implemented)

**Delta Backups**: Full snapshot + patches. Designed in schema (`StorageType: patch`, `BaseSnapshotID`, `PatchFrom`) but not implemented.

**Compression**: Snapshots stored as-is. TOML configs are small, compression not critical.

**Remote Storage**: S3/database backends. Interface exists, only `FilesystemStorage` implemented.

**Log Rotation**: Audit log grows indefinitely. No size limit or rotation.

**Encryption**: Currently plaintext. Future enhancement for sensitive configs.

## Common Pitfalls

1. **Forgetting to Close Audit Logger**: Buffered writes won't be visible until close/flush
2. **Assuming Symlink Paths**: Always use `os.Getwd()` for actual paths, not the path you passed to `os.Chdir()`
3. **Variable Shadowing in Tests**: Test file is in `main` package, global vars are visible
4. **Not Checking Dedup First**: Create backup before checking hash wastes storage
5. **Expecting Delta Chains**: Phase 1 has only full snapshots, each is own chain
6. **Manual Storage Init**: Manager auto-initializes, don't call `storage.Initialize()` manually
7. **Nil BackupManager**: All integration points must handle `nil` gracefully for backwards compatibility