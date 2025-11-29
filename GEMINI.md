# GEMINI.md

Context and instructions for Gemini when working on the `klaudiush` project.

## Project Overview

`klaudiush` is a **validation dispatcher for Claude Code hooks**, written in **Go**. It intercepts `PreToolUse` events from Claude Code to validate commands and file operations before they are executed. Its primary goal is to enforce git workflow standards, commit message conventions, code quality rules, and security policies (e.g., preventing writes to `/tmp`).

**Key Architecture:**
*   **Flow:** CLI Entry (`cmd/klaudiush`) → JSON Parser → Dispatcher → Registry (Validators matched via Predicates) → Result (Pass/Fail/Warn).
*   **Parsers:** Uses `mvdan.cc/sh` for advanced Bash parsing and `go-git` for high-performance git operations.
*   **Configuration:** Hierarchical TOML configuration (CLI flags > Env vars > Project config > Global config > Defaults).
*   **Extensibility:** Supports dynamic rules via TOML and external plugins (Go/Exec/gRPC).

## Building and Running

This project uses `task` (Taskfile) for automation and `mise` for tool version management.

*   **Build:**
    *   `task build`: Development build (bin/klaudiush).
    *   `task build:prod`: Production build.
    *   `task install`: Installs binary to `~/.local/bin` or `~/bin`.

*   **Testing:**
    *   `task test`: Run all tests (concise output).
    *   `task test:unit`: Run unit tests only.
    *   `task test:integration`: Run integration tests only.
    *   `task test:fuzz`: Run fuzz tests.
    *   `task verify`: Run format + lint + test (Recommended before PR).

*   **Linting & Quality:**
    *   `task check`: Run linters and auto-fix.
    *   `task lint`: Run linters only.
    *   `task fmt`: Format code.

*   **Dependencies:**
    *   `task deps`: Download and tidy Go modules.

## Development Conventions

*   **Language:** Go 1.25.4+.
*   **Style:**
    *   Follow idiomatic Go.
    *   Use `github.com/cockroachdb/errors` for error handling (not stdlib `errors` or `pkg/errors`).
    *   Use `cmp` package for comparisons and `slices`/`maps` for collections.
*   **Testing:**
    *   Framework: Ginkgo and Gomega.
    *   Tests must be comprehensive (success, failure, edge cases).
    *   Mocks are generated via `uber-go/mock` (`go generate ./...`).
*   **Git & Workflow:**
    *   **Branches:** Kebab-case with type prefix (e.g., `feat/my-feature`, `fix/bug-fix`).
    *   **Commits:** Strict **Conventional Commits** format: `type(scope): description`.
        *   Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`, `build`, `perf`.
        *   **MUST** use `-sS` flags (Signed-off-by and GPG sign).
    *   **PRs:** Semantic title matching commit format. Body must include Summary and Test Plan.
*   **Error Handling Policy:**
    *   Validators return structured errors with codes (e.g., `GIT001`).
    *   Use `FailWithRef` to populate fix hints automatically from the registry.
*   **Logging:**
    *   Use the structured logger provided in the context.
    *   Logs are written to `~/.claude/hooks/dispatcher.log`.
