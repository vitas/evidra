# Evidra v0.2.0-rc5 — Current State Baseline

## Purpose
This document freezes the starting point. It describes every module
in the current codebase, what it does, why it exists, what depends
on it, and what happens to it in the benchmark architecture.

Read this before touching any code.

---

## Summary

```
Version:     v0.2.0-rc5
Product:     OPA-based infrastructure policy engine
Architecture: Agent/CI → MCP validate → OPA evaluate → deny/allow → evidence

Total Go:    19,223 lines (124 files)
Total Rego:   4,153 lines (67 files)
UI (TS/TSX):  3,928 lines
Test infra:   2,214 lines

Dependencies: OPA v1.13.2, MCP SDK v1.3.1, ULID, YAML v3
              + 40 transitive deps (mostly from OPA)

Binary:       3 binaries (evidra CLI, evidra-mcp, evidra-api)
```

### Target Architecture (evidra-benchmark)

```
3 binaries, same shared core, different shells:

evidra CLI     — CI pipelines.  prescribe/report via shell. Local evidence.
evidra-mcp     — AI agents.     prescribe/report via MCP.   Local evidence.
evidra-api     — Backend.       Evidence aggregation, scorecards, /metrics.

v0.3.0: CLI + MCP (local only, no server)
v0.5.0: + API (centralized)
```

---

## Dependency Graph

```
cmd/evidra ─────────────┐
cmd/evidra-mcp ─────────┤
cmd/evidra-api ─────────┤
                        ▼
              ┌─────────────────┐
              │   internal/api  │──→ internal/engine ──→ pkg/runtime ──→ pkg/policy ──→ OPA
              │   internal/auth │                                        pkg/bundlesource
              │   internal/db   │
              │   internal/store│
              └────────┬────────┘
                       │
              ┌────────▼────────┐
              │ internal/evidence│──→ pkg/invocation
              │                  │──→ pkg/policy (for DecisionRecord)
              └──────────────────┘
                       │
              ┌────────▼────────────────────────────────────┐
              │              pkg/ layer                       │
              │                                              │
              │ mcpserver ──→ client, config, evidence,     │
              │               invocation, validate           │
              │                                              │
              │ validate ──→ bundlesource, config, evidence, │
              │              invocation, policysource,        │
              │              runtime, scenario                │
              │                                              │
              │ runtime ──→ bundlesource, invocation, policy │
              │ policy  ──→ bundlesource, invocation, OPA    │
              │                                              │
              │ evidence ──→ config, evlock, invocation      │
              │ scenario ──→ (standalone)                    │
              │ mode ──→ client                              │
              │ client ──→ invocation, validate              │
              └──────────────────────────────────────────────┘
```

Key insight: **OPA is reachable from almost every package** through
the chain `mcpserver → validate → runtime → policy → OPA`.
Removing OPA requires cutting at the `runtime → policy` boundary.

---

## Binaries

### cmd/evidra (2,610 lines)

CLI tool. Three subcommand groups:

| File | Lines | Purpose |
|------|-------|---------|
| main.go | 1,200 | CLI entry, `validate` command, config resolution, mode wiring |
| evidence_cmd.go | 1,100 | `evidence inspect`, `evidence verify`, `evidence export` |
| policy_sim_cmd.go | 310 | `policy-sim` — run scenarios against policy bundle |

**Fate:**
- main.go → REWRITE. Remove validate command, add scorecard/compare/prescribe/report.
- evidence_cmd.go → COPY + EXTEND. Evidence commands stay, add scorecard output.
- policy_sim_cmd.go → DROP. No policy simulation in benchmark.

### cmd/evidra-mcp (1,465 lines)

MCP server binary. Starts stdio or SSE transport, wires policy engine.

**Fate:** COPY + SIMPLIFY. Remove policy engine wiring. Wire
canonicalization adapters + risk analysis instead.

### cmd/evidra-api (627 lines)

HTTP API server. Postgres-backed, multi-tenant.

**Fate:** DROP for v0.3.0. Defer to v0.5.0 hosted platform.

---

## Package: pkg/mcpserver (2,684 lines, 10 files)

