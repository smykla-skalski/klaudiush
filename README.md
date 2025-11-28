# Klaudiush

[![CI](https://github.com/smykla-labs/klaudiush/actions/workflows/ci.yml/badge.svg)](https://github.com/smykla-labs/klaudiush/actions/workflows/ci.yml)
[![CodeQL](https://github.com/smykla-labs/klaudiush/actions/workflows/codeql.yml/badge.svg)](https://github.com/smykla-labs/klaudiush/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/smykla-labs/klaudiush/badge)](https://scorecard.dev/viewer/?uri=github.com/smykla-labs/klaudiush)
[![Go Report Card](https://goreportcard.com/badge/github.com/smykla-labs/klaudiush)](https://goreportcard.com/report/github.com/smykla-labs/klaudiush)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/smykla-labs/klaudiush)](https://github.com/smykla-labs/klaudiush/releases/latest)

A validation dispatcher for Claude Code hooks that intercepts tool invocations and enforces git workflow standards, commit message conventions, and code quality rules.

## Overview

Klaudiush is a Go-based validation system that runs as a PreToolUse hook in Claude Code. It parses commands using advanced Bash parsing (via `mvdan.cc/sh`), detects file operations, and validates them against project-specific rules before execution.

**Key Features:**

- **Git Workflow Validation**: Enforce commit message format, flag requirements, and push policies
- **Code Quality Checks**: Run shellcheck, markdownlint, terraform fmt, and actionlint
- **Advanced Command Parsing**: Handle command chains (&&, ||, ;), pipes, subshells, and redirections
- **File Write Detection**: Detect and validate file writes via redirections, tee, cp, mv
- **Protected Path Prevention**: Block writes to /tmp, suggest project-local tmp/
- **Dynamic Validation Rules**: Configure validation behavior via TOML without code changes

## Installation

### Quick Install (Recommended)

```bash
# Install latest release
curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh

# Install specific version
curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh -s -- -v v1.0.0

# Install to custom directory
curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh -s -- -b /usr/local/bin
```

### Build from Source

```bash
# Build the binary (development build)
task build

# Build production binary
task build:prod

# Install to ~/.local/bin or ~/bin
task install
```

### Configure Claude Code

After installation, update `~/.claude/settings.json` to use the `klaudiush` command:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "klaudiush --hook-type PreToolUse"
          }
        ]
      },
      {
        "matcher": "Write|Edit|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "klaudiush --hook-type PreToolUse",
            "timeout": 30
          }
        ]
      }
    ],
    "Notification": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "klaudiush --hook-type Notification"
          }
        ]
      }
    ]
  }
}
```

**Note**: After installation, the binary is available as `klaudiush` (installed to `~/.local/bin` or `~/bin`). Ensure the install directory is in your `$PATH`.

**Important**: File validators use **PreToolUse** to block invalid writes **before** they happen, not PostToolUse which only validates after the file is written.

## Commands

### Build

```bash
task build       # Development build
task build:prod  # Production build with signoff validation
task install     # Install to ~/.claude/hooks/
```

### Testing

```bash
task test              # Run all tests (439 specs)
task test:unit         # Unit tests only
task test:integration  # Integration tests only
task test:staged       # Test packages with staged files
```

### Code Quality

```bash
task check        # Lint and auto-fix
task lint         # Lint only (67 linters enabled)
task lint:fix     # Lint with auto-fix
task lint:staged  # Lint only modified and staged files
task fmt          # Format code
task verify       # Run fmt + lint + test
```

### Git Hooks

```bash
task install:hooks  # Install pre-commit and pre-push hooks
```

The project uses [Lefthook](https://github.com/evilmartians/lefthook) for git hook management.

**Pre-commit hook** runs before each commit (parallel):

- Lints only modified and staged files
- Tests only packages with changes

**Pre-push hook** runs before each push (parallel):

- Full linting of entire codebase
- Full test suite

Install hooks:

```bash
task install:hooks
```

### Other

```bash
task deps   # Download dependencies
task clean  # Remove build artifacts
```

## Architecture

### Core Flow

```text
Claude Code JSON → CLI → JSON Parser → Dispatcher → Registry → Validators → Result
```

1. **CLI Entry** (`cmd/klaudiush/main.go`): Receives JSON from stdin, parses `--hook-type` flag
2. **JSON Parser** (`internal/parser/json.go`): Converts JSON to `hook.Context`
3. **Dispatcher** (`internal/dispatcher/dispatcher.go`): Orchestrates validation
4. **Registry** (`internal/validator/registry.go`): Matches validators to context using predicates
5. **Validators**: Execute validation logic, return `Result` (Pass/Fail/Warn)

### Directory Structure

```text
klaudiush/
├── cmd/klaudiush/           # CLI entry point
├── pkg/
│   ├── hook/                   # Event types, Context
│   ├── parser/                 # Bash/Git/command parsing
│   └── logger/                 # Structured logging
└── internal/
    ├── dispatcher/             # Validation orchestration
    ├── validator/              # Validator interface, registry
    ├── exec/                   # Command execution helpers
    ├── git/                    # Git SDK implementation
    ├── github/                 # GitHub API client
    ├── linters/                # Linter abstractions
    ├── rules/                  # Dynamic validation rules engine
    ├── templates/              # Error messages
    └── validators/             # Git, file, notification validators
