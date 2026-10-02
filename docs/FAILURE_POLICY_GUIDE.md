# Failure policy guide

Decide what happens when klaudiush cannot validate an action.

## Table of contents

- [Overview](#overview)
- [Outcomes](#outcomes)
- [Configuration](#configuration)
- [Deadlines](#deadlines)
- [What providers do with a failed hook](#what-providers-do-with-a-failed-hook)
- [Limits of hooks](#limits-of-hooks)
- [opencode](#opencode)
- [Doctor checks](#doctor-checks)
- [Troubleshooting](#troubleshooting)

## Overview

A check that could not run is neither a pass nor a violation. klaudiush reports it as **validation unavailable** (error code [HOOK001](errors/HOOK001.md)) with the reason, and the failure policy decides whether the action goes ahead with a warning or is denied.

klaudiush answers every failure it can catch with exit code 0 and a response the provider honors. It never relies on a non-zero exit code to stop anything, because every supported provider lets the action through when a hook exits non-zero.

## Outcomes

| Cause | Reason | Default |
|:--|:--|:--|
| A linter is not installed (shellcheck, tofu/terraform, tflint, actionlint, gofumpt, ruff, oxlint, rustfmt, gitleaks when `use_gitleaks` is on) | `missing_tool` | Ignored (the old "Neither 'tofu' nor 'terraform' found" warning now follows `missing_tools` too) |
| A linter or check ran past its timeout | `timeout` | Warning |
| A check was cut short by the hook deadline | `timeout` | Warning |
| A check was canceled | `canceled` | Warning |
| A linter failed without reporting anything | `error` | Warning |
| A validator panicked | `panic` | Warning |
| A plugin timed out, exited non-zero or crashed | `timeout`, `error` | Blocks |
| A plugin answered with unreadable output | `malformed_output` | Blocks |
| A plugin failed to load | `error` | Warning |
| The hook input is not valid JSON | `malformed_input` | Warning |
| The configuration cannot be loaded or is invalid | `config` | Warning |
| klaudiush itself panicked | `panic` | Warning, crash dump written |
| The whole hook ran past its deadline | `timeout` | Warning |
| Session state could not be read or written | `state` | Warning, never blocks |

A check that passed only after its deadline or a cancellation is not trusted: it is reported as unavailable, never recorded as a clean pass, and never resolves earlier findings.

Empty hook input still exits silently: no provider sends it, and running `klaudiush` by hand without input should not print anything.

## Configuration

```toml
[failure_policy]
mode = "block"                          # or "warn"; unset keeps each check's own choice
missing_tools = "warn"                  # "ignore" (default), "warn" or "block"
critical = ["git.commit", "secrets"]    # always block when these cannot run
deadline = "20s"                        # one hook run, default 20s
```

The policy resolves each unavailable check in this order:

1. A validator listed in `critical` blocks, for every reason including a missing tool.
2. A missing tool follows `missing_tools`.
3. Anything else follows `mode` when it is set.
4. Without `mode`, the check keeps its own choice: plugin failures block, everything else warns.

Failures of klaudiush itself (unreadable input, broken configuration, crash, deadline) follow `mode` and warn when it is unset. When the whole hook overruns its deadline, klaudiush cannot tell which check hung, so it blocks whenever any validator is listed in `critical`.

A failure of klaudiush itself only blocks where the event can stop the action before it runs: a tool call about to run, a permission request, an MCP elicitation. After a tool ran, nothing can be stopped, so the failure is reported to the agent. At a completion gate (Stop, SubagentStop, AfterAgent) it only warns, so a broken setup cannot keep the agent working forever.

An unavailable check follows the same event rules as a violation: after a tool it is advisory, at a completion gate it stays bounded by the completion gate limit, and on a prompt submission a blocking plugin failure blocks the prompt, as it did before.

`critical` accepts runtime validator names (`commit`, `git-push`, `shellscript`, `plugin-registry`) and override names (`git.commit`, `git.push`, `file.shellscript`, `plugins`). Plugins can only be made critical together, as `plugins`. A plugin that fails to load warns by default, while one that fails at run time blocks.

A critical validator blocks when any tool it uses is missing. For `file.terraform` that includes tflint while `use_tflint` is on; when only tflint is missing and the format check found something, the format findings are shown instead. For `file.workflow`, a missing actionlint blocks before the tool runs, but after it ran the digest pinning check still decides.

A blocked HOOK001 can be waived like any other code with an [exception token](EXCEPTIONS_GUIDE.md) when exceptions are enabled for it.

### When the configuration cannot be read

The configuration that sets the policy may be what failed. klaudiush then looks for the mode in this order:

1. The `--failure-mode=warn|block` flag of the hook command.
2. The `KLAUDIUSH_FAILURE_POLICY_MODE` environment variable.
3. The configuration read without validation, so an invalid value elsewhere does not hide the mode.
4. A line scan of the project and global files for `mode` under `[failure_policy]`, so a syntax or type error elsewhere does not hide it. The strictest mode found wins.
5. The default, `warn`.

An unreadable mode at any step counts as `block`. Put `--failure-mode=block` in the hook command when the guarantee must survive an edit to the configuration files:

```json
{
  "type": "command",
  "command": "klaudiush --provider claude --event PreToolUse --failure-mode=block",
  "timeout": 30
}
```

## Deadlines

Each hook run has a deadline, 20 seconds by default, measured from when klaudiush starts. klaudiush registers its hooks with a 30 second timeout, which leaves time to answer.

- Validators receive the deadline through their context. Linters and plugins are stopped when it passes, and their output pipes are closed within 2 seconds even if they started background processes.
- Validators that had not started report `timeout`; validators that passed after it report `timeout` instead of a pass.
- A watchdog answers 3 seconds after the deadline if validation still has not finished, for example while stuck reading input or waiting on a lock. The answer follows the policy like any other failure.

Keep `deadline` at least 5 seconds below the hook timeout. `klaudiush doctor --category failure_policy` compares it with the timeouts in your Claude settings.

## What providers do with a failed hook

From the provider documentation:

| Provider | Exit code 2 | Other non-zero exit | Hook killed at its timeout | Default timeout |
|:--|:--|:--|:--|:--|
| [Claude Code](https://code.claude.com/docs/en/hooks) | Blocks, stderr is the reason | Non-blocking error, the action proceeds | Output discarded, the tool call proceeds | 600s |
| [Codex](https://learn.chatgpt.com/docs/hooks) | Blocks | Treated as a failure, the action proceeds | The action proceeds | 600s |
| [Gemini CLI](https://geminicli.com/docs/hooks/reference/) | Blocks, stderr is the reason | Warning, the CLI continues | Not documented | 60s |
| [opencode](https://opencode.ai/docs/plugins/) | n/a (plugin) | Up to the plugin | Up to the plugin | Plugin timeout |

Claude Code also reports invalid JSON on stdout as a non-blocking error, and Codex treats it as a hook failure. That is why klaudiush exits 0 with valid JSON for every failure it catches, including its own crashes. Exit code 3 is left for crashes outside a hook's validation, such as in `doctor` or `debug`.

## Limits of hooks

Some failures happen where klaudiush cannot answer:

- The binary is missing, not executable, or the hook command is wrong.
- The provider kills klaudiush at the hook timeout before the watchdog fires, for example with a timeout shorter than the deadline.
- The machine is out of memory or the process is killed.
- The provider skips hooks entirely (hooks disabled, a session started without them, a tool the hook matcher does not select).

In each of these cases the provider lets the action through. Hooks are a guard for an agent following your rules, not a security boundary. For guarantees, enforce the same rules where the agent cannot skip them:

- Git hooks (`pre-commit`, `commit-msg`, `pre-push`) in the repository.
- Server-side branch protection, required reviews, and required status checks.
- CI jobs that run the same linters on every push.
- Signed-commit requirements on the remote.

## opencode

opencode runs klaudiush through a generated plugin instead of a hook command. The plugin honors any response klaudiush prints, including one printed with a non-zero exit. When klaudiush cannot answer at all (binary missing, killed at the plugin timeout, unreadable output), the plugin decides:

- By default it logs the failure on stderr and lets the tool call through.
- With `KLAUDIUSH_FAILURE_POLICY_MODE=block` in opencode's environment, `tool.execute.before` refuses the tool call with a "Validation unavailable" reason.

The plugin cannot read the configuration file, because reading it is klaudiush's job, so only the environment variable applies there.

## Doctor checks

```bash
klaudiush doctor --category failure_policy
```

- **Validation deadline below hook timeout** warns when `deadline` plus 5 seconds exceeds a klaudiush hook timeout in a Claude settings file. Codex and Gemini settings are not read yet; keep their klaudiush timeouts at 30 seconds or more (Gemini counts milliseconds).
- **Critical validators have their tools** fails when a validator in `critical` needs a tool that is not installed, because it would block every action it checks, and when a name in `critical` matches no validator.

## Troubleshooting

**Every tool call is blocked with HOOK001.** The message names the reason. Run `klaudiush doctor`, fix the cause (install the tool, fix the configuration, raise a timeout), or switch `mode` to `warn` while you do.

**A missing linter is not reported.** `missing_tools` defaults to `ignore`. Set it to `warn`, or list the validator in `critical`.

**The log shows `hook deadline exceeded`.** A validator did not stop at the deadline. The log names the validators that ran. Lower their own timeouts below the deadline, or raise the deadline and the hook timeout together.

**The provider shows a hook error instead of a klaudiush message.** klaudiush could not answer at all; see [limits of hooks](#limits-of-hooks).
