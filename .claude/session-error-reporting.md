# Error Reporting System Architecture

Structured error references with fix hints and documentation URLs for validation failures.

## Core Design Philosophy

**Unified Reference URLs**: Single `Reference` field combines error code and documentation link (`https://klaudiu.sh/{CODE}`). No separate ErrorCode/DocLink fields - simpler API surface and easier to work with.

**Auto-Populated Fix Hints**: Validators use `FailWithRef(ref, msg)` which automatically looks up fix hints from suggestions registry. Never set FixHint manually - keeps suggestions centralized and consistent.

**Plugin-Owned Documentation**: Plugins provide their own error URLs. Core system doesn't manage plugin error codes or documentation - plugins are fully autonomous.

**Error Priority System**: When multiple validators fail, only the highest-priority error's reference is shown. Priority determined by validator registration order in registry.

## Reference System

### Reference Type

```go
// internal/validator/reference.go
type Reference string

const ReferenceBaseURL = "https://klaudiu.sh"

const (
    RefGitNoSignoff    Reference = ReferenceBaseURL + "/GIT001"
    RefGitMissingFlags Reference = ReferenceBaseURL + "/GIT010"
    RefShellcheck      Reference = ReferenceBaseURL + "/FILE001"
    RefSecretsAPIKey   Reference = ReferenceBaseURL + "/SEC001"
)

// Methods
func (r Reference) String() string  // Full URL
func (r Reference) Code() string    // Extract code: "GIT001" from URL
func (r Reference) Category() string // Extract category: "GIT" from "GIT001"
```

### Error Code Organization

Error codes use category prefixes with numeric suffixes:

- **GIT001-GIT024**: Git operations (signoff, GPG, staging, commits, push, PR)
- **FILE001-FILE009**: File validation (shellcheck, terraform, actionlint, markdown, gofumpt, ruff, oxlint, rustfmt)
- **SEC001-SEC005**: Security (API keys, passwords, tokens, connection strings, gitleaks)
- **SHELL001-SHELL005**: Shell validation (backticks in double-quotes)

**Adding New Categories**: Define new prefix in reference.go constants (e.g., `DOCKER001-DOCKER999` for Docker validators).

## Suggestions Registry

Fix hints keyed by Reference for centralized management:

```go
// internal/validator/suggestions.go
var DefaultSuggestions = map[Reference]string{
    RefGitMissingFlags: "Add -sS flags: git commit -sS -m \"message\"",
    RefShellcheck:      "Run 'shellcheck <file>' to see detailed errors",
    RefSecretsAPIKey:   "Remove API key or add to secrets allow list",
}
```

**Why Map-Based**: Single source of truth for fix hints. Changing suggestion text updates all validators using that reference. No scattered string literals across validators.

## Validator Integration

### Using FailWithRef/WarnWithRef

```go
// BAD - Manual FixHint management
result := validator.Fail("Git commit missing required flags: -s -S")
result.FixHint = "Add -sS flags: git commit -sS -m \"message\""
result.Reference = validator.RefGitMissingFlags

// GOOD - Auto-populated from suggestions registry
return validator.FailWithRef(
    validator.RefGitMissingFlags,
    "Git commit missing required flags: -s -S",
)
```

`FailWithRef` automatically:

1. Sets `Reference` field to provided reference URL
2. Looks up `FixHint` in suggestions registry
3. Returns Result with `Passed=false`, `ShouldBlock=true`

`WarnWithRef` does the same but with `ShouldBlock=false`.

**Gotcha**: If reference not in suggestions registry, FixHint is empty string. This is intentional - not all errors need fix hints (e.g., some are informational only).

### ValidationError Struct

```go
// internal/dispatcher/errors.go
type ValidationError struct {
    Validator   string                    // Validator name
    Message     string                    // Error message
    Details     map[string]string         // Additional context
    ShouldBlock bool                      // Should block command
    Reference   validator.Reference       // Error documentation URL
    FixHint     string                    // Fix suggestion
}
```

Dispatcher converts `validator.Result` → `ValidationError` for output formatting.

### Error Output Format

```text
❌ Validation Failed: git

Git commit missing required flags: -S
   Fix: Add -sS flags: git commit -sS -m "message"
   Reference: https://klaudiu.sh/GIT010
```

Format uses ANSI colors for terminal output:

- Red `❌` for blocking errors
- Yellow `⚠️` for warnings
- Indented fix hints and references

## Plugin System Integration

### Plugin-Provided Documentation

Plugins manage their own error codes and documentation:

```go
// internal/plugin/adapter.go
func (a *ValidatorAdapter) Validate(ctx *hook.Context) validator.Result {
    // ... call plugin ...

    // Use plugin's documentation URL directly
    if resp.DocLink != "" {
        result.Reference = validator.Reference(resp.DocLink)
    }
    result.FixHint = resp.FixHint

    return result
}
```

Example plugin response:

