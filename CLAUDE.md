# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working with this repository.

## Project Overview

`klaudiush` is a validation dispatcher for Claude Code hooks. Intercepts PreToolUse events and validates commands before execution, enforcing git workflow standards and commit message conventions.

## Commands

```bash
# Completion (shell completion scripts)
klaudiush completion bash         # generate bash completion
klaudiush completion fish         # generate fish completion
klaudiush completion zsh          # generate zsh completion
klaudiush completion powershell   # generate powershell completion

# Init (interactive setup wizard, creates config.toml)
./bin/klaudiush init              # project config
./bin/klaudiush init --global     # global config
./bin/klaudiush init --force      # overwrite existing

# Doctor (diagnose setup and configuration)
./bin/klaudiush doctor            # run all checks
./bin/klaudiush doctor --verbose  # detailed output
./bin/klaudiush doctor --fix      # auto-fix issues
./bin/klaudiush doctor --category binary,hook  # filter by category

# Debug (inspect configuration)
./bin/klaudiush debug rules                       # show all rules
./bin/klaudiush debug rules --validator git.push  # filter by validator
./bin/klaudiush debug exceptions                  # show exception config
./bin/klaudiush debug exceptions --state          # include rate limit state

# Crash (crash dump management)
klaudiush debug crash list                        # list all crash dumps
klaudiush debug crash view <id>                   # view crash dump details
klaudiush debug crash clean                       # remove old dumps
klaudiush debug crash clean --dry-run             # show what would be removed

# Audit (exception audit log management)
./bin/klaudiush audit list                        # list all entries
./bin/klaudiush audit list --error-code GIT019    # filter by code
./bin/klaudiush audit list --outcome allowed      # filter by outcome
./bin/klaudiush audit stats                       # show statistics
./bin/klaudiush audit cleanup                     # remove old entries

# Build & Install
task build                        # dev build
task build:prod                   # prod build (validates signoff)
task install                      # install to ~/.claude/hooks/dispatcher

# Testing
task test                         # all tests
task test:unit                    # unit tests only
task test:integration             # integration tests only
task test:fuzz                    # fuzz tests (10s each)
task test:fuzz:git                # git parser fuzz (60s)
FUZZ_TIME=5m task test:fuzz:git   # custom duration

# Linting & Development
task check                        # lint + auto-fix
task lint                         # lint only
task fmt                          # format code
task deps                         # update dependencies
task verify                       # fmt + lint + test
task clean                        # clean artifacts
```

**Init Extensibility**: Add new options via `ConfigOption` interface in `internal/initcmd/options.go`.

## Architecture

### Core Flow

1. CLI Entry (`cmd/klaudiush/main.go`) → 2. JSON Parser (`internal/parser/json.go`) → 3. Dispatcher (`internal/dispatcher/dispatcher.go`) → 4. Registry (`internal/validator/registry.go`) matches validators via predicates → 5. Validators return `Result` (Pass/Fail/Warn)

### Execution Abstractions (`internal/exec/`)

Unified command execution abstractions eliminating ~134 lines of duplication:

- **CommandRunner**: Execute commands with timeout/context, returns `CommandResult`
- **ToolChecker**: Check tool availability (`IsAvailable`, `FindTool` for alternatives like `tofu` vs `terraform`)
- **TempFileManager**: Temp file lifecycle management

### Hook Context (`pkg/hook/context.go`)

Represents tool invocations: `EventType` (PreToolUse/PostToolUse/Notification), `ToolName` (Bash/Write/Edit/Grep), `ToolInput` (Command/FilePath/Content).

### Validator System

**Registration** (`internal/validator/registry.go`): Predicate-based matching (e.g., `validator.And(EventTypeIs(PreToolUse), ToolTypeIs(Bash), CommandContains("git commit"))`)

**Results** (`internal/validator/validator.go`): `Pass()`, `Fail(msg)` (blocks, exit 2), `Warn(msg)` (logs, allows)

**Creating**: 1) Embed `BaseValidator`, 2) Implement `Validate(ctx *hook.Context)`, 3) Register in `main.go:registerValidators()`

**Error Format Policy**: Validators return errors with structured format including error codes (GIT001-GIT024, FILE001-FILE009, SEC001-SEC005, SHELL001-SHELL005, SESS001), automatic fix hints from suggestions registry, and documentation URLs (`https://klaudiu.sh/{CODE}`). Use `FailWithRef(ref, msg)` to auto-populate fix hints - NEVER set `FixHint` manually. Error priority determines which reference is shown when multiple rules fail. See `.claude/validator-error-format-policy.md` for comprehensive guide.

