# ADR 0001: Standardized Validator Output Format

## Status

Proposed

## Context

klaudiush validators currently produce inconsistent output formats across different validation types:

- **Git validators** emit structured messages with error codes and fix hints
- **File validators** forward raw linter output (shellcheck, actionlint, tflint)
- **Secrets validators** produce line-based findings with varying formats
- **Shell validators** emit simple messages without location context

This inconsistency creates several problems:

1. **Readability** - Users must mentally parse different output styles
2. **Actionability** - Some errors show what's wrong but not how to fix it
3. **Automation** - Difficult to programmatically parse varied formats
4. **Maintenance** - Each validator implements its own formatting logic

The current `Result` struct provides reference URLs and fix hints, but the actual output formatting is scattered and inconsistent.

## Decision

Adopt a standardized output format with these characteristics:

### Header Line

```text
Failed: {validator1}, {validator2}, ...
```

A single line listing all failing validators, comma-separated.

### Line-Based Findings

For validators that report issues at specific file locations:

```text
  Line {N} {severity} {rule}: {actionable message}
  Lines {N}-{M} {severity} {rule}: {actionable message}
```

Example (single line):

```text
  Line 1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script
  Line 30 ⚠ SC2034: PROVIDER_ENV_SETUP_CMD is unused - export it or remove it
```

Example (line range):

```text
  Lines 15-20 ✖ MD055: Align table columns consistently
  Lines 42-47 ⚠ SEC001: Potential API key detected in multi-line string
```

### Non-Line-Based Findings

For structural or command-level validation:

```text
  {severity} {code}: {actionable message}
```

Example:

```text
  ✖ GIT010: Add -sS flags to your commit command
  ✖ GIT003: Stage files before committing (git add <files>)
```

### Severity Icons

| Icon | Unicode | Severity | Semantics        |
|:----:|:--------|:---------|:-----------------|
| `✖`  | U+2716  | Error    | Blocks operation |
| `⚠`  | U+26A0  | Warning  | Allows, warns    |
| `ℹ`  | U+2139  | Info     | Informational    |

### Suggestions (Copy-Pasteable Fixes)

When a fix can be auto-generated:

```text
  Fix for line {N}:
  {suggested content}
```

### Lists

For validators that enumerate multiple items:

```text
  {header}:
  - item1
  - item2
```

### Reference URLs

Single validator failure:

```text
  Reference: https://klaudiu.sh/{CODE}
```

Multiple validator failures:

```text
  References:
  - https://klaudiu.sh/{CODE1}
  - https://klaudiu.sh/{CODE2}
```

### Complete Examples

**Single validator (line-based):**

```text
Failed: shellscript

  Line 1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script
  Line 30 ⚠ SC2034: PROVIDER_ENV_SETUP_CMD is unused - export it or remove it

  Reference: https://klaudiu.sh/FILE001
```

**With suggestion:**

```text
Failed: shellscript

  Line 1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script

  Fix for line 1:
  #!/bin/bash

  Reference: https://klaudiu.sh/FILE001
```

**Multiple validators:**

```text
Failed: shellscript, markdown

  shellscript
  Line 1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script

  markdown
  Lines 21-25 ✖ MD055: Align table columns consistently

  References:
  - https://klaudiu.sh/FILE001
  - https://klaudiu.sh/FILE005
```

**Non-line-based (git commit):**

```text
Failed: commit

  ✖ GIT010: Add -sS flags to your commit command
  ✖ GIT003: Stage files before committing (git add <files>)

  Reference: https://klaudiu.sh/GIT010
```

### Key Principle

Messages must be **actionable** - tell the user what to do, not what's wrong.

**Bad vs Good messages:**

- Bad: "Missing flag" → Good: "Add -sS flags to your commit command"
- Bad: "Invalid format" → Good: "Use format: type(scope): description"
- Bad: "Shellcheck error" → Good: "Add a shebang (#!/bin/bash) at line 1"
- Bad: "Table formatting error" → Good: "Align table columns (see suggestion)"

### Multi-Line Messages

Continuation lines are indented by 2 spaces relative to the first line:

```text
  ✖ GIT013: Title doesn't follow conventional commits format
    Expected: type(scope): description
    Valid types: feat, fix, docs, style, refactor, test, chore
```

```text
  Line 42 ✖ SEC001: Potential AWS access key detected
    Value matches pattern: AKIA[0-9A-Z]{16}
    Remove or use environment variable instead
```

## Implementation

### Migration Strategy

This is a **big-bang replacement** of the existing `validator.Result` type. The current `validator.Result` struct in `internal/validator/validator.go` will be replaced entirely with the new output-focused types. All validators will be updated in a single PR.

**Rationale:** The tool is not yet in wide use, so backwards compatibility is not a concern. A clean break allows for a simpler, more consistent implementation.

### Shared Output Package

Create `internal/output/` with types for validators to return and a single format function for the dispatcher.

**Core types:**

