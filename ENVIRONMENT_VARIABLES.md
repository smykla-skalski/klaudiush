# Environment Variables Reference

This document provides a comprehensive reference for all environment variables supported by klaudiush.

## Overview

All klaudiush environment variables use the `KLAUDIUSH_` prefix and follow a hierarchical naming convention that maps directly to the TOML configuration structure.

**Naming Convention**:

```
KLAUDIUSH_<CATEGORY>_<VALIDATOR>_<SECTION>_<OPTION>=value
```

**Examples**:

- `KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false`
- `KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH=72`
- `KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY=warning`

## Global Settings

### Git SDK Configuration

Control which git implementation is used for git operations.

```bash
# Use SDK implementation (default)
export KLAUDIUSH_USE_SDK_GIT=true

# Use CLI implementation
export KLAUDIUSH_USE_SDK_GIT=false
```

## Git Validators

### Git Add Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_GIT_ADD_ENABLED=true

# Set severity (error or warning)
export KLAUDIUSH_VALIDATORS_GIT_ADD_SEVERITY=error

# Set blocked patterns (comma-separated)
export KLAUDIUSH_VALIDATORS_GIT_ADD_BLOCKED_PATTERNS="tmp/*,*.secret"
```

### Git Commit Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_SEVERITY=error

# Required flags (comma-separated)
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_REQUIRED_FLAGS="-s,-S"

# Check staging area
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_CHECK_STAGING_AREA=true

# Enable message validation
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLE_MESSAGE_VALIDATION=true
```

#### Commit Message Validation

```bash
# Title max length
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH=50

# Body max line length
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_BODY_MAX_LINE_LENGTH=72

# Body line tolerance
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_BODY_LINE_TOLERANCE=5

# Check conventional commits
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_CHECK_CONVENTIONAL_COMMITS=true

# Valid commit types (comma-separated)
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_VALID_TYPES="feat,fix,docs,style,refactor,perf,test,build,ci,chore,revert"

# Require scope
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_REQUIRE_SCOPE=true

# Block infrastructure scope misuse
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_BLOCK_INFRA_SCOPE_MISUSE=true

# Block PR references
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_BLOCK_PR_REFERENCES=true

# Block AI attribution
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_BLOCK_AI_ATTRIBUTION=true

# Expected signoff
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_EXPECTED_SIGNOFF="Your Name <your.email@klaudiu.sh>"
```

### Git Push Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_GIT_PUSH_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_GIT_PUSH_SEVERITY=error
```

### Git PR Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_GIT_PR_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_GIT_PR_SEVERITY=error

# Title max length
export KLAUDIUSH_VALIDATORS_GIT_PR_TITLE_MAX_LENGTH=50

# Enable conventional commits for PR title
export KLAUDIUSH_VALIDATORS_GIT_PR_ENABLE_CONVENTIONAL_COMMITS=true

# Valid types (comma-separated)
export KLAUDIUSH_VALIDATORS_GIT_PR_VALID_TYPES="feat,fix,docs,style,refactor,perf,test,build,ci,chore,revert"

# Require changelog
export KLAUDIUSH_VALIDATORS_GIT_PR_REQUIRE_CHANGELOG=false

# Check CI labels
export KLAUDIUSH_VALIDATORS_GIT_PR_CHECK_CI_LABELS=false

# Require body
export KLAUDIUSH_VALIDATORS_GIT_PR_REQUIRE_BODY=false

# Markdownlint rules to disable for PR body (comma-separated)
export KLAUDIUSH_VALIDATORS_GIT_PR_MARKDOWN_DISABLED_RULES="MD013,MD034,MD041"
```

### Git Branch Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_GIT_BRANCH_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_GIT_BRANCH_SEVERITY=error

# Protected branches (comma-separated)
export KLAUDIUSH_VALIDATORS_GIT_BRANCH_PROTECTED_BRANCHES="main,master"

# Valid branch types (comma-separated)
export KLAUDIUSH_VALIDATORS_GIT_BRANCH_VALID_TYPES="feat,fix,chore,docs,refactor,test"

# Require type prefix
export KLAUDIUSH_VALIDATORS_GIT_BRANCH_REQUIRE_TYPE=true

# Allow uppercase in branch names
export KLAUDIUSH_VALIDATORS_GIT_BRANCH_ALLOW_UPPERCASE=false
```

### Git No-Verify Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_GIT_NO_VERIFY_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_GIT_NO_VERIFY_SEVERITY=error
```

## File Validators

### Markdown Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY=error

# Timeout (e.g., "10s", "30s")
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_TIMEOUT=10s

