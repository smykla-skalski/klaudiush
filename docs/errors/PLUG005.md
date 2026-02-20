# PLUG005: Dangerous characters in plugin path

## Error

The plugin path contains shell metacharacters (`;`, `|`, `&`, `$`, backticks, quotes, `<`, `>`, `(`, `)`).

## Why this matters

While `exec.Command` doesn't interpret shell metacharacters, rejecting them adds defense-in-depth against potential injection if the path is ever used in a shell context. Suspicious paths are blocked as a precaution.

## How to fix

Remove shell metacharacters from the plugin path:

```bash
# Instead of:
~/.klaudiush/plugins/my;plugin.sh
~/.klaudiush/plugins/my|plugin.sh

# Use:
~/.klaudiush/plugins/my-plugin.sh
~/.klaudiush/plugins/my_plugin.sh
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[PLUG005] Dangerous characters in plugin path`

**systemMessage** (shown to user):
Formatted error with reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [PLUG001](PLUG001.md) - path traversal in plugin path
- [PLUG002](PLUG002.md) - plugin path outside allowed directories
