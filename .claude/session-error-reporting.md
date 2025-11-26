# Session: Enhanced Error Reporting

Date: 2024-11-26

## What Was Implemented

Added structured error codes, fix suggestions, and documentation links to validation errors.

## Key Patterns Discovered

### ErrorCode System

Error codes organized by category with numeric suffixes:

- **GIT001-GIT099**: Git-related errors (signoff, GPG, staging, commits)
- **FILE001-FILE099**: File validation errors (shellcheck, terraform, actionlint, markdown)
- **SEC001-SEC099**: Security errors (API keys, passwords, tokens) - reserved for future

### Creating Error Codes

`internal/validator/error_code.go`:

```go
type ErrorCode string

const (
    ErrGitMissingFlags ErrorCode = "GIT010"
    ErrShellcheck      ErrorCode = "FILE001"
)
```

### Registries Pattern

Suggestions and doc links are kept in separate registries (not in validators):

```go
// internal/validator/suggestions.go
var DefaultSuggestions = map[ErrorCode]string{
    ErrGitMissingFlags: "Add -sS flags: git commit -sS -m \"message\"",
}

// internal/validator/doc_links.go
var DefaultDocLinks = map[ErrorCode]string{
    ErrGitMissingFlags: BaseDocURL + "/GIT010.md",
}
```

### Using FailWithCode

Auto-populates FixHint and DocLink from registries:

```go
// Instead of:
return validator.Fail("Git commit missing required flags: -s -S")

// Use:
return validator.FailWithCode(
    validator.ErrGitMissingFlags,
    "Git commit missing required flags: -s -S",
)
```

Result automatically includes:

- `ErrorCode`: "GIT010"
- `FixHint`: from suggestions registry
- `DocLink`: from doc links registry

### ValidationError Struct

Extended `ValidationError` in dispatcher to pass through new fields:

```go
type ValidationError struct {
    Validator   string
    Message     string
    Details     map[string]string
    ShouldBlock bool
    ErrorCode   validator.ErrorCode  // NEW
    FixHint     string               // NEW
    DocLink     string               // NEW
}
```

### Error Output Format

Formatted errors now show structured information:

```text
Git commit missing required flags: -S
   Code: GIT010
   Fix: Add -sS flags: git commit -sS -m "message"
   Docs: https://github.com/smykla-labs/klaudiush/blob/main/docs/errors/GIT010.md
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
func formatErrorMetadata(...)   // Writes code/fix/docs
func formatErrorDetails(...)    // Writes details map
```

## Magic Number Constants

Linter `mnd` flags magic numbers. Define constants:

```go
// Before:
if len(code) < 3 {

// After:
const minCategoryLength = 3
if len(code) < minCategoryLength {
```

## Testing Error Codes

Ginkgo tests for error code functionality:

```go
var _ = Describe("FailWithCode", func() {
    It("creates a failing result with error code", func() {
        result := validator.FailWithCode(validator.ErrGitMissingFlags, "test")

        Expect(result.Passed).To(BeFalse())
        Expect(result.ErrorCode).To(Equal(validator.ErrGitMissingFlags))
        Expect(result.FixHint).To(ContainSubstring("-sS"))
        Expect(result.DocLink).To(ContainSubstring("GIT010"))
    })
})
```

## Files Structure

```text
internal/validator/
├── error_code.go       # ErrorCode type and constants
├── error_code_test.go  # Tests for error codes
├── suggestions.go      # Fix hints registry
├── doc_links.go        # Documentation URLs registry
└── validator.go        # Extended Result type, FailWithCode()
```

## Adding New Error Codes

1. Add constant in `error_code.go`
2. Add suggestion in `suggestions.go`
3. Add doc link in `doc_links.go`
4. Use `FailWithCode()` in validator