```go
// Finding represents a single validation finding
type Finding struct {
    Severity   Severity
    Code       string     // e.g., "GIT010", "SC2148"
    Message    string     // actionable message
    Line       int        // 0 if not line-based
    EndLine    int        // 0 if single line or not line-based
    Suggestion string     // optional fix content
    Details    []string   // optional continuation lines
}

// ValidatorResult is returned by each validator
type ValidatorResult struct {
    Name      string      // validator name (e.g., "commit", "shellscript")
    Findings  []Finding
    Reference Reference   // documentation URL
}

// Severity levels
type Severity int

const (
    Error   Severity = iota  // ✖ blocks operation
    Warning                  // ⚠ allows, logs warning
    Info                     // ℹ informational
)
```

**Format function:**

```go
// Format produces the final formatted output from validator results
func Format(results []ValidatorResult, opts ...Option) string

// Option configures formatting behavior
type Option func(*config)

func WithIndent(s string) Option
func WithMaxFindings(n int) Option

// ShouldBlock returns true if any finding has Error severity
func ShouldBlock(results []ValidatorResult) bool
```

### Blocking Semantics

The operation-blocking behavior is determined by finding severity:

- **Error** (`Severity == Error`): Blocks the operation, exit code 2
- **Warning** (`Severity == Warning`): Logs warning, allows operation, exit code 0
- **Info** (`Severity == Info`): Informational only, exit code 0

A result with **any** `Error`-severity finding blocks the operation. This replaces the previous `ShouldBlock` boolean with severity-based semantics.

**Usage in dispatcher:**

```go
fmt.Println(output.Format(results))

if output.ShouldBlock(results) {
    os.Exit(2)
}
```

**Usage in validators:**

```go
func (v *CommitValidator) Validate(ctx, hookCtx) *output.ValidatorResult {
    return &output.ValidatorResult{
        Name: "commit",
        Findings: []output.Finding{
            {Severity: output.Error, Code: "GIT010", Message: "Add -sS flags"},
        },
        Reference: output.RefGitMissingFlags,
    }
}
```

### Validator Updates

Each validator category requires updates:

| Category | Current            | New                       |
|:---------|:-------------------|:--------------------------|
| Git      | Structured w/ refs | Non-line-based findings   |
| File     | Raw linter output  | Line-based w/ suggestions |
| Secrets  | Line-based         | Line-based findings       |
| Shell    | Simple messages    | Non-line-based findings   |
| GitHub   | Structured         | Line or non-line-based    |

### Dispatcher Changes

Update `internal/dispatcher/dispatcher.go` to:

1. Collect all validation errors
2. Group by validator
3. Format using shared output functions
4. Append references section

## Consequences

### Positive

- **Consistent UX** - Users learn one format, apply everywhere
- **Actionable errors** - Every message tells users what to do
- **Copy-pasteable fixes** - Suggestions can be pasted directly
- **Easier debugging** - Line numbers and codes enable quick location
- **Better automation** - Consistent format enables parsing
- **Reduced maintenance** - Shared formatting logic

### Negative

- **Migration effort** - All validators require updates
- **Breaking change** - Tools parsing current output will break
- **Increased complexity** - Linter output must be parsed and reformatted
- **Unicode dependency** - Severity icons require UTF-8 support

### Neutral

- **Documentation** - Error documentation URLs unchanged
- **Exit codes** - No change to exit code semantics
- **Plugin API** - Plugins continue to use their own formatting

## Alternatives Considered

### Alternative 1: JSON Output

```json
{
  "failed": ["shellscript"],
  "errors": [{"line": 1, "severity": "error", "rule": "SC2148"}],
  "reference": "https://klaudiu.sh/FILE001"
}
```

**Rejected because:**

- Poor human readability in terminal
- Requires additional tooling to parse
- Doesn't match CLI tool conventions

### Alternative 2: Keep Current Format

Maintain existing per-validator formatting.

**Rejected because:**

- Inconsistent user experience
- Difficult to learn and remember
- Higher maintenance burden

### Alternative 3: LSP-Style Diagnostics

```text
script.sh:1:1: error SC2148: Tips depend on target shell...
```

**Rejected because:**

- Too dense for multiple errors
- Doesn't support suggestions well
- Less readable for non-IDE contexts

### Alternative 4: SARIF Output

Use Static Analysis Results Interchange Format (SARIF) for structured output.

**Rejected because:**

- Overkill for CLI tool
- Requires JSON parsing
- Not human-readable without tooling

## Related Decisions

- Error code organization (GIT001-GIT023, FILE001-FILE005, SEC001-SEC005)
- Suggestions registry (`internal/validator/suggestions.go`)
- Reference system (`internal/validator/reference.go`)

## Links

- [Issue #60: Formalize validator output format][issue-60]
- [MADR template][madr]
- [Conventional Commits][cc]
- [ShellCheck output formats][shellcheck]

[issue-60]: https://github.com/smykla-labs/klaudiush/issues/60
[madr]: https://adr.github.io/madr/
[cc]: https://www.conventionalcommits.org/
[shellcheck]: https://github.com/koalaman/shellcheck/wiki/Output-formats
