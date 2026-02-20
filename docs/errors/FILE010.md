# FILE010: Linter ignore directives detected

## Error

Code being written or edited contains linter ignore/suppress directives (e.g., `# noqa`, `// eslint-disable`, `//nolint`, `#[allow(...)]`).

## Why this matters

Linter ignore directives mask real issues instead of fixing them. Over time, suppressed warnings accumulate and erode code quality. Fix the underlying issue instead of silencing the linter.

## How to fix

Remove the ignore directive and fix the linter error properly:

```python
# Instead of:
import os  # noqa: F401

# Fix: remove unused import or use it
import os
path = os.getcwd()
```

```go
// Instead of:
//nolint:errcheck
_ = doSomething()

// Fix: handle the error
if err := doSomething(); err != nil {
    return err
}
```

## Detected patterns

klaudiush detects ignore directives for many languages:

- **Python**: `# noqa`, `# type: ignore`, `# pylint: disable`, `# pyright: ignore`, `# mypy: ignore`, `# pyrefly: ignore`
- **JavaScript/TypeScript**: `// eslint-disable`, `// @ts-ignore`, `// @ts-nocheck`, `// @ts-expect-error`, `/* eslint-disable */`
- **Go**: `//nolint`, `// nolint`
- **Rust**: `#[allow(...)]`, `#![allow(...)]`
- **Ruby**: `# rubocop:disable`
- **Shell**: `# shellcheck disable=`
- **Java**: `@SuppressWarnings`
- **C#**: `#pragma warning disable`
- **PHP**: `// phpcs:ignore`, `// @phpstan-ignore`
- **Swift**: `// swiftlint:disable`

## Configuration

```toml
[validators.file.linter_ignore]
enabled = true
patterns = []   # custom patterns (overrides defaults when set)
```

Disable the validator:

```toml
[validators.file.linter_ignore]
enabled = false
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE010] Linter ignore directives are not allowed. Fix linter errors properly instead of suppressing them with ignore directives`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [FILE001](FILE001.md) - shellcheck failure
- [FILE006](FILE006.md) - gofumpt formatting
