# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build                              # build bin/evidra, bin/evidra-mcp, bin/evidra-api
make test                               # go test ./... -v -count=1
make fmt                                # gofmt -w .
make lint                               # golangci-lint run
make tidy                               # go mod tidy
make canon-fixtures-update              # regenerate canon golden test files
make docker-mcp                         # build MCP server Docker image
make docker-cli                         # build CLI Docker image
make docker-api                         # build API server Docker image

go test -run TestFunctionName ./internal/canon/...   # run a single test
go test -race ./...                                  # race detector
```

## Git
Alsways ask to push to remote repo
Always use Signed-off-by to make a git commits


## Architecture

Evidra is a **flight recorder for infrastructure automation** — it observes and measures AI agent and CI pipeline reliability without blocking operations.

**Module:** `samebits.com/evidra` (Go 1.24)

### Binaries

- `cmd/evidra/` — CLI for prescribe, report, scorecard, explain, compare, record, import, export, validate, import-findings, prompts, detectors, keygen, skill, and version commands
- `cmd/evidra-mcp/` — MCP server with two modes: direct (agent calls prescribe/report) and proxy (wraps upstream MCP server, auto-records mutations)
- `cmd/evidra-api/` — Self-hosted API server with embedded UI, webhooks, and database-backed storage

### Core pipeline

```
raw artifact → canonicalize (adapter) → CanonicalAction + digests
                                              │
                                    assess.Pipeline [Assessor 1..N]
                                              │
                                    risk_inputs[] + effective_risk → Prescription
                                                                          ↓
