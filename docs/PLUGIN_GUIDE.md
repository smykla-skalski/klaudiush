# Plugin development guide

Guide for developing klaudiush exec plugins.

## Table of contents

- [Overview](#overview)
- [Quick start](#quick-start)
- [Protocol reference](#protocol-reference)
- [Plugin configuration](#plugin-configuration)
- [Predicate matching](#predicate-matching)
- [Examples](#examples)
- [Best practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

klaudiush plugins add custom validation logic. You can enforce org-specific
rules, call external services, or block specific patterns.

Plugins are standalone executables that communicate with klaudiush via JSON over
stdin/stdout. Any language that can read stdin and write JSON to stdout works:
shell scripts, Python, Ruby, Node.js, compiled binaries, etc.

### How it works

1. klaudiush spawns the plugin executable as a subprocess
2. A JSON request is written to the plugin's stdin
3. The plugin writes a JSON response to stdout
4. klaudiush acts on the response (pass, warn, or block)

Each invocation is a fresh process, so plugins are stateless by default and
changes to the script take effect immediately (no restart needed).

## Quick start

### 1. Create a plugin

```bash
#!/usr/bin/env bash
set -euo pipefail

# Handle --info flag (metadata request)
if [[ "${1:-}" == "--info" ]]; then
  echo '{"name":"my-plugin","version":"1.0.0","description":"My custom validator"}'
  exit 0
fi

# Read validation request from stdin
read -r request
tool_name=$(echo "$request" | jq -r '.tool_name')
command=$(echo "$request" | jq -r '.command // empty')

# Validation logic
if [[ "$tool_name" == "Bash" ]] && [[ "$command" == *"sudo"* ]]; then
  cat <<EOF
{"passed":false,"should_block":true,"message":"sudo commands are not allowed","error_code":"NO_SUDO","fix_hint":"Run without sudo or request elevated permissions"}
EOF
  exit 0
fi

echo '{"passed":true,"should_block":false}'
```

### 2. Install

```bash
chmod +x my-plugin.sh
mkdir -p ~/.klaudiush/plugins
cp my-plugin.sh ~/.klaudiush/plugins/
```

### 3. Configure

Add to `~/.klaudiush/config.toml` or `.klaudiush/config.toml`:

```toml
[plugins]
enabled = true

[[plugins.plugins]]
name = "my-plugin"
type = "exec"
path = "~/.klaudiush/plugins/my-plugin.sh"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
```

### 4. Test

```bash
./my-plugin.sh --info
echo '{"tool_name":"Bash","command":"sudo rm -rf /"}' | ./my-plugin.sh
echo '{"tool_name":"Bash","command":"ls"}' | ./my-plugin.sh
```

## Protocol reference

### Info request

klaudiush calls the plugin with `--info` to retrieve metadata:

```bash
./my-plugin --info
```

**Response** (JSON to stdout):

```json
{"name": "my-plugin", "version": "1.0.0", "description": "Plugin description"}
```

Fields `name` and `version` are required. Optional: `description`, `author`, `url`.

### Validate request

klaudiush writes a JSON object to stdin. Fields present depend on the tool:

| Field        | Type           | Present When    | Description                           |
|:-------------|:---------------|:----------------|:--------------------------------------|
| `event_type` | string         | Always          | `"PreToolUse"`, `"PostToolUse"`, etc. |
| `tool_name`  | string         | Always          | `"Bash"`, `"Write"`, `"Edit"`, etc.   |
| `command`    | string         | Bash tool       | Shell command being executed          |
| `file_path`  | string         | Write/Edit/Read | Path to the file being operated on    |
| `content`    | string         | Write tool      | Content being written                 |
| `old_string` | string         | Edit tool       | String being replaced                 |
| `new_string` | string         | Edit tool       | Replacement string                    |
| `pattern`    | string         | Grep/Glob tools | Search pattern                        |
| `config`     | map[string]any | If configured   | Plugin-specific config from TOML      |

### Validate response

The plugin writes a JSON response to stdout:

| Field          | Type              | Required | Description                                         |
|:---------------|:------------------|:---------|:----------------------------------------------------|
| `passed`       | bool              | Yes      | Whether validation passed                           |
| `should_block` | bool              | Yes      | Whether to block the operation (only if not passed) |
| `message`      | string            | No       | Human-readable result message                       |
| `error_code`   | string            | No       | Unique error identifier for programmatic handling   |
| `fix_hint`     | string            | No       | Short suggestion for fixing the issue               |
| `doc_link`     | string            | No       | URL to detailed error documentation                 |
| `details`      | map[string]string | No       | Additional structured information                   |

**Response semantics**:

- `passed: true` -- operation proceeds
- `passed: false, should_block: false` -- warning logged, operation proceeds
- `passed: false, should_block: true` -- operation blocked (exit 2)

### Exit codes

- **Exit 0**: Plugin ran successfully. Result is read from stdout.
- **Non-zero exit**: Plugin error. klaudiush logs the error and allows the operation (fail-open).

Always exit 0 and communicate validation failures through JSON, not exit codes.

## Plugin configuration

### Global configuration

**File**: `~/.klaudiush/config.toml`

```toml
[plugins]
enabled = true
directory = "~/.klaudiush/plugins"
default_timeout = "5s"

[[plugins.plugins]]
name = "example"
type = "exec"
enabled = true
path = "~/.klaudiush/plugins/example.sh"
timeout = "10s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
command_patterns = ["^git"]

[plugins.plugins.config]
custom_key = "custom_value"
```

### Project configuration

**File**: `.klaudiush/config.toml` (project root)

Project config merges with global config. Use for project-specific plugins.

```toml
[[plugins.plugins]]
name = "project-linter"
type = "exec"
path = "./scripts/validate.sh"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
file_patterns = ["*.go", "**/*.ts"]
```

### Configuration reference

**Plugin system** (`[plugins]`):

| Option            | Type     | Default                | Description                     |
|:------------------|:---------|:-----------------------|:--------------------------------|
| `enabled`         | bool     | false                  | Global enable/disable           |
| `directory`       | string   | `~/.klaudiush/plugins` | Default plugin directory        |
| `default_timeout` | duration | `5s`                   | Default timeout for all plugins |

**Plugin instance** (`[[plugins.plugins]]`):

| Option    | Type     | Default    | Description                            |
|:----------|:---------|:-----------|:---------------------------------------|
| `name`    | string   | (required) | Unique plugin identifier               |
| `type`    | string   | (required) | Plugin type: `"exec"`                  |
| `enabled` | bool     | true       | Per-plugin enable/disable              |
| `path`    | string   | (required) | Path to plugin executable              |
| `args`    | string[] | []         | Extra command-line arguments           |
| `timeout` | duration | inherited  | Per-plugin timeout (overrides default) |

## Predicate matching

Predicates control when plugins are invoked. All conditions must match (AND
logic). Omitting a predicate means "match all" for that dimension.

### Event types

```toml
[plugins.plugins.predicate]
event_types = ["PreToolUse"]  # Most common
```

Available: `PreToolUse`, `PostToolUse`, `Notification`. Empty matches all.

### Tool types

```toml
[plugins.plugins.predicate]
tool_types = ["Bash", "Write", "Edit"]
```

Available: `Bash`, `Write`, `Edit`, `Read`, `Grep`, `Glob`, `WebFetch`, `WebSearch`. Empty matches all.

### File patterns

```toml
[plugins.plugins.predicate]
file_patterns = ["*.go", "**/*.ts", "src/**/*.rs"]
```

Glob syntax. Only applies to file tools (Write, Edit, Read). Empty matches all.

### Command patterns

```toml
[plugins.plugins.predicate]
command_patterns = ["^git commit", "docker build"]
```

Regex syntax. Only applies to Bash tool. Empty matches all.

### Common predicate combinations

```toml
# Git commits only
[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
command_patterns = ["^git commit"]

# Go file writes
[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Write", "Edit"]
file_patterns = ["**/*.go"]

# Catch-all (matches everything)
[plugins.plugins.predicate]
```

## Examples

### Shell script with configuration

Plugins receive custom configuration via the `config` field in the request JSON.

```bash
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "--info" ]]; then
  echo '{"name":"pattern-blocker","version":"1.0.0","description":"Blocks commands matching patterns"}'
  exit 0
fi

read -r request
tool_name=$(echo "$request" | jq -r '.tool_name')
command=$(echo "$request" | jq -r '.command // empty')
patterns=$(echo "$request" | jq -r '.config.blocked_patterns // [] | .[]')

if [[ "$tool_name" == "Bash" ]]; then
  for pattern in $patterns; do
    if echo "$command" | grep -qE "$pattern"; then
      cat <<EOF
{"passed":false,"should_block":true,"message":"Command matches blocked pattern: $pattern","error_code":"BLOCKED_PATTERN","fix_hint":"Avoid using this command pattern"}
EOF
      exit 0
    fi
  done
fi

echo '{"passed":true,"should_block":false}'
```

```toml
[[plugins.plugins]]
name = "pattern-blocker"
type = "exec"
path = "~/.klaudiush/plugins/pattern-blocker.sh"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]

[plugins.plugins.config]
blocked_patterns = ["rm -rf", "dd if=", "mkfs"]
```

### Python plugin

```python
#!/usr/bin/env python3
import sys, json

def main():
    if len(sys.argv) > 1 and sys.argv[1] == "--info":
        print(json.dumps({"name": "python-validator", "version": "1.0.0",
                          "description": "Example Python validator"}))
        return

    request = json.load(sys.stdin)
    tool_name = request.get("tool_name", "")
    file_path = request.get("file_path", "")

    if tool_name in ("Write", "Edit") and file_path.endswith(".exe"):
        print(json.dumps({"passed": False, "should_block": True,
                          "message": "Binary files (.exe) are not allowed",
                          "error_code": "NO_BINARIES",
                          "fix_hint": "Use source code instead"}))
        return

    print(json.dumps({"passed": True, "should_block": False}))

if __name__ == "__main__":
    main()
```

### Passing extra arguments

```toml
[[plugins.plugins]]
name = "my-plugin"
type = "exec"
path = "~/.klaudiush/plugins/my-plugin.sh"
args = ["--strict", "--env=production"]
```

### Working example

A working example lives in `examples/plugins/exec-shell/`:

- `file_validator.sh` -- Shell script that validates file operations
- `README.md` -- Installation, configuration, and testing instructions

## Best practices

### Performance

- Return early for non-matching contexts instead of doing unnecessary work.
- Use narrow predicates so the plugin is only spawned when relevant.
- Keep startup fast. Each invocation is a fresh process; avoid heavy initialization.
- Set reasonable timeouts: 1-5s for fast checks, 10-30s for external APIs, never exceed 60s.

### Error handling

- Always return valid JSON and exit 0. Non-zero exits cause fail-open behavior.
- Use error codes (`error_code`) for programmatic handling and documentation links.
- Distinguish blocking vs warning: use `should_block: true` for critical issues, `false` for advisories.

### Configuration

- Document your config schema with comments in the TOML example.
- Provide sensible defaults: handle missing config values gracefully.
- Validate config early: return a clear error if required config is missing.

### Security

- Validate inputs: do not trust `command` or `file_path` values blindly.
- Limit resource usage: respect timeouts, avoid unbounded allocations.
- Handle secrets carefully: never log credentials or include them in responses.

### Testing

```bash
# Test metadata
./my-plugin.sh --info | jq .

# Test validation (readable output)
echo '{"tool_name":"Bash","command":"sudo rm -rf /"}' | ./my-plugin.sh | jq .

# Test with klaudiush debug logging
klaudiush --debug
grep "Loading plugin" ~/.claude/hooks/dispatcher.log
```

Test edge cases: empty fields, missing config keys, large inputs, special JSON characters.

## Troubleshooting

### Plugin not loading

1. Check `[plugins] enabled = true` in config.
2. Check plugin instance `enabled` is not `false`.
3. Verify the path is correct and the file is executable (`chmod +x`).
4. Check predicates match your context (`klaudiush --debug`).

### Plugin timeout

1. Increase timeout: `timeout = "30s"` in the plugin config.
2. Optimize the plugin (cache operations, return early).
3. Test directly: `echo '{"tool_name":"Bash"}' | timeout 5s ./my-plugin.sh`

### Wrong validation result

1. Test the plugin directly with `jq` to inspect the JSON output.
2. Check for **stderr pollution** -- only write the JSON response to stdout. Diagnostics go to stderr (`>&2`).
3. Validate JSON: `./my-plugin.sh --info | jq . || echo "Invalid JSON"`

### Plugin not found

1. Use correct path format: absolute (`/home/...`), home shorthand (`~/.klaudiush/...`), or project-relative (`./scripts/...`).
2. Verify the file exists and has executable permissions.

## Resources

- API reference: `pkg/plugin/api.go`
- Integration tests: `internal/plugin/integration_test.go`
- [Issues](https://github.com/smykla-skalski/klaudiush/issues)
- [Discussions](https://github.com/smykla-skalski/klaudiush/discussions)
