# Configuration Examples

This directory contains example configurations for klaudiush.

## Files

### minimal.toml

The simplest possible configuration showing how to disable specific validators.

**Use case**: Quick setup to disable validators you don't need.

**Location**: `~/.klaudiush/config.toml` (global) or `.klaudiush/config.toml` (project)

### full.toml

Complete configuration showing all available options with their default values.

**Use case**: Reference for all configuration options, starting point for customization.

**Location**: `~/.klaudiush/config.toml` (global)

### project-override.toml

Example showing how project-specific configuration overrides global settings.

**Use case**: Customize validation rules for a specific project while keeping global defaults.

**Location**: `.klaudiush/config.toml` or `klaudiush.toml` (project root)

### comprehensive-backticks.toml

Comprehensive backtick validation configuration for all Bash commands.

**Use case**: Strict shell safety across your entire workflow, catching command substitution issues in all Bash commands (not just git/gh).

**Location**: `~/.klaudiush/config.toml` (global) or `.klaudiush/config.toml` (project)

**Features**:

- Validates ALL Bash commands (not just git commit, gh pr/issue create)
- Detects unquoted backticks (e.g., `echo \`date\``)
- Detects backticks in double quotes
- Suggests single quotes when no variables are present
- Provides context-specific fix suggestions

### javascript.toml

JavaScript/TypeScript project configuration with oxlint validation.

**Use case**: JavaScript and TypeScript projects.

**Location**: `.klaudiush/config.toml` (project)

### rust.toml

Rust project configuration with rustfmt validation.

**Use case**: Rust projects.

**Location**: `.klaudiush/config.toml` (project)

## Configuration Hierarchy

Configuration is loaded from multiple sources with the following precedence (highest to lowest):

1. **CLI Flags** (e.g., `--disable=commit,markdown`)
2. **Environment Variables** (e.g., `KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false`)
3. **Project Config** (`.klaudiush/config.toml` > `klaudiush.toml`)
4. **Global Config** (`~/.klaudiush/config.toml`)
5. **Defaults** (built-in defaults)

## Usage

### Global Configuration

Copy a configuration file to `~/.klaudiush/config.toml`:

```bash
mkdir -p ~/.klaudiush
cp examples/config/full.toml ~/.klaudiush/config.toml
```

Edit the file to customize your global settings.

### Project Configuration

Copy a configuration file to your project root:

```bash
mkdir -p .klaudiush
cp examples/config/project-override.toml .klaudiush/config.toml
```

Or use the root-level filename:

```bash
cp examples/config/project-override.toml klaudiush.toml
```

### CLI Flags

Override configuration at runtime:

```bash
# Use custom config file
klaudiush --config=./my-config.toml

# Disable specific validators
klaudiush --disable=commit,markdown

# Use custom global config
klaudiush --global-config=~/.config/klaudiush.toml
```

### Environment Variables

Set environment variables to override configuration:

```bash
# Disable commit validator
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED=false

# Change commit title max length
export KLAUDIUSH_VALIDATORS_GIT_COMMIT_MESSAGE_TITLE_MAX_LENGTH=72

# Disable Markdown validation
export KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_ENABLED=false
```

## Deep Merge Example

Given:

**Global config** (`~/.klaudiush/config.toml`):

```toml
[validators.git.commit.message]
title_max_length = 50
check_conventional_commits = true
```

**Project config** (`.klaudiush/config.toml`):

```toml
[validators.git.commit.message]
title_max_length = 72
```

**Result** (merged):

```toml
[validators.git.commit.message]
title_max_length = 72              # From project config
check_conventional_commits = true  # From global config
```

## Tips

1. **Start with minimal**: Begin with `minimal.toml` and add options as needed
2. **Use global for defaults**: Set your preferred defaults in `~/.klaudiush/config.toml`
3. **Override per project**: Use project config to customize for specific repositories
4. **Environment for CI**: Use environment variables in CI/CD pipelines
5. **CLI for quick tests**: Use CLI flags for one-off changes during testing

## Validation

Invalid configurations will fail with clear error messages:

```bash
$ klaudiush
Error: failed to load configuration: invalid configuration:
  validators.git.commit.message.title_max_length: must be positive
```

## No Configuration

klaudiush works without any configuration files. All validators are enabled with sensible defaults.

## Performance

Configuration loading is optimized for fast startup:

| Scenario           | Time  | Notes                                  |
| ------------------ | ----- | -------------------------------------- |
| Cache hit          | ~5ns  | Subsequent loads within same session   |
| Defaults only      | ~7µs  | No config files                        |
| Single config file | ~25µs | Global or project only                 |
| All sources        | ~45µs | Global + project + env + flags         |

Key findings:

- **Cache provides ~200x speedup** - config is cached after first load
- **File I/O dominates** - each config file adds ~17-21µs
- **Env vars are fast** - parsing adds only ~1-2µs
- **CLI flags are fastest** - ~100-300ns overhead

Run benchmarks yourself:

```bash
# Human-readable report
go test -v -run TestConfigPerformanceReport ./internal/config/provider/

# Go benchmarks with memory stats
go test -bench=. -benchmem -run=NONE ./internal/config/provider/
```
