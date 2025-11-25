# Contributing to Klaudiush

Thank you for your interest in contributing to Klaudiush! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Commit Guidelines](#commit-guidelines)
- [Pull Request Process](#pull-request-process)
- [Testing](#testing)
- [Code Quality](#code-quality)

## Code of Conduct

This project adheres to a Code of Conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Getting Started

### Prerequisites

- [mise](https://mise.jdx.dev/) for tool version management

All other tools (Go, golangci-lint, task, markdownlint-cli) are managed by mise and will be installed automatically

### Setup

1. Fork the repository on GitHub
2. Clone your fork:

   ```bash
   git clone https://github.com/YOUR_USERNAME/klaudiush.git
   cd klaudiush
   ```

3. Add upstream remote:

   ```bash
   git remote add upstream https://github.com/smykla-labs/klaudiush.git
   ```

4. Install dependencies and tools:

   ```bash
   mise install
   task deps
   ```

5. Install git hooks:

   ```bash
   task install:hooks
   ```

## Development Workflow

### Branch Naming

Create descriptive, kebab-case branches with a type prefix:

```bash
git checkout -b feat/add-validator
git checkout -b fix/commit-message-parsing
git checkout -b docs/update-architecture
```

Valid branch types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`, `build`, `perf`

### Making Changes

1. **Create a feature branch** from `main`:
   
   ```bash
   git fetch upstream
   git checkout -b feat/my-feature upstream/main
   ```

2. **Write tests first** (TDD approach):

   ```bash
   task test
   ```

3. **Make your changes**:

   - Follow Go best practices and idioms
   - Keep changes focused and minimal
   - Add comments where logic isn't self-evident
   - Update documentation if needed

4. **Ensure quality**:

   ```bash
   task verify  # Runs fmt + lint + test
   ```

5. **Commit your changes** (see [Commit Guidelines](#commit-guidelines))

6. **Push to your fork**:

   ```bash
   git push origin feat/my-feature
   ```

### Adding a New Validator

1. Create validator file in `internal/validators/{category}/`:

   ```go
   package category
   
   import (
       "github.com/smykla-labs/klaudiush/internal/validator"
       "github.com/smykla-labs/klaudiush/pkg/hook"
   )
   
   type MyValidator struct {
       validator.BaseValidator
   }
   
   func NewMyValidator(logger logger.Logger) *MyValidator {
       v := &MyValidator{}
       v.SetLogger(logger)
       return v
   }
   
   func (v *MyValidator) Name() string {
       return "MyValidator"
   }
   
   func (v *MyValidator) Validate(ctx *hook.Context) *validator.Result {
       // Validation logic here
       return validator.Pass()
   }
   ```

2. Write comprehensive tests using Ginkgo/Gomega

3. Register validator in `cmd/klaudiush/main.go`

4. Update documentation in README.md and CLAUDE.md

## Commit Guidelines

### Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/) format:

```
type(scope): description

Optional body with more details.
Lines should be ≤72 characters.
```

### Commit Message Rules

- **Title**: ≤50 characters
- **Body lines**: ≤72 characters
- **Type**: Use appropriate type for the change
- **Scope**: Use lowercase, descriptive scope
- **Description**: Clear, concise summary in imperative mood

### Commit Types

**User-facing changes**:

- `feat`: New feature for users
- `fix`: Bug fix for users

**Infrastructure changes** (use specific type, NOT `feat` or `fix`):

- `ci`: CI/CD changes
- `test`: Test changes
- `docs`: Documentation changes
- `build`: Build system changes
- `chore`: Maintenance tasks
- `refactor`: Code refactoring
- `style`: Code style changes
- `perf`: Performance improvements

### Examples

✅ **Good**:

```
feat(validators): add Terraform format validator

ci(upgrade): handle /v2 module path in workflow

test(parser): add test for heredoc parsing
```

❌ **Bad**:

```
fix(ci): update workflow  # Use ci(...) instead
feat(test): add helper    # Use test(...) instead
update code              # Missing type/scope
This is a very long commit message that exceeds fifty characters
```

### Required Flags

Always use `-sS` flags when committing:

```bash
git commit -sS -m "feat(api): add user endpoint"
```

- `-s`: Add Signed-off-by line
- `-S`: GPG sign the commit

## Pull Request Process

### Creating a Pull Request

1. **Ensure your branch is up to date**:

   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run quality checks**:
   
   ```bash
   task verify
   ```

3. **Push your changes**:

   ```bash
   git push origin feat/my-feature
   ```

4. **Create PR** using semantic title:

   ```bash
   gh pr create --title "feat(validators): add Terraform validator" \
     --body "## Summary
   - Add Terraform format validation
   - Add tflint integration
   
   ## Test plan
   - Unit tests for Terraform validator
   - Integration tests with sample .tf files"
   ```

### PR Title Format

Use same format as commit messages:

```
type(scope): description
```

### PR Body Requirements

Include these sections:

- **Summary**: 1-3 bullet points describing changes
- **Test plan**: How you tested the changes
- **Documentation**: Any docs updates needed

### PR Guidelines

- Keep PRs focused on a single concern
- Link related issues using `Fixes #123` or `Relates to #456`
- Respond to review comments promptly
- Update PR based on feedback
- Ensure all CI checks pass
- Do not remove existing labels unless explicitly asked

### Changelog

- **Default**: PR title is used automatically
- **Override**: Use `> Changelog: type(scope): description` in PR body when title is insufficient
- **Skip**: Use `> Changelog: skip` for trivial/internal changes

## Testing

### Running Tests

```bash
# All tests
task test

# Unit tests only
task test:unit

# Integration tests only
task test:integration

# Specific package
go test -v ./pkg/parser

# Single test
go test -v ./pkg/parser -run TestBashParser
```

### Writing Tests

- Use Ginkgo/Gomega framework
- Write descriptive test names
- Test both success and failure cases
- Test edge cases and error conditions
- Aim for high test coverage

Example:

```go
var _ = Describe("MyValidator", func() {
    var (
        validator *MyValidator
        logger    logger.Logger
    )

    BeforeEach(func() {
        logger = logger.NewLogger(io.Discard, false, false)
        validator = NewMyValidator(logger)
    })

    It("should pass for valid input", func() {
        ctx := &hook.Context{
            EventType: hook.PreToolUse,
            ToolName:  hook.Bash,
            Command:   "valid command",
        }
        result := validator.Validate(ctx)
        Expect(result.ShouldBlock).To(BeFalse())
    })

    It("should fail for invalid input", func() {
        ctx := &hook.Context{
            EventType: hook.PreToolUse,
            ToolName:  hook.Bash,
            Command:   "invalid command",
        }
        result := validator.Validate(ctx)
        Expect(result.ShouldBlock).To(BeTrue())
    })
})
```

## Code Quality

### Linting

The project uses comprehensive linting with golangci-lint (65+ linters):

```bash
# Lint and auto-fix
task check

# Lint only
task lint

# Lint staged files
task lint:staged
```

### Code Style

- Follow idiomatic Go practices
- Use meaningful variable and function names
- Keep functions focused and small
- Prefer composition over inheritance
- Handle errors explicitly
- Use context for cancellation and timeouts

### Go-specific Guidelines

From `CLAUDE.md`:

- Use `slices` package (NEVER `sort.Slice`)
- Use `maps` package for map operations
- Use `cmp.Compare` or `cmp.Or` for comparisons
- Use `errors` or `github.com/pkg/errors` (NO `fmt.Errorf`)
- Prefer `switch` over `if` chains
- Package-level consts/vars instead of hardcoded strings

### Pre-commit Hooks

The project includes git hooks that run automatically:

**Pre-commit**:

- Lints staged files
- Tests packages with changes

**Pre-push**:

- Full linting
- Full test suite

To bypass hooks (not recommended):

```bash
git commit --no-verify
git push --no-verify
```

## Getting Help

- **Issues**: https://github.com/smykla-labs/klaudiush/issues
- **Discussions**: https://github.com/smykla-labs/klaudiush/discussions
- **Documentation**: See README.md and CLAUDE.md

## License

By contributing to Klaudiush, you agree that your contributions will be licensed under the MIT License.
