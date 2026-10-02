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

A missing or timed-out linter, a failing plugin, unreadable hook input, a broken configuration, a crash, or a run past the deadline is reported as "Validation unavailable" (HOOK001), never as a pass. By default these warn and plugin failures block; make them block with:

```toml
[failure_policy]
mode = "block"
critical = ["git.commit", "secrets"]
```

klaudiush always answers with exit code 0 and a response the provider honors, because every provider lets the action through when a hook exits non-zero or times out. See the [failure policy guide](docs/FAILURE_POLICY_GUIDE.md) for deadlines, provider behavior, and what hooks cannot guarantee.

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
