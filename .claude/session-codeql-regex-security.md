# Regex Security Patterns Architecture

Security-hardened regex patterns for URL validation and ReDoS prevention, addressing CodeQL CWE-020 alerts.

## Core Design Philosophy

**Explicit Anchoring Over Word Boundaries**: Word boundaries (`\b`) are insufficient for URL pattern security. They match character class transitions but don't prevent embedded URL attacks. Use explicit prefix matching to ensure URLs appear in valid contexts only.

**Bounded Quantifiers by Default**: All quantifiers must have upper bounds to prevent ReDoS attacks. Reasonable bounds based on real-world limits (e.g., PR numbers ≤10 digits, Slack IDs ≤20 chars).

**Prefix-Aware Match Handling**: Anchor patterns consume prefixes as part of matches. Code must strip prefixes before display to avoid malformed URLs in error messages.

## Critical Implementation Details

### URL Anchoring Pattern

Use `(?:^|://|[^/a-zA-Z0-9])` prefix to prevent embedded URL attacks:

```go
// INSECURE - Matches "evil.com/github.com/user/repo"
`\bgithub\.com/[a-z0-9_-]+/[a-z0-9_-]+`

// SECURE - Only matches legitimate URLs
`(?:^|://|[^/a-zA-Z0-9])github\.com/[a-z0-9_-]+/[a-z0-9_-]+`
```

The pattern allows three valid contexts:

- `^` - Start of string
- `://` - URL scheme separator (`https://github.com/...`)
- `[^/a-zA-Z0-9]` - Non-path characters (whitespace, quotes, etc.)

This prevents matching `evil.com/github.com/...` (path segment) while allowing `https://github.com/...` or `"github.com/..."` (quoted).

**Gotcha**: Hash symbol `#` is NOT a word character. Pattern `\b#[0-9]+` won't match `issue #123` because there's no word boundary between space and `#`. Use `#[0-9]+\b` (trailing boundary only).

### Bounded Quantifiers

Always add upper bounds based on real-world limits:

```go
// VULNERABLE - Unbounded quantifiers enable ReDoS
`T[A-Z0-9]{8,}`           // Slack token prefix
`[0-9]+`                   // PR numbers
`[a-zA-Z0-9_-]+`          // Repository names

// HARDENED - Bounded quantifiers
`T[A-Z0-9]{8,20}`         // Slack IDs: 8-20 chars
`[0-9]{1,10}`              // PR numbers: max ~10 billion
`[a-zA-Z0-9_-]{1,100}`    // Repo names: GitHub max 100
```

Bounds are based on platform limits and real-world maximums. GitHub PR numbers rarely exceed 10 digits. Slack workspace/bot IDs are typically 8-15 characters.

### Prefix Consumption in Matches

Anchor patterns capture prefixes as part of the match. Strip before display:

```go
pattern := `(?:^|://|[^/a-zA-Z0-9])github\.com/[a-z0-9_-]+/[a-z0-9_-]+`
matches := pattern.FindAllString(content, -1)

for _, match := range matches {
    // BAD - Produces "https://://github.com/..." or "https:// github.com/..."
    fmt.Sprintf("Found: 'https://%s'", match)

    // GOOD - Strip prefix before formatting
    cleanURL := match
    if idx := strings.Index(match, "github.com"); idx > 0 {
        cleanURL = match[idx:]
    }
    fmt.Sprintf("Found: 'https://%s'", cleanURL)
}
```

The prefix (`://` or space) becomes part of the match. Format URLs only after stripping the anchor prefix.

## Integration Points

### Secrets Validator Integration

Patterns in `internal/validators/secrets/patterns.go`:

```go
{
    Name:        "slack_incoming_webhook_url",
    Pattern:     `(?:^|://|[^/a-zA-Z0-9])hooks\.slack\.com/services/T[A-Z0-9]{8,20}/B[A-Z0-9]{8,20}/[a-zA-Z0-9]{24,24}`,
    Description: "Slack Incoming Webhook URL",
    Severity:    Critical,
}
```

### Commit Message Validator Integration

Patterns in `internal/validators/git/commit_rules.go`:

```go
// GitHub PR URLs
`(?:^|://|[^/a-zA-Z0-9])github\.com/[a-z0-9_-]{1,100}/[a-z0-9_-]{1,100}/pull/[0-9]{1,10}`

// Hash references
`#[0-9]{1,10}\b`
```

## GitHub Operational Patterns

### Push Protection Bypass for Test Secrets

Test files with intentional secrets (fuzz corpus, detector tests) may trigger push protection:

```bash
# Extract placeholder_id from error URL (last path segment)
# e.g., https://github.com/ORG/REPO/security/secret-scanning/unblock-secret/PLACEHOLDER_ID

gh api repos/ORG/REPO/secret-scanning/push-protection-bypasses \
  -X POST \
  -f secret_type="slack_incoming_webhook_url" \
  -f reason="used_in_tests" \
  -f placeholder_id="PLACEHOLDER_ID"
```

Valid reasons: `used_in_tests`, `false_positive`, `will_fix_later`

### PR Review Thread Resolution

```bash
# Get unresolved thread IDs
gh api graphql -f query='
query {
  repository(owner: "ORG", name: "REPO") {
    pullRequest(number: 123) {
      reviewThreads(first: 20) {
        nodes { id isResolved path line }
      }
    }
  }
}' --jq '.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false)'

# Resolve thread
gh api graphql -f query='
mutation {
  resolveReviewThread(input: {threadId: "THREAD_ID"}) {
    thread { id isResolved }
  }
}'

# Reply to comment
gh api repos/ORG/REPO/pulls/123/comments/COMMENT_ID/replies \
  -X POST -f body="Fixed in latest commit"
```

## Common Pitfalls

1. **Using `\b` for URL anchoring**: Word boundaries match character class transitions, not semantic URL boundaries. `\bhttps://hooks.slack.com/...` matches `evil.com/https://hooks.slack.com/...` because `/` to `h` is a word boundary.

2. **Unbounded quantifiers in production**: Patterns like `[0-9]+` or `[A-Z0-9]{8,}` enable ReDoS attacks. Always add reasonable upper bounds based on real-world maximums.

3. **Leading boundary on hash patterns**: Pattern `\b#[0-9]+` won't match `issue #123` because `#` is not a word character (no boundary between space and `#`). Use trailing boundary only: `#[0-9]+\b`.

4. **Formatting matches without prefix stripping**: Anchor patterns consume prefixes. Formatting `"https://" + match` produces `"https://://github.com/..."` if match starts with `://`. Always strip anchor prefix before display.

5. **Overly restrictive anchors**: Pattern `^github\.com/...` only matches URLs at string start. Most commit messages have text before URLs. Use `(?:^|://|[^/a-zA-Z0-9])` to allow all valid contexts.

6. **Assuming all regex engines handle anchors identically**: CodeQL's security analysis has specific requirements beyond what Go's `regexp` enforces. Patterns passing Go tests may still fail CodeQL security checks.

7. **Not testing with embedded URL attacks**: Test patterns with malicious inputs like `evil.com/github.com/user/repo` and `javascript:alert(document.location='https://github.com/user/repo')` to verify anchors work.
