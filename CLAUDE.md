# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`klaudiush` is a validation dispatcher for Claude Code hooks. It intercepts tool invocations (PreToolUse events) and validates commands before execution, enforcing git workflow standards and commit message conventions.

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

1. **CLI Entry** (`cmd/klaudiush/main.go`): Receives JSON from stdin, parses `--hook-type` flag
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
3. Register with predicate in `cmd/klaudiush/main.go:registerValidators()`

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

- **MarkdownValidator**: Validates Markdown format conventions (uses `MarkdownLinter`)
- **ShellScriptValidator**: Validates shell scripts with shellcheck (uses `ShellChecker`)
- **TerraformValidator**: Validates Terraform/OpenTofu formatting and linting (uses `TerraformFormatter`, `TfLinter`)
- **WorkflowValidator**: Validates GitHub Actions workflows (uses `ActionLinter`)

**Notification Validators** (`internal/validators/notification/`):

- **BellValidator**: Sends bell character (ASCII 7) to `/dev/tty` for all notification events (permission prompts, etc.) to trigger terminal bell/dock bounce

### Linter Abstractions

**Linter Package** (`internal/linters/`):

Provides typed interfaces for external linting tools:

- **ShellChecker** (`shellcheck.go`): Shell script validation via shellcheck
  - Returns structured `LintResult` with findings
  - Handles tool availability checking

- **TerraformFormatter** (`terraform.go`): Terraform/OpenTofu formatting validation
  - Detects `tofu` vs `terraform` binary
  - Parses `fmt -check -diff` output
  - Returns file-level formatting issues

- **TfLinter** (`tflint.go`): Terraform linting via tflint
  - Uses `--format=compact` for structured output
  - Distinguishes between findings and errors

- **ActionLinter** (`actionlint.go`): GitHub Actions workflow validation
  - Parses `file:line:col: message` format
  - Returns structured findings with location data

- **MarkdownLinter** (`markdownlint.go`): Markdown validation
  - Custom rules via `AnalyzeMarkdown()` function
  - Checks for proper heading spacing, etc.

**Common Types** (`result.go`):

- `LintResult`: Structured result with success/findings
- `LintFinding`: Individual issue with file/line/message
- `LintSeverity`: Error, Warning, Info levels

**Benefits**:

- Type-safe linter invocations
- Testable via interface mocking
- Consistent error handling
- Foundation for parallel execution and caching

### Git Operations

**Git SDK Architecture** (`internal/git/`):

The project uses a dual implementation strategy for git operations:

- **SDK Implementation** (default in future): Uses `go-git/go-git/v6` for native Go git operations
  - `Repository` interface (`repository.go`): Core git repository operations
  - `SDKRepository`: Implementation using go-git SDK
  - `DiscoverRepository()`: Finds and caches repository instance
  - Performance: 2-5.9M× faster than CLI for cached operations

- **CLI Implementation** (backward compatible): Executes git commands via shell
  - `CLIGitRunner`: Uses `exec.CommandRunner` to execute git CLI
  - Fallback when SDK initialization fails

**Runner Interface** (`internal/git/runner.go`):

Unified interface implemented by both CLI and SDK:

- Operations: `IsInRepo()`, `GetStagedFiles()`, `GetModifiedFiles()`, `GetUntrackedFiles()`
- Repository info: `GetRepoRoot()`, `GetCurrentBranch()`, `GetBranchRemote()`
- Remote operations: `GetRemoteURL()`, `GetRemotes()`

**Factory Pattern** (`internal/validators/git/git_runner.go`):

```go
func NewGitRunner() GitRunner {
    // By default, uses SDK implementation for better performance
    // Set CLAUDE_HOOKS_USE_SDK_GIT to "false" or "0" to use CLI
    // Falls back to CLI if SDK initialization fails
}
```

**Environment Variable**:

- Not set or `CLAUDE_HOOKS_USE_SDK_GIT=true`: Use SDK implementation (default)
- `CLAUDE_HOOKS_USE_SDK_GIT=false` or `CLAUDE_HOOKS_USE_SDK_GIT=0`: Use CLI implementation

**Adapter Pattern** (`internal/git/adapter.go`):

- `RepositoryAdapter`: Wraps `Repository` to implement `Runner` interface
- Provides backward compatibility with existing validators
- `NewSDKRunner()`: Creates SDK-backed runner instance

**Testing**:

- `MockGitRunner`: For testing validators with controlled git state
- All 169 git validator tests pass with both implementations

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
go build -ldflags="-X 'github.com/smykla-labs/klaudiush/internal/validators/git.ExpectedSignoff=Name <email>'" ./cmd/klaudiush
```

This enforces exact signoff match in commit messages when using `task build:prod`.

## Exit Codes

- `0`: Operation allowed (validation passed or no validators matched)
- `2`: Operation blocked (validation failed with `ShouldBlock=true`)

Warnings (`ShouldBlock=false`) print to stderr but allow operation (exit 0).
