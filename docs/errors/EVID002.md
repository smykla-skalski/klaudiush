# EVID002: Tool withheld by the evidence tool phase

## Error

Gemini called a tool that the evidence tool phase withholds: a prerequisite check has no passing result for the files as they are now. The message names each unmet prerequisite and why its latest result does not count, using the statuses of [EVID001](EVID001.md).

## Why this matters

A tool phase keeps an agent from changing files before the work it depends on exists, such as a checked plan. Gemini `BeforeToolSelection` already stops offering the withheld tools, but another hook can offer them again and the model can still call a tool it saw earlier, so every call is checked as well.

## How to fix

Run the verifier for each prerequisite through the shell, as its own command:

```bash
klaudiush evidence run plan
```

While the phase is restricted the shell accepts only this command (or `klaudiush evidence status`), run from the session's directory without `cd` or `dir_path`, and `write_file` and `replace` may change only the paths in `writable_paths`, never configuration or the scripts a prerequisite runs. Once every prerequisite passed, Gemini is offered every tool again on the next model call. Changing the files a prerequisite covers withholds the tools again until it passes on the new content.

If the check fails, fix what it reports first. `klaudiush evidence status` shows whether the phase is restricted and each check's latest result.

## Configuration

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

## Related

- [Gemini tool phases](../EVIDENCE_GUIDE.md#gemini-tool-phases)
- [EVID001](EVID001.md) when a required check is missing at the completion gate
- [HOOK001](HOOK001.md) when klaudiush cannot read results or fingerprint the files
