# GitHub Copilot Instructions

This file provides guidance to GitHub Copilot when working with code in this repository.

## Project Overview

`klaudiush` is a validation dispatcher for Claude Code hooks. It intercepts tool invocations (PreToolUse events) and validates commands before execution, enforcing git workflow standards and commit message conventions.

## Commands

### Build

```bash
# Development build (no signoff validation)
mise run build

# Production build (validates signoff matches git config)
mise run build:prod

# Install to ~/.claude/hooks/dispatcher
mise run install
```

### Testing

```bash
# Run all tests
mise run test

# Unit tests only
mise run test:unit

# Integration tests only
mise run test:integration
```

### Linting

```bash
# Lint and auto-fix
mise run check
mise run lint:fix

# Lint only
mise run lint
```

### Development

```bash
# Format code
mise run fmt

# Clean build artifacts
mise run clean

# Update dependencies
mise run deps

# Run all verification (fmt + lint + test)
mise run verify
```

**Before Committing Code:**

Always run `mise run lint` and `mise run test` to ensure code quality and test coverage.

## Architecture

### Core Flow

1. **CLI Entry** (`cmd/klaudiush/main.go`): Receives JSON from stdin, parses `--hook-type` flag
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
- `Fail(msg)`: Validation failed, blocks operation (JSON deny on stdout)
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

## Hook output

klaudiush always exits 0 and writes structured JSON to stdout:

- **Validation blocked**: `permissionDecision: "deny"` with error details in `permissionDecisionReason` and human-readable output in `systemMessage`
- **Warnings only**: `permissionDecision: "allow"` with warning in `additionalContext`
- **Clean pass**: No output, exit 0
- **Crash**: Exit 3 with panic info on stderr (no JSON)

## Project Structure

### Temporary Files

**During Development/Testing:**

- Use `tmp/` directory in project root for temporary files
- Never use `/tmp` or system temp directories during development
- The `tmp/` directory is already in `.gitignore`
- Examples: test artifacts, analysis files, scratch work

**In Production Code (validators running on user machines):**

- Use system temp directory via `os.TempDir()`
- The dispatcher runs on user machines without project directory
- Example: terraform.go, workflow.go create temp files for validation

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

