# Validator Error Format Policy

Comprehensive guide for error reporting and formatting in klaudiush validators.

## Error Format Structure

Validators return `*validator.Result` with these fields:

```go
type Result struct {
    Passed      bool                      // Whether validation passed
    Message     string                    // Human-readable error message
    Details     map[string]string         // Additional contextual information
    ShouldBlock bool                      // Whether to block the operation
    Reference   validator.Reference       // Error documentation URL
    FixHint     string                    // Short fix suggestion
}
```

**Key semantics:**

- `Passed=true` → validation passed (any other state ignored)
- `Passed=false` + `ShouldBlock=true` → blocks operation (exit 2)
- `Passed=false` + `ShouldBlock=false` → warning only (exit 0)

## Reference System

References are URLs that uniquely identify error types: `https://klaudiu.sh/{CODE}`

```go
type Reference string

const ReferenceBaseURL = "https://klaudiu.sh"

// Example references
RefGitNoSignoff   Reference = "https://klaudiu.sh/GIT001"
RefGitMissingFlags Reference = "https://klaudiu.sh/GIT010"
RefShellcheck     Reference = "https://klaudiu.sh/FILE001"
```

### Reference Methods

- **`Code()`** - Extracts error code: `"GIT001"` from full URL
- **`Category()`** - Extracts category prefix: `"GIT"`, `"FILE"`, `"SEC"`
- **`String()`** - Returns full URL

### Error Code Organization

**GIT001-GIT023**: Git operations

