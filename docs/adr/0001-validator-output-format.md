# ADR 0001: Standardized Validator Output Format

## Status

Proposed

## About This Document

This ADR follows the **[Markdown Any Decision Records (MADR)][madr]** template format, a lightweight standard for documenting architectural decisions. MADR provides a structured approach to capturing the context, decision, consequences, and alternatives for technical choices.

**Why MADR?**
- **Lightweight**: Markdown-based, easy to write and review
- **Structured**: Consistent sections guide complete documentation
- **Collaborative**: Plain text enables version control and code review
- **Standard**: Widely adopted in open-source projects

For more details on the MADR template choice, see [ADR 0000: MADR Template Choice][adr-0000].

[adr-0000]: https://github.com/smykla-labs/klaudiush/blob/main/docs/adr/0000-madr-template-choice.md

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
{Status}: {validator1}, {validator2}, ...
```

A single line listing all validators that produced findings, comma-separated. The status prefix indicates the outcome:

- `Failed:` when any finding has Error severity (blocks operation)
- `Passed:` when all findings are Warning or Info severity (allows operation)

The header is always present when there are findings, ensuring consistent parsing.

### Line-Based Findings

For validators that report issues at specific file locations:

```text
  {path}:{line} {severity} {rule}: {actionable message}
  {path}:{startLine}-{endLine} {severity} {rule}: {actionable message}
```

Example (single line):

```text
  script.sh:1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script
  script.sh:30 ⚠ SC2034: PROVIDER_ENV_SETUP_CMD is unused - export it or remove it
```

Example (line range):

```text
  README.md:15-20 ✖ MD055: Align table columns consistently
  config.yaml:42-47 ⚠ SEC001: Potential API key detected in multi-line string
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
  Fix for {path}:{line}:
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

These examples demonstrate the format's versatility across different scenarios.

#### Example 1: Single validator with errors (line-based)

```text
Failed: shellscript

  deploy.sh:1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script
  deploy.sh:30 ✖ SC2086: Double quote to prevent word splitting

  Reference: https://klaudiu.sh/FILE001
```

#### Example 2: Mixed severity levels (errors and warnings)

```text
Failed: shellscript

  deploy.sh:1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script
  deploy.sh:30 ⚠ SC2034: PROVIDER_ENV_SETUP_CMD is unused - export it or remove it
  deploy.sh:45 ℹ SC2086: Consider quoting variable for safety

  Reference: https://klaudiu.sh/FILE001
```

#### Example 3: With copy-pasteable suggestion

```text
Failed: shellscript

  deploy.sh:1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script

  Fix for deploy.sh:1:
  #!/bin/bash

  Reference: https://klaudiu.sh/FILE001
```

#### Example 4: Line ranges (multi-line findings)

```text
Failed: markdown, secrets

  markdown
  README.md:21-25 ✖ MD055: Align table columns consistently

  secrets
  config.yaml:42-47 ⚠ SEC001: Potential API key detected in multi-line string
    Value matches pattern: [a-zA-Z0-9_-]{32,}
    Consider using environment variable instead

  References:
  - https://klaudiu.sh/FILE005
  - https://klaudiu.sh/SEC001
```

#### Example 5: Non-line-based (command validation)

```text
Failed: commit

  ✖ GIT010: Add -sS flags to your commit command
  ✖ GIT003: Stage files before committing (git add <files>)

  Reference: https://klaudiu.sh/GIT010
```

#### Example 6: Info-only output (no errors or warnings)

```text
Passed: commit

  ℹ Commit message follows conventional commits format
  ℹ All staged files passed validation
```

#### Example 7: No reference URL (validator without documentation)

```text
Failed: custom-plugin

  ✖ CUSTOM001: Configuration file must include required field 'timeout'
```

#### Example 8: Multiple validators with suggestions

```text
Failed: shellscript, terraform

  shellscript
  deploy.sh:1 ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script

  Fix for deploy.sh:1:
  #!/bin/bash

  terraform
  ✖ TF001: Run 'terraform fmt' to format this file
    Detected formatting issues in resource blocks

  Fix:
  terraform fmt main.tf

  References:
  - https://klaudiu.sh/FILE001
  - https://klaudiu.sh/FILE003
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
  config.yaml:42 ✖ SEC001: Potential AWS access key detected
    Value matches pattern: AKIA[0-9A-Z]{16}
    Remove or use environment variable instead
```

