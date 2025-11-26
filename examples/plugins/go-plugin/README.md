# Go Plugin Example: Dangerous Commands Blocker

This example demonstrates how to create a klaudiush Go plugin (.so) that blocks dangerous shell commands.

## Overview

The plugin validates `Bash` tool commands and blocks patterns like:

- `rm -rf /` - Recursive root deletion
- `dd if=/dev/zero` - Disk wiping
- `mkfs` - Filesystem formatting
- `:(){ :|:& };:` - Fork bomb

## Building

### Prerequisites

- Go 1.19 or later (must match klaudiush's Go version)
- Same OS and architecture as klaudiush

### Build Command

```bash
go build -buildmode=plugin -o dangerous_commands.so dangerous_commands.go
```

### Install

```bash
# Copy to plugin directory
mkdir -p ~/.klaudiush/plugins
cp dangerous_commands.so ~/.klaudiush/plugins/
```

## Configuration

Add to `~/.klaudiush/config.toml`:

```toml
[[plugins.plugins]]
name = "dangerous-commands"
type = "go"
enabled = true
path = "~/.klaudiush/plugins/dangerous_commands.so"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]

[plugins.plugins.config]
# Optional: Override default blocked commands
blocked_commands = [
    "rm -rf /",
    "dd if=/dev/zero",
    "mkfs",
    ":(){ :|:& };:",
    "curl | sh"  # Add custom patterns
]
```

## Testing

### Test with echo

```bash
# This should be blocked
echo '{"event_type":"PreToolUse","tool_name":"Bash","command":"rm -rf /"}' | \
  klaudiush --hook-type PreToolUse
```

### Integration Test

Start klaudiush dispatcher, then try a dangerous command in Claude Code - it should be blocked

## Customization

### Adding Custom Patterns

Edit config to include your own dangerous patterns:

```toml
[plugins.plugins.config]
blocked_commands = [
    "rm -rf /",
    "sudo reboot",
    "shutdown -h now",
    "pkill -9"
]
```

### Modifying the Code

```go
// Add warning mode instead of blocking
if someCondition {
    return plugin.WarnResponse("Warning: potentially dangerous command")
}
```

## Limitations

- **No hot-reload**: Requires klaudiush restart after rebuild
- **Version compatibility**: Must rebuild if klaudiush updates Go version
- **Platform-specific**: Must rebuild for each OS/architecture
- **No unloading**: Plugin stays in memory until klaudiush restart

## Next Steps

- See [Exec Plugin Example](../exec-shell/) for cross-language compatibility
- See [gRPC Plugin Example](../grpc-go/) for hot-reload capability
- Read [Plugin Development Guide](../../../docs/PLUGIN_GUIDE.md) for more details
