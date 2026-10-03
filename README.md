# Klaudiush

[![CI](https://github.com/smykla-skalski/klaudiush/actions/workflows/ci.yml/badge.svg)](https://github.com/smykla-skalski/klaudiush/actions/workflows/ci.yml)
[![CodeQL](https://github.com/smykla-skalski/klaudiush/actions/workflows/codeql.yml/badge.svg)](https://github.com/smykla-skalski/klaudiush/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/smykla-skalski/klaudiush/badge)](https://scorecard.dev/viewer/?uri=github.com/smykla-skalski/klaudiush)
[![Go Report Card](https://goreportcard.com/badge/github.com/smykla-skalski/klaudiush)](https://goreportcard.com/report/github.com/smykla-skalski/klaudiush)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/smykla-skalski/klaudiush)](https://github.com/smykla-skalski/klaudiush/releases/latest)

A validation dispatcher for AI coding-agent hooks. Klaudiush supports Claude Code hooks today, experimental Codex command hooks, and an opencode bridge plugin, enforcing git workflow standards, commit conventions, and code quality rules.

For Claude, klaudiush runs in blocking `before_tool` flows (`PreToolUse`). For Codex, it can also participate in experimental `session_start`, `after_tool`, and `turn_stop` command hooks. For opencode, it blocks in `tool.execute.before` and observes the session lifecycle. It parses Bash commands via `mvdan.cc/sh`, detects file operations, and validates them against project-specific rules.

- Git workflow validation (commits, pushes, branches, PRs)
- Code quality checks (shellcheck, terraform fmt, actionlint, gofumpt, ruff, oxlint, rustfmt)
- Bash AST parsing for command chains, pipes, subshells, redirections
- File write detection and path protection
- Secret detection (25+ patterns, optional gitleaks integration)
- Dynamic validation rules via TOML

## Installation

### Homebrew

```bash
brew install smykla-skalski/tap/klaudiush
```

### Install script

```bash
curl -sSfL https://klaudiu.sh/install.sh | sh

# Specific version or custom directory
curl -sSfL https://klaudiu.sh/install.sh | sh -s -- -v v1.0.0
curl -sSfL https://klaudiu.sh/install.sh | sh -s -- -b /usr/local/bin
```

### Nix

```bash
nix run github:smykla-skalski/klaudiush?dir=nix
nix profile install github:smykla-skalski/klaudiush?dir=nix
```

Home Manager module:

```nix
{
  inputs.klaudiush.url = "github:smykla-skalski/klaudiush?dir=nix";
}

{
  imports = [ inputs.klaudiush.homeManagerModules.default ];
  programs.klaudiush.enable = true;
}
```

### Build from source

```bash
mise run build && mise run install
```

### Setup

After installing, register the hooks and verify:

```bash
klaudiush init --global
klaudiush doctor
```

The binary installs to `~/.local/bin` or `~/bin`. Make sure the install directory is in your `$PATH`.

Shell completions are available for bash, zsh, fish, and PowerShell via `klaudiush completion <shell>`.

### Providers

Claude is enabled by default. The other providers are opt-in:

```toml
[providers.claude]
enabled = true

[providers.codex]
enabled = true
experimental = true
hooks_config_path = "~/.codex/hooks.json"

[providers.gemini]
enabled = true
settings_path = "~/.gemini/settings.json"

[providers.opencode]
enabled = true
# Optional; defaults to ~/.config/opencode/plugin/klaudiush.ts
plugin_path = "~/.config/opencode/plugin/klaudiush.ts"
```

Claude, Codex, and Gemini read hooks from JSON settings files, so klaudiush
registers commands inside the file you point it at. opencode has no declarative
hook config — hooks are TypeScript plugins — so klaudiush instead generates a
bridge plugin at `plugin_path` that forwards opencode's hooks to the validator.

Write or refresh the integrations with either command:

```bash
klaudiush init --install-hooks --global
klaudiush doctor --fix
```

Regenerate the opencode plugin after upgrading klaudiush, since it embeds the
resolved binary path. `klaudiush doctor` reports a stale plugin as an
unregistered dispatcher.

opencode 1.x and 2.x load plugins through different APIs, and each rejects the
other's plugin with only a log warning, after which every tool runs unchecked.
Both commands run `opencode --version` (from `PATH`, or
`~/.opencode/bin/opencode`) and write the matching bridge. When opencode is
not found they keep the API of the plugin already installed, or write the 1.x
bridge on a fresh install, and `klaudiush doctor` warns that the bridge is
unverified. Rerun
`klaudiush doctor --fix` after upgrading opencode across a major version;
`klaudiush doctor` reports a bridge the installed opencode rejects as an error.

For Codex, klaudiush registers `SessionStart`, `PreToolUse` (no matcher, so
shell, `apply_patch`, MCP, and local function tools are all checked before they
run), and `Stop`. Each file in an `apply_patch` is checked on its own: an added
file like a write of that file, a single-hunk update like an edit, and other
updates, deletions, and move targets through their path and added lines. A pre-tool denial uses `permissionDecision: "deny"`; Codex
reports any other field on `PreToolUse` as a hook failure and runs the tool
anyway, so klaudiush never sends one. Hosted tools such as web search never
reach hooks. Codex only runs new or changed hooks after you trust them in
`/hooks`. Re-running the install migrates entries from the retired
`AfterToolUse` event and adds a synchronous matcherless `PreToolUse` handler
when the existing one is async or narrowed by a matcher. `klaudiush doctor`
reports which tool calls are actually blocked, not just whether a hook is
registered; it counts MCP or local function tools as blocked only when the
matcher covers the whole family, not just a few named tools.

Only `tool.execute.before` can refuse a call in opencode, by aborting the tool.
`tool.execute.after` and the compaction hook can add text the model reads;
opencode's remaining hooks expose no such channel, so findings on those reach
you as a notification instead.

## How it works

```text
Provider Hook JSON -> CLI -> JSON Parser -> Dispatcher -> Registry -> Validators -> Result
```

Claude Code and Codex send hook payloads as JSON on stdin. Klaudiush normalizes the provider payload, matches it against registered validators using a predicate system, and returns a result: pass (no output), deny/block (JSON on stdout), or warn/advisory context. Exit code is always 0. On crash, exit code 3 with panic info on stderr.

Validators register with predicates that control when they fire:

```go
registry.Register(validator, validator.And(
    validator.EventIs(hook.CanonicalEventBeforeTool),
    validator.ToolTypeIs(hook.Bash),
    validator.CommandContains("git commit"),
))
```

Available predicates: `EventIs`, `EventTypeIs`, `ProviderIs`, `ToolTypeIs`, `CommandContains`, `FileExtensionIs`, `FilePathMatches`, `And`, `Or`, `Not`.

### Validators

Git validators handle commit message format (conventional commits, <=50 char title, <=72 char body), required flags (`-sS`), branch naming (`type/description`), push policies, PR validation (title, body, changelog), and staging rules.

File validators run shellcheck, terraform/tofu fmt + tflint, GitHub Actions digest pinning + actionlint, gofumpt, ruff, oxlint, and rustfmt. Markdown formatting is checked too.

Secrets detection covers 25+ regex patterns for AWS keys, GitHub tokens, private keys, and connection strings. Optional gitleaks integration with configurable allow lists.

Shell validators detect backticks in commit/PR commands, with an optional comprehensive mode for all Bash commands.

A notification validator rings the terminal bell on permission prompts (dock bounce on macOS).

## Configuration

No configuration is required. All validators have working defaults.

Klaudiush uses TOML configuration with this precedence (highest first):

1. CLI flags (`--disable=commit,markdown`)
2. Environment variables (`KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false`)
3. Project config (`.klaudiush/config.toml`)
4. Global config (`$XDG_CONFIG_HOME/klaudiush/config.toml`)
5. Built-in defaults

Sources are deep-merged - nested values merge rather than replace.

```toml
# Disable commit validation
[validators.git.commit]
enabled = false

# Allow longer titles
[validators.git.commit.message]
title_max_length = 72

# Downgrade shellscript to warning
[validators.file.shellscript]
severity = "warning"
```

All validators support `enabled` (on/off) and `severity` ("error" to block, "warning" to log only). Git validators add options for message format, required flags, branch naming, and push policies. File validators add timeouts and per-linter configuration.

See [`examples/config/`](examples/config/) for complete examples with all options.

### Dynamic rules

The rule engine lets you block, warn, or allow operations based on patterns without code changes:

```toml
[rules]
enabled = true

[[rules.rules]]
name = "block-main-push"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"

[rules.rules.action]
type = "block"
message = "Direct push to main is not allowed. Use a pull request."
```

Rules support glob and regex patterns (auto-detected), priority ordering, validator scoping (`git.push` or `git.*`), and negation. See the [rules guide](docs/RULES_GUIDE.md) and [`examples/rules/`](examples/rules/).

### Exception workflow

Bypass a validation block by adding an exception token to the command:

```bash
git push origin main  # EXC:GIT019:Emergency+hotfix
```

Exceptions require explicit policy configuration per error code, enforce rate limits, and log to an audit trail. See the [exceptions guide](docs/EXCEPTIONS_GUIDE.md).

Exceptions only apply to blocking `before_tool` command flows. They are not used for Codex lifecycle hooks.

### Skipping approval prompts

Sessions started with `--dangerously-skip-permissions` (Claude), `--dangerously-bypass-approvals-and-sandbox` (Codex), or `--yolo` (Gemini) are still validated. Skipping prompts says how much you want to be asked, not which commit conventions apply.

```bash
klaudiush bypass status              # What happens in those modes right now
klaudiush bypass skip --duration 4h  # Opt out, expires on its own
klaudiush bypass skip --global       # Opt out everywhere
klaudiush bypass enforce             # Back to the default
klaudiush bypass notify off          # Keep validating, hide the reminder
```

While validating such a session, klaudiush shows a reminder in `systemMessage` once per session. That field goes to you, not to the model, so the agent cannot act on it. See the [bypass guide](docs/BYPASS_GUIDE.md).

### When validation cannot run

A timed-out linter, a failing plugin, unreadable hook input, a broken configuration, a crash, or a run past the deadline is reported as "Validation unavailable" (HOOK001), never as a pass. By default these warn, plugin failures block, and missing linters stay ignored (`missing_tools`). Make them block with:

```toml
[failure_policy]
mode = "block"
critical = ["git.commit", "secrets"]
```

klaudiush always answers with exit code 0 and a response the provider honors, because every provider lets the action through when a hook exits non-zero or times out. See the [failure policy guide](docs/FAILURE_POLICY_GUIDE.md) for deadlines, provider behavior, and what hooks cannot guarantee.

### Requiring fresh test and review results

Opt in to keep an agent from finishing until required checks passed against the files as they are now:

```toml
[evidence]
enabled = true

[[evidence.checks]]
name = "tests"
commands = ["mise run test"]
paths = ["**/*.go", "go.mod", "go.sum"]
```

When a session changed covered files, the completion gate (Claude `Stop`, Codex `Stop`, Gemini `AfterAgent`) blocks with EVID001 until the check passed on exactly that content. Results are tied to a digest of the covered files, so a later edit makes them stale; failed, running, canceled and background runs never count; review checks record the exact diff they reviewed. Read-only sessions and changes the check does not cover are not gated. Claude reports how a shell command ended, so running a check's command there counts; in every provider, `klaudiush evidence run tests` runs the check and records its exit status itself. See the [evidence guide](docs/EVIDENCE_GUIDE.md).

Add `[evidence.tool_phase]` with `requires = ["plan"]` to keep Gemini from changing files until those checks pass: `BeforeToolSelection` offers only read-only tools and the verifier, and `BeforeTool` denies anything else with EVID002. Other providers have no tool-selection event and are not restricted. See [Gemini tool phases](docs/EVIDENCE_GUIDE.md#gemini-tool-phases).

### Protecting policy files and trusting MCP servers

Opt in to keep the agent from editing what enforces policy on it, and to trust MCP servers by where the harness says they came from:

```toml
[protection]
enabled = true

[mcp_trust]
enabled = true
trusted_sources = ["managed", "user"]
unknown_provenance = "warn"
```

Protection blocks (POL001) writes, edits, patches, MCP tool calls and shell commands that would change klaudiush configuration or state, hook registrations of every harness (`.claude/settings*.json`, `~/.codex/hooks.json`, `.gemini/settings.json`, managed settings), hook and evidence check scripts, or the binary, following symlinks, hard links, case-insensitive spellings, globs, variables and `cd` chains; commands klaudiush cannot inspect fail closed. Claude `ConfigChange` keeps settings changed mid-session from taking effect (POL002), and klaudiush commands that change policy, such as `bypass skip` or `disable`, are blocked (POL003). MCP trust reads Claude's `mcp_server` source and Gemini's `mcp_context` transport instead of the spoofable `mcp__<server>__` name (MCP004), with a configured action for calls without provenance (MCP005). Maintenance is authorized explicitly with `protection.allow` or an exception policy for the code. See the [protection guide](docs/PROTECTION_GUIDE.md).

### Measuring what was enforced

Every hook appends one redacted line to a local, size-capped log. `klaudiush metrics` reports what the responses actually did by provider and event (prevented, completion gate held, advisory, warned, exception accepted, unavailable), repair retries and recurring violations per code, checks that could not run, and hook and validator latency:

```bash
klaudiush metrics report --since 7d
```

Findings after a tool ran are advisory and never count as prevented. No command, message, path or session ID is stored, and nothing leaves the machine. Turn it off with `[metrics] enabled = false`. See the [metrics guide](docs/METRICS_GUIDE.md).

### Checking real harnesses

`mise run test:harness` runs the installed Claude Code, Codex and opencode binaries against a scripted local model in a disposable home directory and checks the file system: a denied call must leave no trace, a warning must leave the harness permission flow in charge, and the completion gate must keep the agent working. It records the exact harness versions and captures the hook payloads it saw as fixtures, which every `mise run test` checks against the provider capability table. It needs no credentials and never runs in CI. See the [harness testing guide](docs/HARNESS_TESTING_GUIDE.md) for the version matrix and the paths no hook can see.

## Performance

End-to-end binary execution on Apple M3 Max (hyperfine, 30 runs, CLI git backend):

| Payload                      | Time          |
|------------------------------|---------------|
| Baseline (empty stdin)       | 59ms +/- 7ms  |
| Non-git bash                 | 68ms +/- 6ms  |
| Git commit (full validation) | 112ms +/- 6ms |
| Git push                     | 87ms +/- 4ms  |

Internal micro-benchmarks: JSON parse 0.5-3.6us, Bash AST parse 1.4-23us, dispatcher overhead 1.2-1.8us per dispatch.

The default git SDK backend (go-git/v6) is 2-5.9M times faster than CLI for cached operations. Set `KLAUDIUSH_USE_SDK_GIT=false` to use the CLI fallback.

```bash
mise run bench             # in-process micro-benchmarks
mise run bench:hyperfine   # end-to-end comparison
```

## Development

```bash
mise run test       # all tests
mise run verify     # fmt + lint + test
mise run check      # lint + auto-fix
```

Add validators in `internal/validators/{category}/`, implement `Validate()`, register in `cmd/klaudiush/main.go` with predicates. Logs go to `$XDG_STATE_HOME/klaudiush/dispatcher.log`.

The project uses [Lefthook](https://github.com/evilmartians/lefthook) for git hooks. Run `mise run install:hooks` to set up pre-commit (staged files only) and pre-push (full suite) hooks.

## Contributing

1. Create a feature branch (`feat/my-feature`)
2. Write tests first
3. Run `mise run verify`
4. Open a PR with a semantic title

## Support

- [Issues](https://github.com/smykla-skalski/klaudiush/issues)
- [Discussions](https://github.com/smykla-skalski/klaudiush/discussions)

## License

MIT - Copyright (c) 2025 Smykla Skalski Labs. See [LICENSE](LICENSE).
