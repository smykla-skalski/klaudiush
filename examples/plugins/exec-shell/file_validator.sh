#!/usr/bin/env bash

# file_validator.sh - Sample klaudiush exec plugin that validates file operations
#
# This plugin demonstrates exec plugin capabilities:
# - JSON parsing and generation
# - Configuration handling
# - File pattern matching
# - Warning vs blocking modes
#
# Install:
#   chmod +x file_validator.sh
#   cp file_validator.sh ~/.klaudiush/plugins/
#
# Configure in ~/.klaudiush/config.toml:
#   [[plugins.plugins]]
#   name = "file-validator"
#   type = "exec"
#   path = "~/.klaudiush/plugins/file_validator.sh"
#   timeout = "5s"
#
#   [plugins.plugins.predicate]
#   event_types = ["PreToolUse"]
#   tool_types = ["Write", "Edit"]
#
#   [plugins.plugins.config]
#   warn_on_exe = "true"
#   block_on_bin = "true"
#   max_file_size = "1048576"  # 1MB

set -euo pipefail

# Handle --info flag
if [[ "${1:-}" == "--info" ]]; then
  cat <<EOF
{
  "name": "file-validator",
  "version": "1.0.0",
  "description": "Validates file operations (blocks binaries, warns on executables)",
  "author": "klaudiush"
}
EOF
  exit 0
fi

# Read request from stdin
read -r request

# Parse JSON fields using jq
tool_name=$(echo "$request" | jq -r '.tool_name // empty')
file_path=$(echo "$request" | jq -r '.file_path // empty')
content=$(echo "$request" | jq -r '.content // empty')

# Read configuration
warn_on_exe=$(echo "$request" | jq -r '.config.warn_on_exe // "true"')
block_on_bin=$(echo "$request" | jq -r '.config.block_on_bin // "true"')
max_file_size=$(echo "$request" | jq -r '.config.max_file_size // "1048576"')

# Only validate file operations
if [[ "$tool_name" != "Write" && "$tool_name" != "Edit" ]]; then
  echo '{"passed":true,"should_block":false}'
  exit 0
fi

# Check if file path is empty
if [[ -z "$file_path" ]]; then
  echo '{"passed":true,"should_block":false}'
  exit 0
fi

# Check for binary files (block if configured)
if [[ "$block_on_bin" == "true" ]]; then
  case "$file_path" in
    *.exe|*.dll|*.so|*.dylib|*.bin)
      cat <<EOF
{
  "passed": false,
  "should_block": true,
  "message": "Binary files are not allowed: $file_path",
  "error_code": "BIN_FILE",
  "fix_hint": "Use source code or text files instead",
  "doc_link": "https://github.com/smykla-labs/klaudiush/blob/main/docs/PLUGIN_GUIDE.md"
}
EOF
      exit 0
      ;;
  esac
fi

# Warn on executable scripts (unless disabled)
if [[ "$warn_on_exe" == "true" ]]; then
  case "$file_path" in
    *.sh|*.bash|*.py|*.rb|*.pl)
      # Check if content starts with shebang
      if echo "$content" | head -n1 | grep -q "^#!"; then
        cat <<EOF
{
  "passed": false,
  "should_block": false,
  "message": "Executable script detected: $file_path",
  "error_code": "EXEC_SCRIPT",
  "fix_hint": "Ensure script has proper validation and error handling",
  "details": {
    "shebang": "$(echo "$content" | head -n1)"
  }
}
EOF
        exit 0
      fi
      ;;
  esac
fi

# Check content size if available
if [[ -n "$content" ]]; then
  content_size=$(echo -n "$content" | wc -c | tr -d ' ')
  if [[ "$content_size" -gt "$max_file_size" ]]; then
    cat <<EOF
{
  "passed": false,
  "should_block": true,
  "message": "File content exceeds maximum size: $content_size bytes > $max_file_size bytes",
  "error_code": "FILE_TOO_LARGE",
  "fix_hint": "Reduce file size or increase max_file_size in config",
  "details": {
    "size": "$content_size",
    "max_size": "$max_file_size"
  }
}
EOF
    exit 0
  fi
fi

# Pass validation
echo '{"passed":true,"should_block":false}'
