# FILE009: Rustfmt formatting failure

## Error

Rust code has formatting issues detected by rustfmt.

## Why this matters

Consistent formatting reduces cognitive load during code reviews and prevents unnecessary diffs. rustfmt is the community standard for Rust code formatting.

## How to fix

Run rustfmt to auto-fix formatting:

```bash
rustfmt path/to/file.rs
```

For the entire project:

```bash
cargo fmt
```

klaudiush auto-detects the Rust edition from `Cargo.toml` (defaults to 2021 if not found).

## Configuration

```toml
[validators.file.rust]
enabled = true
use_rustfmt = true
timeout = "10s"
edition = ""               # auto-detected from Cargo.toml
context_lines = 2
rustfmt_config = ""        # path to rustfmt config
```

Disable the validator:

```toml
[validators.file.rust]
enabled = false
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE009] Rust code formatting issues detected. Run 'rustfmt <file>' to auto-fix formatting`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [FILE006](FILE006.md) - gofumpt formatting
- [rustfmt documentation](https://github.com/rust-lang/rustfmt)
