# Exceptions

Exceptions let blocking command validations be bypassed when there's a good reason. Each policy controls a specific error code - whether exceptions are allowed, whether a justification is required, and how many are permitted per hour or day.

All three presets include audit logging. The difference is how strict the requirements are.

Exceptions apply to blocking `before_tool` command flows, not Codex lifecycle hooks.

See the [exceptions guide](/docs/exceptions) for the full policy syntax and audit log format.