### Rule Engine (`internal/rules/`)

Dynamic validation configuration without modifying code. Rules allow users to define custom validation behavior via TOML configuration.

**Components**: Pattern system (glob/regex auto-detection via `gobwas/glob`), Matchers (repo/remote/branch/file/content/command), Registry (priority sorting, merge), Evaluator (first-match semantics), Engine (main entry point), ValidatorAdapter (bridges with validators).

**Usage**: Validators use `RuleValidatorAdapter.CheckRules()` before built-in logic. If rule matches, returns validator.Result; otherwise continues with built-in validation.

**Documentation**: See `docs/RULES_GUIDE.md` for complete configuration guide with examples. Example configurations in `examples/rules/`.

### Parsers

**Bash** (`pkg/parser/bash.go`): AST parsing via `mvdan.cc/sh/v3/syntax`, extracts commands/file writes/git ops

**Git** (`pkg/parser/git.go`): Parses to `GitCommand`, handles combined flags (`-sS` → `["-s", "-S"]`), `HasFlag()` checks both forms

### Validators

**Git** (`internal/validators/git/`): AddValidator (file existence), CommitValidator (flags `-sS`, staging, message), PushValidator (remote/branch), PRValidator (title/body/changelog)

**Commit Message** (`commit_message.go`): Conventional commits `type(scope): description`, title ≤50 chars, body ≤72 chars, blocks `feat(ci)`/`fix(test)` (use `ci(...)`/`test(...)` instead), no PR refs/Claude attribution

**File** (`internal/validators/file/`): MarkdownValidator, ShellScriptValidator (shellcheck), TerraformValidator (tofu/terraform fmt+tflint), WorkflowValidator (actionlint), GofumptValidator (gofumpt with go.mod auto-detection), PythonValidator (ruff), JavaScriptValidator (oxlint), RustValidator (rustfmt with Cargo.toml edition auto-detection)

**Secrets** (`internal/validators/secrets/`): SecretsValidator (25+ regex patterns for AWS/GitHub/private keys/connection strings, optional gitleaks integration, configurable allow lists)

**Shell** (`internal/validators/shell/`): BacktickValidator (detects command substitution in strings)

- **Legacy mode** (default): Validates backticks only in git commit, gh pr create, gh issue create commands
- **Comprehensive mode** (opt-in via config): Validates all Bash commands for:
  - Unquoted backticks (e.g., `echo \`date\``)
  - Backticks in double quotes (e.g., `echo "Fix \`parser\`"`)
  - Variable analysis: suggests single quotes when no variables present
  - Configurable via `check_all_commands`, `check_unquoted`, `suggest_single_quotes` options

**Notification** (`internal/validators/notification/`): BellValidator (ASCII 7 to `/dev/tty` for dock bounce)

**Plugins** (`internal/plugin/`): External validators via exec plugins (JSON over stdin/stdout). Predicate-based matching (event/tool/file/command filters), per-plugin config, enable/disable flags. See `docs/PLUGIN_GUIDE.md`.

### Exception Workflow (`internal/exceptions/`)

Allow bypassing validation blocks with explicit acknowledgment and audit trail.

**Core Components**:

- **Token Parser** (`token.go`): Extracts `EXC:<CODE>:<REASON>` from shell comments or `KLACK` env var
- **Policy Engine** (`policy.go`, `engine.go`): Per-error-code policies with reason validation
- **Rate Limiter** (`ratelimit.go`): Global + per-code hourly/daily limits, file-persisted state
- **Audit Logger** (`audit.go`): JSONL format with rotation and retention
- **Handler** (`handler.go`): Coordinates all components for exception checking

**Integration Point** (`internal/dispatcher/exception.go`): `ExceptionChecker` interface hooks into dispatcher after validation failure.

**Token Format**: `EXC:<ERROR_CODE>:<URL_ENCODED_REASON>` (e.g., `# EXC:GIT019:Emergency+hotfix`)

**Bypass Flow**:

1. Validator returns blocking error with error code (e.g., `GIT019`)
2. Dispatcher extracts error code from reference URL
3. Exception checker looks for token matching the error code
4. If policy allows + rate limit OK → Block converted to Warning
5. Audit entry logged, command proceeds

**Usage**: Add exception token to command:

```bash
# Shell comment (recommended)
git push origin main  # EXC:GIT019:Emergency+hotfix

# Environment variable
KLACK="EXC:SEC001:Test+fixture" git commit -sS -m "msg"
```

**Enabling Exceptions for Error Codes**: Configure in `.klaudiush/config.toml`:

```toml
[exceptions]
enabled = true

[exceptions.policies.GIT019]
enabled = true
allow_exception = true
require_reason = true
min_reason_length = 10
```

**Documentation**: See `docs/EXCEPTIONS_GUIDE.md` for complete guide. Example configs in `examples/exceptions/`.

### Session Tracking (`internal/session/`)

Fast-fail mechanism preventing subsequent commands from executing after a blocking error occurs in the same Claude Code session.

**Problem**: When klaudiush blocks a command (exit 2), Claude Code continues executing queued commands independently, causing poor UX as each fails one-by-one.

**Solution**: Track session state. When any command is blocked, "poison" the session. Subsequent commands immediately fail with reference to original error.

**Core Components**:

- **Tracker** (`tracker.go`): Main session tracking logic with thread-safe state management
  - `IsPoisoned(sessionID)` - Check if session is poisoned
  - `Poison(sessionID, codes []string, message)` - Mark session as poisoned with multiple codes
  - `Unpoison(sessionID)` - Clear poisoned state, return to clean
  - `RecordCommand(sessionID)` - Increment command count
  - `CleanupExpired()` - Remove sessions older than max age

- **State** (`state.go`): Session state types
  - `SessionInfo` - Per-session metadata (status, poison info, command count, timestamps)
  - `PoisonCodes []string` - All error codes that caused poisoning
  - `Status` enum - Clean/Poisoned (enumer generated)
  - `SessionState` - Global state container with sessions map

- **Unpoison Parser** (`unpoison.go`): Parse unpoison tokens from commands
  - Token format: `SESS:<CODE1>[,<CODE2>,...]`
  - Sources: `KLACK` env var or shell comment
  - `CheckUnpoisonAcknowledgment()` - Check if all poison codes are acknowledged

- **Audit Logger** (`audit.go`): Audit trail for poison/unpoison events
  - JSONL format with rotation and retention
  - Records action type, session ID, codes, source, command
  - Configurable via `[session.audit]` in config

- **Persistence** (`persistence.go`): Atomic file I/O with proper permissions
  - JSON serialization to `~/.klaudiush/session_state.json`
  - Atomic writes (tmp + rename), file permissions 0600, dir permissions 0700
  - Home directory expansion for `~` in paths

**Integration**: Dispatcher checks session state before validation. If poisoned, checks for unpoison token; if all codes acknowledged, unpoisons and continues to validators. Otherwise returns `SESS001` error with unpoison instructions. On validation failure with `ShouldBlock=true`, dispatcher poisons the session with all blocking error codes.

**Configuration** (`pkg/config/session.go`):

```toml
[session]
enabled = true  # Default: true
state_file = "~/.klaudiush/session_state.json"
max_session_age = "24h"  # Auto-expire old sessions

[session.audit]
enabled = true  # Default: true
log_file = "~/.klaudiush/session_audit.jsonl"
max_size_mb = 10
max_age_days = 30
max_backups = 5
```

**Error Code**: `SESS001` - Session poisoned by previous blocking error

**Session Context**: Parser extracts `session_id`, `tool_use_id`, `transcript_path` from Claude Code hook JSON. Available in `pkg/hook/context.go` Context struct.

**Documentation**: See `docs/SESSION_GUIDE.md` for user guide and troubleshooting.

### Linter Abstractions (`internal/linters/`)

Type-safe interfaces for external tools: **ShellChecker** (shellcheck), **TerraformFormatter** (tofu/terraform fmt), **TfLinter** (tflint), **ActionLinter** (actionlint), **MarkdownLinter** (custom rules), **GofumptChecker** (gofumpt), **RuffChecker** (ruff), **OxlintChecker** (oxlint), **RustfmtChecker** (rustfmt), **GitleaksChecker** (gitleaks)

**Common Types** (`result.go`): `LintResult` (success/findings), `LintFinding` (file/line/message), `LintSeverity` (Error/Warning/Info)

### Git Operations (`internal/git/`)

**Dual Implementation**: SDK (go-git/v6, 2-5.9M× faster, default) and CLI (fallback). Set `KLAUDIUSH_USE_SDK_GIT=false` to force CLI.

