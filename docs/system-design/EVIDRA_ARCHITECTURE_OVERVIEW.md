# Evidra — Architecture Overview

## Status
Entry point. Start here. This document links to everything else.

This is the orientation document for Evidra architecture.
For v1 design decisions, use the dedicated architecture sources below.

## Document Type
**Non-normative.** This is an overview for orientation. It does
not define contracts. Normative sources:
- **EVIDRA_SIGNAL_SPEC.md** — signal definitions, metric contracts
- **CANONICALIZATION_CONTRACT_V1.md** — adapter interface, digests
- **EVIDRA_PROTOCOL.md** — integration contract (session, correlation, scope, actor, findings)
- **EVIDRA_CORE_DATA_MODEL.md** — core data model (CanonicalAction, Prescription, Report, EvidenceEntry, Scorecard)
- **EVIDRA_SESSION_OPERATION_EVENT_MODEL.md** — session/operation/event hierarchy, OTel and CloudEvents mappings
- **EVIDRA_CNCF_STANDARDS_ALIGNMENT.md** — CloudEvents, SARIF, in-toto, OTel alignment

## Architecture Sources (v1)

Use these as the architecture source stack:

- **V1_ARCHITECTURE.md** — one-page end-to-end system map (pipeline, layers, interfaces, access points)
- **V1_IMPLEMENTATION_NOTES.md** — delivered detector architecture, signal/scoring integration, validation gate
- **EVIDRA_ARCHITECTURE_OVERVIEW.md** (this file) — narrative entry point and cross-links to normative contracts

## One-liner
Evidra is a flight recorder + reliability score for infrastructure automation.

---

## 30-Second Explanation

```
Automation asks Evidra before execution.
Evidra records intent and artifact.
After execution, automation reports result.
Signals are computed from evidence.
Signals produce reliability score.
```

That's it. Two calls (prescribe, report). Five signals. One score.

---

## Architecture

### Runtime Components

```
┌─────────────────────────┐
│     AI Agent Host        │
│                         │
│  Agent ◄──► evidra-mcp  │──── forward ────┐
│          (MCP server)   │                 │
└─────────────────────────┘                 │
                                            │
┌─────────────────────────┐                 │
│      CI Runner          │                 ▼
│                         │     ┌────────────────────┐
│  terraform / kubectl    │     │   evidra-api       │
│       │                 │     │   (backend)        │
│  evidra CLI             │──── │                    │
│  (shell wrapper)        │     │  evidence agg.     │
└─────────────────────────┘     │  scorecards        │
                                │  /metrics (Prom)   │
                                └────────────────────┘
```

**evidra-mcp** — sidecar for automation clients over MCP (stdio/SSE).
Common initiators include AI agents, but any MCP-capable client can use it.
Exposes prescribe + report tools. Local evidence JSONL.

**evidra CLI** — for CI pipelines. Same protocol, shell wrapper.
`evidra prescribe`, `evidra report`, `evidra scorecard`, `evidra validate`,
`evidra ingest-findings`.

**evidra-api** — centralized backend. Aggregates evidence from all
sources. Scorecards, comparison, Prometheus metrics. Multi-tenant.
Planned for v0.5.0.

### Initiators and Ingress Paths

| Initiator | Current ingress (v0.3.1) | Planned ingress |
|---|---|---|
| MCP-capable automation clients (including AI agents) | MCP (`evidra-mcp`) | REST sidecar/API |
| CI/CD pipelines | CLI (`evidra prescribe/report`) | REST sidecar/API |
| Scanner-only workflows | CLI (`evidra ingest-findings`) | REST `/v1/findings` |
| Custom automation services | CLI wrapper or MCP client | REST sidecar/API + SDKs |

All three share the same core: canon adapters, risk analysis,
signal detectors, evidence chain. v0.3.1 works fully local
without evidra-api.

No OPA. No Rego. No policy engine. No deny.

### Core Pipeline Architecture

The three components above are deployment shells. Inside each one
lives the same core pipeline:

```
Agent / CI
    │
    │ prescribe(raw_artifact)
    ▼
┌──────────────────────────────────────────────────────────┐
│                    Inspector API                         │
│  ┌────────────────────────────────────────────────────┐  │
│  │              Canonicalization                      │  │
│  │  raw artifact → adapter → CanonicalAction          │  │
│  │  artifact_digest (raw bytes)                       │  │
│  │  intent_digest (canonical JSON)                    │  │
│  │  resource_shape_hash (normalized spec)             │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                │
│  ┌──────────────────────▼─────────────────────────────┐  │
│  │              Risk Analysis                         │  │
│  │  risk matrix (op_class × scope_class → risk_level) │  │
│  │  catastrophic detectors (raw artifact → risk_tags) │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                │
│                         ▼                                │
│               Prescription returned                      │
│               (risk_level, risk_details)                  │
└──────────────────────────┬───────────────────────────────┘
                           │
                    Agent executes
                           │
    │ report(prescription_id, exit_code, artifact_digest)
    ▼
┌──────────────────────────────────────────────────────────┐
│                   Evidence Store                         │
│  append-only JSONL, hash-linked, Ed25519 signed          │
│  prescription → report → protocol_entry                  │
└──────────────────────────┬───────────────────────────────┘
                           │
                           │ on demand (scorecard / explain)
                           ▼
┌──────────────────────────────────────────────────────────┐
│                   Signals Engine                         │
│  protocol_violation │ artifact_drift │ retry_loop        │
│  blast_radius       │ new_scope                          │
└──────────────────────────┬───────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────┐
│                Score + Explain                           │
│  penalty = Σ(weight × rate)                              │
│  score = 100 × (1 - penalty)                             │
│  confidence = f(completeness, canon_trust, actor_trust)  │
│  band = excellent | good | fair | poor                   │
└──────────────────────────┬───────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────┐
│                   Export Plane                            │
│  /metrics (Prom)  │  OTLP (OTel)  │  JSONL  │  SIEM     │
└──────────────────────────────────────────────────────────┘
```

Key properties of this pipeline:
- **Canonicalization and risk analysis are synchronous** (on prescribe call)
- **Evidence write is synchronous** (prescription and report are written immediately)
- **Signals and scoring are lazy** (computed on demand, not real-time)
- **Export is a read path** (consumes evidence, never writes back)

### Fourth Component: Signal Export

```
evidra-mcp / evidra CLI / evidra-api
          │
          ▼
    Signal Export Plane
          │
    ┌─────┼─────┬──────────┐
    ▼     ▼     ▼          ▼
  /metrics  OTLP  JSONL    SIEM
  (Prom)   (OTel) (file)  (webhook)
```

