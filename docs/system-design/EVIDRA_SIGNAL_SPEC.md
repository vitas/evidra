# Evidra Signal Specification v1.0

## What This Is
An open specification for infrastructure automation behavior
telemetry. Like OpenTelemetry standardized distributed traces,
Evidra Signal Spec standardizes automation behavior signals.

```
OpenTelemetry : distributed traces = Evidra Signal Spec : automation behavior
```

Any tool that modifies infrastructure can emit Evidra signals.
Any platform can consume them. The spec is the contract between
producers and consumers.

## Status
Stable. All eight signals are v1.1 stable (risk_escalation added in v1.1).

## Document Type
**Normative.** This is the single source of truth for signal
definitions, metric contracts, and scoring formula. The key words
"MUST", "MUST NOT", "SHOULD", "MAY" in this document are to be
interpreted as described in RFC 2119.

Other documents reference this spec but do not override it:
- EVIDRA_AGENT_RELIABILITY_BENCHMARK.md is a consumer (scoring, comparison)
- docs/ARCHITECTURE.md is non-normative (overview)

---

## Versioning

### Spec version
This document is **Signal Spec v1.0**. Version is independent of
Evidra product version.

### What is a breaking change

| Change | Breaking? | Requires |
|--------|-----------|----------|
| Rename a signal | YES | Major bump (v2.0) |
| Remove a signal | YES | Major bump (v2.0) |
| Change detection algorithm semantics | YES | Major bump (v2.0) |
| Change metric name or type | YES | Major bump (v2.0) |
| Remove a metric label | YES | Major bump (v2.0) |
| Change score formula structure | YES | Major bump (v2.0) |
| Change default parameter value (affecting detection) | YES | Major bump (v2.0) |
| Add new signal | NO | Minor bump (v1.1) |
| Add new sub-signal to existing signal | NO | Minor bump (v1.1) |
| Add new optional label to metric | NO | Minor bump (v1.1) |
| Change default weights | NO | Minor bump (v1.1) |
| Add new optional field to SignalEvent | NO | Minor bump (v1.1) |
| Clarify wording without changing semantics | NO | Patch (v1.0.x) |

### Mixed versions
If a scoring window contains evidence from two spec versions
(e.g. signals emitted under v1.0 and v1.1), the scorer MUST:
- Use the NEWER spec version for detection
- Log a warning: "Mixed signal spec versions in scoring window"
- NOT reject older evidence

### Migration
When bumping from vN to vN+1:
- Both versions MUST be emitted simultaneously for at least one
  minor release (transition period)
- Old version deprecated, new version active
- After transition period, old version removed

---

## Metric Registry

All Evidra metrics follow these rules. Implementations MUST NOT
deviate from this registry.

### Namespace
All metrics MUST be prefixed `evidra_`.

### Metric catalog

| Metric | Type | Description |
|--------|------|-------------|
| `evidra_signal_total` | counter | Signal events detected |
| `evidra_prescriptions_total` | counter | Prescriptions issued |
| `evidra_reports_total` | counter | Reports received |
| `evidra_reliability_score` | gauge | Current reliability score (0-100) |
| `evidra_catastrophic_context_total` | counter | Catastrophic risk patterns detected |

### Label rules

**ALLOWED labels (low cardinality only):**

| Label | Applies to | Values |
|-------|-----------|--------|
| `agent` | all metrics | actor.id (SHOULD be < 50 unique values) |
| `tool` | signal, prescriptions, reports | kubectl, terraform, helm, argocd, other |
| `scope` | signal, prescriptions, reports | production, staging, development, unknown |
| `signal` | evidra_signal_total only | protocol_violation, artifact_drift, retry_loop, blast_radius, new_scope, repair_loop, thrashing |

**FORBIDDEN labels (MUST NOT be used):**

| Label | Why forbidden |
|-------|--------------|
| `prescription_id` | Unbounded cardinality |
| `artifact_digest` | Unbounded cardinality |
| `intent_digest` | Unbounded cardinality |
| `resource_name` | Unbounded cardinality |
| `namespace` | High cardinality in large clusters |
| `model_id` | Unbounded (use actor_meta in evidence, not in metrics) |
| `prompt_id` | Unbounded |

