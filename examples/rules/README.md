# Rules

Rules let you block, warn, or allow operations based on patterns. They match against file paths, branch names, remote names, command strings, and file content.

Each rule has a priority - higher numbers match first. When `stop_on_first_match` is enabled, the first matching rule wins.

Start with `organization.toml` for a fork-based git workflow, or `secrets-allow-list.toml` to stop false positives in test directories. `advanced-patterns.toml` covers every pattern type including glob, regex, content matching, negation, and multi-pattern logic.

See the [rules guide](/docs/rules) for the full matching syntax and action types.