**Runner Interface** (`runner.go`): Unified interface for both - `IsInRepo()`, `GetStagedFiles()`, `GetModifiedFiles()`, `GetUntrackedFiles()`, `GetRepoRoot()`, `GetCurrentBranch()`, `GetBranchRemote()`, `GetRemoteURL()`, `GetRemotes()`

**Utilities**: `ConfigReader` (git config via SDK), `ExcludeManager` (.git/info/exclude), `RepositoryAdapter` (wraps SDK for Runner), `MockGitRunner` (testing)

### Configuration System

Clean Architecture layers: Application → Factory → Provider → Implementation → Schema

**Schema** (`pkg/config/`): Root config, validator configs (git/file/notification), types (Severity/Duration)

**Implementation** (`internal/config/`): TOML loader, validation, deep merge, defaults, secure writer (0600/0700)

**Provider** (`internal/config/provider/`): Multi-source loading (files/env vars/CLI flags), caching

**Factory** (`internal/config/factory/`): Builds validators from config, RegistryBuilder creates complete registry

**Precedence** (highest to lowest): CLI Flags → Env Vars (`KLAUDIUSH_*`) → Project Config (`.klaudiush/config.toml`) → Global Config (`~/.klaudiush/config.toml`) → Defaults

**Examples**:

```bash
# CLI flags
klaudiush --config=./my-config.toml --disable=commit,markdown --hook-type PreToolUse

# Env vars
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH=72
```

```toml
# TOML (deep merge: global defaults, project overrides)
[validators.git.commit.message]
title_max_length = 72
check_conventional_commits = true
```

**Interactive Setup** (`internal/initcmd/`): Extensible options via `ConfigOption` interface, prompts via `Prompter`

**No Config Required**: Validators accept `nil` config and use built-in defaults when no configuration is provided

### Logging

Logs to `~/.claude/hooks/dispatcher.log`: `--debug` (default), `--trace` (verbose). Use `BaseValidator.Logger()`.

## Testing

Framework: Ginkgo/Gomega. 336 tests. Run: `mise exec -- go test -v ./pkg/parser -run TestBashParser`

**Mocks**: Generated via `mockgen` (uber-go/mock). Add `//go:generate mockgen -source=<file>.go -destination=<file>_mock.go -package=<pkg>` directive, then run `go generate ./...`. NEVER manually edit generated mock files.

## Development

**Tools** (mise): Go 1.25.4, golangci-lint 2.6.2, task 3.45.5, markdownlint-cli 0.46.0. Run `mise install`. See `SETUP.md`.

**Linters** (`.golangci.yml`): Nil safety (nilnesserr, govet), completeness (exhaustive, gochecksumtype), quality (gocognit, goconst, cyclop, dupl)

**Error Handling**: NEVER use `fmt.Errorf`, `errors`, or `github.com/pkg/errors` - linter will reject. ALWAYS use `github.com/cockroachdb/errors` for error creation and wrapping

## Exit Codes

- `0`: Allowed (pass/warn/no match)
- `2`: Blocked (fail with `ShouldBlock=true`)
- `3`: Crash (panic with crash dump created)

## GitHub Push Protection

When pushing code with intentional test secrets (e.g., in fuzz tests or detector tests), GitHub may block the push. To allow test secrets:

```bash
# Extract placeholder_id from the error message URL (last path segment)
# e.g., https://github.com/OWNER/REPO/security/secret-scanning/unblock-secret/PLACEHOLDER_ID

# Allow the secret with reason "used_in_tests"
gh api repos/OWNER/REPO/secret-scanning/push-protection-bypasses \
  -X POST \
  -f secret_type="SECRET_TYPE" \
  -f reason="used_in_tests" \
  -f placeholder_id="PLACEHOLDER_ID"
```

Common secret types: `stripe_api_key`, `slack_api_token`, `github_token`, `aws_access_key_id`

Valid reasons: `used_in_tests`, `false_positive`, `will_fix_later`

## Documentation

Additional implementation details and policies are in `.claude/` files:

- `validator-error-format-policy.md` - Comprehensive guide for validator error formatting, reference system (GIT001-GIT024, FILE001-FILE009, SEC001-SEC005), suggestions registry, FailWithRef pattern, error display format, best practices
- `session-parallel-execution.md` - Parallel validator execution, category-specific worker pools, race detection testing
- `session-error-reporting.md` - Error codes, suggestions/doc links registries, FailWithCode pattern, cognitive complexity refactoring

