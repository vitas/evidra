# Architecture Alignment Remediation Design

**Date:** 2026-03-09
**Status:** Approved

## Context

The architecture review found that the repository has one strong core path and several misleading edges:

- CLI and MCP implement the evidence -> signals -> score flow and pass tests.
- Self-hosted analytics endpoints exist, but currently return placeholder content instead of real scorecards or explanations.
- `risk_level` semantics in runtime do not match the documented severity-aware model.
- `report` documentation promises immediate signal feedback that the implementation does not return.
- Some user-facing docs and UI copy drift from real flags, score bands, and product scope.

The goal of this design is to make the supported product boundary explicit and bring the runtime contracts back into alignment with the docs.

## Goals

1. Make CLI and MCP the authoritative supported analytics surfaces.
2. Make `risk_level` deterministic, severity-aware, and consistent across runtime and docs.
3. Make `report` return immediate, useful assessment feedback instead of a vague or empty post-write response.
4. Keep self-hosted in the repo, but make its current limitations explicit and honest.
5. Remove or downgrade misleading fields, endpoints, and placeholder claims.

## Non-Goals

1. This work does not implement full server-side analytics parity for self-hosted.
2. This work does not redesign the signal engine or scoring model beyond contract alignment.
3. This work does not add new detectors, new signals, or new hosted features.

## Approved Product Boundary

### Supported

- `evidra` CLI
- `evidra-mcp`
- Local evidence chain
- Local scorecard/explain/report assessment behavior

### Experimental

- `evidra-api`
- Docker Compose self-hosted deployment
- Hosted evidence ingestion and storage

### Not Yet Implemented in Self-Hosted

- Authoritative server-side scorecard computation
- Authoritative server-side explain computation

Self-hosted remains available, but it must stop pretending to support analytics that do not exist yet.

## Decision 1: Severity-Aware Risk Resolution

### Exact rule

`final risk_level = max(matrix(operation_class, scope_class), max BaseSeverity() of fired detectors)`

Severity order:

`low < medium < high < critical`

### Consequences

- A low-severity detector does not automatically inflate a high matrix risk.
- A critical detector can override a medium matrix risk.
- Detector metadata stops being dead configuration.
- Runtime, docs, and future prompt contracts use one exact formula instead of two competing ideas.

### Implementation shape

- Keep the existing `internal/risk` package as the single risk-resolution surface.
- Add a detector metadata lookup by tag from the detector registry.
- Change `ElevateRiskLevel` semantics to compute the max severity instead of "one step if any tag exists".
- Update tests to assert behavior using real registered detector tags, not placeholder strings.

## Decision 2: `report` Returns Immediate Session Assessment

The supported product path is CLI/MCP, and MCP has no dedicated scorecard tool. `report` therefore must do more than append evidence.

### New contract direction

`report` returns:

- existing write/result fields:
  - `ok`
  - `report_id`
  - `prescription_id`
  - `exit_code`
  - `verdict`
- immediate assessment fields:
  - `signal_summary`
  - `score`
  - `score_band`
  - `basis`
  - `confidence`

### Explicit removal

- Remove the fake/unused `signals[]` return shape from MCP `report`.
- Remove `risk_classification` from `run`/`record` output because it has no independent meaning today and currently aliases `score_band`.

### Rationale

- Agents need immediate feedback after execution.
- The engine already exists in the CLI path.
- A session assessment snapshot is clearer than an ambiguous `signals[]` list.
- Pre-release correctness is more important than preserving a misleading contract.

## Decision 3: Shared Assessment Builder

Today, `run` and `record` compute assessments in CLI-only code. `report` and MCP do not.

### New architecture

Create a shared internal package for "assessment from evidence path + session":

- reusable by `run`
- reusable by `record`
- reusable by CLI `report`
- reusable by MCP `report`

This removes duplicated contract logic and prevents future drift between binaries.

## Decision 4: Explicit Experimental Failure for Hosted Analytics

`/v1/evidence/scorecard` and `/v1/evidence/explain` should not return placeholder `200` responses.

### New behavior

- Keep the endpoints.
- Return an explicit experimental/not-implemented error.
- Use `501 Not Implemented`.
- Response body must explain:
  - hosted analytics are experimental
  - ingestion/storage are available
  - CLI/MCP remain the authoritative analytics path
  - where to read current support status

### Required documentation

Create a dedicated document:

- `docs/guides/self-hosted-experimental-status.md`

This doc becomes the canonical status page for:

- what self-hosted supports today
- what is intentionally unavailable
- what users should use instead
- what future parity work would need to deliver

## Decision 5: Documentation and UI Must Match Reality

The documentation sweep is part of the fix, not follow-up cleanup.

### Must be corrected

- `--api-url` -> `--url`
- score bands must match runtime: `excellent`, `good`, `fair`, `poor`, `insufficient_data`
- self-hosted analytics must be marked experimental
- `report` examples must show assessment snapshot behavior, not `signals[]`
- `risk_classification` must be removed from docs/contracts/examples
- architecture docs must stop claiming hosted analytics are implemented

### UI implications

Landing page and static API docs must stop overstating self-hosted readiness.

## Decision 6: Cleanup Misleading Placeholders

The review found dead or misleading placeholders that should not survive this alignment pass.

### Cleanup targets

- blank Terraform Azure/GCP detector packages imported only as placeholders
- architecture claims that imply those domains are implemented when they are not
- placeholder comments in self-hosted analytics wiring

The rule for this pass is simple: if it has no runtime behavior and materially misleads readers, remove it or move it to backlog docs.

## Rollout Order

1. Fix risk semantics first.
2. Extract shared assessment logic.
3. Align CLI/MCP output contracts around the shared assessment.
4. Make hosted analytics explicitly experimental.
5. Sweep docs/UI/OpenAPI.
6. Remove misleading placeholders and backlog-only stubs.

This order minimizes time spent rewriting docs for behavior that has not been corrected yet.

## Acceptance Criteria

The remediation is complete when all of the following are true:

1. Runtime `risk_level` uses detector severity, not the current one-step bump rule.
2. `BaseSeverity()` materially affects real outputs and tests prove it.
3. CLI `report` and MCP `report` return immediate session assessment fields.
4. `run` and `record` no longer emit misleading `risk_classification`.
5. Hosted analytics endpoints return explicit experimental/not-implemented errors.
6. A separate self-hosted status doc exists and is linked from public docs.
7. README, CLI reference, MCP guide, architecture docs, UI copy, and OpenAPI all match the implemented product boundary.
8. Placeholder success responses and dead placeholder imports are removed.

## Verification Strategy

Minimum verification for the implementation plan:

```bash
go test ./internal/risk ./internal/assessment ./cmd/evidra ./pkg/mcpserver ./internal/api ./internal/detectors/... -count=1
go test ./... -count=1
go test -tags e2e ./tests/e2e -count=1 -timeout=120s
make test-signals
```

Doc/UI drift checks:

```bash
rg -n "risk_classification|--api-url|signals\\[\\]|critical < 25|placeholder response until full" README.md docs cmd internal ui
rg -n "experimental" README.md docs/guides/self-hosted-experimental-status.md docs/ARCHITECTURE.md cmd/evidra-api/static/openapi.yaml ui/public/openapi.yaml
```