```

## Validators

### Git Validators

- **GitAddValidator**: Blocks staging files in `tmp/` directory, suggests adding to `.git/info/exclude`
- **CommitValidator**: Requires `-sS` flags, validates conventional commit format (≤50 char title, ≤72 char body), blocks `feat(ci)`/`fix(test)`, no PR refs or "Claude" mentions, checks forbidden patterns (default: blocks `tmp/` and `tmp` word)
- **PushValidator**: Validates remote existence with configurable rules
- **BranchValidator**: Enforces `type/description` format (lowercase, no spaces). Valid types: feat, fix, docs, style, refactor, test, chore, ci, build, perf
- **PRValidator**: Validates PR title (semantic format, blocks `feat(ci)`/`fix(test)`), body (template sections, changelog rules, no formal language), Markdown formatting, suggests CI labels, checks forbidden patterns (default: blocks `tmp/` and `tmp` word)

### File Validators

- **MarkdownValidator**: Validates Markdown formatting: empty lines after headers and before lists/code blocks, proper code block indentation in lists
- **ShellScriptValidator**: Runs shellcheck on `*.sh`/`*.bash` files (skips Fish scripts, 10s timeout)
- **TerraformValidator**: Validates `*.tf` files with `terraform`/`tofu` fmt and tflint
- **WorkflowValidator**: Enforces digest pinning for GitHub Actions with version comments, checks for latest versions via GitHub API, runs actionlint

### Notification Validators

- **BellValidator**: Sends bell character to `/dev/tty` for all notification events (permission prompts, etc.)

## Predicate System

Validators are registered with predicates that determine when they run:

```go
registry.Register(
    validator,
    validator.And(
        validator.EventTypeIs(hook.PreToolUse),
        validator.ToolTypeIs(hook.Bash),
        validator.CommandContains("git commit"),
    ),
)
```

**Common Predicates**:

- `EventTypeIs(eventType)`: Match event type (PreToolUse, PostToolUse, Notification)
- `ToolTypeIs(toolType)`: Match tool type (Bash, Write, Edit, etc.)
- `CommandContains(substring)`: Match command substring
- `FileExtensionIs(ext)`: Match file extension
- `FilePathMatches(pattern)`: Match file path pattern
- `And(predicates...)`: All predicates must match
- `Or(predicates...)`: Any predicate must match
- `Not(predicate)`: Predicate must not match

## Bash Parsing

Uses `mvdan.cc/sh` for production-grade parsing supporting command chains, pipes, subshells, redirections, and heredocs. Detects file writes via redirections (`>`, `>>`), `tee`, `cp`, and `mv`. Blocks writes to `/tmp` and suggests project-local `tmp/` directory.

## Development

**Adding Validators**: Create validator in `internal/validators/{category}/`, implement `Validate()` method, write tests, register in `cmd/klaudiush/main.go` with predicates.

**Testing**: Use `task test` for all tests, `go test -v ./pkg/parser` for specific suites. Logs in `~/.claude/hooks/dispatcher.log`.

**Debugging**: `tail -f ~/.claude/hooks/dispatcher.log` to follow logs in real-time.

## Exit Codes

- `0`: Operation allowed (validation passed or no validators matched)
- `2`: Operation blocked (validation failed with `ShouldBlock=true`)

Warnings (`ShouldBlock=false`) print to stderr but allow operation (exit 0).

## Configuration

Klaudiush supports flexible configuration through multiple sources with a clear precedence hierarchy. All validators are fully configurable - you can enable/disable them, change severity levels, and customize individual rules.

### Interactive Setup

The quickest way to get started is using the interactive `init` command:

```bash
# Initialize project configuration (interactive)
./bin/klaudiush init

