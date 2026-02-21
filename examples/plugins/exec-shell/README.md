# File validator

A working exec plugin in Bash that validates file operations. Blocks binary files, warns on executable scripts, and enforces a configurable size limit.

Requires `bash` and `jq`. Install jq with your package manager (`brew install jq`, `apt-get install jq`).

## Install

```bash
chmod +x file_validator.sh
mkdir -p ~/.klaudiush/plugins
cp file_validator.sh ~/.klaudiush/plugins/
```

## Configure

Add to `~/.klaudiush/config.toml`:

```toml
[[plugins.plugins]]
name = "file-validator"
type = "exec"
enabled = true
path = "~/.klaudiush/plugins/file_validator.sh"
timeout = "5s"

[plugins.plugins.predicate]
event_types = ["PreToolUse"]
tool_types = ["Write", "Edit"]

[plugins.plugins.config]
warn_on_exe = "true"
block_on_bin = "true"
max_file_size = "1048576"  # 1MB
```

## Test

Run `--info` to check the plugin loads:

```bash
./file_validator.sh --info
```

Test a binary file (should block):

```bash
echo '{"tool_name":"Write","file_path":"malware.exe","content":"binary"}' | ./file_validator.sh
```

Test an executable script (should warn):

```bash
echo '{"tool_name":"Write","file_path":"script.sh","content":"#!/bin/bash\necho hello"}' | ./file_validator.sh
```

Test a normal file (should pass):

```bash
echo '{"tool_name":"Write","file_path":"README.md","content":"# Docs"}' | ./file_validator.sh
```

## Protocol

Plugins receive a JSON request on stdin and return a JSON response on stdout.

Request fields: `event_type`, `tool_name`, `file_path`, `content`, `config`.

Pass response:

```json
{"passed": true, "should_block": false}
```

Fail response:

```json
{
  "passed": false,
  "should_block": true,
  "message": "Error description",
  "error_code": "CODE",
  "fix_hint": "How to fix it"
}
```

See the [plugins guide](/docs/plugins) for the full protocol spec and other plugin types.