### File Context Guidelines

File paths and line numbers provide essential context for actionable error messages. However, their applicability depends on the event type:

**PostToolUse events**: File paths and line numbers refer to the actual file on disk after the tool executed. This is the primary use case for file context.

**PreToolUse events**: File paths and line numbers refer to the *proposed content* before execution. Validators MUST NOT include file paths/lines when validating proposed changes because:

1. The content doesn't exist on disk yet
2. Line numbers in proposed content may not match final file locations
3. Users cannot navigate to non-existent locations

For PreToolUse validation of file writes, use non-line-based format:

```text
Failed: shellscript

  ✖ SC2148: Add a shebang (#!/bin/bash) at the start of the script
  ⚠ SC2034: PROVIDER_ENV_SETUP_CMD is unused - export it or remove it

  Reference: https://klaudiu.sh/FILE001
```

**Exception**: When validating proposed changes to existing files (Edit tool), validators MAY reference the existing file's line numbers for context, but MUST clearly indicate they refer to the original file, not the proposed changes.

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
    Severity  Severity
    Code      string   // e.g., "GIT010", "SC2148"
    Message   string   // actionable message
    File      string   // file path (empty for command-level findings)
    Line      int      // 0 if not line-based
    EndLine   int      // 0 if single line or not line-based
    Column    int      // 0 if not column-specific
    EndColumn int      // 0 if single column or not column-specific
    Suggestion string  // optional fix content
    Details   []string // optional continuation lines
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

## Extensibility and Future-Proofing

The format is designed to accommodate future validator types and finding categories without breaking existing parsers or user expectations.

### Adding New Severity Levels

The severity system uses a fixed set of icons (✖, ⚠, ℹ) that map to error, warning, and info semantics. New severity levels can be added by:

1. **Extending the `Severity` enum** in `internal/output/finding.go`
2. **Mapping to an appropriate icon** in the format function
3. **Defining blocking semantics** in `ShouldBlock()`

Example future additions:
- **Notice** (📌 U+1F4CC): Important information without blocking
- **Deprecated** (🚫 U+1F6AB): Features scheduled for removal

### Adding New Finding Types

The format supports both line-based and non-line-based findings. New types can be added without format changes:

- **File-level findings**: Use non-line-based format with file context
- **Range-based findings**: Extend `Finding` struct with column information
- **Multi-file findings**: Group findings by file under validator section

### Parser Compatibility

The format is designed for stable parsing:

1. **Header line** is always present with format `{Status}: {validators}` (comma-separated)
   - `Failed:` when any finding has Error severity (blocks operation)
   - `Passed:` when all findings are Warning or Info severity (allows operation)
2. **Indentation** is consistent (2 spaces per level)
3. **Icon placement** is predictable (after `path:line` or at start for non-line-based)
4. **Reference section** is always last (when present)

Parsers can rely on these invariants even as new finding types are added.

**Note:** The header is always emitted, even for info-only output. This ensures consistent parsing and provides validator context regardless of severity.

### Configuration Options

Future formatting options can be added via the `Option` pattern:

```go
// Potential future options
func WithColorOutput(enabled bool) Option
func WithCompactMode(enabled bool) Option
func WithMaxLineLength(n int) Option
func WithGroupingStrategy(s GroupingStrategy) Option
```

These options modify presentation without changing the underlying structure.

### Plugin Integration

External validators (Go plugins, exec plugins, gRPC plugins) can return findings in this format by:

1. **Returning `ValidatorResult`** from the plugin interface
2. **Using severity-based blocking** instead of custom logic
3. **Following the same icon/reference conventions**

This ensures consistent output regardless of validator implementation.

### Backwards Compatibility Strategy

When format changes are necessary:

1. **Version the format** in output package (`FormatV1`, `FormatV2`)
2. **Support both versions** during transition period
3. **Deprecate old version** with clear migration timeline
4. **Provide format detection** for automated migration

The current design minimizes the need for breaking changes by using flexible internal types.

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
