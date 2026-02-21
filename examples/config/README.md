# Configuration examples

This directory contains example configurations for klaudiush.

## Files

### minimal.toml

The simplest possible configuration showing how to disable specific validators. Good for quick setup when you want to turn off validators you don't need.

Place in `~/.klaudiush/config.toml` (global) or `.klaudiush/config.toml` (project).

### full.toml

Complete configuration showing all available options with their default values. Use as a reference or starting point for customization.

Place in `~/.klaudiush/config.toml` (global).

### project-override.toml

Example showing how project-specific configuration overrides global settings. Use to customize validation rules for a specific project while keeping global defaults.

Place in `.klaudiush/config.toml` or `klaudiush.toml` (project root).

### comprehensive-backticks.toml

Comprehensive backtick validation configuration for all Bash commands. Catches command substitution issues in all Bash commands, not just git/gh. Detects unquoted backticks, backticks in double quotes, and suggests single quotes when no variables are present.

Place in `~/.klaudiush/config.toml` (global) or `.klaudiush/config.toml` (project).

### javascript.toml

JavaScript/TypeScript project configuration with oxlint validation.

Place in `.klaudiush/config.toml` (project).

### rust.toml

Rust project configuration with rustfmt validation.

Place in `.klaudiush/config.toml` (project).

## Configuration hierarchy

Configuration is loaded from multiple sources with the following precedence (highest to lowest):

1. CLI flags (e.g., `--disable=commit,markdown`)
2. Environment variables (e.g., `KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false`)
3. Project config (`.klaudiush/config.toml` > `klaudiush.toml`)
4. Global config (`~/.klaudiush/config.toml`)
5. Defaults (built-in defaults)

## Usage

### Global configuration

Copy a configuration file to `~/.klaudiush/config.toml`:

```bash
mkdir -p ~/.klaudiush
cp examples/config/full.toml ~/.klaudiush/config.toml
```

Edit the file to customize your global settings.

### Project configuration

Copy a configuration file to your project root:

```bash
mkdir -p .klaudiush
cp examples/config/project-override.toml .klaudiush/config.toml
```

Or use the root-level filename:

```bash
cp examples/config/project-override.toml klaudiush.toml
```

### CLI flags

Override configuration at runtime:

```bash
# Use custom config file
klaudiush --config=./my-config.toml

# Disable specific validators
klaudiush --disable=commit,markdown

# Use custom global config
klaudiush --global-config=~/.config/klaudiush.toml
```

### Environment variables

Set environment variables to override configuration:

```bash
# Disable commit validator
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false

# Change commit title max length
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH=72

# Disable Markdown validation
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED=false
```

## Deep merge example

Given:

Global config (`~/.klaudiush/config.toml`):

```toml
[validators.git.commit.message]
title_max_length = 50
check_conventional_commits = true
```

Project config (`.klaudiush/config.toml`):

```toml
[validators.git.commit.message]
title_max_length = 72
```

Result (merged):

```toml
[validators.git.commit.message]
title_max_length = 72              # From project config
check_conventional_commits = true  # From global config
```

## Tips

1. Start with `minimal.toml` and add options as needed
2. Set your preferred defaults in `~/.klaudiush/config.toml`
3. Use project config to customize for specific repositories
4. Use environment variables in CI/CD pipelines
5. Use CLI flags for one-off changes during testing

## Validation

Invalid configurations will fail with clear error messages:

```bash
$ klaudiush
Error: failed to load configuration: invalid configuration:
  validators.git.commit.message.title_max_length: must be positive
```

## No configuration

klaudiush works without any configuration files. All validators are enabled with sensible defaults.

## Performance

Configuration loading is optimized for fast startup:

| Scenario           | Time  | Notes                                  |
| ------------------ | ----- | -------------------------------------- |
| Cache hit          | ~5ns  | Subsequent loads within same session   |
| Defaults only      | ~7µs  | No config files                        |
| Single config file | ~25µs | Global or project only                 |
| All sources        | ~45µs | Global + project + env + flags         |

Findings:

- Cache provides ~200x speedup - config is cached after first load
- File I/O dominates - each config file adds ~17-21µs
- Env vars are fast - parsing adds only ~1-2µs
- CLI flags are fastest - ~100-300ns overhead

Run benchmarks yourself:

```bash
# Human-readable report
go test -v -run TestConfigPerformanceReport ./internal/config/provider/

# Go benchmarks with memory stats
go test -bench=. -benchmem -run=NONE ./internal/config/provider/
```
