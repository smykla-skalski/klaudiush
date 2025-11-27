# FILE004: Actionlint Validation Failed

## Error

GitHub Actions workflow file has issues detected by actionlint or digest pinning validation.

## Why This Matters

- Catches workflow syntax errors before pushing
- Enforces security best practices (digest pinning)
- Validates action inputs and outputs
- Ensures expression syntax is correct

## How to Fix

### Actionlint Issues

Run actionlint to see detailed errors:

```bash
actionlint .github/workflows/your-workflow.yml
```

### Digest Pinning Issues

Use SHA digest instead of tags for security:

```yaml
# Wrong (vulnerable to tag hijacking):
- uses: actions/checkout@v4

# Correct (with version comment):
- uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1
```

If digest pinning isn't possible, add an explanation comment:

```yaml
# Cannot pin by digest: action requires dynamic tag resolution
- uses: vendor/custom-action@v1
```

## Configuration

Configure in `config.toml`:

```toml
[validators.file.workflow]
use_actionlint = true           # Enable actionlint (default: true)
enforce_digest_pinning = true   # Require digest pins (default: true)
require_version_comment = true  # Require version comment with digest (default: true)
check_latest_version = true     # Warn about outdated versions (default: true)
timeout = "10s"
gh_api_timeout = "5s"
```

Disable digest pinning enforcement:

```toml
[validators.file.workflow]
enforce_digest_pinning = false
```

## Getting Digests

Use `gh` CLI to find action digests:

```bash
gh api repos/actions/checkout/git/ref/tags/v4.1.1 --jq '.object.sha'
```

## Related

- [actionlint Documentation](https://github.com/rhysd/actionlint)
- [GitHub Actions Security Hardening](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions)