# Initialize global configuration
./bin/klaudiush init --global

# Force overwrite existing configuration
./bin/klaudiush init --force
```

The `init` command guides you through configuration options with sensible defaults from your git config.

### Configuration Files

Klaudiush uses TOML configuration files:

**Global Configuration**: `~/.klaudiush/config.toml`

- Applies to all projects
- Set your default preferences

**Project Configuration**:

- `.klaudiush/config.toml` (preferred)
- `klaudiush.toml` (alternative)
- Overrides global settings
- Committed to repository for team standards

**No Configuration Required**: All validators work with sensible defaults if no config files exist.

### Configuration Hierarchy

Configuration sources are merged with the following precedence (highest to lowest):

1. **CLI Flags** - Runtime overrides (e.g., `--disable=commit,markdown`)
2. **Environment Variables** - Shell-level config (e.g., `KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false`)
3. **Project Config** - Repository-specific settings
4. **Global Config** - User-wide defaults
5. **Built-in Defaults** - Sensible defaults matching current behavior

Settings from higher precedence sources override lower ones using deep merge (nested values are merged, not replaced entirely).

### CLI Flags

Override configuration at runtime:

```bash
# Use custom config file
klaudiush --config=./my-config.toml --hook-type PreToolUse

# Use custom global config
klaudiush --global-config=~/.config/klaudiush.toml --hook-type PreToolUse

# Disable specific validators
klaudiush --disable=commit,markdown --hook-type PreToolUse

# Debug mode (enabled by default)
klaudiush --hook-type PreToolUse --debug

# Trace mode (verbose logging)
klaudiush --hook-type PreToolUse --trace
```

### Environment Variables

All environment variables use the `KLAUDIUSH_` prefix:

```bash
# Disable specific validator
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false

# Change commit title max length
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH=72

# Disable Markdown validation
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED=false

# Git SDK configuration (default: true)
export KLAUDIUSH_USE_SDK_GIT=false  # Use CLI instead of SDK
```

**Git SDK Performance**:

The project supports two git operation implementations:

- **SDK Implementation** (default): Native Go using `go-git/go-git/v6`
  - 2-5.9M× faster for cached operations (`IsInRepo`, `GetRepoRoot`)
  - 177× faster for `GetCurrentBranch`
  - 1.5× faster for `GetStagedFiles`
  - Used by default, no configuration needed

- **CLI Implementation**: Executes git commands via shell
  - Fully tested fallback implementation
  - Automatic fallback if SDK initialization fails
  - Opt-in with `KLAUDIUSH_USE_SDK_GIT=false` or `KLAUDIUSH_USE_SDK_GIT=0`

### Example Configurations

Complete examples with all options are available in [`examples/config/`](examples/config/):

- **[full.toml](examples/config/full.toml)** - All available options with defaults
- **[minimal.toml](examples/config/minimal.toml)** - Quick setup to disable validators
- **[project-override.toml](examples/config/project-override.toml)** - Project-specific customization

**Quick Start Examples**:

```toml
# Disable commit message validation
[validators.git.commit]
enabled = false

