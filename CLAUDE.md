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

# Init (interactive setup wizard, creates config.toml + registers hooks)
./bin/klaudiush init                           # project config + hooks
./bin/klaudiush init --global                  # global config + hooks
./bin/klaudiush init --force                   # overwrite existing
./bin/klaudiush init --install-hooks           # register hooks only
./bin/klaudiush init --install-hooks --global  # register hooks globally
./bin/klaudiush init --install-hooks=false     # config only, no hooks

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

# Metrics (local enforcement outcomes)
./bin/klaudiush metrics report --since 7d         # outcomes, repairs, unavailable, latency
./bin/klaudiush metrics report --json             # machine-readable
./bin/klaudiush metrics prune                     # drop records older than retention
./bin/klaudiush metrics clear                     # remove logs and salt

# Bypass (validation when approval prompts are off)
./bin/klaudiush bypass status                     # show effective setting
./bin/klaudiush bypass skip --reason "spike"      # stop validating in bypass modes
./bin/klaudiush bypass skip --duration 4h         # time-boxed opt-out
./bin/klaudiush bypass enforce                    # restore the default
./bin/klaudiush bypass notify off                 # keep validating, hide the reminder

# Build & Install
mise run build                        # dev build
mise run build:prod                   # prod build (validates signoff)
mise run install                      # install to ~/.claude/hooks/dispatcher

# Testing
mise run test                         # all tests
mise run test:unit                    # unit tests only
mise run test:integration             # integration tests only
mise run test:fuzz                    # fuzz tests (10s each)
mise run test:fuzz:git                # git parser fuzz (60s)
FUZZ_TIME=5m mise run test:fuzz:git   # custom duration
mise run test:harness                 # live harness checks (local only, never CI)
mise run test:harness:fixtures        # same, and rewrite captured fixtures

# Linting & Development
mise run check                        # lint + auto-fix
mise run lint                         # lint only
mise run fmt                          # format code
mise run deps                         # update dependencies
mise run verify                       # fmt + lint + test
mise run clean                        # clean artifacts
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

**Results** (`internal/validator/validator.go`): `Pass()`, `Fail(msg)` (blocks, JSON deny on stdout), `Warn(msg)` (logs, allows)

**Creating**: 1) Embed `BaseValidator`, 2) Implement `Validate(ctx *hook.Context)`, 3) Register in `main.go:registerValidators()`

**Error Format Policy**: Validators return errors with structured format including error codes (GIT001-GIT024, FILE001-FILE009, SEC001-SEC005, SHELL001-SHELL005, HOOK001, EVID001-EVID002, POL001-POL003, MCP001-MCP005), automatic fix hints from suggestions registry, and documentation URLs (`https://klaudiu.sh/{CODE}`). Use `FailWithRef(ref, msg)` to auto-populate fix hints - NEVER set `FixHint` manually. Error priority determines which reference is shown when multiple rules fail. See `.claude/validator-error-format-policy.md` for comprehensive guide.

### Rule Engine (`internal/rules/`)

Dynamic validation configuration without modifying code. Rules allow users to define custom validation behavior via TOML configuration.

**Components**: Pattern system (glob/regex auto-detection via `gobwas/glob`), Matchers (repo/remote/branch/file/content/command), Registry (priority sorting, merge), Evaluator (first-match semantics), Engine (main entry point), ValidatorAdapter (bridges with validators).

**Usage**: Validators use `RuleValidatorAdapter.CheckRules()` before built-in logic. If rule matches, returns validator.Result; otherwise continues with built-in validation.

**Documentation**: See `docs/RULES_GUIDE.md` for complete configuration guide with examples. Example configurations in `examples/rules/`.

### Parsers