Signal Export is how external systems consume Evidra signals.
v0.3.0 ships Prometheus /metrics and JSONL evidence. v0.5.0 adds
OpenTelemetry export and webhook-based SIEM integration.

Metric registry (names, labels, cardinality rules) is defined
normatively in EVIDRA_SIGNAL_SPEC.md §Metric Registry.

---

## Data Flow (One Operation)

**Canonicalization location:** Steps 2–8 execute **server-side**
(inside evidra-mcp or evidra CLI process). The agent/CI sends raw
bytes; Evidra computes digests, parses, canonicalizes, and runs
risk detectors. Client-side canonicalization (pre-canonicalized
path) is an optional optimization — the client MAY send a
`canonical_action`, but Evidra is the authority. Pre-canonicalized
entries are marked `canon_source=external` and subject to
confidence penalties (see Confidence Model).

```
1. Agent/CI sends raw_artifact to prescribe()
        │
2. artifact_digest = SHA256(raw bytes)           ← before any parsing
        │
3. Domain adapter parses raw artifact
   k8s: unstructured.Unstructured
   tf:  tfjson.Plan
   generic: opaque hash
        │
4. Noise removal (frozen list per canon version)
        │
5. canonical_action produced
   resource_identity, operation_class, scope_class, resource_count
        │
6. intent_digest = SHA256(canonical_json(canonical_action))
   resource_shape_hash = SHA256(normalized spec)
        │
7. Risk matrix: operation_class × scope_class → risk_level
        │
8. Catastrophic risk detectors read RAW artifact
   → risk_tags (privileged, hostPath, wildcard IAM, etc.)
        │
9. Prescription written to evidence chain
   Prescription returned to agent (risk_level, risk_details)
        │
10. Agent executes the operation
        │
11. Agent calls report(prescription_id, exit_code, artifact_digest)
        │
12. Signal evaluation:
    - Protocol Violation: report exists? prescription matched?
    - Artifact Drift: prescription.artifact_digest == report.artifact_digest?
    - Retry Loop: same actor + same intent_digest + same shape_hash,
                   N times in T minutes, after failure exit code?
    - Blast Radius: resource_count > threshold for operation_class?
    - New Scope: first (tool, operation_class, scope_class) tuple?
        │
13. Protocol entry written to evidence chain
        │
14. Scorecard computed on demand from evidence chain
```

Details: [End-to-End Example](EVIDRA_END_TO_END_EXAMPLE_v2.md)

---

## Key Decisions

### Tool-Agnostic Protocol
The prescribe/report protocol doesn't know what tool it's talking to.
The `tool` field is a string, not an enum. Any value is accepted.
Built-in adapters handle known tools (kubectl, terraform, helm).
Unknown tools fall through to the generic adapter or accept
pre-canonicalized input. New tools integrate without code changes
to Evidra core.

Why: Evidra is a telemetry layer, not a tool-specific product.

### Inspector, Not Enforcer
prescribe() never denies. Never blocks. Returns risk_level and
risk_details. Agent decides. Both behaviors recorded.

Why: inspector boundary is part of the core architecture and protocol semantics
(`EVIDRA_PROTOCOL.md`, `EVIDRA_AGENT_RELIABILITY_BENCHMARK.md`).

### Five Signals, Not Policies
No OPA. No Rego. No policy rules. Five behavioral signals detect
anomalies. Catastrophic risk detectors are Go functions (~200 lines),
not a policy engine.

Why: [Benchmark](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) §2, §7

### Canonicalization Is the Foundation
Intent identity = resource identity (apiVersion, kind, namespace,
name for K8s; type, name, actions for Terraform). NOT the full spec.
Spec content goes to resource_shape_hash (for retry detection) and
to risk detectors (for catastrophic pattern matching). Two separate
concerns.

Contract: [Canonicalization Contract v1](CANONICALIZATION_CONTRACT_V1.md)

### Two Digests, Two Purposes
artifact_digest = raw bytes. Protocol integrity.
intent_digest = canonical JSON. Behavioral identity.
Same artifact_digest → same intent_digest. Not the reverse.

Details: [Canonicalization Contract v1](CANONICALIZATION_CONTRACT_V1.md) §1

### Scope-Aware Comparison
Agents with different workload profiles (different tools, different
scopes) are not directly comparable. Evidra warns when overlap is
low. Fair comparison filters by shared tool+scope. Version
comparison (same agent, different versions) is always valid.

Details: [Benchmark](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) §4-5

### No Deny, No Kill-Switch
Catastrophic risk detectors produce risk_tags (informational), not
deny/allow. risk_level comes from a fixed matrix (operation_class ×
scope_class). Detectors only cover patterns that have caused real
production outages. Missing labels, YAML formatting, no resource
limits — not detectors.

Details: [Benchmark](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) §7

### Detectors Receive Both Canonical Action AND Raw Artifact
Detectors receive BOTH canonical_action (for identity and scope
context) AND raw artifact bytes (for content inspection). The
canonical_action tells them "what kind of resource." The raw
artifact tells them "what's inside."

Source: [V1 Architecture](V1_ARCHITECTURE.md) §Layers

### risk_tags Belong to Prescription, Not CanonicalAction
risk_tags are computed by catastrophic risk detectors AFTER
canonicalization. They live in Prescription, not CanonicalAction.
canonical_action is adapter output (deterministic, testable).
risk_tags are detector output (separate concern).

Source: [V1 Architecture](V1_ARCHITECTURE.md) §Data Flow Example,
[Core Data Model](EVIDRA_CORE_DATA_MODEL.md) §Prescription

### TTL Detection at Scorecard Time, Not Real-Time
TTL detection happens when `evidra scorecard` scans the evidence
chain — it finds prescriptions without matching reports and
retroactively marks them as protocol violations. No background
process needed. Real-time TTL detection is a v0.5.0 feature of
evidra-api (long-running, can run periodic scans).

Source: [V1 Architecture](V1_ARCHITECTURE.md) §Layers,
[Signal Spec](EVIDRA_SIGNAL_SPEC.md)

### Prescription Matching: 1:1, ULID-Keyed, Four Violation Types
- prescription_id is globally unique (ULID)
- First report wins; second report → duplicate_report violation
- Unknown prescription_id → unprescribed_action violation
- Cross-actor report → cross_actor_report violation
- Batched apply (e.g. terraform apply with 10 resources) = one
  prescription with resource_count=10, one report

Source: [Core Data Model](EVIDRA_CORE_DATA_MODEL.md) §Prescription/Report

