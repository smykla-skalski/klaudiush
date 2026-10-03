# SHELL002: Command cannot be inspected

## Error

Klaudiush cannot see what the command finally runs. The command does not parse as bash, runs another command through more layers of launchers, scripts, aliases or functions than klaudiush follows, runs a script klaudiush cannot read, runs a git subcommand that is neither built in, installed, nor an alias klaudiush can see, or takes eval's command line or a git or gh command word from a variable or command output klaudiush cannot resolve.

## Why this matters

Klaudiush follows `env`, `sudo`, `nice`, `bash -c`, `eval`, script files, aliases and functions to find the command that really runs, and checks that command like any other. It stops after eight levels. A command nested deeper, or one it cannot parse at all, could hide a `git commit` or `git push` from every validator, so klaudiush blocks it instead of letting it through unchecked.

```bash
env env env env env env env env env git commit -m "message"
```

A script that names itself, such as a Python helper whose usage text shows `python3 helper.py --dry-run` or a shell script that reruns itself, does not count as nesting. Klaudiush reads the script once and checks the commands it finds in it, including any `git` or `gh` call. It follows the script into itself again whenever something has changed since the last pass: the directory, the variables, aliases or functions in scope, the command table (`hash -p`, `enable`), or the commands recorded so far. A script that `cd`s elsewhere and reruns itself is still checked in the new directory.

## What the message says

Each finding names the operation klaudiush could not see through, the programs that led to it (`via sudo > bash`), and a repair. Findings name programs, scripts and subcommands only, never their arguments.

| Cause                                 | Example                                         | Repair                                                    |
|:--------------------------------------|:------------------------------------------------|:----------------------------------------------------------|
| Command does not parse as bash        | `git commit -m "x" && (`                        | Fix the syntax at the reported line and column            |
| zsh syntax bash does not parse        | `for x in ${(s:,:)list}; do echo $x; done`      | Rewrite it in bash syntax                                 |
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

A variable assigned a literal value earlier on the same line, or set in the environment klaudiush runs in, is resolved: `X=status; git $X` is checked as `git status`. Only a plain assignment statement counts. A variable also assigned in a subshell, pipeline, condition, loop, function or background job, or as a command prefix, or set by `read`, `printf -v`, `mapfile`, `getopts`, a `for` loop, `eval` or a sourced script, is treated as unknown, and so is every variable used inside a loop. After a write to a name klaudiush cannot read (`declare "$v"`, `printf -v "$v"`) a `declare -l`, `-u` or `-n`, a `source`, a `mapfile -C` callback, or a program named by a variable or command output, no variable is resolved. Inside a new shell (`bash -c`, a script file), which sees only exported variables and may source `BASH_ENV` first, no variable is resolved either. A subcommand that is not a valid git command name, such as `'push '` or `$'push\n'`, is checked as the builtin git autocorrect would run, or blocked as an unknown subcommand.

Klaudiush parses every command as bash, even when the login shell is zsh. A command bash cannot parse but the zsh grammar accepts (parameter expansion flags such as `${(s:,:)var}`, `${=var}` splitting, glob qualifiers such as `*.go(N)`, `=(...)` process substitutions, anonymous functions, `foreach`, `&|`) is blocked as zsh syntax, and the message names the construct when klaudiush recognizes it. A command that is broken even as zsh is reported at the position where the zsh grammar stops. Constructs like these can run code klaudiush cannot follow, such as the `(e)` flag re-evaluating a variable's value. Short `for x (a b) cmd` loops and `for x in a; { cmd }` loops are not known to the zsh grammar klaudiush uses, so a command with one is reported as not parsing as bash and using that zsh syntax, together with the bash error position in case the command is also broken. Other zsh forms that grammar does not know, such as `if [[ -n $x ]] { cmd }`, `{ cmd } always { cmd }` or `for x in a b; cmd`, get the plain message that the command does not parse as bash, not zsh. The zsh grammar also accepts some input zsh itself rejects, such as an unknown flag in `${(Y)x}`, so every repair says to fix the syntax at the reported position if it is broken. Inline scripts (`bash -c`, `zsh -c`) and script files are parsed as bash too.

## How to fix

Fix the shell syntax if the command does not parse, or rewrite zsh-only syntax in bash. Otherwise run the inner command directly:

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
