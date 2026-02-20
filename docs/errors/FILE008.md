# FILE008: Oxlint JavaScript/TypeScript validation failure

## Error

JavaScript or TypeScript code has linting issues detected by oxlint.

## Why this matters

oxlint catches common JS/TS errors, unused variables, type issues, and potential bugs. It runs much faster than ESLint and catches the most impactful issues.

## How to fix

Run oxlint to see detailed issues:

```bash
oxlint path/to/file.js
```

For Edit operations, klaudiush validates only the changed fragment with surrounding context lines to reduce false positives (e.g., `no-unused-vars`, `no-undef`, `import/no-unresolved` are excluded in fragment mode).

## Configuration

```toml
[validators.file.javascript]
enabled = true
use_oxlint = true
timeout = "10s"
context_lines = 2
exclude_rules = []          # rules to exclude
oxlint_config = ""          # path to oxlint config file
```

Disable the validator:

```toml
[validators.file.javascript]
enabled = false
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE008] JavaScript/TypeScript linting issues detected. Run 'oxlint <file>' to see JavaScript/TypeScript code quality issues`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [FILE007](FILE007.md) - ruff Python validation
- [oxlint documentation](https://oxc-project.github.io/docs/guide/usage/linter.html)