### Pre-Canonicalized Path: Accepted Trade-Off
When a tool sends its own canonical_action, Evidra trusts the
resource identity. Pre-canonicalized path trades accuracy for reach.
Signals are only as good as the input. Entries are marked with
`canon_source=external` so scorecards can show what percentage of
data is self-reported. Blast radius may be inaccurate for
pre-canonicalized input — documented as a known limitation.

Source: [Core Data Model](EVIDRA_CORE_DATA_MODEL.md) §MCP/CLI tool inputs

### Post-Implementation Decisions (v0.3.x)

### CLI and MCP Server Both Write Evidence
MCP server was the sole evidence writer in early v0.3.0. CLI commands
(`prescribe`, `report`) now also write evidence entries. Both paths
produce identical `EvidenceEntry` format with the same hash chain.

### Session and Correlation Model
Every evidence entry supports hierarchical correlation:

- **session_id** (persisted MUST) — run/session boundary. If omitted
  on ingress, Evidra generates one before writing evidence.
- **trace_id** (persisted MUST) — correlation key for related events.
  Prescribe generates one when omitted; report inherits from the
  referenced prescription when available.
- **span_id / parent_span_id** (MAY) — hierarchical tracing for
  multi-step agent workflows. Allows tree-structured correlation
  compatible with OpenTelemetry span model.

See [Integration Protocol](EVIDRA_PROTOCOL.md) §3 for the full
correlation model.

### Report Actor Resolution
`ReportInput` accepts an optional `actor` field. Falls back to
the referenced prescription actor when omitted. CLI defaults to
`actor.id="cli"` when no explicit actor is provided.

### Signing Modes
`BuildEntry` requires a `Signer` interface and returns an error if none
is provided. Every evidence entry is Ed25519-signed — the `signature` field
(base64-encoded) is mandatory. The `Signer` module lives in
`internal/evidence/signer.go`.

Runtime modes:

- **strict (default):** requires `EVIDRA_SIGNING_KEY` or
  `EVIDRA_SIGNING_KEY_PATH`.
- **optional:** generates an ephemeral in-process key when no key is
  configured (local/test convenience, not durable across restarts).

- **CLI:** `--signing-key-path key.pem` or `EVIDRA_SIGNING_KEY_PATH` env var
- **MCP:** `EVIDRA_SIGNING_KEY` or `EVIDRA_SIGNING_KEY_PATH` env vars

Validation via `evidra validate --public-key <pem>`.
`ValidateChainWithSignatures()` verifies both hash chain and signatures.

### Standalone Findings Ingestion (v0.3.0)
`evidra ingest-findings --sarif <file>` ingests scanner findings (SARIF)
as evidence entries without requiring a prescribe call. This supports
pre-merge scanner workflows (e.g. Trivy, Kubescape in CI) where no
artifact execution occurs.

### Session and Scoring Boundary (v0.3.0)
v0.3.0 scorecard is scoped by **actor + time period**. `session_id` is
recorded in evidence entries but does NOT affect scoring grouping in v0.3.0.
Per-session scorecards are a v0.5.0 feature that requires evidra-api for
session lifecycle management. The data is captured now so future scoring
can leverage it without re-ingestion.

### Compare Command Requires Evidence History
`evidra compare` reads evidence, builds per-actor workload profiles
(tools × scopes), and computes Jaccard similarity. Requires sufficient
data per actor to be meaningful.

### SARIF Findings Are Evidence Entries
Scanner findings (Checkov, Trivy, tfsec) parsed from SARIF format
are written as `finding` evidence entries linked primarily by
`artifact_digest` (and optionally by session/trace correlation when
available). CLI uses `--scanner-report`; MCP server can ingest
findings through the prescribe flow.

---

## Architecture Invariants

Non-negotiable properties. If an implementation violates any of
these, it is architecturally incorrect even if it "works."

### Evidence Is Single Source of Truth
- **Evidence-first computation:** All signals, scores, and
  explanations MUST be derived from recorded evidence entries.
  No side-channel state affects scoring.
- **Replay determinism:** Same evidence log + configuration +
  versions → same signals and scores.

### Evidence Store Is Append-Only and Tamper-Evident
- Evidence entries MUST NOT be mutated or deleted. Corrections
  are new entries.
- Hash-linked chain: insertion, reordering, or modification is
  detectable during verification (offline-capable).
- Server receipts (if any) are also evidence entries — no
  parallel evidence model.

### Inspector Protocol (Prescribe/Report Lifecycle)
- Lifecycle states: PRESCRIBED → REPORTED → CLOSED, or
  EXPIRED (no report within TTL), or UNPRESCRIBED (report
  without preceding prescribe).
- prescribe/report are the protocol. No approve, cancel, deny.

### TTL Is Data
- TTL MUST be a field of the prescription (`ttl_ms`).
- Default TTL is allowed but MUST be materialized into the
  stored prescription data (replay determinism).

### Identity and Correlation
- **actor is mandatory** for every prescribe/report: actor.type,
  actor.id, actor.provenance (or actor.origin). Optional fields:
  actor.instance_id (runner/pod/container — NOT used in metrics)
  actor.version (agent software version), and actor.skill_version
  (contract/prompt version used by the agent).
- **session_id** is the run/session boundary. Persisted entries MUST
  carry session_id; if omitted at ingress, Evidra generates one.
  In v0.3.x scorecards are still grouped by actor+period (session
  grouping is planned for service mode).
- **trace_id** is the per-operation correlation key. A single
  trace_id MAY span multiple prescribe/report pairs (a terraform
  plan that touches 3 resources = 3 prescriptions under one
  trace_id). A trace_id MUST NOT span multiple actors or tenants.
- **span_id / parent_span_id** (MAY) support hierarchical agent
  workflows (OpenTelemetry compatible).
- **scope_dimensions** (MAY) is a map of detailed environment
  metadata (cluster, namespace, account, region). Not used in
  metrics — scope_class remains the low-cardinality dimension.
- **tenant_id** is always present in service mode (v0.5.0+).
- Optional correlation: repo, work_item_key, commit_sha, env,
  target. Missing fields = unknown/empty, not ambiguous.

### Canonicalization Is Mandatory
- Follows the versioned contract (CANONICALIZATION_CONTRACT_V1).
- intent_digest (canonical semantic hash) ≠ artifact_digest
  (raw bytes hash). They MUST NOT be treated as interchangeable.
- Canonicalization failures MUST produce evidence entries
  (type=canonicalization_failure), not silent drops.

### Signals Are Behavioral, Not Policy Rules
- Signals model behaviors: drift, retries, scope expansion,
  protocol violations, blast radius.
