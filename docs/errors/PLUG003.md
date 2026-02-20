# PLUG003: Invalid plugin file extension

## Error

The plugin file has an extension that is not in the allowed list.

## Why this matters

Restricting file extensions prevents loading unexpected file types as plugins. Only known executable formats should be loaded.

## How to fix

Ensure the plugin file has an allowed extension. Common allowed extensions include `.sh`, `.bash`, `.py`, `.rb`, `.js`.

```toml
[plugins.my-plugin]
path = "~/.klaudiush/plugins/my-plugin.sh"   # .sh is allowed
```

## Configuration

Allowed extensions are configured per-plugin or globally:

```toml
[plugins]
allowed_extensions = [".sh", ".bash", ".py", ".rb", ".js"]
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[PLUG003] Invalid plugin file extension`

**systemMessage** (shown to user):
Formatted error with reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [PLUG002](PLUG002.md) - plugin path outside allowed directories
