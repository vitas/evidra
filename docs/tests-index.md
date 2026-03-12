# Tests Index

Short map of the main test folders in Evidra. Use this first, then go to
[E2E_TESTING.md](E2E_TESTING.md) for the detailed suite breakdown.

| Path | Purpose | Primary Data | Notes |
| --- | --- | --- | --- |
| `tests/canon_fixtures/` | Canonicalization fixture corpus for `internal/canon` | Frozen manifest/plan inputs plus expected digest files | Input fixtures, not end-to-end tests |
| `tests/e2e/` | Authoritative real-world acceptance | Promoted OSS corpus fixtures plus curated acceptance artifacts | Top-level product acceptance layer |
| `tests/contracts/` | Synthetic contract validation | Handwritten fixtures and temp evidence stores | Keeps CLI/MCP/output contracts deterministic |
| `tests/inspector/` | MCP inspector and transport checks | Inspector fixtures and transport cases | Focused on protocol/runtime transport behavior |
| `tests/benchmark/` | Dataset integrity and benchmark validation | Vendored OSS corpus, case metadata, contract snapshots | Authoritative for benchmark dataset health |
| `tests/signal-validation/` | Behavioral signal calibration | Scripted local evidence sequences | Validates score/signal differentiation, not live infra |
| `tests/experiments/` | Research and experiment harness support | Experimental run configs and helper scripts | Not the authoritative acceptance layer |
| `tests/testdata/` | Shared low-level fixtures | Small parser/unit support files | Used by package tests |
| `cmd/`, `internal/`, `pkg/` test files | Narrow package behavior | Temp files and local fixtures | Unit and command-local verification |

Related docs:
- [E2E Testing Map](E2E_TESTING.md)
- [Acceptance Fixture Status](guides/acceptance-fixture-status.md)
- [Shared Artifact Fixtures](../tests/artifacts/fixtures/README.md)
