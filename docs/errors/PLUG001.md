# PLUG001: Path traversal in plugin path

## Error

The plugin path contains directory traversal patterns (`../`) that could escape the allowed plugin directory.

## Why this matters

Path traversal is a common attack vector. A plugin path like `../../etc/passwd` could reference files outside the intended plugin directory, potentially leading to arbitrary code execution.

## How to fix

Use an absolute path or a path relative to the plugin directory without `..` components:

```toml
# Instead of:
[plugins.my-plugin]
path = "../../../malicious/script.sh"

# Use:
[plugins.my-plugin]
path = "~/.klaudiush/plugins/my-plugin.sh"
```

## Configuration

Plugin paths are restricted to allowed directories:

- Global: `~/.klaudiush/plugins/`
- Project: `.klaudiush/plugins/`

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[PLUG001] Path traversal detected in plugin path`

**systemMessage** (shown to user):
Formatted error with reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [PLUG002](PLUG002.md) - plugin path outside allowed directories
- [PLUG005](PLUG005.md) - dangerous characters in plugin path
