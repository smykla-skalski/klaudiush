# SESS001: Session poisoned by previous blocking error

## Error

A previous command in this Claude Code session was blocked by klaudiush. All subsequent commands are blocked until the original error is acknowledged.

## Why this matters

When klaudiush blocks a command, Claude Code may continue executing queued commands independently. Each would fail one-by-one with confusing errors. Session poisoning provides a fast-fail mechanism that immediately blocks all subsequent commands with a clear reference to the original error.

## How to fix

Acknowledge the original error by adding an unpoison token to your next command:

```bash
# Token format: SESS:<CODE1>[,<CODE2>,...]
# Add as a shell comment:
git commit -sS -m "fix: something"  # SESS:GIT010

# Or via environment variable:
KLACK="SESS:GIT010" git commit -sS -m "fix: something"
```

If multiple codes caused poisoning, acknowledge all of them:

```bash
git commit -sS -m "fix: something"  # SESS:GIT010,GIT004
```

## How session poisoning works

1. A validator returns a blocking error (e.g., GIT010 for missing flags)
2. klaudiush poisons the session, recording all blocking error codes
3. Subsequent commands immediately fail with SESS001
4. To unpoison: include a `SESS:<codes>` token acknowledging the original errors
5. Session resumes normal validation

## Configuration

```toml
[session]
enabled = true
state_file = "~/.klaudiush/session_state.json"
max_session_age = "24h"

[session.audit]
enabled = true
log_file = "~/.klaudiush/session_audit.jsonl"
max_size_mb = 10
max_age_days = 30
```

Disable session tracking:

```toml
[session]
enabled = false
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[SESS001] Session poisoned by previous blocking error. Acknowledge with SESS:<codes> token`

**systemMessage** (shown to user):
Formatted error with unpoison instructions and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [Exception workflow](../EXCEPTIONS_GUIDE.md) - bypassing validation blocks
- [Session guide](../SESSION_GUIDE.md) - session tracking details
