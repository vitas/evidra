# Evidra — Architecture Overview

## Status
Entry point. Start here. This document links to everything else.

## One-liner
Evidra is the standard signal and metrics layer for infrastructure
automation.

---

## Strategic Positioning

Evidra is **behavioral telemetry for automation** — the same way
Prometheus is metrics for infrastructure.

```
Infrastructure observability stack:
  Metrics → Prometheus
  Logs    → Loki / Elasticsearch
  Traces  → OpenTelemetry
  Automation behavior → Evidra          ← new layer
```

Evidra is NOT a policy engine. NOT a security scanner. NOT runtime
enforcement. It measures, records, and scores automation behavior
through standard signals.

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

## Document Map

```
                    ┌──────────────────────────────────┐
                    │  ARCHITECTURE OVERVIEW (this doc) │
                    └──────────────────┬───────────────┘
                                       │
            ┌──────────────────────────┼──────────────────────────┐
            ▼                          ▼                          ▼
   ┌────────────────┐      ┌────────────────────┐     ┌────────────────────┐
   │ DESIGN         │      │ CONTRACTS          │     │ EXAMPLES           │
   │                │      │                    │     │                    │
   │ Benchmark [1]  │      │ Canonicalization   │     │ End-to-End [5]     │
   │ Inspector [2]  │      │ Contract [3]       │     │                    │
   │                │      │ Test Strategy [4]  │     │                    │
   └────────────────┘      └────────────────────┘     └────────────────────┘
            │
            ▼
   ┌────────────────┐
   │ HISTORY        │
   │                │
   │ Telemetry [6]  │
   │ Signals [7]    │
   │ Canon Review[8]│
   └────────────────┘
```

### Active Documents (current architecture)

| # | Document | Purpose | Status |
|---|----------|---------|--------|
| 1 | [EVIDRA_AGENT_RELIABILITY_BENCHMARK.md](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) | **Primary design doc.** Signals, scoring, benchmark tables, risk analysis, protocol, CI integration, golden path. | Active |
| 2 | [EVIDRA_INSPECTOR_MODEL_ARCHITECTURE.md](EVIDRA_INSPECTOR_MODEL_ARCHITECTURE.md) | Inspector model rationale. Why prescribe/report, why no execution binding, why zero-privilege. | Active (foundational) |
| 3 | [CANONICALIZATION_CONTRACT_V1.md](CANONICALIZATION_CONTRACT_V1.md) | **Canonicalization ABI.** Adapter specs, library decisions, noise lists, identity extraction, guarantees table, versioning. | Active (frozen contract) |
| 4 | [EVIDRA_CANONICALIZATION_TEST_STRATEGY.md](EVIDRA_CANONICALIZATION_TEST_STRATEGY.md) | Golden corpus, noise immunity, shape_hash sensitivity, fuzz strategy. ~65 tests, ~105 lines. | Active |
| 5 | [EVIDRA_END_TO_END_EXAMPLE_v2.md](EVIDRA_END_TO_END_EXAMPLE_v2.md) | Worked example: MCP agent flow, CI flow, scorecard output, failure cases. | Active |

### Historical Documents (design evolution)

| # | Document | Purpose | Status |
|---|----------|---------|--------|
| 6 | [EVIDRA_TELEMETRY_PLANE_architect_review.md](EVIDRA_TELEMETRY_PLANE_architect_review.md) | Telemetry plane review. Led to tiered metrics, agent scorecard concept. | Historical |
| 7 | [EVIDRA_SIGNALS_ENGINE_architect_review.md](EVIDRA_SIGNALS_ENGINE_architect_review.md) | Signals engine review. Reduced from 10 signals to 5. Introduced baselines discussion. | Historical |
| 8 | [CANONICALIZATION_CONTRACT_architect_review.md](CANONICALIZATION_CONTRACT_architect_review.md) | Review of the original canonicalization draft. Led to v1 contract. | Historical |

### Documents Not in Repo (referenced only)

| Document | Where | Notes |
|----------|-------|-------|
| ANTI_GOODHART_BACKLOG_ADDENDUM.md | Backlog | Accepted for v0.5.0+. Not implemented in v1. |
| EVIDRA_END_TO_END_EXAMPLE.md | Superseded | Replaced by v2. |

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

## Implementation Roadmap

### v0.3.0 — Local (2 binaries, no server)
1. Canonicalization contract (frozen)
2. K8s adapter + golden corpus
3. Terraform adapter + golden corpus
4. **evidra CLI**: prescribe, report, scorecard, compare
5. **evidra-mcp**: prescribe + report MCP tools for agents
6. Five signal detectors
7. Risk matrix + catastrophic detectors
8. Reliability score computation
9. Evidence chain with canon versioning
10. Local-only: everything reads/writes JSONL on disk

### v0.4.0 — Team (local + forward)
11. Evidence forwarding (push to remote URL)
12. `evidra fleet` — all agents at a glance (local)
13. Prometheus /metrics endpoint (embedded in evidra-mcp)
14. actor_meta labels for comparison
15. CI integration: GitHub Actions, GitLab CI examples

### v0.5.0 — Platform (3 binaries)
16. **evidra-api**: HTTP backend, receives forwarded evidence
17. Aggregated scorecards across agents and pipelines
18. Multi-agent comparison dashboard (web UI)
19. Multi-tenant with API keys
20. Signed PDF scorecard export

### v0.6.0 — Ecosystem
21. Agent framework SDKs
22. Public benchmark registry (opt-in)
23. LangSmith/Langfuse correlation
24. Compliance report generation

Details: [Benchmark](EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) §10

---

## Testing

Golden corpus (10 cases) + action snapshots (4 cases) + noise
immunity (50 subtests) + shape_hash sensitivity (1 test).
~65 tests, ~105 lines of code.

Version bump: `EVIDRA_UPDATE_GOLDEN=1 go test -run TestGolden -update`

Details: [Test Strategy](EVIDRA_CANONICALIZATION_TEST_STRATEGY.md)

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

---

## Quick Reference: Where to Find What

| Question | Document | Section |
|----------|----------|---------|
| What are the five signals? | Benchmark | §2 |
| How is the score computed? | Benchmark | §3 |
| How does comparison work? | Benchmark | §4-5 |
| What does prescribe/report look like? | End-to-End Example | Part 1 |
| What libraries does Evidra use? | Canonicalization Contract | §16 |
| What fields are noise? | Canonicalization Contract | §4.5 |
| What's in intent_digest? | Canonicalization Contract | §2 |
| What are the canonicalization guarantees? | Canonicalization Contract | §12 |
| How is CI integrated? | Benchmark | §11 |
| What happens when the agent crashes? | End-to-End Example | Failure Cases |
| How are adapters tested? | Test Strategy | §1 |
| Why inspector model? | Inspector Model | §3 |
| Why no OPA? | Benchmark | §7 |
| What's the risk matrix? | Benchmark | §7 |
| Architecture evolution history? | Telemetry / Signals reviews | Full docs |
