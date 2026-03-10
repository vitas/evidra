# Contributing to Evidra

Thank you for your interest in contributing to Evidra.

## Development Setup

```bash
git clone https://github.com/vitas/evidra.git
cd evidra
make build
make test
```

Requires Go 1.24+.

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

## Pull Requests

1. Fork the repo and create a feature branch.
2. Write tests for new functionality.
3. Ensure `make test && make lint` pass.
4. Sign every commit with the Developer Certificate of Origin (DCO).
5. Open a PR with a clear description of what and why.

## Developer Certificate Of Origin (DCO)

Evidra uses the Developer Certificate of Origin instead of a CLA.

Every commit merged into the project must include a `Signed-off-by:` trailer:

```text
Signed-off-by: Your Name <you@example.com>
```

The easiest way to do this is:

```bash
git commit -s
```

GitHub Actions verifies sign-offs on pull requests and pushes to `main`.

## Reporting Issues

Open a GitHub issue with:
- What you expected
- What happened
- Steps to reproduce
- Evidra version (`evidra version`)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
