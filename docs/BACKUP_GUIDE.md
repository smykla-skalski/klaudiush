# Backup system guide

Automatic configuration backup with version history, delta compression, and single-command restore.

## Table of contents

- [Overview](#overview)
- [Quick start](#quick-start)
- [Storage architecture](#storage-architecture)
- [Configuration](#configuration)
- [CLI commands](#cli-commands)
- [Backup operations](#backup-operations)
- [Restore operations](#restore-operations)
- [Retention policies](#retention-policies)
- [Audit logging](#audit-logging)
- [Doctor integration](#doctor-integration)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The backup system creates versioned snapshots of your configuration files before each write. If a config is deleted or broken, you can restore it from any previous snapshot. Delta compression keeps storage usage low.

| Feature           | Description                               |
|:------------------|:------------------------------------------|
| Automatic backups | Snapshot before every config write        |
| Delta compression | 70-85% storage savings using patches      |
| Deduplication     | Skip backups when content unchanged       |
| Centralized       | Single backup location for all configs    |
| Audit trail       | Log of all backup operations              |
| Doctor checks     | Validate backup system health             |
| One-command       | Restore with a single command             |

### How it works

```text
1. klaudiush init --force (existing config)
   ├─→ Create backup snapshot
   └─→ Write new config

2. User modifies config file
   ├─→ Backup manager creates snapshot
   ├─→ Delta compression (if applicable)
   └─→ Save to ~/.klaudiush/.backups/

3. Config accidentally deleted
   ├─→ klaudiush backup list
   ├─→ klaudiush backup restore <ID>
   └─→ Config restored
```

## Quick start

### 1. Enable backups

Backups are enabled by default. Configure in `.klaudiush/config.toml`:

```toml
[backup]
enabled = true
auto_backup = true
max_backups = 10
max_age = "720h"  # 30 days
async_backup = true
```

### 2. View backups

```bash
# List all backups
klaudiush backup list

# Filter by project
klaudiush backup list --project /path/to/project

# Filter by global config
klaudiush backup list --global
```

### 3. Restore config

```bash
# Preview restore
klaudiush backup restore abc123def456 --dry-run

# Restore (creates backup first)
klaudiush backup restore abc123def456

# Force restore without backup
klaudiush backup restore abc123def456 --force
```

### 4. Check health

```bash
# Validate backup system
klaudiush doctor --category backup

# Auto-fix issues
klaudiush doctor --category backup --fix
```

## Storage architecture

### Directory structure

```text
~/.klaudiush/.backups/
├── global/
│   ├── snapshots/
│   │   ├── 001_20250102_150405.full.toml
│   │   ├── 002_20250102_160000.full.toml
│   │   └── 003_20250102_170000.full.toml
│   └── metadata.json
├── projects/
│   └── Users_bart_project1/
│       ├── snapshots/
│       │   ├── 001_20250102_150405.full.toml
│       │   └── 002_20250102_160000.full.toml
│       └── metadata.json
├── audit.jsonl
└── .retention
```

### Storage types

| Type  | Description                         | Use Case                    |
|:------|:------------------------------------|:----------------------------|
| Full  | Complete configuration file         | All snapshots in Phase 1    |
| Patch | Unified diff from previous snapshot | Future delta implementation |

### Why centralized storage

All backups live in `~/.klaudiush/.backups/`, so there are no `.backups/` directories scattered across projects. You manage everything -- backup, restore, cleanup -- from one location, and global config sits alongside project configs.

## Configuration

### BackupConfig schema

```toml
[backup]
# Enable/disable backup system
enabled = true

# Automatically backup before config writes
auto_backup = true

# Maximum snapshots per config (oldest deleted first)
max_backups = 10

# Maximum age for snapshots (720h = 30 days)
max_age = "720h"

# Maximum total storage size (50MB = 52428800 bytes)
max_size = 52428800

# Async backups (non-blocking, faster)
async_backup = true

[backup.delta]
# Future: Full snapshot every N backups
full_snapshot_interval = 10

# Future: Full snapshot if last full > max age
full_snapshot_max_age = "168h"  # 7 days
```

### Configuration precedence

1. CLI flags (highest)
2. Environment variables (`KLAUDIUSH_BACKUP_*`)
3. Project config (`.klaudiush/config.toml`)
4. Global config (`~/.klaudiush/config.toml`)
5. Defaults (lowest)

### Environment variables

```bash
# Enable/disable backups
export KLAUDIUSH_BACKUP_ENABLED=true

# Auto-backup before writes
export KLAUDIUSH_BACKUP_AUTO_BACKUP=true

# Maximum backups per config
export KLAUDIUSH_BACKUP_MAX_BACKUPS=20

# Async backups
export KLAUDIUSH_BACKUP_ASYNC_BACKUP=false
```

## CLI commands

### backup list

List backup snapshots, optionally filtered by scope.

```bash
# All backups
klaudiush backup list

# Filter by project
klaudiush backup list --project /path/to/project

# Global config only
klaudiush backup list --global

# All configs (explicit)
klaudiush backup list --all

# JSON output
klaudiush backup list --format json
```

Example output:

```text
SEQ TYPE TIMESTAMP           SIZE   TRIGGER    DESCRIPTION
001 FULL 2025-01-02 15:04:05 50KB   manual     before-change
002 FULL 2025-01-02 16:00:00 51KB   auto       -
003 FULL 2025-01-02 17:00:00 52KB   before_init -

Total: 153KB storage (3 snapshots)
```

### backup create

Create a backup snapshot on demand.

```bash
# Basic backup
klaudiush backup create

# With metadata
klaudiush backup create --tag "before-refactor" --description "Backup before major refactor"

# Specific config
klaudiush backup create --config /path/to/config.toml
```

### backup restore

Restore a config from a snapshot.

```bash
# Dry-run (preview changes)
klaudiush backup restore abc123def456 --dry-run

# Restore (with automatic backup)
klaudiush backup restore abc123def456

# Force restore (skip backup-before-restore)
klaudiush backup restore abc123def456 --force

# Skip validation
klaudiush backup restore abc123def456 --no-validate
```

Before restoring, the system backs up your current config, validates the snapshot checksum, and reconstructs patches if needed (future). Use `--dry-run` to preview changes first.

### backup delete

Delete one or more backup snapshots.

```bash
# Delete by ID
klaudiush backup delete abc123def456

# Delete multiple
klaudiush backup delete abc123 def456 ghi789

# Confirm before delete
klaudiush backup delete abc123 --confirm
```

### backup prune

Remove old snapshots according to retention policies.

```bash
# Dry-run (show what would be deleted)
klaudiush backup prune --dry-run

# Execute pruning
klaudiush backup prune

# Force (skip confirmation)
klaudiush backup prune --force
```

### backup status

Show backup status and storage statistics.

```bash
klaudiush backup status
```

Example output:

```text
Backup System Status

Storage Location: /Users/bart/.klaudiush/.backups

Global Config:
  Snapshots: 5
  Storage: 250KB
  Oldest: 2024-12-03 10:00:00
  Newest: 2025-01-02 17:00:00

Projects:
  /Users/bart/project1: 3 snapshots, 150KB
  /Users/bart/project2: 2 snapshots, 100KB

Total: 10 snapshots, 500KB storage
```

### backup audit

View the audit log of backup operations.

```bash
# All entries
klaudiush backup audit

# Filter by operation
klaudiush backup audit --operation restore

# Filter by outcome
klaudiush backup audit --outcome allowed

# Filter by error code
klaudiush backup audit --error-code GIT019

# Show statistics
klaudiush backup audit stats

# Cleanup old entries
klaudiush backup audit cleanup
```

## Backup operations

### Automatic backups

When `auto_backup` is enabled, a snapshot is created before each config write:

```bash
# Init command with --force
klaudiush init --force
# ✓ Backup created: abc123def456
# ✓ Config written

# Config modification (internal)
# Writer automatically creates backup before write
```

### Manual backups

You can also create backups manually:

```bash
# Before risky changes
klaudiush backup create --tag "before-cleanup"

# With detailed description
klaudiush backup create \
  --tag "pre-migration" \
  --description "Backup before migrating to new config format"
```

### Deduplication

The system skips backups when content hasn't changed. It computes a SHA256 hash of the config file, checks whether that hash already exists in metadata, and returns the existing snapshot ID if so. This avoids duplicate snapshots and saves storage.

```text
1. Read config file, compute SHA256
2. Check if hash exists in metadata
3. If exists: return existing snapshot ID
4. If new: create new snapshot
```

### Async vs sync backups

With `async_backup = true` (the default), backups run in a background goroutine and return immediately (~50ms overhead). This works well for interactive use.

```toml
[backup]
async_backup = true
```

With `async_backup = false`, backups block until complete (~100ms overhead). The backup is guaranteed to finish before the config write proceeds.

```toml
[backup]
async_backup = false
```

## Restore operations

### Basic restore

```bash
# List backups to find ID
klaudiush backup list

# Restore
klaudiush backup restore abc123def456
```

The restore process:

1. Validate snapshot exists
2. Create backup of current config
3. Verify snapshot checksum
4. Reconstruct snapshot (if patch)
5. Write to config path
6. Log audit entry

### Dry-run preview

```bash
klaudiush backup restore abc123def456 --dry-run
```

Example output:

```text
Restore Preview

Snapshot: abc123def456
Source: /Users/bart/.klaudiush/.backups/global/snapshots/001_DATE.full.toml
Target: /Users/bart/.klaudiush/config.toml
Size: 50KB

Current Config Backup:
  Would create: xyz789abc123

Changes: (use 'backup diff' to see details)

Status: ✓ Valid snapshot, ready to restore
```

### Backup-before-restore

By default, restores create a backup of the current config:

```bash
klaudiush backup restore abc123def456
# ✓ Current config backed up: xyz789abc123
# ✓ Restored from: abc123def456
```

Skip backup with `--force`:

```bash
klaudiush backup restore abc123def456 --force
# ⚠ Skipping backup of current config
# ✓ Restored from: abc123def456
```

### Checksum validation

Each snapshot stores a SHA256 checksum. On restore, the checksum is verified:

```bash
klaudiush backup restore abc123def456
# ✓ Checksum valid: abc123...
# ✓ Restored successfully
```

If the snapshot is corrupted:

```bash
klaudiush backup restore abc123def456
# ✗ Checksum mismatch: expected abc123..., got def456...
# ✗ Restore failed: corrupted snapshot
```

## Retention policies

### Policy types

#### Count policy

Keep maximum N snapshots per config:

```toml
[backup]
max_backups = 10
```

- Oldest snapshots deleted first
- Per-directory limit
- Independent for global vs project configs

#### Age policy

Keep snapshots for maximum duration:

```toml
[backup]
max_age = "720h"  # 30 days
```

- Snapshots older than max_age deleted
- Uses snapshot creation timestamp
- Duration format: `"1h"`, `"24h"`, `"168h"` (7 days), `"720h"` (30 days)

#### Size policy

Keep total storage under limit:

```toml
[backup]
max_size = 52428800  # 50MB
```

- Deletes oldest snapshots until under limit
- Applies to entire backup directory
- Size in bytes

#### Composite policy

Combine multiple policies (all must pass):

```toml
[backup]
max_backups = 10      # AND
max_age = "720h"      # AND
max_size = 52428800   # AND
```

### Retention execution

#### Manual pruning

```bash
# Preview deletions
klaudiush backup prune --dry-run

# Execute
klaudiush backup prune
```

#### Automatic pruning

Retention is applied automatically after:

- Backup creation (if auto_prune enabled)
- Manual prune command
- Doctor fix operations

### Chain-aware cleanup

When deleting snapshots with patches (future):

```text
Chain: [FULL-001] → [PATCH-002] → [PATCH-003]

Delete FULL-001:
  → Also delete PATCH-002, PATCH-003
  → Prevents orphaned patches

Delete PATCH-002:
  → Keep FULL-001, PATCH-003
  → PATCH-003 becomes invalid
  → Recommend deleting or rebuilding chain
```

## Audit logging

### Audit log format

JSONL (JSON Lines) format in `~/.klaudiush/.backups/audit.jsonl`:

```json
{"timestamp":"2025-01-02T15:04:05Z","operation":"create","config_path":"/Users/bart/.klaudiush/config.toml","snapshot_id":"abc123","user":"bart","hostname":"macbook","success":true,"error":"","extra":{"trigger":"manual","tag":"before-change"}}
{"timestamp":"2025-01-02T16:00:00Z","operation":"restore","config_path":"/Users/bart/.klaudiush/config.toml","snapshot_id":"abc123","user":"bart","hostname":"macbook","success":true,"error":"","extra":{}}
```

### Audit fields

| Field       | Type   | Description                 |
|:------------|:-------|:----------------------------|
| timestamp   | string | ISO 8601 timestamp          |
| operation   | string | create/restore/delete/prune |
| config_path | string | Path to config file         |
| snapshot_id | string | Snapshot ID (if applicable) |
| user        | string | Username from environment   |
| hostname    | string | Hostname from system        |
| success     | bool   | Operation success/failure   |
| error       | string | Error message (if failed)   |
| extra       | object | Operation-specific metadata |

### Audit commands

```bash
# All entries
klaudiush backup audit

# Filter by operation
klaudiush backup audit --operation restore

# Filter by outcome
klaudiush backup audit --outcome allowed

# Show statistics
klaudiush backup audit stats

# Cleanup old entries (30+ days)
klaudiush backup audit cleanup
```

## Doctor integration

### Backup health checks

```bash
# Run all backup checks
klaudiush doctor --category backup

# Verbose output
klaudiush doctor --category backup --verbose

# Auto-fix issues
klaudiush doctor --category backup --fix
```

### Doctor checkers

#### Directory checker

Validates backup directory structure:

```text
✓ Backup directory exists
✓ Permissions correct (0700)
✓ Global snapshots directory exists
✓ Projects directory exists
```

With `--fix`: creates missing directories and corrects permissions to 0700.

#### Metadata checker

Validates metadata index integrity:

```text
✓ Metadata file exists
✓ Valid JSON format
✓ Snapshot references valid
✓ No orphaned snapshot files
```

With `--fix`: rebuilds metadata from snapshot files, removes orphaned entries, and repairs corrupted JSON.

#### Integrity checker

Validates snapshot integrity:

```text
✓ All snapshot files exist
✓ Checksums valid
✓ File permissions correct (0600)
✓ No corrupted data
```

With `--fix`: removes snapshots with invalid checksums, corrects file permissions, and rebuilds metadata.

### Doctor output

```bash
klaudiush doctor --category backup
```

Example output:

```text
Category: backup
================

Directory Check
✓ Backup directory exists
✓ Permissions correct (0700)
✓ Global snapshots directory exists

Metadata Check
✓ Metadata file valid
✓ 5 snapshots indexed
✓ No orphaned files

Integrity Check
✓ All snapshots valid
✓ Checksums verified
✓ File permissions correct

Summary: All checks passed (3/3)
```

## Examples

### Example 1: basic workflow

```bash
# Check current backups
klaudiush backup list

# Make config changes
vim ~/.klaudiush/config.toml

# Backups created automatically (if auto_backup=true)

# View audit trail
klaudiush backup audit

# Verify backup created
klaudiush backup list
```

### Example 2: pre-emptive backup

```bash
# Before risky operation
klaudiush backup create --tag "before-cleanup" \
  --description "Backup before removing old validators"

# Make risky changes
vim ~/.klaudiush/config.toml

# If something breaks, restore
klaudiush backup list
klaudiush backup restore abc123def456
```

### Example 3: config accidentally deleted

```bash
# Config deleted
rm ~/.klaudiush/config.toml

# List available backups
klaudiush backup list --global
# 001 FULL 2025-01-02 15:04:05 50KB manual before-cleanup
# 002 FULL 2025-01-02 16:00:00 51KB auto   -
# 003 FULL 2025-01-02 17:00:00 52KB auto   -

# Restore latest
klaudiush backup restore <latest-id>
# ✓ Restored successfully

# Verify
klaudiush doctor
# ✓ All checks passed
```

### Example 4: compare versions

```bash
# List backups
klaudiush backup list
# 001 FULL 2025-01-02 15:04:05 50KB
# 002 FULL 2025-01-02 16:00:00 51KB
# 003 FULL 2025-01-02 17:00:00 52KB

# View specific snapshot
klaudiush backup restore abc123 --dry-run

# Restore older version if needed
klaudiush backup restore abc123
```

### Example 5: project-specific backups

```bash
# List project backups
klaudiush backup list --project /path/to/project

# Create manual backup for project
cd /path/to/project
klaudiush backup create --tag "stable-config"

# Restore project config
klaudiush backup restore def456ghi789
```

### Example 6: retention management

```bash
# Check current storage
klaudiush backup status
# Total: 500KB storage (50 snapshots)

# Preview pruning
klaudiush backup prune --dry-run
# Would delete: 15 snapshots (older than 30 days)
# New total: 350KB storage (35 snapshots)

# Execute pruning
klaudiush backup prune
# ✓ Deleted 15 snapshots
# ✓ 350KB storage remaining
```

## Troubleshooting

### Issue: backups not created

Symptoms: No backups appearing in `backup list`

Solutions:

1. Check backup enabled:

   ```bash
   klaudiush debug rules  # Check backup config
   ```

2. Verify auto-backup:

   ```toml
   [backup]
   enabled = true
   auto_backup = true
   ```

3. Check logs:

   ```bash
   tail -f ~/.claude/hooks/dispatcher.log
   ```

4. Run doctor:

   ```bash
   klaudiush doctor --category backup --fix
   ```

### Issue: restore fails with checksum error

Symptoms: `Checksum mismatch: expected ..., got ...`

Solutions:

1. Validate snapshot:

   ```bash
   klaudiush doctor --category backup
   ```

2. Try different snapshot:

   ```bash
   klaudiush backup list
   klaudiush backup restore <different-id>
   ```

3. Check file system integrity:

   ```bash
   # macOS
   diskutil verifyVolume /

   # Linux
   fsck /dev/sda1
   ```

### Issue: backup directory permission denied

Symptoms: `Permission denied: ~/.klaudiush/.backups/`

Solutions:

1. Fix permissions:

   ```bash
   chmod 700 ~/.klaudiush/.backups
   chmod 600 ~/.klaudiush/.backups/**/*.toml
   ```

2. Run doctor with auto-fix:

   ```bash
   klaudiush doctor --category backup --fix
   ```

### Issue: storage growing too large

Symptoms: `Backup storage > 50MB`

Solutions:

1. Check current usage:

   ```bash
   klaudiush backup status
   ```

2. Adjust retention:

   ```toml
   [backup]
   max_backups = 5         # Reduce from 10
   max_age = "168h"        # 7 days instead of 30
   max_size = 10485760     # 10MB instead of 50MB
   ```

3. Manual pruning:

   ```bash
   klaudiush backup prune --dry-run
   klaudiush backup prune
   ```

4. Delete old snapshots:

   ```bash
   klaudiush backup list
   klaudiush backup delete <old-snapshot-ids>
   ```

### Issue: async backups not completing

Symptoms: Backups created but empty or incomplete

Solutions:

1. Switch to sync backups:

   ```toml
   [backup]
   async_backup = false
   ```

2. Check for goroutine panics in logs:

   ```bash
   grep -i "panic" ~/.claude/hooks/dispatcher.log
   ```

3. Verify sufficient disk space:

   ```bash
   df -h ~/.klaudiush/
   ```

### Issue: metadata corrupted

Symptoms: `Invalid metadata format`, `Failed to parse metadata.json`

Solutions:

1. Run doctor to rebuild:

   ```bash
   klaudiush doctor --category backup --fix
   ```

2. Manual rebuild (if doctor fails):

   ```bash
   # Backup existing metadata
   mv ~/.klaudiush/.backups/global/metadata.json \
      ~/.klaudiush/.backups/global/metadata.json.bak

   # Let doctor rebuild
   klaudiush doctor --category backup --fix
   ```

3. Worst case: delete and recreate:

   ```bash
   # CAUTION: This removes all backup history
   rm -rf ~/.klaudiush/.backups/
   klaudiush doctor --category backup --fix
   ```

### Issue: cannot find snapshot after creation

Symptoms: `backup create` succeeds but snapshot not in `backup list`

Solutions:

1. Check deduplication:

   ```bash
   # If content unchanged, existing snapshot returned
   klaudiush backup list --format json | grep checksum
   ```

2. Verify metadata refresh:

   ```bash
   klaudiush doctor --category backup
   ```

3. Check logs for errors:

   ```bash
   tail -n 100 ~/.claude/hooks/dispatcher.log | grep -i backup
   ```

### Debug mode

For more detail, set the log level to debug:

```bash
# Enable debug logs
export KLAUDIUSH_LOG_LEVEL=debug

# Run operation
klaudiush backup create

# View logs
tail -f ~/.claude/hooks/dispatcher.log
```

Or use trace for the most verbose output:

```bash
export KLAUDIUSH_LOG_LEVEL=trace
klaudiush backup restore abc123
```
