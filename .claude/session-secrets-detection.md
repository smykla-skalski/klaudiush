# Secrets Detection Architecture

Two-tier secret detection system combining fast regex patterns with optional gitleaks integration for file write validation.

## Core Design Philosophy

**Two-Tier Detection Over Single Method**: Fast regex patterns (always enabled, CPU-bound) provide baseline protection with zero dependencies. Optional gitleaks binary (when available) adds comprehensive coverage with community-maintained rules. This design prioritizes defense-in-depth while maintaining fast-path performance and zero external dependencies for basic protection.

**Pattern Metadata Over Raw Regex**: Every pattern includes name, description, and error code alongside compiled regex. Metadata enables actionable error messages ("AWS Access Key ID detected" vs "pattern match") and categorized error codes (SEC001-SEC005). This design makes validation failures educational rather than cryptic.

**Allow Lists for Precision**: False positives are inevitable with regex-based detection. Allow list patterns suppress known-safe content (test fixtures, examples, mock data) without disabling entire pattern categories. This design maintains security coverage while reducing noise.

**File Size Boundaries**: Processing multi-megabyte files for secrets is expensive and low-value (large files are typically binary artifacts, not source code). Default 1MB limit skips large files automatically. This design optimizes validator performance for typical codebases.

**CPU-Bound Categorization**: Regex matching is compute-intensive, not I/O-bound. Return `CategoryCPU` for parallel execution scheduler to allocate appropriate worker pools. This design ensures regex validators don't starve I/O-bound validators of workers.

## Critical Implementation Details

### Pattern Definition with Metadata

```go
type Pattern struct {
    Name        string              // "aws-access-key-id"
    Description string              // "AWS Access Key ID"
    Regex       *regexp.Regexp      // Compiled pattern
    ErrorCode   validator.ErrorCode // ErrSecretsAPIKey (SEC001)
}
```

Patterns are compiled once at initialization. Metadata travels with patterns throughout detection pipeline, enabling rich error messages without string matching or lookup tables.

### Built-in Pattern Categories (25+)

**Cloud Providers**:

- AWS: Access Key ID (`AKIA[A-Z0-9]{16}`), Secret Access Key (40 alphanumeric)
- Google/GCP: API Keys (`AIza[0-9A-Za-z_-]{35}`), Service Account JSON
- Azure: Storage keys, connection strings

**Version Control Platforms**:

- GitHub: PAT (`ghp_`), OAuth (`gho_`), App (`ghs_`/`ghu_`), Refresh (`ghr_`)
- GitLab: Personal Access Token (`glpat-[a-zA-Z0-9_-]{20,22}`)

**Communication Platforms**:

- Slack: Bot tokens (`xoxb-`), User tokens (`xoxp-`), Webhook URLs
- SendGrid, Mailgun, Twilio API keys

**Database Connections**:

- MongoDB (`mongodb://`), PostgreSQL (`postgresql://`), MySQL (`mysql://`)
- Redis connection strings with passwords

**Cryptographic Keys**:

- RSA/DSA/EC private keys (BEGIN/END markers)
- OpenSSH private keys, PGP private key blocks
- JWT tokens (base64 pattern with dots)

**Generic High-Risk Patterns**:

- Password variables (`password\s*=\s*['"]\S+['"]`)
- API key variables (`api_key\s*=\s*['"]\S+['"]`)
- Secret variables (`secret\s*=\s*['"]\S+['"]`)

Generic patterns have higher false positive rates. Disable via `disabled_patterns` if noise outweighs value.

### Content Extraction by Tool Type

```go
// Write tool: entire file content
content := hookCtx.GetContent()

// Edit tool: only new content being written
content := hookCtx.ToolInput.NewString
```

**Design Rationale**: Edit tool validates only changes, not entire file. User may edit file with existing secrets (legacy code, documented examples). Validator blocks only *new* secrets being introduced. This prevents blocking legitimate work on files with pre-existing issues.

