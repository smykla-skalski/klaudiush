# Claude Hooks

A validation dispatcher for Claude Code hooks that intercepts tool invocations and enforces git workflow standards, commit message conventions, and code quality rules.

## Overview

Claude Hooks is a Go-based validation system that runs as a PreToolUse hook in Claude Code. It parses commands using advanced Bash parsing (via `mvdan.cc/sh`), detects file operations, and validates them against project-specific rules before execution.

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

After installation, update `~/.claude/settings.json` to use the `chook` command:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "chook -T PreToolUse"
          }
        ]
      },
      {
        "matcher": "Write|Edit|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "chook -T PreToolUse",
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
            "command": "chook -T Notification"
          }
        ]
      }
    ]
  }
}
```

**Note**: After installation, the binary is available as `chook` (installed to `~/.local/bin` or `~/bin`). Ensure the install directory is in your `$PATH`.

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

1. **CLI Entry** (`cmd/claude-hooks/main.go`): Receives JSON from stdin, parses `--hook-type` flag
2. **JSON Parser** (`internal/parser/json.go`): Converts JSON to `hook.Context`
3. **Dispatcher** (`internal/dispatcher/dispatcher.go`): Orchestrates validation
4. **Registry** (`internal/validator/registry.go`): Matches validators to context using predicates
5. **Validators**: Execute validation logic, return `Result` (Pass/Fail/Warn)

### Directory Structure

```
claude-hooks/
├── cmd/claude-hooks/
│   └── main.go                      # CLI entry point + validator registration
├── pkg/
│   ├── hook/
│   │   └── context.go               # Event types, Context struct
│   ├── parser/
│   │   ├── bash.go                  # Bash parser (mvdan.cc/sh)
│   │   ├── git.go                   # Git command parser
│   │   ├── command.go               # Command extraction
│   │   ├── files.go                 # File write detection
│   │   ├── ast_walker.go            # AST traversal utilities
│   │   └── path_validator.go       # Path validation helpers
│   └── logger/
│       └── logger.go                # Structured logging
├── internal/
│   ├── dispatcher/
│   │   └── dispatcher.go            # Validation orchestration
│   ├── parser/
│   │   └── json.go                  # JSON input parser
│   ├── validator/
│   │   ├── validator.go             # Validator interface
│   │   └── registry.go              # Predicate-based registry
│   ├── exec/                        # Command execution abstractions
│   │   ├── command.go               # CommandRunner
│   │   ├── tempfile.go              # TempFileManager
│   │   └── tool.go                  # ToolChecker
│   ├── git/                         # Git SDK implementation
│   │   ├── repository.go            # Repository interface, SDKRepository
│   │   ├── adapter.go               # RepositoryAdapter
│   │   ├── runner.go                # Runner interface
│   │   └── errors.go                # Git-specific errors
│   ├── github/                      # GitHub API client
│   │   ├── client.go                # GitHub API client
│   │   └── cache.go                 # Response cache
│   ├── linters/                     # Linter abstractions
│   │   ├── shellcheck.go            # ShellChecker
│   │   ├── terraform.go             # TerraformFormatter
│   │   ├── tflint.go                # TfLinter
│   │   ├── actionlint.go            # ActionLinter
│   │   ├── markdownlint.go          # MarkdownLinter
│   │   └── result.go                # LintResult types
│   ├── templates/                   # Error message templates
│   │   ├── git.go                   # Git error messages
│   │   ├── file.go                  # File error messages
│   │   └── templates.go             # Template utilities
│   └── validators/
│       ├── git/                     # Git validators
│       │   ├── add.go               # git add validation
│       │   ├── commit.go            # git commit validation
│       │   ├── commit_message.go    # Commit message format
│       │   ├── push.go              # git push validation
│       │   ├── branch.go            # git branch validation
│       │   ├── no_verify.go         # --no-verify flag detection
│       │   ├── pr.go                # GitHub PR validation
│       │   ├── pr_title.go          # PR title validation
│       │   ├── pr_body.go           # PR body validation
│       │   ├── pr_markdown.go       # PR Markdown validation
│       │   └── git_runner.go        # Git runner factory
│       ├── file/                    # File validators
│       │   ├── markdown.go          # Markdown format validation
│       │   ├── fragment.go          # Markdown fragment validation
│       │   ├── shellscript.go       # Shell script validation
│       │   ├── terraform.go         # Terraform validation
│       │   └── workflow.go          # GitHub workflow validation
│       ├── notification/
│       │   └── bell.go              # Notification bell handler
│       └── markdown_utils.go        # Shared Markdown utilities
├── Taskfile.yaml                    # Task definitions
└── .golangci.yml                    # Linter configuration
```

## Validators

### Git Validators

#### GitAddValidator

**Triggers**: `git add` commands

**Validates**:

- Blocks staging files in `tmp/` directory
- Suggests adding to `.git/info/exclude`
- Handles chained commands

**Example**:

```bash
git add tmp/test.txt  # ❌ Blocked
git add src/main.go   # ✅ Allowed
```

#### CommitValidator

**Triggers**: `git commit` commands

**Validates**:

- Required flags: `-s` (signoff) and `-S` (GPG sign)
- Staging area: must have files staged or use `-a`/`-A`
- Message format: conventional commits (`type(scope): description`)
- Title length: ≤50 characters
- Body line length: ≤72 characters (77 with tolerance)
- Infrastructure types: blocks `feat(ci)`, `fix(test)`, `feat(docs)`, etc.
- No PR references: `#123` or GitHub URLs
- No "Claude" references in commit message
- Signoff validation (build-time configurable)
- Empty line before first list item

