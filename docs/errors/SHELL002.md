# SHELL002: Command cannot be inspected

## Error

Klaudiush cannot see what the command finally runs. Either the command does not parse as shell, or it runs another command through more layers of launchers, scripts, aliases or functions than klaudiush follows.

## Why this matters

Klaudiush follows `env`, `sudo`, `nice`, `bash -c`, `eval`, script files, aliases and functions to find the command that really runs, and checks that command like any other. It stops after eight levels. A command nested deeper, or one it cannot parse at all, could hide a `git commit` or `git push` from every validator, so klaudiush blocks it instead of letting it through unchecked.

```bash
env env env env env env env env env git commit -m "message"
```

## How to fix

Fix the shell syntax if the command does not parse. Otherwise run the inner command directly:

```bash
git commit -sS -m "message"
```

If a wrapper is really needed, use one layer of it:

```bash
env FOO=1 git commit -sS -m "message"
```

## Disabling

Legitimate commands almost never nest this deep. If one does, disable the check:

```bash
klaudiush disable SHELL002
```
