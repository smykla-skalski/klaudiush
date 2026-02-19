# FILE003: TFLint validation failed

## Error

tflint found issues in a Terraform/OpenTofu file.

## Why this matters

TFLint detects:

- Deprecated syntax and resources
- Provider-specific best practices
- Potential errors before `terraform plan`
- Security and compliance issues

## How to fix

1. Run tflint to see what it caught:

   ```bash
   tflint path/to/file.tf
   ```

2. Fix each issue -- the rule name tells you what to change

3. For false positives, suppress with an ignore comment:

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

## TFLint configuration

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

- [FILE002](FILE002.md) - Terraform format validation
- [TFLint documentation](https://github.com/terraform-linters/tflint)
