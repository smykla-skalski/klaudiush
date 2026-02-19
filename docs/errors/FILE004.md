# FILE004: Actionlint validation failed

## Error

actionlint or digest pinning validation found problems in a GitHub Actions workflow file.

## Why this matters

Workflow files that reach GitHub with syntax errors, bad expressions, or unpinned action references will either fail at runtime or expose the repository to tag-hijacking attacks. Catching these locally saves a push-and-wait cycle and keeps the CI supply chain pinned to known-good digests.

## How to fix

### Actionlint issues

Run actionlint to see what went wrong:

```bash
actionlint .github/workflows/your-workflow.yml
```

### Digest pinning issues

Pin actions by SHA digest instead of a mutable tag:

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

## Getting digests

Look up the commit SHA behind a tag with `gh`:

```bash
gh api repos/actions/checkout/git/ref/tags/v4.1.1 --jq '.object.sha'
```

## Related

- [actionlint Documentation](https://github.com/rhysd/actionlint)
- [GitHub Actions Security Hardening](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions)