**Bash** (`pkg/parser/bash.go`): AST parsing via `mvdan.cc/sh/v3/syntax`, extracts commands/file writes/git ops. `Command.Name` is the program that really runs, never the literal first word: paths, `\git`, `GIT`, variables (line then environment), `$(which git)`, `git-commit`, `hub`, symlinks and copies of git resolve to `git`, and an unknown name invoked with a validated git subcommand is checked as git (fail closed). Commands run through launchers (`env`, `sudo`, `xargs`, `find -exec`, `mise exec`, `docker run`, `--entrypoint git` of docker, podman, nerdctl and compose with any image, ...), scripts (`bash -c`, script files, `source`, stdin, `<<<`, `<(...)`, `eval`), interpreter code (`python -c`, `node -e`, `awk`, ...; in Python and JavaScript a plain string counts as a command line only when the code can run one: a shell call, a split argv, a file write or environment change, a subprocess argv that is not a literal list or names a shell, launcher or interpreter, or a module klaudiush does not know), commands git and gh run themselves (`rebase -x`, shell-valued `-c` settings, `GIT_*_EDITOR`, gh shell aliases), same-line aliases and functions (positional parameters are substituted by their quoting in the body, and a call to a function the top-level line always defines runs only its body, so its arguments are not scanned as a command line), git and gh aliases, scripts under `$HOME` found on PATH, and startup files a new shell reads (`BASH_ENV`, `ENV` for interactive shells, `--rcfile` of interactive bash, when set on the line; `$ZDOTDIR/.zshenv` for every zsh, `.zprofile`/`.zlogin`/`.zshrc` for login or interactive zsh, `.bash_profile`/`.bash_login`/`.profile` for login bash, `.bashrc` for interactive bash, `.profile` for login sh, read from disk or from a write earlier on the line, in `pkg/parser/startup_home.go`; a file in the inherited home that the line never touches is walked leniently, so its opaque parts do not block) are recorded too. Container runs are read up to the program after the image (a non-literal subcommand, option or image, or an unquoted one that may split, is opaque; a non-literal program is a program word), `parallel` command lines are rebuilt from literal inputs (non-literal command words, or command lines from unseen input, are opaque), and unseen `xargs -I` input stands as `{}`. Resolution that needs the system (env, files, program identity, PATH, git and gh config) goes through the injectable `Resolver` (`pkg/parser/resolver.go`). Anything the parser cannot see (nesting deeper than 8 levels, the work budget running out, a script it cannot read, a git subcommand that is not a builtin, an installed `git-<name>` or a visible alias, eval of a line, a program name, a git or gh command word or a container `--entrypoint` built from a variable, command output or glob it cannot resolve, a startup file whose path or content it cannot know) marks `ParseResult.Truncated` and records a typed `parser.Opacity` (cause, operation, origin chain of program names, never arguments) in `ParseResult.Opacities`; the always-on nesting validator blocks it and any command that fails to parse (SHELL002), with one finding per opacity. `git push` arguments and `git commit` options from command output, an unresolved variable, a glob or word splitting are opaque too, and resolved variables in them are substituted (`pkg/parser/git_args.go`). Validators read the command through `hook.Context.ParsedCommand()`, which parses once per hook

**Git** (`pkg/parser/git.go`): Parses to `GitCommand`, handles combined flags (`-sS` → `["-s", "-S"]`), `HasFlag()` checks both forms

### Validators

**Git** (`internal/validators/git/`): AddValidator (file existence), CommitValidator (flags `-sS`, staging, message), PushValidator (remote/branch), PRValidator (title/body/changelog)

**Commit Message** (`commit_message.go`): Conventional commits `type(scope): description`, title ≤50 chars, body ≤72 chars, blocks `feat(ci)`/`fix(test)` (use `ci(...)`/`test(...)` instead), no PR refs/AI attribution

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

### Bypass Permissions (`internal/bypass/`)

Decides what happens when the session runs without approval prompts (Claude `--dangerously-skip-permissions` → `bypassPermissions`, Codex `--dangerously-bypass-approvals-and-sandbox` → `danger-full-access`, Gemini `--yolo` → `yolo`).

**Default**: validation runs in every permission mode. The dispatcher only short-circuits when `dispatcher.WithBypassPolicy` gets a policy whose config sets `skip_validation = true`.

**Components**: `Policy` (`policy.go`) answers `ModeActive`/`SkipValidation`/`NotifyEnabled`, `Notice` (`notice.go`) builds the reminder, `NoticeTracker` (`tracker.go`) keeps it to once per session via `$XDG_STATE_HOME/klaudiush/bypass_notice.json`.

**Reminder**: emitted in `systemMessage` only, so the AI never sees it and cannot act on it. Suppressed by `notify = false`, by an active skip, and after the first hook of a session.