- Evidra MUST NOT evolve into a policy engine or scanner rule
  system.
- Signals are derived from evidence + configuration, no hidden
  inputs.

### Scoring Is Stable, Simple, and Versioned
- Score computation is versioned and replayable.
- Bands are deterministic given score + config.
- Safety floors: score ceilings for untrusted evidence; floor
  overrides for catastrophic risk signals (if configured).

### Validators Are External
- Evidra does not run validators (v1). Validators produce
  findings; Evidra records them as independent evidence entries
  (type=finding) and may transform them into behavioral signals.
- Findings are linked to operations by artifact_digest, not
  embedded in reports. This decouples scanner timing from the
  prescribe/report lifecycle.
- Validator outputs normalized into a stable finding schema:
  tool, rule_id, severity, resource, message, artifact_digest.

### Multi-Tenancy Is Strict (v0.5.0+)
- Tenant isolation for storage and queries — no cross-tenant
  reads, explains, or benchmarks.
- tenant_id derived from auth middleware, not guessed.

### Versioning Is Visible Everywhere
- Every record and output carries: spec_version,
  canonical_version, adapter_version, scoring_version.
- Mandatory for benchmark reproducibility.

---

## Design Principles

1. **Tool-agnostic protocol.** Any automation tool integrates via prescribe/report.
2. **Inspector, not enforcer.** prescribe() never denies.
3. **Signals over policies.** Five behavioral signals, not rules.
4. **Canonicalization defines intent.** Frozen contract, versioned, golden-tested.
5. **Evidence chain as source of truth.** Append-only, signed, hash-linked.
6. **Scope-aware comparison.** Only compare agents doing the same work.
7. **Catastrophic risk only.** Detectors cover outage patterns, not style.
8. **Minimal dependencies.** Two external libraries, ~2.3MB total.
9. **Simple tests.** ~65 tests catch the same bugs as 8000.
10. **Standard signals.** Same five signals for every tool, every actor.

### Drift Guards

Three boundaries that MUST NOT be crossed. See EVIDRA_SIGNAL_SPEC.md
for normative definitions.

| Boundary | Rule | Violation smell |
|----------|------|-----------------|
| Adapter growth | Contract defines schema, adapters are libraries. Contract MUST NOT grow per tool. | "Let's add a Pulumi section to the contract" |
| Detector scope | Detectors produce signal context, not policy. Max 15. | "Let's add a compliance detector" |
| Evidence simplicity | Evidence is append-only log. Not a database. Not a queue. | "Let's add indexing to evidence" |

---

## Signals

| Signal | What it detects | Metric |
|--------|----------------|--------|
| Protocol Violation | Missing report, missing prescribe | violations / total_ops |
| Artifact Drift | Agent changed artifact between prescribe and report | drifts / total_reports |
| Retry Loop | Same actor + same intent + same content, repeated after failure | retry_events / total_ops |
| Blast Radius | Operation affects too many resources | blast_events / total_ops |
| New Scope | First operation in a (tool, op_class, scope_class) tuple | scope_events / total_ops |

Protocol Violation distinguishes two sub-signals:
stalled_operation (agent hung) vs crash_before_report (agent died).

**Catastrophic Context** (not a signal, but a red lamp):
When risk detectors find catastrophic patterns (privileged
containers, world-open ingress, wildcard IAM), the event is
tagged, shown in the scorecard's CATASTROPHIC CONTEXT section,
and exported as `evidra_catastrophic_context_total` Prometheus
counter. Evidra doesn't block — ops alerting stack can.

Details: [Benchmark](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) §2, §7

---

## Reliability Score

```
score = 100 × (1 - penalty)
penalty = 0.35 × violation_rate
        + 0.30 × drift_rate
        + 0.20 × retry_rate
        + 0.10 × blast_rate
        + 0.05 × scope_rate
```

| Band | Score | Meaning |
|------|-------|---------|
| Excellent | 99-100 | Production-ready |
| Good | 95-99 | Minor issues |
| Fair | 90-95 | Needs attention |
| Poor | <90 | Unreliable |

Minimum sample: 100 operations. Below that: "insufficient data."

### Confidence Model

Raw score assumes trusted evidence. Confidence adjusts the
ceiling — untrusted evidence cannot produce a top-band score.

```
confidence = f(evidence_completeness, canon_trust, actor_trust)
```

Three inputs:

| Factor | High | Medium | Low |
|--------|------|--------|-----|
| Evidence completeness | All prescriptions have reports, no gaps | <5% unreported prescriptions | >10% unreported or evidence gaps |
| Canonicalization trust | canon_source=adapter (Evidra parsed) | Mixed adapter + external | All canon_source=external (self-reported) |
| Actor identity trust | Verified (OIDC/API key, v0.5.0+) | Self-reported, consistent | Self-reported, inconsistent or missing |

Score ceiling by confidence:

| Confidence | Score ceiling | Rationale |
|------------|--------------|-----------|
| High | 100 (no cap) | Evidence is trustworthy |
| Medium | 95 | Cannot claim "excellent" without full trust |
| Low | 85 | Self-reported data gets "fair" at best |

Rules:
1. If >50% of entries have `canon_source=external` → confidence
   drops to Medium (at best).
2. If protocol_violation_rate > 10% → confidence drops to Low
   (evidence is incomplete).
3. If actor identity is unverified AND tenant_id is empty →
   confidence drops to Medium.
4. Confidence is computed per scorecard, not per entry or per
   signal. Individual signals are binary (detected or not) — they
   do not carry their own confidence.
5. Scorecard output MUST include `confidence` field alongside
   `score` and `band`.

This protects against self-reported artifacts, fake
canonicalization, and incomplete traces. An agent that skips
prescribe calls or sends pre-canonicalized data cannot achieve
"excellent" — by design.

Details: [Benchmark](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) §3

---

## Evidence Chain

Append-only JSONL. Three entries per operation:

```
prescription → report → protocol_entry
```

Each entry: hash-linked to previous, Ed25519 signed, timestamped.
Includes canonicalization_version and adapter_version.

Scorecard reads evidence chain on demand. Single-pass scan.
No database. No aggregation service.

### Evidence Entry Schema

Every JSONL line is a single evidence entry. All entry types share
the same envelope. This schema is **normative** — implementations
MUST write entries conforming to it. Replay determinism depends on
every entry having the same shape.

