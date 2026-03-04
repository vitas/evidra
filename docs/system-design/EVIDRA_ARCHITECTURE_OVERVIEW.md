# Evidra — Architecture Overview

## Status
Entry point. Start here. This document links to everything else.

This is the **single architecture reference** for Evidra. It consolidates
content from the architecture review and PO recommendation documents
(archived in `done/`).

## Document Type
**Non-normative.** This is an overview for orientation. It does
not define contracts. Normative sources:
- **EVIDRA_SIGNAL_SPEC.md** — signal definitions, metric contracts
- **CANONICALIZATION_CONTRACT_V1.md** — adapter interface, digests

## One-liner
Evidra is the standard signal and metrics layer for infrastructure
automation.

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

### Three Components

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

**evidra-mcp** — sidecar for AI agents. MCP protocol (stdio/SSE).
Exposes prescribe + report tools. Local evidence JSONL. Forwards
to evidra-api if configured. v0.3.0.

**evidra CLI** — for CI pipelines. Same protocol, shell wrapper.
`evidra prescribe`, `evidra report`, `evidra scorecard`. v0.3.0.

**evidra-api** — centralized backend. Aggregates evidence from all
sources. Scorecards, comparison, Prometheus metrics. Multi-tenant.
v0.5.0.

All three share the same core: canon adapters, risk analysis,
signal detectors, evidence chain. v0.3.0 works fully local
without evidra-api.

No OPA. No Rego. No policy engine. No deny.

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
    - Retry Loop: same intent_digest + same shape_hash, N times in T minutes?
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

Why: [Inspector Model Architecture](EVIDRA_INSPECTOR_MODEL_ARCHITECTURE.md) §3

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

Source: Architecture Review §3.2

### risk_tags Belong to Prescription, Not CanonicalAction
risk_tags are computed by catastrophic risk detectors AFTER
canonicalization. They live in Prescription, not CanonicalAction.
canonical_action is adapter output (deterministic, testable).
risk_tags are detector output (separate concern).

Source: Architecture Review §3.1

### TTL Detection at Scorecard Time, Not Real-Time
TTL detection happens when `evidra scorecard` scans the evidence
chain — it finds prescriptions without matching reports and
retroactively marks them as protocol violations. No background
process needed. Real-time TTL detection is a v0.5.0 feature of
evidra-api (long-running, can run periodic scans).

Source: Architecture Review §1.1

### Prescription Matching: 1:1, ULID-Keyed, Four Violation Types
- prescription_id is globally unique (ULID)
- First report wins; second report → duplicate_report violation
- Unknown prescription_id → unprescribed_action violation
- Cross-actor report → cross_actor_report violation
- Batched apply (e.g. terraform apply with 10 resources) = one
  prescription with resource_count=10, one report

Source: PO Recommendation §2.2

### Pre-Canonicalized Path: Accepted Trade-Off
When a tool sends its own canonical_action, Evidra trusts the
resource identity. Pre-canonicalized path trades accuracy for reach.
Signals are only as good as the input. Entries are marked with
`canon_source=external` so scorecards can show what percentage of
data is self-reported. Blast radius may be inaccurate for
pre-canonicalized input — documented as a known limitation.

Source: PO Recommendation §2.5

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
  actor.id, actor.provenance (or actor.origin).
- **trace_id** is the primary correlation key for inspection
  sessions.
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
  findings; Evidra records them as evidence and may transform
  them into behavioral signals.
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
| Retry Loop | Same intent + same content, repeated after failure | retry_events / total_ops |
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
    "id":          "claude-code",      // stable identifier
    "origin":      "mcp"              // "mcp" | "cli" | "api"
  },
  "tenant_id":     "",                 // empty in local mode, required in service mode (v0.5.0+)
  "trace_id":      "01JNG...",         // correlation key for inspection session

  // === Digests (present on prescription and report) ===
  "artifact_digest": "sha256:abc...",  // SHA256 of raw artifact bytes
  "intent_digest":   "sha256:def...", // SHA256 of canonical JSON (prescription only)

  // === Payload (type-specific) ===
  "payload":       { },                // see Payload by Type below

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
| `finding` | Scanner output attached to prescription | tool, rule_id, severity, resource, message |
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

