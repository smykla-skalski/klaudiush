# Session: CodeQL Regex Security Fixes

Fixing CodeQL "Missing regular expression anchor" alerts (CWE-020) for URL patterns in validators.

## Key Learnings

### Word Boundary Limitations

`\b` (word boundary) is NOT sufficient for URL pattern anchoring:

- CodeQL requires proper start anchoring to prevent embedded URL attacks
- `\bhttps://...` still matches `evil.com/https://hooks.slack.com/...` because `\b` matches between `/` and `h`
- Hash symbol `#` is NOT a word character, so `\b#[0-9]+` won't match `issue #123` (no boundary between space and `#`)

### Proper URL Anchoring Pattern

Use explicit prefix matching instead of word boundaries:

```go
// BAD - CodeQL will flag this
`\bhttps://hooks\.slack\.com/services/...`

// GOOD - Explicit prefix anchoring
`(?:^|://|[^/a-zA-Z0-9])https://hooks\.slack\.com/services/...`
```

The pattern `(?:^|://|[^/a-zA-Z0-9])` allows:

- `^` - Start of string
- `://` - URL scheme separator (for `https://github.com/...`)
- `[^/a-zA-Z0-9]` - Non-path characters (whitespace, quotes, etc.)

This prevents matching embedded paths like `evil.com/github.com/...` while allowing legitimate URLs.

### Bounded Quantifiers for ReDoS Prevention

Always add upper bounds to prevent ReDoS attacks:

```go
// BAD - Unbounded, vulnerable to ReDoS
`T[A-Z0-9]{8,}`
`[0-9]+`

// GOOD - Bounded
`T[A-Z0-9]{8,20}`
`[0-9]{1,10}`
```

Reasonable bounds:

- Slack workspace/bot IDs: `{8,20}`
- GitHub PR numbers: `{1,10}` (PRs rarely exceed 10 billion)

### Hash Reference Pattern

For `#123` style references, only trailing boundary works:

```go
// BAD - Won't match "#123" after space
`\b#[0-9]{1,10}\b`

// GOOD - Matches "#123" anywhere, prevents "#123abc"
`#[0-9]{1,10}\b`
```

### Prefix Consumption in Matches

When using prefix anchors, the prefix becomes part of the match. This can break error message formatting:

```go
// Pattern captures the prefix (e.g., "://" or space)
pattern := `(?:^|://|[^/a-zA-Z0-9])github\.com/...`

// BAD - Will produce "https://://github.com/..." or "https:// github.com/..."
fmt.Sprintf("Found: 'https://%s'", urlMatch)

// GOOD - Strip prefix before display
cleanURL := urlMatch
if idx := strings.Index(urlMatch, "github.com"); idx > 0 {
    cleanURL = urlMatch[idx:]
}
fmt.Sprintf("Found: 'https://%s'", cleanURL)
```

## GitHub Push Protection Bypass

When pushing test files containing intentional secrets (e.g., test patterns for secret detection), GitHub may block the push. Bypass with:

```bash
# Extract placeholder_id from error URL (last path segment)
# e.g., .../unblock-secret/PLACEHOLDER_ID

gh api repos/OWNER/REPO/secret-scanning/push-protection-bypasses \
  -X POST \
  -f secret_type="slack_incoming_webhook_url" \
  -f reason="used_in_tests" \
  -f placeholder_id="PLACEHOLDER_ID"
```

Valid reasons: `used_in_tests`, `false_positive`, `will_fix_later`

## GitHub PR Review Workflow

### Reply to Review Comments

```bash
gh api repos/OWNER/REPO/pulls/PR/comments/COMMENT_ID/replies \
  -X POST -f body="Response text"
```

### Resolve Review Threads

```bash
# Get thread IDs
gh api graphql -f query='
query {
  repository(owner: "OWNER", name: "REPO") {
    pullRequest(number: PR) {
      reviewThreads(first: 20) {
        nodes { id isResolved path line }
      }
    }
  }
}'

# Resolve thread
gh api graphql -f query='
mutation {
  resolveReviewThread(input: {threadId: "THREAD_ID"}) {
    thread { id isResolved }
  }
}'
```

## Files Modified

- `internal/validators/secrets/patterns.go` - Slack webhook URL pattern
- `internal/validators/git/commit_rules.go` - GitHub PR URL and hash reference patterns
- `internal/validators/git/commit_rules_test.go` - PR reference rule tests
- `internal/validators/secrets/detector_test.go` - Slack webhook URL tests
