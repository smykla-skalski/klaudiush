# SHELL001: Unescaped backticks in strings

## Error

Command substitution (backticks or `$()`) detected in double-quoted strings, which can cause unexpected behavior when the shell interprets them.

## Why this matters

Backticks inside double quotes trigger command substitution. In commit messages like `fix bug in `parser` module`, the shell tries to execute `parser` as a command. This produces garbled output or errors.

## How to fix

### Use HEREDOC (recommended)

```bash
git commit -sS -m "$(cat <<'EOF'
Fix bug in `parser` module
EOF
)"
```

### Use file-based input

```bash
echo "Fix bug in \`parser\` module" > commit-msg.txt
git commit -sS -F commit-msg.txt
```

### Escape backticks

```bash
git commit -sS -m "Fix bug in \`parser\` module"
```

## Modes

### Legacy mode (default)

Validates backticks only in specific commands: `git commit -m`, `gh pr create --body/--title`, `gh issue create --body/--title`.

### Comprehensive mode (opt-in)

Validates all Bash commands for backtick issues. Detects unquoted backticks, backticks in double quotes, and suggests single quotes when no variables are present.

## Configuration

```toml
[validators.shell.backtick]
enabled = true
check_all_commands = false      # enable comprehensive mode
check_unquoted = false          # check unquoted backticks (comprehensive mode)
suggest_single_quotes = true    # suggest single quotes when no vars present
```

## Hook output

When this error is triggered, klaudiush writes JSON to stdout:

**permissionDecisionReason** (shown to Claude):
`[SHELL001] Command substitution detected in double-quoted strings. Use HEREDOC syntax or file-based input (git commit -F file.txt)`

**systemMessage** (shown to user):
Formatted error with fix hint and reference URL.

**additionalContext** (behavioral guidance):
`Automated klaudiush validation check. Fix the reported errors and retry the same command.`

## Related

- [GIT004](GIT004.md) - commit title issues
