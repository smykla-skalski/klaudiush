# Exec Plugin Example: File Validator (Shell)

This example demonstrates how to create a klaudiush exec plugin using a shell script.

## Overview

The plugin validates file operations (`Write` and `Edit` tools) and:

- **Blocks** binary files (`.exe`, `.dll`, `.so`, `.dylib`, `.bin`)
- **Warns** about executable scripts with shebangs (`.sh`, `.py`, `.rb`, `.pl`)
- **Blocks** files exceeding configured size limit

## Features

- Cross-platform (any shell with bash)
- No compilation required
- Hot-reload capable (changes take effect immediately)
- Configurable via TOML

## Installation

### Make Executable

```bash
chmod +x file_validator.sh
```

### Install to Plugin Directory

```bash
mkdir -p ~/.klaudiush/plugins
cp file_validator.sh ~/.klaudiush/plugins/
```

## Configuration

Add to `~/.klaudiush/config.toml`:

```toml
[[plugins.plugins]]
name = "file-validator"
type = "exec"
enabled = true
path = "~/.klaudiush/plugins/file_validator.sh"
timeout = "5s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Write", "Edit"]

[plugins.plugins.config]
warn_on_exe = "true"        # Warn about executable scripts
block_on_bin = "true"       # Block binary files
max_file_size = "1048576"   # 1MB max file size
```

## Testing

### Test Info

```bash
./file_validator.sh --info
```

Expected output:

```json
{
  "name": "file-validator",
  "version": "1.0.0",
  "description": "Validates file operations (blocks binaries, warns on executables)",
  "author": "klaudiush"
}
```

### Test Binary File (Should Block)

```bash
echo '{
  "tool_name": "Write",
  "file_path": "malware.exe",
  "content": "binary content"
}' | ./file_validator.sh
```

Expected output:

```json
{
  "passed": false,
  "should_block": true,
  "message": "Binary files are not allowed: malware.exe",
  "error_code": "BIN_FILE",
  "fix_hint": "Use source code or text files instead"
}
```

### Test Executable Script (Should Warn)

```bash
echo '{
  "tool_name": "Write",
  "file_path": "script.sh",
  "content": "#!/bin/bash\necho hello"
}' | ./file_validator.sh
```

Expected output:

```json
{
  "passed": false,
  "should_block": false,
  "message": "Executable script detected: script.sh",
  "error_code": "EXEC_SCRIPT",
  "fix_hint": "Ensure script has proper validation and error handling"
}
```

### Test Normal File (Should Pass)

```bash
echo '{
  "tool_name": "Write",
  "file_path": "README.md",
  "content": "# Documentation"
}' | ./file_validator.sh
```

Expected output:

```json
{
  "passed": true,
  "should_block": false
}
```

## Customization

### Disable Binary Blocking

```toml
[plugins.plugins.config]
block_on_bin = "false"
```

### Increase File Size Limit

```toml
[plugins.plugins.config]
max_file_size = "10485760"  # 10MB
```

### Add Custom File Extensions

Edit the script to add more patterns:

```bash
case "$file_path" in
  *.exe|*.dll|*.so|*.dylib|*.bin|*.jar|*.war)
    # Block these file types
    ;;
esac
```

## Protocol

### Info Request

```bash
./file_validator.sh --info
```

### Validate Request (stdin)

```json
{
  "event_type": "PreToolUse",
  "tool_name": "Write",
  "file_path": "/path/to/file.txt",
  "content": "file content here",
  "config": {
    "warn_on_exe": "true",
    "block_on_bin": "true",
    "max_file_size": "1048576"
  }
}
```

### Validate Response (stdout)

Pass response:

```json
{
  "passed": true,
  "should_block": false
}
```

Fail response:

```json
{
  "passed": false,
  "should_block": true,
  "message": "Error message",
  "error_code": "ERROR_CODE",
  "fix_hint": "Suggested fix",
  "doc_link": "https://docs.example.com",
  "details": {
    "key": "value"
  }
}
```

## Requirements

- bash
- jq (for JSON parsing)

Install jq:

```bash
# macOS
brew install jq

# Ubuntu/Debian
apt-get install jq

# RHEL/CentOS
yum install jq
```

## Next Steps

- See [Python Exec Plugin Example](../exec-python/) for another language example
- See [Go Plugin Example](../go-plugin/) for maximum performance
- See [gRPC Plugin Example](../grpc-go/) for persistent connections
- Read [Plugin Development Guide](../../../docs/PLUGIN_GUIDE.md) for more details