# Context lines for error messages
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_CONTEXT_LINES=2

# Enable heading spacing check
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_HEADING_SPACING=true

# Enable code block formatting check
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_CODE_BLOCK_FORMATTING=true

# Enable list formatting check
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_LIST_FORMATTING=true

# Use markdownlint-cli
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_USE_MARKDOWNLINT=false

# Custom markdownlint path (optional)
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_MARKDOWNLINT_PATH=/custom/path/to/markdownlint

# Markdownlint config file (optional)
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_MARKDOWNLINT_CONFIG=.markdownlint.json
```

### Shell Script Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_SHELLSCRIPT_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_SHELLSCRIPT_SEVERITY=error

# Timeout
export KLAUDIUSH_VALIDATORS_FILE_SHELLSCRIPT_TIMEOUT=10s

# Context lines
export KLAUDIUSH_VALIDATORS_FILE_SHELLSCRIPT_CONTEXT_LINES=2
```

### Terraform Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_SEVERITY=error

# Timeout
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_TIMEOUT=10s

# Context lines
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_CONTEXT_LINES=2

# Check format
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_CHECK_FORMAT=true

# Use tflint
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_USE_TFLINT=true
```

### GitHub Actions Workflow Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_SEVERITY=error

# Timeout
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_TIMEOUT=10s

# GitHub API timeout
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_GH_API_TIMEOUT=5s

# Enforce digest pinning
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_ENFORCE_DIGEST_PINNING=true

# Require version comment
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_REQUIRE_VERSION_COMMENT=true

# Check latest version
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_CHECK_LATEST_VERSION=true

# Use actionlint
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_USE_ACTIONLINT=true
```

## Notification Validators

### Bell Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_NOTIFICATION_BELL_ENABLED=true

# Custom notification command (optional)
export KLAUDIUSH_VALIDATORS_NOTIFICATION_BELL_CUSTOM_COMMAND="osascript -e 'beep'"
```

## Secrets Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_SECRETS_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_SECRETS_SEVERITY=error

# Gitleaks integration
export KLAUDIUSH_VALIDATORS_SECRETS_USE_GITLEAKS=false

# Disabled patterns (comma-separated)
export KLAUDIUSH_VALIDATORS_SECRETS_DISABLED_PATTERNS="generic-secret"

# Allow list (comma-separated, supports regex)
export KLAUDIUSH_VALIDATORS_SECRETS_ALLOW_LIST="AKIAIOSFODNN7EXAMPLE,test_.*"
```

## Shell Validators

### Backtick Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_SHELL_BACKTICK_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_SHELL_BACKTICK_SEVERITY=error

# Enable comprehensive mode (all commands)
export KLAUDIUSH_VALIDATORS_SHELL_BACKTICK_CHECK_ALL_COMMANDS=false

# Check unquoted backticks (comprehensive mode)
export KLAUDIUSH_VALIDATORS_SHELL_BACKTICK_CHECK_UNQUOTED=false

# Suggest single quotes when no variables present
export KLAUDIUSH_VALIDATORS_SHELL_BACKTICK_SUGGEST_SINGLE_QUOTES=true
```

## GitHub Validators

### Issue Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_GITHUB_ISSUE_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_GITHUB_ISSUE_SEVERITY=error

# Timeout
export KLAUDIUSH_VALIDATORS_GITHUB_ISSUE_TIMEOUT=10s

# Require body
export KLAUDIUSH_VALIDATORS_GITHUB_ISSUE_REQUIRE_BODY=false

# Markdownlint rules to disable (comma-separated)
export KLAUDIUSH_VALIDATORS_GITHUB_ISSUE_MARKDOWN_DISABLED_RULES="MD013,MD034,MD041,MD047"
```

## File Validators (additional)

### Gofumpt Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_GOFUMPT_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_GOFUMPT_SEVERITY=error

# Timeout
export KLAUDIUSH_VALIDATORS_FILE_GOFUMPT_TIMEOUT=10s

# Enable extra rules
export KLAUDIUSH_VALIDATORS_FILE_GOFUMPT_EXTRA_RULES=true

# Go language version (auto-detected from go.mod if empty)
export KLAUDIUSH_VALIDATORS_FILE_GOFUMPT_LANG=""

