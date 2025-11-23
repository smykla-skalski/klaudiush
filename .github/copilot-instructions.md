# GitHub Copilot Instructions

This file provides guidance to GitHub Copilot when working with code in this repository.

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

### Git Operations

**GitRunner Interface** (`internal/validators/git/git_runner.go`):

- Abstracts git commands for testing
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
- Run single test: `go test -v ./pkg/parser -run TestBashParser`

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

## Code Style

- Use Go 1.21+ features
- Follow standard Go formatting (gofmt, goimports)
- Write tests using Ginkgo/Gomega BDD style
- Add minimal comments only where clarification is needed
- Use structured logging via BaseValidator.Logger()

## Commit Message Standards

All commits must follow these rules:

### Format

```
type(scope): subject line (max 50 characters)

Body text wrapped at 72 characters per line.
Explain what and why, not how.

Additional paragraphs separated by blank lines.
```

### Rules

1. **Title Line (max 50 chars)**:

   - Format: `type(scope): description`
   - Scope is **required** (not optional)
   - Use lowercase for description
   - No period at the end

2. **Body (max 72 chars per line)**:

   - Wrap all lines at 72 characters
   - Separate paragraphs with blank lines
   - Explain what and why, not how

3. **Markdown Lists**:

   - Add empty line before first list item (ordered/unordered)
   - This applies to commits, PR descriptions, all markdown

4. **Valid Types**:

   - `feat`: New feature
   - `fix`: Bug fix
   - `docs`: Documentation only
   - `style`: Code style (formatting, no logic change)
   - `refactor`: Code refactoring
   - `test`: Adding/updating tests
   - `chore`: Maintenance tasks
   - `ci`: CI/CD changes
   - `build`: Build system changes
   - `perf`: Performance improvements

5. **Scope Examples**:

   - `validators`, `parser`, `git`, `templates`
   - `commit`, `pr`, `branch`, `add`, `push`
   - `copilot`, `docs`, `ci`, `test`

6. **Sign-off Required**:

   - All commits must be signed with `-sS` flags
   - Use: `git commit -sS -m "message"`

### Examples

```
feat(validators): add terraform format validation

Add validator to check terraform file formatting using
terraform fmt -check. Validates .tf files before commit.

Blocks commit if formatting issues detected.
```

```
fix(parser): handle combined flags in git commands

Parse combined flags like -sS into individual flags.
Fixes issue where HasFlag() couldn't detect combined
flags properly.
```

