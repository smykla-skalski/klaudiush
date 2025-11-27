# FILE005: Markdown Lint Validation Failed

## Error

Markdown file has formatting issues.

## Why This Matters

- Consistent Markdown renders correctly across platforms
- Proper formatting improves readability
- Some issues (like tables) break rendering entirely

## How to Fix

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
