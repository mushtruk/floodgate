# Contributing to floodgate

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/mushtruk/floodgate.git`
3. Add upstream remote: `git remote add upstream https://github.com/original/floodgate.git`
4. Create a feature branch: `git checkout -b feature/your-feature-name`

## Development Setup

### Prerequisites

- Go 1.22 or later
- Make (optional, but recommended)

### Install Development Tools

```bash
make install-tools
```

This installs:
- `golangci-lint` - Linter
- `goimports` - Import formatter

## Making Changes

### Code Style

- Follow standard Go conventions
- Run `make fmt` before committing
- Run `make lint` and fix all issues
- Use meaningful variable names
- Add comments for exported functions

### Testing

- Write tests for all new functionality
- Maintain or improve test coverage
- Run tests: `make test`
- Run benchmarks: `make bench`

### Commit Messages

Follow conventional commits format:

```
type(scope): subject

body (optional)

footer (optional)
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or updating tests
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Build/tooling changes

Examples:
```
feat(tracker): add support for custom percentile calculations
fix(circuit): prevent race condition in state transitions
docs(readme): update installation instructions
```

## Pull Request Process

1. Update documentation for any user-facing changes
2. Add tests for new functionality
3. Ensure all tests pass: `make test`
4. Update CHANGELOG.md (if applicable)
5. Submit PR with clear description of changes
6. Link related issues in PR description

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Manual testing performed
- [ ] Benchmarks run (if performance-related)

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex logic
- [ ] Documentation updated
- [ ] Tests pass locally
```

## Code Review

- Be respectful and constructive
- Focus on code quality, not personal preferences
- Explain reasoning behind suggestions
- Be open to feedback

## Versioning

We use [Semantic Versioning](https://semver.org/):
- MAJOR: Incompatible API changes
- MINOR: Backwards-compatible functionality
- PATCH: Backwards-compatible bug fixes

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Questions?

Open an issue or start a discussion on GitHub.
