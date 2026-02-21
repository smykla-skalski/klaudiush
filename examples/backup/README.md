# Backup configuration examples

Example backup configurations for different use cases.

## Available examples

### basic.toml

Standard configuration suitable for most users.

- 10 snapshots max
- 30 days retention
- 50MB storage limit
- Async backups

Use when you need general purpose backup with balanced storage and history.

### minimal.toml

Conservative configuration for limited storage.

- 5 snapshots max
- 7 days retention
- 10MB storage limit
- Async backups

Use when disk space is limited or short-term version history is sufficient.

### production.toml

Production-focused with extended retention.

- 20 snapshots max
- 90 days retention
- 100MB storage limit
- Sync backups (guaranteed completion)

Use in production environments where backup integrity and extended version history matter.

### development.toml

Development-optimized for frequent changes.

- 15 snapshots max
- 14 days retention
- 50MB storage limit
- Async backups

Use during active development with frequent config changes and two-week sprint cycles.

## Usage

Copy the desired configuration to your config file:

```bash
# Global config
cp examples/backup/basic.toml ~/.klaudiush/config.toml

# Project config
cp examples/backup/production.toml .klaudiush/config.toml
```

Or merge with existing config:

```bash
# Append backup section to existing config
cat examples/backup/basic.toml >> ~/.klaudiush/config.toml
```

## Configuration reference

See `docs/BACKUP_GUIDE.md` for complete documentation.

### Settings

| Setting      | Description                     | Values                 |
|:-------------|:--------------------------------|:-----------------------|
| enabled      | Enable/disable backup system    | true, false            |
| auto_backup  | Automatic backups before writes | true, false            |
| max_backups  | Maximum snapshots per directory | 1-100                  |
| max_age      | Maximum snapshot age            | Duration (e.g. "720h") |
| max_size     | Maximum total storage           | Bytes                  |
| async_backup | Non-blocking backups            | true, false            |

### Retention policies

All retention policies work together (AND logic):

- Count: keeps N most recent snapshots
- Age: deletes snapshots older than duration
- Size: deletes oldest when total size exceeds limit

### Duration format

| Duration | Format  | Example |
|:---------|:--------|:--------|
| 1 hour   | "1h"    | "1h"    |
| 1 day    | "24h"   | "24h"   |
| 7 days   | "168h"  | "168h"  |
| 30 days  | "720h"  | "720h"  |
| 90 days  | "2160h" | "2160h" |

## Testing configuration

Test your backup configuration:

```bash
# Check configuration
klaudiush debug rules

# Create test backup
klaudiush backup create --tag "test"

# List backups
klaudiush backup list

# Check system status
klaudiush backup status

# Verify with doctor
klaudiush doctor --category backup
```

## Customization

Adjust settings based on your needs:

More history:

```toml
max_backups = 30
max_age = "4320h"  # 180 days
```

Less storage:

```toml
max_backups = 3
max_size = 5242880  # 5MB
```

Guaranteed backups:

```toml
async_backup = false
```

## See also

- `docs/BACKUP_GUIDE.md` - Complete backup system guide
- `examples/config/` - Full configuration examples
- `examples/exceptions/` - Exception configuration examples
- `examples/rules/` - Rule configuration examples