### Schema Rules

1. **entry_id** is a ULID. Monotonically increasing within a single writer.
2. **type** is a closed enum. Adding a new type requires a spec version bump.
3. **ts** is always UTC. Clock skew between writers is accepted; ordering is by entry position in the chain, not by timestamp.
4. **actor** is mandatory on prescription and report. MAY be empty on signal and receipt entries.
5. **tenant_id** is empty string in local mode. In service mode (v0.5.0+), every entry MUST have a non-empty tenant_id derived from auth, not self-reported.
6. **trace_id** correlates prescribe/report pairs within one inspection session. Same ULID for both entries of a pair.
7. **Digests** use `sha256:` prefix. artifact_digest is present on both prescription and report (for drift detection). intent_digest is prescription-only.
8. **Versions** are mandatory on every entry. If an entry is written by a component that doesn't know scoring_version (e.g. CLI prescribe), it writes `""` — never omits the field.
9. **previous_hash** creates the append-only chain. First entry in a segment has `previous_hash: ""`.
10. **Entries are immutable.** Corrections are new entries, not mutations.

---

## Strategic Positioning

See **EVIDRA_STRATEGIC_MOAT_AND_STANDARDIZATION.md** for the full
strategic analysis. Key points below.

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

### Deferred to v0.5.0+

| Gap | Source | Reason |
|-----|--------|--------|
| Actor auth_context / OIDC | RECOMMENDATION §2.4 | Requires evidra-api |
| Forward integrity + server receipts | RECOMMENDATION §2.3 | Requires evidra-api |
| Label provenance | RECOMMENDATION §5.1 | Requires verification source |
| Intent fingerprinting beyond operation_class | RECOMMENDATION §5.2 | Enhancement, not blocking |

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
| Comparison | `evidra compare --actors X,Y --tool kubectl` | Side-by-side agent comparison |
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

Details: [Test Strategy](EVIDRA_CANONICALIZATION_TEST_STRATEGY.md)

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

Details: [Integration Roadmap](EVIDRA_INTEGRATION_ROADMAP.md)

---

## Document Map

```
                    ┌──────────────────────────────────┐
                    │  ARCHITECTURE OVERVIEW (this doc) │
                    └──────────────────┬───────────────┘
                                       │
       ┌───────────────────┬───────────┼───────────┬──────────────────┐
       ▼                   ▼           ▼           ▼                  ▼
┌────────────┐   ┌──────────────┐  ┌────────┐  ┌──────────┐  ┌────────────┐
│ DESIGN     │   │ CONTRACTS    │  │ SPECS  │  │ EXAMPLES │  │ OPERATIONS │
│            │   │              │  │        │  │          │  │            │
│ Benchmark  │   │ Canon v1 [2] │  │ Signal │  │ E2E [4]  │  │ Baseline   │
│ [1]        │   │ Tests [3]    │  │ Spec   │  │          │  │ Migration  │
│ Inspector  │   │              │  │ [5]    │  │          │  │ Bootstrap  │
│ [6]        │   │              │  │        │  │          │  │ Post-Migr. │
└────────────┘   └──────────────┘  └────────┘  └──────────┘  └────────────┘
```

### Active Documents (current architecture)

