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
| `paths` | Glob patterns, relative to the repository root, of the files the check covers | every file |
| `exclude` | Glob patterns of files the check ignores | none |
| `base` | Branch a review diffs against, such as `origin/main`. The diff starts at the merge base of `base` and `HEAD` | required for reviews |
| `timeout` | Longest a verifier run may take, and how long a run without a result counts as running | `30m` |

Commands must be plain literal words: no pipes, `;`, `||`, `&&`, variables, substitutions, globs or quotes that hide expansions. klaudiush rejects other commands when it loads the configuration. Changing a check's `commands`, `paths`, `exclude`, `kind` or `base` invalidates its earlier results.

Turn the gate on or off for one shell with `KLAUDIUSH_EVIDENCE_ENABLED=true` or `false`.

## When a check is required

On the first hook of a session in a repository, klaudiush records a digest of the files each check covers. At the completion gate it compares the current digest with that baseline:

- Nothing covered changed (read-only work, or only files the check does not cover): the check is not required and the gate does not block.
- A covered file changed, was added or was removed: the check is required.
- An edit that was later undone: not required, because the content is back to the baseline.
- Only changes made while the session used a tool that can change files there count. A session that only read files (Read, Grep, Glob) is never gated by edits someone else made in the meantime. Shell commands count as tools that can change files.

Every repository the session records a baseline for is judged at the completion gate, not only the one the agent stops in. A repository gets a baseline when a hook runs in it, or when a file tool (Write, Edit, a patch) edits a file in it; each repository is judged with the checks its own configuration defines. Shell commands that edit files in a repository no hook ran in are not seen.

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

Only the latest result of each check counts. Starting a new run replaces the previous result until it finishes.

### Claude shell runs

Claude fires `PostToolUse` only after a command succeeded and `PostToolUseFailure` after it failed or was interrupted. For a few commands (`grep`, `diff`, `test`, `git diff`) Claude treats exit status 1 as success and says so in `returnCodeInterpretation`; klaudiush counts those as failed. klaudiush records a run when the `PreToolUse` it allowed is one of the check's commands, word for word, run from the repository root (optionally after `cd <dir> &&` steps that end there). It fingerprints the files when the command starts and again when Claude reports the outcome. Anything chained, piped, backgrounded or prefixed with variables does not count, because its exit status may not be the check's.

A command started with `run_in_background` never counts: Claude does not report a background command's exit status to hooks. Run it in the foreground, or use the verifier.

## Running checks with the verifier

```bash
klaudiush evidence run tests
```

The verifier runs the check's first command from the repository root, streams its output, records its exit status, and exits with it. It fingerprints the covered files before and after the run. It is the only way to produce results in Codex and Gemini, and it works in Claude too.

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

## What the gate cannot prevent

The gate stops an agent from claiming a check passed without one. It does not stop an agent determined to cheat with full access to your machine:

- The configuration, the check scripts, and the state file (`$XDG_STATE_HOME/klaudiush/hook_sessions/state.json`) are ordinary files the agent can edit. Cover check scripts and build files with `paths` so editing them invalidates results.
- A check passes if its command exits 0. A test suite with tests removed or skipped still passes.
- A program earlier on `PATH` with the same name as the check's command runs instead of it.
- The verifier runs the check with the caller's environment, so a variable the check honours (`GOFLAGS`, a skip switch) changes what it runs.
- A requirement belongs to the session that made the change. Once the completion gate's block limit lets a session stop, a new session starts from the changed files as its baseline. A `.git/info/exclude` entry hides a new file from the fingerprint.

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

tests (test): mise run test
  current: sha256:... (42 file(s))
  verdict: stale, its passed result is for content that has changed since
  latest:  tests passed; command: mise run test; source: claude; digest: sha256:...; 42 file(s)
```

## Troubleshooting

**The gate blocks after I ran the tests.** The command must be exactly one of `commands`, run on its own from the repository root, in the foreground. Otherwise run `klaudiush evidence run <name>`.

**The gate never blocks.** Check that `[evidence] enabled = true`, that the project is a git repository, that the changed files match `paths`, and that your provider runs klaudiush on its completion event (`klaudiush doctor --category evidence`).

**"klaudiush could not fingerprint ..."** klaudiush could not list or read the covered files, or resolve the merge base of a review's `base` and `HEAD`. A directory git cannot read counts too, since files in it would be left out. This is reported as [HOOK001](errors/HOOK001.md) and blocks by default, because unknown content must not count as checked; `failure_policy.mode = "warn"` turns it into a warning. Session state klaudiush cannot read warns unless `mode = "block"` or `critical` includes `evidence`.
