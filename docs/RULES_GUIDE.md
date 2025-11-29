# Dynamic Validation Rules Guide

Configure klaudiush validators dynamically without modifying code.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Rule Configuration](#rule-configuration)
- [Pattern Matching](#pattern-matching)
- [Match Conditions](#match-conditions)
- [Actions](#actions)
- [Configuration Precedence](#configuration-precedence)
- [Validator Types](#validator-types)
- [Examples](#examples)
- [Exceptions Integration](#exceptions-integration)
- [Troubleshooting](#troubleshooting)

## Overview

The rule engine allows users to define custom validation behavior through TOML configuration. Rules can:

- Block operations based on patterns (repository, branch, file, command)
- Warn about potentially dangerous operations
- Allow operations that would otherwise be blocked
- Apply different validation logic per validator type

### Key Features

| Feature           | Description                                           |
|:------------------|:------------------------------------------------------|
| Pattern Matching  | Auto-detect glob or regex patterns                    |
| Priority System   | Higher priority rules evaluate first                  |
| Config Precedence | Project config overrides global config                |
| Validator Scoping | Apply rules to specific or all validators             |
| First-Match       | Stop evaluation on first matching rule (configurable) |

## Quick Start

### 1. Create Configuration File

**Project-level** (`.klaudiush/config.toml`):

```toml
[rules]
enabled = true

[[rules.rules]]
name = "block-main-push"
description = "Prevent direct pushes to main branch"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Direct push to main branch is not allowed. Use a pull request."
```

**Global** (`~/.klaudiush/config.toml`):

```toml
[rules]
enabled = true

[[rules.rules]]
name = "warn-force-push"
description = "Warn about force pushes"
priority = 50

[rules.rules.match]
validator_type = "git.push"
command_pattern = "*--force*"

[rules.rules.action]
type = "warn"
message = "Force push detected. Ensure you have the latest changes."
```

### 2. Verify Rules Load

```bash
# Run with debug logging
klaudiush --debug
```

## Rule Configuration

### RulesConfig Schema

```toml
[rules]
# Enable/disable the rule engine (default: true)
enabled = true

# Stop evaluation on first matching rule (default: true)
stop_on_first_match = true

# List of rules
[[rules.rules]]
# ...rule definitions...
```

### RuleConfig Schema

```toml
[[rules.rules]]
# Required: Unique identifier for override precedence
name = "my-rule"

# Optional: Human-readable description
description = "What this rule does"

# Optional: Enable/disable this rule (default: true)
enabled = true

# Optional: Evaluation order, higher = first (default: 0)
priority = 100

# Required: Match conditions (all must match)
[rules.rules.match]
# ...match conditions...

# Required: Action to take when rule matches
[rules.rules.action]
# ...action configuration...
```

## Pattern Matching

The rule engine automatically detects pattern type:

### Glob Patterns

Simple wildcard patterns using `*` and `**`:

```toml
# Match any file in docs directory
file_pattern = "docs/*"

# Match markdown files in any subdirectory
file_pattern = "**/**.md"

# Match specific branches
branch_pattern = "feat/*"
branch_pattern = "release-*"

# Match repository paths
repo_pattern = "**/myorg/**"
```

**Glob syntax:**

- `*` - matches any sequence of characters (except path separator)
- `**` - matches any sequence of characters including path separators
- `?` - matches any single character
- `[abc]` - matches any character in the set
- `{a,b}` - matches either pattern

### Regex Patterns

Automatically detected when pattern contains regex indicators:

```toml
# Match semantic version branches
branch_pattern = "^release/v[0-9]+\\.[0-9]+$"

# Match ticket references in commands
command_pattern = ".*JIRA-[0-9]+.*"

# Match specific file extensions
file_pattern = ".*\\.(tf|tfvars)$"
```

**Regex indicators** (trigger regex mode):

- `^` `$` - anchors
- `(?` - non-capturing groups
- `\\d` `\\w` `\\s` - character classes
- `[` `]` - character classes
- `(` `)` - groups
- `|` - alternation
- `+` `.+` `.*` - quantifiers

## Match Conditions

All non-empty conditions must match (AND logic).

### ValidatorType

Filter by validator type:

```toml
# Specific validator
validator_type = "git.push"

# All git validators
validator_type = "git.*"

# All validators
validator_type = "*"
```

### RepoPattern

Match against repository root path:

```toml
# Match organization repositories
repo_pattern = "**/myorg/**"

# Match specific project
repo_pattern = "**/my-project"
```

### Remote

Exact match against git remote name:

```toml
# Match origin remote
remote = "origin"

# Match upstream remote
remote = "upstream"
```

### BranchPattern

Match against branch name:

```toml
# Match main branch
branch_pattern = "main"

# Match feature branches
branch_pattern = "feat/*"

# Match release branches (regex)
branch_pattern = "^release/v[0-9]+$"
```

### FilePattern

Match against file path:

```toml
# Match test files
file_pattern = "**/test/**"

# Match terraform files
file_pattern = "*.tf"

# Match workflow files
file_pattern = ".github/workflows/*.yml"
```

### ContentPattern

Match against file content (always regex):

```toml
# Match files containing secrets
content_pattern = "(?i)password\\s*=\\s*[\"'][^\"']+[\"']"

# Match TODO comments
content_pattern = "TODO|FIXME|HACK"
```

### CommandPattern

Match against bash command:

```toml
# Match git push commands
command_pattern = "git push*"

# Match force operations
command_pattern = "*--force*"

# Match dangerous rm commands (regex)
command_pattern = "rm\\s+-rf\\s+/"
```

### ToolType and EventType

Match against hook context:

```toml
# Match bash commands
tool_type = "Bash"

# Match file writes
tool_type = "Write"

# Match pre-execution hook
event_type = "PreToolUse"
```

## Actions

### Block

Stop the operation with an error:

```toml
[rules.rules.action]
type = "block"
message = "This operation is not allowed"
reference = "RULE001"  # Optional error code
```

### Warn

Log a warning but allow the operation:

```toml
[rules.rules.action]
type = "warn"
message = "This operation might cause issues"
```

### Allow

Explicitly allow the operation (skip further rules and built-in validation):

```toml
[rules.rules.action]
type = "allow"
message = "Operation allowed by rule"  # Optional
```

## Configuration Precedence

Rules are loaded and merged from multiple sources:

1. **CLI Flags** (highest priority)
2. **Environment Variables** (`KLAUDIUSH_*`)
3. **Project Config** (`.klaudiush/config.toml`)
4. **Global Config** (`~/.klaudiush/config.toml`)
5. **Defaults** (lowest priority)

### Rule Merge Semantics

When loading rules from multiple sources:

- **Same name**: Project rule overrides global rule
- **Different names**: Rules are combined

```toml
# Global config: ~/.klaudiush/config.toml
[[rules.rules]]
name = "warn-upstream"
priority = 50
# ...

# Project config: .klaudiush/config.toml
[[rules.rules]]
name = "warn-upstream"  # Same name = override
priority = 100          # Project value takes precedence
# ...

[[rules.rules]]
name = "block-main"     # Different name = add to list
# ...
```

### Priority-Based Evaluation

Rules evaluate in priority order (highest first):

```toml
[[rules.rules]]
name = "allow-test-files"
priority = 1000  # Evaluated first
[rules.rules.match]
file_pattern = "**/test/**"
[rules.rules.action]
type = "allow"

[[rules.rules]]
name = "block-secrets"
priority = 500   # Evaluated second
[rules.rules.match]
content_pattern = "password"
[rules.rules.action]
type = "block"
```

## Validator Types

### Git Validators

| Type            | Description            |
|:----------------|:-----------------------|
| `git.push`      | Git push operations    |
| `git.commit`    | Git commit operations  |
| `git.add`       | Git add operations     |
| `git.pr`        | Pull request ops       |
| `git.merge`     | Git merge operations   |
| `git.branch`    | Git branch operations  |
| `git.no_verify` | --no-verify flag usage |
| `git.*`         | All git validators     |

### File Validators

| Type             | Description               |
|:-----------------|:--------------------------|
| `file.markdown`  | Markdown file validation  |
| `file.shell`     | Shell script validation   |
| `file.terraform` | Terraform file validation |
| `file.workflow`  | GitHub Actions workflow   |
| `file.*`         | All file validators       |

### Other Validators

| Type                | Description                |
|:--------------------|:---------------------------|
| `secrets.secrets`   | Secrets detection          |
| `shell.backtick`    | Backtick command injection |
| `notification.bell` | Terminal notifications     |
| `*`                 | All validators             |

## Examples

### Block Direct Push to Main

```toml
[[rules.rules]]
name = "block-main-push"
description = "Prevent direct pushes to main branch"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Direct push to main is not allowed. Please use a pull request."
reference = "GIT019"
```

### Allow Test File Secrets

```toml
[[rules.rules]]
name = "allow-test-secrets"
description = "Allow secrets in test fixtures"
priority = 1000

[rules.rules.match]
validator_type = "secrets.secrets"
file_pattern = "**/test/**"

[rules.rules.action]
type = "allow"
```

### Warn About Upstream Push

```toml
[[rules.rules]]
name = "warn-upstream-push"
description = "Warn when pushing to upstream remote"
priority = 50

[rules.rules.match]
validator_type = "git.push"
remote = "upstream"

[rules.rules.action]
type = "warn"
message = "Pushing to upstream remote. Ensure this is intentional."
```

### Require Ticket Reference

```toml
[[rules.rules]]
name = "require-ticket-main"
description = "Require JIRA ticket in commits to main"
priority = 100

[rules.rules.match]
validator_type = "git.commit"
branch_pattern = "main"

[rules.rules.action]
type = "block"
message = "Commits to main must reference a JIRA ticket (e.g., JIRA-123)"
```

### Strict Docs Formatting

```toml
[[rules.rules]]
name = "strict-docs"
description = "Enforce strict markdown formatting in docs"
priority = 100

[rules.rules.match]
validator_type = "file.markdown"
file_pattern = "docs/**/*.md"

[rules.rules.action]
type = "block"
message = "Documentation files must pass strict markdown validation"
```

### Organization-Specific Rules

```toml
# Block pushing to origin in organization repositories (forks)
[[rules.rules]]
name = "block-org-origin"
description = "Block push to origin in organization repos"
priority = 100

[rules.rules.match]
validator_type = "git.push"
repo_pattern = "**/myorg/**"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Organization repos use 'upstream' for main repository. Push to your fork."
```

### Disable Rule

```toml
[[rules.rules]]
name = "block-org-origin"
enabled = false  # Disable this rule
```

## Exceptions Integration

Rules work alongside the exception workflow system. When a rule blocks an operation,
Claude can use an exception token to bypass it (if configured).

### How Rules and Exceptions Interact

1. **Rule evaluates** - If a rule matches, it returns block/warn/allow
2. **Block triggers exception check** - If blocked, klaudiush checks for exception token
3. **Exception evaluated** - If token present and valid, block becomes warning
4. **Audit logged** - Exception usage is logged for compliance

### Allowing Exceptions for Rule Blocks

To allow exceptions for a rule-generated block, ensure the rule includes a `reference`:

```toml
[[rules.rules]]
name = "block-main-push"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"

[rules.rules.action]
type = "block"
message = "Direct push to main is not allowed"
reference = "RULE001"  # This enables exception bypass
```

Then configure an exception policy for that reference:

```toml
[exceptions.policies.RULE001]
enabled = true
allow_exception = true
require_reason = true
min_reason_length = 15
description = "Exception for direct push to main"
```

### Bypassing with Exception Token

When a rule blocks with a reference, Claude can include an exception token:

```bash
# In shell comment
git push origin main  # EXC:RULE001:Emergency+hotfix+approved+by+lead

# Via environment variable
KLAUDIUSH_ACK="EXC:RULE001:Hotfix+deployment" git push origin main
```

See [EXCEPTIONS_GUIDE.md](EXCEPTIONS_GUIDE.md) for complete exception configuration.

## Troubleshooting

### Rule Not Matching

1. **Check pattern type**: Ensure glob vs regex is correctly detected
2. **Check all conditions**: All non-empty conditions must match
3. **Check priority**: Higher priority rules evaluate first
4. **Enable debug logging**: `klaudiush --debug`

### Rule Conflicts

If rules conflict, the first matching rule wins (by priority):

```toml
# This rule matches first (higher priority)
[[rules.rules]]
name = "allow-test"
priority = 1000
[rules.rules.action]
type = "allow"

# This rule is skipped if "allow-test" matches
[[rules.rules]]
name = "block-all"
priority = 500
[rules.rules.action]
type = "block"
```

### Config Not Loading

1. **Check file location**: `.klaudiush/config.toml` (project) or `~/.klaudiush/config.toml` (global)
2. **Check TOML syntax**: Validate with `toml` command or online validator
3. **Check permissions**: Ensure file is readable

### Pattern Debugging

Test patterns in isolation:

```go
// Glob pattern test
glob.Compile("**/myorg/**", '/')

// Regex pattern test
regexp.Compile("^release/v[0-9]+$")
```

### Common Mistakes

1. **Missing separator in glob**: Use `/` as path separator
   - Wrong: `**\test\**`
   - Correct: `**/test/**`

2. **Unescaped regex characters**: Escape special characters
   - Wrong: `.*\.tf`
   - Correct: `.*\\.tf`

3. **Wrong pattern type**: Check if pattern is detected as glob or regex
   - `feat/*` -> glob
   - `feat/.*` -> regex (due to `.*`)

4. **Missing quotes**: TOML strings need quotes
   - Wrong: `pattern = **/test/**`
   - Correct: `pattern = "**/test/**"`
