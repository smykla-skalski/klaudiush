# FILE005: Markdown lint validation failed

## Error

Markdown file has formatting issues that may affect rendering.

## Why this matters

Inconsistent Markdown renders differently across platforms, and some issues (like malformed tables) break rendering entirely.

## How to fix

### Tables

Tables need consistent column widths with proper alignment markers:

```text
| Name | Description |
|:-----|:------------|
| foo  | bar         |
```

The separator row must:

- Start with `|:` for left-align, `|` for default, or `|-` followed by `:`
- Have dashes filling the width
- End with `|`

### Headers

Headers need blank lines before and after:

```markdown
## Header

Content here
```

### Lists

Lists need a blank line before the first item:

```markdown
Some text:

- item 1
- item 2
```

## Configuration

Configure in `config.toml`:

```toml
[validators.file.markdown]
use_markdownlint = true   # Enable linting (default: true)
timeout = "10s"
context_lines = 2
```

Disable Markdown linting:

```toml
[validators.file.markdown]
use_markdownlint = false
```

## Related

- [markdownlint Rules](https://github.com/DavidAnson/markdownlint#rules)

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE005] Markdown formatting errors. Check markdown formatting and structure.`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`
