# 1. Switch hook output from exit-code-2/stderr to JSON stdout

* Status: accepted
* Deciders: @bartsmykla
* Date: 2026-02-19

## Context and problem statement

klaudiush originally communicated validation failures by writing formatted text to stderr and exiting with code 2. This approach has two problems:

1. Claude Code conflates exit-code-2 hook blocks with user permission denials, causing the model to stop and ask the user for approval instead of fixing the error and retrying. ([Issue #24327](https://github.com/anthropics/claude-code/issues/24327))
2. Using `systemMessage` alone (without `permissionDecisionReason`) means Claude receives only a generic "Hook denied this tool" message and never sees the actual validation error. ([Issue #12446](https://github.com/anthropics/claude-code/issues/12446))

Claude Code hooks support richer JSON stdout output with separate channels: `permissionDecisionReason` and `additionalContext` for the model, `systemMessage` for the user.

## Decision drivers

* Claude must see the specific error code, message, and fix hint so it can self-correct.
* The model must distinguish "automated validation block" from "user denied permission."
* Exception bypasses should use `permissionDecision: "allow"` rather than the current block-then-convert-to-warning approach.
* The solution should be a clean cut with no backwards-compatibility flags.

## Considered options

* Keep exit-code-2 and add JSON stdout alongside it.
* Switch entirely to JSON stdout, always exit 0.

## Decision outcome

Chosen option: "Switch entirely to JSON stdout, always exit 0," because it cleanly solves both issues. Exit code 2 is removed. Only exit 3 (crash/panic) remains non-zero.

### JSON structure

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "[GIT001] Missing -s flag. Add -s flag: git commit -sS -m \"message\"",
    "additionalContext": "Automated klaudiush validation check. Fix the reported errors and retry the same command."
  },
  "systemMessage": "\n❌ Validation Failed: commit\n\nMissing -s flag\n   Fix: Add -s flag\n   Reference: https://klaudiu.sh/GIT001\n\n"
}
```

### Mapping

| Scenario | `permissionDecision` | `permissionDecisionReason` | `additionalContext` |
|:--|:--|:--|:--|
| Blocking errors | `"deny"` | `[CODE] msg. Fix hint.` | `"Automated klaudiush validation check..."` |
| Session poisoned | `"deny"` | `[SESS001] msg. Unpoison hint.` | `"Automated klaudiush session check..."` |
| Warnings only | `"allow"` | — | `"klaudiush warning: ... Not blocking."` |
| Bypassed exception | `"allow"` | — | `"klaudiush: Exception EXC:CODE accepted..."` |
| Clean pass | No output | — | — |

### Positive consequences

* Claude sees the actual error and fix hint via `permissionDecisionReason`, enabling self-correction.
* The `additionalContext` field tells Claude this is an automated check, not a user denial.
* Exception bypasses are cleaner: `"allow"` with context instead of block-then-convert.
* Single exit code (0) eliminates the conflation with user permission denials.

### Negative consequences

* Any external tooling that checked for exit code 2 must be updated to parse JSON stdout instead.

## Links

* [Claude Code hooks documentation](https://docs.anthropic.com/en/docs/claude-code/hooks)
* [Issue #24327: Hook blocks conflated with user denials](https://github.com/anthropics/claude-code/issues/24327)
* [Issue #12446: systemMessage without permissionDecisionReason](https://github.com/anthropics/claude-code/issues/12446)
