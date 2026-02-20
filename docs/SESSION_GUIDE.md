# Session tracking guide

Session tracking adds fast-fail behavior to Claude Code sessions. When klaudiush denies a command (JSON `permissionDecision: "deny"`), subsequent commands in the same session are immediately rejected with a reference to the original error.

## The problem

Without session tracking:

1. Claude Code executes command A
2. klaudiush denies command A (JSON deny response)
3. Claude Code continues with command B
4. klaudiush evaluates command B independently
5. Command B may also fail, wasting time
6. This repeats for all queued commands

Each command fails one-by-one, with delays between each failure.

## How session tracking helps

With session tracking:

1. Claude Code executes command A in session `abc-123`
2. klaudiush denies command A (JSON deny response)
3. Session `abc-123` is marked as "poisoned" with error code `GIT001`
4. Claude Code executes command B in session `abc-123`
5. klaudiush immediately fails with: "Blocked: session poisoned by GIT001 at 10:30:05"
6. All subsequent commands in `abc-123` fail immediately

Commands fail fast, giving immediate feedback instead of evaluating each one.

## Configuration

### Defaults

Session tracking is enabled by default:

```toml
[session]
enabled = true
state_file = "~/.klaudiush/session_state.json"
max_session_age = "24h"
```

### Disabling session tracking

To disable (not recommended):

```toml
[session]
enabled = false
```

### Custom state file

To store session state elsewhere:

```toml
[session]
state_file = "/custom/path/session_state.json"
```

The state file path supports home directory expansion (`~`).

### Session expiration

Set how long sessions are tracked before cleanup:

```toml
[session]
max_session_age = "48h"  # Keep sessions for 48 hours
```

Valid duration formats: `1h`, `30m`, `24h`, `7d`, etc.

Expired sessions are removed automatically when klaudiush starts, during `IsPoisoned` checks, and on `RecordCommand` calls.

## Error code: SESS001

When a session is poisoned, subsequent commands receive error `SESS001` as a JSON deny response:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "[SESS001] Session poisoned by GIT001, GIT002 at 2025-12-04 10:30:05. Acknowledge violations to unpoison: KLACK=\"SESS:GIT001,GIT002\" your_command",
    "additionalContext": "Automated klaudiush validation check. Fix the reported errors and retry."
  },
  "systemMessage": "Blocked: session poisoned by GIT001, GIT002 at 2025-12-04 10:30:05\n\nDetails:\n  original_error: git commit -sS flag missing\n  unpoison: To unpoison: KLACK=\"SESS:GIT001,GIT002\" command  # or comment: # SESS:GIT001,GIT002\n\nFix: Acknowledge violations to unpoison: KLACK=\"SESS:GIT001,GIT002\" your_command\n\nDocumentation: https://klaudiu.sh/e/SESS001"
}
```

The `unpoison` field contains machine-parseable instructions that Claude Code can use to automatically acknowledge violations.

## Session lifecycle

### Clean state

New sessions start clean:

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

### Poisoned state

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

### Unpoisoned state

Acknowledging the violations returns the session to clean state:

```text
Session: abc-123
Status: Clean (unpoisoned)
Commands executed: 4
```

Normal validation resumes from here.

### Session expiry

After `max_session_age` (default: 24h):

```text
Session: abc-123
Status: Expired (removed from tracking)
```

If Claude Code resumes an expired session, it starts fresh in a clean state.

## Troubleshooting

### Session still poisoned after fix

You fixed the original error but still get `SESS001`.

Add an unpoison token to your next command:

```bash
KLACK="SESS:GIT001" git commit -sS -m "fix"
```

Or start a new Claude Code session. Session state persists across klaudiush invocations but is tied to the Claude Code session ID.

### Session state file corrupted

If the state file can't be loaded, klaudiush recovers by creating a fresh state. All sessions reset to clean. Corrupted state files are logged but don't block operation.

### Old sessions not cleaning up

If the state file keeps growing with old session data, check that cleanup is working. Cleanup runs automatically based on `max_session_age`. To verify manually:

1. Check state file: `cat ~/.klaudiush/session_state.json`
2. Verify `max_session_age` in config
3. Old sessions are removed on next klaudiush invocation

### Session tracking not working

If commands don't fast-fail after a deny response and no `SESS001` errors appear:

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

Fixes:

- `enabled = false` in config: remove or set to `true`
- `session_id` missing from hook JSON: update Claude Code (requires session_id support)
- State file missing: normal on first run, created automatically

### Graceful fallback

If Claude Code doesn't provide `session_id`, klaudiush falls back to its original behavior (no session tracking). This keeps it compatible with older Claude Code versions.

## Unpoisoning a session

To unpoison a session, acknowledge the violations that caused it. This lets you continue working after you've understood the error.

### Token format

Unpoison tokens use the format `SESS:<CODE1>[,<CODE2>,...]`.

As an environment variable (recommended):

```bash
KLACK="SESS:GIT001" git commit -sS -m "fix"
```

As a shell comment:

```bash
git commit -sS -m "fix"  # SESS:GIT001
```

### Multiple codes

When a session is poisoned by multiple violations, acknowledge all codes:

```bash
# Session poisoned by GIT001 and GIT002
KLACK="SESS:GIT001,GIT002" git push origin feature
```

Partial acknowledgment (only some codes) is rejected and the session stays poisoned.

### How unpoisoning works

1. Session is poisoned with one or more error codes (e.g., `GIT001`, `GIT002`)
2. You add an unpoison token to the next command
3. Dispatcher checks whether all poison codes are acknowledged
4. All codes match -- session unpoisoned, command proceeds to validation
5. Partial match -- session stays poisoned, error shows unacknowledged codes

### Example

```text
# Step 1: Command denied, session poisoned
$ git commit -m "fix"  # Missing -sS
→ JSON deny: permissionDecision "deny", reason "[GIT001] ..."