```jsonc
{
  // === Envelope (all entry types) ===
  "entry_id":      "01JNG...",         // ULID, globally unique
  "type":          "prescription",     // see Entry Types below
  "ts":            "2026-03-04T12:00:00.000Z",  // RFC 3339, UTC

  // === Identity ===
  "actor": {
    "type":        "agent",            // "agent" | "ci" | "human"
    "id":          "claude-code",      // stable, low-cardinality identifier
    "origin":      "mcp",             // "mcp" | "cli" | "api"
    "instance_id": "runner-234",      // (MAY) pod/container/runner — NOT in metrics
    "version":     "1.4.2"            // (MAY) actor software version
  },
  "tenant_id":     "",                 // empty in local mode, required in service mode (v0.5.0+)

  // === Correlation ===
  "session_id":    "01JNG...",         // run/session boundary (persisted MUST)
  "trace_id":      "01JNG...",         // per-operation correlation key
  "span_id":       "01JNG...",         // (MAY) step within a trace
  "parent_span_id": "01JNG...",        // (MAY) parent span for hierarchical workflows

  // === Digests (present on prescription and report) ===
  "artifact_digest": "sha256:abc...",  // SHA256 of raw artifact bytes
  "intent_digest":   "sha256:def...", // SHA256 of canonical JSON (prescription only)

  // === Payload (type-specific) ===
  "payload":       { },                // see Payload by Type below

  // === Scope ===
  "scope_dimensions": {                // (MAY) detailed environment metadata
    "cluster":   "prod-cluster-1",
    "namespace": "payments",
    "account":   "aws-prod",
    "region":    "eu-central-1"
  },

  // === Chain integrity ===
  "previous_hash": "sha256:...",       // hash of previous entry (empty for first)
  "hash":          "sha256:...",       // hash of this entry (all fields except hash itself)
  "signature":     "ed25519:...",      // Ed25519 signature of hash

  // === Versions (mandatory) ===
  "spec_version":       "1.0",
  "canon_version":      "k8s/v1",     // adapter-specific, e.g. "k8s/v1", "terraform/v1"
  "adapter_version":    "0.3.0",
  "scoring_version":    "1.0"
}
```

### Entry Types

| Type | When written | Payload contains |
|------|-------------|-----------------|
| `prescription` | prescribe() processes an artifact | canonical_action, risk_level, risk_tags, risk_details, ttl_ms, canon_source |
| `report` | report() records execution outcome | prescription_id, exit_code, verdict |
| `canonicalization_failure` | Adapter fails to parse artifact | error_code, error_message, adapter, raw_digest |
| `finding` | Scanner output (independent entry, linked by artifact_digest) | tool, rule_id, severity, resource, message, artifact_digest |
| `signal` | Signal detector fires (written at scorecard time) | signal_name, sub_signal, entry_refs, details |
| `receipt` | evidra-api acknowledges forwarded batch (v0.5.0+) | batch_id, entry_count, server_ts |

### Payload by Type

**prescription:**
```jsonc
{
  "prescription_id": "01JNG...",
  "canonical_action": {
    "tool":               "kubectl",
    "operation":          "apply",
    "operation_class":    "mutate",
    "resource_identity":  [{"api_version": "apps/v1", "kind": "Deployment", "namespace": "prod", "name": "web"}],
    "scope_class":        "production",
    "resource_count":     1,
    "resource_shape_hash": "sha256:..."
  },
  "risk_level":    "high",
  "risk_tags":     ["privileged_container"],
  "risk_details":  ["Container runs as privileged in production namespace"],
  "ttl_ms":        600000,
  "canon_source":  "adapter"           // "adapter" | "external"
}
```

**report:**
```jsonc
{
  "prescription_id": "01JNG...",
  "exit_code":       0,
  "verdict":         "success"         // "success" | "failure" | "error"
}
```

**canonicalization_failure:**
```jsonc
{
  "error_code":    "parse_error",
  "error_message": "invalid YAML at line 42",
  "adapter":       "k8s",
  "raw_digest":    "sha256:..."        // always computable from raw bytes
}
```

**finding:**
```jsonc
{
  "tool":            "checkov",
  "rule_id":         "CKV_AWS_18",
  "severity":        "high",
  "resource":        "aws_s3_bucket.data",
  "message":         "Ensure the S3 bucket has access logging enabled",
  "artifact_digest": "sha256:..."      // links finding to the operation's artifact
}
```

Validator findings are **independent evidence entries**. They are
NOT embedded in Report. Linking is by `artifact_digest` — the same
digest that appears on the prescription and report for that
operation. This decouples scanner timing from the prescribe/report
lifecycle: findings may arrive before, during, or after execution.

### Schema Rules

1. **entry_id** is a ULID. Monotonically increasing within a single writer.
2. **type** is a closed enum. Adding a new type requires a spec version bump.
3. **ts** is always UTC. Clock skew between writers is accepted; ordering is by entry position in the chain, not by timestamp.
4. **actor** is mandatory on prescription and report. MAY be empty on signal and receipt entries.
5. **tenant_id** is empty string in local mode. In service mode (v0.5.0+), every entry MUST have a non-empty tenant_id derived from auth, not self-reported.
6. **trace_id** identifies an automation task/session. One trace_id MAY span multiple prescribe/report pairs (e.g. multi-resource apply). A trace_id MUST NOT span multiple actors or tenants.
7. **Digests** use `sha256:` prefix. artifact_digest is present on both prescription and report (for drift detection). intent_digest is prescription-only.
8. **Versions** are mandatory on every entry. If an entry is written by a component that doesn't know scoring_version (e.g. CLI prescribe), it writes `""` — never omits the field.
9. **previous_hash** creates the append-only chain. First entry in a segment has `previous_hash: ""`.
10. **Entries are immutable.** Corrections are new entries, not mutations.

---

## Strategic Positioning

Consolidated from prior design iterations. Key points below.

### What is defensible

| Layer | Defensibility | Why |
|-------|--------------|-----|
| Canonicalization contract | **High** | Cross-tool ABI. Hard to replicate correctly. Golden corpus = compatibility history. |
| Signal semantics | **High** | Shared vocabulary. If "retry_loop" becomes industry term, Evidra is the reference. |
| Golden corpus | **High** | Years of curated artifacts, mutations, version transitions. Grows with every adapter. |
| Ecosystem integrations | **High** | Once embedded (GH Action, TF plugin, agent SDKs), switching cost is real. |
| Benchmark dataset | **Medium→High** | Cross-org comparison data. Grows with adoption. |

### What is NOT defensible

| Layer | Defensibility | Accept it |
|-------|--------------|-----------|
| Five signals | Low | Any vendor can implement similar counters |
| Score formula | Low | Weighted sum is trivial to replicate |
| CLI / MCP / API | Low | Standard engineering practice |
| Evidence log | Low | Append-only JSONL is not proprietary |

