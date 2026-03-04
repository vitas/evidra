# Chief Architect Review: System Design Alignment and Reuse Map

Date: 2026-03-04  
Scope: `docs/system-design/` (source of truth) + current repo code + `../evidra-mcp`

## Executive Verdict

The architecture in `docs/system-design/` is strong and coherent, but implementation is only partially synchronized. The repo has valuable reused infrastructure from `evidra-mcp`, yet benchmark semantics are not fully contract-conformant.

## Critical Gaps

1. Evidence schema mismatch.
- Design requires `EvidenceEntry` envelope (`entry_id`, `type`, `trace_id`, version fields).
- Code still persists legacy policy-era record shape (`PolicyDecision`, `policy_ref`, `bundle_revision`) in [pkg/evidence/types.go](/Users/vitas/git/evidra-benchmark/pkg/evidence/types.go).

2. Scorecard is not evidence-driven.
- `evidra scorecard` computes from empty input (`signal.AllSignals(nil)`) in [cmd/evidra/main.go:67](/Users/vitas/git/evidra-benchmark/cmd/evidra/main.go:67).

3. Canonicalization contract drift.
- Scope class is topology (`single|namespace|cluster`) in [internal/canon/types.go:143](/Users/vitas/git/evidra-benchmark/internal/canon/types.go:143), not environment scope.
- `intent_digest` currently includes `resource_shape_hash` via full action marshal in [internal/canon/k8s.go:92](/Users/vitas/git/evidra-benchmark/internal/canon/k8s.go:92), [internal/canon/terraform.go:96](/Users/vitas/git/evidra-benchmark/internal/canon/terraform.go:96).

4. Signal logic/spec mismatch.
- Retry window 10m in [internal/signal/retry_loop.go:14](/Users/vitas/git/evidra-benchmark/internal/signal/retry_loop.go:14) vs spec 30m.
- Blast thresholds in [internal/signal/blast_radius.go:5](/Users/vitas/git/evidra-benchmark/internal/signal/blast_radius.go:5) differ from spec destructive-only threshold.
- New scope key in [internal/signal/new_scope.go:6](/Users/vitas/git/evidra-benchmark/internal/signal/new_scope.go:6) misses actor+scope dimensions.

5. Prescribe/report lifecycle fields incomplete.
- Report path lacks full actor/trace semantics for strict protocol checks in [pkg/mcpserver/server.go:304](/Users/vitas/git/evidra-benchmark/pkg/mcpserver/server.go:304), [pkg/mcpserver/server.go:337](/Users/vitas/git/evidra-benchmark/pkg/mcpserver/server.go:337).
- `ttl_ms`, `finding`, and `canonicalization_failure` entry logic is not implemented.

## High Gaps

1. Risk taxonomy mismatch.
- Matrix in [internal/risk/matrix.go](/Users/vitas/git/evidra-benchmark/internal/risk/matrix.go) uses topology-like scope classes instead of environment scope classes.

2. Digest format inconsistency.
- Design examples use `sha256:` style; code emits raw hex digests in [internal/canon/generic.go:51](/Users/vitas/git/evidra-benchmark/internal/canon/generic.go:51).

3. Forwarding surface exposed but not wired.
- CLI flags/env exist in [cmd/evidra-mcp/main.go:29](/Users/vitas/git/evidra-benchmark/cmd/evidra-mcp/main.go:29), [cmd/evidra-mcp/main.go:97](/Users/vitas/git/evidra-benchmark/cmd/evidra-mcp/main.go:97), but runtime forward path is not implemented in [pkg/mcpserver/server.go](/Users/vitas/git/evidra-benchmark/pkg/mcpserver/server.go).

## Medium Gaps

1. Confidence model not implemented in scoring.
- [internal/score/score.go](/Users/vitas/git/evidra-benchmark/internal/score/score.go) only computes weighted penalty, no trust ceiling logic.

2. Evidence subsystem test depth regressed.
- `evidra-mcp` has stronger evidence tests not ported to this repo.

3. Terminology drift across docs.
- Enum naming differs across documents (`mutate` vs `mutating`, etc.), causing implementation drift risk.

## Reuse Map from `evidra-mcp`

### Keep and adapt

1. `pkg/evidence/*` mechanics (segmenting, locking, chain validation).
2. `pkg/evlock/*`.
3. `pkg/mcpserver/*` shell/tool wiring patterns.
4. `internal/risk/*` detector implementation style.

### Reused but not synchronized

1. [pkg/evidence/types.go](/Users/vitas/git/evidra-benchmark/pkg/evidence/types.go)
2. [pkg/mcpserver/server.go](/Users/vitas/git/evidra-benchmark/pkg/mcpserver/server.go)
3. [internal/canon](/Users/vitas/git/evidra-benchmark/internal/canon)
4. [internal/signal](/Users/vitas/git/evidra-benchmark/internal/signal)
5. [cmd/evidra/main.go](/Users/vitas/git/evidra-benchmark/cmd/evidra/main.go)

### Recommended reuse next (with adaptation)

1. `../evidra-mcp/internal/api/*` middleware/response/health primitives.
2. `../evidra-mcp/internal/auth/*`.
3. `../evidra-mcp/internal/store/keys.go`.
4. `../evidra-mcp/internal/db/*`.
5. `../evidra-mcp/cmd/evidra-api/main.go` bootstrap structure.
6. `../evidra-mcp/pkg/evidence/*_test.go` and `../evidra-mcp/cmd/evidra-mcp/test/*`.

### Do not reuse

1. `pkg/policy/*`
2. `pkg/runtime/*`
3. `pkg/validate/*`
4. `pkg/bundlesource/*`
5. `pkg/policysource/*`
6. `internal/engine/*`
7. Any `validate`-first semantics

## Recommendation Plan

### P0: Contract Conformance

1. Freeze one canonical enum set in docs, then enforce in code/schemas.
2. Implement unified `EvidenceEntry` envelope and migrate reader/writer logic.
3. Wire real scorecard pipeline: evidence -> signal entries -> score.
4. Fix canonicalization scope mapping and intent digest composition.
5. Align signal defaults/algorithms to `EVIDRA_SIGNAL_SPEC.md`.

### P1: Trust and Completeness

1. Add `trace_id`, `ttl_ms`, `canon_source`, and required version fields.
2. Add `canonicalization_failure` and `finding` entry support.
3. Implement confidence model and score ceilings.
4. Port/adapt evidence and MCP integration tests from `evidra-mcp`.

### P2: Platform Layer (v0.5+)

1. Reuse API/auth/db/store scaffolding from `evidra-mcp`.
2. Replace legacy `/v1/validate` with benchmark protocol endpoints.
3. Implement forward path and metrics per signal spec.

## Immediate PR Sequence

1. PR-1: EvidenceEntry schema migration.
2. PR-2: Canonicalization and risk taxonomy sync.
3. PR-3: Real scorecard pipeline and confidence output.
4. PR-4: Signal detector spec conformance.
5. PR-5: Port critical evidence/MCP tests from `evidra-mcp`.

## Final Position

Use `evidra-mcp` as infrastructure primitive source, not semantic authority.  
System design docs must remain the contract authority; reused modules must be refactored to match.
