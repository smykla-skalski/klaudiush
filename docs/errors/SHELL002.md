# SHELL002: Command cannot be inspected

## Error

Klaudiush cannot see what the command finally runs. The command does not parse as shell, runs another command through more layers of launchers, scripts, aliases or functions than klaudiush follows, runs a script klaudiush cannot read, or runs a git subcommand that is neither built in, installed, nor an alias klaudiush can see.

## Why this matters

Klaudiush follows `env`, `sudo`, `nice`, `bash -c`, `eval`, script files, aliases and functions to find the command that really runs, and checks that command like any other. It stops after eight levels. A command nested deeper, or one it cannot parse at all, could hide a `git commit` or `git push` from every validator, so klaudiush blocks it instead of letting it through unchecked.

```bash
env env env env env env env env env git commit -m "message"
```

## What the message says

Each finding names the operation klaudiush could not see through, the programs that led to it (`via sudo > bash`), and a repair. Findings name programs, scripts and subcommands only, never their arguments.

| Cause                                 | Example                                         | Repair                                                    |
|:--------------------------------------|:------------------------------------------------|:----------------------------------------------------------|
| Command does not parse                | `git commit -m "x" && (`                        | Fix the syntax at the reported line and column            |
| Nesting past eight levels             | nine `env` wrappers around `git commit`         | Run the inner command directly                            |
| Inspection budget spent               | a function fanning out to thousands of calls    | Split the work, call programs directly                    |
| Script path from a variable           | `bash "$DIR/run.sh"`                            | Use a literal script path                                 |
| Relative script after an unknown `cd` | `cd "$DIR" && bash run.sh`                      | Use an absolute path or a literal `cd`                    |
| Script written with unknown content   | `echo "$BODY" > s.sh && bash s.sh`              | Write literal content, or write it in a separate command  |
| Script that cannot be read in full    | a script over 256 KiB, or unreadable            | Run its commands directly, or keep it small and readable  |
| Nested script that does not parse     | `bash -c 'git status && ('`                     | Fix the nested script's syntax                            |
| Unknown git subcommand                | `HOME=/x git cm`                                | Use the builtin, or define the alias in git config first  |
| Function arguments it cannot follow   | `f() { git "${@:1}"; }; f commit`               | Forward arguments with plain `"$@"`                       |

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
