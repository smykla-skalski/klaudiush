# PLUG004: Insecure gRPC plugin connection

## Error

A gRPC plugin is configured to connect without TLS encryption.

## Why this matters

Unencrypted gRPC connections expose validation data in transit. An attacker on the network could intercept or modify plugin communication, potentially bypassing security checks.

## How to fix

Use TLS for gRPC plugin connections:

```toml
[plugins.my-grpc-plugin]
type = "grpc"
address = "localhost:50051"
tls = true
ca_cert = "/path/to/ca.pem"
```

For local development, if you must use insecure connections:

```toml
[plugins.my-grpc-plugin]
type = "grpc"
address = "localhost:50051"
insecure = true   # Only for local development
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[PLUG004] Insecure gRPC plugin connection detected`

**systemMessage** (shown to user):
Formatted error with reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [PLUG001](PLUG001.md) - path traversal in plugin path
