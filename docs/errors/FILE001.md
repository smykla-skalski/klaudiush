# FILE001: Shellcheck Validation Failed

## Error

Shell script failed shellcheck static analysis.

## Why This Matters

Shellcheck detects common shell scripting bugs, portability issues, and style problems that can cause:

- Unexpected behavior in different shells
- Security vulnerabilities (injection, globbing)
- Quoting errors that break with special characters

## How to Fix

1. Run shellcheck directly to see detailed errors:

   ```bash
   shellcheck your-script.sh
   ```

2. Fix each issue as indicated by the SC code

3. For legitimate cases, add inline ignores:

   ```bash
   # shellcheck disable=SC2086
   command $unquoted_var
   ```

## Common Issues

| Code   | Issue                | Fix                             |
|:-------|:---------------------|:--------------------------------|
| SC2086 | Unquoted variable    | Use `"$var"` instead of `$var`  |
| SC2046 | Unquoted command sub | Use `"$(cmd)"` instead          |
| SC2006 | Backticks deprecated | Use `$(cmd)` instead of \`cmd\` |
| SC2155 | Declare + assign     | Split declaration and assign    |
| SC2034 | Unused variable      | Remove or use the variable      |

## Configuration

Adjust timeout in `config.toml`:

```toml
[validators.file.shellscript]
timeout = "15s"
context_lines = 2
```

## Skipped Scripts

Fish shell scripts (`.fish` extension or fish shebang) are automatically skipped since shellcheck only supports POSIX-like shells.

## Related

- [shellcheck wiki](https://www.shellcheck.net/wiki/)
