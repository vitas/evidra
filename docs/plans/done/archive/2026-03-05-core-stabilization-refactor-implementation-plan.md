# Core Stabilization Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Stabilize Evidra core contract and runtime behavior (signing, session/trace integrity, transport parity) so benchmark dataset work can proceed on a reliable base.

**Architecture:** Introduce a shared lifecycle service used by both CLI and MCP transports, remove mutable per-process request state, and enforce protocol invariants at write time. Keep behavior changes gated with focused tests and incremental commits, then align product/system docs with actual runtime semantics.

**Tech Stack:** Go 1.24, `go test`, MCP Inspector CLI tests, GitHub Actions CI, Markdown docs in `docs/plans` and `docs/system-design`.

---

### Task 1: Freeze Baseline and Add Regression Repro

**Files:**
- Create: `tests/e2e/inspector_smoke_test.go`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Test: `tests/inspector/run_inspector_tests.sh`

**Step 1: Write the failing test**

Create `tests/e2e/inspector_smoke_test.go` with a single test that:
- runs `make test-mcp-inspector` via `exec.Command`
- expects zero exit code
- prints captured stderr/stdout on failure

**Step 2: Run test to verify it fails**

Run: `go test -tags e2e ./tests/e2e -run InspectorSmoke -v`  
Expected: FAIL because local inspector currently cannot start `evidra-mcp` without signing key.

**Step 3: Wire into CI as non-blocking baseline (temporary)**

In `.github/workflows/ci.yml`, add a step in `e2e` job:
- run `make test-mcp-inspector`
- `continue-on-error: true`
- upload output artifact for diagnosis

In `Makefile`, add:
- `test-mcp-inspector-ci` target that logs output to `tests/inspector/out/latest.log`

**Step 4: Run test to verify instrumentation works**

Run: `make test-mcp-inspector-ci`  
Expected: current failure reproduced with log artifact path.

**Step 5: Commit**

```bash
git add tests/e2e/inspector_smoke_test.go Makefile .github/workflows/ci.yml
git commit -m "test: capture inspector baseline failure in ci"
```

### Task 2: Introduce Explicit Signing Mode (Strict vs Optional)

**Files:**
- Create: `internal/config/signing.go`
- Modify: `cmd/evidra/main.go`
- Modify: `cmd/evidra-mcp/main.go`
- Modify: `tests/inspector/mcp-config.json`
- Modify: `tests/inspector/run_inspector_tests.sh`
- Test: `cmd/evidra/main_test.go`
- Test: `pkg/mcpserver/e2e_test.go`

**Step 1: Write the failing tests**

Add tests for both binaries:
- default startup in local test mode works without signing key when `EVIDRA_SIGNING_MODE=optional`
- strict mode fails without key
- strict mode succeeds with key

**Step 2: Run tests to verify failure**

Run: `go test ./cmd/evidra ./pkg/mcpserver -run SigningMode -v`  
Expected: FAIL (mode not implemented).

**Step 3: Write minimal implementation**

Create `internal/config/signing.go`:
- `type SigningMode string` with `strict`, `optional`
- parser from env/flag with default `strict` for production, `optional` for explicit local test env

Update `resolveSigner` logic in CLI and MCP:
- if strict and no key: error
- if optional and no key: use no-op signer implementation that leaves `signature` empty

Update inspector config/script:
- set `EVIDRA_SIGNING_MODE=optional` for local-mcp baseline

**Step 4: Run tests to verify pass**

Run:
- `go test ./cmd/evidra ./pkg/mcpserver -run SigningMode -v`
- `make test-mcp-inspector`

Expected: PASS in local inspector mode.

**Step 5: Commit**

```bash
git add internal/config/signing.go cmd/evidra/main.go cmd/evidra-mcp/main.go tests/inspector/mcp-config.json tests/inspector/run_inspector_tests.sh cmd/evidra/main_test.go pkg/mcpserver/e2e_test.go
git commit -m "feat: add explicit signing mode with local optional path"
```

### Task 3: Extract Shared Lifecycle Service (Single Source of Truth)

