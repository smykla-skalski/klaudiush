# Harness testing guide

Check that installed coding-agent harnesses actually enforce what klaudiush decides, not only that klaudiush prints the right JSON.

## Table of contents

- [Overview](#overview)
- [Version matrix](#version-matrix)
- [Running the live checks](#running-the-live-checks)
- [What each scenario proves](#what-each-scenario-proves)
- [Isolation](#isolation)
- [Contract checks in CI](#contract-checks-in-ci)
- [Updating fixtures](#updating-fixtures)
- [Unsupported paths](#unsupported-paths)

## Overview

There are two layers:

- **Live checks** (`mise run test:harness`) run the real Claude Code, Codex and opencode binaries in a disposable home directory. The harness talks to a scripted local model (`internal/harness/model.go`) that replays a tool-call script embedded in the prompt, so every run makes the same calls, needs no credentials, and costs nothing. Tools, permission handling and hooks are the harness's own. Hooks are registered by `klaudiush init --install-hooks`, the same way a user installs them, and run through a shim that records each payload and response. The live checks never run in `mise run test` or CI.
- **Contract checks** run in every `mise run test`. They load the fixtures the live checks captured (`internal/harness/testdata/fixtures/`), check each payload and recorded response against the provider capability table (`pkg/hook/capabilities.go`), and replay each payload through the current klaudiush build.

## Version matrix

Captured on 2026-10-03:

| Harness | Version | How it ran | Result |
|:--|:--|:--|:--|
| Claude Code | 2.1.288 | live, scripted model | all 10 scenarios pass |
| Codex CLI | 0.160.0 | live, scripted model | 7 scenarios pass, 3 not applicable |
| opencode | 2.0.19 | live, scripted model | 7 scenarios pass, 3 not applicable |
| Gemini CLI | not installed | fixtures from the [hook reference](https://geminicli.com/docs/hooks/reference/) | payload and response contracts only |

Every live run writes a JSON report with the exact version of each harness, the events klaudiush registers for it, the events the capability table has contracts for, the unsupported paths below, and a result per scenario. The path is printed at the end of the run (`KLAUDIUSH_HARNESS_REPORT` overrides it).

## Running the live checks

```bash
mise run test:harness            # run, keep fixtures as they are
mise run test:harness:fixtures   # run and rewrite the captured fixtures
```

| Variable | Effect |
|:--|:--|
| `KLAUDIUSH_HARNESS_CLAUDE`, `KLAUDIUSH_HARNESS_CODEX`, `KLAUDIUSH_HARNESS_OPENCODE` | Harness binary to use instead of the one on `PATH`. Takes precedence over `PATH`; a shim named here is resolved the same way. Use an absolute path: a relative one is taken from `internal/harness`, where `go test` runs |
| `KLAUDIUSH_HARNESS_ONLY` | Comma-separated harness names to run, such as `claude,codex` |
| `KLAUDIUSH_HARNESS_TMPDIR` | Where sandboxes are created (default: the system temp directory) |
| `KLAUDIUSH_HARNESS_KEEP=1` | Keep sandboxes on disk for debugging |
| `KLAUDIUSH_HARNESS_REPORT` | Report path |

Each harness binary is resolved before the sandbox is built, because version-manager shims cannot run in the empty sandbox environment:

- Symlinks are followed to the real file.
- A mise shim (a link to the `mise` binary) is resolved with `mise which <tool>`, and an asdf shim (a script running `asdf exec`) with `asdf which <tool>`. Both run in the caller's environment and directory, the same place the shim would run, and never inside the sandbox.
- A mise shim whose tool is not active for the directory is skipped, the way the shim itself falls through to the next `PATH` entry. Any other shim error (a broken or untrusted mise config, a missing asdf version) is not skipped.
- A script whose interpreter is not on the sandbox `PATH` (for example an npm `#!/usr/bin/env node` launcher when node comes from a version manager) is rejected, since it cannot start in the sandbox.
- Resolution happens on first use, so `mise run test` does not call the version managers.
- When a shim or the override variable does not resolve to something the sandbox can run, the harness fails with the reason and the variable to set.

A harness that is not installed, or whose `--version` does not run in the sandbox, is skipped and the report says why. The report records the resolved binary path. A scenario that needs a feature the harness lacks is skipped as unsupported.

## What each scenario proves

Every denial scenario checks the file system, not only the hook response: the file the call would have created must not exist. Every scenario also requires that the harness finished before the timeout, every hook exited 0, every captured payload and response passes the contract checks below, and every event in the installed hook file is one the provider fires.

| Scenario | Claude | Codex | opencode | Proves |
|:--|:--|:--|:--|:--|
| `control_unguarded` | yes | yes | yes | With protection off the same calls create their files, so the denial checks are not vacuous. The user's unrelated hook still runs and is still registered after install |
| `deny_shell` | yes | yes | yes | A denied shell command (`POL001`) never runs, and the reason reaches the model |
| `deny_write` | `Write` | `apply_patch` | `write` | A denied file write never lands |
| `parallel` | yes | yes | yes | Two calls in one model turn: the denied one never runs, the allowed one does |
| `warn_keeps_prompt` | yes | n/a | n/a | A warning (`GIT010` at warning severity) sets no `permissionDecision`, so a command the user did not approve is still refused by Claude's own permission flow |
| `warn_allows` | yes | yes | yes | A warning does not block an approved command |
| `after_tool_repair` | yes | n/a | 2.x | A file a shell command broke is reported after the tool ran (`FILE005`) and the repair request reaches the model |
| `completion_gate` | yes | yes | n/a | `Stop` blocks with `EVID001` while a required check is missing, and the reason comes back to the model as its next instruction |
| `failure_policy` | yes | yes | yes | With `failure_policy.mode = "block"`, an unreadable project configuration denies the call (`HOOK001`) |
| `subagent` | yes | n/a | n/a | A subagent's tool call goes through the same hooks, carries `agent_id`, and is denied |

opencode's pre-tool hook can only refuse a call by throwing, so a warning there is a user notice (`systemMessage`) and `warn_allows` expects no model-facing context on it. The repair request reaches the model after the tool, through `tool.execute.after`.

Completion gates are checked only where the capability table says the event can keep the agent working: Claude and Codex `Stop`. opencode `session.idle` cannot, and Gemini `AfterAgent` is covered by fixtures only.

## Isolation

Each scenario gets its own sandbox with `home/`, `work/`, `bin/` and `captures/` directories, and a fresh scripted model on a loopback port.

- The environment is built from scratch. `HOME`, every `XDG_*` directory, `CODEX_HOME`, `CLAUDE_CONFIG_DIR` and `TMPDIR` point into the sandbox, and nothing is inherited from the caller. That keeps out variables such as `OPENCODE_CONFIG_DIR`, the session sockets of an agent running the suite, and `KLAUDIUSH_*` overrides.
- opencode runs with `--standalone`. Without it, opencode 2.x attaches to the user's background service, which runs with the real configuration.
- The opencode binary is linked into the sandbox `bin/`, so `klaudiush init` finds it on `PATH` and writes the bridge for that version, as it does for a user.
- Codex hooks are trusted the way `/hooks` does: the suite asks `codex app-server` for the hook hashes and records them in the sandbox `config.toml`. The trust bypass flag is not used.
- No credentials are read or copied. The scripted model accepts a fixed placeholder key.
- Claude Code keeps per-project task files under `/tmp/claude-<uid>` whatever `TMPDIR` says, so `CLAUDE_CODE_TMPDIR` points into the sandbox too.
- Before the run the suite records the size, mode and modification time of the real harness hook and configuration files and the klaudiush configuration (never their contents), and fails if any changed afterwards. Credential files are left out: an agent session running elsewhere may refresh its token mid-run.
- opencode loads its OpenAI-compatible provider package from npm on first use, so its run needs network access to the registry. Nothing is written outside the sandbox.
- Every command the suite starts in a sandbox runs in a new session. When the harness exits, and again before the sandbox is removed, the suite kills whatever is still running in those sessions, any process whose `HOME`, `TMPDIR`, `CODEX_HOME`, `CLAUDE_CONFIG_DIR` or `XDG_STATE_HOME` points into the sandbox, and their children. That stops background work such as the Codex plugin clone under `.codex/.tmp/plugins-clone-*` and children left by a harness killed at the timeout. The sandbox is removed, checked again after a pause, and removed again if anything reappeared; the cleanup fails if files keep coming back. With `KLAUDIUSH_HARNESS_KEEP=1` the processes are still stopped and only the files stay.
- Processes are found only on macOS and Linux. While a harness runs, its process tree is listed every 50ms (by parent and session only, without reading environments) and every member is remembered with its start time. A child that starts its own session and outlives its parent (something that daemonizes) is therefore still found, as long as it ran for one listing before its parent exited; one that detaches faster is found only by its environment, which macOS hides for Apple binaries (`/bin/sh`, `sleep`) and Linux hides for non-dumpable processes. A session is forgotten once nothing runs in it, and a remembered process once it exits, so a later process reusing either id is left alone. A process is signalled only while it has the start time it was listed with (on Linux through a pidfd), so a pid reused in between is not hit.
- Harnesses run in their own sessions, so they have no controlling terminal and do not get the terminal's Ctrl-C directly; a run that is canceled or times out kills the whole session.
- Each sandbox starts a keeper on first use: a copy of the test binary in its own session that reads the sessions and processes the sandbox learns about from a pipe. When the pipe closes, because the sandbox was closed or because the test binary died (even by `SIGKILL`), the keeper stops every process it knows of and exits. On Linux each harness also gets `SIGKILL` from the kernel as soon as the test binary dies.

## Contract checks in CI

`internal/harness/contract.go` holds the checks:

- `CheckEvent` accepts only event names the provider fires today (`hook.NativeEventNames`). Stale names fail with the replacement, for example Codex `AfterToolUse` (now `PostToolUse`) and opencode `permission.ask`. klaudiush still normalizes many old spellings, so a hook config or fixture using one would otherwise look fine and never fire.
- `CheckPayload` checks the event name and, for tool events, `tool_name` and `tool_input`.
- `CheckResponse` fails on any top-level or `hookSpecificOutput` field the capability table does not list for that provider and event, on a `hookEventName` that names a different event, and on `permissionDecision` values the provider does not read. Codex treats an unsupported `PreToolUse` field as a failed hook and runs the tool anyway, so this is an enforcement check, not style.

The specs also mutate every recorded response with an extra field and rename Codex events to `AfterToolUse`, and expect both to fail, so the checks cannot pass by accepting everything. `cmd/klaudiush/harness_replay_test.go` replays each payload through the current build and requires the same outcome the harness saw when it was captured. Fixtures whose result depends on earlier hooks of the session (the completion gate baseline) are validated but not replayed.

## Updating fixtures

Run `mise run test:harness:fixtures` after a harness upgrade or a response change. Captures go to a staging directory first; a harness's fixtures are replaced only when every one of its scenarios ran and none failed, so a harness that is not installed, not selected, a known gap, or failing keeps the fixtures it had. Fixtures are redacted before they are written: sandbox paths become `{{WORK}}`, `{{HOME}}` and `{{ROOT}}`, the klaudiush binary path becomes `klaudiush`, and a fixture that still contains a home or temp path, an API key or a token field is rejected. Gemini fixtures (`source: docs`) are written by hand from the published reference; they carry no recorded response and are checked by replay only.

## Unsupported paths

| Harness | Not covered |
|:--|:--|
| Claude Code | `klaudiush init` does not register `Stop`; the completion gate needs it added by hand (see the [evidence guide](EVIDENCE_GUIDE.md)). The `PreToolUse` matcher covers `Bash`, `Write`, `Edit` and `MultiEdit` only |
| Codex | Input sent with `write_stdin` to a running unified-exec session does not fire `PreToolUse`. Hosted tools such as web search never reach command hooks. klaudiush registers no `PostToolUse`, so files a shell command broke are not reported after the tool. New or changed hooks run only after `/hooks` trust review. Subagents spawned through the multi-agent tools are not exercised |
| opencode | `session.idle` cannot keep the agent working, so there is no completion gate. There is no declarative hook config to check coexistence with. Subagents are not driven. Only the installed 2.x release runs live; the 1.x bridge is the one earlier releases shipped and is checked by unit tests only. The 2.x plugin context has no toast, so user notices from the 2.x bridge go to opencode's log |
| Gemini CLI | Not run locally; payload shapes come from the hook reference |