# Step 2: Next command denied immediately (session poisoned)
$ git status
→ JSON deny: permissionDecision "deny", reason "[SESS001] Session poisoned by GIT001 ..."

# Step 3: Acknowledge and fix
$ KLACK="SESS:GIT001" git commit -sS -m "fix"
# Session unpoisoned, command proceeds to validation
```

### Error message with unpoison instructions

Poisoned session errors are returned as JSON deny responses with machine-parseable unpoison instructions in `permissionDecisionReason`:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "[SESS001] Session poisoned by GIT001, GIT002 at 2025-12-04 10:30:05. Acknowledge violations to unpoison: KLACK=\"SESS:GIT001,GIT002\" your_command",
    "additionalContext": "Automated klaudiush validation check. Fix the reported errors and retry."
  },
  "systemMessage": "Blocked: session poisoned by GIT001, GIT002 at 2025-12-04 10:30:05\n\nDetails:\n  original_error: git commit -sS flag missing\n  unpoison: To unpoison: KLACK=\"SESS:GIT001,GIT002\" command  # or comment: # SESS:GIT001,GIT002\n\nFix: Acknowledge violations to unpoison: KLACK=\"SESS:GIT001,GIT002\" your_command"
}
```

Claude Code can parse the `permissionDecisionReason` and `systemMessage` fields to automatically add the token to the next command attempt.

## Audit logging

Session audit logging records poison and unpoison events for troubleshooting.

### Audit configuration

Audit logging is enabled by default:

```toml
[session.audit]
enabled = true
log_file = "~/.klaudiush/session_audit.jsonl"
max_size_mb = 10
max_age_days = 30
max_backups = 5
```

### Disabling audit logging

```toml
[session.audit]
enabled = false
```

### Entry format

Each entry is a single-line JSON object (JSONL):

```json
{
  "timestamp": "2025-12-04T10:30:05Z",
  "action": "Poison",
  "session_id": "abc-123",
  "poison_codes": ["GIT001", "GIT002"],
  "poison_message": "git commit -sS flag missing",
  "command": "git commit -m \"fix\"",
  "working_dir": "/project"
}
```

```json
{
  "timestamp": "2025-12-04T10:31:00Z",
  "action": "Unpoison",
  "session_id": "abc-123",
  "poison_codes": ["GIT001", "GIT002"],
  "source": "env_var",
  "command": "KLACK=\"SESS:GIT001,GIT002\" git commit -sS -m \"fix\"",
  "working_dir": "/project"
}
```

### Entry fields

| Field            | Description                                                |
|------------------|------------------------------------------------------------|
| `timestamp`      | When the action occurred                                   |
| `action`         | `Poison` or `Unpoison`                                     |
| `session_id`     | Claude Code session identifier                             |
| `poison_codes`   | Error codes involved                                       |
| `source`         | Token source: `env_var` or `comment` (unpoison only)       |
| `command`        | Command that triggered the action (truncated to 500 chars) |
| `poison_message` | Original error message (poison only)                       |
| `working_dir`    | Working directory                                          |

### Log rotation

Rotation triggers when the log exceeds `max_size_mb`. Rotated files are named `session_audit.YYYYMMDD-HHMMSS.jsonl`. The oldest backups are deleted when the count exceeds `max_backups`.

### Log cleanup

Entries older than `max_age_days` are removed during rotation. To trigger cleanup manually, restart klaudiush or force a rotation.

### Viewing audit logs

```bash
# View recent entries
tail ~/.klaudiush/session_audit.jsonl | jq

# Filter by action
jq 'select(.action == "Unpoison")' ~/.klaudiush/session_audit.jsonl

# Filter by session
jq 'select(.session_id == "abc-123")' ~/.klaudiush/session_audit.jsonl

# Count poison/unpoison events
jq -s 'group_by(.action) | map({action: .[0].action, count: length})' ~/.klaudiush/session_audit.jsonl
```

## Implementation details

### Session context fields

These fields are extracted from the Claude Code hook JSON:

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

### State persistence

Session state is persisted to `~/.klaudiush/session_state.json` (configurable) as JSON. Writes use the tmp + rename pattern for atomicity. File permissions are 0600, directory permissions 0700. Access is thread-safe via `sync.RWMutex`.

Example state file:

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

### Dispatcher integration

Session tracking fits into the dispatcher flow like this:

```text
1. Hook JSON → Parser → Context (with session fields)
2. Dispatcher.Dispatch(ctx)
3. Check: IsPoisoned(ctx.SessionID)?
   - Yes → Check for unpoison token in command
     - Token valid (all codes acked) → Unpoison(sessionID), continue to validators
     - Token invalid/partial → Return SESS001 JSON deny with unpoison instructions
   - No → Continue to validators
4. Run validators
5. If blocking error:
   - Poison(ctx.SessionID, codes, message)  # codes is []string
   - Return JSON deny output
6. RecordCommand(ctx.SessionID) on success/warning
```

## Tips

For configuration: leave session tracking enabled, use the default state file path, and stick with the 24h expiry unless you have a reason to change it.

For testing: use unique session IDs to avoid cross-contamination, reset the state file between test runs, and use `WithTimeFunc` for deterministic expiry tests.

For production: check that the state file has proper permissions (0600), search logs for `SESS001` to spot repeated failures, and include the state file in your backup strategy if session continuity matters.

## Related documentation

- Architecture: `CLAUDE.md`, "Session Tracking" section
- Configuration schema: `pkg/config/session.go`
- Implementation: `internal/session/`
- Error codes: `.claude/validator-error-format-policy.md` (SESS001)