**Cardinality budget:** `agent` × `tool` × `scope` = N × 5 × 4.
For 10 agents: 200 series per metric. MUST stay under 10,000
total series across all metrics.

### Example /metrics output

```
# HELP evidra_signal_total Total signal events detected
# TYPE evidra_signal_total counter
evidra_signal_total{agent="claude-code",tool="kubectl",scope="production",signal="protocol_violation"} 3
evidra_signal_total{agent="claude-code",tool="kubectl",scope="production",signal="artifact_drift"} 1
evidra_signal_total{agent="ci-pipeline",tool="terraform",scope="production",signal="retry_loop"} 2

# HELP evidra_prescriptions_total Total prescriptions issued
# TYPE evidra_prescriptions_total counter
evidra_prescriptions_total{agent="claude-code",tool="kubectl",scope="production"} 847

# HELP evidra_reports_total Total reports received
# TYPE evidra_reports_total counter
evidra_reports_total{agent="claude-code",tool="kubectl",scope="production"} 845

# HELP evidra_reliability_score Current reliability score
# TYPE evidra_reliability_score gauge
evidra_reliability_score{agent="claude-code"} 97.2
evidra_reliability_score{agent="ci-pipeline"} 99.8

# HELP evidra_catastrophic_context_total Catastrophic risk patterns detected
# TYPE evidra_catastrophic_context_total counter
evidra_catastrophic_context_total{agent="claude-code"} 1
```

---

## Purpose

This document is the formal specification for Evidra signals.
It defines the detection contract, metric contract, and stability
guarantees for each signal.

Implementations MUST follow this spec to produce comparable
results. A conforming implementation emits the same signals
given the same evidence chain, regardless of language or platform.

---

## Signal Model

### Scope Boundaries (MUST NOT cross)

Signals and detectors measure **automation behavior**. They MUST
NOT become:

| MUST NOT become | Why |
|----------------|-----|
| Security policy engine | That's Gatekeeper/Kyverno/OPA territory |
| Compliance rule engine | That's Checkov/Trivy territory |
| Cost analysis tool | That's Infracost territory |
| Performance monitor | That's Prometheus/Datadog territory |

**Detectors produce signal context, not policy decisions.**

A detector says: "this operation touches a privileged container."
It does NOT say: "this operation is denied." It does NOT say:
"this violates SOC2." It does NOT say: "this costs $500/month."

**Growth test:** Before adding a detector, ask:
1. Has this pattern caused a production outage? → Yes → detector
2. Is this a style/compliance/cost concern? → Yes → out of scope
3. Would this make Evidra compete with security scanners? → Yes → out of scope

If Evidra's detector count exceeds 15, something is wrong.

### Evidence Plane Boundaries (MUST NOT cross)

Evidence is an **append-only log**. It MUST NOT become:

| MUST NOT become | Why |
|----------------|-----|
| Distributed log | Adds consensus, replication, partitioning |
| Blockchain | Adds mining, proof-of-work, decentralization |
| Database | Adds queries, indexes, transactions |
| Message queue | Adds consumers, offsets, backpressure |

Evidence plane: append. hash-link. sign. read. That's all.

Aggregation, querying, and indexing happen in evidra-api (v0.5.0),
not in the evidence plane itself. The evidence plane is a file.

### Scoring Transparency (MUST maintain)

Scoring MUST remain transparent and deterministic.

| MUST NOT use | Why |
|-------------|-----|
| Machine learning | Scores must be explainable without a model |
| Adaptive thresholds | Same evidence → same score, always |
| Statistical baselines | Adds complexity, reduces trust |
| Hidden weights | All weights visible and configurable |

Every score MUST be reproducible: given the same evidence chain
and the same parameters, any implementation MUST produce the
exact same score. No randomness. No learned parameters.

---

Every signal follows the same structure:

```
Signal:
  name:       unique identifier (snake_case)
  version:    semver (major.minor)
  status:     experimental | stable | deprecated
  input:      what the detector reads
  algorithm:  how the detector decides
  output:     what the detector produces
  metric:     Prometheus metric name and labels
  weight:     default weight in reliability score
```

