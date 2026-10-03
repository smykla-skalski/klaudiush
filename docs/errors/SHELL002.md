# SHELL002: Command cannot be inspected

## Error

Klaudiush cannot see what the command finally runs. The command does not parse as bash, runs another command through more layers of launchers, scripts, aliases or functions than klaudiush follows, runs a script klaudiush cannot read, runs a git subcommand that is neither built in, installed, nor an alias klaudiush can see, or takes eval's command line, the program name, or a git or gh command word from a variable, command output or glob klaudiush cannot resolve.

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
| Program name from a variable          | `$TOOL push`, `xargs $CMD`, top-level `"$@"`    | Write it literally, or assign it literally on the line    |
| Program name from output or a glob    | `$(echo git) push`, `/usr/bin/gi? push`         | Write the program name or path literally                  |
| git or gh word from a variable        | `git $SUB`, `gh pr $ACTION`                     | Write it literally, or assign it literally on the line    |
| git or gh word from command output    | `git $(echo commit)`, `git c?mmit`              | Write the subcommand literally                            |
| eval of a variable or command output  | `eval "$LINE"`, `eval "$(tool init)"`           | Run the commands directly instead of through eval         |
| eval of a known tool's shell setup    | `eval "$(mise activate bash)"`                  | Run the command through the tool (see below)              |

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

A variable assigned a literal value earlier on the same line, or set in the environment klaudiush runs in, is resolved: `X=status; git $X` is checked as `git status`. Only a plain assignment statement counts. A variable also assigned in a subshell, pipeline, condition, loop, function or background job, or as a command prefix, or set by `read`, `printf -v`, `mapfile`, `getopts`, a `for` loop, `${X:=word}`, an array element assignment, a `declare`, `export` or `readonly` of an array, `eval` or a sourced script, is treated as unknown, and so is every variable used inside a loop. After a write to a name klaudiush cannot read (`declare "$v"`, `printf -v "$v"`), a change to `IFS`, a `declare -l`, `-u` or `-n`, a `source`, a `mapfile -C` callback, or a program named by a variable or command output, no variable is resolved. Inside a new shell (`bash -c`, a script file), which sees only exported variables and may source `BASH_ENV` first, no variable is resolved either. The program name follows the same rules: `G=git; $G status` is checked as `git status`, `$EDITOR file` resolves through the environment, `${EDITOR:-vi}` takes the default when the variable is unset (or empty, for `:-`) and its value can be trusted, an unquoted variable that expands to nothing leaves the next word as the program, `$a` of an array (its first element only) is unknown while `"${a[@]}"` resolves unless an element comes from command output, eval of `${a[@]}` or `${a[*]}` is blocked, and a path built on an unresolved variable or on command output (`"$DIR/run.sh"`, `$(cat dir)/tool`) is blocked. Three lookups are the exception when written literally, with no variables, extra arguments or redirects: `$(go env GOPATH)`, `$(go env GOBIN)` and `$(git rev-parse --show-toplevel)` are run by klaudiush in the command's directory and their path is checked like any other, so `$(go env GOPATH)/bin/git push` is still checked as `git push` and a script under the repository root is followed. They stay blocked inside a loop, after a `cd` to an unknown directory, and after any earlier command on the line other than `cd`, `pushd`, `popd`, `pwd`, `set`, `echo`, `printf` or `test`, any file write, a change to `PATH`, or an assignment to `HOME`, `XDG_CONFIG_HOME` or a `GIT_*`, `GO*` or `CGO_*` variable. In a script file klaudiush follows, `$(dirname "$0")`, `$(dirname -- "$0")` and `${0%/*}` resolve from the path the script was run by, and `"$@"`, `"$*"` and `"$1"` to `"$9"` take the arguments the script was given, so a wrapper doing `exec "$@"` is checked against what it runs. The arguments are unknown when the script was run through a launcher such as `xargs` or `sudo`, given command output, sourced, or after `shift`, `set --` or a loop. A command substitution in the program name counts as a lookup only for `which`, `command -v`, `type -p`, `type -P`, `readlink` or `realpath` naming one program; any other (`$(echo git)`, `$(type git)`) is command output, and so are arithmetic and the `{}` that `find -exec` fills in. A subcommand that is not a valid git command name, such as `'push '` or `$'push\n'`, is checked as the builtin git autocorrect would run, or blocked as an unknown subcommand.

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