The MCP protocol layer. This is the entry point for AI agent interactions.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| server.go | 800 | MCP server setup, tool registration, validate handler, get_event handler | COPY + REFACTOR: remove validate tool, add prescribe + report tools |
| intent.go | 450 | SemanticIntent extraction, IntentKey (SHA256 hash) | SPLIT: identity fields → internal/canon/k8s.go, security fields → internal/risk/detectors.go |
| intent_test.go | 200 | Tests for intent extraction | SPLIT with code |
| deny_cache.go | 100 | Tracks denied intents, blocks retry loops | RENAME to retry_tracker.go, track (intent_digest, shape_hash) instead of deny keys |
| deny_cache_test.go | 100 | Tests | UPDATE |
| guidance_content.go | 300 | Loads markdown guidance for agent prompts | COPY + UPDATE content |
| guidance_content_test.go | 100 | Tests | COPY |
| schema_embed.go | 50 | Embeds JSON schema for validate input | UPDATE: new schema for prescribe/report |
| schemas/*.json | - | JSON schemas | REWRITE for prescribe/report input |
| server_test.go | 500 | Integration tests | REWRITE |

**Critical code to preserve:**

`intent.go` — contains the proto-canonicalization logic. The function
`ExtractSemanticIntent()` already extracts namespace, kind, name for
K8s and resource_types, destroy_count for Terraform. This is 70% of
what the new K8s adapter needs. But it mixes identity (namespace,
kind, name) with content (Images, SecurityPosture, CIDRs) in one
struct. The refactor splits these concerns.

`deny_cache.go` — tracks intent keys and deny counts with TTL. The
retry_loop detector needs the same mechanism but keyed on
(intent_digest + shape_hash) instead of the deny cache key.

`server.go` — the MCP server setup, transport wiring, and JSON
schema loading are reusable. The `validate` tool handler and its
OPA evaluation path are replaced by `prescribe` and `report`.

**Current flow:**
```
agent → validate(ToolInvocation) → ExtractSemanticIntent
      → DenyCache.CheckDenyLoop → doEvaluate(OPA) → deny/allow
      → BuildEvidence → sign → append to JSONL → return result
```

**New flow:**
```
agent → prescribe(raw_artifact) → Canonicalize(adapter)
      → RiskMatrix + Detectors → RetryTracker.Check
      → BuildPrescription → sign → append to JSONL → return prescription

agent → report(prescription_id, exit_code, artifact_digest)
      → MatchPrescription → EvaluateSignals
      → BuildProtocolEntry → sign → append to JSONL → return verdict
```

---

## Package: pkg/evidence (1,909 lines, 14 files)

Append-only JSONL evidence store with hash-linked chain.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| evidence.go | 300 | Public API, mode dispatch (segmented vs legacy) | COPY + SIMPLIFY (remove legacy mode) |
| store.go | 250 | Core append/read operations | COPY as-is |
| types.go | 150 | Record struct, Event types | COPY + EXTEND (add prescription, report, protocol types) |
| hash.go | 80 | SHA256 chain hashing | COPY as-is |
| io.go | 100 | JSONL read/write utilities | COPY as-is |
| segment.go | 200 | Segmented store (by date) | COPY as-is |
| segmented_test.go | 150 | Tests | COPY |
| manifest.go | 100 | Segment manifest | COPY as-is |
| forwarder.go | 200 | Push evidence to remote | COPY as-is (for v0.5.0 telemetry) |
| resource_links.go | 80 | MCP resource links for evidence | COPY as-is |
| lock.go | 50 | File locking for concurrent access | COPY as-is |
| legacy.go | 150 | Legacy non-segmented store | DROP (only segmented in new project) |

**This package is almost entirely reusable.** The core evidence chain
(append, hash-link, sign, verify) is tool-agnostic. It needs new
entry types (prescription, report, protocol_entry) but the storage
engine doesn't change.

---

## Package: internal/evidence (1,445 lines, 7 files)

Server-side evidence builder and signer.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| builder.go | 400 | Constructs evidence records with server metadata | COPY + EXTEND (add canonicalization fields) |
| builder_test.go | 200 | Tests | COPY + EXTEND |
| payload.go | 250 | Deterministic signing payload construction | COPY + EXTEND (add new fields to signing payload) |
| payload_test.go | 150 | Tests | COPY + EXTEND |
| signer.go | 200 | Ed25519 signing and verification | COPY as-is |
| signer_test.go | 100 | Tests | COPY as-is |
| types.go | 150 | DecisionRecord, field constants | COPY + REFACTOR (DecisionRecord → PrescriptionRecord) |

**Dependency risk:** `types.go` imports `pkg/policy` for the
DecisionRecord type. This is the one place where internal/evidence
touches OPA. Must replace `policy.Decision` with new prescription
types.

---

## Package: pkg/invocation (370 lines, 3 files)

Defines the ToolInvocation request struct.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| doc.go | 10 | Package documentation | COPY |
| invocation.go | 360 | Actor, ToolInvocation, validation | COPY + SIMPLIFY |

**Refactoring:**
- Remove `allowedParamKeys` and `rejectUnknownKeys` — these enforce
  OPA-specific payload structure.
- Remove `KeyRiskTags`, `KeyScenarioID` from canonical keys — these
  are OPA concepts.
- Keep: `Actor` struct, `ToolInvocation` struct basics, `KeyTarget`,
  `KeyPayload`, `KeyAction`.
- The struct becomes the prescribe request envelope.

---

## Package: pkg/policy (692 lines, 4 files)

OPA engine wrapper.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| policy.go | 400 | NewOPAEngine, Evaluate, result parsing | DROP entirely |
| policy_test.go | 200 | Tests | DROP |
| types.go | 50 | Decision struct (allow, deny, risk_level, rules) | REPLACE with new Prescription type |
| errors.go | 40 | Error types | DROP |

**This is the OPA core.** It creates a Rego VM, loads policy modules,
evaluates input, and parses results. None of this exists in the new
architecture. The Decision struct concept survives as Prescription
but with different fields.

---

## Package: pkg/runtime (296 lines, 2 files)

Evaluator abstraction over OPA.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| runtime.go | 200 | PolicySource interface, Evaluator, NewEvaluator | DROP entirely |
| policy_wiring_test.go | 96 | Tests | DROP |

**Replaced by:** direct calls to `internal/canon/` adapters and
`internal/risk/` detectors. No indirection layer needed.

---

## Package: pkg/bundlesource (404 lines, 2 files)

OPA bundle loading and management.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| bundlesource.go | 300 | Load OPA bundles from filesystem, parse manifest | DROP entirely |
| bundlesource_test.go | 104 | Tests | DROP |

**No bundles in new architecture.** Canonicalization adapters are
compiled Go code, not loaded at runtime.

---

## Package: pkg/policysource (154 lines, 2 files)

Local filesystem policy source for OPA.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| local_file.go | 100 | Scan directory for .rego files, build policy map | DROP entirely |
| local_file_test.go | 54 | Tests | DROP |

---

## Package: pkg/validate (2,007 lines, 9 files)

The "big validate" package. Orchestrates the full evaluation pipeline.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| validate.go | 800 | Core validation flow: resolve policy, evaluate, build evidence | DROP (replaced by prescribe/report flow) |
| prefixes.go | 100 | Extract allowed tool prefixes from policy data | DROP |
| schema.go | 200 | JSON schema validation for invocations | SIMPLIFY and move to prescribe input validation |
| validate_test.go | 600 | Tests | DROP |
| (5 more test files) | 300 | Various test scenarios | DROP |

**This is the largest package and it's entirely OPA-centric.** It
loads policy sources, resolves evaluation mode (offline/online),
calls OPA, builds evidence records, and handles fallback policies.
None of this applies to the benchmark.

---

## Package: pkg/scenario (360 lines, 3 files)

Policy simulation scenarios.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| load.go | 150 | Load scenario YAML files | DROP |
| schema.go | 150 | Scenario struct, validation | DROP |
| schema_test.go | 60 | Tests | DROP |

---

## Package: pkg/mode (180 lines, 2 files)

Evaluation mode resolution (enforce vs observe).

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| mode.go | 120 | Resolve mode from config/env/flags | DROP |
| mode_test.go | 60 | Tests | DROP |

**Always inspector in new architecture.** No mode switching.

---

## Package: pkg/client (524 lines, 3 files)

HTTP client for evidra-api.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| client.go | 350 | HTTP client, Validate endpoint | DROP for v0.3.0 (no API server) |
| errors.go | 100 | Error types | DROP |
| client_test.go | 74 | Tests | DROP |

---

## Package: pkg/config (237 lines, 4 files)

Configuration resolution.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| config.go | 100 | Config struct, file paths | SIMPLIFY (remove OPA-specific paths) |
| defaults.go | 50 | Default paths for evidence, policy | COPY + trim |
| config_test.go | 87 | Tests | UPDATE |

---

## Package: pkg/tokens (399 lines, 2 files)

API key management.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| tokens.go | 250 | Generate, validate API tokens | DROP for v0.3.0 (defer to hosted) |
| tokens_test.go | 149 | Tests | DROP |

---

## Package: pkg/outputlimit (64 lines, 2 files)

Limits output size for MCP responses.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| outputlimit.go | 40 | Truncate large outputs | COPY (still useful for MCP) |
| outputlimit_test.go | 24 | Tests | COPY |

---

## Package: pkg/evlock (133 lines, 3 files)

Cross-platform file locking.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| lock_unix.go | 60 | flock-based locking | COPY as-is |
| lock_windows.go | 50 | Windows locking | COPY as-is |
| lock_test.go | 23 | Tests | COPY |

---

## Package: pkg/version (15 lines, 1 file)

Version string.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| version.go | 15 | `var Version = "dev"` | COPY as-is |

---

## Package: internal/api (1,402 lines, 9 files)

HTTP API handlers for evidra-api server.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| router.go | 200 | HTTP router, middleware wiring | DROP for v0.3.0 |
| validate_handler.go | 300 | POST /v1/validate endpoint | DROP |
| health_handler.go | 50 | GET /healthz | DEFER |
| pubkey_handler.go | 100 | GET /v1/pubkey | DEFER |
| keys_handler.go | 250 | API key management endpoints | DROP |
| ui_handler.go | 100 | SPA serving | DROP |
| middleware.go | 150 | Body limit, request logging | DEFER |
| response.go | 80 | JSON response helpers | DEFER |
| api_test.go | 200 | Tests | DROP |

---

## Package: internal/auth (438 lines, 4 files)

Authentication middleware.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| middleware.go | 200 | API key auth, tenant resolution | DROP for v0.3.0 |
| context.go | 100 | Tenant context helpers | DROP |
| middleware_test.go | 100 | Tests | DROP |
| context_test.go | 38 | Tests | DROP |

---

## Package: internal/engine (323 lines, 2 files)

Adapter between API layer and OPA runtime.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| adapter.go | 200 | Wraps runtime.Evaluator for API use | DROP (replaced by direct canon + risk calls) |
| adapter_test.go | 123 | Tests | DROP |

---

## Package: internal/db (97 lines, 1 file)

PostgreSQL connection management.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| db.go | 97 | Connection pool, schema migration | DROP for v0.3.0 |

---

## Package: internal/store (180 lines, 1 file)

API key storage in PostgreSQL.

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| keys.go | 180 | CRUD for API keys | DROP for v0.3.0 |

---

## Rego Policies (4,153 lines, 67 files)

### Policy rules (23 deny, 4 warn)

| Rule | Lines | Purpose | Fate |
|------|-------|---------|------|
| deny_privileged_container | 30 | Privileged security context | → risk/detectors.go |
| deny_host_namespace | 35 | hostPID/hostIPC/hostNetwork | → risk/detectors.go |
| deny_hostpath_mount | 25 | hostPath volume | → risk/detectors.go |
| deny_dangerous_capabilities | 30 | CAP_SYS_ADMIN etc | → risk/detectors.go |
| deny_run_as_root | 20 | runAsUser: 0 | → risk/detectors.go |
| deny_kube_system | 25 | Protected namespace operations | → risk/detectors.go |
| deny_mass_delete | 30 | Terraform destroy > threshold | → risk/detectors.go |
| deny_sg_open_world | 35 | 0.0.0.0/0 ingress on dangerous ports | → risk/detectors.go |
| deny_aws_iam_wildcard_policy | 30 | Action:* Resource:* | → risk/detectors.go |
| deny_aws_iam_wildcard_principal | 25 | Principal:* | → risk/detectors.go |
| deny_terraform_iam_wildcard | 30 | Terraform IAM wildcards | → risk/detectors.go |
| deny_s3_public_access | 30 | Missing Block Public Access | → risk/detectors.go (defer) |
| deny_aws_s3_no_encryption | 25 | Missing SSE | DROP (not catastrophic) |
| deny_aws_s3_no_versioning | 25 | Missing versioning | DROP (not catastrophic) |
| deny_public_exposure | 35 | Public-facing services | → risk/detectors.go (defer) |
| deny_prod_without_approval | 30 | Prod changes need approval | DROP (policy, not detector) |
| deny_argocd_autosync | 25 | ArgoCD auto-sync in prod | DEFER to v0.5.0 |
| deny_argocd_dangerous_sync | 30 | Prune + selfHeal combo | DEFER to v0.5.0 |
| deny_argocd_wildcard_dest | 25 | Wildcard destination | DEFER to v0.5.0 |
| deny_insufficient_context | 40 | Missing required fields | DROP (protocol validation, not detector) |
| deny_terraform_metadata_only | 30 | Metadata-only plan | DROP (not catastrophic) |
| deny_truncated_context | 20 | Truncated plan output | DROP (protocol issue) |
| deny_unknown_destructive | 25 | Unknown destructive op | DROP (handled by risk matrix) |
| warn_no_resource_limits | 20 | Missing CPU/memory limits | DROP (not catastrophic) |
| warn_mutable_image_tag | 20 | :latest tag | DROP (not catastrophic) |
| warn_autonomous_execution | 25 | Agent acting without human | DROP (not our problem) |
| warn_breakglass | 15 | Breakglass override used | DROP (not catastrophic) |

### Infrastructure Rego

| File | Lines | Purpose | Fate |
|------|-------|---------|------|
| canonicalize.rego | 250 | Payload normalization in Rego | REPLACED by Go adapters |
| decision.rego | 80 | Aggregate deny/warn into decision | DROP |
| defaults.rego | 40 | Default values | DROP |
| insufficient_context_core.rego | 100 | Context validation rules | DROP |
| policy.rego | 10 | Entry point | DROP |

### Test Rego (40+ files)

All test .rego files are dropped. Detection logic tests move to
Go test files in `internal/risk/risk_test.go`.

**Porting guide:** Each deny rule is 20-40 lines of Rego. The
equivalent Go function is 10-25 lines. The test cases in _test.rego
files contain the input payloads — use these as test fixtures for
the Go detectors.

---

## UI (3,928 lines)

React + TypeScript dashboard for policy decisions.

| Directory | Purpose | Fate |
|-----------|---------|------|
| ui/src/ | SPA: decision viewer, evidence explorer | DROP entirely |
| ui/test/ | Component tests | DROP |
| ui/e2e/ | Playwright tests | DROP |
| ui/public/ | Static assets | DROP |

**Replaced by:** `evidra scorecard` CLI output (text).
Web UI deferred to v0.5.0 as hosted scorecard.

---

## Tests (2,214 lines)

| Directory | Purpose | Fate |
|-----------|---------|------|
| tests/corpus/ | OPA corpus tests (invoke → evaluate → assert decision) | DROP |
| tests/e2e/ | End-to-end OPA evaluation tests | DROP |
| tests/golden_real/ | Golden tests with real policy bundles | DROP |
| tests/inspector/ | Inspector mode tests (observe-only) | PARTIALLY REUSE concepts |

**Replaced by:** tests/golden/ for canonicalization, plus Go unit
tests per package.

---

## Other Files

| File | Purpose | Fate |
|------|---------|------|
| go.mod | Module definition with OPA dep | REWRITE (new module, no OPA) |
| go.sum | Dependency checksums | REGENERATE |
| bundleembed.go | Embeds OPA bundles into binary | DROP |
| promptsembed.go | Embeds MCP prompts | COPY + UPDATE |
| uiembed.go / uiembed_embed.go | Embeds React UI | DROP |
| Dockerfile / Dockerfile.api / Dockerfile.hosted | Container builds | REWRITE (simpler) |
| docker-compose.yml | Local dev stack (Postgres) | SIMPLIFY (no Postgres for v0.3.0) |
| server.json | MCP server config | UPDATE |
| Makefile | Build targets | REWRITE |
| verify_p0.sh | P0 verification script | REWRITE |
| scripts/run_golden_real.sh | Run golden tests | REPLACE |
| POLICY_CATALOG.md | Policy rule catalog | REPLACE with detector docs |
| ROADMAP.md | Product roadmap | REWRITE |
| README.md | Project README | REWRITE |
| CHANGELOG.md | Release history | START FRESH |

---

## Quantitative Summary

### By fate

| Fate | Go lines | Rego lines | Files |
|------|----------|------------|-------|
| COPY as-is | 1,400 | 0 | 18 |
| COPY + EXTEND | 1,100 | 0 | 8 |
| COPY + REFACTOR | 1,600 | 0 | 6 |
| CREATE NEW | 0 → ~1,500 | 0 | ~20 |
| DROP | 15,100 | 4,153 | ~95 |
| DEFER (v0.5.0) | 2,800 | 0 | 15 |

### Reuse ratio

```
Copied code:     ~4,100 lines (21% of Go)
Created new:     ~1,500 lines
Dropped:         ~15,100 lines Go + 4,153 lines Rego (79% of Go, 100% of Rego)
Deferred:        ~2,800 lines (14% of Go, comes back in v0.5.0)

New project estimated total: ~5,600 lines Go (vs 19,223 current)
Binary size: ~15MB (vs ~40MB with OPA)
External deps: 2 (vs 40+)
```