**Investment follows defensibility.** Canonicalization correctness
and ecosystem distribution get the most attention. Score formula
gets the least.

### Primary failure risk

Canonicalization complexity. If the contract becomes too complex,
tool authors cannot implement it, ecosystem adoption slows, and
standardization fails. The contract must remain small,
deterministic, and testable.

### Signal Export as independent layer

Benchmark scoring is **one consumer** of the signal layer. Not the
only one. The spec stack:

```
Canonicalization Contract → produces canonical intent
Signal Spec              → defines signals and metrics
Signal Export            → Prometheus, OTel, SIEM, JSONL
        │
        ├── Benchmark (scoring, comparison)
        ├── Dashboards (Grafana)
        ├── SIEM (security correlation)
        ├── Data warehouse (historical analysis)
        └── Agent frameworks (runtime decisions)
```

Evidra's value is in the bottom two layers (canonicalization +
signals). Everything above is a consumer that can be replaced.

Evidra is **behavioral telemetry for automation** — the same way
Prometheus is metrics for infrastructure, and OpenTelemetry is
the standard for distributed traces.

```
Infrastructure observability stack:
  Metrics → Prometheus
  Logs    → Loki / Elasticsearch
  Traces  → OpenTelemetry
  Automation behavior → Evidra Signal Spec     ← new layer
```

The spec stack:

```
Evidra spec stack:
  EVIDRA_SIGNAL_SPEC.md         = OpenTelemetry Semantic Conventions
  CANONICALIZATION_CONTRACT.md  = Protocol Buffers / schema definition
  Benchmark                     = Consumer (like Jaeger consumes OTel)
```

Evidra is NOT a policy engine. NOT a security scanner. NOT runtime
enforcement. It measures, records, and scores automation behavior
through standard signals.

**Evidra integrates with security scanners, not replaces them.**
Checkov, Trivy, tfsec produce security findings. Evidra consumes
their SARIF output as risk context on prescriptions. Scanners
provide point-in-time validation; Evidra provides longitudinal
behavioral telemetry. The combination is stronger than either alone.

Any tool that modifies infrastructure can integrate:

| Tool | Integration | v0.3.0? |
|------|------------|---------|
| kubectl / K8s | Built-in adapter (raw YAML) | Yes |
| Terraform | Built-in adapter (plan JSON) | Yes |
| Helm | Via K8s adapter (template output) | Yes |
| Docker / nerdctl (containerd) | Built-in adapter (command string parsing) | Yes |
| Pulumi | Pre-canonicalized prescribe | Ready (no adapter needed) |
| Ansible | Pre-canonicalized prescribe | Ready |
| CloudFormation | Pre-canonicalized prescribe | Ready |
| ArgoCD | Built-in adapter | v0.5.0 |
| Custom tools | Pre-canonicalized prescribe | Ready |

Two integration paths:
- **Adapter path:** Evidra parses the tool's native artifact format
- **Pre-canonicalized path:** Tool sends its own resource identity,
  Evidra handles risk analysis, signals, scoring

Both produce identical evidence, signals, and scores.

---

## Inspector Model

### Why This Model

Evidra is NOT an admission controller (Gatekeeper, Kyverno), NOT
an execution proxy (Spacelift, Terraform Cloud), NOT a runtime
scanner (Trivy, Falco), NOT a post-execution verifier (AWS Config).
All of these exist. Competing with them is a losing strategy.

Evidra is an **independent inspector** for infrastructure automation
operations. AI agents are one use case; CI and scripted automation are
equally in scope. The primary customer is **the platform team running
automation** — their problem is "How do I prove this automation behaves correctly?"

### The Accountability Chain

```
Agent wants to act
       ↓
Agent asks Evidra: "I want to do X"
       ↓
Evidra evaluates risk, issues signed Prescription
       ↓
Agent acts (with its own tools, its own credentials)
       ↓
Agent reports back: "I did Y"
       ↓
Evidra records Report, compares to Prescription
       ↓
Evidence: prescription → report → verdict
```

If the agent lies in its report — that's the agent developer's
problem. Evidra recorded what it prescribed. If the agent doesn't
report back — that's a protocol violation, also recorded.

### What Evidra Does NOT Do

| Concern | Who handles it | Why not Evidra |
|---------|---------------|----------------|
| Block bad deployments | Gatekeeper, Kyverno | They sit in the API path |
| Verify agent did what it reported | K8s audit log, CloudTrail | They have API-level visibility |
| Remediate violations | Agent framework, gitops | Evidra has no write access |
| Scan running workloads | Falco, Trivy | Runtime security is different |
| Enforce prescription compliance | Nobody — advisory | Agent developers fix their agents |

Evidra's value is precisely that it does NONE of these things.
An enforcer's logs are self-serving. An independent inspector's
protocol is not.

### Zero-Privilege Model

Evidra has **zero infrastructure privileges**:
- No kubeconfig, no AWS credentials, no terraform state
- Reads the artifact the agent sends, evaluates risk, writes
  local evidence
- Attack surface: MCP stdio + local filesystem + signing key

### Value Proposition

| Customer | Value |
|----------|-------|
| Agent developers | Deviation rate as a quality metric. Bug discovery from prescription/report mismatches. |
| Enterprise security | 90-day pilot protocol for automation approval (including AI agents). Signed evidence for security review. |
| Compliance auditors | Independently verifiable prescriptions. Hash-linked, tamper-evident evidence chain. |

---

## Known Gaps

Consolidated from architecture review and PO recommendation.
Resolved items are captured in Key Decisions above.

### Open for v0.3.0

| Gap | Source | Priority | Effort |
|-----|--------|----------|--------|
| Wire CLI to evidence store | RECOMMENDATION §11.1 | P0 | 2 days |
| `evidra explain` command | RECOMMENDATION §11.2 | P0 | 1 day |
| Parse failures as evidence entries | RECOMMENDATION §3.4 | P1 | 0.5 day |
| canon_source field: adapter/external | RECOMMENDATION §2.5 | P1 | 0.5 day |
| SARIF scanner integration | RECOMMENDATION §8.2 | P1 | 3 days |
| Scorecard run metadata / version in all outputs | RECOMMENDATION §7.1, §11.3 | P1 | 1 day |
| Required fields table in docs | RECOMMENDATION §6.2 | P1 | 0.5 day |
| Safety floor in scoring | RECOMMENDATION §4.3 | P2 | 0.5 day |
| Scorecard breakdown by tool/scope | RECOMMENDATION §7.3 | P2 | 1 day |
| TF unknown values marker | RECOMMENDATION §3.3 | P2 | 0.5 day |
| Security section in README | RECOMMENDATION §9.1 | P2 | 0.5 day |

