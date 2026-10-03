# EVID001: Required check has no fresh passing result

## Error

The session changed files a required check covers, and the check has no passing result for the files as they are now. The message names the check and why its latest result does not count:

| Status | Meaning |
|:--|:--|
| missing | The check has not run since the files changed, or its definition changed since it ran |
| failed | The check exited non-zero |
| running | The check started and has not reported a result yet |
| canceled | The check was interrupted, timed out, or the verifier stopped without reporting |
| stale | The check ran on content that has changed since, or the files changed while it ran |
| unverified | The check ran in a way klaudiush cannot verify, such as a background shell command |

## Why this matters

Saying tests passed is not evidence that they passed on the code being handed back. klaudiush ties each result to a digest of the files the check covers, so only a run against the current content counts.

## How to fix

Run the check the message names, then finish again:

- In Claude, run one of the check's `commands` exactly, on its own, from the repository root, in the foreground.
- In any provider, run `klaudiush evidence run <check>`. klaudiush runs the check and records its exit status itself.

If the check fails, fix what it reports first. `klaudiush evidence status` shows each check's latest result and the digest it covers.

## Configuration

```toml
[evidence]
enabled = true

[[evidence.checks]]
name = "tests"
commands = ["mise run test"]
paths = ["**/*.go", "go.mod", "go.sum"]
```

After 3 consecutive blocks in one turn the agent is allowed to stop and you are told the check is still missing.

## Related

- [Evidence guide](../EVIDENCE_GUIDE.md)
- [HOOK001](HOOK001.md) when klaudiush cannot fingerprint the files
