# Session Tracking Guide

Session tracking enables fast-fail behavior for Claude Code sessions. When klaudiush blocks a command (exit 2), subsequent commands in the same session are immediately rejected with a reference to the original error.

## Problem Statement

Without session tracking:

1. Claude Code executes command A
2. klaudiush blocks command A (exit 2)
3. Claude Code continues with command B
4. klaudiush evaluates command B independently
5. Command B may also fail, wasting time
6. This repeats for all queued commands

**Result**: Poor UX as each command fails one-by-one, with delays between each failure.

## Solution

With session tracking:

1. Claude Code executes command A in session `abc-123`
2. klaudiush blocks command A (exit 2)
3. Session `abc-123` is marked as "poisoned" with error code `GIT001`
4. Claude Code executes command B in session `abc-123`
5. klaudiush immediately fails with: "Blocked: session poisoned by GIT001 at 10:30:05"
6. All subsequent commands in `abc-123` fail immediately

**Result**: Fast-fail behavior, immediate feedback, no wasted time.

## Configuration Options

### Default Behavior

Session tracking is enabled by default with these settings:

```toml
[session]
enabled = true
state_file = "~/.klaudiush/session_state.json"
max_session_age = "24h"
```

### Disabling Session Tracking

To disable (not recommended):

```toml
[session]
enabled = false
```

### Custom State File

Store session state in a custom location:

```toml
[session]
state_file = "/custom/path/session_state.json"
```

The state file path supports home directory expansion (`~`).

### Session Expiration

Control how long sessions are tracked before automatic cleanup:

```toml
[session]
max_session_age = "48h"  # Keep sessions for 48 hours
```

Valid duration formats: `1h`, `30m`, `24h`, `7d`, etc.

Expired sessions are automatically removed:

- On load (when klaudiush starts)
- During `IsPoisoned` checks
- On `RecordCommand` for expired sessions

## Error Code: SESS001

When a session is poisoned, subsequent commands receive error `SESS001`:

```text
Blocked: session poisoned by GIT001, GIT002 at 2025-12-04 10:30:05

Details:
  original_error: git commit -sS flag missing
  unpoison: To unpoison: KLACK="SESS:GIT001,GIT002" command  # or comment: # SESS:GIT001,GIT002

Fix: Acknowledge violations to unpoison: KLACK="SESS:GIT001,GIT002" your_command

Documentation: https://klaudiu.sh/SESS001
```

The error includes machine-parseable unpoison instructions in the `unpoison` field that Claude Code can use to automatically acknowledge violations.

## Session Lifecycle

### Clean State

New sessions start in a clean state:

```text
Session: abc-123
Status: Clean
Commands executed: 0
```

Each command increments the command count:

```text
Session: abc-123
Status: Clean
Commands executed: 3
```

### Poisoned State

When a validator returns a blocking error:

```text
Session: abc-123
Status: Poisoned
Poison codes: GIT001, GIT002
Poison message: git commit -sS flag missing
Poisoned at: 2025-12-04 10:30:05
Commands executed: 3
```

All subsequent commands in this session immediately fail with `SESS001`.

### Unpoisoned State

When you acknowledge the violations, the session returns to clean state:

```text
Session: abc-123
Status: Clean (unpoisoned)
Commands executed: 4
```

The session can now proceed with normal validation.

### Session Expiry

After `max_session_age` (default: 24h):

```text
Session: abc-123
Status: Expired (removed from tracking)
```

If Claude Code resumes an expired session, it starts fresh in a clean state.

## Troubleshooting

### Session Still Poisoned After Fix

**Problem**: Fixed the original error but still getting `SESS001`.

**Solutions**:

1. **Unpoison the session** (recommended): Add unpoison token to your next command:

   ```bash
   KLACK="SESS:GIT001" git commit -sS -m "fix"
   ```

2. **Start new session**: Start a new Claude Code session. Session state persists across klaudiush invocations but is tied to the Claude Code session ID.

### Session State File Corrupted

**Problem**: Error loading session state file.

**Solution**: klaudiush automatically recovers by creating a fresh state. All sessions reset to clean state.

Corrupted state files are logged but don't block operation.

### Old Sessions Not Cleaning Up

**Problem**: State file growing with old session data.

**Solution**: Session cleanup is automatic based on `max_session_age`. To manually verify:

1. Check state file: `cat ~/.klaudiush/session_state.json`
2. Verify `max_session_age` in config
3. Old sessions are removed on next klaudiush invocation

### Session Tracking Not Working

**Symptoms**:

- Commands don't fast-fail after blocking error
- No `SESS001` errors appearing

**Diagnosis**:

1. Check if enabled:

   ```bash
   grep -A 3 "\[session\]" ~/.klaudiush/config.toml
   ```

2. Verify session_id in hook JSON:

   ```bash
   # Temporarily enable debug logging
   klaudiush --debug --hook-type PreToolUse < test_input.json
   ```

3. Check state file exists:

   ```bash
   ls -l ~/.klaudiush/session_state.json
   ```

**Solutions**:

- If `enabled = false`: Remove or set to `true`
- If session_id missing: Update Claude Code (requires session_id support)
- If state file missing: Normal on first run, will be created automatically

### Graceful Fallback

If Claude Code doesn't provide `session_id`, klaudiush gracefully degrades to original behavior (no session tracking). This ensures compatibility with older Claude Code versions.

