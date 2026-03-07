# Contributing to Evidra

Thank you for your interest in contributing to Evidra.

## Development Setup

```bash
git clone https://github.com/vitas/evidra.git
cd evidra
make build
make test
```

Requires Go 1.23+.

## Running Tests

```bash
make test          # unit tests
make lint          # golangci-lint
make fmt           # gofmt
make e2e           # end-to-end tests (requires build)
make test-signals  # signal validation scenarios
```

## Code Style

- Go stdlib conventions. No web frameworks.
- `gofmt -w .` before every commit.
- Error wrapping: `fmt.Errorf("context: %w", err)`.
- See `CLAUDE.md` for full conventions.

## Pull Requests

1. Fork the repo and create a feature branch.
2. Write tests for new functionality.
3. Ensure `make test && make lint` pass.
4. Open a PR with a clear description of what and why.

## Reporting Issues

Open a GitHub issue with:
- What you expected
- What happened
- Steps to reproduce
- Evidra version (`evidra version`)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
