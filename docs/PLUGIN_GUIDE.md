# Plugin Development Guide

Complete guide for developing klaudiush validator plugins.

## Table of Contents

- [Overview](#overview)
- [Plugin Types Comparison](#plugin-types-comparison)
- [Quick Start](#quick-start)
- [Go Plugins](#go-plugins)
- [Exec Plugins](#exec-plugins)
- [gRPC Plugins](#grpc-plugins)
- [Plugin Configuration](#plugin-configuration)
- [Predicate Matching](#predicate-matching)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

klaudiush plugins extend the validation system with custom logic. Plugins can:

- Enforce organization-specific rules
- Integrate with external services (security scanners, issue trackers)
- Implement domain-specific validations
- Block or warn about specific patterns

### Plugin Types Comparison

| Feature              | Go (.so)     | gRPC       | Exec           |
|:---------------------|:-------------|:-----------|:---------------|
| **Performance**      | Fastest      | Fast       | Slowest        |
| **Language Support** | Go only      | Any        | Any            |
| **Process**          | In-process   | Separate   | Separate       |
| **Connection**       | Direct call  | Persistent | Per-invocation |
| **Overhead**         | Minimal      | Network    | Process spawn  |
| **Reload**           | Restart only | Hot-reload | Per-invocation |

**Recommendation**:

- **Go plugins**: Maximum performance, Go-only
- **gRPC plugins**: Balanced performance, any language, persistent
- **Exec plugins**: Maximum compatibility, any language, simple

## Quick Start

### 1. Choose Plugin Type

Start with **exec plugins** for simplicity, migrate to gRPC or Go for performance.

### 2. Implement Plugin Interface

All plugins must implement:

```go
type Plugin interface {
    Info() Info
    Validate(req *ValidateRequest) *ValidateResponse
}
```

### 3. Configure Plugin

Add to `~/.klaudiush/config.toml` or `.klaudiush/config.toml`:

```toml
[[plugins.plugins]]
name = "my-plugin"
type = "exec"  # or "go" or "grpc"
path = "/path/to/plugin"
enabled = true

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
```

### 4. Test Plugin

```bash
# Enable debug logging
klaudiush --debug
```

Trigger validation (plugin invoked based on predicates)

## Go Plugins

Native Go plugins compiled as shared libraries.

### Plugin Requirements

- Go 1.19+ (same version as klaudiush)
- Same OS and architecture as klaudiush
- No `main` package, exported `Plugin` variable

### Example: Simple Validator

**File**: `my-plugin.go`

```go
package main

import "github.com/smykla-labs/klaudiush/pkg/plugin"

// MyPlugin implements the plugin.Plugin interface.
type MyPlugin struct{}

// Info returns plugin metadata.
func (p *MyPlugin) Info() plugin.Info {
    return plugin.Info{
        Name:        "my-plugin",
        Version:     "1.0.0",
        Description: "Blocks dangerous commands",
        Author:      "Your Name",
        URL:         "https://github.com/yourorg/my-plugin",
    }
}

// Validate performs the validation logic.
func (p *MyPlugin) Validate(req *plugin.ValidateRequest) *plugin.ValidateResponse {
    // Only validate bash commands
    if req.ToolName != "Bash" {
        return plugin.PassResponse()
    }

    // Block rm -rf commands
    if strings.Contains(req.Command, "rm -rf /") {
        return plugin.FailResponse("Dangerous command blocked: rm -rf /")
    }

    return plugin.PassResponse()
}

// Plugin is the exported symbol that klaudiush will load.
var Plugin MyPlugin
```

### Build

```bash
# Build shared library
go build -buildmode=plugin -o my-plugin.so my-plugin.go

# Install to plugin directory
mkdir -p ~/.klaudiush/plugins
cp my-plugin.so ~/.klaudiush/plugins/
```

### Go Plugin Configuration

```toml
[[plugins.plugins]]
name = "my-plugin"
type = "go"
path = "~/.klaudiush/plugins/my-plugin.so"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
```

### Advanced Example: Configuration Support

```go
package main

import (
    "regexp"
    "strings"

    "github.com/smykla-labs/klaudiush/pkg/plugin"
)

type ConfigurablePlugin struct{}

func (p *ConfigurablePlugin) Info() plugin.Info {
    return plugin.Info{
        Name:        "pattern-blocker",
        Version:     "1.0.0",
        Description: "Blocks commands matching configured patterns",
    }
}

func (p *ConfigurablePlugin) Validate(req *plugin.ValidateRequest) *plugin.ValidateResponse {
    if req.ToolName != "Bash" {
        return plugin.PassResponse()
    }

    // Read patterns from config
    patterns, ok := req.Config["blocked_patterns"].([]interface{})
    if !ok {
        return plugin.PassResponse()
    }

    // Check each pattern
    for _, p := range patterns {
        pattern := p.(string)
        matched, _ := regexp.MatchString(pattern, req.Command)
        if matched {
            return plugin.FailWithCode(
                "BLOCKED_PATTERN",
                "Command matches blocked pattern: "+pattern,
                "Avoid using this command pattern",
                "https://docs.klaudiu.sh/blocked-patterns",
            )
        }
    }

    return plugin.PassResponse()
}

var Plugin ConfigurablePlugin
```

**Configuration**:

```toml
[[plugins.plugins]]
name = "pattern-blocker"
type = "go"
path = "~/.klaudiush/plugins/pattern-blocker.so"

[plugins.plugins.config]
blocked_patterns = ["rm -rf", "dd if=", "mkfs"]

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
```

### Limitations

- **No hot-reload**: Requires klaudiush restart
- **Version compatibility**: Must match klaudiush Go version
- **Platform-specific**: Recompile for each OS/architecture
- **No unloading**: Memory persists until restart

## Exec Plugins

Subprocess-based plugins using JSON over stdin/stdout.

### Exec Plugin Requirements

- Executable file (any language)
- JSON parsing support
- Handles `--info` flag for metadata

### Example: Shell Script

**File**: `my-plugin.sh`

```bash
#!/usr/bin/env bash

set -euo pipefail

# Handle --info flag
if [[ "${1:-}" == "--info" ]]; then
  cat <<EOF
{
  "name": "shell-validator",
  "version": "1.0.0",
  "description": "Example shell-based validator"
}
EOF
  exit 0
fi

# Read request from stdin
read -r request

# Parse JSON (using jq for simplicity)
tool_name=$(echo "$request" | jq -r '.tool_name')
command=$(echo "$request" | jq -r '.command // empty')

# Validation logic
if [[ "$tool_name" == "Bash" ]] && [[ "$command" == *"sudo"* ]]; then
  cat <<EOF
{
  "passed": false,
  "should_block": true,
  "message": "sudo commands are not allowed",
  "error_code": "NO_SUDO",
  "fix_hint": "Run without sudo or request elevated permissions"
}
EOF
  exit 0
fi

# Pass response
cat <<EOF
{
  "passed": true,
  "should_block": false
}
EOF
```

**Make executable**:

```bash
chmod +x my-plugin.sh
```

### Example: Python

**File**: `my_plugin.py`

```python
#!/usr/bin/env python3

import sys
import json

def get_info():
    """Return plugin metadata."""
    return {
        "name": "python-validator",
        "version": "1.0.0",
        "description": "Example Python-based validator",
        "author": "Your Name",
    }

def validate(request):
    """Perform validation."""
    tool_name = request.get("tool_name", "")
    file_path = request.get("file_path", "")

    # Example: Check file extension
    if tool_name in ["Write", "Edit"]:
        if file_path.endswith(".exe"):
            return {
                "passed": False,
                "should_block": True,
                "message": "Binary files (.exe) are not allowed",
                "error_code": "NO_BINARIES",
                "fix_hint": "Use source code instead of compiled binaries",
            }

    # Pass by default
    return {
        "passed": True,
        "should_block": False,
    }

def main():
    # Handle --info flag
    if len(sys.argv) > 1 and sys.argv[1] == "--info":
        print(json.dumps(get_info()))
        return 0

    # Read request from stdin
    request = json.load(sys.stdin)

    # Validate and return response
    response = validate(request)
    print(json.dumps(response))

    return 0

if __name__ == "__main__":
    sys.exit(main())
```

**Make executable**:

```bash
chmod +x my_plugin.py
```

### Exec Plugin Configuration

```toml
[[plugins.plugins]]
name = "my-plugin"
type = "exec"
path = "/path/to/my-plugin.sh"  # or my_plugin.py
timeout = "5s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash", "Write", "Edit"]
```

### Protocol

**Info Request** (via flag):

```bash
./my-plugin --info
```

**Info Response** (JSON to stdout):

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Plugin description"
}
```

**Validate Request** (JSON to stdin):

```json
{
  "event_type": "PreToolUse",
  "tool_name": "Bash",
  "command": "git commit -m \"message\"",
  "config": {
    "max_length": "100"
  }
}
```

**Validate Response** (JSON to stdout):

```json
{
  "passed": false,
  "should_block": true,
  "message": "Validation failed",
  "error_code": "EXAMPLE_001",
  "fix_hint": "Try this instead...",
  "doc_link": "https://docs.klaudiu.sh/errors/EXAMPLE_001"
}
```

## gRPC Plugins

Persistent server-based plugins using Protocol Buffers.

### gRPC Plugin Requirements

- gRPC server implementation
- Protobuf definitions from klaudiush
- Network accessibility (localhost or remote)

### Protocol Definition

**File**: `api/plugin/v1/plugin.proto` (from klaudiush repository)

```protobuf
syntax = "proto3";
package plugin.v1;

service ValidatorPlugin {
  rpc Info(InfoRequest) returns (InfoResponse);
  rpc Validate(ValidateRequest) returns (ValidateResponse);
}

message InfoRequest {}

message InfoResponse {
  string name = 1;
  string version = 2;
  string description = 3;
  string author = 4;
  string url = 5;
}

message ValidateRequest {
  string event_type = 1;
  string tool_name = 2;
  string command = 3;
  string file_path = 4;
  string content = 5;
  string old_string = 6;
  string new_string = 7;
  string pattern = 8;
  map<string, string> config = 9;
}

message ValidateResponse {
  bool passed = 1;
  bool should_block = 2;
  string message = 3;
  string error_code = 4;
  string fix_hint = 5;
  string doc_link = 6;
  map<string, string> details = 7;
}
```

### Example: Go Server

**File**: `main.go`

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net"
    "strings"

    "google.golang.org/grpc"
    pluginv1 "github.com/smykla-labs/klaudiush/api/plugin/v1"
)

type server struct {
    pluginv1.UnimplementedValidatorPluginServer
}

func (s *server) Info(ctx context.Context, req *pluginv1.InfoRequest) (*pluginv1.InfoResponse, error) {
    return &pluginv1.InfoResponse{
        Name:        "grpc-validator",
        Version:     "1.0.0",
        Description: "Example gRPC validator",
        Author:      "Your Name",
        Url:         "https://github.com/yourorg/grpc-validator",
    }, nil
}

func (s *server) Validate(ctx context.Context, req *pluginv1.ValidateRequest) (*pluginv1.ValidateResponse, error) {
    // Example: Block force pushes
    if req.ToolName == "Bash" && strings.Contains(req.Command, "git push --force") {
        return &pluginv1.ValidateResponse{
            Passed:      false,
            ShouldBlock: true,
            Message:     "Force push detected",
            ErrorCode:   "NO_FORCE_PUSH",
            FixHint:     "Use --force-with-lease instead",
            DocLink:     "https://docs.klaudiu.sh/git-best-practices",
        }, nil
    }

    return &pluginv1.ValidateResponse{
        Passed:      true,
        ShouldBlock: false,
    }, nil
}

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    s := grpc.NewServer()
    pluginv1.RegisterValidatorPluginServer(s, &server{})

    fmt.Println("gRPC server listening on :50051")
    if err := s.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}
```

### Example: Python Server

**File**: `server.py`

```python
from concurrent import futures
import grpc
import plugin_pb2
import plugin_pb2_grpc

class ValidatorPlugin(plugin_pb2_grpc.ValidatorPluginServicer):
    def Info(self, request, context):
        return plugin_pb2.InfoResponse(
            name="python-grpc-validator",
            version="1.0.0",
            description="Example Python gRPC validator",
            author="Your Name",
        )

    def Validate(self, request, context):
        # Example: Warn about TODO comments in production
        if request.tool_name in ["Write", "Edit"]:
            if "TODO" in request.content:
                return plugin_pb2.ValidateResponse(
                    passed=False,
                    should_block=False,  # Warn only
                    message="TODO comment found in code",
                    error_code="TODO_FOUND",
                    fix_hint="Resolve TODO before committing",
                )

        return plugin_pb2.ValidateResponse(
            passed=True,
            should_block=False,
        )

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    plugin_pb2_grpc.add_ValidatorPluginServicer_to_server(
        ValidatorPlugin(), server
    )
    server.add_insecure_port('[::]:50051')
    server.start()
    print("gRPC server listening on :50051")
    server.wait_for_termination()

if __name__ == '__main__':
    serve()
```

### gRPC Plugin Configuration

```toml
[[plugins.plugins]]
name = "grpc-validator"
type = "grpc"
address = "localhost:50051"
timeout = "5s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash", "Write", "Edit"]
```

### Starting the Server

```bash
# Go
go run main.go

# Python
python server.py

# Background (production)
nohup ./grpc-validator > /var/log/grpc-validator.log 2>&1 &
```

### Connection Pooling

klaudiush automatically pools connections by address. Multiple plugin instances sharing the same address reuse a single connection.

**Benefits**:

- Reduced connection overhead
- Lower resource usage
- Faster validation after initial connection

## Plugin Configuration

### Global Configuration

**File**: `~/.klaudiush/config.toml`

```toml
[plugins]
enabled = true
directory = "~/.klaudiush/plugins"
default_timeout = "5s"

[[plugins.plugins]]
name = "example"
type = "go"
enabled = true
path = "~/.klaudiush/plugins/example.so"
timeout = "10s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
command_patterns = ["^git"]

[plugins.plugins.config]
# Plugin-specific configuration
custom_key = "custom_value"
```

### Project Configuration

**File**: `.klaudiush/config.toml` (project root)

Project config merges with global config. Use for project-specific plugins.

```toml
[[plugins.plugins]]
name = "project-specific"
type = "exec"
path = "./scripts/validate.sh"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
file_patterns = ["*.go", "**/*.proto"]
```

### Configuration Options

| Option              | Type     | Default | Description                          |
|:--------------------|:---------|:--------|:-------------------------------------|
| `enabled`           | bool     | true    | Global enable/disable                |
| `directory`         | string   | -       | Default plugin directory             |
| `default_timeout`   | duration | 5s      | Default timeout for all plugins      |
| `plugins[].name`    | string   | -       | Unique plugin identifier (required)  |
| `plugins[].type`    | string   | -       | Plugin type: "go", "grpc", or "exec" |
| `plugins[].enabled` | bool     | true    | Per-plugin enable/disable            |
| `plugins[].path`    | string   | -       | Path to plugin file (go/exec)        |
| `plugins[].address` | string   | -       | Server address (grpc)                |
| `plugins[].timeout` | duration | 5s      | Per-plugin timeout                   |

## Predicate Matching

Predicates control when plugins are invoked. All specified predicates must match.

### Event Types

```toml
[plugins.plugins.predicate]
event_types = ["PreToolUse", "PostToolUse", "Notification"]
```

**Available**:

- `PreToolUse`: Before tool execution (most common)
- `PostToolUse`: After tool execution
- `Notification`: System notifications

Empty list matches all events

### Tool Types

```toml
[plugins.plugins.predicate]
tool_types = ["Bash", "Write", "Edit", "Grep", "Glob"]
```

**Available**:

- `Bash`: Shell commands
- `Write`: File creation
- `Edit`: File modification
- `Read`: File reading
- `Grep`: Content search
- `Glob`: Pattern search
- `WebFetch`: Web requests
- `WebSearch`: Web searches

Empty list matches all tools

### File Patterns

```toml
[plugins.plugins.predicate]
file_patterns = ["*.go", "**/*.proto", "src/**/*.rs"]
```

- Uses glob syntax (`*`, `**`, `?`)
- Only applies to file tools (Write, Edit, Read)
- Matches against `file_path` field
- Empty list matches all files

### Command Patterns

```toml
[plugins.plugins.predicate]
command_patterns = ["^git commit", "docker build", "terraform apply"]
```

- Uses regex syntax
- Only applies to Bash tool
- Matches against `command` field
- Empty list matches all commands

### Examples

**Match all git commits**:

```toml
[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
command_patterns = ["^git commit"]
```

**Match Go file writes**:

```toml
[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Write", "Edit"]
file_patterns = ["**/*.go"]
```

**Match all file operations**:

```toml
[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Write", "Edit"]
```

**Catch-all plugin**:

```toml
[plugins.plugins.predicate]
```

## Best Practices

### Performance

1. **Use appropriate plugin type**:
   - Go plugins: CPU-intensive validation
   - gRPC plugins: External service calls
   - Exec plugins: Simple scripts

2. **Minimize work in Validate()**:
   - Return early for non-matching contexts
   - Cache expensive operations
   - Use predicates to filter invocations

3. **Set reasonable timeouts**:
   - Fast validations: 1-5s
   - External API calls: 10-30s
   - Never exceed 60s

### Error Handling

1. **Always return a response**:

   ```go
   // Bad: panics
   panic("validation failed")

   // Good: returns error response
   return plugin.FailResponse("validation failed")
   ```

2. **Use error codes for programmatic handling**:

   ```go
   return plugin.FailWithCode(
       "CUSTOM_001",
       "Descriptive message",
       "Suggested fix",
       "https://docs.klaudiu.sh/errors/CUSTOM_001",
   )
   ```

3. **Distinguish blocking vs warning**:

   ```go
   // Block operation
   return plugin.FailResponse("Critical security issue")

   // Warn but allow
   return plugin.WarnResponse("Consider refactoring")
   ```

### Plugin Configuration Best Practices

1. **Document config schema**:

   ```toml
   [plugins.plugins.config]
   # Max line length (default: 100)
   max_line_length = 120

   # Require copyright headers (default: false)
   require_copyright = true
   ```

2. **Provide sensible defaults**:

   ```go
   maxLength := 100
   if val, ok := req.Config["max_line_length"].(float64); ok {
       maxLength = int(val)
   }
   ```

3. **Validate config early**:

   ```go
   patterns, ok := req.Config["patterns"].([]interface{})
   if !ok {
       return plugin.FailResponse("Invalid config: 'patterns' must be array")
   }
   ```

### Testing

1. **Test plugin independently**:

   ```bash
   # Test exec plugin
   echo '{"tool_name":"Bash","command":"test"}' | ./my-plugin.sh

   # Test gRPC plugin
   grpcurl -plaintext -d '{}' localhost:50051 plugin.v1.ValidatorPlugin/Info
   ```

2. **Test with klaudiush**:

   ```bash
   # Enable debug logging
   klaudiush --debug

   # Check plugin loading
   grep "Loading plugin" ~/.claude/hooks/dispatcher.log
   ```

3. **Unit test validation logic**:

   ```go
   func TestValidate(t *testing.T) {
       p := &MyPlugin{}
       resp := p.Validate(&plugin.ValidateRequest{
           ToolName: "Bash",
           Command:  "rm -rf /",
       })
       if resp.Passed {
           t.Error("Expected validation to fail")
       }
   }
   ```

### Security

1. **Validate inputs**:

   ```go
   // Check for malicious patterns
   if strings.Contains(req.Command, "../") {
       return plugin.FailResponse("Path traversal detected")
   }
   ```

2. **Limit resource usage**:
   - Respect timeout deadlines
   - Avoid unbounded memory allocation
   - Close resources properly

3. **Don't trust config blindly**:

   ```go
   // Validate config values
   if maxSize, ok := req.Config["max_size"].(float64); ok {
       if maxSize > 1000000 {
           return plugin.FailResponse("max_size too large")
       }
   }
   ```

## Troubleshooting

### Plugin Not Loading

**Symptom**: Plugin not invoked

**Check**:

1. Plugin enabled in config:

   ```toml
   [[plugins.plugins]]
   enabled = true  # Check this
   ```

2. Path is correct:

   ```bash
   ls -l ~/.klaudiush/plugins/my-plugin.so
   ```

3. Predicates match your context:

   ```bash
   # Enable debug logging
   klaudiush --debug
   tail -f ~/.claude/hooks/dispatcher.log
   ```

### Go Plugin Load Error

**Symptom**: `plugin.Open failed`

**Causes**:

1. **Version mismatch**:

   ```bash
   # Check Go version
   go version

   # Rebuild with matching version
   go build -buildmode=plugin -o my-plugin.so
   ```

2. **Architecture mismatch**:

   ```bash
   # Check architecture
   file my-plugin.so
   file $(which klaudiush)

   # Must match (e.g., both "x86_64")
   ```

3. **Dependency conflicts**:
   - Ensure shared dependencies use same versions
   - Use `go.mod` for reproducible builds

### Exec Plugin Timeout

**Symptom**: `plugin execution timed out`

**Solutions**:

1. **Increase timeout**:

   ```toml
   [[plugins.plugins]]
   timeout = "30s"  # Increase from default 5s
   ```

2. **Optimize plugin**:
   - Cache expensive operations
   - Use background workers
   - Return early when possible

3. **Check for hangs**:

   ```bash
   # Test plugin directly
   echo '{"tool_name":"Bash"}' | timeout 5s ./my-plugin.sh
   ```

### gRPC Connection Failed

**Symptom**: `failed to connect to gRPC plugin`

**Check**:

1. Server is running:

   ```bash
   lsof -i :50051
   ```

2. Address is correct:

   ```toml
   [[plugins.plugins]]
   address = "localhost:50051"  # Check port
   ```

3. Firewall allows connection:

   ```bash
   telnet localhost 50051
   ```

### Plugin Returns Wrong Data

**Symptom**: Unexpected validation behavior

**Debug**:

1. **Enable debug logging**:

   ```bash
   klaudiush --debug
   ```

2. **Test plugin directly**:

   ```bash
   # Exec plugin
   echo '{"tool_name":"Bash","command":"test"}' | ./my-plugin.sh | jq

   # gRPC plugin
   grpcurl -plaintext \
     -d '{"tool_name":"Bash","command":"test"}' \
     localhost:50051 \
     plugin.v1.ValidatorPlugin/Validate
   ```

3. **Add logging to plugin**:

   ```go
   log.Printf("Validating: tool=%s, command=%s", req.ToolName, req.Command)
   ```

### Memory Leak

**Symptom**: klaudiush memory usage grows over time

**Causes**:

1. **Go plugin limitation**: Plugins never unload
   - **Solution**: Use exec or gRPC plugins instead

2. **Plugin resource leak**: Not closing resources
   - **Solution**: Implement proper cleanup

3. **gRPC connection leak**: Not closing clients
   - **Solution**: klaudiush handles this, report bug if seen

## Examples Repository

Find complete working examples at:

```text
examples/plugins/
├── go-plugin/          # Go plugin with build script
├── exec-shell/         # Shell script exec plugin
├── exec-python/        # Python exec plugin
└── grpc-go/            # Go gRPC server
```

Each example includes:

- Complete source code
- Build/run instructions
- Configuration examples
- Tests

## Resources

- **API Reference**: `pkg/plugin/api.go`
- **Protocol Definition**: `api/plugin/v1/plugin.proto`
- **Session Notes**: `.claude/session-plugin-system.md`, `.claude/session-grpc-loader.md`
- **Integration Tests**: `internal/plugin/integration_test.go`

## Support

- **Issues**: <https://github.com/smykla-labs/klaudiush/issues>
- **Discussions**: <https://github.com/smykla-labs/klaudiush/discussions>