## Unpoisoning a Session

When a session is poisoned, you can acknowledge the violations to unpoison it and continue working. This is useful when you've understood the error and want to proceed with a fix.

### Token Format

Unpoison tokens use the format `SESS:<CODE1>[,<CODE2>,...]`:

**Environment Variable** (recommended):

```bash
KLACK="SESS:GIT001" git commit -sS -m "fix"
```

**Shell Comment**:

```bash
git commit -sS -m "fix"  # SESS:GIT001
```

### Multiple Codes

When a session is poisoned by multiple violations, you must acknowledge ALL codes:

```bash
# Session poisoned by GIT001 and GIT002
KLACK="SESS:GIT001,GIT002" git push origin feature
```

Partial acknowledgment (only some codes) is not accepted - the session remains poisoned.

### How It Works

1. Session is poisoned with one or more error codes (e.g., `GIT001`, `GIT002`)
2. User adds unpoison token to next command
3. Dispatcher checks if all poison codes are acknowledged
4. If all codes match → session unpoisoned, command proceeds to validation
5. If partial match → session remains poisoned, error shows unacknowledged codes

### Example Workflow

```text
# Step 1: Command blocked, session poisoned
$ git commit -m "fix"  # Missing -sS
Error: Blocked - git commit -sS flag missing

# Step 2: Next command fails immediately (session poisoned)
$ git status
Error: Blocked: session poisoned by GIT001 at 2025-12-04 10:30:05
       To unpoison: KLACK="SESS:GIT001" command

# Step 3: Acknowledge and fix
$ KLACK="SESS:GIT001" git commit -sS -m "fix"
# Session unpoisoned, command proceeds to validation
```

### Error Message with Unpoison Instructions

When a session is poisoned, the error includes machine-parseable unpoison instructions:

```text
Blocked: session poisoned by GIT001, GIT002 at 2025-12-04 10:30:05

Details:
  original_error: git commit -sS flag missing
  unpoison: To unpoison: KLACK="SESS:GIT001,GIT002" command  # or comment: # SESS:GIT001,GIT002

Fix: Acknowledge violations to unpoison: KLACK="SESS:GIT001,GIT002" your_command
```

Claude Code can parse the `unpoison` field to automatically add the token to the next command attempt.

## Implementation Details

### Session Context Fields

The following fields are extracted from Claude Code hook JSON:

```json
{
  "session_id": "d267099c-6c3a-45ed-997c-2fa4c8ec9b39",
  "tool_use_id": "toolu_012EzpTqLzKXw5C4XP5E733v",
  "transcript_path": "/Users/.../session.jsonl"
}
```

Available in validator context:

```go
ctx.SessionID      // Session identifier
ctx.ToolUseID      // Individual tool invocation ID
ctx.TranscriptPath // Path to session transcript
ctx.HasSessionID() // Check if session ID present
```

### State Persistence

Session state is persisted to disk:

- **Location**: `~/.klaudiush/session_state.json` (configurable)
- **Format**: JSON
- **Atomicity**: Writes use tmp + rename pattern
- **Permissions**: File 0600, directory 0700
- **Thread-safe**: Protected by `sync.RWMutex`

State file example:

```json
{
  "sessions": {
    "abc-123": {
      "session_id": "abc-123",
      "status": "Poisoned",
      "poisoned_at": "2025-12-04T10:30:05Z",
      "poison_codes": ["GIT001", "GIT002"],
      "poison_message": "git commit -sS flag missing",
      "command_count": 3,
      "last_activity": "2025-12-04T10:30:05Z"
    }
  },
  "last_updated": "2025-12-04T10:30:05Z"
}
```

### Integration with Dispatcher

Session tracking integrates with the dispatcher flow:

```text
1. Hook JSON → Parser → Context (with session fields)
2. Dispatcher.Dispatch(ctx)
3. Check: IsPoisoned(ctx.SessionID)?
   - Yes → Check for unpoison token in command
     - Token valid (all codes acked) → Unpoison(sessionID), continue to validators
     - Token invalid/partial → Return SESS001 error with unpoison instructions (exit 2)
   - No → Continue to validators
4. Run validators
5. If blocking error:
   - Poison(ctx.SessionID, codes, message)  # codes is []string
   - Return blocking error (exit 2)
6. RecordCommand(ctx.SessionID) on success/warning
```

## Best Practices

### Configuration Recommendations

- **Leave enabled**: Session tracking improves UX with no downsides
- **Default state file**: Use default path unless specific reason to change
- **Reasonable expiry**: 24h default balances cleanup vs false-resets

### Development

- **Test sessions**: Use unique session IDs in tests to avoid cross-contamination
- **Clean state**: Reset state file between test runs if needed
- **Mock time**: Use `WithTimeFunc` option for deterministic expiry tests

### Production

- **Monitor state file**: Ensure proper permissions (0600)
- **Log analysis**: Search logs for `SESS001` to identify repeated failures
- **State file backup**: Include in backup strategy if session continuity critical

## Related Documentation

- **Architecture**: See `CLAUDE.md` → Architecture → Session Tracking
- **Configuration**: See `pkg/config/session.go` for schema
- **Implementation**: See `internal/session/` for code details
- **Error codes**: See `.claude/validator-error-format-policy.md` for SESS001 details
