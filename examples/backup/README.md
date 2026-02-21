# Backup

Four presets covering common backup scenarios. Each file is a complete `[backup]` section you can drop into your config.

All retention policies work together: count (keep N most recent), age (delete after duration), and size (delete oldest when limit is exceeded).

See the [backup guide](/docs/backup) for full documentation on retention, delta compression, and the `klaudiush backup` commands.