**Example**:

```bash
git commit -sS -m "feat(api): add user endpoint"           # ✅ Valid
git commit -m "update code"                                # ❌ Missing -sS flags
git commit -sS -m "feat(ci): add workflow"                 # ❌ Use ci(...) instead
git commit -sS -m "This is a very long commit message..."  # ❌ Title too long
```

#### PushValidator

**Triggers**: `git push` commands

**Validates**:

- Remote existence
- Project-specific rules:
  - Kong projects: blocks `origin`, requires `upstream`
  - kumahq/kuma: warns on `upstream` push
- Default remote handling (tracking branch or origin fallback)

**Example**:

```bash
# In Kong project:
git push origin main    # ❌ Blocked (use upstream)
git push upstream main  # ✅ Allowed

# In kumahq/kuma:
git push upstream main  # ⚠️ Warning (push to fork?)
```

#### BranchValidator

**Triggers**: `git checkout -b` and `git branch` commands

**Validates**:

- Format: `type/description` (e.g., `feat/add-feature`)
- Lowercase only
- No spaces in branch names
- Valid branch types: feat, fix, docs, style, refactor, test, chore, ci, build, perf
- Skips validation for main/master branches

**Example**:

```bash
git checkout -b feat/add-api       # ✅ Valid
git checkout -b Feature/AddAPI     # ❌ Must be lowercase
git checkout -b add-feature        # ❌ Missing type prefix
git checkout -b feat/add api       # ❌ No spaces allowed
```

#### PRValidator

**Triggers**: `gh pr create` commands

**Validates**:

- **Title**: Semantic commit format, blocks `feat(ci)`, `fix(test)`, etc.
- **Body**:
  - Template sections: Motivation, Implementation information, Supporting documentation
  - Changelog rules: ci/test/chore/build/docs/style/refactor should use `> Changelog: skip`
  - No formal language (utilize, leverage, facilitate, implement)
  - No line breaks in paragraphs (heuristic-based)
  - Empty Supporting documentation gets warning
- **Markdown**: Runs markdownlint if available
- **Base branch labels**: Non-main/master PRs need matching label
- **CI label heuristics**: Suggests `ci/skip-test`, `ci/skip-e2e-test`

**Example**:

