# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`claude-hooks` is a validation dispatcher for Claude Code hooks. It intercepts tool invocations (PreToolUse events) and validates commands before execution, enforcing git workflow standards and commit message conventions.

## Commands

### Build

```bash
# Development build (no signoff validation)
task build

# Production build (validates signoff matches git config)
task build:prod

# Install to ~/.claude/hooks/dispatcher
task install
```

### Testing

```bash
# Run all tests
task test

# Unit tests only
task test:unit

# Integration tests only
task test:integration
```

### Linting

```bash
# Lint and auto-fix
task check
task lint:fix

# Lint only
task lint
```

### Development

```bash
# Format code
task fmt

# Clean build artifacts
task clean

# Update dependencies
task deps

# Run all verification (fmt + lint + test)
task verify
```

## Architecture

### Core Flow

1. **CLI Entry** (`cmd/claude-hooks/main.go`): Receives JSON from stdin, parses `--hook-type` flag
2. **JSON Parser** (`internal/parser/json.go`): Converts JSON to `hook.Context`
3. **Dispatcher** (`internal/dispatcher/dispatcher.go`): Orchestrates validation
4. **Registry** (`internal/validator/registry.go`): Matches validators to context using predicates
5. **Validators**: Execute validation logic, return `Result` (Pass/Fail/Warn)

### Execution Abstractions

**Command Execution Package** (`internal/exec/`):

Provides unified abstractions for external command execution, eliminating code duplication across validators.

- **CommandRunner**: Executes commands with timeout/context management
  - `Run(ctx, name, args...)`: Execute command and return result
  - `RunWithStdin(ctx, stdin, name, args...)`: Execute with stdin input
  - Returns `CommandResult` with stdout, stderr, exit code
  - Automatic `ExitError` handling with `errors.As`

- **ToolChecker**: Checks tool availability in PATH
  - `IsAvailable(tool)`: Check if single tool exists
  - `FindTool(tools...)`: Find first available tool from list
  - Used for tool detection (e.g., `tofu` vs `terraform`)

- **TempFileManager**: Manages temporary file lifecycle
  - `Create(pattern, content)`: Create temp file with content
  - `Cleanup(path)`: Remove temp file
  - Uses system temp directory (`os.TempDir()`)

**Benefits**:

- ~134 lines of boilerplate eliminated across validators
- Consistent error handling with `errors.As`
- Single source of truth for command execution
- Better testability with mockable interfaces
- Prevents nil pointer dereferences

**Migrated Validators**:

- ✅ `git_runner.go` (8 methods) - eliminated ~60 lines
- ✅ `shellscript.go` - eliminated ~30 lines
- ✅ `terraform.go` - eliminated ~44 lines

### Hook Context

The `hook.Context` struct (`pkg/hook/context.go`) represents tool invocations:

- `EventType`: PreToolUse, PostToolUse, Notification
- `ToolName`: Bash, Write, Edit, Grep, etc.
- `ToolInput`: Command, FilePath, Content, etc.

### Validator System

**Predicate-based Registration** (`internal/validator/registry.go`):

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

**Validation Results** (`internal/validator/validator.go`):

- `Pass()`: Validation passed
- `Fail(msg)`: Validation failed, blocks operation (exit code 2)
- `Warn(msg)`: Validation failed, logs warning but allows operation

**Creating Validators**:

1. Embed `validator.BaseValidator` for logging/naming
2. Implement `Validate(ctx *hook.Context) *validator.Result`
3. Register with predicate in `cmd/claude-hooks/main.go:registerValidators()`

### Parsers

**Bash Parser** (`pkg/parser/bash.go`):

- Uses `mvdan.cc/sh/v3/syntax` for AST parsing
- Extracts commands, file writes, git operations
- Returns `ParseResult` with all parsed commands

**Git Parser** (`pkg/parser/git.go`):

- Parses `Command` into `GitCommand` struct
- Handles combined flags (`-sS` → `["-s", "-S"]`)
- Extracts: commit messages, remotes, branches, file paths
- `HasFlag()` checks both standalone and combined flags

### Validators

**Git Validators** (`internal/validators/git/`):

- **AddValidator**: Validates `git add` commands (file existence, patterns)
- **CommitValidator**: Validates commit flags (`-sS`), staging area, message format
- **PushValidator**: Validates remote exists, branch tracking
- **PRValidator**: Validates PR title, body format, changelog

**Commit Message Validation** (`internal/validators/git/commit_message.go`):

- Conventional commits format: `type(scope): description`
- Title ≤50 chars, body lines ≤72 chars (77 with tolerance)
- Blocks `feat(ci)`, `fix(test)` - infrastructure changes should use `ci(...)`, `test(...)`
- No PR references (`#123` or GitHub URLs)
- No Claude AI attribution
- Signoff validation when built with `-ldflags` setting `ExpectedSignoff`

**File Validators** (`internal/validators/file/`):

- **MarkdownValidator**: Validates Markdown format conventions
- **ShellScriptValidator**: Validates shell scripts with shellcheck (uses `internal/exec`)
- **TerraformValidator**: Validates Terraform/OpenTofu formatting and linting (uses `internal/exec`)

### Git Operations

**GitRunner Interface** (`internal/validators/git/git_runner.go`):

- Abstracts git commands for testing (uses `internal/exec` for execution)
- `RealGitRunner`: Executes actual git commands
- `MockGitRunner`: For testing validators
- Operations: staged files, modified files, untracked files, remote validation

### Logging

All validators log to `~/.claude/hooks/dispatcher.log`:

- Debug mode: enabled by default (`--debug`)
- Trace mode: `--trace` for verbose output
- Use `BaseValidator.Logger()` for structured logging

## Testing

- **Framework**: Ginkgo/Gomega
- **Mocks**: `git_runner_mock.go` for git operations
- **Test files**: `*_test.go`, `*_suite_test.go` for Ginkgo suites
- **Coverage**: 336 tests across all packages
- Run single test: `mise exec -- go test -v ./pkg/parser -run TestBashParser`

## Development Environment

**Tool Version Management**:

- Uses [mise](https://mise.jdx.dev/) for consistent tool versions
- Go 1.25.4 (latest stable as of 2025-11-23)
- golangci-lint 2.6.2 (latest version)
- Run `mise install` to install pinned versions
- See `SETUP.md` for detailed setup instructions

**Linting**:

- Comprehensive linter configuration in `.golangci.yml`
- Nil safety checks: nilnesserr, govet (nilness), staticcheck
- Completeness checks: exhaustive, gochecksumtype
- Code quality: gocognit, goconst, cyclop, dupl
- All linters pass with 0 issues

## Build-time Configuration

**Signoff Validation**:

```bash
go build -ldflags="-X 'github.com/smykla-labs/claude-hooks/internal/validators/git.ExpectedSignoff=Name <email>'" ./cmd/claude-hooks
```

This enforces exact signoff match in commit messages when using `task build:prod`.

## Exit Codes

- `0`: Operation allowed (validation passed or no validators matched)
- `2`: Operation blocked (validation failed with `ShouldBlock=true`)

Warnings (`ShouldBlock=false`) print to stderr but allow operation (exit 0).
