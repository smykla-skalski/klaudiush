# SHELL002: Command cannot be inspected

## Error

Klaudiush cannot see what the command finally runs. The command does not parse as bash, runs another command through more layers of launchers, scripts, aliases or functions than klaudiush follows, runs a script klaudiush cannot read, runs a git subcommand that is neither built in, installed, nor an alias klaudiush can see, takes eval's command line, a git or gh command word, or a container `--entrypoint` from a variable or command output klaudiush cannot resolve, or starts a shell whose startup file (`BASH_ENV`, `ENV`, `--rcfile`) klaudiush cannot read.

## Why this matters

Klaudiush follows `env`, `sudo`, `nice`, `bash -c`, `eval`, script files, aliases and functions to find the command that really runs, and checks that command like any other. It stops after eight levels. A command nested deeper, or one it cannot parse at all, could hide a `git commit` or `git push` from every validator, so klaudiush blocks it instead of letting it through unchecked.

```bash
env env env env env env env env env git commit -m "message"
```

A script that names itself, such as a Python helper whose usage text shows `python3 helper.py --dry-run` or a shell script that reruns itself, does not count as nesting. Klaudiush reads the script once and checks the commands it finds in it, including any `git` or `gh` call. It does not follow the script into itself again while the directory, the variables, aliases or functions in scope, the command table (`hash -p`, `enable`) and the files written so far are all unchanged; any change makes it follow the script again, so a script that `cd`s elsewhere and reruns itself is still checked in the new directory. Once the outer pass is done, klaudiush walks the script once more with everything recorded so far. If that pass records a new command, writes new content, redefines a git or gh alias or changes the command table, or the line writes some file more than one way or runs more than one different `git config` or `gh alias` command, the passes have not settled and the command is blocked as nesting too deeply.

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
| eval of a known tool's shell setup    | `eval "$(mise activate bash)"`                  | Run the command through the tool (see below)              |
| Container entrypoint not literal      | `docker run --entrypoint "$EP" img push`        | Write the entrypoint, options and image literally         |
| Startup file it cannot read           | `BASH_ENV=$(mktemp) bash -c true`               | Assign a literal path of a readable file, or empty        |
| zsh glob qualifier that runs code     | `ls *(e:'git push':)`, `ls *(+fn)`              | Select the files another way and run the command directly |
| Command substitution in an extglob    | `ls *(a$(git push))`                            | Run the command separately                                |
| Variable in an extglob, or `$~var`    | `ls *($q)`, `ls $~q`                            | Write the glob literally                                  |

When eval runs the output of one command substitution whose program is a literal name from the list below, with arguments that make it print shell setup, the finding names the tool and a form klaudiush can inspect. The block stays. A computed program name (`$TOOL`, `$(which mise)`) or argument, a name that only contains a known one (`evil-mise`), or a tool or `eval` redefined as an alias or function on the same line gets the generic repair. A literal path is named by its last part (`/opt/homebrew/bin/brew` is `brew`); this changes only the text.

| Tool                                                               | Instead of eval                                      |
|:-------------------------------------------------------------------|:-----------------------------------------------------|
| `ssh-agent [-s\|-c]`                                               | `ssh-agent <command>`, the command runs as its child |
| `mise activate`, `mise env`, `mise hook-env`                       | `mise exec -- <command>`                             |
| `direnv export`, `direnv hook`                                     | `direnv exec . <command>`                            |
| `rbenv init`, `pyenv init`, `pyenv virtualenv-init`, `nodenv init` | `rbenv exec <command>` (and `pyenv`, `nodenv`)       |
| `conda shell.<shell> hook`                                         | `conda run -n <env> <command>`                       |
| `brew shellenv`                                                    | the program's path under `brew --prefix`'s `bin`     |
| `starship init`                                                    | nothing: it only sets up the interactive prompt      |
| `zoxide init`                                                      | `zoxide query <keywords>`, then a literal `cd`       |
| `fnm env`                                                          | `fnm exec --using=<version> <command>`               |

If the eval is really needed and your exception policy allows it, add `# EXC:SHELL002:<reason>` to the command.

A shell reads a startup file before its script: `BASH_ENV` for bash run without `-i` (and any program that starts bash, such as git hooks or make), `ENV` for an interactive shell, and the `--rcfile` or `--init-file` of interactive bash, taken as written. When the line sets one of these (as a prefix, an `env` operand, `export` or a plain assignment), klaudiush reads the file and checks its commands with the shell's script, in the same shell, so functions it defines are followed too. The file is blocked when its path comes from command output, a variable klaudiush cannot resolve, or an expansion the shell runs at startup (`'$(cmd)'`), when it is a relative path after an unknown `cd`, written earlier on the line with unknown content, a device other than `/dev/null` or `/dev/stdin`, or cannot be read in full. A missing file is skipped, as the shell skips it. A value set only in the environment klaudiush runs in is not read. `/dev/stdin` is followed when stdin is literal or redirected from a regular file. After a write to a name klaudiush cannot read (`export $(cat .env)`, `env $(cat .env)`, `declare "$n"`, `set -k`), a shell or a script run by path is blocked, while other programs are not: a program that starts bash itself may still read a file set that way.

