# FILE011: Filler comment not allowed

## Error

Code being written or edited contains a comment that looks like filler: prose
that only restates what the adjacent code already says.

If `mode = "strict"` is configured for source files, klaudiush also blocks
in-body comments unless they are an allowed form: a task marker, machine
directive, doc comment on a declaration, test phase marker, or exception token.

## Why this matters

LLM-generated code tends to narrate itself by restating *what* a line does.
Well-named identifiers already communicate the "what"; comments should carry
intent, invariants, protocol notes, or test structure.

## How to fix

Remove the comment and let the code speak, or rename identifiers so the intent
is obvious:

```go
// Instead of:
count := 0 // holds the running total

// Fix: name it well and drop the comment
runningTotal := 0
```

In strict mode, if a non-doc in-body comment is genuinely load-bearing
(documents a non-obvious invariant), append an exception token so it is allowed:

```go
if err != nil {
    return true // EXC:FILE011:unreadable-store-assumes-a-tracker-exists
}
```

## Allowed (not flagged)

- **Task and annotation markers**: `TODO`, `FIXME`, `HACK`, `XXX`, `BUG`, `WARNING`, `NOTE`, `OPTIMIZE`, `REVIEW`, `DEPRECATED`, and `@annotations`.
- **Doc comments** directly above a declaration (Go package/func/type/const/var, JS/TS `export`, Python `def`/`class`). A blank line between the comment and the declaration breaks this exemption. Generic restatements such as `This function does ...` are still filler comments and can be flagged.
- **Standalone BDD/test phase markers** in Go test files (`*_test.go`): full-line `given`, `when`, `then`, `arrange`, `act`, and `assert` comments.
- **Machine directives**: shebangs, Go compiler directives (build constraints, code generation), cgo directives, legacy build tags, character-encoding cookies, and the type/lint/coverage suppression comments recognised by language tooling.
- **Exception tokens**: any comment containing `EXC:<CODE>:<reason>`.
- **PEP 723 inline script metadata** in Python files: a `# /// <type>` line (for example `# /// script`), at least one line that is `#` or `# ...`, and a closing `# ///`, every line a top-level comment starting in column 0, anywhere in the file (also right after a shebang). The block closes on the last `# ///` of its unbroken run of comment lines. A block that is never closed, or is broken by code, an indented or unspaced line, exempts nothing, so comments after it are still checked. An Edit inside an existing block is matched against the file around it.
- **Text inside string literals**: `//` or `#` inside quoted strings, Go raw strings and JS template literals, and Python and TOML triple-quoted strings (`"""`/`'''`), so a Markdown `## Heading` inside a Python docstring or multi-line string is not a comment. A comment inside a Python f-string replacement field (`{...}`) is still a comment. In Python and TOML files an Edit continues the line and multi-line string state found at the edited spot in the file, so new text written into an existing comment is checked as a comment; in other languages an Edit starts in code, so a fragment inside an existing Go raw string or JS template literal can still be flagged; a multi-hunk patch with no anchoring context is scanned line by line, so a heading it adds inside a multi-line string can still be flagged. In Python and TOML files only `#` marks a comment, so Python floor division (`a // b`) is not a comment.
- **All comments in non-source files**: config, markup, data and shell files (`.toml`, `.yaml`, `.json`, `.md`, `.ini`, `.env`, `.sh`, `Makefile`, `Dockerfile`, ...) use the lenient pattern-based behaviour instead.

## Modes

- **filler** (default): blocks only comments matching a filler pattern — a verb-first restatement (initialize, loop, return, configure, handle, parse, encode, ...) or a "This function/method/... does/is/handles/..." restatement. Legitimate "why" comments are allowed.
- **strict**: blocks all in-body comments in source files except the allowed forms above. Non-source files still use filler behavior.

## Configuration

```toml
[validators.file.ai_comments]
enabled = true
mode = "filler"   # "filler" (default) or "strict"
patterns = []     # custom filler-mode patterns (overrides defaults when set)
```

Enable strict block-all behavior for source files:

```toml
[validators.file.ai_comments]
mode = "strict"
```

Disable the validator entirely:

```toml
[validators.file.ai_comments]
enabled = false
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):

Default filler mode:

`[FILE011] Filler comments that only restate the code are not allowed. ...`

Strict mode:

`[FILE011] Inline comments are not allowed — write self-explanatory code instead. ...`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [FILE010](FILE010.md) - linter ignore directives