### Finding Representation

```go
type Finding struct {
    Pattern *Pattern  // Which pattern matched
    Match   string    // Actual matched text
    Line    int       // 1-indexed line number
    Column  int       // 1-indexed column offset
}
```

Line and column numbers enable precise error messages: `secrets_validator.go:42:15: AWS Access Key ID detected`. Without location data, user must search entire file for match.

### Error Code Mapping

| Code   | Category             | Patterns                                                           |
|:-------|:---------------------|:-------------------------------------------------------------------|
| SEC001 | ErrSecretsAPIKey     | AWS keys, Google keys, GitHub PATs, Slack tokens, service API keys |
| SEC002 | ErrSecretsPassword   | Password variables, hardcoded passwords                            |
| SEC003 | ErrSecretsPrivKey    | RSA, DSA, EC, OpenSSH, PGP private keys                            |
| SEC004 | ErrSecretsToken      | JWT, OAuth tokens, refresh tokens                                  |
| SEC005 | ErrSecretsConnString | MongoDB, PostgreSQL, MySQL, Redis connection strings               |

Mapping is 1-to-N (one error code covers multiple patterns). This design balances error code granularity with maintainability.

## Integration Points

### Factory Configuration

```go
// internal/config/factory/secrets_factory.go
func (f *SecretsValidatorFactory) Create(hookCtx *hook.Context) (validator.Validator, error) {
    detector := createPatternDetector(config)
    gitleaksChecker := createGitleaksChecker(config)

    return &secrets.SecretsValidator{
        Detector:        detector,
        GitleaksChecker: gitleaksChecker,
        Config:          config,
    }, nil
}
```

Factory creates validator with:

1. Pattern detector (default + custom patterns, minus disabled)
2. Optional gitleaks checker (nil if disabled or binary unavailable)
3. Predicate: `PreToolUse` + `Write|Edit` tools only

```go
validator.And(
    validator.EventTypeIs(hook.EventTypePreToolUse),
    validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit),
)
```

**Why this predicate**: Secrets matter only in persistent writes. Read, Glob, Grep, Bash (without redirection) don't persist secrets to repository.

### Gitleaks Integration

```go
// internal/linters/gitleaks.go
type GitleaksChecker interface {
    CheckContent(content string) (*LintResult, error)
}
```

Gitleaks runs as second-tier detection when:

1. Binary available (`which gitleaks` succeeds)
2. `use_gitleaks = true` in config
3. First-tier patterns didn't already block

**Operational Note**: Gitleaks requires installation via package manager or mise. Binary is not vendored. Validator gracefully degrades to regex-only if unavailable.

### Configuration Schema

```toml
[validators.secrets.secrets]
enabled = true
use_gitleaks = false           # Enable second-tier detection
max_file_size = 1048576        # 1MB (use config.ByteSize for human-readable)
block_on_detection = true      # false = warn only (useful for gradual rollout)

# Suppress false positives
allow_list = [
    "AKIA.*EXAMPLE",           # AWS documentation examples
    "ghp_test[a-zA-Z0-9]{36}", # Test token pattern
    "mongodb://localhost",     # Local development connection
]

# Disable noisy patterns
disabled_patterns = [
    "generic-password",        # Too many false positives in comments/docs
    "generic-api-key",         # Matches variable names, not actual keys
]

# Add organization-specific patterns
[[validators.secrets.secrets.custom_patterns]]
name = "internal-api-key"
description = "Internal API Key"
regex = "MYCOMPANY_[A-Z0-9]{32}"
error_code = "SEC001"          # Map to existing SEC category
```

**ByteSize Type**: Config accepts human-readable sizes (`"1MB"`, `"512KB"`, `"2.5MB"`). Internally converted to bytes. This design improves config readability for non-technical users.

## Testing Patterns

### Pattern Length Requirements