**CLI**: `klaudiush bypass status|skip|enforce|notify` (`cmd/klaudiush/bypass.go`), with `--global`, `--reason`, and `--duration` mirroring the overrides commands.

**Documentation**: See `docs/BYPASS_GUIDE.md`. Example config in `examples/config/bypass-permissions.toml`.

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

**Precedence** (highest to lowest): CLI Flags → Env Vars (`KLAUDIUSH_*`) → Project Config (`.klaudiush/config.toml`) → Global Config (`$XDG_CONFIG_HOME/klaudiush/config.toml`) → Defaults

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

Logs to `$XDG_STATE_HOME/klaudiush/dispatcher.log` (default `~/.local/state/klaudiush/dispatcher.log`). Override with `KLAUDIUSH_LOG_FILE` env var. Levels: `--debug` (default), `--trace` (verbose). Use `BaseValidator.Logger()`.

### Path management (`internal/xdg/`)

All global/user-level paths go through `internal/xdg/`. Follows XDG Base Directory spec:

- Config: `$XDG_CONFIG_HOME/klaudiush/` (default `~/.config/klaudiush/`)
- Data: `$XDG_DATA_HOME/klaudiush/` (default `~/.local/share/klaudiush/`)
- State: `$XDG_STATE_HOME/klaudiush/` (default `~/.local/state/klaudiush/`)

Automatic migration from `~/.klaudiush/` on first run. Legacy fallback via `xdg.ResolveFile()`. Testable via `PathResolver` interface.

## Testing

Framework: Ginkgo/Gomega. Run: `mise exec -- go test -v ./pkg/parser -run TestBashParser`

**Mocks**: Generated via `mockgen` (uber-go/mock). Add `//go:generate mockgen -source=<file>.go -destination=<file>_mock.go -package=<pkg>` directive, then run `go generate ./...`. NEVER manually edit generated mock files.

## Development

**Tools** (mise): Go 1.26.0, golangci-lint 2.10.1, markdownlint-cli2 0.21.0. Run `mise install`. See `SETUP.md`.

**Linters** (`.golangci.yml`): Nil safety (nilnesserr, govet), completeness (exhaustive, gochecksumtype), quality (gocognit, goconst, cyclop, dupl)

**Error Handling**: NEVER use `fmt.Errorf`, `errors`, or `github.com/pkg/errors` - linter will reject. ALWAYS use `github.com/cockroachdb/errors` for error creation and wrapping

### Evidence Gate (`internal/evidence/`, `cmd/klaudiush/evidence_gate.go`, `cmd/klaudiush/evidence.go`)

Opt-in `[evidence]` (off by default). Each `[[evidence.checks]]` has `commands` (plain literal argv, validated by `evidence.Compile`), `paths`/`exclude` globs, `kind` (`test` digests covered file content; `review` digests the diff from `git merge-base <base> HEAD`, `base` required). The first hook of a session records a content-digest baseline per check (`hooksession` `evidence_baselines`; `unknown` when the first hook comes after a tool ran). At `TurnStop` (Claude/Codex Stop, Gemini AfterAgent) every repository the session has baselines for is judged with its own config; a check is required when the session used a non-read-only tool there (`evidence_touched`) and its digest differs from the baseline (a redefined check or an unfingerprintable baseline counts as `unknown`), and `evidence.Judge` accepts only a `passed` receipt on the current digest (latest, or the kept pass when a later run never finished; Claude `returnCodeInterpretation` counts as failed) (failed/running/canceled/stale/unverified/missing block with EVID001, still capped by `maxCompletionBlocks`). Receipts are repository-scoped in the state file. Sources: Claude `PreToolUse`/`PostToolUse(Failure)` of a command `evidence.MatchCommand` accepts (exact argv, only `cd <dir> &&` prefixes ending at the repo root), fingerprinted at start and end; or `klaudiush evidence run <check>`, which runs the check itself. `hook.ReportsCommandOutcome` is true only for Claude; Codex/Gemini need the verifier. `klaudiush evidence status`, `klaudiush doctor --category evidence`. Fingerprint failures (including directories git cannot read) are HOOK001 and block by default (`Policy.Resolve("evidence", ReasonState, true)`); store errors warn. Opt-in `[evidence.tool_phase]` (`requires` check names, `read_only_tools`, `writable_paths`; `evidence.CompilePhase`) is judged from receipts on every hook (`cmd/klaudiush/tool_phase.go`, no own state): Gemini `BeforeToolSelection` (`CanonicalEventToolSelection`, `EnforcementFilterTools`, `hook.FiltersTools` true only for Gemini) is answered in `answerToolSelection` with `toolConfig` AUTO + `allowedFunctionNames` while a prerequisite lacks a pass; Gemini `BeforeTool` denies calls outside the phase with EVID002 (shell only a single `klaudiush evidence run <prereq>|status` with no cd/dir_path via `Phase.AllowsVerifier`, file tools only `writable_paths` minus config dirs and prerequisite scripts; `filter_tools=false` skips the selection answer; phase errors do not fail config validation). `settings.InstallGeminiToolSelection` registers the event only when the phase is enabled. See `docs/EVIDENCE_GUIDE.md`.