**Files:**
- Create: `internal/lifecycle/service.go`
- Create: `internal/lifecycle/types.go`
- Create: `internal/lifecycle/service_test.go`
- Modify: `cmd/evidra/main.go`
- Modify: `pkg/mcpserver/server.go`
- Test: `cmd/evidra/main_test.go`
- Test: `pkg/mcpserver/integration_test.go`

**Step 1: Write the failing test**

In `internal/lifecycle/service_test.go`, add table tests for:
- prescribe parse success/failure
- report with unknown prescription
- report with matched prescription
- canonical-action path with tool/operation normalization

**Step 2: Run test to verify it fails**

Run: `go test ./internal/lifecycle -v`  
Expected: FAIL (package not present).

**Step 3: Write minimal implementation**

Create shared service methods:
- `Prescribe(ctx, input) (output, error)`
- `Report(ctx, input) (output, error)`
- uses existing canon/risk/evidence packages
- returns typed errors used by CLI and MCP adapters

Refactor CLI and MCP handlers to call shared service instead of owning business logic.

**Step 4: Run tests to verify pass**

Run:
- `go test ./internal/lifecycle ./cmd/evidra ./pkg/mcpserver -v`
- `go test ./...`

Expected: all pass, no behavior regressions.

**Step 5: Commit**

```bash
git add internal/lifecycle cmd/evidra/main.go pkg/mcpserver/server.go cmd/evidra/main_test.go pkg/mcpserver/integration_test.go
git commit -m "refactor: unify prescribe/report logic behind shared lifecycle service"
```

### Task 4: Remove Cross-Request Mutable State in MCP Service

**Files:**
- Modify: `pkg/mcpserver/server.go`
- Create: `pkg/mcpserver/correlation.go`
- Create: `pkg/mcpserver/correlation_test.go`
- Test: `pkg/mcpserver/e2e_test.go`

**Step 1: Write the failing test**

