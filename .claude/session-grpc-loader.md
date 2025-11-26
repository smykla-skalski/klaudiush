# Session: gRPC Loader Implementation

Implementation details for gRPC plugin loader with connection pooling.

## Overview

The gRPC loader enables external validator plugins to run as standalone gRPC services, offering persistent connections and cross-language support. This complements the existing Go plugin (.so) and exec plugin (subprocess) implementations.

## Architecture

### Protocol Definition

**File**: `api/plugin/v1/plugin.proto`

```protobuf
service ValidatorPlugin {
  rpc Info(InfoRequest) returns (InfoResponse);
  rpc Validate(ValidateRequest) returns (ValidateResponse);
}
```

- Uses protobuf v3 syntax
- Mirrors the internal `pkg/plugin.Plugin` interface
- Config field uses `map<string, string>` for wire format compatibility

### Code Generation

- **Tool**: buf 1.61.0 (latest as of Nov 2024)
- **Config Files**:
  - `buf.yaml` - Module and linting configuration
  - `buf.gen.yaml` - Code generation settings
- **Generated Files**:
  - `api/plugin/v1/plugin.pb.go` - Protocol buffer types
  - `api/plugin/v1/plugin_grpc.pb.go` - gRPC client/server stubs

### Implementation

**File**: `internal/plugin/grpc_loader.go`

#### Connection Pooling

```go
type GRPCLoader struct {
    mu          sync.RWMutex
    connections map[string]*grpc.ClientConn
    dialTimeout time.Duration
}
```

- **Pooling Strategy**: By address (e.g., "localhost:50051")
- **Thread Safety**: RWMutex with double-check locking pattern
- **Reuse**: Multiple plugin instances sharing same address use same connection
- **Cleanup**: All connections closed on `Close()`

#### Connection Lifecycle

1. **Load()** called with plugin config
2. **Fast path**: Check existing connection (read lock)
3. **Slow path**: Create new connection (write lock with double-check)
4. **Connection**: Created lazily using `grpc.NewClient()` (non-blocking)
5. **Validation**: Info RPC called immediately to verify connection

#### Type Conversion

**grpcPluginAdapter** handles conversion between:

- Internal `plugin.ValidateRequest` → protobuf `pluginv1.ValidateRequest`
- Protobuf `pluginv1.ValidateResponse` → internal `plugin.ValidateResponse`

**Config Handling**:

- Internal: `map[string]any`
- Protobuf: `map[string]string`
- Conversion: JSON marshaling for non-string types

#### Error Handling

Static errors for consistent wrapping:

```go
var (
    ErrGRPCAddressRequired = errors.New("address is required for gRPC plugins")
    ErrGRPCInfoFailed      = errors.New("failed to fetch plugin info via gRPC")
)
```

## Integration

### Registry

**File**: `internal/plugin/registry.go`

```go
loaders: map[config.PluginType]Loader{
    config.PluginTypeGo:   NewGoLoader(),
    config.PluginTypeGRPC: NewGRPCLoader(),  // Added
    config.PluginTypeExec: NewExecLoader(runner),
}
```

### Categorization

gRPC plugins categorized as `CategoryIO` (alongside exec plugins):

```go
if cfg.Type == config.PluginTypeExec || cfg.Type == config.PluginTypeGRPC {
    category = validator.CategoryIO
}
```

**Rationale**: Network I/O operations are I/O-bound, not CPU-bound.

## Configuration

```toml
[[plugins.plugins]]
name = "security-scanner"
type = "grpc"
address = "localhost:50051"
timeout = "5s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Write", "Edit"]
```

## Testing

**File**: `internal/plugin/grpc_loader_test.go`

### Mock gRPC Server

```go
type mockGRPCServer struct {
    pluginv1.UnimplementedValidatorPluginServer
    // ... control fields
}
```

- Implements `ValidatorPluginServer` interface
- Configurable responses and delays
- Runs on random port (`:0`)
- Clean shutdown after each test

### Test Coverage

15 tests covering:

- **Happy Path**: Plugin loading, validation success
- **Connection Pooling**: Reuse verification, concurrent access
- **Errors**: Missing address, connection failures, RPC failures
- **Timeouts**: Context cancellation, request timeouts
- **Config**: Type conversion (string, int, bool, map)

