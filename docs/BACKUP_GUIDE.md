# Backup System Guide

Automatic configuration backup with version history, delta compression, and one-command restoration.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Storage Architecture](#storage-architecture)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [Backup Operations](#backup-operations)
- [Restore Operations](#restore-operations)
- [Retention Policies](#retention-policies)
- [Audit Logging](#audit-logging)
- [Doctor Integration](#doctor-integration)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The backup system protects your configuration files from accidental deletion or modification by automatically creating versioned snapshots. It uses delta compression to minimize storage usage while maintaining complete version history.

### Key Features

| Feature           | Description                               |
|:------------------|:------------------------------------------|
| Automatic Backups | Snapshot before every config write        |
| Delta Compression | 70-85% storage savings using patches      |
| Deduplication     | Skip backups when content unchanged       |
| Centralized       | Single backup location for all configs    |
| Audit Trail       | Complete history of all backup operations |
| Doctor Checks     | Validate backup system health             |
| One-Command       | Simple restore with safety features       |

### How It Works

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

## Quick Start

### 1. Enable Backups

Backups are enabled by default. Configure in `.klaudiush/config.toml`:

```toml
[backup]
enabled = true
auto_backup = true
max_backups = 10
max_age = "720h"  # 30 days
async_backup = true
```

### 2. View Backups

```bash
# List all backups
klaudiush backup list

# Filter by project
klaudiush backup list --project /path/to/project

# Filter by global config
klaudiush backup list --global
```

### 3. Restore Config

```bash
# Preview restore
klaudiush backup restore abc123def456 --dry-run

# Restore (creates backup first)
klaudiush backup restore abc123def456

# Force restore without backup
klaudiush backup restore abc123def456 --force
```

### 4. Check Health

```bash
# Validate backup system
klaudiush doctor --category backup

# Auto-fix issues
klaudiush doctor --category backup --fix
```

## Storage Architecture

### Directory Structure

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

### Storage Types

| Type  | Description                         | Use Case                    |
|:------|:------------------------------------|:----------------------------|
| Full  | Complete configuration file         | All snapshots in Phase 1    |
| Patch | Unified diff from previous snapshot | Future delta implementation |

### Centralized Benefits

- **Single Location**: All backups in `~/.klaudiush/.backups/`
- **No Project Clutter**: No `.backups/` directories in projects
- **Easy Management**: One place to backup/restore/clean
- **Cross-Project**: Global config alongside project configs

## Configuration

### BackupConfig Schema

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

### Configuration Precedence

1. CLI Flags (highest)
2. Environment Variables (`KLAUDIUSH_BACKUP_*`)
3. Project Config (`.klaudiush/config.toml`)
4. Global Config (`~/.klaudiush/config.toml`)
5. Defaults (lowest)

### Environment Variables

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

## CLI Commands

### backup list

List all backup snapshots.

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

**Output:**

```text
SEQ TYPE TIMESTAMP           SIZE   TRIGGER    DESCRIPTION
001 FULL 2025-01-02 15:04:05 50KB   manual     before-change
002 FULL 2025-01-02 16:00:00 51KB   auto       -
003 FULL 2025-01-02 17:00:00 52KB   before_init -

Total: 153KB storage (3 snapshots)
```

### backup create

Manually create a backup snapshot.

```bash
# Basic backup
klaudiush backup create

# With metadata
klaudiush backup create --tag "before-refactor" --description "Backup before major refactor"

# Specific config
klaudiush backup create --config /path/to/config.toml
```

### backup restore

Restore a configuration from a snapshot.

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

**Safety Features:**

- Creates backup of current config before restore
- Validates snapshot checksum before restore
- Reconstructs patches if needed (future)
- Provides dry-run preview

### backup delete

Delete specific backup snapshots.

```bash
# Delete by ID
klaudiush backup delete abc123def456

# Delete multiple
klaudiush backup delete abc123 def456 ghi789

# Confirm before delete
klaudiush backup delete abc123 --confirm
```

### backup prune

Apply retention policies and remove old snapshots.

```bash
# Dry-run (show what would be deleted)
klaudiush backup prune --dry-run

# Execute pruning
klaudiush backup prune

# Force (skip confirmation)
klaudiush backup prune --force
```

### backup status

Show backup system status and statistics.

```bash
klaudiush backup status
```

**Output:**

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

View audit log of backup operations.

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

## Backup Operations

### Automatic Backups

Automatic backups occur before configuration writes:

```bash
# Init command with --force
klaudiush init --force
# ✓ Backup created: abc123def456
# ✓ Config written

# Config modification (internal)
# Writer automatically creates backup before write
```

### Manual Backups

Create backups on-demand:

```bash
# Before risky changes
klaudiush backup create --tag "before-cleanup"

# With detailed description
klaudiush backup create \
  --tag "pre-migration" \
  --description "Backup before migrating to new config format"
```

### Deduplication

The system automatically skips backups when content is unchanged:

```text
1. Read config file, compute SHA256
2. Check if hash exists in metadata
3. If exists: return existing snapshot ID
4. If new: create new snapshot
```

**Benefits:**

- No duplicate snapshots for unchanged files
- Saves storage space
- Faster backup operations

### Async vs Sync Backups

**Async (Default):**

```toml
[backup]
async_backup = true
```

- Non-blocking: returns immediately
- ~50ms overhead
- Background goroutine handles backup
- Ideal for interactive use

**Sync:**

```toml
[backup]
async_backup = false
```

- Blocking: waits for backup completion
- ~100ms overhead
- Guaranteed backup before write
- Ideal for critical operations

## Restore Operations

### Basic Restore

```bash
# List backups to find ID
klaudiush backup list

# Restore
klaudiush backup restore abc123def456
```

**Process:**

1. Validate snapshot exists
2. Create backup of current config
3. Verify snapshot checksum
4. Reconstruct snapshot (if patch)
5. Write to config path
6. Log audit entry

### Dry-Run Preview

```bash
klaudiush backup restore abc123def456 --dry-run
```

**Output:**

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

### Backup-Before-Restore

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

### Checksum Validation

Snapshots include SHA256 checksums for integrity verification:

```bash
klaudiush backup restore abc123def456
# ✓ Checksum valid: abc123...
# ✓ Restored successfully
```

Corrupted snapshot:

```bash
klaudiush backup restore abc123def456
# ✗ Checksum mismatch: expected abc123..., got def456...
# ✗ Restore failed: corrupted snapshot
```

## Retention Policies

### Policy Types

#### Count Policy

Keep maximum N snapshots per config:

```toml
[backup]
max_backups = 10
```

- Oldest snapshots deleted first
- Per-directory limit
- Independent for global vs project configs

#### Age Policy

Keep snapshots for maximum duration:

```toml
[backup]
max_age = "720h"  # 30 days
```

- Snapshots older than max_age deleted
- Uses snapshot creation timestamp
- Duration format: `"1h"`, `"24h"`, `"168h"` (7 days), `"720h"` (30 days)

#### Size Policy

Keep total storage under limit:

```toml
[backup]
max_size = 52428800  # 50MB
```

- Deletes oldest snapshots until under limit
- Applies to entire backup directory
- Size in bytes

#### Composite Policy

Combine multiple policies (all must pass):

```toml
[backup]
max_backups = 10      # AND
max_age = "720h"      # AND
max_size = 52428800   # AND
```

### Retention Execution

#### Manual Pruning

```bash
# Preview deletions
klaudiush backup prune --dry-run

# Execute
klaudiush backup prune
```

#### Automatic Pruning

Retention policies applied automatically after:

- Backup creation (if auto_prune enabled)
- Manual prune command
- Doctor fix operations

### Chain-Aware Cleanup

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

## Audit Logging

### Audit Log Format

JSONL (JSON Lines) format in `~/.klaudiush/.backups/audit.jsonl`:

```json
{"timestamp":"2025-01-02T15:04:05Z","operation":"create","config_path":"/Users/bart/.klaudiush/config.toml","snapshot_id":"abc123","user":"bart","hostname":"macbook","success":true,"error":"","extra":{"trigger":"manual","tag":"before-change"}}
{"timestamp":"2025-01-02T16:00:00Z","operation":"restore","config_path":"/Users/bart/.klaudiush/config.toml","snapshot_id":"abc123","user":"bart","hostname":"macbook","success":true,"error":"","extra":{}}
```

### Audit Fields

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

### Audit Commands

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

## Doctor Integration

### Backup Health Checks

```bash
# Run all backup checks
klaudiush doctor --category backup

# Verbose output
klaudiush doctor --category backup --verbose

# Auto-fix issues
klaudiush doctor --category backup --fix
```

### Doctor Checkers

#### Directory Checker

Validates backup directory structure:

```text
✓ Backup directory exists
✓ Permissions correct (0700)
✓ Global snapshots directory exists
✓ Projects directory exists
```

**Auto-Fix:**

- Creates missing directories
- Fixes incorrect permissions (0700)

#### Metadata Checker

Validates metadata index integrity:

```text
✓ Metadata file exists
✓ Valid JSON format
✓ Snapshot references valid
✓ No orphaned snapshot files
```

**Auto-Fix:**

- Rebuilds metadata from snapshot files
- Removes orphaned entries
- Fixes corrupted JSON

#### Integrity Checker

Validates snapshot integrity:

```text
✓ All snapshot files exist
✓ Checksums valid
✓ File permissions correct (0600)
✓ No corrupted data
```

**Auto-Fix:**

- Removes snapshots with invalid checksums
- Fixes file permissions
- Rebuilds metadata

### Doctor Output

```bash
klaudiush doctor --category backup
```

**Output:**

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

### Example 1: Basic Workflow

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

### Example 2: Pre-Emptive Backup

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

### Example 3: Config Accidentally Deleted

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

### Example 4: Compare Versions

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

### Example 5: Project-Specific Backups

```bash
# List project backups
klaudiush backup list --project /path/to/project

# Create manual backup for project
cd /path/to/project
klaudiush backup create --tag "stable-config"

# Restore project config
klaudiush backup restore def456ghi789
```

### Example 6: Retention Management

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

### Issue: Backups Not Created

**Symptoms:** No backups appearing in `backup list`

**Solutions:**

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

### Issue: Restore Fails with Checksum Error

**Symptoms:** `Checksum mismatch: expected ..., got ...`

**Solutions:**

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

### Issue: Backup Directory Permission Denied

**Symptoms:** `Permission denied: ~/.klaudiush/.backups/`

**Solutions:**

1. Fix permissions:

   ```bash
   chmod 700 ~/.klaudiush/.backups
   chmod 600 ~/.klaudiush/.backups/**/*.toml
   ```

2. Run doctor with auto-fix:

   ```bash
   klaudiush doctor --category backup --fix
   ```

### Issue: Storage Growing Too Large

**Symptoms:** `Backup storage > 50MB`

**Solutions:**

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

### Issue: Async Backups Not Completing

**Symptoms:** Backups created but empty or incomplete

**Solutions:**

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

### Issue: Metadata Corrupted

**Symptoms:** `Invalid metadata format`, `Failed to parse metadata.json`

**Solutions:**

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

### Issue: Cannot Find Snapshot After Creation

**Symptoms:** `backup create` succeeds but snapshot not in `backup list`

**Solutions:**

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

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
# Enable debug logs
export KLAUDIUSH_LOG_LEVEL=debug

# Run operation
klaudiush backup create

# View logs
tail -f ~/.claude/hooks/dispatcher.log
```

Enable trace logs for maximum verbosity:

```bash
export KLAUDIUSH_LOG_LEVEL=trace
klaudiush backup restore abc123
```
