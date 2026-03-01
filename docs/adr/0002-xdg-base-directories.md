# 2. Adopt XDG Base Directory specification for path management

* Status: accepted
* Deciders: @bartsmykla
* Date: 2026-02-21

## Context and problem statement

All klaudiush file paths are hardcoded to `~/.klaudiush/` or `~/.claude/hooks/`. Each component (exceptions, crashdump, patterns, plugins) has its own tilde expansion code - five duplicate implementations across the codebase. There is no single place that manages paths on the machine, making it hard to reason about where files end up or to change the layout.

The [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/) is the standard convention for organizing user-level files by purpose (config vs data vs state vs cache). Most CLI tools on Linux and macOS follow it.

## Decision drivers

* Eliminate five duplicate tilde expansion implementations
* Follow a convention users already expect from other CLI tools
* Separate config files from runtime data and state
* Make paths testable without manipulating `$HOME`

## Considered options

* Keep `~/.klaudiush/` as the single directory for everything
* Adopt XDG with automatic migration from legacy paths

## Decision outcome

Chosen option: "Adopt XDG with automatic migration," because it solves all four drivers. A new `internal/xdg/` package becomes the single source of truth for every path klaudiush touches on disk.

### XDG mapping

| XDG var | Default | klaudiush subdir |
|---------|---------|------------------|
| `XDG_CONFIG_HOME` | `~/.config` | `klaudiush/` |
| `XDG_DATA_HOME` | `~/.local/share` | `klaudiush/` |
| `XDG_STATE_HOME` | `~/.local/state` | `klaudiush/` |

### Path changes

| What | Old path | New path |
|------|----------|----------|
| Global config | `~/.klaudiush/config.toml` | `$XDG_CONFIG_HOME/klaudiush/config.toml` |
| Log file | `~/.claude/hooks/dispatcher.log` | `$XDG_STATE_HOME/klaudiush/dispatcher.log` |
| Exception state | `~/.klaudiush/exceptions/state.json` | `$XDG_DATA_HOME/klaudiush/exceptions/state.json` |
| Exception audit | `~/.klaudiush/exception_audit.jsonl` | `$XDG_STATE_HOME/klaudiush/exception_audit.jsonl` |
| Crash dumps | `~/.klaudiush/crash_dumps/` | `$XDG_DATA_HOME/klaudiush/crash_dumps/` |
| Patterns (global) | `~/.klaudiush/patterns/` | `$XDG_DATA_HOME/klaudiush/patterns/` |
| Backups | `~/.klaudiush/.backups/` | `$XDG_DATA_HOME/klaudiush/backups/` |
| Plugins | `~/.klaudiush/plugins/` | `$XDG_DATA_HOME/klaudiush/plugins/` |

Project-local paths (`.klaudiush/config.toml`, `.klaudiush/patterns.json`) are unchanged.

### Migration behavior

On first run after upgrade, klaudiush checks for `~/.klaudiush/` and migrates:

1. Moves files to their XDG locations
2. Creates symlinks at `~/.klaudiush/config.toml` and `~/.claude/hooks/dispatcher.log` pointing to XDG paths
3. Writes a migration marker at `$XDG_STATE_HOME/klaudiush/.migration_v2`
4. Idempotent - skips files that already exist at destination, second run is a no-op

### Backward compatibility

`xdg.ResolveFile(xdgPath, legacyPath)` checks XDG location first, falls back to legacy if the file exists only there. New files always go to XDG paths.

### Testability

`PathResolver` interface with `DefaultResolver()` for production and `ResolverFor(homeDir)` for tests. No `$HOME` manipulation needed.

### Positive consequences

* Single package owns all paths - no more scattered path construction
* Five tilde expansion implementations replaced by one
* `KLAUDIUSH_LOG_FILE` env var for custom log location
* XDG env vars work as expected (`XDG_CONFIG_HOME`, etc.)
* `klaudiush doctor --category xdg` detects and fixes path issues

### Negative consequences

* Symlinks left behind at legacy locations after migration
* Users who scripted against `~/.klaudiush/` paths need to update (symlinks cover config and log)

## Links

* [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/)
