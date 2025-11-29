# Exception Workflow Guide

Allow Claude to bypass validation blocks with explicit acknowledgment and audit trail.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Token Format](#token-format)
- [Policy Configuration](#policy-configuration)
- [Rate Limiting](#rate-limiting)
- [Audit Logging](#audit-logging)
- [CLI Commands](#cli-commands)
- [Integration with Rules](#integration-with-rules)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The exception workflow allows Claude Code to bypass specific validation blocks when:

1. An exception policy exists for the error code
2. Claude includes an acknowledgment token in the command
3. The token includes a valid justification (if required)
4. Rate limits have not been exceeded

### Key Features

| Feature        | Description                                  |
|:---------------|:---------------------------------------------|
| Token-Based    | Explicit acknowledgment embedded in commands |
| Policy Control | Per-error-code configuration                 |
| Rate Limiting  | Prevent abuse with hourly/daily limits       |
| Audit Trail    | JSONL log of all exception attempts          |
| Configurable   | Justification requirements per policy        |

### How It Works

```text
1. Claude runs: git push origin main  # EXC:GIT019:Emergency+hotfix
2. klaudiush detects validation failure (GIT019)
3. klaudiush finds exception token in command
4. Policy check: Is GIT019 exception allowed?
5. Rate limit check: Within limits?
6. If allowed: Block → Warning, command proceeds
7. Audit entry logged
```

## Quick Start

### 1. Enable Exceptions

Create `.klaudiush/config.toml`:

```toml
[exceptions]
enabled = true

# Define a policy for GIT019 (direct push to protected branch)
[exceptions.policies.GIT019]
enabled = true
allow_exception = true
require_reason = true
min_reason_length = 10
description = "Exception for pushing to protected branches"
```

### 2. Bypass a Block

When Claude encounters a block, it can include an exception token:

```bash
# Shell comment format
git push origin main  # EXC:GIT019:Emergency+hotfix+for+production

# Environment variable format
KLAUDIUSH_ACK="EXC:GIT019:Emergency+hotfix" git push origin main
```

### 3. Verify Configuration

```bash
# Check exception configuration
klaudiush debug exceptions

# View audit log
klaudiush audit list
```

## Token Format

Exception tokens follow this format:

```text
<PREFIX>:<ERROR_CODE>:<URL_ENCODED_REASON>
```

### Components

| Component  | Required | Description                       | Example            |
|:-----------|:---------|:----------------------------------|:-------------------|
| PREFIX     | Yes      | Token identifier (default: `EXC`) | `EXC`              |
| ERROR_CODE | Yes      | Validator error code              | `GIT019`           |
| REASON     | Depends  | URL-encoded justification         | `Emergency+hotfix` |

### Token Placement

#### Shell Comment (Recommended)

```bash
git push origin main  # EXC:GIT019:Emergency+hotfix

# Multiple commands
git add . && git commit -sS -m "fix" && git push  # EXC:GIT022:Hotfix
```

#### Environment Variable

```bash
KLAUDIUSH_ACK="EXC:SEC001:Test+fixture" git commit -sS -m "Add test data"

# With reason
export KLAUDIUSH_ACK="EXC:GIT019:Emergency+release"
git push origin main
```

### URL Encoding Reasons

Reasons must be URL-encoded to avoid shell parsing issues:

| Character | Encoded |
|:----------|:--------|
| Space     | `+`     |
| `&`       | `%26`   |
| `=`       | `%3D`   |
| `/`       | `%2F`   |
| `#`       | `%23`   |

**Examples:**

- `Emergency hotfix` → `Emergency+hotfix`
- `Fix for issue #123` → `Fix+for+issue+%23123`
- `Deploy v1.2.3` → `Deploy+v1.2.3`

## Policy Configuration

### ExceptionsConfig Schema

```toml
[exceptions]
# Enable/disable the exception system (default: true)
enabled = true

# Custom token prefix (default: "EXC")
token_prefix = "EXC"

# Per-error-code policies
[exceptions.policies.ERROR_CODE]
# ...policy settings...

# Global rate limits
[exceptions.rate_limit]
# ...rate limit settings...

# Audit logging
[exceptions.audit]
# ...audit settings...
```

### ExceptionPolicyConfig Schema

```toml
[exceptions.policies.GIT019]
# Enable/disable this policy (default: true)
enabled = true

# Allow exceptions for this error code (default: true)
allow_exception = true

# Require a justification reason (default: false)
require_reason = true

# Minimum reason length when required (default: 10)
min_reason_length = 15

# List of pre-approved reasons (optional)
valid_reasons = ["emergency hotfix", "approved by lead", "security patch"]

# Per-code rate limits (optional, 0 = unlimited)
max_per_hour = 5
max_per_day = 20

# Human-readable description
description = "Exception for pushing to protected branches"
```

### Policy Options

| Option              | Type     | Default | Description                             |
|:--------------------|:---------|:--------|:----------------------------------------|
| `enabled`           | bool     | true    | Enable this policy                      |
| `allow_exception`   | bool     | true    | Allow exceptions for this code          |
| `require_reason`    | bool     | false   | Require justification reason            |
| `min_reason_length` | int      | 10      | Minimum reason length                   |
| `valid_reasons`     | []string | []      | Pre-approved reasons (case-insensitive) |
| `max_per_hour`      | int      | 0       | Max uses per hour (0 = unlimited)       |
| `max_per_day`       | int      | 0       | Max uses per day (0 = unlimited)        |
| `description`       | string   | ""      | Human-readable description              |

### Valid Reasons List

When `valid_reasons` is set, only these reasons are accepted:

```toml
[exceptions.policies.SEC001]
require_reason = true
valid_reasons = [
    "test fixture",
    "mock data",
    "example config",
    "documentation sample"
]
```

Matching is case-insensitive and supports prefix matching:

- `test fixture` matches `test+fixture`, `Test+Fixture`, `TEST+FIXTURE`
- `test` matches `test+fixture+data` (prefix match)

## Rate Limiting

### Global Rate Limits

```toml
[exceptions.rate_limit]
# Enable/disable rate limiting (default: true)
enabled = true

# Global limits (all error codes combined)
max_per_hour = 10   # default: 10
max_per_day = 50    # default: 50

# State file location (default: ~/.klaudiush/exception_state.json)
state_file = "~/.klaudiush/exception_state.json"
```

### Per-Code Rate Limits

Set limits for specific error codes:

```toml
[exceptions.policies.GIT019]
max_per_hour = 3
max_per_day = 10

[exceptions.policies.SEC001]
max_per_hour = 5
max_per_day = 25
```

### How Rate Limiting Works

1. **Window tracking**: Hourly and daily windows reset automatically
2. **Global + per-code**: Both limits must pass
3. **State persistence**: Survives restarts via state file
4. **Graceful degradation**: Continues if state file is unavailable

### Rate Limit State

View current state:

```bash
klaudiush debug exceptions --state
```

Output:

```text
Rate Limit State
----------------
  Hour Window Start: 2025-01-15 10:00:00
  Day Window Start: 2025-01-15 00:00:00
  Global Hourly Count: 3
  Global Daily Count: 12
  Hourly Usage by Code:
    GIT019: 2
    SEC001: 1
```

## Audit Logging

### Audit Configuration

```toml
[exceptions.audit]
# Enable/disable audit logging (default: true)
enabled = true

# Log file location (default: ~/.klaudiush/exception_audit.jsonl)
log_file = "~/.klaudiush/exception_audit.jsonl"

# Max file size before rotation (MB, default: 10)
max_size_mb = 10

# Max age of entries (days, default: 30)
max_age_days = 30

# Number of backup files to keep (default: 3)
max_backups = 3
```

### Audit Entry Format

Entries are stored in JSONL format (one JSON object per line):

```json
{
  "timestamp": "2025-01-15T10:30:45Z",
  "error_code": "GIT019",
  "validator_name": "git.push",
  "allowed": true,
  "reason": "Emergency hotfix",
  "source": "comment",
  "command": "git push origin main",
  "working_dir": "/Users/dev/project",
  "repository": "/Users/dev/project"
}
```

### Audit Entry Fields

| Field            | Description                          |
|:-----------------|:-------------------------------------|
| `timestamp`      | When the exception was processed     |
| `error_code`     | Validator error code                 |
| `validator_name` | Name of the validator                |
| `allowed`        | Whether exception was allowed        |
| `reason`         | Justification provided               |
| `denial_reason`  | Why exception was denied (if denied) |
| `source`         | Token source (comment, env_var)      |
| `command`        | Command that triggered the exception |
| `working_dir`    | Working directory                    |
| `repository`     | Git repository path                  |

## CLI Commands

### Debug Exceptions

View exception configuration:

```bash
# Show all configuration
klaudiush debug exceptions

# Include rate limit state
klaudiush debug exceptions --state
```

### Audit Commands

```bash
# List all entries
klaudiush audit list

# Filter by error code
klaudiush audit list --error-code GIT019

# Filter by outcome
klaudiush audit list --outcome allowed
klaudiush audit list --outcome denied

# Limit results
klaudiush audit list --limit 10

# JSON output
klaudiush audit list --json

# View statistics
klaudiush audit stats

# Clean up old entries
klaudiush audit cleanup
```

## Integration with Rules

Exception tokens work with both built-in validators and custom rules.

### Built-in Validator Errors

Built-in validators use error codes like:

- `GIT001`-`GIT024`: Git validators
- `FILE001`-`FILE005`: File validators
- `SEC001`-`SEC005`: Secrets validators
- `SHELL001`-`SHELL005`: Shell validators

### Custom Rule References

Custom rules can define references for exception support:

```toml
[[rules.rules]]
name = "block-production-deploy"
priority = 100

[rules.rules.match]
command_pattern = "*kubectl apply*production*"

[rules.rules.action]
type = "block"
message = "Production deployments require approval"
reference = "DEPLOY001"  # Enable exceptions for this rule
```

Then configure the exception policy:

```toml
[exceptions.policies.DEPLOY001]
enabled = true
require_reason = true
min_reason_length = 20
valid_reasons = ["approved by SRE", "emergency rollback", "scheduled release"]
```

## Examples

### Emergency Hotfix Workflow

Configuration:

```toml
[exceptions.policies.GIT019]
enabled = true
require_reason = true
min_reason_length = 10
max_per_hour = 3
max_per_day = 10
description = "Emergency push to protected branch"
```

Usage:

```bash
# Fix critical bug
git add . && git commit -sS -m "fix: critical security patch"

# Push with exception (normally blocked)
git push origin main  # EXC:GIT019:Critical+security+patch+CVE-2025-1234
```

### Test Fixture Secrets

Configuration:

```toml
[exceptions.policies.SEC001]
enabled = true
require_reason = true
valid_reasons = ["test fixture", "mock data", "example config"]
description = "Allow secrets in test files"
```

Usage:

```bash
# Write test file with mock credentials
KLAUDIUSH_ACK="EXC:SEC001:test+fixture" cat > test/fixtures/config.json << 'EOF'
{
  "api_key": "test_key_12345",
  "password": "mock_password"
}
EOF
```

### Strict Policy (No Exceptions)

```toml
[exceptions.policies.SEC003]
enabled = true
allow_exception = false
description = "Never allow exceptions for private key commits"
```

### Rate-Limited Policy

```toml
[exceptions.policies.GIT022]
enabled = true
require_reason = true
max_per_hour = 2
max_per_day = 5
description = "Limited exceptions for commit message format"
```

## Troubleshooting

### Exception Not Allowed

**Symptoms:** Block not bypassed despite token present

**Check:**

1. **Policy exists:** `klaudiush debug exceptions`
2. **Token format:** Verify `EXC:CODE:reason` format
3. **Error code match:** Token code must match block code
4. **Policy enabled:** Check `enabled = true` and `allow_exception = true`
5. **Reason provided:** If `require_reason = true`, include reason

### Rate Limit Exceeded

**Symptoms:** "Rate limit exceeded" message

**Check:**

1. **Current state:** `klaudiush debug exceptions --state`
2. **Global limits:** Check `max_per_hour` and `max_per_day`
3. **Per-code limits:** Check policy-specific limits
4. **Wait for reset:** Hourly resets on the hour, daily at midnight

### Audit Log Issues

**Symptoms:** Entries not appearing or log too large

**Check:**

1. **Logging enabled:** Check `audit.enabled = true`
2. **File location:** Verify `audit.log_file` path
3. **Run cleanup:** `klaudiush audit cleanup`
4. **Check rotation:** Review `max_size_mb` setting

### Token Not Detected

**Symptoms:** Token in command but exception not processed

**Check:**

1. **Comment format:** Ensure `# EXC:CODE:reason` (space after `#`)
2. **URL encoding:** Encode special characters in reason
3. **Env var name:** Use `KLAUDIUSH_ACK` exactly
4. **Token prefix:** Match configured `token_prefix` (default: `EXC`)

### Common Mistakes

1. **Missing space after #:**
   - Wrong: `git push #EXC:GIT019:reason`
   - Correct: `git push # EXC:GIT019:reason`

2. **Unencoded spaces:**
   - Wrong: `EXC:GIT019:Emergency hotfix`
   - Correct: `EXC:GIT019:Emergency+hotfix`

3. **Wrong error code:**
   - Check the error reference in the block message
   - Error codes are case-sensitive

4. **Policy not loaded:**
   - Project config: `.klaudiush/config.toml`
   - Global config: `~/.klaudiush/config.toml`
