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

# Debug (inspect configuration)
./bin/klaudiush debug rules                       # show all rules
./bin/klaudiush debug rules --validator git.push  # filter by validator
./bin/klaudiush debug exceptions                  # show exception config
./bin/klaudiush debug exceptions --state          # include rate limit state

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

**Error Format Policy**: Validators return errors with structured format including error codes (GIT001-GIT018, FILE001-FILE005, SEC001-SEC005, SHELL001-SHELL005), automatic fix hints from suggestions registry, and documentation URLs (`https://klaudiu.sh/{CODE}`). Use `FailWithRef(ref, msg)` to auto-populate fix hints - NEVER set `FixHint` manually. Error priority determines which reference is shown when multiple rules fail. See `.claude/validator-error-format-policy.md` for comprehensive guide.

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

**File** (`internal/validators/file/`): MarkdownValidator, ShellScriptValidator (shellcheck), TerraformValidator (tofu/terraform fmt+tflint), WorkflowValidator (actionlint)

**Secrets** (`internal/validators/secrets/`): SecretsValidator (25+ regex patterns for AWS/GitHub/private keys/connection strings, optional gitleaks integration, configurable allow lists)

**Shell** (`internal/validators/shell/`): BacktickValidator (detects command substitution in double-quoted strings for git commit, gh pr create, gh issue create)

**Notification** (`internal/validators/notification/`): BellValidator (ASCII 7 to `/dev/tty` for dock bounce)

**Plugins** (`internal/plugin/`): External validators via Go plugins (.so), exec plugins (JSON over stdin/stdout), or gRPC. Predicate-based matching (event/tool/file/command filters), per-plugin config, enable/disable flags. See `docs/PLUGIN_GUIDE.md`, `.claude/session-plugin-system.md`, and `.claude/session-grpc-loader.md`.

### Exception Workflow (`internal/exceptions/`)

Allow bypassing validation blocks with explicit acknowledgment and audit trail.

**Core Components**:

- **Token Parser** (`token.go`): Extracts `EXC:<CODE>:<REASON>` from shell comments or `KLAUDIUSH_ACK` env var
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
KLAUDIUSH_ACK="EXC:SEC001:Test+fixture" git commit -sS -m "msg"
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

**Error Handling**: NEVER use `fmt.Errorf`, `errors`, or `github.com/pkg/errors` - linter will reject. ALWAYS use `github.com/cockroachdb/errors` for error creation and wrapping

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

## Documentation

Additional implementation details and policies are in `.claude/` files:

- `validator-error-format-policy.md` - Comprehensive guide for validator error formatting, reference system (GIT001-GIT018, FILE001-FILE005, SEC001-SEC005), suggestions registry, FailWithRef pattern, error display format, best practices
- `session-parallel-execution.md` - Parallel validator execution, category-specific worker pools, race detection testing
- `session-error-reporting.md` - Error codes, suggestions/doc links registries, FailWithCode pattern, cognitive complexity refactoring
- `session-secrets-detection.md` - Secrets validator with 25+ regex patterns, two-tier detection (patterns + gitleaks), configuration schema for allow lists/custom patterns
- `session-plugin-system.md` - Plugin system with Go/exec loaders, predicate-based matching, flexible configuration, validator adapter integration
- `session-grpc-loader.md` - gRPC plugin loader with connection pooling, buf 1.61.0 setup, protobuf protocol, double-check locking, type conversion between internal and protobuf types, TLS security (auto/manual modes, mTLS support)
- `session-plugin-integration-tests.md` - Integration tests for plugin system with real shell scripts and gRPC servers, testing best practices research (bufconn vs real network, subprocess patterns)
- `session-fuzzing.md` - Go native fuzzing for parsers, fuzz targets by risk, type limitations, progress tracking in `tmp/fuzzing/`
- `session-github-quality.md` - OSSF Scorecard, branch rulesets API, Renovate version sync (customManagers:githubActionsVersions), smyklot bot workflows
- `session-codeql-regex-security.md` - CodeQL regex anchor fixes (CWE-020), URL pattern anchoring with `(?:^|://|[^/a-zA-Z0-9])`, bounded quantifiers for ReDoS, prefix consumption in matches, GitHub push protection bypass for test secrets, PR review thread resolution
- `session-rule-engine.md` - Rule engine implementation details (covered more comprehensively in `docs/RULES_GUIDE.md`)

## Plugin Documentation

Comprehensive plugin development guide available in `docs/PLUGIN_GUIDE.md` with working examples in `examples/plugins/`:

- **Go plugins** (`examples/plugins/go-plugin/`) - Native .so plugins for maximum performance
- **Exec plugins** (`examples/plugins/exec-shell/`) - Shell script plugins for cross-platform compatibility
- **gRPC plugins** (`examples/plugins/grpc-go/`) - Persistent server plugins with hot-reload capability

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