## Dependencies

**Added to go.mod**:

- `google.golang.org/grpc` v1.77.0
- `google.golang.org/genproto/googleapis/rpc` (transitive)
- `google.golang.org/protobuf` (transitive)

**Tool Dependencies**:

- buf 1.61.0 (added to `.mise.toml`)

## Design Decisions

### Why Connection Pooling?

gRPC connections are expensive to create (TCP handshake, TLS negotiation, HTTP/2 setup). Pooling provides:

- **Performance**: Avoid connection overhead for repeated validations
- **Resource Efficiency**: Reduce file descriptors and network connections
- **Scalability**: Support multiple plugin instances on same server

### Why grpc.NewClient vs DialContext?

**Old API** (`grpc.DialContext`):

- Blocking connection establishment
- Required `grpc.WithBlock()` option
- Deprecated in gRPC-Go 1.x

**New API** (`grpc.NewClient`):

- Lazy/non-blocking connection
- No blocking options needed
- Future-proof (recommended API)

**Trade-off**: Connection validation happens during first RPC (Info call), not during Load().

### Why JSON for Config Values?

Protobuf maps require homogeneous value types (`map<string, string>`). Internal config uses `map[string]any` for flexibility. JSON bridges this gap:

```go
// Internal: config["threshold"] = 42
// Wire:     config["threshold"] = "42"  (JSON: "42")
```

Alternative considered: protobuf `google.protobuf.Any` (rejected as overly complex).

## Performance Characteristics

### Connection Pooling Impact

**Without pooling** (naive implementation):

- 3 plugin instances × 100 validations = 300 connections
- ~50ms connection overhead each = ~15s total

**With pooling**:

- 1 connection shared across instances
- ~50ms one-time overhead
- ~0ms subsequent validations

**Savings**: 299 connections avoided, ~14.95s saved

### Lock Contention

RWMutex allows:

- **Multiple readers**: Concurrent `Load()` for existing connections
- **Single writer**: Exclusive access when creating new connections

**Hot path** (existing connection): Read lock only, minimal contention.

## Future Improvements

### Connection Health Checks

Current: Lazy validation (first RPC fails if server down)

Future: Proactive health checks via `grpc.health.v1.Health` service

### Connection Limits

Current: Unbounded pool (one connection per unique address)

Future: Configurable max connections with LRU eviction

### Retry Logic

Current: No automatic retries

Future: Configurable retry policy with exponential backoff

## Comparison: Go vs gRPC vs Exec Plugins

| Feature              | Go (.so)     | gRPC        | Exec           |
|:---------------------|:-------------|:------------|:---------------|
| **Performance**      | Fastest      | Fast        | Slowest        |
| **Language Support** | Go only      | Any         | Any            |
| **Process**          | In-process   | Separate    | Separate       |
| **Connection**       | Direct call  | Persistent  | Per-invocation |
| **Overhead**         | Minimal      | Network     | Process spawn  |
| **Category**         | CategoryCPU  | CategoryIO  | CategoryIO     |
| **Reload**           | Restart only | Hot-reload* | Per-invocation |

*Hot-reload: Server can be restarted without restarting klaudiush.

## Example Plugin Server

Minimal gRPC plugin in Python:

```python
import grpc
from concurrent import futures
import plugin_pb2
import plugin_pb2_grpc

class MyValidator(plugin_pb2_grpc.ValidatorPluginServicer):
    def Info(self, request, context):
        return plugin_pb2.InfoResponse(
            name="my-validator",
            version="1.0.0",
            description="Example Python validator"
        )

    def Validate(self, request, context):
        # Validation logic here
        if request.command.startswith("rm -rf"):
            return plugin_pb2.ValidateResponse(
                passed=False,
                should_block=True,
                message="Dangerous command blocked",
                error_code="DANGEROUS_001"
            )
        return plugin_pb2.ValidateResponse(passed=True)

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    plugin_pb2_grpc.add_ValidatorPluginServicer_to_server(
        MyValidator(), server
    )
    server.add_insecure_port('[::]:50051')
    server.start()
    server.wait_for_termination()

if __name__ == '__main__':
    serve()
```

## Related Session Files

- `session-plugin-system.md` - Overall plugin system architecture
- `session-parallel-execution.md` - CategoryIO execution pools