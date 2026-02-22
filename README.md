# Klaudiush

[![CI](https://github.com/smykla-skalski/klaudiush/actions/workflows/ci.yml/badge.svg)](https://github.com/smykla-skalski/klaudiush/actions/workflows/ci.yml)
[![CodeQL](https://github.com/smykla-skalski/klaudiush/actions/workflows/codeql.yml/badge.svg)](https://github.com/smykla-skalski/klaudiush/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/smykla-skalski/klaudiush/badge)](https://scorecard.dev/viewer/?uri=github.com/smykla-skalski/klaudiush)
[![Go Report Card](https://goreportcard.com/badge/github.com/smykla-skalski/klaudiush)](https://goreportcard.com/report/github.com/smykla-skalski/klaudiush)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/smykla-skalski/klaudiush)](https://github.com/smykla-skalski/klaudiush/releases/latest)

A validation dispatcher for Claude Code hooks. Intercepts tool invocations before execution and enforces git workflow standards, commit conventions, and code quality rules.

Klaudiush runs as a PreToolUse hook in Claude Code. It parses Bash commands via `mvdan.cc/sh`, detects file operations, and validates them against project-specific rules - all before the tool runs.

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

After installing, register the hook and verify:

```bash
klaudiush init --global
klaudiush doctor
```

The binary installs to `~/.local/bin` or `~/bin`. Make sure the install directory is in your `$PATH`.

Shell completions are available for bash, zsh, fish, and PowerShell via `klaudiush completion <shell>`.

## How it works

```text
Claude Code JSON -> CLI -> JSON Parser -> Dispatcher -> Registry -> Validators -> Result
```

Claude Code sends tool invocations as JSON on stdin. Klaudiush parses the payload, matches it against registered validators using a predicate system, and returns a result: pass (no output), deny (JSON on stdout), or warn (allows with context). Exit code is always 0. On crash, exit code 3 with panic info on stderr.

Validators register with predicates that control when they fire:

```go
registry.Register(validator, validator.And(
    validator.EventTypeIs(hook.PreToolUse),
    validator.ToolTypeIs(hook.Bash),
    validator.CommandContains("git commit"),
))
```

Available predicates: `EventTypeIs`, `ToolTypeIs`, `CommandContains`, `FileExtensionIs`, `FilePathMatches`, `And`, `Or`, `Not`.

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
