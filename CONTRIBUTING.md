# Contributing to Smykla Labs Projects

Thank you for your interest in contributing! This document provides guidelines and instructions for contributing to Smykla Labs projects.

## Code of Conduct

This project adheres to a Code of Conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Getting Started

### Prerequisites

Check the project's README for specific prerequisites. Common requirements include:

- [mise](https://mise.jdx.dev/) for tool version management
- Project-specific tools managed by mise

### Setup

1. Fork the repository on GitHub
2. Clone your fork:

   ```bash
   git clone https://github.com/YOUR_USERNAME/REPO_NAME.git
   cd REPO_NAME
   ```

3. Add upstream remote:

   ```bash
   git remote add upstream https://github.com/smykla-labs/REPO_NAME.git
   ```

4. Install dependencies (if applicable):

   ```bash
   mise install
   ```

## Development Workflow

### Branch Naming

Create descriptive, kebab-case branches with a type prefix:

```bash
git checkout -b feat/add-feature
git checkout -b fix/bug-description
git checkout -b docs/update-readme
```

Valid branch types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`, `build`, `perf`

### Making Changes

1. **Create a feature branch** from `main`:

   ```bash
   git fetch upstream
   git checkout -b feat/my-feature upstream/main
   ```

2. **Make your changes**:

   - Follow project-specific coding standards
   - Keep changes focused and minimal
   - Add comments where logic isn't self-evident
   - Update documentation if needed

3. **Run quality checks** (if applicable):

   ```bash
   make check  # or task check
   ```

4. **Commit your changes** (see [Commit Guidelines](#commit-guidelines))

5. **Push to your fork**:

   ```bash
   git push origin feat/my-feature
   ```

## Commit Guidelines

### Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/) format:

```text
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

```text
feat(api): add user authentication endpoint

ci(workflows): update release workflow

test(parser): add edge case tests
```

❌ **Bad**:

```text
fix(ci): update workflow  # Use ci(...) instead
feat(test): add helper    # Use test(...) instead
update code              # Missing type/scope
```

## Pull Request Process

### Creating a Pull Request

1. **Ensure your branch is up to date**:

   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run quality checks** (if applicable)

3. **Push your changes**:

   ```bash
   git push origin feat/my-feature
   ```

4. **Create PR** using semantic title:

   ```bash
   gh pr create --title "feat(scope): description" --body "..."
   ```

### PR Title Format

Use same format as commit messages:

```text
type(scope): description
```

### PR Guidelines

- Keep PRs focused on a single concern
- Link related issues using `Fixes #123` or `Relates to #456`
- Respond to review comments promptly
- Update PR based on feedback
- Ensure all CI checks pass

## Getting Help

- **Issues**: Open an issue in the relevant repository
- **Discussions**: Use GitHub Discussions if available

## License

By contributing, you agree that your contributions will be licensed under the project's license.
