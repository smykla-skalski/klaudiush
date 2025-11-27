# FILE003: TFLint Validation Failed

## Error

Terraform/OpenTofu file has issues detected by tflint.

## Why This Matters

TFLint detects:

- Deprecated syntax and resources
- Provider-specific best practices
- Potential errors before `terraform plan`
- Security and compliance issues

## How to Fix

1. Run tflint to see detailed issues:

   ```bash
   tflint path/to/file.tf
   ```

2. Fix each issue based on the rule name

3. For false positives, add ignores:

   ```hcl
   # tflint-ignore: aws_instance_invalid_type
   resource "aws_instance" "example" {
     instance_type = "custom.large"
   }
   ```

## Configuration

Configure in `config.toml`:

```toml
[validators.file.terraform]
use_tflint = true    # Enable tflint (default: true)
timeout = "10s"      # Command timeout
```

Disable tflint:

```toml
[validators.file.terraform]
use_tflint = false
```

## TFLint Configuration

Create `.tflint.hcl` in your project root:

```hcl
config {
  call_module_type = "local"
}

plugin "aws" {
  enabled = true
  version = "0.30.0"
  source  = "github.com/terraform-linters/tflint-ruleset-aws"
}
```

## Related

- [FILE002](FILE002.md) - Terraform Format Validation
- [TFLint Documentation](https://github.com/terraform-linters/tflint)