```bash
gh pr create --title "feat(api): add user endpoint" --body "..."  # ✅ Valid
gh pr create --title "feat(ci): add workflow" --body "..."        # ❌ Use ci(...) instead
```

### File Validators

#### MarkdownValidator

**Triggers**: Write/Edit on `*.md` files, commit message bodies, PR descriptions

**Validates** (blocks invalid writes):

- Empty line before code blocks
- Empty line before first list item
- Empty line after headers
- Code block indentation in lists (must align with list content)
- No multiple consecutive empty lines before code blocks

**Validation rules**:

- Headers must have an empty line after them
- Lists must have an empty line before the first item
- Code blocks must have exactly one empty line before them
- Code blocks inside lists must be indented to align with the list content (e.g., 3 spaces for "1. ", 2 spaces for "- ")
- Partial indentation (e.g., 1 space when 3 are required) is blocked as it suggests incorrect intent

#### ShellScriptValidator

**Triggers**: Write/Edit on `*.sh`, `*.bash` files

**Validates**:

- Runs shellcheck on script content
- Skips Fish scripts (`.fish` extension or Fish shebang)
- Timeout protection (10 seconds)
- Graceful fallback if shellcheck not installed

**Example**: Catches unquoted variables, missing error handling, etc.

#### TerraformValidator

**Triggers**: Write/Edit on `*.tf` files

**Validates**:

- Detects tofu vs terraform
- Runs `fmt -check` for formatting
- Runs tflint if available
- Creates temp file for content validation
- Graceful fallback if tools not installed

#### WorkflowValidator

**Triggers**: Write/Edit on `.github/workflows/*.{yml,yaml}` files

**Validates**:

- **Digest pinning**: Enforces SHA-1/SHA-256 for action versions
- **Version comments**: Digest-pinned actions need `# v1.2.3` comment
- **Explanation comments**: Tag-pinned actions need explanation
- **Latest version warnings**: Checks GitHub API for updates
- Runs actionlint if available
- Skips local actions (`./...`) and Docker actions

**Example**:

```yaml
# ✅ Good: digest-pinned with version comment
- uses: actions/checkout@abc123def456  # v4.1.1

# ❌ Bad: digest-pinned without version comment
- uses: actions/checkout@abc123def456

# ❌ Bad: tag-pinned without explanation
- uses: actions/checkout@v4

# ✅ Good: tag-pinned with explanation
- uses: actions/checkout@v4  # Using tag due to compatibility requirements
```

### Notification Validators

#### BellValidator

**Triggers**: Notification events with type "bell"

**Action**: Sends bell character to `/dev/tty` for terminal notifications

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

## Advanced Bash Parsing

Uses `mvdan.cc/sh` for production-grade Bash parsing:

**Supported Constructs**:

- Command chains: `git add . && git commit -m "msg"`
- Pipes: `cat file | grep pattern | tee output`
- Subshells: `(cd dir && git commit)`
- Command substitution: `echo "$(git log)"`
- Redirections: `echo "text" > file.txt`, `cmd >> file.txt`
- Heredocs: `cat <<EOF ... EOF`
- Quoted strings: Handles `"msg && trick"` vs real chain

**File Write Detection**:

- Redirections: `>`, `>>`
- Tee: `tee file1 file2`
- Copy: `cp src dest`
- Move: `mv src dest`
- Heredoc: `cat <<EOF > file`

**Protected Path Validation**:

- Blocks writes to `/tmp`, `/var/tmp`
- Suggests project-local `tmp/` directory
- Helpful error messages with guidance

## Development

### Adding a New Validator

1. **Create validator file** in `internal/validators/{category}/`

```go
package category

import (
    "github.com/smykla-labs/claude-hooks/internal/validator"
    "github.com/smykla-labs/claude-hooks/pkg/hook"
)

type MyValidator struct {
    validator.BaseValidator
}

func NewMyValidator(logger logger.Logger) *MyValidator {
    v := &MyValidator{}
    v.SetLogger(logger)
    return v
}

func (v *MyValidator) Name() string {
    return "MyValidator"
}

func (v *MyValidator) Validate(ctx *hook.Context) *validator.Result {
    // Validation logic here
    return validator.Pass()
}
```

