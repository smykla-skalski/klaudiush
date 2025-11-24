# Klaudiush

A validation dispatcher for Claude Code hooks that intercepts tool invocations and enforces git workflow standards, commit message conventions, and code quality rules.

## Overview

Klaudiush is a Go-based validation system that runs as a PreToolUse hook in Claude Code. It parses commands using advanced Bash parsing (via `mvdan.cc/sh`), detects file operations, and validates them against project-specific rules before execution.

**Key Features:**

- **Git Workflow Validation**: Enforce commit message format, flag requirements, and push policies
- **Code Quality Checks**: Run shellcheck, markdownlint, terraform fmt, and actionlint
- **Advanced Command Parsing**: Handle command chains (&&, ||, ;), pipes, subshells, and redirections
- **File Write Detection**: Detect and validate file writes via redirections, tee, cp, mv
- **Protected Path Prevention**: Block writes to /tmp, suggest project-local tmp/
- **Project-Specific Rules**: Kong and Kuma repository-specific validations

## Installation

### Build and Install

```bash
# Build the binary (development build, no signoff validation)
task build

# Build with signoff validation (uses git config for expected signoff)
task build:prod

# Install to ~/.claude/hooks/
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

The project includes two git hooks for quality assurance:

**Pre-commit hook** runs before each commit:
- `task lint:staged` - Lints only modified and staged files
- `task test:staged` - Tests only packages with changes

**Pre-push hook** runs before each push:
- `task lint` - Full linting of entire codebase
- `task test` - Full test suite

To bypass hooks (not recommended), use:
```bash
git commit --no-verify    # Skip pre-commit hook
git push --no-verify      # Skip pre-push hook
```

### Other

```bash
task deps   # Download dependencies
task clean  # Remove build artifacts
```

## Architecture

### Core Flow

```
Claude Code JSON → CLI → JSON Parser → Dispatcher → Registry → Validators → Result
```

1. **CLI Entry** (`cmd/klaudiush/main.go`): Receives JSON from stdin, parses `--hook-type` flag
2. **JSON Parser** (`internal/parser/json.go`): Converts JSON to `hook.Context`
3. **Dispatcher** (`internal/dispatcher/dispatcher.go`): Orchestrates validation
4. **Registry** (`internal/validator/registry.go`): Matches validators to context using predicates
5. **Validators**: Execute validation logic, return `Result` (Pass/Fail/Warn)

### Directory Structure

```
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
    ├── templates/              # Error messages
    └── validators/             # Git, file, notification validators
```

## Validators

### Git Validators

- **GitAddValidator**: Blocks staging files in `tmp/` directory, suggests adding to `.git/info/exclude`
- **CommitValidator**: Requires `-sS` flags, validates conventional commit format (≤50 char title, ≤72 char body), blocks `feat(ci)`/`fix(test)`, no PR refs or "Claude" mentions
- **PushValidator**: Validates remote existence with project-specific rules (Kong: requires `upstream`, kumahq/kuma: warns on `upstream`)
- **BranchValidator**: Enforces `type/description` format (lowercase, no spaces). Valid types: feat, fix, docs, style, refactor, test, chore, ci, build, perf
- **PRValidator**: Validates PR title (semantic format, blocks `feat(ci)`/`fix(test)`), body (template sections, changelog rules, no formal language), Markdown formatting, and suggests CI labels

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

### Environment Variables

**Git SDK Configuration**:

SDK is used by default for better performance. To use CLI-based operations:

```bash
export CLAUDE_HOOKS_USE_SDK_GIT=false
```

The project supports two git operation implementations:

- **SDK Implementation** (default): Native Go using `go-git/go-git/v6`
  - 2-5.9M× faster for cached operations (`IsInRepo`, `GetRepoRoot`)
  - 177× faster for `GetCurrentBranch`
  - 1.5× faster for `GetStagedFiles`
  - Used by default, no configuration needed

- **CLI Implementation**: Executes git commands via shell
  - Fully tested and backward compatible
  - Automatic fallback if SDK initialization fails
  - Opt-in with `CLAUDE_HOOKS_USE_SDK_GIT=false` or `CLAUDE_HOOKS_USE_SDK_GIT=0`

### Build-time Configuration

**Signoff Validation**:

```bash
# Build with specific signoff requirement
go build -ldflags="-X 'github.com/smykla-labs/klaudiush/internal/validators/git.ExpectedSignoff=Name <email>'" ./cmd/klaudiush

# Or use task build:prod (uses git config)
task build:prod
```

### Runtime Flags

```bash
# Debug mode (enabled by default)
klaudiush --hook-type PreToolUse --debug

# Trace mode (verbose logging)
klaudiush --hook-type PreToolUse --trace
```

## Performance

- **Cold start**: <100ms target
- **Parser**: <100µs for typical commands
- **Validators**: <50ms each (I/O dependent)
- **Total**: <500ms for full validation chain

## License

MIT License - Copyright © 2025 Bart Smykla

See [LICENSE](LICENSE) for details.

## Contributing

1. Create feature branch: `git checkout -b feat/my-feature`
2. Write tests first: `task test`
3. Implement changes
4. Ensure quality: `task verify`
5. Create PR with semantic title

## Support

- **Issues**: https://github.com/smykla-labs/klaudiush/issues
- **Discussions**: https://github.com/smykla-labs/klaudiush/discussions
- **Logs**: `~/.claude/hooks/dispatcher.log`
