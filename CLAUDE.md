# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working with this repository.

## Project Overview

`klaudiush` is a validation dispatcher for Claude Code hooks. Intercepts PreToolUse events and validates commands before execution, enforcing git workflow standards and commit message conventions.

## Commands

```bash
# Init (interactive setup wizard, creates config.toml)
./bin/klaudiush init              # project config
./bin/klaudiush init --global     # global config
./bin/klaudiush init --force      # overwrite existing

# Doctor (diagnose setup and configuration)
./bin/klaudiush doctor            # run all checks
./bin/klaudiush doctor --verbose  # detailed output
./bin/klaudiush doctor --fix      # auto-fix issues
./bin/klaudiush doctor --category binary,hook  # filter by category

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

### Parsers

**Bash** (`pkg/parser/bash.go`): AST parsing via `mvdan.cc/sh/v3/syntax`, extracts commands/file writes/git ops

**Git** (`pkg/parser/git.go`): Parses to `GitCommand`, handles combined flags (`-sS` → `["-s", "-S"]`), `HasFlag()` checks both forms

### Validators

**Git** (`internal/validators/git/`): AddValidator (file existence), CommitValidator (flags `-sS`, staging, message), PushValidator (remote/branch), PRValidator (title/body/changelog)

**Commit Message** (`commit_message.go`): Conventional commits `type(scope): description`, title ≤50 chars, body ≤72 chars, blocks `feat(ci)`/`fix(test)` (use `ci(...)`/`test(...)` instead), no PR refs/Claude attribution

**File** (`internal/validators/file/`): MarkdownValidator, ShellScriptValidator (shellcheck), TerraformValidator (tofu/terraform fmt+tflint), WorkflowValidator (actionlint)

**Secrets** (`internal/validators/secrets/`): SecretsValidator (25+ regex patterns for AWS/GitHub/private keys/connection strings, optional gitleaks integration, configurable allow lists)

**Notification** (`internal/validators/notification/`): BellValidator (ASCII 7 to `/dev/tty` for dock bounce)

### Linter Abstractions (`internal/linters/`)

Type-safe interfaces for external tools: **ShellChecker** (shellcheck), **TerraformFormatter** (tofu/terraform fmt), **TfLinter** (tflint), **ActionLinter** (actionlint), **MarkdownLinter** (custom rules), **GitleaksChecker** (gitleaks)

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

## Exit Codes

- `0`: Allowed (pass/warn/no match)
- `2`: Blocked (fail with `ShouldBlock=true`)

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

## Session Notes

Additional implementation details from specific sessions are in `.claude/session-*.md` files:

- `session-parallel-execution.md` - Parallel validator execution, category-specific worker pools, race detection testing
- `session-error-reporting.md` - Error codes, suggestions/doc links registries, FailWithCode pattern, cognitive complexity refactoring
- `session-secrets-detection.md` - Secrets validator with 25+ regex patterns, two-tier detection (patterns + gitleaks), configuration schema for allow lists/custom patterns
- `session-fuzzing.md` - Go native fuzzing for parsers, fuzz targets by risk, type limitations, progress tracking in `tmp/fuzzing/`
- `session-github-quality.md` - OSSF Scorecard, branch rulesets API, Renovate version sync (customManagers:githubActionsVersions), smyklot bot workflows
- `session-codeql-regex-security.md` - CodeQL regex anchor fixes (CWE-020), URL pattern anchoring with `(?:^|://|[^/a-zA-Z0-9])`, bounded quantifiers for ReDoS, prefix consumption in matches, GitHub push protection bypass for test secrets, PR review thread resolution
