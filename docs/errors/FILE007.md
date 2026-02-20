# FILE007: Ruff Python validation failure

## Error

Python code has linting issues detected by ruff.

## Why this matters

ruff catches common Python errors, style issues, and potential bugs. These range from unused imports to security vulnerabilities. Catching them before commit prevents bugs from reaching production.

## How to fix

Run ruff to see detailed issues:

```bash
ruff check path/to/file.py
```

Auto-fix where possible:

```bash
ruff check --fix path/to/file.py
```

For Edit operations, klaudiush validates only the changed fragment with surrounding context lines to reduce false positives from incomplete context (e.g., F401 unused imports, F841 unused variables are excluded in fragment mode).

## Configuration

```toml
[validators.file.python]
enabled = true
use_ruff = true
timeout = "10s"
context_lines = 2
exclude_rules = ["E501"]    # rules to exclude
ruff_config = ""            # path to ruff config file
```

Disable the validator:

```toml
[validators.file.python]
enabled = false
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE007] Python linting issues detected. Run 'ruff check <file>' to see Python code quality issues`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [FILE006](FILE006.md) - gofumpt formatting
- [ruff documentation](https://docs.astral.sh/ruff/)
