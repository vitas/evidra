# RC1 Review Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the remaining high-priority gaps from `evidra-benchmark-v0.3.1-rc1-review.md` and align code/docs/tooling with actual runtime behavior.

**Architecture:** Keep runtime behavior stable while tightening protocol invariants at persistence boundaries. Update MCP server metadata defaults, enforce session/trace invariants where entries are written, and align normative docs to implementation. Keep changes incremental with test-first steps.

**Tech Stack:** Go 1.23, `go test`, existing MCP/lifecycle packages, Markdown docs.

---

### Task 1: Fix MCP Server Default Version Drift

**Files:**
- Test: `pkg/mcpserver/server_test.go`
- Modify: `pkg/mcpserver/server.go`

**Step 1: Write failing test**
- Add a unit test that verifies empty `Options.Version` resolves to `version.Version`.

**Step 2: Run test to verify it fails**
- `go test ./pkg/mcpserver -run DefaultVersion -v`

**Step 3: Implement minimal fix**
- Replace hardcoded `v0.3.0-dev` default with runtime version constant.

**Step 4: Run test to verify pass**
- `go test ./pkg/mcpserver -run DefaultVersion -v`

---

### Task 2: Enforce Persisted Entry Correlation Invariants

**Files:**
- Test: `pkg/evidence/entry_store_test.go`
- Modify: `pkg/evidence/entry_store.go`
- Modify: `pkg/evidence/chain_test.go` (ensure fixtures satisfy new invariant)

**Step 1: Write failing tests**
- Add tests that `AppendEntryAtPath` rejects entries missing `session_id`.
- Add tests that `AppendEntryAtPath` rejects entries missing `trace_id`.

**Step 2: Run tests to verify failure**
- `go test ./pkg/evidence -run AppendEntryAtPath_Rejects -v`

**Step 3: Implement minimal fix**
- Add append-time validation in `AppendEntryAtPath` (or unlocked append path):
  - fail if `SessionID` is empty
  - fail if `TraceID` is empty

**Step 4: Update existing tests/fixtures**
- Ensure tests that append entries include `SessionID`.

**Step 5: Run package tests**
- `go test ./pkg/evidence -count=1`

---

### Task 3: Align Normative Trace/Session Docs with Runtime Contract

**Files:**
- Modify: `docs/system-design/EVIDRA_PROTOCOL.md`
- Modify: `docs/system-design/EVIDRA_SESSION_OPERATION_EVENT_MODEL.md`

**Step 1: Update protocol requirements**
- Correlation table: `trace_id` from SHOULD to MUST.
- Write-path invariants: persisted entries must include non-empty `trace_id`.

**Step 2: Update session/event model**
- Required IDs table: make `trace_id` required and keep `span_id` optional.
- Keep OTel mapping recommendation (`trace_id` SHOULD equal `session_id`) as guidance, not storage requirement change.

---

### Task 4: Fix Go Version Contract Mismatch

**Files:**
- Modify: `README.md`

**Step 1: Align README with go.mod**
- Update Build/Test section to match `go.mod` requirement (`go 1.23.0`).

---

### Task 5: Final Verification

**Commands:**
- `go test ./pkg/mcpserver ./pkg/evidence ./internal/lifecycle -count=1`
- `go test ./... -count=1`
- `make e2e`
- `make test-mcp-inspector`

**Exit criteria:**
- All targeted tests pass.
- No protocol/runtime mismatch remains for MCP default version and persisted session/trace invariants.
- Docs and toolchain requirement are internally consistent.

