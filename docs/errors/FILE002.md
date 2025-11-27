# FILE002: Terraform Format Validation Failed

## Error

Terraform/OpenTofu file has formatting issues detected by `terraform fmt` or `tofu fmt`.

## Why This Matters

- Consistent formatting improves code readability
- Standard format reduces diff noise in reviews
- Many teams require formatted code in CI checks

## How to Fix

Run the formatter for your tool:

```bash
# For OpenTofu:
tofu fmt path/to/file.tf

# Or for all files:
tofu fmt -recursive

# For Terraform:
terraform fmt path/to/file.tf
```

## Tool Detection

klaudiush automatically detects which tool is available:

1. Checks for `tofu` first (OpenTofu)
2. Falls back to `terraform` if tofu not found
3. Skips validation if neither is available

## Configuration

Configure in `config.toml`:

```toml
[validators.file.terraform]
check_format = true       # Enable format checking (default: true)
use_tflint = true         # Enable tflint integration (default: true)
timeout = "10s"           # Command timeout
context_lines = 2         # Lines of context for edit validation
```

Disable format checking:

```toml
[validators.file.terraform]
check_format = false
```

## Related

- [FILE003](FILE003.md) - TFLint Validation