```go
// In plugin code
return pluginapi.FailWithCode(
    "MYPLUGIN001",                                      // ErrorCode (for logging)
    "validation failed",                                // Message
    "try using --flag",                                 // FixHint
    "https://my-plugin.smyk.la/errors/MYPLUGIN001",    // DocLink (becomes Reference)
)
```

**Why This Way**: Plugins are autonomous. Core system doesn't maintain registry of plugin error codes. Plugins can evolve documentation independently without core changes.

## Testing Patterns

### Reference Functionality Tests

```go
var _ = Describe("FailWithRef", func() {
    It("auto-populates fix hint from suggestions registry", func() {
        result := validator.FailWithRef(validator.RefGitMissingFlags, "test message")

        Expect(result.Passed).To(BeFalse())
        Expect(result.ShouldBlock).To(BeTrue())
        Expect(result.Reference).To(Equal(validator.RefGitMissingFlags))
        Expect(result.FixHint).To(ContainSubstring("-sS"))
    })

    It("returns empty fix hint when reference not in registry", func() {
        unknownRef := validator.Reference("https://klaudiu.sh/UNKNOWN999")
        result := validator.FailWithRef(unknownRef, "test")

        Expect(result.FixHint).To(BeEmpty())
    })
})
```

### Reference Methods Tests

```go
var _ = Describe("Reference methods", func() {
    It("extracts code from URL", func() {
        ref := validator.RefGitMissingFlags // "https://klaudiu.sh/GIT010"
        Expect(ref.Code()).To(Equal("GIT010"))
    })

    It("extracts category from code", func() {
        ref := validator.RefGitMissingFlags
        Expect(ref.Category()).To(Equal("GIT"))
    })
})
```

## Cognitive Complexity Refactoring

When `gocognit` linter flags high complexity in error formatting, extract helpers:

```go
// Before: formatErrorList had complexity 23 (too high)
func formatErrorList(header string, errors []*ValidationError) string {
    var builder strings.Builder
    // ... 50 lines of nested logic ...
    return builder.String()
}

// After: Split into single-purpose helpers
func formatErrorList(header string, errors []*ValidationError) string {
    var builder strings.Builder
    formatErrorHeader(&builder, header, errors)
    for _, err := range errors {
        formatSingleError(&builder, err)
    }
    return builder.String()
}

func formatErrorHeader(b *strings.Builder, header string, errors []*ValidationError)
func formatSingleError(b *strings.Builder, err *ValidationError)
func formatErrorMetadata(b *strings.Builder, err *ValidationError)  // Fix/Reference lines
func formatErrorDetails(b *strings.Builder, details map[string]string)
```

Each helper has single responsibility, reducing cognitive complexity of main function.

## Magic Number Constants

`mnd` linter flags magic numbers in code. Define named constants:

```go
// BAD - Magic number
if len(code) < 3 {
    return ""
}

// GOOD - Named constant explains meaning
const minCodeLength = 3  // Minimum valid error code length (e.g., "GIT")
if len(code) < minCodeLength {
    return ""
}
```

## Adding New Error References

1. **Define reference** in `internal/validator/reference.go`:

   ```go
   const RefGitNewError Reference = ReferenceBaseURL + "/GIT025"
   ```

2. **Add suggestion** in `internal/validator/suggestions.go`:

   ```go
   var DefaultSuggestions = map[Reference]string{
       RefGitNewError: "Fix hint for new error",
   }
   ```

3. **Use in validator**:

   ```go
   return validator.FailWithRef(validator.RefGitNewError, "error message")
   ```

4. **Test** in validator's test file:

   ```go
   It("returns correct reference", func() {
       result := sut.Validate(ctx)
       Expect(result.Reference).To(Equal(validator.RefGitNewError))
       Expect(result.FixHint).ToNot(BeEmpty())
   })
   ```

## Common Pitfalls

1. **Setting FixHint manually**: Never set `result.FixHint` directly. Use `FailWithRef()` which auto-populates from suggestions registry. Manual setting bypasses centralized suggestions.

2. **Creating Reference without suggestion**: If you add `RefXxx` constant but forget entry in `DefaultSuggestions`, FixHint will be empty. This is valid (not all errors need hints) but usually unintended.

3. **Using separate ErrorCode and DocLink fields**: Old pattern had separate fields. Current design uses single `Reference` URL. Don't try to set both.

4. **Not testing reference and fix hint**: Always verify both `result.Reference` and `result.FixHint` in tests. Easy to forget fix hint verification.

5. **Managing plugin error codes in core**: Plugins provide their own `DocLink` which becomes `Reference`. Don't try to map plugin codes to core references - plugins are autonomous.

6. **Hardcoding fix hints in error messages**: Keep fix hints in suggestions registry, not in error message text. Allows updating hints without changing error messages.

7. **Magic numbers without constants**: Linter will flag numbers like `3` for min code length. Define named constants explaining what number means.

8. **High cognitive complexity in formatters**: Error formatting can get complex quickly. Extract single-purpose helper functions when complexity exceeds 15.