Add concurrent test:
- two prescribe calls from different actors/sessions
- out-of-order report calls
- assert each report uses its own actor/trace/session linkage

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/mcpserver -run ConcurrentReportCorrelation -v`  
Expected: FAIL with cross-call contamination.

**Step 3: Write minimal implementation**

Replace `lastActor`/`lastTraceID` state with deterministic lookup:
- resolve correlation fields from referenced prescription entry
- explicit input overrides allowed
- no mutable shared actor/trace fields in service struct

**Step 4: Run tests to verify pass**

Run: `go test ./pkg/mcpserver -v`  
Expected: PASS including new concurrent test.

**Step 5: Commit**

```bash
git add pkg/mcpserver/server.go pkg/mcpserver/correlation.go pkg/mcpserver/correlation_test.go pkg/mcpserver/e2e_test.go
git commit -m "fix: remove shared mutable correlation state from mcp service"
```

### Task 5: Enforce Protocol Session and Trace Invariants

**Files:**
- Modify: `internal/lifecycle/service.go`
- Modify: `cmd/evidra/main.go`
- Modify: `pkg/mcpserver/server.go`
- Create: `internal/lifecycle/invariants_test.go`
- Modify: `docs/system-design/EVIDRA_PROTOCOL.md`

**Step 1: Write the failing tests**

Add tests for:
- report without session_id derives from prescription session (not new generated value)
- signal/canonicalization-failure entries inherit originating session/trace where available
- invalid report session mismatch returns validation error

**Step 2: Run tests to verify failure**

Run: `go test ./internal/lifecycle ./cmd/evidra ./pkg/mcpserver -run Session -v`  
Expected: FAIL with current behavior.

**Step 3: Write minimal implementation**

Implement invariant rules:
- every written entry must have session linkage (or explicit documented exception with code path removed)
- correlation fields resolved from prescription for report
- unknown prescription error path writes signal entry with current report session if provided

**Step 4: Run tests to verify pass**

Run:
- `go test ./internal/lifecycle ./cmd/evidra ./pkg/mcpserver -v`
- `go test ./...`

Expected: PASS with invariant enforcement.

**Step 5: Commit**

```bash
git add internal/lifecycle/service.go internal/lifecycle/invariants_test.go cmd/evidra/main.go pkg/mcpserver/server.go docs/system-design/EVIDRA_PROTOCOL.md
git commit -m "fix: enforce session and trace invariants across evidence entries"
```

### Task 6: Align Risk Payload Contract (`risk_tags` vs `risk_details`)

**Files:**
- Modify: `pkg/evidence/payloads.go`
- Modify: `internal/pipeline/bridge.go`
- Modify: `cmd/evidra/main.go`
- Modify: `pkg/mcpserver/server.go`
- Create: `internal/pipeline/bridge_contract_test.go`
- Modify: `docs/plans/2026-03-05-benchmark-dataset-proposal.md`
- Modify: `docs/system-design/EVIDRA_SIGNAL_SPEC.md`

**Step 1: Write the failing test**

Contract tests:
- prescription payload contains canonical risk field(s) expected by benchmark validator
- pipeline reads same field(s) and exposes them consistently

**Step 2: Run tests to verify failure**

Run: `go test ./internal/pipeline -run RiskContract -v`  
Expected: FAIL (contract mismatch).

**Step 3: Write minimal implementation**

Choose one contract:
- canonical: `risk_details`
- compatibility: keep reading legacy `risk_tags` during transition

Implement:
- dual-write or mapping in payload builder
- dual-read in pipeline bridge
- deprecation comment with removal target version

Update benchmark proposal validation language to match runtime contract.

**Step 4: Run tests to verify pass**

Run: `go test ./internal/pipeline ./cmd/evidra ./pkg/mcpserver -v`  
Expected: PASS with backward compatibility.

**Step 5: Commit**

```bash
git add pkg/evidence/payloads.go internal/pipeline/bridge.go cmd/evidra/main.go pkg/mcpserver/server.go internal/pipeline/bridge_contract_test.go docs/plans/2026-03-05-benchmark-dataset-proposal.md docs/system-design/EVIDRA_SIGNAL_SPEC.md
git commit -m "refactor: align risk payload contract with benchmark validator"
```

### Task 7: Scope Vocabulary Normalization

**Files:**
- Modify: `internal/canon/types.go`
- Modify: `internal/risk/matrix.go`
- Create: `internal/canon/scope_vocab_test.go`
- Modify: `docs/system-design/EVIDRA_PROTOCOL.md`

**Step 1: Write the failing test**

Add tests for accepted scope aliases:
- `prod` -> `production`
- `dev` -> `development`
- `test` and `sandbox` mapping policy defined and asserted

**Step 2: Run test to verify failure**

Run: `go test ./internal/canon -run ScopeVocab -v`  
Expected: FAIL (aliases unsupported).

**Step 3: Write minimal implementation**

Implement normalization function used by:
- `ResolveScopeClass`
- risk matrix lookup path

Preserve existing behavior while accepting protocol aliases.

**Step 4: Run test to verify pass**

Run: `go test ./internal/canon ./internal/risk -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/canon/types.go internal/risk/matrix.go internal/canon/scope_vocab_test.go docs/system-design/EVIDRA_PROTOCOL.md
git commit -m "fix: normalize protocol and runtime scope vocabulary"
```

### Task 8: Implement `evidra benchmark` Stub and Feature Gate

**Files:**
- Create: `cmd/evidra/benchmark.go`
- Modify: `cmd/evidra/main.go`
- Create: `cmd/evidra/benchmark_test.go`
- Modify: `docs/system-design/EVIDRA_BENCHMARK_CLI.md`
- Modify: `docs/plans/2026-03-05-benchmark-dataset-proposal.md`

**Step 1: Write the failing test**

Add CLI tests:
- `evidra benchmark --help` available
- `evidra benchmark run` returns clear “not yet implemented” with roadmap link when no dataset engine wired

**Step 2: Run test to verify failure**

Run: `go test ./cmd/evidra -run BenchmarkCommand -v`  
Expected: FAIL (`benchmark` command missing).

**Step 3: Write minimal implementation**

Add subcommand shell:
- `benchmark run|list|validate|record|compare|version`
- no-op stubs with deterministic exit codes/messages
- feature gate env var for partial rollout

Update docs to reflect current stage (implemented stub vs full execution).

**Step 4: Run tests to verify pass**

Run: `go test ./cmd/evidra -v`  
Expected: PASS with explicit command contract.

**Step 5: Commit**

```bash
git add cmd/evidra/benchmark.go cmd/evidra/main.go cmd/evidra/benchmark_test.go docs/system-design/EVIDRA_BENCHMARK_CLI.md docs/plans/2026-03-05-benchmark-dataset-proposal.md
git commit -m "feat: add benchmark command scaffold with explicit feature gating"
```

### Task 9: Documentation and Actionable Operator Paths

**Files:**
- Modify: `README.md`
- Modify: `docs/integrations/SCANNER_SARIF_QUICKSTART.md`
- Modify: `server.json`
- Modify: `tests/inspector/README.md`

**Step 1: Write the failing doc check**

Create a lightweight check script `scripts/check-doc-commands.sh` that asserts:
- commands in README quickstart are runnable with current defaults
- required env vars documented for MCP and inspector modes

**Step 2: Run check to verify failure**

Run: `bash scripts/check-doc-commands.sh`  
Expected: FAIL on missing signing-mode/env guidance.

**Step 3: Write minimal documentation updates**

Update docs:
- strict vs optional signing modes
- local inspector quickstart variables
- MCP package env variables in `server.json`
- scanner quickstart commands including signing guidance

**Step 4: Run check to verify pass**

Run:
- `bash scripts/check-doc-commands.sh`
- `make test-mcp-inspector`

Expected: PASS for local docs path.

**Step 5: Commit**

```bash
git add README.md docs/integrations/SCANNER_SARIF_QUICKSTART.md server.json tests/inspector/README.md scripts/check-doc-commands.sh
git commit -m "docs: align operator docs with enforced runtime contract"
```

### Task 10: Turn Inspector and Composition Gates Fully Blocking

**Files:**
- Modify: `.github/workflows/ci.yml`
- Create: `tests/benchmark/scripts/validate-source-composition.sh`
- Modify: `docs/plans/2026-03-05-benchmark-dataset-proposal.md`

**Step 1: Write the failing gate test**

Add CI dry-run script tests:
- `validate-source-composition.sh` fails without dataset manifests/thresholds
- inspector job failure fails workflow once signing/config refactor is merged

**Step 2: Run test to verify failure**

Run: `bash tests/benchmark/scripts/validate-source-composition.sh`  
Expected: FAIL until dataset scaffolding exists.

**Step 3: Write minimal implementation**

Implement composition validator:
- counts case `source_refs`
- resolves source manifests
- enforces `>=80%` real-derived, `<=20%` custom-only

Update CI:
- make `test-mcp-inspector` blocking
- add composition validation job as blocking only when `tests/benchmark/cases` exists

**Step 4: Run tests to verify pass**

Run:
- `make test-mcp-inspector`
- `go test ./...`
- `bash tests/benchmark/scripts/validate-source-composition.sh` (or skip message if no cases yet)

Expected: PASS or deterministic skip in empty-dataset state.

**Step 5: Commit**

```bash
git add .github/workflows/ci.yml tests/benchmark/scripts/validate-source-composition.sh docs/plans/2026-03-05-benchmark-dataset-proposal.md
git commit -m "ci: enforce inspector and source-composition quality gates"
```

---

## Execution Order and Risk Controls

1. Task 1-2 first: restore local operability and deterministic baseline.
2. Task 3-5 second: remove architectural duplication and enforce protocol invariants.
3. Task 6-7 third: close contract mismatches (`risk_details`, scope vocabulary).
4. Task 8 last before dataset engine: expose stable CLI surface without false claims.
5. Task 9-10 final: lock docs and CI to prevent regression.

Rollback approach per task:
- keep commits atomic (one task = one commit)
- revert single task commit if integration fails
- rerun `go test ./...`, `make e2e`, `make test-mcp-inspector` after every task

---

## Definition of Done

- `go test ./...` passes
- `make e2e` passes
- `make test-mcp-inspector` passes in `local-mcp` and `local-rest` (with local API URL)
- signing behavior is explicit and documented (`strict`/`optional`)
- MCP and CLI use shared lifecycle service
- no shared mutable request state in MCP service
- session and trace invariants enforced by tests
- benchmark proposal and benchmark CLI docs reflect real implementation status

