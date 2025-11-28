# Plugin Security Guide

Security documentation for klaudiush plugin system.

## Overview

klaudiush implements defense-in-depth security for the plugin system,
protecting against common attack vectors while maintaining usability.

## Plugin Directory Restrictions

Plugins can only be loaded from approved directories:

- **Global**: `~/.klaudiush/plugins/`
- **Project**: `.klaudiush/plugins/` (relative to project root)

**Why**: Prevents loading arbitrary code from untrusted locations. Even if an
attacker modifies the config file, they cannot load plugins from outside these
directories.

### Installation

```bash
# Create plugin directories
mkdir -p ~/.klaudiush/plugins
mkdir -p .klaudiush/plugins

# Install plugins with secure permissions (500 = read+execute for owner)
cp my-plugin.so ~/.klaudiush/plugins/
chmod 500 ~/.klaudiush/plugins/my-plugin.so
```

## Path Validation

Plugin paths are validated for:

- **Path traversal**: `../` patterns are rejected
- **Symlink resolution**: Symlinks are resolved to real paths
- **Extension validation**: Go plugins must end with `.so`
- **Dangerous characters**: Shell metacharacters rejected (defense-in-depth)

## gRPC TLS Configuration

By default, gRPC plugins require TLS for remote connections:

- **localhost**: Insecure allowed (backward compatible)
- **Remote**: TLS required or explicit bypass

### Basic TLS

```toml
[[plugins.plugins]]
name = "secure-plugin"
type = "grpc"
address = "plugin.example.com:50051"

[plugins.plugins.tls]
enabled = true
ca_file = "/etc/ssl/certs/ca-bundle.crt"
```

### mTLS (Mutual TLS)

For client certificate authentication:

```toml
[[plugins.plugins]]
name = "mtls-plugin"
type = "grpc"
address = "secure.example.com:50051"

[plugins.plugins.tls]
enabled = true
ca_file = "/path/to/ca.pem"
cert_file = "/path/to/client.pem"
key_file = "/path/to/client.key"
```

### Self-Signed Certificates (Development Only)

```toml
[[plugins.plugins]]
name = "dev-plugin"
type = "grpc"
address = "dev.local:50051"

[plugins.plugins.tls]
enabled = true
ca_file = "/path/to/self-signed-ca.pem"
# WARNING: Only use for development
insecure_skip_verify = true
```

### Insecure Remote Connections

**⚠️ SECURITY RISK**: Only use in trusted networks or for testing.

```toml
[[plugins.plugins]]
name = "internal-plugin"
type = "grpc"
address = "internal.corp:50051"

[plugins.plugins.tls]
enabled = false
# Explicitly acknowledge the security risk
allow_insecure_remote = true
```

This logs a warning:

```text
WARNING: insecure connection to remote plugin address=internal.corp:50051
```

## TLS Configuration Options

| Option                  | Type   | Default | Description            |
|:------------------------|:-------|:--------|:-----------------------|
| `enabled`               | bool   | auto    | TLS mode control       |
| `ca_file`               | string | -       | CA certificate path    |
| `cert_file`             | string | -       | Client cert (mTLS)     |
| `key_file`              | string | -       | Client key (mTLS)      |
| `insecure_skip_verify`  | bool   | false   | Skip server cert check |
| `allow_insecure_remote` | bool   | false   | Allow insecure remote  |

## Security Error Codes

| Code    | Description                          |
|:--------|:-------------------------------------|
| PLUG001 | Path traversal detected              |
| PLUG002 | Plugin path not in allowed directory |
| PLUG003 | Invalid plugin file extension        |
| PLUG004 | Insecure connection to remote host   |
| PLUG005 | Dangerous characters in plugin path  |

## Best Practices

1. **Use TLS for remote plugins**: Always enable TLS for non-localhost
2. **Verify certificates**: Avoid `insecure_skip_verify` in production
3. **Use mTLS**: Client certificates add authentication layer
4. **Keep plugins in allowed directories**: Avoid symlinks from untrusted paths
5. **Set file permissions**: Use `chmod 600` for plugin files
6. **Audit plugin sources**: Review plugin code before installation

## See Also

- [Plugin Development Guide](PLUGIN_GUIDE.md)
- [Session notes](.claude/session-grpc-loader.md)