A variable assigned a literal value earlier on the same line, or set in the environment klaudiush runs in, is resolved: `X=status; git $X` is checked as `git status`. Only a plain assignment statement counts. A variable also assigned in a subshell, pipeline, condition, loop, function or background job, or as a command prefix, or set by `read`, `printf -v`, `mapfile`, `getopts`, a `for` loop, `eval` or a sourced script, is treated as unknown, and so is every variable used inside a loop. After a write to a name klaudiush cannot read (`declare "$v"`, `printf -v "$v"`) a `declare -l`, `-u` or `-n`, a `source`, a `mapfile -C` callback, or a program named by a variable or command output, no variable is resolved. Inside a new shell (`bash -c`, a script file), which sees only exported variables and may source `BASH_ENV` first, no variable is resolved either. A subcommand that is not a valid git command name, such as `'push '` or `$'push\n'`, is checked as the builtin git autocorrect would run, or blocked as an unknown subcommand.

Klaudiush parses every command as bash, even when the login shell is zsh. A command bash cannot parse but the zsh grammar accepts (parameter expansion flags such as `${(s:,:)var}`, `${=var}` splitting, glob qualifiers such as `*.go(N)`, `=(...)` process substitutions, anonymous functions, `foreach`, `&|`) is blocked as zsh syntax, and the message names the construct when klaudiush recognizes it. A command that is broken even as zsh is reported at the position where the zsh grammar stops. Constructs like these can run code klaudiush cannot follow, such as the `(e)` flag re-evaluating a variable's value. Short `for x (a b) cmd` loops and `for x in a; { cmd }` loops are not known to the zsh grammar klaudiush uses, so a command with one is reported as not parsing as bash and using that zsh syntax, together with the bash error position in case the command is also broken. Other zsh forms that grammar does not know, such as `if [[ -n $x ]] { cmd }`, `{ cmd } always { cmd }` or `for x in a b; cmd`, get the plain message that the command does not parse as bash, not zsh. The zsh grammar also accepts some input zsh itself rejects, such as an unknown flag in `${(Y)x}`, so every repair says to fix the syntax at the reported position if it is broken. Inline scripts (`bash -c`, `zsh -c`) and script files are parsed as bash too. Bash reads `*(e:'cmd':)` as an extended glob, but in a zsh login shell the trailing parentheses are glob qualifiers, and `e:...:`, `+func`, `oe:...:` and `o+func` run code for every file the glob matches. A `[...]` subscript qualifier is arithmetic, so one naming a variable, such as `*([x])`, expands the variable's value and any command substitution in it; a numeric one such as `*([1,3])` passes. Klaudiush blocks an extended glob that zsh would read as one of those qualifiers, anywhere in a word (the word of `${x:-word}` included, and after a numeric glob such as `<0-9>(e:cmd:)`, which bash reads as a redirect and a `>(...)` process substitution) and whatever shell runs it, so `bash -c 'ls *(e:x:)'` is blocked too. A plain pattern holding `|` or `(` and no quotes, backslashes or `$`, such as `@(a|b).go`, is a pattern group in zsh, not qualifiers, so it passes, and so do extended globs without a code-running qualifier, such as `*(foo)` or `!(x).go`. A bash extended glob that happens to spell one, such as `*(e:x:)`, is blocked as well. zsh accepts many spellings of a qualifier, so klaudiush errs toward blocking: quotes and backslashes are removed first, as zsh does, so `*('e':cmd:)` counts too, an extended glob holding any `$` expansion is blocked (zsh with `glob_subst` reads the value as qualifiers), so is zsh's `$~var`, which bash reads as plain text, an `e` followed by any character that appears again later, such as `*(:s/e/x/)`, is blocked even when zsh would read it differently, and a `+` before a name is blocked unless it is the sign of a number right after a size, time or count qualifier (`*(m+3)`, `*(Lk+10)`), so a symbolic mode such as `*(f:u+x:)` is blocked too. Ordinary-looking words can hold real qualifiers: zsh reads `!(tests)` as the `t` qualifier followed by `e` with `s` as the delimiter, which runs `t` as code, so it is blocked. Heredoc bodies and here-strings are data zsh does not glob, so qualifiers in them pass, though a shell that reads a heredoc as its script (`zsh <<'EOF'`) is checked. An extended glob holding a command substitution, such as `@(a|$(cmd))`, is blocked in any shell: bash and zsh both run it, but klaudiush keeps an extended glob as plain text and never inspects the command.

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
