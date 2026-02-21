# Development Environment Setup

## Prerequisites

This project uses [mise](https://mise.jdx.dev/) for managing development tool versions.

### Install mise

```bash
# macOS
brew install mise

# Linux & Windows (WSL)
curl https://mise.run | sh
```

### Activate mise in your shell

Add this to your shell configuration (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
eval "$(mise activate bash)"  # or zsh, fish, etc.
```

Or for the current session:

```bash
eval "$(mise activate bash --shims)"
```

## Quick Start

1. Install project tools:
   
   ```bash
   mise install
   ```
   
   This will install:

   - Go 1.26.0
   - golangci-lint 2.10.1
   - markdownlint-cli2 0.21.0
   - ginkgo 2.28.1
   - lefthook 2.1.1

2. Verify installation:

   ```bash
   mise list
   ```

3. Run development tasks:

   ```bash
   mise tasks        # Show available tasks
   mise run build    # Build the binary
   mise run test     # Run tests
   mise run lint     # Run linters
   ```

## Tool Versions

Tool versions are pinned in `.mise.toml`:

```toml
[tools]
go = "1.26.0"
golangci-lint = "2.10.1"
markdownlint-cli2 = "0.21.0"
ginkgo = "2.28.1"
lefthook = "2.1.1"
```

To update versions:

1. Check available versions:

   ```bash
   mise ls-remote go
   mise ls-remote golangci-lint
   mise ls-remote markdownlint-cli2
   ```

2. Update `.mise.toml` with new versions
3. Run `mise install` to apply changes

## Benefits of mise

- **Version pinning**: Ensures all developers use the same tool versions
- **Automatic activation**: Tools are available when you `cd` into the project
- **Per-project isolation**: Different projects can use different versions
- **Fast**: Tools are downloaded and cached locally
- **No system pollution**: Tools are installed in `~/.local/share/mise`

## Without mise

If you prefer not to use mise, you can install tools manually:

```bash
# Install Go 1.26.0
# See https://go.dev/dl/

# Install golangci-lint 2.10.1
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.10.1
```

Then run Go commands directly instead of via `mise run`.