### Policy Protection and MCP Trust (`internal/protection/`, `internal/validators/policy/`)

Opt-in `[protection]` and `[mcp_trust]` (both off by default; `PolicyValidatorFactory` registers them, no rule engine). `protection.NewSet(Options)` compiles protected paths: anywhere-rules (`.klaudiush/`, `klaudiush.toml`, `.claude/settings*.json`, `.claude/hooks/`, `.mcp.json`, `.codex/{hooks.json,config.toml}`, `.gemini/settings.json`), absolute rules (XDG config/state/data, legacy dir, binary, `~/.claude.json`, `$CODEX_HOME` files, Claude/Codex/Gemini managed paths, opencode plugin, configured hook files), scripts named by registered hook commands and evidence commands, plugins, `protection.paths`; `protection.allow` exempts. Matching uses clean and symlink-resolved spellings, hard links (`os.SameFile` when nlink > 1), Unicode case folding on darwin/windows. `Set.CheckCommand` walks `ParseResult` (FileWrites incl. `>|`, `&>`, `<>`, `>&file`; `Command.Dynamic`/`FileWrite.Dynamic`/`DynamicWrites` mark `$(...)` parts the rendered args drop) with read-only/dest-only/flag-aware program classes, globs/braces/unknown parts as regexes, and a "mentions a protected path anywhere" rule for targets known only at run time; `PolicyCommand` blocks mutating klaudiush subcommands. `ToolTargets` covers Write/Edit/MultiEdit/NotebookEdit/apply_patch (lenient header regex) and path-like strings of other tools. Truncated/unparseable commands fail closed (POL001). ConfigChange (new `CanonicalEventConfigChange`, Claude `decision:block`, `policy_settings` never blocked) yields POL002; klaudiush policy commands POL003. `MCPTrustValidator` trusts by `hook.Context.MCPServer` (Claude `mcp_server{name,source}`, Gemini `mcp_context` transport), never the `mcp__<server>__` prefix: MCP004 untrusted, MCP005 no provenance (`unknown_provenance`). POL001-003/MCP004-005 need an explicit `[exceptions.policies.<CODE>]` (`exceptions.RequiresExplicitPolicy`), and enabled guards are critical (`buildPolicy` → `Policy.WithCritical`). Git commands that rewrite the work tree without paths (clean, stash, reset --hard, checkout/switch/merge/rebase/cherry-pick/revert <rev>, apply/am, `patch`) are checked by querying git (`gitrewrite.go`). `inheritPolicyGuards` keeps the guards on when the hook cwd or `CLAUDE_PROJECT_DIR`/`GEMINI_PROJECT_DIR` config enables them while config loaded from a cd target does not. Doctor: `klaudiush doctor --category protection` (ConfigChange registration, MCP matcher, provenance coverage, managed hooks). See `docs/PROTECTION_GUIDE.md`.

### Outcome Metrics (`internal/metrics/`, `cmd/klaudiush/metrics.go`, `cmd/klaudiush/metrics_hook.go`)

