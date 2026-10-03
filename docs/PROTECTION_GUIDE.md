# Protection guide

Keep an agent from weakening the policy enforced on it, and decide which MCP servers it may call by where they came from rather than by what they call themselves.

## Table of contents

- [Overview](#overview)
- [Protected files](#protected-files)
- [Configuration](#configuration)
- [What counts as a change](#what-counts-as-a-change)
- [Settings changed mid-session](#settings-changed-mid-session)
- [klaudiush commands](#klaudiush-commands)
- [Authorized maintenance](#authorized-maintenance)
- [MCP trust](#mcp-trust)
- [Provider coverage](#provider-coverage)
- [Managed configuration](#managed-configuration)
- [What protection cannot prevent](#what-protection-cannot-prevent)
- [Troubleshooting](#troubleshooting)

## Overview

Both features are off by default. `[protection]` blocks tool calls and shell commands that would change a file that enforces policy ([POL001](errors/POL001.md)), keeps Claude from applying settings changed during a session ([POL002](errors/POL002.md)), and blocks klaudiush commands that change policy ([POL003](errors/POL003.md)). `[mcp_trust]` blocks MCP tool calls from servers whose harness-reported provenance matches nothing trusted ([MCP004](errors/MCP004.md)) and applies a configured action to calls that carry no provenance ([MCP005](errors/MCP005.md)).

These checks guard the rest of klaudiush, so they fail closed:

- An exception token cannot bypass POL001-POL003, MCP004 or MCP005 unless the configuration has an `[exceptions.policies.<CODE>]` entry for that exact code.
- When protection or MCP trust is enabled, the validator is critical: a crash or a hook running past its deadline blocks the action whatever `failure_policy.mode` says.
- A shell command klaudiush cannot fully inspect (the same commands [SHELL002](errors/SHELL002.md) reports) is blocked with POL001 too, so an exception for SHELL002 does not open a way around protection.
- Neither validator consults the rule engine: a `[[rules]]` entry is not the explicit `protection.allow` or `mcp_trust` entry these checks require.

## Protected files

With `[protection] enabled = true`, these are protected:

| What | Paths |
|:--|:--|
| klaudiush configuration | any `.klaudiush/` directory and any `klaudiush.toml` (the loader walks up from the working directory, so a new one in a subdirectory would win), `$XDG_CONFIG_HOME/klaudiush/`, `~/.klaudiush/` |
| klaudiush state | `$XDG_STATE_HOME/klaudiush/` (hook session and evidence state, exception rate limits, audit log), `$XDG_DATA_HOME/klaudiush/` |
| klaudiush binary | the running binary, `~/.claude/hooks/dispatcher`, and `klaudiush` or `dispatcher` in every `PATH` directory, so a shim cannot take its place. Copying or linking the binary is blocked with POL003 |
| Claude Code | any `.claude/settings.json` and `.claude/settings.local.json`, any `.claude/hooks/` directory, any `.mcp.json`, `~/.claude.json`, the managed settings directory (`/Library/Application Support/ClaudeCode`, `/etc/claude-code`) |
| Codex | `$CODEX_HOME` (default `~/.codex`) `hooks.json`, `config.toml`, `requirements.toml`, `managed_config.toml`; any `.codex/hooks.json` and `.codex/config.toml`; `/etc/codex/requirements.toml` and `managed_config.toml` |
| Gemini CLI | any `.gemini/settings.json`, the system settings and defaults files (`/Library/Application Support/GeminiCli/`, `/etc/gemini-cli/`, or the paths in `GEMINI_CLI_SYSTEM_SETTINGS_PATH` and `GEMINI_CLI_SYSTEM_DEFAULTS_PATH`) |
| opencode | the klaudiush bridge plugin (`$XDG_CONFIG_HOME/opencode/plugin/klaudiush.ts`, or `providers.opencode.plugin_path`) |
| Configured hook files | `providers.codex.hooks_config_path`, `providers.gemini.settings_path` |
| Hook scripts | files named by the hook commands registered in the Claude, Codex (`hooks.json`, `config.toml`, `requirements.toml`) and Gemini settings above, with `$CLAUDE_PROJECT_DIR`, `$HOME` and other variables expanded and quotes removed, including scripts that do not exist yet |
| Evidence check scripts | files named by `evidence.checks.commands`, such as `./scripts/test.sh`, including ones that do not exist yet |
| klaudiush plugins | `plugins.plugins.path` |
| Extra paths | `protection.paths` |

A path is protected when it is one of these, lies below a protected directory, resolves to one through symlinks, or is a second hard link to a protected file. When `~/.claude/settings.json` or `~/.claude` is a symlink into a dotfiles repository, the file in the repository is protected too. On macOS and Windows names are compared ignoring case, using Unicode case folding, so `.CLAUDE/Settings.json` is the same file.

Task runner manifests that an evidence check runs through (`mise.toml`, `Makefile`, `package.json`) are not protected by default, because projects edit them routinely. Add them to `protection.paths` when a check depends on them.

## Configuration

```toml
[protection]
enabled = true

# Extra protected paths: a name without a slash matches in any directory,
# anything else is relative to the project root; globs and ~ are accepted
paths = ["scripts/ci/**", "Makefile"]

# Paths the agent may change although they are protected
allow = [".claude/settings.local.json"]

# Claude ConfigChange sources blocked mid-session (default shown)
config_change_sources = ["user_settings", "project_settings"]
```

| Field | Meaning | Default |
|:--|:--|:--|
| `enabled` | Turn protection on | `false` |
| `paths` | Extra protected paths or patterns. Entries protect what is below them | none |
| `allow` | Paths or patterns exempt from protection, in the same form | none |
| `config_change_sources` | Claude `ConfigChange` sources whose changes are kept from taking effect. `policy_settings` cannot be blocked. An empty list blocks none. `local_settings` is not in the default because Claude Code writes "don't ask again" permission rules to `.claude/settings.local.json`; add it to block those too, at the cost of repeated prompts | user and project settings |

Turn protection on or off for one shell with `KLAUDIUSH_PROTECTION_ENABLED=true` or `false`. The agent cannot set the environment of the hook process.

klaudiush loads the project configuration of the directory a command `cd`s into before running git, so that commits follow that project's rules. Protection and MCP trust stay on when they are enabled in the configuration of that directory, of the hook's working directory, or of the harness project directory (`CLAUDE_PROJECT_DIR`, `GEMINI_PROJECT_DIR`). Enable them in the global configuration to protect every project the agent can reach.

## What counts as a change

File tools: `Write`, `Edit`, `MultiEdit`, `NotebookEdit`, Codex `apply_patch` (every `Add`, `Update`, `Delete` and `Move to` file of the patch, however it is spaced), Gemini `write_file` and `replace`, opencode `write` and `edit`, and tools of any other kind, including MCP tools, through every path-like string in their input. Tools whose name says they only read (`read_file`, `list_directory`, `get_file_contents`, ...) are not checked, unless an MCP tool's name also mentions writing (`find_and_replace`, `read_then_overwrite`): a server names its own tools. `file://` URIs count as paths. Patch bodies in any input field (including opencode `patchText`) are read for their file lines, and strings with spaces count when they start like a path.

Shell commands, through the parser that every other validator uses, including commands run by launchers (`env`, `sudo`, `xargs`, `find -exec`), shells (`bash -c`, `eval`, scripts on `PATH` under `$HOME`) and interpreters:

- Output redirects of every kind: `>`, `>>`, `>|`, `&>`, `&>>`, `<>`, `>&file`, heredocs and `tee`.
- Programs that change the files they name: `rm`, `mv`, `ln`, `touch`, `truncate`, `chmod`, `chown`, `dd of=`, `curl -o`, editors, interpreters (`python -c`, `node -e`, `perl -pi`), and any program klaudiush does not know to be read-only. Words inside interpreter code count, whether passed as an argument or on stdin (heredoc, here-string), and a relative name in the code is also tried next to every other path in it, so `os.symlink('settings.json', '.claude/settings.local.json')` counts.
- `cp`, `install`, `rsync`, `scp` and `ditto` change only their destination, including `destination/<source name>`, unless they link (`cp -l`, `cp -s`) or remove sources.
- `cp`, `install` and `mv` with `-t`/`--target-directory` treat every operand as a source. `ditto`, recursive copies and sources ending in `/` or `/.` change everything below the destination.
- `sed`, `sort`, `yq`, `awk` and `find` count only with in-place, output, write or delete options; a `find -name` test that matches a protected name, such as `settings.json` or `.klaudiush`, counts wherever it runs. `git` counts for subcommands that take paths (`checkout`, `restore`, `rm`, `mv`, `clean`, `config -f`, ...) when a path argument (including a `:/` root pathspec) names a protected file or a directory holding one.
- Git commands that rewrite the working tree without naming paths are checked against the repository: `git clean` (untracked, and ignored with `-x`/`-X`, within its pathspecs), `git stash` (changed files, untracked with `-u`/`-a`), `git stash pop`/`apply` (what the stash holds), `git checkout -f`, `git switch -f`, `git checkout-index -f` and `git read-tree -u` (changed files), `git reset`, `git checkout`/`switch` (including `-B`/`-C` start points), `git update-ref`, `git branch -f`, `git symbolic-ref`, `git merge`, `git rebase`, `git cherry-pick` and `git revert` (the files that differ from `HEAD` in the revision they move to or bring in; a ref moved to a revision klaudiush cannot see, such as `$(git commit-tree ...)`, counts), and `git apply`, `git am` and `patch` (the files the patch names). Each counts when it would change a protected file, so switching to a branch whose `.klaudiush/config.toml` differs is blocked; ask the user to do it.
- Directories: removing, moving or changing permissions of a directory that holds a protected file counts, including the working directory itself (`rm -rf .`, `git checkout .`, `chmod -R 777 .`). Other programs naming the working directory (`pytest .`) do not.
- Paths are resolved against the hook's working directory, the directory a shell tool call names (Gemini `dir_path`, Codex `workdir`) and earlier `cd` commands (after a `cd` to a directory klaudiush cannot resolve, a relative name matches in any directory), with backslash escapes and `$'...'` strings decoded as the shell does, `./` and `..` cleaned in globs, POSIX classes (`[[:alpha:]]`), brace ranges (`{a..z}`) and zsh `^` negation understood, with `~`, `$HOME`, variables assigned on the same line, `..`, globs (`*`, `?`, `[...]`, `**`) and brace lists (`{a,b}`) expanded. A part klaudiush cannot know before the command runs (an unset variable, `$(...)`) matches anything there: `rm "$(echo .claude)/settings.json"` is blocked because `*/settings.json` can be a protected file. A target known only when the command runs (`> "$f"`, `> "$(cmd)"`, a variable assigned from `$(...)`, `xargs` fed by output klaudiush cannot reconstruct, zsh `>!`) counts when the command names a protected path anywhere, including as a glob in a `for` list.
- Read-only programs (`cat`, `grep`, `rg`, `jq`, `diff`, `ls`, `gh`, `git log`, ...) never count, and commands that only mention a path, such as `git commit -m "update .claude/settings.json"`, pass.

After a Claude shell command ran, PostToolUse reports protected files listed in `tool_response.bashEditDiff` (when Claude Code records it), asking the agent to restore them. That catches what the command text did not show; it cannot undo the change.

## Settings changed mid-session

Claude Code runs `ConfigChange` hooks when a settings file changes during a session, whoever changed it. With protection on, klaudiush answers `decision: "block"` for the sources in `config_change_sources`, so the new settings are not applied to the running session. Claude Code shows nobody the reason and applies the file at the next start, so you can still edit settings yourself: restart the session to load them. `policy_settings` changes cannot be blocked, and server-managed settings never reach the hook.

klaudiush does not register `ConfigChange` itself. Add it to the settings that run klaudiush:

```json
{
  "hooks": {
    "ConfigChange": [
      {"hooks": [{"type": "command", "command": "klaudiush --provider claude --event ConfigChange"}]}
    ]
  }
}
```

`klaudiush doctor --category protection` warns about Claude settings that run klaudiush without it.

## klaudiush commands

klaudiush is recognized by its name, the `dispatcher` name the installer uses, or by being the same file as (or a byte-identical copy of) the running binary. The agent can run read-only klaudiush commands: `--help`, `--version`, `evidence`, `doctor` without `--fix`, `debug` (but not `debug crash clean`), `audit list` and `stats`, `backup list`, `status` and `audit`, `bypass status`, `patterns list` and `stats`, `suggest`, `version`. Everything else changes configuration, overrides, state, backups or the binary (`init`, `disable`, `enable`, `bypass skip`, `backup restore`, `doctor --fix`, `update`, ...) and is blocked with POL003, and so is hook mode (`klaudiush --event ...` with a payload on stdin), since a forged payload can reset session state or record check runs that never happened. `suggest --output` is checked like any other write. Run those yourself.

## Authorized maintenance

There is no automatic bypass. To let the agent maintain a policy file, say so in the configuration, which the agent cannot change:

- List the path in `protection.allow` for as long as the work takes.
- Or add an exception policy for the code, so a token with a reason can bypass it and is audited:

```toml
[exceptions.policies.POL001]
enabled = true
allow_exception = true
require_reason = true
min_reason_length = 20
max_per_day = 3
```

## MCP trust

Claude Code reports the server behind an MCP tool call in `mcp_server` (Claude Code 2.1.274 and later): its registered `name` and a `source` saying where its definition came from (`sdk`, `plugin`, `user`, `project`, `local`, `dynamic`, `managed`, `enterprise`, `claudeai`, `agent`). Gemini CLI reports `mcp_context`: the server name and its transport (`command` and `args` for stdio, `url` for HTTP or SSE, `tcp` for WebSocket). The name in `mcp__<server>__<tool>` is chosen by whoever configured the server, so anyone can reuse a trusted name; klaudiush bases trust on the provenance instead.

```toml
[mcp_trust]
enabled = true

# Claude sources whose servers are trusted whatever their name
trusted_sources = ["managed", "enterprise", "user"]

# What happens to an untrusted call: block or warn
untrusted = "block"

# What happens to a call without provenance: block, warn or allow
unknown_provenance = "block"

# Individually trusted servers: every field set must match, and at least
# one of source, command or url must be set
[[mcp_trust.servers]]
name = "docs"
source = "project"
tools = ["search*", "read*"]

[[mcp_trust.servers]]
url = "https://mcp.example.com/*"
```

| Field | Meaning | Default |
|:--|:--|:--|
| `enabled` | Turn MCP trust checks on | `false` |
| `trusted_sources` | Claude `mcp_server.source` values trusted for any server. An unknown source is trusted only when listed exactly | none |
| `servers` | Trusted servers by `name`, `source`, `command`, `args`, `url` and optionally `tools`, all globs. `url` is compared by scheme, host and path separately. Set `args` with generic commands such as `npx`, `uvx` or `docker`, which run whatever their arguments name. A name alone is rejected when the configuration loads | none |
| `untrusted` | `block` or `warn` for calls from untrusted servers | `block` |
| `unknown_provenance` | `block`, `warn` or `allow` for calls whose payload names no server: Codex, opencode, Claude before 2.1.274, Gemini without a transport | `block` |

A message about an untrusted call names the reported server and source, escaped, and says when the tool name claims a different server.

Claude routes MCP tools to klaudiush only when the `PreToolUse` matcher selects them. The installer registers `Bash|Write|Edit|MultiEdit`; add `mcp__.*` when you enable MCP trust. `klaudiush doctor --category protection` warns when it is missing.

## Provider coverage

| Provider | File and shell changes | Settings changed mid-session | MCP provenance |
|:--|:--|:--|:--|
| Claude Code | `PreToolUse` for the tools the matcher selects; `PostToolUse` reports changed protected files | `ConfigChange`, except `policy_settings` | `mcp_server` name and source (2.1.274+) |
| Codex | `PreToolUse` for Bash, `apply_patch`, MCP and local function tools | none; Codex asks to trust changed hooks again | none |
| Gemini CLI | `BeforeTool` for every tool | none; Gemini warns about changed project hooks | `mcp_context` transport, no source |
| opencode | `tool.execute.before` for every tool | none | none |

## Managed configuration

A user, unlike the agent, can always remove hooks they registered. To make klaudiush mandatory, register it where users cannot remove it:

- Claude Code: managed settings (`managed-settings.json`) with `allowManagedHooksOnly` so user and project hooks cannot replace it; `disableAllHooks` outside managed settings cannot disable managed hooks.
- Codex: `requirements.toml` `[hooks]` with `[features].hooks = true` pinned, and `allow_managed_hooks_only = true`.

`klaudiush doctor --category protection` lists managed settings files that exist and whether they run klaudiush.

## What protection cannot prevent

- `git pull`, and `git rebase` or `git merge` of a branch fetched in the same command: what they bring in is known only after the fetch.
- Interpreter code that assembles a path at run time, such as `open('.cl' + 'aude/settings.json', 'w')`, a module run with `python -m`, and bash namerefs (`declare -n`).
- Archives extracted in place (`tar x`, `unzip`, `git archive | tar x`): member names are not read.
- `go install` and other build tools writing into `PATH` directories they do not name.
- A program that writes files it does not name: a script file, a compiled program, a build tool, a formatter run over the whole tree (`prettier --write .`), or an archive extracted in place. Claude's `bashEditDiff`, when recorded, reports such changes afterwards.
- A command whose target comes from a file or a program klaudiush cannot see (`rm $(cat list)`), when the command names no protected path.
- Tools that do not reach klaudiush: Claude tools outside the matcher, Codex hosted tools, and anything outside the agent.
- Changes made by you or other processes; `ConfigChange` keeps Claude settings from applying mid-session, but other harnesses read their files at startup.
- An invalid configuration file: klaudiush then cannot load the protection settings and follows `failure_policy`.

## Troubleshooting

`klaudiush doctor --category protection` checks that:

- The protection configuration compiles.
- Claude settings running klaudiush also run it on `ConfigChange`.
- Claude settings running klaudiush on `PreToolUse` select MCP tools when MCP trust is on.
- Some MCP server is trusted, and whether `unknown_provenance = "allow"` leaves providers unchecked.

It also lists what each provider lets protection enforce and which managed configuration runs klaudiush.
