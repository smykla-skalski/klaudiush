# Metrics guide

See what klaudiush hooks actually enforced, what they only advised, how agents repaired violations, which checks could not run, and how long hooks took.

## Table of contents

- [Overview](#overview)
- [Configuration](#configuration)
- [Outcomes](#outcomes)
- [Repairs and retries](#repairs-and-retries)
- [Unavailable checks](#unavailable-checks)
- [Latency](#latency)
- [Privacy and storage](#privacy-and-storage)
- [Commands](#commands)
- [Relation to failure patterns and the audit log](#relation-to-failure-patterns-and-the-audit-log)

## Overview

Every hook appends one line to a local log after it writes its response. The line records what that response did, the error codes it reported, which checks could not run, the session and resource keys needed to follow a violation across hooks, and how long the hook and each validator took. `klaudiush metrics` turns the log into a report.

Recording is on by default and never leaves the machine. Nothing in a hook depends on it: the line is written after the response, a sample whose lock cannot be taken within 250ms is dropped, and any other write error only loses that sample. `klaudiush doctor --category metrics` tells you when samples cannot be written.

## Configuration

```toml
[metrics]
enabled = true          # default
retention = "720h"      # default report window and prune age (30 days)
max_file_size_mb = 8    # active log size before it rotates (at most 256)
```

`KLAUDIUSH_METRICS_ENABLED=false` turns recording off for one shell, and is honored even when no configuration file can be read. `enabled = false` in a file that does not parse is honored too. The report still reads what was recorded earlier.

The log is shared by every project, but `retention` and `max_file_size_mb` come from the effective configuration of the hook or command that uses them. Set them in the global configuration; a smaller project value makes hooks in that project rotate the shared log sooner.

## Outcomes

Each hook gets the strongest outcome of its findings. The class comes from the response klaudiush wrote, not from the finding's wording, so the report matches what the provider was asked to do.

| Outcome     | Meaning                                                                                                                                                                  |
|:------------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| prevented   | The response stopped the action before it ran: a denied tool call or approval, a blocked prompt, a declined elicitation, or `continue: false`.                             |
| held        | A completion gate (Claude and Codex `Stop`/`SubagentStop`, Gemini `AfterAgent`) kept the agent working.                                                                   |
| released    | A completion gate let the turn end over unresolved findings after its continuation limit.                                                                                |
| advisory    | A blocking finding the response could not stop: every finding after a tool ran, findings on events the provider cannot block, and Gemini tool selections that withheld tools. |
| unavailable | A check could not run and the failure policy warned or ignored it.                                                                                                       |
| excepted    | An exception token turned a block into a warning.                                                                                                                         |
| warned      | Only non-blocking findings.                                                                                                                                               |
| passed      | No findings.                                                                                                                                                             |
| skipped     | The bypass policy skipped validation (`bypass_permissions.skip_validation`).                                                                                              |

**Enforced** is prevented plus held. After a tool ran the action already happened, so findings there are advisory even when the provider received `decision: "block"` (Claude and Codex `PostToolUse`). They never count as prevented.

A check that could not run and blocked under `failure_policy` counts as prevented, and also appears under unavailable checks with `blocked` set.

## Repairs and retries

A violation is a finding that asks the agent to change something: a blocking finding before the tool, or any unwaived finding after it. It is identified by session, validator, resource and code; an error with several structured findings counts each distinct code as its own violation. For commands the resource is "a command", so a later command the same validator passes clears it; for files it is the file. A file a shell command writes is checked under that file, so rewriting it through the shell is not counted as a retry.

Only a check of the whole resource repairs a file violation: the file as the tool left it, or the complete content a Write proposes. A check of an edit fragment, of a shell write before it runs, or a run that skipped the file (linter disabled, content not readable, file too large) repairs nothing. A completion gate repairs only what it checked: `Stop` the whole session, `SubagentStop` that subagent's violations.

| Field               | Meaning                                                                                                                   |
|:--------------------|:--------------------------------------------------------------------------------------------------------------------------|
| violations          | Distinct violations reported.                                                                                             |
| repaired            | The validator checked the same resource again without reporting the code, or a completion gate that had held it passed.    |
| repaired_first_try  | Repaired after a single report.                                                                                           |
| retries             | Reports of an open violation by a hook that acted on its resource (the same command validator, or the same file) or tried to complete the turn. Rechecks of an unrepaired file during unrelated tool calls are not retries. |
| recurring           | Violations reported more than once.                                                                                       |
| closed_by_exception | Open violations an accepted exception waived.                                                                              |
| unresolved          | Still open at the end of the window.                                                                                      |
| uncorrelated        | Violations reported by a hook without a session ID, which cannot be followed.                                              |

The per-code table adds the exception rate: the share of a code's reports that an exception waived. A high rate is the closest local signal that a rule produces false positives.

## Unavailable checks

Checks that could not run are counted by reason (`missing_tool`, `timeout`, `config`, `state`, `malformed_input`, `panic`, ...) and validator, with how many of them blocked. Checks the failure policy ignores, such as a missing linter under the default `missing_tools = "ignore"`, are counted too, although the response says nothing about them. See the [failure policy guide](FAILURE_POLICY_GUIDE.md) for what each reason means and when it blocks.

## Latency

Hook latency runs from the start of the hook pipeline (after flag parsing and first-run migration) until the response is written and session cleanup ran, per provider and event (p50, p95, max). Validator latency is each validator's run time, summed when it ran more than once in one hook (for example on several files of a patch). Writing the metrics line itself is not included. It usually takes well under a millisecond, but it holds the process open while it waits for the lock (at most 250ms, for example during `klaudiush metrics prune`).

## Privacy and storage

- The log is `$XDG_STATE_HOME/klaudiush/metrics/outcomes.jsonl` (mode 0600, directory 0700).
- No command, message, file path, content or session ID is stored. Sessions and resources are 64-bit keys from HMAC-SHA256 with a random salt kept next to the log, so they can be compared but not reversed or linked to another machine. Validator names, codes and event names are reduced to at most 64 characters of `[A-Za-z0-9._:-]`.
- A line lists at most 32 findings, 64 checks and 64 validator timings. The outcome is still taken over every finding; a line with more findings is marked truncated and never repairs anything.
- When the active log reaches `max_file_size_mb` it becomes `outcomes.jsonl.1`, replacing the previous backup, so at most twice the cap is kept.
- Writers hold `outcomes.jsonl.lock`, so concurrent hooks never interleave lines or lose a rotation. Lines that cannot be read (an interrupted write) are skipped and counted in the report.
- `klaudiush metrics prune` drops records older than `retention`; `klaudiush metrics clear` removes the logs and the salt, so new keys cannot be linked to old ones.

## Commands

```bash
klaudiush metrics                          # report for the retention window
klaudiush metrics report --since 7d        # last 7 days (Go durations work too)
klaudiush metrics report --provider codex  # one provider
klaudiush metrics report --event Stop      # one native event
klaudiush metrics report --json            # machine-readable
klaudiush metrics prune                    # drop records older than retention
klaudiush metrics clear                    # remove all metrics and the salt
klaudiush doctor --category metrics        # can hooks record?
```

## Relation to failure patterns and the audit log

- Failure patterns (`[patterns]`, `klaudiush patterns`) learn which error tends to follow another and warn the agent. They are unchanged.
- The exception audit log (`klaudiush audit`) keeps every exception request with its reason. Metrics count accepted exceptions per code without the reason.