# Allow longer commit titles
[validators.git.commit.message]
title_max_length = 72

# Disable conventional commit checking
check_conventional_commits = false

# Set custom signoff
expected_signoff = "Your Name <your.email@klaudiu.sh>"

# Disable Markdown validation
[validators.file.markdown]
enabled = false

# Change validator severity to warning
[validators.file.shellscript]
severity = "warning"

# Increase timeout for Terraform operations
[validators.file.terraform]
timeout = "30s"
```

See the [examples/config/README.md](examples/config/README.md) for complete documentation and more examples.

### What's Configurable

**All validators** support:

- `enabled` - Enable/disable entire validator
- `severity` - "error" (block) or "warning" (log only)

**Git validators** support additional options:

- Commit: required flags, staging checks, message format rules
- PR: title format, changelog requirements, CI labels
- Branch: protected branches, naming patterns
- Add: blocked file patterns
- Push: remote restrictions

**File validators** support:

- Timeouts for linter operations
- Enable/disable specific linters
- Context lines for error messages
- Linter-specific rules (shellcheck, tflint, actionlint)

**Notification validators** support:

- Custom notification commands

See [`examples/config/full.toml`](examples/config/full.toml) for the complete list of options.

## Dynamic Validation Rules

Configure validators dynamically without modifying code. The rule engine allows you to:

- Block operations based on patterns (repository, branch, file, command)
- Warn about potentially dangerous operations
- Allow operations that would otherwise be blocked
- Apply different validation logic per validator type

### Quick Example

```toml
# .klaudiush/config.toml
[rules]
enabled = true

# Block direct pushes to main branch
[[rules.rules]]
name = "block-main-push"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"

[rules.rules.action]
type = "block"
message = "Direct push to main is not allowed. Use a pull request."
```

### Rule Features

| Feature           | Description                                                      |
|:------------------|:-----------------------------------------------------------------|
| Pattern Matching  | Auto-detect glob (`feat/*`) or regex (`^release/v[0-9]+$`)       |
| Priority System   | Higher priority rules evaluate first                             |
| Config Precedence | Project config overrides global config                           |
| Validator Scoping | Apply rules to specific (`git.push`) or all (`git.*`) validators |
| Advanced Patterns | Negation (`!*.tmp`), case-insensitive, multi-patterns            |

### Debug Rules

Inspect loaded rules with the debug command:

```bash
# Show all rules
klaudiush debug rules

# Filter by validator
klaudiush debug rules --validator git.push
```

### Examples

Example configurations are available in [`examples/rules/`](examples/rules/):

- **[organization.toml](examples/rules/organization.toml)** - Remote restrictions, branch protection
- **[secrets-allow-list.toml](examples/rules/secrets-allow-list.toml)** - Allow list for test fixtures
- **[advanced-patterns.toml](examples/rules/advanced-patterns.toml)** - Complex pattern matching

See the [Rules Guide](docs/RULES_GUIDE.md) for comprehensive documentation.

## Performance

- **Cold start**: <100ms target
- **Parser**: <100µs for typical commands
- **Validators**: <50ms each (I/O dependent)
- **Total**: <500ms for full validation chain
- **Rule evaluation**: <1ms per rule (155ns-10.7µs achieved)

## Contributing

1. Create feature branch: `git checkout -b feat/my-feature`
2. Write tests first: `task test`
3. Implement changes
4. Ensure quality: `task verify`
5. Create PR with semantic title

## Support

- **Issues**: [GitHub Issues](https://github.com/smykla-labs/klaudiush/issues)
- **Discussions**: [GitHub Discussions](https://github.com/smykla-labs/klaudiush/discussions)
- **Logs**: `~/.claude/hooks/dispatcher.log`

## License

MIT License - Copyright © 2025 Bart Smykla

See [LICENSE](LICENSE) for details.