`[metrics]` on by default. After the response is written, `hookRun.recordMetrics` appends one JSONL record to `xdg.MetricsFile()` under `filelock` (250ms timeout; a failure only drops the sample). The class comes from the written response (`hookresponse.Stops`) and the event: a stop before the tool is `prevented`, at a completion gate `held`; after a tool, or when the response could not stop, `advisory`. Records carry codes, `token`-sanitized validator/event names, HMAC-salted session/resource keys, `dispatcher.Outcome.Checks` (repair detection) and `Timings`; never commands, messages, paths or session IDs. Size cap rotates into one `.1` backup. `metrics.Summarize` replays records to follow violations (session, validator, resource, code) to repair. `klaudiush doctor --category metrics`. See `docs/METRICS_GUIDE.md`.

## Hook output

klaudiush always exits 0. Validation results are JSON on stdout:

- `0`: JSON stdout (pass, deny, or warning). No output for clean pass. Also every failure klaudiush catches during a hook run (malformed input, config error, panic, deadline): it answers "Validation unavailable" (HOOK001) per the failure policy, because every provider lets the action through on a non-zero exit or timeout.
- `3`: Crash outside a hook's validation (crash dump created, stderr only)

**Failure policy** (`internal/failpolicy/`, `[failure_policy]`, `cmd/klaudiush/hook_failure.go`): unavailable checks carry `Result.UnavailableReason` (missing_tool, timeout, canceled, panic, malformed_output, malformed_input, config, state, error); build them with `validator.Unavailable(reason, msg)`, never a plain Pass/Fail. The dispatcher resolves each via `Policy.Resolve`: `critical` blocks, missing tools follow `missing_tools` (default ignore), else `mode` (warn/block), else the check's own choice. Executors recover validator panics, report validators skipped or passing after ctx ended as unavailable. `hookRun.supervise` runs validation under a deadline (default 20s, below the 30s registered hook timeout) with a watchdog answering 3s later; `--failure-mode` and `KLAUDIUSH_FAILURE_POLICY_MODE` apply when config cannot load. File validators use `lintUnavailable`. See `docs/FAILURE_POLICY_GUIDE.md`.

JSON fields: `hookSpecificOutput.permissionDecision` (`"deny"` only; warnings and accepted exceptions omit it so the harness permission flow still decides, never `"allow"`), `permissionDecisionReason` (shown to Claude), `additionalContext` (behavioral framing), `systemMessage` (human-readable).

`[output]` config toggles these: `user_messages = false` hides every `systemMessage` (master), `validation_messages = false` hides only validation details, `agent_summary` (default on) makes `hookresponse.AppendAgentSummary` add a one-sentence-explanation instruction to `additionalContext` on Claude denials only (advisory after-tool results let the action through, other providers drop `additionalContext` on deny).

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

## Protection Documentation

Guide available in `docs/PROTECTION_GUIDE.md` with commented examples in `examples/config/protection.toml` and `examples/config/mcp-trust.toml`. Doctor: `klaudiush doctor --category protection`.

## Failure Policy Documentation

Guide available in `docs/FAILURE_POLICY_GUIDE.md` with a commented example in `examples/config/failure-policy.toml`. Doctor: `klaudiush doctor --category failure_policy`.

## Bypass Permissions Documentation

Guide available in `docs/BYPASS_GUIDE.md` with a commented example in `examples/config/bypass-permissions.toml`.

Inspect the current setting with: `klaudiush bypass status --all`

## Harness Testing Documentation

Live checks against installed Claude Code, Codex and opencode, plus the CI contract checks over captured fixtures, are in `internal/harness/` and documented in `docs/HARNESS_TESTING_GUIDE.md` (version matrix, isolation rules, unsupported paths, known gaps). The live suite builds the sandbox environment from scratch and drives a scripted local model, so it needs no credentials; never point it at the real HOME.

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
dump_dir = "~/.local/share/klaudiush/crash_dumps"  # Storage location
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

## Claude Code skills

The `git-stage-hunk` SAI plugin stages partial file changes without a TTY. Use `/stage-hunk` when only some changes in a file belong in the current commit, multiple sessions modified the same file, or `git add -p` is unavailable.

Install: `claude --plugin-dir ~/Projects/github.com/smykla-skalski/sai/git-stage-hunk/`
Modes: `--list`, `--hunk H1,H2`, `--pattern REGEX`, `--file PATH`, `--range FILE:S-E`, `--verify`, `--dry-run`
