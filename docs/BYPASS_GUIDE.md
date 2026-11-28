# Bypass permissions guide

Control what klaudiush does when the session runs without approval prompts.

## Table of contents

- [Overview](#overview)
- [Quick start](#quick-start)
- [CLI commands](#cli-commands)
- [Configuration](#configuration)
- [The reminder](#the-reminder)
- [Bypass modes](#bypass-modes)
- [Environment variables](#environment-variables)
- [Troubleshooting](#troubleshooting)

## Overview

Every supported provider has a mode that turns off approval prompts:

| Provider | Flag | Permission mode |
| --- | --- | --- |
| Claude Code | `--dangerously-skip-permissions` | `bypassPermissions` |
| Codex | `--dangerously-bypass-approvals-and-sandbox` | `danger-full-access` |
| Gemini CLI | `--yolo` | `yolo` |

klaudiush validates every hook in those modes. Skipping prompts is a statement about how much you want to be asked, not about which commit conventions apply, so the two settings are kept separate. Opt out explicitly when you want the old short-circuit behavior.

## Quick start

```bash
# See what happens in bypass modes right now
klaudiush bypass status

# Stop validating in bypass modes, for this project
klaudiush bypass skip --reason "throwaway spike"

# Same, for every project
klaudiush bypass skip --global

# Time-boxed opt-out, expires on its own
klaudiush bypass skip --duration 4h --reason "migration sprint"

# Back to the default
klaudiush bypass enforce

# Keep validating, stop reminding me
klaudiush bypass notify off
```

## CLI commands

### klaudiush bypass status

Prints the project setting, the global setting when asked, and the effective merged result the hook applies.

```bash
klaudiush bypass status            # Project setting plus effective result
klaudiush bypass status --global   # Global setting plus effective result
klaudiush bypass status --all      # Both scopes
```

### klaudiush bypass skip

Writes `skip_validation = true` to the project config, or the global config with `--global`.

| Flag | Description |
| --- | --- |
| `--reason`, `-r` | Why validation is skipped, stored in the config |
| `--duration`, `-d` | How long the opt-out lasts (`4h`, `7d`), stored as `expires_at` |
| `--global` | Write to the global config instead of the project one |

An expired opt-out stops applying without any cleanup - validation resumes on its own, and `klaudiush bypass status` reports `ENFORCED (skip expired)`. An `expires_at` that is not valid RFC3339 counts as expired, so a typo resumes validation rather than skipping forever; `status` says `ENFORCED (expires_at is not RFC3339, skip ignored)`.

### klaudiush bypass enforce

Restores the default by writing `skip_validation = false`. The value is stored explicitly rather than removed, because an absent setting inherits from the wider scope - a project that merely omits the setting cannot override `klaudiush bypass skip --global`.

Every write prints the merged result as `Effective now:`, so a setting that loses to another scope is visible instead of assumed.

### klaudiush bypass notify

Toggles the reminder without changing what gets validated.

```bash
klaudiush bypass notify off
klaudiush bypass notify on
```

The general output controls can also hide this reminder:

```toml
[output]
bypass_reminder = false
```

## Configuration

```toml
[bypass_permissions]
skip_validation = false
notify = true
modes = ["bypassPermissions", "danger-full-access", "yolo"]
```

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `skip_validation` | bool | `false` | Skip every validator while a bypass mode is active |
| `notify` | bool | `true` | Show the user-only reminder |
| `modes` | list | provider defaults | Permission mode values treated as bypass modes |
| `reason` | string | - | Why validation is skipped |
| `skipped_at` | string | - | RFC3339 timestamp when skipping was enabled |
| `expires_at` | string | - | RFC3339 timestamp when skipping stops applying |
| `skipped_by` | string | - | Who enabled skipping |

The last four fields are written by `klaudiush bypass skip`. Project config wins over global config, same as everywhere else.

See [examples/config/bypass-permissions.toml](../examples/config/bypass-permissions.toml) for a commented reference.

## The reminder

While klaudiush validates a session that runs without approval prompts, it emits this in the `systemMessage` field:

```text
klaudiush still validates with approval prompts off (bypassPermissions).
Not what you want? klaudiush bypass skip [--global]
Keep validating, hide this note: klaudiush bypass notify off
```

`systemMessage` is shown to you, not to the AI - it never reaches the model's context, so the agent cannot act on it or be nudged into disabling validation. The reminder appears once per session, tracked in `$XDG_STATE_HOME/klaudiush/bypass_notice.json`, and records older than seven days are pruned. Sessions without a session ID share one record.

## Bypass modes

`modes` replaces the built-in list rather than extending it. To treat Claude's `dontAsk` as a bypass mode as well, list every mode you want:

```toml
[bypass_permissions]
modes = ["bypassPermissions", "danger-full-access", "yolo", "dontAsk"]
```

The permission mode arrives in the hook payload as `permission_mode`. Anything the provider does not send leaves the field empty, and an empty mode is never treated as a bypass.

Nothing restricts `modes` to prompt-free modes. Listing an ordinary mode such as `default` alongside `skip_validation = true` turns klaudiush off for ordinary sessions - the same reach a rule with `action = "allow"` already has, and worth knowing before committing either to a shared repo. `klaudiush bypass status` spells the list out (`SKIPPED in permission modes: default, acceptEdits`) rather than reporting the generic bypass-mode wording.

## Environment variables

```bash
export KLAUDIUSH_BYPASS_PERMISSIONS_SKIP_VALIDATION=true
export KLAUDIUSH_BYPASS_PERMISSIONS_NOTIFY=false
export KLAUDIUSH_BYPASS_PERMISSIONS_MODES=bypassPermissions,dontAsk
```

Environment variables outrank both config files, which makes them a good fit for one-off shells and CI jobs.

## Troubleshooting

### Validation still runs after klaudiush bypass skip

Check which scope holds the setting and whether it expired:

```bash
klaudiush bypass status --all
```

A project config wins over a global one, so a project-level `skip_validation = false` overrides `klaudiush bypass skip --global`. That is what `klaudiush bypass enforce` writes.

Check the expiry too - an `expires_at` in the past, or one that is not valid RFC3339, makes the skip stop applying.

### The reminder does not appear

It shows once per session. Delete the state file to see it again:

```bash
rm ~/.local/state/klaudiush/bypass_notice.json
```

It is also suppressed when `notify = false`, when `output.bypass_reminder = false`, when validation is being skipped, and when the payload carries no `permission_mode`.

### I want validation off entirely, not just in bypass modes

That is a different setting. Use `klaudiush disable` for specific error codes or validators, and see the [rules guide](RULES_GUIDE.md) for conditional behavior.

## See also

- [Exception workflow guide](EXCEPTIONS_GUIDE.md) - one-off bypasses for a single blocked command
- [Rules guide](RULES_GUIDE.md) - conditional validation by repo, branch, or path
- [Environment variables reference](../ENVIRONMENT_VARIABLES.md)