A signal detector is a pure function:

```
detect(entry, evidence_chain) → SignalEvent | nil
```

No side effects. No external I/O. No state beyond the evidence
chain. Deterministic: same input → same output.

### Prescription Risk Field Contract

- Canonical field: `risk_details` (array of detector-emitted tags)
- Legacy compatibility field: `risk_tags` (deprecated)
- Consumers SHOULD read `risk_details` first, and MAY fallback to `risk_tags` for older evidence
- Producers SHOULD dual-write both fields during migration; removal target for `risk_tags` is v0.5.0

---

## Signal Registry

| Name | Version | Status | Weight |
|------|---------|--------|--------|
| protocol_violation | 1.0 | stable | 0.35 |
| artifact_drift | 1.0 | stable | 0.30 |
| retry_loop | 1.0 | stable | 0.20 |
| blast_radius | 1.0 | stable | 0.10 |
| new_scope | 1.0 | stable | 0.05 |
| repair_loop | 1.0 | stable | -0.05 |
| thrashing | 1.0 | stable | 0.15 |

---

## Signal 1: protocol_violation

### Identity
```
name:    protocol_violation
version: 1.0
status:  stable
```

### Detection Contract

**Input:** A prescription entry or report entry from the evidence
chain, plus the full chain for context.

**Algorithm:**

```
For each prescription in the scoring window:
  1. Find matching report (same prescription_id)
  2. If no matching report within TTL → FIRE (unreported_prescription)
  3. If matching report has different actor.id → FIRE (cross_actor_report)

For each report in the scoring window:
  1. Find matching prescription (same prescription_id)
  2. If no matching prescription → FIRE (unprescribed_action)
  3. If a previous report with same prescription_id exists → FIRE (duplicate_report)
```

**Parameters:**
- TTL: default 10 minutes, configurable per scorecard invocation
- TTL detection happens at scorecard computation time, not real-time

**Sub-signals:**

| Sub-signal | Trigger | Meaning |
|------------|---------|---------|
| unreported_prescription | prescription without report within TTL | Agent didn't report |
| unprescribed_action | report without matching prescription | Agent acted without prescribing |
| duplicate_report | second report for same prescription_id | Agent reported twice |
| cross_actor_report | report actor != prescription actor | Wrong agent reported |
| stalled_operation | unreported + no further agent activity | Agent is hung |
| crash_before_report | unreported + agent sent new prescribe | Agent crashed and restarted |
| report_without_digest | prescription had artifact_digest, report omits it | Drift detection disabled for this pair |

`report_without_digest` does not block the report from being recorded. It signals that
artifact drift detection is unavailable for this prescribe/report pair. An agent that
consistently omits artifact_digest at report time has a protocol compliance gap.

Sub-signals are informational breakdowns. All count as
protocol_violation in the score.

**Output:**
```go
type SignalEvent struct {
    Signal    string    // "protocol_violation"
    SubSignal string    // "unreported_prescription", "stalled_operation", etc.
    Timestamp time.Time
    EntryRef  string    // prescription_id or report entry_id that triggered
    Details   string    // human-readable description
}
```

### Metric Contract

```
evidra_signal_total{signal="protocol_violation", agent, tool, scope}
```

Counter. Incremented by 1 for each protocol violation detected.
Rate: `rate(evidra_signal_total{signal="protocol_violation"}[5m])`

### Score Contribution

```
violation_rate = protocol_violation_count / total_operations
penalty_contribution = 0.35 × violation_rate
```

total_operations = prescriptions + unprescribed reports (deduplicated).

---

## Signal 2: artifact_drift

### Identity
```
name:    artifact_drift
version: 1.0
status:  stable
```

### Detection Contract

**Input:** A report entry with its matching prescription.

**Algorithm:**

```
For each report with matching prescription:
  If prescription.artifact_digest != report.artifact_digest → FIRE
```

