# SHELL002: Command cannot be inspected

## Error

Klaudiush cannot see what the command finally runs. The command does not parse as shell, runs another command through more layers of launchers, scripts, aliases or functions than klaudiush follows, runs a script klaudiush cannot read, runs a git subcommand that is neither built in, installed, nor an alias klaudiush can see, or takes eval's command line or a git or gh command word from a variable or command output klaudiush cannot resolve.

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
| git or gh word from a variable        | `git $SUB`, `gh pr $ACTION`                     | Write it literally, or assign it literally on the line    |
| git or gh word from command output    | `git $(echo commit)`, `git c?mmit`              | Write the subcommand literally                            |
| eval of a variable or command output  | `eval "$LINE"`, `eval "$(tool init)"`           | Run the commands directly instead of through eval         |

A variable assigned a literal value earlier on the same line, or set in the environment klaudiush runs in, is resolved: `X=status; git $X` is checked as `git status`. A subcommand that is not a valid git command name, such as `'push '` or `$'push\n'`, is checked as the builtin git autocorrect would run, or blocked as an unknown subcommand.

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