### Evidence Format (v0.3.0 breaking change)

v0.3.0 introduces a new evidence format (`EvidenceEntry` envelope
per `EVIDRA_CORE_DATA_MODEL.md §5`) that is **incompatible with
v0.2.0 evidence files**. The legacy format used `PolicyDecision`,
`PolicyRef`, and `BundleRevision` fields from the OPA-era model.

**No migration path is provided.** v0.3.0 is a clean break.
Existing v0.2.0 evidence files should be archived or deleted
before running v0.3.0. The `evidra scorecard` command will reject
evidence files that do not conform to the v0.3.0 schema.

### Deferred to v0.5.0+

| Gap | Source | Reason |
|-----|--------|--------|
| Actor auth_context / OIDC | RECOMMENDATION §2.4 | Requires evidra-api |
| Forward integrity + server receipts | RECOMMENDATION §2.3 | Requires evidra-api |
| Label provenance | RECOMMENDATION §5.1 | Requires verification source |
| Intent fingerprinting beyond operation_class | RECOMMENDATION §5.2 | Enhancement, not blocking |
| Per-session scorecard grouping | Architecture review P0-A | session_id captured in v0.3.0, scoring grouped by actor+period only |

### Explicitly Rejected

| Item | Source | Reason |
|------|--------|--------|
| Learning mode / baselines | RECOMMENDATION §4.1 | Violates "no ML, no adaptive thresholds" principle |
| Batch signing replacing per-entry | RECOMMENDATION §10 | Already built, no benefit from ripping out |
| Remove resource_shape_hash | RECOMMENDATION §10 | Needed for retry_loop, already built |

---

## Outputs

| Output | Command | Purpose |
|--------|---------|---------|
| Scorecard | `evidra scorecard --actor X --period 30d` | Per-agent reliability report |
| Explain | `evidra explain --actor X` | Signal-level breakdown with weights |
| Comparison | `evidra compare --actors X,Y --tool kubectl` | Side-by-side agent comparison |
| Validate | `evidra validate --public-key key.pem` | Evidence chain + signature verification |
| Ingest | `evidra ingest-findings --sarif report.sarif` | Standalone SARIF findings ingestion |
| Fleet | `evidra fleet --period 30d` | All agents at a glance |
| Metrics | `GET /metrics` | Prometheus counters (low cardinality) |
| CI check | GitHub Actions workflow | Score on PR, fail below threshold |

Details: [Benchmark](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) §11-12

---

## Libraries (2 external dependencies)

| Adapter | Library | Binary cost | Delivery |
|---------|---------|-------------|----------|
| Kubernetes | `k8s.io/apimachinery` (unstructured) | ~2MB | v0.3.0 |
| Terraform | `github.com/hashicorp/terraform-json` | ~200KB | v0.3.0 |
| Helm | reuses K8s adapter | 0 | v0.3.0 (via K8s) |
| ArgoCD | reuses K8s adapter (CRD) | 0 | Spec reserved (v0.5.0+) |
| Generic | stdlib + `gopkg.in/yaml.v3` | ~100KB | v0.3.0 |

Total: ~2.3MB. Compare: OPA alone was ~15MB.

Details: [Canonicalization Contract v1](CANONICALIZATION_CONTRACT_V1.md) §16

---

## Testing

Golden corpus (10 cases) + action snapshots (4 cases) + noise
immunity (50 subtests) + shape_hash sensitivity (1 test).
~65 tests, ~105 lines of code.

Version bump: `EVIDRA_UPDATE_GOLDEN=1 go test -run TestGolden -update`

Details: [Canonicalization Contract §19](CANONICALIZATION_CONTRACT_V1.md)

---

## Implementation Roadmap

### v0.3.0 — Launch (cover 80% of market)
1. Canonicalization contract (frozen)
2. Terraform adapter + golden corpus
3. K8s adapter + golden corpus (covers kubectl + Helm)
4. Generic adapter (pre-canonicalized: Ansible, Pulumi, CF, custom)
5. **SARIF scanner integration** (Checkov, Trivy, tfsec, KICS, Terrascan, Snyk)
6. Five signal detectors + risk matrix + catastrophic detectors
7. Reliability score + evidence chain
8. **evidra CLI**: prescribe, report, scorecard, compare
9. **evidra-mcp**: MCP tools (Claude Code, Cursor, Windsurf)
10. **GitHub Action** on Marketplace
11. **GitLab CI template** in repo
12. **Docker images** on GHCR (multi-arch)
13. **MCP registry** entry

### v0.3.x — Distribution (no features, only reach)
14. Homebrew tap + curl install script
15. Terraform External Data Source example
16. Blog: "Checkov + Evidra" and "Pipeline reliability in 5 minutes"

### v0.4.0 — Team + SDKs
17. Evidence forwarding (push to remote URL)
18. Prometheus /metrics endpoint
19. **Python SDK** on PyPI (LangChain, CrewAI, AutoGen)
20. **TypeScript SDK** on npm (Vercel AI SDK, LangChain.js)
21. **risk_ignorance signal** (agent ignored scanner findings)
22. Grafana dashboard + ArgoCD notification example
23. CircleCI Orb

### v0.5.0 — Platform
24. **evidra-api**: HTTP backend
25. OpenTelemetry export
26. Multi-tenant, API keys, aggregated scorecards
27. Spacelift / env0 integration
28. Slack / PagerDuty alerts

### Scanner Integration Strategy

Evidra does NOT validate infrastructure. Checkov, Trivy, tfsec
already do this. Instead, **Evidra consumes their SARIF output**
as risk context on prescriptions.

```
terraform plan → tfplan.json
         │
    ┌────┼────┐
    ▼    ▼    ▼
 checkov trivy tfsec    →  scanner_report.sarif
                                  │
                                  ▼
                    evidra prescribe --scanner-report scanner_report.sarif
```

One `--scanner-report` flag. Any SARIF-compatible scanner. No
per-scanner integration code. Scanner findings become risk_tags
on the prescription, elevate risk_level, and are recorded as
independent evidence entries (type=finding).

### Integration Priority Matrix

**Infrastructure tools** (by market adoption):