| # | Document | Role | Type |
|---|----------|------|------|
| 1 | [EVIDRA_SIGNAL_SPEC.md](EVIDRA_SIGNAL_SPEC.md) | **Signal definitions, metric contracts, scoring formula, conformance** | **Normative** |
| 2 | [CANONICALIZATION_CONTRACT_V1.md](CANONICALIZATION_CONTRACT_V1.md) | **Adapter interface, digest rules, noise lists, compatibility** | **Normative** |
| 3 | [EVIDRA_AGENT_RELIABILITY_BENCHMARK.md](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) | Scoring, comparison, benchmark methodology, protocol, risk analysis | Consumer |
| 4 | [EVIDRA_ARCHITECTURE_OVERVIEW.md](EVIDRA_ARCHITECTURE_OVERVIEW.md) | Entry point, document map, component overview | Non-normative |
| 5 | [EVIDRA_INSPECTOR_MODEL_ARCHITECTURE.md](EVIDRA_INSPECTOR_MODEL_ARCHITECTURE.md) | Inspector model rationale | Non-normative |
| 6 | [EVIDRA_CANONICALIZATION_TEST_STRATEGY.md](EVIDRA_CANONICALIZATION_TEST_STRATEGY.md) | Golden corpus, test approach | Non-normative |
| 7 | [EVIDRA_END_TO_END_EXAMPLE_v2.md](EVIDRA_END_TO_END_EXAMPLE_v2.md) | Worked examples, failure cases | Non-normative |

### Operational Documents

| # | Document | Purpose | Status |
|---|----------|---------|--------|
| 8 | [EVIDRA_CURRENT_STATE_BASELINE.md](EVIDRA_CURRENT_STATE_BASELINE.md) | v0.2.0 codebase inventory | Reference |
| 9 | [EVIDRA_MIGRATION_MAP.md](EVIDRA_MIGRATION_MAP.md) | Migration instructions: file-by-file | Reference |
| 10 | [EVIDRA_BOOTSTRAP_PROMPT.md](EVIDRA_BOOTSTRAP_PROMPT.md) | Claude Code prompt for migration | Reference |
| 11 | [EVIDRA_POST_MIGRATION_UPDATE.md](EVIDRA_POST_MIGRATION_UPDATE.md) | Post-migration: Dockerfiles, MCP schemas, prompts | Active |

### Historical Documents (design evolution)

| # | Document | Purpose | Status |
|---|----------|---------|--------|
| 12 | [EVIDRA_TELEMETRY_PLANE_architect_review.md](EVIDRA_TELEMETRY_PLANE_architect_review.md) | Telemetry plane review. Led to tiered metrics. | Historical |
| 13 | [EVIDRA_SIGNALS_ENGINE_architect_review.md](EVIDRA_SIGNALS_ENGINE_architect_review.md) | Signals engine review. Reduced from 10 signals to 5. | Historical |
| 14 | [CANONICALIZATION_CONTRACT_architect_review.md](CANONICALIZATION_CONTRACT_architect_review.md) | Review of canonicalization draft. Led to v1 contract. | Historical |
| 15 | [EVIDRA_STRATEGIC_DIRECTION.md](EVIDRA_STRATEGIC_DIRECTION.md) | Product strategy. Prometheus analogy. | Strategic |
| 16 | [EVIDRA_STRATEGIC_MOAT_AND_STANDARDIZATION.md](EVIDRA_STRATEGIC_MOAT_AND_STANDARDIZATION.md) | Moat analysis. Defensibility, standardization path. | Strategic |
| 17 | [EVIDRA_INTEGRATION_ROADMAP.md](EVIDRA_INTEGRATION_ROADMAP.md) | Integration plan. GH Action, GitLab CI, Docker, MCP registry, SDKs. | Active |

### Archived Documents (consolidated into this overview)

Moved to `done/`. Resolved items captured in Key Decisions and
Architecture Invariants above. Unresolved items in Known Gaps.

| Document | Original role |
|----------|--------------|
| [EVIDRA_ARCHITECTURE_REVIEW.md](done/EVIDRA_ARCHITECTURE_REVIEW.md) | Gap analysis, overengineering review |
| [EVIDRA_ARCHITECTURE_RECOMMENTATION_V1.md](done/EVIDRA_ARCHITECTURE_RECOMMENTATION_V1.md) | PO triage with P0/P1/P2 items |
| [EVIDRA_ARCHITECTURE_INVARIANTS.md](done/EVIDRA_ARCHITECTURE_INVARIANTS.md) | Non-negotiable invariants |

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
| How are adapters tested? | Test Strategy §1 | Non-normative |
| Why inspector model? | Inspector Model §3 | Non-normative |
| How is CI integrated? | Benchmark §12 | Non-normative |
