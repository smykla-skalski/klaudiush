# Session: Enhanced Error Reporting

Date: 2024-11-26
Updated: 2024-11-27

## What Was Implemented

Added structured error references, fix suggestions, and documentation URLs to validation errors.

## Key Patterns Discovered

### Unified Reference System (Current)

Reference URLs combine error code and documentation link into a single field:

- Format: `https://klaudiu.sh/{CODE}` (e.g., `https://klaudiu.sh/GIT001`)
- The redirect service (future) resolves to full documentation

Error codes organized by category with numeric suffixes:

- **GIT001-GIT016**: Git-related errors (signoff, GPG, staging, commits)
- **FILE001-FILE005**: File validation errors (shellcheck, terraform, actionlint, markdown)
- **SEC001-SEC005**: Security errors (API keys, passwords, tokens, connection strings)

### Creating References

`internal/validator/reference.go`:

```go
type Reference string

const ReferenceBaseURL = "https://klaudiu.sh"

const (
    RefGitNoSignoff   Reference = ReferenceBaseURL + "/GIT001"
    RefGitMissingFlags Reference = ReferenceBaseURL + "/GIT010"
    RefShellcheck     Reference = ReferenceBaseURL + "/FILE001"
    RefSecretsAPIKey  Reference = ReferenceBaseURL + "/SEC001"
)

// Methods
func (r Reference) String() string  // Returns full URL
func (r Reference) Code() string    // Extracts "GIT001" from URL
func (r Reference) Category() string // Returns "GIT", "FILE", "SEC"
```

### Suggestions Registry

Fix hints are keyed by Reference:

```go
// internal/validator/suggestions.go
var DefaultSuggestions = map[Reference]string{
    RefGitMissingFlags: "Add -sS flags: git commit -sS -m \"message\"",
    RefShellcheck:      "Run 'shellcheck <file>' to see detailed errors",
}
```

### Using FailWithRef/WarnWithRef

Auto-populates FixHint from suggestions registry:

```go
// Instead of:
return validator.Fail("Git commit missing required flags: -s -S")

// Use:
return validator.FailWithRef(
    validator.RefGitMissingFlags,
    "Git commit missing required flags: -s -S",
)
```

Result automatically includes:

- `Reference`: Reference URL for error docs
- `FixHint`: from suggestions registry

### ValidationError Struct

Extended `ValidationError` in dispatcher:

```go
type ValidationError struct {
    Validator   string
    Message     string
    Details     map[string]string
    ShouldBlock bool
    Reference   validator.Reference  // URL for error docs
    FixHint     string               // Fix suggestion
}
```

### Error Output Format

Formatted errors now show structured information:

```text
Git commit missing required flags: -S
   Fix: Add -sS flags: git commit -sS -m "message"
   Reference: https://klaudiu.sh/GIT010
```

## Refactoring for Cognitive Complexity

When `gocognit` linter flags high complexity, extract helper functions:

```go
// Before: formatErrorList had complexity 23

// After: Split into helpers
func formatErrorList(header string, errors []*ValidationError) string {
    formatErrorHeader(&builder, header, errors)
    for _, err := range errors {
        formatSingleError(&builder, err)
    }
}

func formatErrorHeader(...)     // Writes header line
func formatSingleError(...)     // Writes one error
func formatErrorMetadata(...)   // Writes fix/reference
func formatErrorDetails(...)    // Writes details map
```

## Magic Number Constants

Linter `mnd` flags magic numbers. Define constants:

```go
// Before:
if len(code) < 3 {

// After:
const minCodeLength = 3
if len(code) < minCodeLength {
```

## Testing Reference System

Ginkgo tests for reference functionality:

```go
var _ = Describe("FailWithRef", func() {
    It("creates a failing result with reference", func() {
        result := validator.FailWithRef(validator.RefGitMissingFlags, "test")

        Expect(result.Passed).To(BeFalse())
        Expect(result.Reference).To(Equal(validator.RefGitMissingFlags))
        Expect(result.FixHint).To(ContainSubstring("-sS"))
    })
})
```

## Files Structure

```text
internal/validator/
├── reference.go        # Reference type and constants
├── reference_test.go   # Tests for references
├── suggestions.go      # Fix hints registry (keyed by Reference)
└── validator.go        # Extended Result type, FailWithRef(), WarnWithRef()
```

## Adding New References

1. Add constant in `reference.go` with proper URL format
2. Add suggestion in `suggestions.go`
3. Use `FailWithRef()` or `WarnWithRef()` in validator

## Migration Notes (2024-11-27)

Changed from separate `ErrorCode` + `DocLink` fields to unified `Reference` URL:

- Removed `error_code.go` and `doc_links.go`
- Renamed `FailWithCode` → `FailWithRef`, `WarnWithCode` → `WarnWithRef`
- `ErrXxx` constants → `RefXxx` constants
- Result no longer has `DocLink` field (it's embedded in Reference URL)

## Plugin System (2025-11-27)

Plugins manage their own error documentation. The adapter preserves plugin-provided URLs:

```go
// internal/plugin/adapter.go
// Use plugin's own error metadata if provided
// Plugins manage their own error codes and documentation URLs
if resp.DocLink != "" {
    result.Reference = validator.Reference(resp.DocLink)
}

result.FixHint = resp.FixHint
```

Plugin API (`pkg/plugin/api.go`) provides:

- `ErrorCode`: Plugin's error identifier (for internal use)
- `FixHint`: Short fix suggestion
- `DocLink`: URL to plugin's documentation (used as-is for Reference)

Example plugin response with custom documentation:

```go
return pluginapi.FailWithCode(
    "MYPLUGIN001",           // ErrorCode (for logging/debugging)
    "validation failed",     // Message
    "try using --flag",      // FixHint
    "https://my-plugin.smyk.la/errors/MYPLUGIN001", // DocLink
)
```
