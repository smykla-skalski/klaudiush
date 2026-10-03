# Evidence guide

Keep an agent from finishing until required checks passed against the files as they are now.

## Table of contents

- [Overview](#overview)
- [Configuration](#configuration)
- [When a check is required](#when-a-check-is-required)
- [What counts as a result](#what-counts-as-a-result)
- [Running checks with the verifier](#running-checks-with-the-verifier)
- [Review receipts](#review-receipts)
- [Provider coverage](#provider-coverage)
- [Gemini tool phases](#gemini-tool-phases)
- [What the gate cannot prevent](#what-the-gate-cannot-prevent)
- [Inspecting results](#inspecting-results)
- [Troubleshooting](#troubleshooting)

## Overview

The evidence gate is off by default. When it is on, every required check has a list of files it covers. If a session changed any of those files, the completion gate (Claude `Stop`, Codex `Stop`, Gemini `AfterAgent`) blocks with [EVID001](errors/EVID001.md) until the check passed against exactly the current content. A test result is tied to a SHA-256 digest of every covered file; a review result is tied to the exact diff against a base revision. Any later change to a covered file makes the result stale.

klaudiush never trusts what the agent says about a check, and never reads exit status out of command output. A result counts only when the provider reports how the command ended (Claude), or when klaudiush ran the check itself (`klaudiush evidence run`).

The gate shares the completion gate's limit: after 3 consecutive blocks in one turn the agent is allowed to stop, and the user is told the check is still missing.

## Configuration

```toml
[evidence]
enabled = true

[[evidence.checks]]
name = "tests"
commands = ["mise run test", "go test ./..."]
paths = ["**/*.go", "go.mod", "go.sum", ".mise.toml"]
exclude = ["docs/**"]
timeout = "20m"

[[evidence.checks]]
name = "review"
kind = "review"
commands = ["./scripts/review.sh"]
paths = ["**/*.go"]
base = "origin/main"
```

| Field | Meaning | Default |
|:--|:--|:--|
| `name` | Name used in messages and `klaudiush evidence run <name>` | required |
| `kind` | `test` (tied to file content) or `review` (tied to the exact diff against `base`) | `test` |
| `commands` | Exact commands that run the check. The first is what the verifier runs | required |
| `paths` | Glob patterns, relative to the repository root, of the files the check covers. klaudiush's own `.klaudiush/` directory is never covered | every file |
| `exclude` | Glob patterns of files the check ignores | none |
| `base` | Branch a review diffs against, such as `origin/main`. The diff starts at the merge base of `base` and `HEAD` | required for reviews |
| `timeout` | Longest a verifier run may take, including fingerprinting the files before it, and how long a run without a result counts as running | `30m` |

Commands must be plain literal words: no pipes, `;`, `||`, `&&`, variables, substitutions, globs or quotes that hide expansions. klaudiush rejects other commands when it loads the configuration. Changing a check's `commands`, `paths`, `exclude`, `kind` or `base` invalidates its earlier results.

Turn the gate on or off for one shell with `KLAUDIUSH_EVIDENCE_ENABLED=true` or `false`.

## When a check is required

On the first hook of a session in a repository, klaudiush records a digest of the files each check covers. At the completion gate it compares the current digest with that baseline:

- Nothing covered changed (read-only work, or only files the check does not cover): the check is not required and the gate does not block.
- A covered file changed, was added or was removed: the check is required.
- An edit that was later undone: not required, because the content is back to the baseline.
- Only changes made while the session used a tool that can change files there count. A session that only read files (Read, Grep, Glob) is never gated by edits someone else made in the meantime. Shell commands count as tools that can change files.

Every repository the session records a baseline for is judged at the completion gate, not only the one the agent stops in. A repository gets a baseline when a hook runs in it, or when a file tool (Write, Edit, a patch) edits a file in it; each repository is judged with the checks its own configuration defines, even when the agent stops in a directory without the gate. A repository the session edited whose configuration cannot be loaded or whose checks do not compile is not treated as ungated: the completion gate reports it as [HOOK001](errors/HOOK001.md), which blocks unless `[failure_policy]` says otherwise. Shell commands that edit files in a repository no hook ran in are not seen.

If a check's definition changes during a session, the check is required, since its new baseline would include whatever the session changed before.

If the first hook klaudiush sees in a session comes after a tool already ran (for example because only `PostToolUse` is registered), klaudiush cannot tell what the session changed, so every check is required for that session.

Baselines live in the session state and are dropped when the session ends. Results are kept per repository, so a check that passed on exactly this content counts in every session.

## What counts as a result

| Status | Meaning | Satisfies the gate |
|:--|:--|:--|
| `passed` | The check succeeded and covered files did not change while it ran. The latest pass is kept when a later run never finishes; a failure on the same content drops it | Only if the digest matches the current files |
| `failed` | The check exited non-zero | No |
| `running` | The check started and has not reported yet | No |
| `canceled` | The check was interrupted or denied, timed out, or the verifier died. A Claude shell run the stopping agent started but never finished counts as canceled | No |
| `stale` | The result is for content that has changed since, or the files changed while the check ran | No |
| `unverified` | The check ran but klaudiush cannot know how it ended (background run, fingerprint failure) | No |
| `missing` | The check has not run on this definition | No |

The latest result of each check is what `klaudiush evidence status` shows as `latest`, and starting a new run replaces it. When that latest result is `running`, `canceled`, `stale` or `unverified`, an earlier pass on exactly the current content still satisfies the gate (`status` shows it as `kept`). Only a later failure on the same content invalidates that pass.

### Claude shell runs

Claude fires `PostToolUse` only after a command succeeded and `PostToolUseFailure` after it failed or was interrupted. For a few commands (`grep`, `diff`, `test`, `git diff`) Claude treats exit status 1 as success and says so in `returnCodeInterpretation`; klaudiush counts those as failed. klaudiush records a run when the `PreToolUse` it allowed is one of the check's commands, word for word, run from the repository root (optionally after `cd <dir> &&` steps that end there). Each `cd` target must be absolute or start with `./` or `../` (or be `.` or `..`): the shell looks any other relative target up in `CDPATH`, which klaudiush cannot see. It fingerprints the files when the command starts and again when Claude reports the outcome. Anything chained, piped, backgrounded or prefixed with variables does not count, because its exit status may not be the check's.

A command started with `run_in_background` never counts: Claude does not report a background command's exit status to hooks. Run it in the foreground, or use the verifier.

## Running checks with the verifier

```bash
klaudiush evidence run tests
```

The verifier runs the check's first command from the repository root, streams its output, records its exit status, and exits with it. It fingerprints the covered files before and after the run. The `timeout` covers the first fingerprint and the run; the fingerprint after the run gets its own one-minute limit, so it is still taken after a timeout or an interrupt. It is the only way to produce results in Codex and Gemini, and it works in Claude too.

The verifier can run in the background. Until it records a result the gate reports the check as running and blocks, so the agent has to wait for it; the completion gate's limit of 3 blocks a turn still applies. A verifier killed before it could record anything is reported as canceled.

## Review receipts

A `review` check identifies the exact reviewed diff: the merge base of `base` and `HEAD`, every covered file that differs from it (tracked changes and untracked files), and the content of each. The receipt records the base commit and the changed file list:

```text
review passed; command: ./scripts/review.sh; source: verifier; digest: sha256:...; base: 3f1c..., 2 changed file(s)
```

Any further edit to a changed file, a new covered file, or a different merge base (after a rebase, for example) makes the review stale. Committing the reviewed changes does not: the merge base and the resulting content stay the same.

## Provider coverage

| Provider | Completion gate | Shell runs of a check count | `klaudiush evidence run` counts |
|:--|:--|:--|:--|
| Claude | `Stop` | Yes, through `PostToolUse` and `PostToolUseFailure` | Yes |
| Codex | `Stop` | No: `PostToolUse` carries only model-facing output | Yes |
| Gemini | `AfterAgent` | No: `AfterTool` carries only `llmContent` and `returnDisplay` | Yes |
| opencode | none | Not gated | Not gated |

The gate message tells the agent which option works for its provider. `klaudiush evidence status` and `klaudiush doctor --category evidence` print the same table.

`klaudiush init` registers Codex `Stop` and Gemini `AfterAgent`, but not Claude `Stop`. Add it to your Claude settings for the gate to apply:

```json
{
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "klaudiush --provider claude --event Stop", "timeout": 30}]}
    ]
  }
}
```

`klaudiush doctor --category evidence` warns when a Claude settings file runs klaudiush but not on `Stop`.

## Gemini tool phases

A tool phase keeps Gemini from changing files until prerequisite checks passed on the current content, for example until a plan exists and was checked. It reuses the evidence checks and their results: the phase has no state of its own, so it opens as soon as every prerequisite has a passing result for the files as they are now, and closes again when a later change makes a result stale.

```toml
[evidence]
enabled = true

[[evidence.checks]]
name = "plan"
commands = ["test -s PLAN.md"]
paths = ["PLAN.md"]

[evidence.tool_phase]
enabled = true
requires = ["plan"]
writable_paths = ["PLAN.md"]
```

| Key | Default | Meaning |
|:--|:--|:--|
| `enabled` | `false` | Turns the phase on. Needs `evidence.enabled = true` |
| `requires` | none | Checks that must pass before mutation tools are offered. Required |
| `read_only_tools` | `ask_user`, `glob`, `google_web_search`, `grep_search`, `list_directory`, `read_file`, `read_many_files`, `web_fetch`, `write_todos` | Gemini tools offered while the phase is restricted. `write_file`, `replace` and `run_shell_command` are refused here |
| `writable_paths` | none | Globs, relative to the repository root, that `write_file` and `replace` may change while restricted |
| `filter_tools` | `true` | Answer `BeforeToolSelection`. Set `false` to rely on `BeforeTool` alone (see the limits below) |

Choose prerequisites that cover only what the phase produces, such as a plan file. A prerequisite covering the code under work closes the phase again on the first edit, and one whose fix needs a withheld tool (failing tests over `src/**`) can never pass.

`writable_paths` never opens, in any directory and in any letter case, `.klaudiush/`, `klaudiush.toml`, `.git/`, `.gemini/`, `.claude/`, `.codex/` or `.mcp.json`, nor the program a prerequisite command names or the script a shell or language interpreter is given directly (`./scripts/check.sh`, `sh /repo/scripts/check.sh`, `python3 -W ignore check.py`), compared after resolving symbolic links. When klaudiush cannot tell which operand is an interpreter's script (inline code, `-m`, standard input, an option it does not know), none of that command's operands is writable. Both the path as written and the file it resolves to must match. A file a symbolic link under one of those names leads to, and on Unix any file with a second hard link, is never writable either. A nested configuration would change what the verifier runs. Files a check only reads, such as the plan it tests, stay writable, and klaudiush cannot tell them apart from files a check runs indirectly (`env sh x.sh`, a `Makefile`, sourced scripts). Keep everything a prerequisite runs out of `writable_paths`.

While a prerequisite has no passing result:

- Gemini `BeforeToolSelection` answers with `toolConfig` mode `AUTO` and `allowedFunctionNames` set to the read-only tools, `run_shell_command`, and `write_file`/`replace` when `writable_paths` is set. The model can still answer without a tool.
- Gemini `BeforeTool` denies with [EVID002](errors/EVID002.md) every call outside that set, so a tool another hook offered, or one the model calls anyway, is still stopped. The shell runs only `klaudiush evidence run <prerequisite>` or `klaudiush evidence status` as one command of plain words, by absolute path or by name on `PATH`, with no `cd`, `dir_path` or redirections, so the verifier loads the same configuration as the hook. File tools may change only `writable_paths`, resolved through symbolic links.
- Every other validator still runs on the calls the phase allows.

Run `klaudiush evidence run plan` (the agent can, through the shell) to pass the prerequisite. The next model call is offered every tool again.

The phase is judged for the repository of Gemini's working directory. Outside a git repository it does not apply, even to edits in repositories below that directory. When git is missing or cannot read the repository (unreadable metadata, a repository git does not trust), or klaudiush cannot read check results or fingerprint a prerequisite's files, the [failure policy](FAILURE_POLICY_GUIDE.md) decides: by default the phase stays restricted, and the calls it allows still run, so the verifier opens it once it can record a result. A path through a dangling symbolic link, in any of its components, is never writable, since writing through it would create a file wherever it points.

The phase fingerprints its prerequisites on every Gemini model call and every tool call that is not read-only, so keep their `paths` small: a plan file, not the source tree.

### Registration

`BeforeToolSelection` runs before every model call, so `klaudiush init` and `klaudiush doctor --fix` register it in the Gemini settings only when the tool phase is enabled and `filter_tools` is on:

```json
{
  "hooks": {
    "BeforeToolSelection": [
      {"hooks": [{"type": "command", "command": "klaudiush --provider gemini --event BeforeToolSelection", "timeout": 30000}]}
    ]
  }
}
```

`klaudiush doctor --category evidence` reports an error, fixed by `klaudiush doctor --fix`, when the hook is missing while `filter_tools` is on, warns when the klaudiush `BeforeTool` matcher does not select `write_file`, `replace` and `run_shell_command`, and lists tools withheld only by the tool selection (MCP tools and `save_memory` under the default matcher).

### Coverage and limits

| Provider | Tools withheld | Calls checked against the phase |
|:--|:--|:--|
| Gemini | Yes, `BeforeToolSelection` | Yes, `BeforeTool` |
| Claude | No: no tool-selection event | No |
| Codex | No: no tool-selection event | No |
| opencode | No: no tool-selection event | No |

`klaudiush evidence status` prints whether the phase is restricted and the same coverage.

- Gemini merges the `allowedFunctionNames` of every `BeforeToolSelection` hook as a union. Another hook that lists `write_file` offers it again; `BeforeTool` still denies the call.
- Gemini keeps every tool declaration in the model request and passes the list as `functionCallingConfig.allowedFunctionNames` with mode `AUTO`; its hook aggregator turns any mode other than `ANY` or `NONE` into `AUTO`. The Gemini API documents `allowedFunctionNames` for modes `ANY` and `VALIDATED` only, so how strictly the model follows the list under `AUTO` is up to the API. Treat the selection as a hint that saves wasted calls; `BeforeTool` is the enforcement. If the model API rejects the request while the phase is restricted, set `filter_tools = false`.
- An invalid `[evidence.tool_phase]` does not stop the configuration from loading: Gemini is offered only the default read-only tools and every other call reports HOOK001, which blocks under the default failure policy. Invalid `[[evidence.checks]]` stop the whole configuration from loading.
- Tools that reach no klaudiush `BeforeTool` hook (MCP tools and `save_memory` under the default matcher) are withheld only by the selection. Widen the `BeforeTool` matcher to check them per call.
- When the `BeforeToolSelection` hook fails or times out, Gemini offers every tool; `BeforeTool` still enforces.

## What the gate cannot prevent

The gate stops an agent from claiming a check passed without one. It does not stop an agent determined to cheat with full access to your machine:

- The configuration, the check scripts, and the state file (`$XDG_STATE_HOME/klaudiush/hook_sessions/state.json`) are ordinary files the agent can edit. Cover check scripts and build files with `paths` so editing them invalidates results.
- A check passes if its command exits 0. A test suite with tests removed or skipped still passes.
- A program earlier on `PATH` with the same name as the check's command runs instead of it.
- The verifier runs the check with the caller's environment, so a variable the check honours (`GOFLAGS`, a skip switch) changes what it runs.
- A requirement belongs to the session that made the change. Once the completion gate's block limit lets a session stop, a new session starts from the changed files as its baseline. Ignored files are not fingerprinted, so include `**/.gitignore` in `paths`; changes to `.git/info/exclude` already make results stale.

## Inspecting results

```bash
klaudiush evidence status
```

```text
Evidence gate: enabled
Repository: /home/me/project
Coverage:
  claude: gated; shell runs of a check and 'klaudiush evidence run' both count
  codex: gated; only 'klaudiush evidence run' counts, hooks do not learn shell exit status
  gemini: gated; only 'klaudiush evidence run' counts, hooks do not learn shell exit status
  opencode: not gated, it has no completion event klaudiush can block
Tool phase: disabled

tests (test): mise run test
  current: sha256:... (42 file(s))
  verdict: stale, its passed result is for content that has changed since
  latest:  tests passed; command: mise run test; source: claude; digest: sha256:...; 42 file(s)
```

## Troubleshooting

**The gate blocks after I ran the tests.** The command must be exactly one of `commands`, run on its own from the repository root, in the foreground. Otherwise run `klaudiush evidence run <name>`.

**The gate never blocks.** Check that `[evidence] enabled = true`, that the project is a git repository, that the changed files match `paths`, and that your provider runs klaudiush on its completion event (`klaudiush doctor --category evidence`).

**"klaudiush could not fingerprint ..."** klaudiush could not list or read the covered files, or resolve the merge base of a review's `base` and `HEAD`. A directory git cannot read counts too, since files in it would be left out. A session that only read files is not gated by this. This is reported as [HOOK001](errors/HOOK001.md) and blocks by default, because unknown content must not count as checked; `failure_policy.mode = "warn"` turns it into a warning. Session state klaudiush cannot read warns unless `mode = "block"` or `critical` includes `evidence`.
