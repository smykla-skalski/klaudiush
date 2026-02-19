# FILE001: Shellcheck validation failed

## Error

Shell script failed shellcheck static analysis.

## Why this matters

Shellcheck catches shell scripting bugs, portability issues, and style problems:

- Broken behavior across different shells
- Security holes (injection, globbing)
- Quoting errors that break on special characters

## How to fix

1. Run shellcheck directly to see the errors:

   ```bash
   shellcheck your-script.sh
   ```

2. Fix each issue as indicated by the SC code

3. To suppress a finding, add an inline ignore:

   ```bash
   # shellcheck disable=SC2086
   command $unquoted_var
   ```

## Common issues

| Code   | Issue                | Fix                             |
|:-------|:---------------------|:--------------------------------|
| SC2086 | Unquoted variable    | Use `"$var"` instead of `$var`  |
| SC2046 | Unquoted command sub | Use `"$(cmd)"` instead          |
| SC2006 | Backticks deprecated | Use `$(cmd)` instead of \`cmd\` |
| SC2155 | Declare + assign     | Split declaration and assign    |
| SC2034 | Unused variable      | Remove or use the variable      |

## Configuration

Timeout is configurable in `config.toml`:

```toml
[validators.file.shellscript]
timeout = "15s"
context_lines = 2
```

## Skipped scripts

Fish shell scripts (`.fish` extension or fish shebang) are skipped because shellcheck only supports POSIX-like shells.

## Related

- [shellcheck wiki](https://www.shellcheck.net/wiki/)

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[FILE001] Shell script failed shellcheck static analysis. Run 'shellcheck <file>' to see detailed errors.`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`
