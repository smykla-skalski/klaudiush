# HOOK001: Validation unavailable

## Error

A check, or klaudiush as a whole, could not run, so the action was not validated. The message names the check and the reason:

| Reason | Meaning |
|:--|:--|
| required tool not installed | A linter the check needs is not on `PATH` |
| timed out | The check, or the whole hook, ran past its deadline |
| canceled | The check was stopped before it finished |
| crashed | The check, or klaudiush, panicked |
| unreadable output | A tool or plugin answered with output klaudiush cannot read |
| unreadable hook input | The provider sent input that is not valid JSON |
| configuration error | The configuration cannot be loaded or is invalid |
| session state unavailable | Findings from earlier tool calls could not be read or saved |
| failed to run | Anything else, such as a plugin exiting non-zero |

## Why this matters

A check that could not run is not a pass. Whether the action goes ahead with a warning or is denied depends on the [failure policy](../FAILURE_POLICY_GUIDE.md). By default only plugin failures block; `failure_policy.mode = "block"` or a `critical` validator makes the others block too.

## How to fix

Timeouts and held locks are often transient, so the next action may pass. If it keeps happening, run:

```bash
klaudiush doctor
```

Then fix the cause the message names:

- Install the missing tool, or remove the validator from `failure_policy.critical`.
- Fix the configuration file `klaudiush doctor --category config` reports.
- Raise the tool timeout, or `failure_policy.deadline` together with the hook timeout.
- Fix or disable the failing plugin.

## Configuration

```toml
[failure_policy]
mode = "warn"            # let actions through with a warning while you fix the cause
missing_tools = "ignore"
```

## Related

- [Failure policy guide](../FAILURE_POLICY_GUIDE.md)
- [Exceptions guide](../EXCEPTIONS_GUIDE.md) to waive a blocked HOOK001 once