exit code + prescription_id → Report → signal detectors → Scorecard
```

### Key packages

**Core pipeline:**

- **`internal/canon/`** — Canonicalization layer. Adapters (`K8sAdapter`, `TerraformAdapter`, `DockerAdapter`, `GenericAdapter`) translate raw artifacts into `CanonicalAction` — Evidra's protocol language. Output consumed by both the assessment pipeline and evidence entry construction.
- **`internal/assess/`** — Pluggable assessment pipeline. Runs `Assessor` implementations against a `CanonicalAction` and aggregates `risk_inputs[]` into `effective_risk`. Used by both `lifecycle` (CLI/MCP) and `ingest` (API) prescribe paths.
- **`internal/risk/`** — Risk matrix and severity comparison. `riskMatrix` maps `operationClass × scopeClass → riskLevel`. Used by `MatrixAssessor`.
- **`internal/detectors/`** — Tag detectors that pattern-match misconfigurations (privileged containers, wildcard RBAC, etc.). Used by `DetectorAssessor`.
- **`internal/signal/`** — Eight behavioral signal detectors: protocol violation, artifact drift, retry loop, blast radius, new scope, repair loop, thrashing, risk escalation. Post-hoc intelligence on evidence sequences.
- **`internal/score/`** — Weighted penalty scoring (`score = 100 × (1 - penalty)`), workload profile comparison.
- **`internal/lifecycle/`** — Core service for prescribe/report operations and evidence entry lifecycle. Delegates assessment to `internal/assess/` pipeline.
- **`internal/pipeline/`** — Converts evidence entries to signal detector input by extracting prescriptions and reports.
- **`internal/analytics/`** — Generates scorecard outputs with signal counts, rates, and weighted scoring.

**Evidence & signing:**

- **`pkg/evidence/`** — Evidence chain persistence (file-based, append-only segments with manifest and locking).
- **`internal/evidence/`** — Ed25519 signer and signing payload construction.
- **`pkg/evlock/`** — Cross-platform file locking for evidence store access.

**MCP & API:**

- **`pkg/mcpserver/`** — MCP server implementation. Tools: `prescribe_full`, `prescribe_smart`, `report`, `get_event`, `run_command`, `collect_diagnostics`, `write_file`, `describe_tool`. JSON schemas embedded from `pkg/mcpserver/schemas/`.
- **`pkg/proxy/`** — MCP stdio proxy: mutation detection, JSON-RPC interception, evidence auto-recording
- **`internal/api/`** — HTTP API router and handlers for entries, scorecards, webhooks, and auth.
- **`internal/auth/`** — Authentication middleware for API keys and tenant context.
- **`internal/store/`** — Database store for entries and API keys.
- **`internal/db/`** — PostgreSQL connection pooling and schema migration.
- **`internal/analyticsdb/`** — Decodes stored JSON payloads from database rows for analytics replay.
- **`pkg/client/`** — HTTP client for Evidra API with retry and error handling.
- **`pkg/mode/`** — Resolves operating mode (online/offline/fallback) for CLI and MCP server.

**Ingestion & assessment:**

- **`internal/assessment/`** — Computes assessment snapshots with scores, signal summaries, and sufficiency checks.
- **`internal/automationevent/`** — Defines v1 ingestion contract for completed automation executions.
- **`internal/sarif/`** — Parses SARIF security analysis reports into evidence format.

**Prompts:**

- **`internal/promptfactory/`** — Loads prompt bundles from the source contract tree (`CONTRACT`, `CLASSIFICATION`) and generates the active MCP server, MCP prompt-reference, and skill outputs.

**Infra:**

- **`internal/config/`** — Configuration resolution for signing mode, evidence write mode, and metrics.
- **`internal/telemetry/`** — Metrics transport and labels for OTLP HTTP export.
- **`pkg/version/`** — Build version, spec version, and scoring version constants.

### Architecture reference

`docs/ARCHITECTURE.md` is the **single architecture reference**.
It consolidates key decisions, invariants, and known gaps from the former review and recommendation docs (now archived in `docs/plans/done/`).

### Conventions

- No web frameworks — stdlib `net/http` only.
- IDs generated with `github.com/oklog/ulid/v2`.
- `internal/` for server-only code, `pkg/` for shared code.
- Canon golden test pattern: fixtures in `tests/canon_fixtures/`, update with `EVIDRA_UPDATE_CANON_FIXTURES=1`.
- Canonicalization adapters implement the `canon.Adapter` interface.

### Environment variables

- `EVIDRA_EVIDENCE_DIR` — evidence storage directory (default: `~/.evidra/evidence`)
- `EVIDRA_ENVIRONMENT` — environment label (MCP server only)
- `EVIDRA_RETRY_TRACKER` — enable retry loop tracking (MCP server only)

## API Changes — Mandatory Checklist

When adding, modifying, or removing any REST API endpoint, ALL of the following must be updated:

### Step 1: Implementation
- Handler in `internal/api/`
- Route registration in `RegisterRoutes`
- Repository method and query implementation when persistence changes
- Types in the appropriate package

### Step 2: Tests
- Handler test — at minimum: happy path + error case
- Update ALL fake/mock repos that implement the Repository interface (there are multiple in tests)
- Run the relevant package tests, usually `go test ./internal/api/... -v -count=1`

### Step 3: OpenAPI Specification
- Update `cmd/evidra-api/static/openapi.yaml` — full endpoint definition with:
  - Path, method, summary, tags
  - Parameters (query, path) with types and descriptions
  - Request body schema (for POST/PUT)
  - Response schema with example
  - Security requirements (bearerAuth if authenticated)
- Copy to `ui/public/openapi.yaml`: `cp cmd/evidra-api/static/openapi.yaml ui/public/openapi.yaml`
- Verify YAML is valid: `python3 -c "import yaml; yaml.safe_load(open('cmd/evidra-api/static/openapi.yaml'))"`

### Step 4: API Reference Documentation
- Update `docs/api-reference.md` — human-readable docs with:
  - Endpoint path and method
  - Query parameters and request body
  - Example response JSON
  - Notes on behavior and edge cases

### Step 5: Changelog
- Add entry under `## Unreleased` in `CHANGELOG.md`

### Step 6: Database Migrations (if needed)
- New migration in `internal/db/migrations/NNN_description.up.sql`
- Keep migrations additive (ADD COLUMN, not DROP)

### Step 7: Version Bump and Release
- Run `./scripts/bump-version.sh X.Y.Z`
- Commit, tag, push: `git tag -a vX.Y.Z -m "description" && git push origin main vX.Y.Z`
- Wait for release pipeline to build Docker images
- Deploy: `gh workflow run deploy.yml --repo vitas/evidra-infra`

### Common mistakes to avoid
- Adding a method to the Repository interface without updating ALL test fakes (there are 4+)
- Forgetting to copy openapi.yaml to ui/public/
- Using nullable arrays in PostgreSQL queries — always coalesce nil slices to empty `[]`
- Not running `gofmt -w .` after changes
