# Contributing to Foundry

Thank you for your interest in contributing to Foundry!

## Development Setup

1. Clone the repo: `git clone https://github.com/thomasvincent/foundry.git`
2. Install Go 1.22+
3. Install golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
4. Run tests: `make test`
5. Run linter: `make lint`

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `test:` tests
- `refactor:` code restructuring
- `chore:` maintenance

## Pull Request Process

1. Fork the repo and create a feature branch from `main`
2. Write tests for new functionality
3. Ensure `make all` passes (lint + test + build)
4. Open a PR with a clear description
5. Link related issues using `Closes #XX`

## Code Standards

- Follow the [Google Go Style Guide](https://google.github.io/styleguide/go/)
- Use `slog` for structured logging
- Wrap errors with `fmt.Errorf("context: %w", err)`
- No globals, no panics, no init() functions
- All exported types and functions must have doc comments

## Architecture

See [docs/TRD.md](docs/TRD.md) for the technical architecture.
See [docs/PRD.md](docs/PRD.md) for product requirements.