- GIT001: Missing signoff (`-s`)
- GIT002: Missing GPG sign (`-S`)
- GIT003: No staged files
- GIT004: Commit title issues
- GIT005: Commit body line length
- GIT006: Infrastructure scope misuse (`feat(ci)` instead of `ci(...)`)
- GIT007: Missing remote
- GIT008: Missing branch
- GIT009: File doesn't exist
- GIT010: Missing required flags
- GIT011: PR reference in commit
- GIT012: Claude attribution
- GIT013: Invalid conventional commit
- GIT014: Forbidden pattern
- GIT015: Signoff identity mismatch
- GIT016: List formatting issues
- GIT017: Merge commit validation failure
- GIT018: Missing signoff in merge body
- GIT019: Blocked files in git add (e.g., tmp/*)
- GIT020: Branch naming violations (spaces, uppercase, patterns)
- GIT021: --no-verify flag not allowed
- GIT022: Kong org push to origin remote blocked
- GIT023: PR validation failure (title, body, markdown, or labels)

**FILE001-FILE005**: File validation

- FILE001: Shellcheck failure
- FILE002: Terraform fmt failure
- FILE003: Tflint failure
- FILE004: Actionlint failure
- FILE005: Markdown linting failure

**SEC001-SEC005**: Security

- SEC001: API key detected
- SEC002: Hardcoded password
- SEC003: Private key detected
- SEC004: Token detected
- SEC005: Connection string with credentials

**SHELL001-SHELL005**: Shell operations

- SHELL001: Command substitution in double-quoted strings

## Suggestions Registry

`internal/validator/suggestions.go` maps references to fix hints:

```go
var DefaultSuggestions = map[Reference]string{
    RefGitMissingFlags: "Add -sS flags: git commit -sS -m \"message\"",
    RefGitNoStaged:     "Stage files first: git add <files> && git commit -sS -m \"message\"",
    RefGitBadTitle:     "Use format: type(scope): description (max 50 chars)",
    RefShellcheck:      "Run 'shellcheck <file>' to see detailed errors",
    // ... 20+ more
}

func GetSuggestion(ref Reference) string { ... }
```

**Characteristics:**

- Short, actionable guidance
- Specific to error type
- Auto-populated by `FailWithRef()`/`WarnWithRef()`
- Returns empty string if no suggestion exists

## Result Construction Patterns

### Basic Patterns

```go
// Passing
validator.Pass()
validator.PassWithMessage("Notice message")

// Failing (blocks)
validator.Fail("Error message")

// Warning (allows)
validator.Warn("Warning message")

// With details
validator.FailWithDetails("message", map[string]string{"key": "value"})
```

### Reference-Based Patterns

```go
// Fail with reference (auto-populates FixHint)
validator.FailWithRef(
    validator.RefGitMissingFlags,
    "Git commit missing required flags: -s -S",
)

// Warn with reference
validator.WarnWithRef(
    validator.RefGitBadTitle,
    "Title exceeds 50 characters",
)

// Add details
validator.FailWithRef(ref, "message").
    AddDetail("errors", "detailed output").
    AddDetail("help", "additional context")
```

### How FailWithRef Works

```go
func FailWithRef(ref Reference, message string) *Result {
    return &Result{
        Passed:      false,
        Message:     message,
        ShouldBlock: true,
        Reference:   ref,
        FixHint:     GetSuggestion(ref),  // Auto-populated from registry
    }
}
```

**Key behavior:**

- `FixHint` automatically retrieved from `DefaultSuggestions`
- Single constructor - never set `FixHint` manually
- Empty suggestion doesn't error - defaults to empty string

## Error Display Format

Errors are formatted and displayed by dispatcher (`internal/dispatcher/dispatcher.go`):

```text
❌ Validation Failed: commit shellscript

Git commit missing required flags: -S
   Fix: Add -sS flags: git commit -sS -m "message"
   Reference: https://klaudiu.sh/GIT010

<additional details from Details map>
```

### Formatting Components

1. **Header**: Shows failing validators (strips `validate-` prefix)
2. **Message**: Main error description
3. **Fix hint**: From suggestions registry (indented 3 spaces)
4. **Reference URL**: Documentation link (indented 3 spaces)
5. **Details**: Additional context from `Details` map (multi-line indented)

## Real-World Examples

### Example 1: Missing Git Flags

`internal/validators/git/commit.go`:

```go
func (v *CommitValidator) checkFlags(gitCmd *parser.GitCommand) *validator.Result {
    requiredFlags := []string{"-s", "-S"}
    missingFlags := []string{}

    for _, flag := range requiredFlags {
        if !gitCmd.HasFlag(flag) {
            missingFlags = append(missingFlags, flag)
        }
    }

    if len(missingFlags) > 0 {
        return validator.FailWithRef(
            validator.RefGitMissingFlags,
            "Git commit missing required flags: "+strings.Join(missingFlags, " "),
        ).AddDetail("help", helpMessage)
    }

    return validator.Pass()
}
```

**Result:**

- ✅ Automatic `FixHint`: "Add -sS flags: git commit -sS -m \"message\""
- ✅ Reference: `https://klaudiu.sh/GIT010`
- ✅ Details with help message
- ✅ Blocks operation

### Example 2: Multi-Reference Commit Message

`internal/validators/git/commit_message.go`:

```go
func (v *CommitValidator) buildErrorResult(
    results []*RuleResult,
    message string,
) *validator.Result {
    // Select most important error by priority
    ref := selectPrimaryReference(results)

    // Collect all errors
    var details strings.Builder
    for _, result := range results {
        for _, err := range result.Errors {
            details.WriteString(err + "\n")
        }
    }

    return validator.FailWithRef(
        ref,
        "Commit message validation failed",
    ).AddDetail("errors", details.String())
}
```

**Priority order** (most to least important):

1. `GIT013` - Conventional commit format (fundamental)
2. `GIT006` - Infrastructure scope misuse (semantic)
3. `GIT004` - Title issues
4. `GIT005` - Body issues
5. `GIT016` - List formatting
6. `GIT011` - PR references
7. `GIT012` - AI attribution
8. `GIT014` - Forbidden patterns
9. `GIT015` - Signoff mismatch

### Example 3: Shellcheck Integration

`internal/validators/file/shellscript.go`:

```go
func (v *ShellScriptValidator) Validate(
    ctx context.Context,
    hookCtx *hook.Context,
) *validator.Result {
    result := v.checker.Check(lintCtx, content)
    if result.Success {
        return validator.Pass()
    }

    return validator.FailWithRef(
        validator.RefShellcheck,
        v.formatShellCheckOutput(result.RawOut),
    )
}
```

**Automatic:**

- ✅ `FixHint`: "Run 'shellcheck' to see detailed errors"
- ✅ `Reference`: `https://klaudiu.sh/FILE001`
- ✅ Formatted linter output in message

## Plugin Error Handling

Plugins manage their own error documentation (`pkg/plugin/api.go`):

```go
type ValidateResponse struct {
    Passed      bool              // Validation result
    ShouldBlock bool              // Whether to block
    Message     string            // Error message
    ErrorCode   string            // Plugin's error ID (internal)
    FixHint     string            // Fix suggestion
    DocLink     string            // Plugin documentation URL
    Details     map[string]string // Additional context
}

func FailWithCode(code, message, fixHint, docLink string) *ValidateResponse {
    return &ValidateResponse{
        Passed:      false,
        ShouldBlock: true,
        Message:     message,
        ErrorCode:   code,
        FixHint:     fixHint,
        DocLink:     docLink,
    }
}
```

**Key pattern:** Plugins use `DocLink` (custom URL) instead of klaudiush's references, enabling plugin-specific documentation.

## Best Practices

### Checklist for Developers

1. ✅ **Use existing references** - Check `internal/validator/reference.go` first
2. ✅ **Always use FailWithRef/WarnWithRef** - Never manually set `FixHint`
3. ✅ **Add details for context** - Use `.AddDetail()` for error output, logs, examples
4. ✅ **Format messages concisely** - Focus on "why" not "how"
5. ✅ **Include examples** - Show current vs. expected in details
6. ✅ **Don't block unnecessarily** - Use `Warn()` for non-breaking issues

### Template for New Validators

```go
func (v *MyValidator) Validate(
    ctx context.Context,
    hookCtx *hook.Context,
) *validator.Result {
    log := v.Logger()

    // Validate condition
    if !isValid(hookCtx) {
        log.Debug("validation failed", "reason", "...", "input", "...")

        return validator.FailWithRef(
            validator.RefXXXError,
            "Clear, specific error message",
        ).AddDetail("help", "Additional context or examples")
    }

    log.Debug("validation passed")
    return validator.Pass()
}
```

## Exit Code Behavior

- **Exit 0**: All pass OR only warnings
- **Exit 2**: Any `ShouldBlock=true`

Dispatcher checks:

```go
func ShouldBlock(errors []*ValidationError) bool {
    for _, err := range errors {
        if err.ShouldBlock {
            return true
        }
    }
    return false
}
```

## Key Files Reference

| File                                        | Purpose                                |
|:--------------------------------------------|:---------------------------------------|
| `internal/validator/reference.go`           | Reference constants and methods        |
| `internal/validator/suggestions.go`         | Fix hint registry (DefaultSuggestions) |
| `internal/validator/validator.go`           | Result type and constructors           |
| `internal/dispatcher/dispatcher.go`         | Error formatting and display           |
| `internal/validators/git/commit.go`         | Git validator examples                 |
| `internal/validators/git/commit_message.go` | Multi-reference handling               |
| `internal/validators/file/shellscript.go`   | File validator examples                |
| `pkg/plugin/api.go`                         | Plugin API for custom validators       |
| `.claude/session-error-reporting.md`        | Design decisions and history           |

## Summary

This system ensures:

- ✅ Consistent error formatting across all validators
- ✅ Automatic fix suggestions from registry
- ✅ Unique documentation links per error type
- ✅ Detailed context in error output
- ✅ Plugin compatibility with custom docs
- ✅ Clear blocking vs. warning semantics
