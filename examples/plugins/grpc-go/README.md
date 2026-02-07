# gRPC Plugin Example: Git Validator (Go)

This example demonstrates how to create a klaudiush gRPC plugin server using Go.

## Overview

The plugin validates git push operations and:

- **Blocks** unsafe force pushes (without `--force-with-lease`) in strict mode
- **Warns** about force pushes in non-strict mode
- **Blocks** direct pushes to protected branches (main, master)

## Features

- Persistent gRPC connection (no subprocess overhead)
- Hot-reload capable (restart server without restarting klaudiush)
- Cross-language compatible (server can be in any language)
- Configuration support via TOML

## Building

### Prerequisites

- Go 1.19 or later
- Access to klaudiush repository for protobuf definitions

### Build Command

```bash
go build -o git-validator main.go
```

## Running

### Start Server

```bash
# Default port (50051)
./git-validator

# Custom port
./git-validator -port 50052
```

### Run in Background

```bash
nohup ./git-validator > git-validator.log 2>&1 &
```

### Check if Running

```bash
lsof -i :50051
```

## Configuration

Add to `~/.klaudiush/config.toml`:

```toml
[[plugins.plugins]]
name = "git-validator"
type = "grpc"
enabled = true
address = "localhost:50051"
timeout = "5s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Bash"]
command_patterns = ["^git push"]

[plugins.plugins.config]
strict_mode = "true"                    # Block unsafe force pushes
protected_branches = "main,master,prod" # Comma-separated list
```

## Testing

### Test with grpcurl

Install grpcurl:

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

Test Info RPC:

```bash
grpcurl -plaintext localhost:50051 plugin.v1.ValidatorPlugin/Info
```

Expected output:

```json
{
  "name": "git-validator",
  "version": "1.0.0",
  "description": "Validates git operations (prevents unsafe force pushes)",
  "author": "klaudiush",
  "url": "https://github.com/smykla-labs/klaudiush/examples/plugins/grpc-go"
}
```

Test Validate RPC (force push):

```bash
grpcurl -plaintext \
  -d '{
    "tool_name": "Bash",
    "command": "git push --force origin main",
    "config": {"strict_mode": "true"}
  }' \
  localhost:50051 \
  plugin.v1.ValidatorPlugin/Validate
```

Expected output:

```json
{
  "passed": false,
  "shouldBlock": true,
  "message": "Force push detected without --force-with-lease",
  "errorCode": "UNSAFE_FORCE_PUSH",
  "fixHint": "Use 'git push --force-with-lease' instead of '--force'",
  "docLink": "https://git-scm.com/docs/git-push#Documentation/git-push.txt---force-with-leaseltrefnamegtltexpectgt",
  "details": {
    "command": "git push --force origin main",
    "mode": "strict"
  }
}
```

Test Validate RPC (safe push):

```bash
grpcurl -plaintext \
  -d '{
    "tool_name": "Bash",
    "command": "git push origin feature-branch"
  }' \
  localhost:50051 \
  plugin.v1.ValidatorPlugin/Validate
```

Expected output:

```json
{
  "passed": true,
  "shouldBlock": false
}
```

### Integration Test with klaudiush

Start server and test with klaudiush:

```bash
./git-validator &
```

Use klaudiush with plugin configured. Try these commands:

- `git push --force origin main` (should be blocked)
- `git push --force-with-lease origin main` (should pass)

## Customization

### Disable Strict Mode (Warn Only)

```toml
[plugins.plugins.config]
strict_mode = "false"
```

### Add Custom Protected Branches

```toml
[plugins.plugins.config]
protected_branches = "main,master,production,staging"
```

### Modify Validation Logic

Edit `main.go` to add custom rules:

```go
// Block pushes to specific remotes
if strings.Contains(command, "production-remote") {
    return &pluginv1.ValidateResponse{
        Passed:      false,
        ShouldBlock: true,
        Message:     "Direct push to production remote not allowed",
    }, nil
}
```

## Protocol

### Service Definition

```protobuf
service ValidatorPlugin {
  rpc Info(InfoRequest) returns (InfoResponse);
  rpc Validate(ValidateRequest) returns (ValidateResponse);
}
```

### Info RPC

Request: `{}`

Response:

```json
{
  "name": "git-validator",
  "version": "1.0.0",
  "description": "...",
  "author": "...",
  "url": "..."
}
```

### Validate RPC

Request:

```json
{
  "event_type": "PreToolUse",
  "tool_name": "Bash",
  "command": "git push --force origin main",
  "config": {
    "strict_mode": "true",
    "protected_branches": "main,master"
  }
}
```

Response (Pass):

```json
{
  "passed": true,
  "should_block": false
}
```

Response (Fail):

```json
{
  "passed": false,
  "should_block": true,
  "message": "Error message",
  "error_code": "ERROR_CODE",
  "fix_hint": "Suggested fix",
  "doc_link": "https://docs.klaudiu.sh",
  "details": {
    "key": "value"
  }
}
```

## Connection Pooling

klaudiush automatically pools gRPC connections by address:

- Multiple plugin instances with same address share one connection
- Connections persist across validations (no reconnection overhead)
- Automatic cleanup when klaudiush exits

## Advantages over Exec Plugins

- **Performance**: No process spawn overhead (~10-100ms saved per invocation)
- **Hot-reload**: Restart server without restarting klaudiush
- **Language**: Server can be in Python, Node.js, Rust, etc.
- **Persistent**: Connection pooling reduces latency

## Next Steps

- See [Go Plugin Example](../go-plugin/) for maximum performance
- See [Exec Plugin Example](../exec-shell/) for simplicity
- Read [Plugin Development Guide](../../../docs/PLUGIN_GUIDE.md) for more details
