# Contributing to Evidra

Thank you for your interest in contributing to Evidra.

## Project scope

Evidra Core is an open-source MCP execution-evidence recorder. One endpoint wraps
one upstream MCP server, enforces protocol order (prescribe before execution,
report after), and writes a signed evidence chain that a human reconciles
afterwards. It records and verifies evidence; it is not a scoring platform, not
an enforcement gateway, and not a generic multi-server proxy.

Changes are welcome when they make the recorded evidence more honest or the
protocol easier to follow: the envelope, digest and signature rules, the
read-side reconciliation, the conformance fixture, and the measurement harness.
Feature requests outside that scope are more useful as an issue than as a PR,
because they change what the project claims.

## Development setup

```bash
git clone https://github.com/vitas/evidra.git
cd evidra
make build
make test
```

Requires Go 1.26+.

## Repository map

- `cmd/evidra-mcp/` — the merged endpoint: the only binary that enforces or records.
- `cmd/evidra/` — read side only: `summarize`, `verify`, `version`.
- `pkg/proxy/` — the endpoint implementation; `pkg/evidence/` — the v2 evidence model.
- `pkg/report/` — reconciliation into `summary.json`.
- `cmd/evidra-fixture/`, `cmd/evidra-gatea/` — the conformance upstream and the
  experiment harness. Not shipped product, but not disposable either: they
  produce the evidence the claims in `docs/validation.md` rest on.
- `tests/` — shell guards; `tests/run_guards.sh` runs every one of them.
- `docs/` — five canonical documents plus the `docs/plans/` corpus (see
  Documentation rules).
- `ui/` — the static landing surface. Nothing in the evidence path imports it.

## Build and test

```bash
make build
make test
make lint
go vet ./...
go test -race ./pkg/evidence/... ./pkg/proxy/... ./pkg/report/... ./cmd/evidra-fixture/... ./cmd/evidra-gatea/...
bash tests/run_guards.sh
cd ui && npm ci && npm run lint && npm test && npm run build
```

`make test` expands to `go test ./cmd/... ./pkg/...`, keeping installed UI
dependencies outside the Go package traversal.

A single Go case: `go test -run 'TestName' -v ./pkg/proxy/`.

Guards and CI reference things that must exist — make targets, package paths,
workflow steps, tests cited by name in gate artifacts. Renaming or deleting one
means updating every citation to it in the same commit.

## Documentation rules

- One topic owns one page. `docs/getting-started.md`, `docs/cli-reference.md`,
  `docs/architecture.md`, `docs/evidence-format.md`, and `docs/validation.md`
  are the canonical set; they must not overlap or contradict each other, and
  the repository guard on their inventory enforces exactly that.
- Every public claim needs a check. Changing what Evidra says it does means
  changing the guard or test that enforces the claim in the same commit — a
  green check that asserts the old sentence is worse than a red one.
- Everything tracked is English: code comments, commit messages, documentation,
  and reports about the work.

## Pull requests

1. Fork the repo and create a feature branch.
2. Write tests for new functionality; extend guards before docs to prove the
   docs are what failed.
3. Ensure the Build and test commands above pass.
4. Sign every commit with the Developer Certificate of Origin (DCO).
5. Open a PR with a clear description of what and why.

## DCO

Evidra uses the Developer Certificate of Origin instead of a CLA.

Every commit merged into the project must include a `Signed-off-by:` trailer:

```text
Signed-off-by: Your Name <you@example.com>
```

The easiest way to do this is:

```bash
git commit -s
```

GitHub Actions verifies sign-offs on pull requests and pushes to `main`. By
contributing, you agree that your contributions will be licensed under the
Apache License 2.0.

## Reporting issues

Open a GitHub issue with:
- What you expected
- What happened
- Steps to reproduce
- Evidra version (`evidra version`)

For security matters, see [SECURITY.md](SECURITY.md) instead of opening a public
issue.