# Module path (auto-detected from go.mod if empty)
export KLAUDIUSH_VALIDATORS_FILE_GOFUMPT_MOD_PATH=""
```

### Python Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_PYTHON_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_PYTHON_SEVERITY=error

# Enable ruff integration
export KLAUDIUSH_VALIDATORS_FILE_PYTHON_USE_RUFF=true

# Timeout
export KLAUDIUSH_VALIDATORS_FILE_PYTHON_TIMEOUT=10s

# Context lines for edit validation
export KLAUDIUSH_VALIDATORS_FILE_PYTHON_CONTEXT_LINES=2

# Exclude rules (comma-separated)
export KLAUDIUSH_VALIDATORS_FILE_PYTHON_EXCLUDE_RULES="E501"

# Ruff config file path
export KLAUDIUSH_VALIDATORS_FILE_PYTHON_RUFF_CONFIG=""
```

### JavaScript Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_JAVASCRIPT_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_JAVASCRIPT_SEVERITY=error

# Enable oxlint integration
export KLAUDIUSH_VALIDATORS_FILE_JAVASCRIPT_USE_OXLINT=true

# Timeout
export KLAUDIUSH_VALIDATORS_FILE_JAVASCRIPT_TIMEOUT=10s

# Context lines for edit validation
export KLAUDIUSH_VALIDATORS_FILE_JAVASCRIPT_CONTEXT_LINES=2

# Exclude rules (comma-separated)
export KLAUDIUSH_VALIDATORS_FILE_JAVASCRIPT_EXCLUDE_RULES=""

# Oxlint config file path
export KLAUDIUSH_VALIDATORS_FILE_JAVASCRIPT_OXLINT_CONFIG=""
```

### Rust Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_RUST_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_RUST_SEVERITY=error

# Enable rustfmt integration
export KLAUDIUSH_VALIDATORS_FILE_RUST_USE_RUSTFMT=true

# Timeout
export KLAUDIUSH_VALIDATORS_FILE_RUST_TIMEOUT=10s

# Context lines for edit validation
export KLAUDIUSH_VALIDATORS_FILE_RUST_CONTEXT_LINES=2

# Rust edition (auto-detected from Cargo.toml if empty)
export KLAUDIUSH_VALIDATORS_FILE_RUST_EDITION=""

# Rustfmt config file path
export KLAUDIUSH_VALIDATORS_FILE_RUST_RUSTFMT_CONFIG=""
```

### Linter Ignore Validator

```bash
# Enable/disable validator
export KLAUDIUSH_VALIDATORS_FILE_LINTER_IGNORE_ENABLED=true

# Set severity
export KLAUDIUSH_VALIDATORS_FILE_LINTER_IGNORE_SEVERITY=error
```

## Global Settings (additional)

```bash
# Default timeout for all validators
export KLAUDIUSH_GLOBAL_DEFAULT_TIMEOUT=10s

# Enable parallel execution
export KLAUDIUSH_GLOBAL_PARALLEL_EXECUTION=true

# Max CPU workers
export KLAUDIUSH_GLOBAL_MAX_CPU_WORKERS=4

# Max IO workers
export KLAUDIUSH_GLOBAL_MAX_IO_WORKERS=4

# Max git workers
export KLAUDIUSH_GLOBAL_MAX_GIT_WORKERS=2
```

## Crash Dump

```bash
# Enable crash dumps
export KLAUDIUSH_CRASH_DUMP_ENABLED=true

# Dump directory
export KLAUDIUSH_CRASH_DUMP_DUMP_DIR="~/.klaudiush/crash_dumps"

# Max dumps to keep
export KLAUDIUSH_CRASH_DUMP_MAX_DUMPS=10

# Max age (duration format)
export KLAUDIUSH_CRASH_DUMP_MAX_AGE=720h

# Include sanitized config in dump
export KLAUDIUSH_CRASH_DUMP_INCLUDE_CONFIG=true

# Include hook context in dump
export KLAUDIUSH_CRASH_DUMP_INCLUDE_CONTEXT=true
```

## Exception Workflow

```bash
# Enable exceptions
export KLAUDIUSH_EXCEPTIONS_ENABLED=true

# Require explicit policy for each error code
export KLAUDIUSH_EXCEPTIONS_REQUIRE_EXPLICIT_POLICY=true
```

### Exception Audit

```bash
# Enable exception audit
export KLAUDIUSH_EXCEPTIONS_AUDIT_ENABLED=true

# Audit log file
export KLAUDIUSH_EXCEPTIONS_AUDIT_LOG_FILE="~/.klaudiush/exception_audit.jsonl"

# Max log size in MB
export KLAUDIUSH_EXCEPTIONS_AUDIT_MAX_SIZE_MB=10

# Max age in days
export KLAUDIUSH_EXCEPTIONS_AUDIT_MAX_AGE_DAYS=90

# Max backups
export KLAUDIUSH_EXCEPTIONS_AUDIT_MAX_BACKUPS=5
```

### Exception Rate Limiting