2. **Write tests** in `internal/validators/{category}/my_validator_test.go`

```go
package category_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("MyValidator", func() {
    It("should pass for valid input", func() {
        // Test implementation
    })
})
```

3. **Register validator** in `cmd/claude-hooks/main.go`

```go
registry.Register(
    validators_category.NewMyValidator(logger),
    validator.And(
        validator.EventTypeIs(hook.PreToolUse),
        validator.ToolTypeIs(hook.Bash),
        validator.CommandContains("my-command"),
    ),
)
```

4. **Run tests and lint**

```bash
task test
task lint
```

### Testing

```bash
# Run all tests with coverage
task test

# Run specific test suite
go test -v ./pkg/parser

# Run single test
go test -v ./pkg/parser -run TestBashParser

# View coverage report
go test -cover ./...
```

### Debugging

Logs are written to `~/.claude/hooks/dispatcher.log`:

```bash
# Follow logs in real-time
tail -f ~/.claude/hooks/dispatcher.log

# View recent logs
tail -100 ~/.claude/hooks/dispatcher.log

# Search for errors
grep "ERROR" ~/.claude/hooks/dispatcher.log
```

## Exit Codes

- `0`: Operation allowed (validation passed or no validators matched)
- `2`: Operation blocked (validation failed with `ShouldBlock=true`)

Warnings (`ShouldBlock=false`) print to stderr but allow operation (exit 0).

## Configuration

### Environment Variables

**Git SDK Configuration**:

```bash
# Use SDK-based git operations (2-5.9M× faster)
export CLAUDE_HOOKS_USE_SDK_GIT=true

# Use CLI-based git operations (default, backward compatible)
unset CLAUDE_HOOKS_USE_SDK_GIT
```

The project supports two git operation implementations:

- **SDK Implementation**: Native Go using `go-git/go-git/v6`
  - 2-5.9M× faster for cached operations (`IsInRepo`, `GetRepoRoot`)
  - 177× faster for `GetCurrentBranch`
  - 1.5× faster for `GetStagedFiles`
  - Enable with `CLAUDE_HOOKS_USE_SDK_GIT=true` or `CLAUDE_HOOKS_USE_SDK_GIT=1`

- **CLI Implementation** (default): Executes git commands via shell
  - Fully tested and backward compatible
  - Automatic fallback if SDK initialization fails

### Build-time Configuration

**Signoff Validation**:

```bash
# Build with specific signoff requirement
go build -ldflags="-X 'github.com/smykla-labs/claude-hooks/internal/validators/git.ExpectedSignoff=Name <email>'" ./cmd/claude-hooks

# Or use task build:prod (uses git config)
task build:prod
```

### Runtime Flags

```bash
# Debug mode (enabled by default)
chook -T preToolUse --debug

# Trace mode (verbose logging)
chook -T preToolUse --trace
```

## Performance

- **Cold start**: <100ms target
- **Parser**: <100µs for typical commands
- **Validators**: <50ms each (I/O dependent)
- **Total**: <500ms for full validation chain

## Migration from Bash

This Go implementation replaces the previous Bash-based validator system. Key differences:

**Improvements**:

- Advanced command parsing (chains, pipes, subshells)
- File write detection across all operations
- Better error messages with actionable guidance
- Comprehensive test coverage (439 specs)
- Faster execution and cold start

**Feature Parity**:

- All Bash validators ported to Go
- Identical validation rules
- Same exit codes and behavior
- Compatible with existing hooks configuration

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

- **Issues**: https://github.com/smykla-labs/claude-hooks/issues
- **Discussions**: https://github.com/smykla-labs/claude-hooks/discussions
- **Logs**: `~/.claude/hooks/dispatcher.log`
