# FILE006: Gofumpt formatting failure

## Error

Go code has formatting issues detected by gofumpt.

## Why this matters

gofumpt enforces stricter formatting rules than `gofmt`, producing more consistent and readable Go code. Inconsistent formatting creates noisy diffs and makes code reviews harder.

## How to fix

Run gofumpt to auto-fix formatting:

```bash
gofumpt -w path/to/file.go
```

For the entire project:

```bash
gofumpt -w .
```

## Configuration

```toml
[validators.file.gofumpt]
enabled = true
timeout = "10s"
extra_rules = true
lang = ""       # auto-detected from go.mod
mod_path = ""   # auto-detected from go.mod
```

Disable the validator:

```toml
[validators.file.gofumpt]
enabled = false
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE006] Go code formatting issues detected. Run 'gofumpt -w <file>' to auto-fix formatting`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [FILE001](FILE001.md) - shellcheck failure
- [gofumpt documentation](https://github.com/mvdan/gofumpt)
