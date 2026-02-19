# FILE002: Terraform format validation failed

## Error

Terraform/OpenTofu file has formatting issues detected by `terraform fmt` or `tofu fmt`.

## Why this matters

Unformatted Terraform files create noisy diffs and tend to fail CI checks that enforce `terraform fmt` or `tofu fmt`.

## How to fix

Run the formatter for your tool:

```bash
# For OpenTofu:
tofu fmt path/to/file.tf

# Or for all files:
tofu fmt -recursive

# For Terraform:
terraform fmt path/to/file.tf
```

## Tool detection

klaudiush checks for `tofu` first, falls back to `terraform`, and skips this validation if neither is installed.

## Configuration

In `config.toml`:

```toml
[validators.file.terraform]
check_format = true       # Enable format checking (default: true)
use_tflint = true         # Enable tflint integration (default: true)
timeout = "10s"           # Command timeout
context_lines = 2         # Lines of context for edit validation
```

To disable format checking:

```toml
[validators.file.terraform]
check_format = false
```

## Related

- [FILE003](FILE003.md) - tflint validation

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE002] Terraform/OpenTofu file has formatting issues. Run 'terraform fmt' or 'tofu fmt' to fix formatting.`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`
