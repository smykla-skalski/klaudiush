# PLUG002: Plugin path outside allowed directories

## Error

The plugin path resolves to a location outside the allowed plugin directories.

## Why this matters

Restricting plugin paths to known directories prevents executing arbitrary binaries. Only plugins in the global (`~/.klaudiush/plugins/`) or project (`.klaudiush/plugins/`) directories are trusted.

## How to fix

Move the plugin to an allowed directory:

```bash
# Global plugins
cp my-plugin.sh ~/.klaudiush/plugins/
chmod +x ~/.klaudiush/plugins/my-plugin.sh

# Project plugins
cp my-plugin.sh .klaudiush/plugins/
chmod +x .klaudiush/plugins/my-plugin.sh
```

Then reference it in configuration:

```toml
[plugins.my-plugin]
path = "~/.klaudiush/plugins/my-plugin.sh"
```

## Configuration

Allowed directories are fixed by design:

- `~/.klaudiush/plugins/` (global)
- `.klaudiush/plugins/` (project-local)

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[PLUG002] Plugin path not in allowed directory`

**systemMessage** (shown to user):
Formatted error with reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [PLUG001](PLUG001.md) - path traversal in plugin path
- [PLUG003](PLUG003.md) - invalid plugin file extension
