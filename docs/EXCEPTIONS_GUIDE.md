# Exception workflow guide

Allow Claude to bypass validation denials with explicit acknowledgment and audit trail.

## Table of contents

- [Overview](#overview)
- [Quick start](#quick-start)
- [Token format](#token-format)
- [Policy configuration](#policy-configuration)
- [Rate limiting](#rate-limiting)
- [Audit logging](#audit-logging)
- [CLI commands](#cli-commands)
- [Integration with rules](#integration-with-rules)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The exception workflow lets Claude Code bypass specific validation denials when:

1. An exception policy exists for the error code
2. Claude includes an acknowledgment token in the command
3. The token includes a valid justification (if required)
4. Rate limits have not been exceeded

Exceptions use token-based acknowledgment embedded in commands, with per-error-code policies, hourly/daily rate limits, JSONL audit logging, and configurable justification requirements.

### How it works

```text
1. Claude runs: git push origin main  # EXC:GIT019:Emergency+hotfix
2. klaudiush detects validation failure (GIT019)
3. klaudiush finds exception token in command
4. Policy check: Is GIT019 exception allowed?
5. Rate limit check: Within limits?
6. If allowed: deny → allow with additionalContext, command proceeds
7. Audit entry logged
```

## Quick start

### 1. Enable exceptions

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

### 2. Bypass a denial

When Claude encounters a deny response, it can include an exception token:

```bash
# Shell comment format
git push origin main  # EXC:GIT019:Emergency+hotfix+for+production

# Environment variable format
KLACK="EXC:GIT019:Emergency+hotfix" git push origin main
```

### 3. Verify configuration

```bash
# Check exception configuration
klaudiush debug exceptions

# View audit log
klaudiush audit list
```

## Token format

Exception tokens follow this format:

```text
<PREFIX>:<ERROR_CODE>:<URL_ENCODED_REASON>
```

### Token components

| Component  | Required | Description                       | Example            |
|:-----------|:---------|:----------------------------------|:-------------------|
| PREFIX     | Yes      | Token identifier (default: `EXC`) | `EXC`              |
| ERROR_CODE | Yes      | Validator error code              | `GIT019`           |
| REASON     | Depends  | URL-encoded justification         | `Emergency+hotfix` |

### Token placement

#### Shell comment (recommended)

```bash
git push origin main  # EXC:GIT019:Emergency+hotfix

# Multiple commands
git add . && git commit -sS -m "fix" && git push  # EXC:GIT022:Hotfix
```

#### Environment variable

```bash
KLACK="EXC:SEC001:Test+fixture" git commit -sS -m "Add test data"

# With reason
export KLACK="EXC:GIT019:Emergency+release"
git push origin main
```

### URL encoding reasons

Reasons must be URL-encoded to avoid shell parsing issues:

| Character | Encoded |
|:----------|:--------|
| Space     | `+`     |
| `&`       | `%26`   |
| `=`       | `%3D`   |
| `/`       | `%2F`   |
| `#`       | `%23`   |

Examples:

- `Emergency hotfix` → `Emergency+hotfix`
- `Fix for issue #123` → `Fix+for+issue+%23123`
- `Deploy v1.2.3` → `Deploy+v1.2.3`

## Policy configuration

### ExceptionsConfig schema

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

### ExceptionPolicyConfig schema

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

### Policy options

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

### Valid reasons list

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

## Rate limiting

### Global rate limits

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

### Per-code rate limits

Set limits for specific error codes:

```toml
[exceptions.policies.GIT019]
max_per_hour = 3
max_per_day = 10

[exceptions.policies.SEC001]
max_per_hour = 5
max_per_day = 25
```

### How rate limiting works

Hourly and daily windows reset automatically. Both global and per-code limits must pass for an exception to go through. State persists across restarts via the state file. If the state file is unavailable, rate limiting degrades gracefully and commands continue.

### Rate limit state

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

## Audit logging

### Audit configuration

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

### Audit entry format

Each line in the log file is a single JSON object:

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

### Audit entry fields

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

## CLI commands

### Debug exceptions

View exception configuration:

```bash
# Show all configuration
klaudiush debug exceptions

# Include rate limit state
klaudiush debug exceptions --state
```

### Audit commands

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

## Integration with rules

Exception tokens work with both built-in validators and custom rules.

### Built-in validator errors

Built-in validators use these error code ranges:

- `GIT001`-`GIT024`: Git validators
- `FILE001`-`FILE005`: File validators
- `SEC001`-`SEC005`: Secrets validators
- `SHELL001`-`SHELL005`: Shell validators

### Custom rule references

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

### Emergency hotfix workflow

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

### Test fixture secrets

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
KLACK="EXC:SEC001:test+fixture" cat > test/fixtures/config.json << 'EOF'
{
  "api_key": "test_key_12345",
  "password": "mock_password"
}
EOF
```

### Strict policy (no exceptions)

```toml
[exceptions.policies.SEC003]
enabled = true
allow_exception = false
description = "Never allow exceptions for private key commits"
```

### Rate-limited policy

```toml
[exceptions.policies.GIT022]
enabled = true
require_reason = true
max_per_hour = 2
max_per_day = 5
description = "Limited exceptions for commit message format"
```

## Troubleshooting

### Exception not allowed

Command still denied despite token present.

1. Confirm a policy exists: `klaudiush debug exceptions`
2. Verify the token format is `EXC:CODE:reason`
3. Make sure the token code matches the denied error code exactly
4. Check that `enabled = true` and `allow_exception = true` in the policy
5. If `require_reason = true`, make sure you included a reason

### Rate limit exceeded

You see a "Rate limit exceeded" message.

1. Check current state: `klaudiush debug exceptions --state`
2. Review global `max_per_hour` and `max_per_day` settings
3. Review policy-specific limits for the error code
4. Wait for reset -- hourly resets on the hour, daily at midnight

### Audit log issues

Entries not appearing, or the log is too large.

1. Check that `audit.enabled = true`
2. Verify the `audit.log_file` path exists and is writable
3. Run `klaudiush audit cleanup` to clear old entries
4. Review the `max_size_mb` rotation setting

### Token not detected

Token is in the command but the exception is not processed.

1. Check comment format: use `# EXC:CODE:reason` (space after `#`)
2. URL-encode special characters in the reason
3. Use the env var name `KLACK` exactly
4. Match the configured `token_prefix` (default: `EXC`)

### Common mistakes

1. Missing space after `#`:
   - Wrong: `git push #EXC:GIT019:reason`
   - Correct: `git push # EXC:GIT019:reason`

2. Unencoded spaces:
   - Wrong: `EXC:GIT019:Emergency hotfix`
   - Correct: `EXC:GIT019:Emergency+hotfix`

3. Wrong error code:
   - Check the error code in the deny response's `permissionDecisionReason`
   - Error codes are case-sensitive

4. Policy not loaded:
   - Project config: `.klaudiush/config.toml`
   - Global config: `~/.klaudiush/config.toml`