## Plugin Documentation

Plugin development guide available in `docs/PLUGIN_GUIDE.md` with working example in `examples/plugins/`:

- **Exec plugins** (`examples/plugins/exec-shell/`) - Shell script plugins for cross-platform compatibility

Each example includes source code, configuration, testing instructions, and customization guidance.

## Rules Documentation

Dynamic validation rules guide available in `docs/RULES_GUIDE.md` with example configurations in `examples/rules/`:

- **organization.toml** - Organization-specific rules (remote restrictions, branch protection)
- **secrets-allow-list.toml** - Allow list for test fixtures and mock data
- **advanced-patterns.toml** - Complex pattern matching examples (glob, regex, combined conditions)

Debug rules with: `klaudiush debug rules`

## Exceptions Documentation

Exception workflow guide available in `docs/EXCEPTIONS_GUIDE.md` with example configurations in `examples/exceptions/`:

- **basic.toml** - Standard exception configuration
- **strict-security.toml** - Production security focused (no exceptions for critical codes)
- **development.toml** - Relaxed limits for development environments

Debug exceptions with: `klaudiush debug exceptions`

## Session Documentation

Session tracking guide available in `docs/SESSION_GUIDE.md`:

- Feature overview and problem statement
- Configuration options (enabled, state_file, max_session_age)
- Troubleshooting session issues
- SESS001 error code explanation

## Backup Documentation

Configuration backup system guide available in `docs/BACKUP_GUIDE.md` with example configurations in `examples/backup/`:

- **basic.toml** - Standard configuration (10 snapshots, 30 days, 50MB, async backups)
- **minimal.toml** - Conservative for limited storage (5 snapshots, 7 days, 10MB)
- **production.toml** - Extended retention (20 snapshots, 90 days, 100MB, sync backups)
- **development.toml** - Development-optimized (15 snapshots, 14 days, 50MB)

Commands:

```bash
klaudiush backup list [--project PATH | --global | --all]
klaudiush backup create [--tag TAG --description DESC]
klaudiush backup restore SNAPSHOT_ID [--dry-run] [--force]
klaudiush backup delete SNAPSHOT_ID...
klaudiush backup prune [--dry-run]
klaudiush backup status
klaudiush backup audit [--operation OP --since TIME --snapshot ID]
```

Doctor integration: `klaudiush doctor --category backup [--fix]`

## Crash Dump System

Automatic diagnostic collection on panic for troubleshooting crashes.

**Core Components**:

- **Dump Writer** (`internal/crashdump/writer.go`): Atomic JSON writes with `crash-{timestamp}-{shortID}.json` naming
- **Collector** (`internal/crashdump/collector.go`): Captures panic value, stack trace (panicking goroutine only), runtime info (GOOS/GOARCH/NumGoroutine/Version), sanitized config, hook context. Handles Go 1.21+ `*runtime.PanicNilError` from `panic(nil)`.
- **Storage** (`internal/crashdump/storage.go`): List/get/delete/prune operations with age and count-based retention
- **Sanitizer** (`internal/crashdump/sanitizer.go`): Removes sensitive fields (*token*, *secret*, *password*, *key*, *credential*) from config snapshots
- **CLI Commands** (`cmd/klaudiush/crash.go`): List, view, and clean crash dumps

**Configuration** (`pkg/config/crashdump.go`):

```toml
[crash_dump]
enabled = true                              # Enable automatic dumps (default)
dump_dir = "~/.klaudiush/crash_dumps"      # Storage location
max_dumps = 10                              # Maximum dumps to keep
max_age = "720h"                            # 30 days retention
include_config = true                       # Include sanitized config
include_context = true                      # Include hook context
```

**Usage**:

```bash
# List all crash dumps (sorted newest first)
klaudiush debug crash list

# View full details including stack trace
klaudiush debug crash view crash-20251204T160432-a1b2c3

# Clean old dumps based on retention policy
klaudiush debug crash clean
klaudiush debug crash clean --dry-run       # Preview removals
```

**Integration** (`cmd/klaudiush/main.go`): Panic recovery wrapper in main() creates dumps on crash, logs dump path, exits with code 3.

**Defaults**: Enabled with automatic cleanup (10 dumps max, 30-day retention). No manual configuration required.

**Example Config**: See `examples/config/crashdump.toml` for all options.
