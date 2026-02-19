# Dynamic validation rules

Configure klaudiush validators without modifying code.

## Table of contents

- [Overview](#overview)
- [Quick start](#quick-start)
- [Rule configuration](#rule-configuration)
- [Pattern matching](#pattern-matching)
- [Match conditions](#match-conditions)
- [Actions](#actions)
- [Configuration precedence](#configuration-precedence)
- [Validator types](#validator-types)
- [Examples](#examples)
- [Exceptions integration](#exceptions-integration)
- [Troubleshooting](#troubleshooting)

## Overview

The rule engine defines validation behavior through TOML configuration. Rules can block operations based on patterns (repository, branch, file, command), warn about risky operations, allow operations that built-in validators would block, or apply different logic per validator type.

The engine auto-detects glob and regex patterns, evaluates rules by priority (highest first), merges project config over global config, and stops on the first matching rule by default. Rules can target specific validators or apply to all of them.

## Quick start

### 1. Create a configuration file

Project-level (`.klaudiush/config.toml`):

```toml
[rules]
enabled = true

[[rules.rules]]
name = "block-main-push"
description = "Block direct pushes to main"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Direct push to main branch is not allowed. Use a pull request."
```

Global (`~/.klaudiush/config.toml`):

```toml
[rules]
enabled = true

[[rules.rules]]
name = "warn-force-push"
description = "Flag force pushes"
priority = 50

[rules.rules.match]
validator_type = "git.push"
command_pattern = "*--force*"

[rules.rules.action]
type = "warn"
message = "Force push detected. Check that you have the latest changes."
```

### 2. Verify rules load

```bash
# Run with debug logging
klaudiush --debug
```

## Rule configuration

### RulesConfig

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

### RuleConfig

```toml
[[rules.rules]]
# Required: unique identifier for override precedence
name = "my-rule"

# Optional: description
description = "What this rule does"

# Optional: enable/disable this rule (default: true)
enabled = true

# Optional: evaluation order, higher = first (default: 0)
priority = 100

# Required: match conditions (all must match)
[rules.rules.match]
# ...match conditions...

# Required: action to take when rule matches
[rules.rules.action]
# ...action configuration...
```

## Pattern matching

The rule engine detects the pattern type automatically:

### Glob patterns

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

Glob syntax:

- `*` - matches any sequence of characters (except path separator)
- `**` - matches any sequence of characters including path separators
- `?` - matches any single character
- `[abc]` - matches any character in the set
- `{a,b}` - matches either pattern

### Regex patterns

Detected when the pattern contains regex indicators:

```toml
# Match semantic version branches
branch_pattern = "^release/v[0-9]+\\.[0-9]+$"

# Match ticket references in commands
command_pattern = ".*JIRA-[0-9]+.*"

# Match specific file extensions
file_pattern = ".*\\.(tf|tfvars)$"
```

Regex indicators (trigger regex mode):

- `^` `$` - anchors
- `(?` - non-capturing groups
- `\\d` `\\w` `\\s` - character classes
- `[` `]` - character classes
- `(` `)` - groups
- `|` - alternation
- `+` `.+` `.*` - quantifiers

## Match conditions

All non-empty conditions must match (AND logic).

### validator_type

Filter by validator:

```toml
# Specific validator
validator_type = "git.push"

# All git validators
validator_type = "git.*"

# All validators
validator_type = "*"
```

### repo_pattern

Match against the repository root path:

```toml
# Match organization repositories
repo_pattern = "**/myorg/**"

# Match specific project
repo_pattern = "**/my-project"
```

### remote

Exact match against git remote name:

```toml
# Match origin remote
remote = "origin"

# Match upstream remote
remote = "upstream"
```

### branch_pattern

Match against branch name:

```toml
# Match main branch
branch_pattern = "main"

# Match feature branches
branch_pattern = "feat/*"

# Match release branches (regex)
branch_pattern = "^release/v[0-9]+$"
```

### file_pattern

Match against file path:

```toml
# Match test files
file_pattern = "**/test/**"

# Match terraform files
file_pattern = "*.tf"

# Match workflow files
file_pattern = ".github/workflows/*.yml"
```

### content_pattern

Match against file content (always regex):

```toml
# Match files containing secrets
content_pattern = "(?i)password\\s*=\\s*[\"'][^\"']+[\"']"

# Match TODO comments
content_pattern = "TODO|FIXME|HACK"
```

### command_pattern

Match against bash command:

```toml
# Match git push commands
command_pattern = "git push*"

# Match force operations
command_pattern = "*--force*"

# Match dangerous rm commands (regex)
command_pattern = "rm\\s+-rf\\s+/"
```

### tool_type and event_type (hook context)

Match against the hook context:

```toml
# Match bash commands
tool_type = "Bash"

# Match file writes
tool_type = "Write"

# Match pre-execution hook
event_type = "PreToolUse"
```

## Actions

### block

Stops the operation with an error:

```toml
[rules.rules.action]
type = "block"
message = "This operation is not allowed"
reference = "RULE001"  # Optional error code
```

### warn

Logs a warning but allows the operation:

```toml
[rules.rules.action]
type = "warn"
message = "This operation might cause issues"
```

### allow

Allows the operation outright, skipping further rules and built-in validation:

```toml
[rules.rules.action]
type = "allow"
message = "Operation allowed by rule"  # Optional
```

## Configuration precedence

Rules load and merge from multiple sources:

1. CLI flags (highest priority)
2. Environment variables (`KLAUDIUSH_*`)
3. Project config (`.klaudiush/config.toml`)
4. Global config (`~/.klaudiush/config.toml`)
5. Defaults (lowest priority)

### Rule merge behavior

When rules exist in both project and global config, rules with the same name merge (project wins), and rules with different names are combined.

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

### Evaluation order

Rules evaluate by priority, highest first:

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

## Validator types

### Git validators

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

### File validators

| Type             | Description               |
|:-----------------|:--------------------------|
| `file.markdown`  | Markdown file validation  |
| `file.shell`     | Shell script validation   |
| `file.terraform` | Terraform file validation |
| `file.workflow`  | GitHub Actions workflow   |
| `file.*`         | All file validators       |

### Other validators

| Type                | Description                |
|:--------------------|:---------------------------|
| `secrets.secrets`   | Secrets detection          |
| `shell.backtick`    | Backtick command injection |
| `notification.bell` | Terminal notifications     |
| `*`                 | All validators             |

## Examples

### Block direct pushes to main

```toml
[[rules.rules]]
name = "block-main-push"
description = "Block direct pushes to main"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Direct push to main is not allowed. Use a pull request instead."
reference = "GIT019"
```

### Allow test file secrets

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

### Warn on upstream push

```toml
[[rules.rules]]
name = "warn-upstream-push"
description = "Flag pushes to upstream remote"
priority = 50

[rules.rules.match]
validator_type = "git.push"
remote = "upstream"

[rules.rules.action]
type = "warn"
message = "Pushing to upstream remote. Confirm this is intentional."
```

### Require ticket reference

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

### Strict docs formatting

```toml
[[rules.rules]]
name = "strict-docs"
description = "Require strict markdown formatting in docs"
priority = 100

[rules.rules.match]
validator_type = "file.markdown"
file_pattern = "docs/**/*.md"

[rules.rules.action]
type = "block"
message = "Documentation files must pass markdown validation"
```

### Organization rules

```toml
# Block pushing to origin in organization repositories (forks)
[[rules.rules]]
name = "block-org-origin"
description = "Block push to origin in org repos"
priority = 100

[rules.rules.match]
validator_type = "git.push"
repo_pattern = "**/myorg/**"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Organization repos use 'upstream' for main repository. Push to your fork."
```

### Disable a rule

```toml
[[rules.rules]]
name = "block-org-origin"
enabled = false  # Disable this rule
```

## Exceptions integration

Rules work with the exception workflow. When a rule blocks an operation, an exception token can bypass the block if configured.

### How it works

1. Rule evaluates and returns block/warn/allow
2. On block, klaudiush checks for an exception token
3. If the token is present and valid, the block becomes a warning
4. Exception usage is recorded in the audit log

### Allowing exceptions for rule blocks

For a rule block to support exceptions, the rule needs a `reference`:

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

### Bypassing with an exception token

When a rule blocks with a reference, include an exception token to bypass it:

```bash
# In shell comment
git push origin main  # EXC:RULE001:Emergency+hotfix+approved+by+lead

# Via environment variable
KLACK="EXC:RULE001:Hotfix+deployment" git push origin main
```

See [EXCEPTIONS_GUIDE.md](EXCEPTIONS_GUIDE.md) for exception configuration details.

## Troubleshooting

### Rule not matching

1. Check pattern type -- confirm glob vs regex is correctly detected
2. Check all conditions -- all non-empty conditions must match
3. Check priority -- higher priority rules evaluate first
4. Enable debug logging: `klaudiush --debug`

### Rule conflicts

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

### Config not loading

1. Check file location: `.klaudiush/config.toml` (project) or `~/.klaudiush/config.toml` (global)
2. Check TOML syntax: validate with a `toml` linter or online validator
3. Check permissions: the file must be readable

### Pattern debugging

Test patterns in isolation:

```go
// Glob pattern test
glob.Compile("**/myorg/**", '/')

// Regex pattern test
regexp.Compile("^release/v[0-9]+$")
```

### Common mistakes

1. Missing separator in glob: use `/` as path separator
   - Wrong: `**\test\**`
   - Correct: `**/test/**`

2. Unescaped regex characters: escape special characters
   - Wrong: `.*\.tf`
   - Correct: `.*\\.tf`

3. Wrong pattern type: check if pattern is detected as glob or regex
   - `feat/*` -> glob
   - `feat/.*` -> regex (due to `.*`)

4. Missing quotes: TOML strings need quotes
   - Wrong: `pattern = **/test/**`
   - Correct: `pattern = "**/test/**"`