Regex quantifiers define exact length requirements. Test data must match:

```go
// Pattern: AIza[0-9A-Za-z_-]{35}
// Requires exactly 35 characters after "AIza"
"AIzaSyD-abcdefghijklmnopqrstuvwxyz12345"  // ✓ Valid (35 chars)
"AIzaSyD-short"                             // ✗ Invalid (9 chars)

// Pattern: SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}
// Requires 22 chars, dot, 43 chars
"SG.abcdefghijklmnopqrstuv.wxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abc"  // ✓ Valid

// Pattern: ghp_[a-zA-Z0-9]{36}
// GitHub PAT: exactly 36 alphanumeric
"ghp_" + strings.Repeat("a", 36)  // ✓ Valid (36 chars)
"ghp_" + strings.Repeat("a", 40)  // ✗ Invalid (40 chars)
```

**Testing Strategy**: Generate test cases programmatically using `strings.Repeat()` to guarantee exact lengths. Hardcoded test strings drift when patterns change.

### Fuzz Testing

Secrets validator includes fuzz targets in `internal/validators/secrets/detector_fuzz_test.go`:

```go
func FuzzPatternDetector(f *testing.F) {
    // Seed with real secret formats
    f.Add("AKIAIOSFODNN7EXAMPLE")
    f.Add("ghp_" + strings.Repeat("x", 36))

    f.Fuzz(func(t *testing.T, content string) {
        // Should never panic, even on malformed input
        findings, err := detector.Detect(content)
        require.NoError(t, err)
    })
}
```

Fuzz testing validates regex patterns don't cause catastrophic backtracking (ReDoS). Run with: `task test:fuzz` (10s default) or `FUZZ_TIME=5m task test:fuzz` (extended).

## Common Pitfalls

1. **Disabling all generic patterns globally**: Patterns like `generic-password` have high false positives but catch real secrets occasionally. Disable per-repository via `.klaudiush/config.toml`, not globally in `~/.klaudiush/config.toml`. Organization-wide settings affect all projects.

2. **Using regex literal strings in allow lists**: Allow list patterns are themselves regexes, not literals. Pattern `AKIA1234` matches `XAKIA1234X`. Use anchors: `^AKIA1234$` or explicit boundaries: `\bAKIA1234\b`.

3. **Forgetting to escape regex metacharacters**: Literal dots must be escaped in allow lists: `mongodb://localhost\.local` not `mongodb://localhost.local`. Unescaped dots match any character (`localhost1local`, `localhostXlocal`).

4. **Setting `max_file_size = 0` to disable limit**: Zero means "skip all files" (0 bytes maximum). Use very large value for unlimited: `max_file_size = 104857600` (100MB). Better: leave default 1MB, large files rarely contain secrets.

5. **Assuming gitleaks is always available**: Validator degrades gracefully when gitleaks binary missing. Don't assume `GitleaksChecker != nil`. Always check before calling methods, or use nil-safe wrapper pattern.

6. **Not testing with base64-encoded secrets**: Many secret formats are base64-encoded (JWT, some API keys). Test patterns with both raw and encoded versions to verify detection across encoding boundaries.

7. **Blocking on single finding without location context**: Error message "Secret detected in file" is useless. Always include line/column numbers and matched pattern name. User needs precise location to fix issue quickly.

8. **Adding custom patterns without error codes**: Custom patterns require `error_code` field mapping to SEC001-SEC005. Omitting error code causes validation error during factory creation. All patterns must map to existing reference documentation.

9. **Using `block_on_detection = false` in production**: Warn-only mode is for gradual rollout during initial deployment. Running long-term in warn-only mode defeats purpose of secrets detection. Commit secrets with warning, discover in production later.

10. **Not maintaining allow list when patterns evolve**: Built-in patterns may change (new formats, tighter bounds, additional prefixes). Allow list patterns matching old formats may stop working. Review allow list when updating klaudiush versions.