**Edge cases:**
- Report without matching prescription → protocol_violation, not drift
- Report with no artifact_digest → no drift check (field optional for
  tools that don't produce artifacts at report time)

**Output:**
```go
type SignalEvent struct {
    Signal    string    // "artifact_drift"
    Timestamp time.Time
    EntryRef  string    // report entry_id
    Details   string    // "prescribed sha256:abc..., reported sha256:def..."
}
```

### Metric Contract

```
evidra_signal_total{signal="artifact_drift", agent, tool, scope}
```

### Score Contribution

```
drift_rate = artifact_drift_count / total_reports
penalty_contribution = 0.30 × drift_rate
```

total_reports = reports with matching prescriptions (excludes
unprescribed reports — those are protocol_violation).

### Trust Model

Artifact drift measures protocol consistency, not ground truth.
Both digests are self-reported by the agent. An agent that lies
consistently (sends same digest both times but applies something
different) shows zero drift. Evidra detects inconsistency within
the protocol, not real-world compliance.

---

## Signal 3: retry_loop

### Identity
```
name:    retry_loop
version: 1.0
status:  stable
```

### Detection Contract

**Input:** A prescription entry, plus recent prescriptions from
the same actor.

**Algorithm:**

```
For each new prescription:
  Find all prescriptions from same actor within retry_window where:
    intent_digest matches AND
    resource_shape_hash matches AND
    previous operation was denied OR failed (exit_code != 0)
  
  If count >= retry_threshold → FIRE
```

**Parameters:**

Exact detector:
- retry_window: default 30 minutes
- retry_threshold: default 3 (third identical attempt fires)

Variant detector (runs alongside exact, results merged and deduplicated):
- retry_window: default 30 minutes (same window)
- variant_retry_threshold: default 5 — higher threshold to tolerate legitimate
  investigative troubleshooting where the operator genuinely changes the artifact
- Groups by (actor, tool, operation_class, scope_class) — ignores artifact content
- Detects agents stuck in an operational area without making real progress, even
  when each attempt looks syntactically different

**Key distinction:**
- Same intent_digest + same shape_hash → retry (agent sending
  identical content after failure) — detected at threshold 3
- Same intent_digest + different shape_hash → NOT retry by the exact
  detector (agent modified the artifact — this is fixing, not retrying)
- Same (actor, tool, operation_class, scope_class) regardless of digest
  → variant retry — detected at threshold 5 (see below)

**Output:**
```go
type SignalEvent struct {
    Signal    string    // "retry_loop"
    Timestamp time.Time
    EntryRef  string    // prescription_id that triggered
    Details   string    // "3rd identical attempt within 12min, all failed"
}
```

### Metric Contract

```
evidra_signal_total{signal="retry_loop", agent, tool, scope}
```

### Score Contribution

```
retry_rate = retry_loop_count / total_prescriptions
penalty_contribution = 0.20 × retry_rate
```

---

## Signal 4: blast_radius

### Identity
```
name:    blast_radius
version: 1.0
status:  stable
```

### Detection Contract

**Input:** A prescription with canonical_action.

**Algorithm:**

```
For each prescription:
  If canonical_action.operation_class == "destructive"
     AND canonical_action.resource_count > blast_threshold → FIRE
```

**Parameters:**
- blast_threshold: default 5 resources per destructive operation

**Scope:**
- Only fires on destructive operations (delete, destroy, uninstall)
- Mutating operations with high resource_count are not flagged
  (deploying 20 services is normal; deleting 20 is suspicious)

**Generic adapter limitation:** resource_count is always 1 for
generic adapter. Blast radius effectively disabled for unknown tools.
This is acceptable — the signal fires for K8s and Terraform where
resource counting is reliable.

**Pre-canonicalized limitation:** resource_count is caller-provided.
If the caller lies, blast_radius is inaccurate. Documented trade-off.

**Output:**
```go
type SignalEvent struct {
    Signal    string    // "blast_radius"
    Timestamp time.Time
    EntryRef  string    // prescription_id
    Details   string    // "destructive operation on 12 resources (threshold: 5)"
}
```

### Metric Contract

```
evidra_signal_total{signal="blast_radius", agent, tool, scope}
```

### Score Contribution

```
blast_rate = blast_radius_count / total_prescriptions
penalty_contribution = 0.10 × blast_rate
```

---

## Signal 5: new_scope

### Identity
```
name:    new_scope
version: 1.0
status:  stable
```

### Detection Contract

**Input:** A prescription with canonical_action, plus full evidence
chain history.

**Algorithm:**

```
scope_key = (actor.id, tool, operation_class, scope_class)

The first prescription in the evidence chain establishes the baseline
scope and is never flagged — penalizing cold start is not useful.

For each subsequent prescription:
  Search evidence chain for any prior prescription with same scope_key
  If no prior prescription → FIRE (first time this actor operates
  in this tool/operation_class/scope_class combination)
```

**Scope:** The very first prescription is always the baseline (no signal).
After that, fires once per new unique scope_key. Once a scope_key has
been seen, subsequent operations with the same key do not fire.

**Example:**
```
claude-code does kubectl/mutating/staging → first prescription → baseline (no fire)
claude-code does kubectl/mutating/staging → same scope → no fire
claude-code does kubectl/mutating/production → new scope → FIRE
claude-code does terraform/destructive/production → new scope → FIRE
```

**Output:**
```go
type SignalEvent struct {
    Signal    string    // "new_scope"
    Timestamp time.Time
    EntryRef  string    // prescription_id
    Details   string    // "first kubectl/mutating/production operation for claude-code"
}
```

### Metric Contract

```
evidra_signal_total{signal="new_scope", agent, tool, scope}
```

### Score Contribution

```
scope_rate = new_scope_count / total_prescriptions
penalty_contribution = 0.05 × scope_rate
```

new_scope has the lowest weight. It's informational — entering
a new scope is often legitimate (first deploy to production).

---

## Signal 6: repair_loop

### Identity
```
name:    repair_loop
version: 1.0
status:  stable
```

### Detection Contract

**Input:** All prescription entries with their matching reports.

**Algorithm:**

```
Group prescriptions by (actor, intent_digest).

For each group with 2+ prescriptions (sorted by timestamp):
  Track failure state:
    If report exit_code != 0 → record failure and artifact_digest
    If report exit_code == 0 AND prior failure exists
       AND artifact_digest differs from failed attempt → FIRE
    Success resets the failure tracking chain.
```

**Key distinction:**
- repair_loop fires when an agent fails, modifies the artifact,
  and then succeeds — indicating iterative self-correction.
- This is a **positive** signal (negative weight reduces penalty).

**Output:**
```go
type SignalEvent struct {
    Signal    string    // "repair_loop"
    Timestamp time.Time
    EntryRef  string    // prescription_id of the successful repair
    Details   string    // "repaired after failure with changed artifact"
}
```

### Metric Contract

```
evidra_signal_total{signal="repair_loop", agent, tool, scope}
```

### Score Contribution

```
repair_rate = repair_loop_count / total_prescriptions
penalty_contribution = -0.05 × repair_rate
```

Negative weight: repair_loop is a positive indicator. An agent
that self-corrects is more reliable than one that gives up.

---

## Signal 7: thrashing

### Identity
```
name:    thrashing
version: 1.0
status:  stable
```

### Detection Contract

**Input:** All prescription entries with their matching reports.

**Algorithm:**

```
Process prescriptions in timestamp order:
  If report exit_code == 0 → reset window (success clears state)
  If no report → skip (unknown state)
  If report exit_code != 0 → add intent_digest to window

  If distinct failed intent_digests in window >= threshold → FIRE
    (flag all entries in the window, then reset)
```

**Parameters:**
- thrashing_threshold: default 3 distinct failed intents

**Key distinction:**
- retry_loop: same intent repeated (agent retrying identical action)
- thrashing: different intents all failing (agent flailing across
  multiple approaches without success)

**Output:**
```go
type SignalEvent struct {
    Signal    string    // "thrashing"
    Timestamp time.Time
    EntryRef  string    // prescription_id(s) in the thrashing window
    Details   string    // "3 distinct failed intents without success"
}
```

### Metric Contract

```
evidra_signal_total{signal="thrashing", agent, tool, scope}
```

### Score Contribution

```
thrashing_rate = thrashing_count / total_prescriptions
penalty_contribution = 0.15 × thrashing_rate
```

Thrashing has a high weight because it indicates an agent that
is not converging — trying many different things without any success.

---

## Reliability Score Formula

```
score = 100 × (1 - penalty)

penalty = Σ(weight_i × rate_i) for all 8 signals

where:
  rate_i = signal_count_i / denominator_i
  
  denominators:
    protocol_violation:  total_operations
    artifact_drift:      total_reports (with matching prescriptions)
    retry_loop:          total_prescriptions
    blast_radius:        total_prescriptions
    new_scope:           total_prescriptions
```

Score range: 0–100. Clamped (never negative).

Minimum sample: 100 operations. Below that, score is not computed
("insufficient data").

Default weights:

```
protocol_violation:  0.35
artifact_drift:      0.30
retry_loop:          0.20
thrashing:           0.15
blast_radius:        0.10
new_scope:           0.05
repair_loop:        -0.05
```

Weights are configurable. Sum must equal 1.0.

---

## Stability Guarantees

### What is stable (v1.0)

- Signal names (the seven names above)
- Detection algorithms (the logic described above)
- Metric names and label keys
- Score formula structure
- Default parameter values

### What can change without version bump

- Default weights (tuning based on real-world data)
- Adding new sub-signals to existing signals
- Adding new labels to metrics (must be low-cardinality)
- Adding new optional parameters with backward-compatible defaults

### What requires version bump (v2.0)

- Changing detection algorithm for any signal
- Removing a signal
- Changing a metric name
- Changing the score formula structure
- Changing a parameter's default value in a way that would change
  detection results for existing evidence chains

### Signal lifecycle

```
experimental → stable → deprecated → removed
```

- experimental: may change without notice. Not counted in score.
- stable: changes require version bump. Counted in score.
- deprecated: still emitted, replacement exists. Counted in score.
- removed: no longer emitted. Version bump required.

Transition requires at least one minor version with both old and
new signal emitted simultaneously (except experimental → stable).

---

## Conformance

An implementation is conforming if:

1. Given the same evidence chain, it MUST produce the same signal
   events as the reference implementation.
2. It MUST export metrics with the names, types, and labels
   specified in the Metric Registry.
3. It MUST NOT export metrics with forbidden labels.
4. It MUST compute reliability score using the specified formula.

### Conformance Test Harness

Minimum 10 test cases. Each case is an evidence chain (JSONL) with
expected signal events and score.

```
tests/signal_conformance/
  case_01_clean.jsonl           → 0 signals, score 100.0
  case_01_expected.json

  case_02_unreported.jsonl      → 1 protocol_violation (unreported)
  case_02_expected.json

  case_03_drift.jsonl           → 1 artifact_drift
  case_03_expected.json

  case_04_retry.jsonl           → 1 retry_loop (3 identical attempts)
  case_04_expected.json

  case_05_blast.jsonl           → 1 blast_radius (delete 10 resources)
  case_05_expected.json

  case_06_new_scope.jsonl       → 1 new_scope (first prod operation)
  case_06_expected.json

  case_07_duplicate_report.jsonl → 1 protocol_violation (duplicate)
  case_07_expected.json

  case_08_cross_actor.jsonl     → 1 protocol_violation (cross_actor)
  case_08_expected.json

  case_09_mixed.jsonl           → multiple signals, computed score
  case_09_expected.json

  case_10_shape_change.jsonl    → NOT retry (shape_hash differs)
  case_10_expected.json
```

Expected output format:

```json
{
  "spec_version": "1.0",
  "signals": [
    {"signal": "protocol_violation", "sub_signal": "unreported_prescription", "entry_ref": "prs-01HX..."}
  ],
  "score": {
    "value": 96.5,
    "total_operations": 200,
    "signal_counts": {
      "protocol_violation": 3,
      "artifact_drift": 1,
      "retry_loop": 0,
      "blast_radius": 0,
      "new_scope": 2
    }
  },
  "metrics": {
    "evidra_signal_total": [
      {"labels": {"signal": "protocol_violation", "agent": "test-agent", "tool": "kubectl", "scope": "production"}, "value": 3}
    ]
  }
}
```

A conformance runner reads each case, runs the seven detectors,
computes the score, and asserts the output matches expected.json.

Conformance suite is available in the reference implementation
under `tests/signal_conformance/`.
