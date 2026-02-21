# Configuration

klaudiush works out of the box with no config files. These examples cover common setups - copy one, trim what you don't need, and you're done.

Config files go in `~/.klaudiush/config.toml` (global) or `.klaudiush/config.toml` (per-project). Project settings override global ones through deep merge - only the fields you set get overridden.

Start with `minimal.toml` to turn off a few validators. Use `full.toml` as a reference for every available option. Language-specific files (`javascript.toml`, `rust.toml`) show how to wire up external tools like oxlint and rustfmt.

See the [configuration guide](/docs/configuration) for the full hierarchy, environment variables, and CLI flags.