| Tool | Integration | Version |
|------|------------|---------|
| Terraform / OpenTofu | Built-in adapter (plan JSON) | v0.3.0 |
| Kubernetes (kubectl) | Built-in adapter (YAML) | v0.3.0 |
| Helm | Via K8s adapter (template output) | v0.3.0 |
| Ansible, Pulumi, CloudFormation | Pre-canonicalized (no adapter code) | v0.3.0 (ready) |
| ArgoCD | Built-in adapter | v0.5.0 |

**Scanners** (all via SARIF — one parser covers all):
Checkov, Trivy, tfsec, KICS, Terrascan, Snyk — all v0.3.0.

**CI/CD:** GitHub Action + GitLab CI template (v0.3.0). Jenkins
and Azure DevOps work with bare CLI.

**Agent frameworks:** MCP-native (Claude Code, Cursor, Windsurf)
via evidra-mcp (v0.3.0). Python/TypeScript SDKs at v0.4.0.

### What NOT to Build

- Full Terraform provider (External Data Source is enough)
- Kubernetes operator (Evidra is CLI/sidecar, not a controller)
- Web dashboard before v0.5.0 (scorecard CLI is sufficient)
- Per-scanner integration code (SARIF is the standard)
- Custom agent protocol (MCP is the standard)
- Chef / Puppet / SaltStack adapters (pre-canonicalized path covers them)

### Publication Surface (v0.3.0)

| Registry | Package |
|----------|---------|
| GitHub Marketplace | evidra-io/setup-evidra Action |
| GHCR | evidra, evidra-mcp images (multi-arch) |
| MCP Registry | evidra server entry |
| Homebrew | evidra-io/tap/evidra |
| GitLab CI catalog | evidra template |

---

## Document Map

```
                    ┌──────────────────────────────────┐
                    │  ARCHITECTURE OVERVIEW (this doc) │
                    └──────────────────┬───────────────┘
                                       │
       ┌───────────────────┬───────────┼───────────────────┐
       ▼                   ▼           ▼                   ▼
┌────────────┐   ┌──────────────┐  ┌────────────┐  ┌────────────┐
│ CONTRACTS  │   │ SPECS        │  │ CONSUMER   │  │ EXAMPLES   │
│            │   │              │  │            │  │            │
│ Canon [1]  │   │ Signal [2]   │  │ Benchmark  │  │ E2E [7]    │
│ Data Mdl   │   │ Protocol [5] │  │ [4]        │  │            │
│ [3]        │   │              │  │ Bench CLI  │  │            │
│            │   │              │  │ [6]        │  │            │
└────────────┘   └──────────────┘  └────────────┘  └────────────┘
```

### Active Documents (8 total)

| # | Document | Role | Type |
|---|----------|------|------|
| 1 | [CANONICALIZATION_CONTRACT_V1.md](CANONICALIZATION_CONTRACT_V1.md) | **Adapter interface, digest rules, noise lists, compatibility, testing** | **Normative** |
| 2 | [EVIDRA_SIGNAL_SPEC.md](EVIDRA_SIGNAL_SPEC.md) | **Signal definitions, metric contracts, scoring formula, conformance** | **Normative** |
| 3 | [EVIDRA_CORE_DATA_MODEL.md](EVIDRA_CORE_DATA_MODEL.md) | **Core data model: CanonicalAction, Prescription, Report, EvidenceEntry, Signal, Scorecard** | **Normative** |
| 4 | [EVIDRA_AGENT_RELIABILITY_BENCHMARK.md](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) | Scoring, comparison, benchmark methodology, protocol, risk analysis | Consumer |
| 5 | [EVIDRA_PROTOCOL.md](EVIDRA_PROTOCOL.md) | **Integration protocol: session/run lifecycle, correlation model, scope dimensions, actor identity, findings ingestion** | **Normative** |
| 6 | [EVIDRA_BENCHMARK_CLI.md](EVIDRA_BENCHMARK_CLI.md) | Benchmark CLI: `evidra benchmark run`, dataset contract, exit codes, CI integration, leaderboard | Consumer |
| 7 | [EVIDRA_ARCHITECTURE_OVERVIEW.md](EVIDRA_ARCHITECTURE_OVERVIEW.md) | Entry point, strategic positioning, inspector model, roadmap, document map | Non-normative |
| 8 | [EVIDRA_END_TO_END_EXAMPLE_v2.md](EVIDRA_END_TO_END_EXAMPLE_v2.md) | Worked examples, failure cases | Non-normative |

Historical drafts are not part of the active architecture set.
Active architecture decisions are captured only in the active documents list above.

### Documents Not in Repo (referenced only)

| Document | Where | Notes |
|----------|-------|-------|
| ANTI_GOODHART_BACKLOG_ADDENDUM.md | Backlog | Accepted for v0.5.0+. Not implemented in v1. |
| EVIDRA_END_TO_END_EXAMPLE.md | Superseded | Replaced by v2. |

---

## Quick Reference: Where to Find What

| Question | Document | Type |
|----------|----------|------|
| What are the five signals? | **Signal Spec** | Normative |
| How are signals detected? | **Signal Spec** §Signal 1-5 | Normative |
| What metrics are exported? | **Signal Spec** §Metric Registry | Normative |
| What labels are forbidden? | **Signal Spec** §Label Rules | Normative |
| What's in CanonicalAction? | **Canon Contract** §Front Contract | Normative |
| What's in intent_digest? | **Canon Contract** §Digest Rules | Normative |
| What adapters are implemented? | **Canon Contract** §Adapter Status | Normative |
| What's a breaking change? | **Canon Contract** §Compatibility | Normative |
| How is the score computed? | Benchmark §3 | Consumer |
| How does comparison work? | Benchmark §4-5 | Consumer |
| What does prescribe/report look like? | End-to-End Example | Non-normative |
| What fields are noise? | Canon Contract §4.5 | Normative |
| How are adapters tested? | Canon Contract §19 | Normative |
| Why inspector model? | Architecture Overview §Inspector Model | Non-normative |
| How is CI integrated? | Benchmark §12 | Non-normative |
| How does `evidra benchmark run` work? | Benchmark CLI §3 | Consumer |
| What are benchmark exit codes? | Benchmark CLI §5 | Consumer |
| What's in results.json? | Benchmark CLI §4 | Consumer |
| How does the GitHub Action work? | Benchmark CLI §10 | Consumer |
| What's in Prescription/Report? | **Data Model** §2-3 | Normative |
| What's in EvidenceEntry? | **Data Model** §5 | Normative |
| What is session_id / span model? | **Protocol** §1-3 | Normative |
| What are scope dimensions? | **Protocol** §5 | Normative |
| How do validators ingest findings? | **Protocol** §7 | Normative |
| What's the confidence model? | Architecture Overview §Confidence Model | Non-normative |