```bash
# Global hourly limit
export KLAUDIUSH_EXCEPTIONS_RATE_LIMIT_GLOBAL_HOURLY=10

# Global daily limit
export KLAUDIUSH_EXCEPTIONS_RATE_LIMIT_GLOBAL_DAILY=50
```

## Backup System

```bash
# Enable backup
export KLAUDIUSH_BACKUP_ENABLED=true

# Auto-backup on config changes
export KLAUDIUSH_BACKUP_AUTO_BACKUP=true

# Max backups to keep
export KLAUDIUSH_BACKUP_MAX_BACKUPS=10

# Max age in days
export KLAUDIUSH_BACKUP_MAX_AGE_DAYS=30

# Max total size in MB
export KLAUDIUSH_BACKUP_MAX_TOTAL_SIZE_MB=50

# Async backup operations
export KLAUDIUSH_BACKUP_ASYNC=true
```

## Plugin System

```bash
# Enable plugins
export KLAUDIUSH_PLUGINS_ENABLED=true

# Plugin directory
export KLAUDIUSH_PLUGINS_DIRECTORY="~/.klaudiush/plugins"

# Default plugin timeout
export KLAUDIUSH_PLUGINS_DEFAULT_TIMEOUT=10s
```

## Rules System

```bash
# Enable rules
export KLAUDIUSH_RULES_ENABLED=true

# Stop on first matching rule
export KLAUDIUSH_RULES_STOP_ON_FIRST_MATCH=true
```

## Patterns System

```bash
# Enable pattern detection
export KLAUDIUSH_PATTERNS_ENABLED=true

# Minimum occurrence count to trigger
export KLAUDIUSH_PATTERNS_MIN_COUNT=3
```

## Standard Environment Variables

These are not prefixed with `KLAUDIUSH_` but affect klaudiush behavior:

```bash
# Exception/unpoison token (inline with command)
export KLACK="EXC:GIT019:Emergency+hotfix"

# GitHub token for API calls (workflow validator)
export GH_TOKEN="..."
export GITHUB_TOKEN="..."

# Disable color output
export NO_COLOR=1

# Enable color output
export CLICOLOR=1

# Terminal type
export TERM=xterm-256color

# Claude Code tool input (set automatically by Claude Code)
export CLAUDE_TOOL_INPUT="..."
```

## Value Types

### Boolean Values

Environment variables accept the following boolean values:

- **True**: `true`, `1`, `yes`, `on`
- **False**: `false`, `0`, `no`, `off`

**Example**:

```bash
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=true
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED=1
export KLAUDIUSH_VALIDATORS_GIT_PR_REQUIRE_CHANGELOG=false
export KLAUDIUSH_VALIDATORS_GIT_BRANCH_ALLOW_UPPERCASE=0
```

### Duration Values

Duration values use Go's duration format:

- `10s` - 10 seconds
- `30s` - 30 seconds
- `1m` - 1 minute
- `5m30s` - 5 minutes and 30 seconds

**Example**:

```bash
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_TIMEOUT=30s
export KLAUDIUSH_VALIDATORS_FILE_WORKFLOW_GH_API_TIMEOUT=10s
```

### String Values

String values are used as-is. For comma-separated lists, use commas without spaces:

```bash
# Correct
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_VALID_TYPES="feat,fix,docs"

# Incorrect (spaces will be included)
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_VALID_TYPES="feat, fix, docs"
```

## Precedence

Environment variables have higher precedence than configuration files but lower precedence than CLI flags:

1. **CLI Flags** (highest)
2. **Environment Variables**
3. **Project Config** (`.klaudiush/config.toml` or `klaudiush.toml`)
4. **Global Config** (`~/.klaudiush/config.toml`)
5. **Built-in Defaults** (lowest)

## Usage Examples

### Disable Commit Validation in CI

```bash
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false
klaudiush --hook-type PreToolUse
```

### Allow Longer Commit Titles

```bash
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH=72
klaudiush --hook-type PreToolUse
```

### Change Validator Severity to Warning

```bash
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_SEVERITY=warning
klaudiush --hook-type PreToolUse
```

### Increase Terraform Timeout

```bash
export KLAUDIUSH_VALIDATORS_FILE_TERRAFORM_TIMEOUT=60s
klaudiush --hook-type PreToolUse
```

### Use Custom Signoff

```bash
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_EXPECTED_SIGNOFF="CI Bot <bot@klaudiu.sh>"
klaudiush --hook-type PreToolUse
```

## See Also

- [Configuration Guide](README.md#configuration) - Complete configuration documentation
- [Example Configurations](examples/config/) - Example TOML configuration files
