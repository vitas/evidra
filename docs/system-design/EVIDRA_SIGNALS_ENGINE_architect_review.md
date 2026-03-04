# Evidra Signals Engine — Architect Review

## Status
Review of EVIDRA_SIGNALS_ENGINE_DRAFT.md v0.1.

---

## Review Summary

The signals-first inversion is the right call. Policy catalogs are a
maintenance trap — they age, they miss novel failures, and they force
you to predict every bad configuration in advance. Signals detect
behavioral patterns regardless of what the agent is doing. 10 signals
cover more failure modes than 30 Rego rules.

This review addresses four issues:

1. Signals need baselines — anomaly detection without history is noise
2. Reliability Score needs a real formula
3. Audit log correlation must be scoped carefully
4. Policy layer positioning — minimum viable vs optional

Plus a concrete v1 signal processing architecture.

---

## 1. Signals Need Baselines

### The Problem

Signal #6 (Frequency Anomaly): "operation frequency exceeds historical
baseline." What baseline? On day one of deployment, Evidra has no
history. Every operation is anomalous.

Signal #3 (Behavior Drift): "agent performs a new action scope not
previously observed." On day one, every scope is new.

Signal #5 (Blast Radius Spike): "large number of resources affected."
Large compared to what?

### Proposal: Learning Mode

Evidra operates in two phases:

**Learning phase** (first N days, configurable, default 14 days):
- All signals are collected but not evaluated against thresholds.
- Evidra builds behavioral profiles per agent:
  - action scope inventory (which tools, operations, namespaces)
  - frequency distribution (operations per hour, per day)
  - typical blast radius per tool/operation
  - typical report latency
- Scorecard shows "learning" status instead of a reliability score.
- Prescriptions still work (policy evaluation is independent).
- No anomaly signals fire.

**Active phase** (after learning period):
- Baseline is established. Signals fire against learned profile.
- Baseline is sliding window (trailing 30 days by default).
- New behavior is "drift" only if it hasn't been seen in the
  baseline window.

```yaml
config:
  signals:
    learning_period_days: 14
    baseline_window_days: 30
    # Per-signal threshold overrides
    frequency_anomaly:
      z_score_threshold: 3.0      # standard deviations from mean
    blast_radius_spike:
      percentile_threshold: 99    # above p99 of historical
    report_latency:
      p95_threshold_seconds: 120
```

### Why This Matters

Without baselines, every signal is either:
- hardcoded threshold (brittle, doesn't fit every environment), or
- meaningless ("anomaly detected" — compared to what?)

Learning mode makes signals self-calibrating. An enterprise with
100 terraform applies/day has a different baseline than one with 5.

### Cold Start

During learning phase, only three signals fire (no baseline needed):
- Protocol Violation (binary — report exists or doesn't)
- Artifact Integrity Drift (binary — digests match or don't)
- Retry Loop (pattern detection, not baseline-dependent)

These three provide value from day one. The rest activate after
learning completes.

---

## 2. Reliability Score — Real Formula

### The Problem

Draft formula: `100 - deviation_rate - unreported_rate - retry_loops
- drift_events`. This subtracts percentages from counts. Meaningless.

### Proposal: Weighted Normalized Score

```
reliability_score = 100 × (1 - weighted_penalty)

weighted_penalty = Σ(signal_weight × normalized_signal_value)
```

Signal weights and normalization:

| Signal | Weight | Normalization | Rationale |
|--------|--------|---------------|-----------|
| Protocol Violation | 0.30 | violations / total_operations | Core contract breach |
| Artifact Drift | 0.25 | drifts / total_reports | Agent modified what it promised |
| Behavior Drift | 0.10 | new_scopes / total_operations | May be legitimate exploration |
| Retry Loop | 0.15 | loop_events / total_operations | Agent stuck, wasting resources |
| Blast Radius Spike | 0.10 | spikes / total_operations | May be legitimate batch job |
| Frequency Anomaly | 0.05 | anomalies / total_operations | Often noise |
| Report Latency (p95 > SLO) | 0.05 | (p95 - slo) / slo, clamped 0-1 | Soft signal |

All normalized values are clamped to [0, 1]. Weights sum to 1.0.

Example:

```
Agent: claude-code
Operations: 4217
Protocol Violations: 2    → 2/4217 = 0.0005
Artifact Drifts: 1        → 1/4100 = 0.0002 (of reports)
Behavior Drifts: 0        → 0
Retry Loops: 3             → 3/4217 = 0.0007
Blast Radius Spikes: 0    → 0
Frequency Anomalies: 1    → 1/4217 = 0.0002
Report Latency p95: 4.2s  → (4.2 - 120) / 120 = negative → 0

weighted_penalty = 0.30×0.0005 + 0.25×0.0002 + 0.15×0.0007
                 = 0.00015 + 0.00005 + 0.000105
                 = 0.000305

score = 100 × (1 - 0.000305) = 99.97
```

### Score Bands

| Score | Band | Interpretation |
|-------|------|----------------|
| 99.0 - 100 | Excellent | Production ready |
| 95.0 - 99.0 | Good | Acceptable with monitoring |
| 90.0 - 95.0 | Fair | Needs attention |
| < 90.0 | Poor | Not production ready |

Weights are configurable per deployment. An enterprise that cares
more about protocol compliance can increase the protocol_violation
weight. The defaults above reflect a reasonable starting point.

### Decay

Score is computed over a sliding window (default 30 days). Old
events age out. An agent that had problems 29 days ago but has been
clean since will recover its score naturally.

---

## 3. Audit Log Correlation — Scope It Carefully

### The Problem

Section 8 lists "Kubernetes Audit Logs, CloudTrail" as optional
integrations. The Inspector Model explicitly states: "Evidra does
not verify agent reports." These contradict.

### Proposal: Verification Is Out of Scope. Correlation Is In Scope.

Evidra never verifies "did the agent actually do what it reported?"
That's the admission controller's job, and it's a different trust model.

But Evidra CAN accept external signals as additional evidence.
The distinction:

**Verification (out of scope):**
"Agent reported kubectl apply. Let me check the K8s audit log to
see if it really happened." → This requires K8s API access, live
queries, and makes Evidra an enforcer. NO.

**Correlation (in scope, premium feature):**
"An external system pushed an event to Evidra's ingest API:
'kube-audit: deployment/api-server created in namespace production
at 14:00:03.' Evidra records this as a correlated event alongside
the agent's report." → This is passive reception, not active
verification. Evidra records what it's told by all parties.

The protocol entry becomes richer:

```yaml
protocol_entry:
  prescription: { ... }
  report: { ... }          # from agent
  correlated_events:       # from external sources (optional)
    - source: kube-audit
      timestamp: "..."
      summary: "deployment created"
      digest: "sha256:..."
  verdict: compliant       # still based on prescription vs report only
```

Correlated events don't change the verdict. They enrich the evidence
for human review. An auditor can see: "agent said it did X, Kubernetes
confirms X happened, timestamps align." Or: "agent said it did X, no
correlated event — investigate."

This preserves the Inspector Model (Evidra doesn't verify) while
adding valuable context (Evidra accepts external signals).

### Implementation

Premium feature. External sources push events via API:

```
POST /v1/correlate
{
  "prescription_id": "prs-...",
  "source": "kube-audit",
  "timestamp": "...",
  "summary": "deployment/api-server created",
  "event_digest": "sha256:..."
}
```

Evidra stores it as a correlated event. No live infrastructure
access required. The customer's existing audit pipeline (Falco,
CloudTrail → Lambda → Evidra API) does the push.

---

## 4. Policy Layer — Minimum Viable, Not Optional

### The Problem

The draft says: "Policies remain optional." But if prescribe() has
no policy evaluation, it returns "ok, go ahead" for everything.
Then Evidra is purely a logger. The prescribe/report protocol only
has value if prescriptions carry conditions.

### Proposal: Three Tiers

**Tier 0: Protocol-only (no policy)**
- prescribe() always returns `allow: true`.
- Evidra logs the intent and tracks reports.
- Signals engine detects behavioral patterns.
- Value: flight recorder + anomaly detection. No guardrails.
- Use case: teams that already have Gatekeeper and just want
  agent observability.

**Tier 1: Catastrophic guardrails (default)**
- Small, curated rule set (10-15 rules). Embedded in binary.
- Covers: privileged containers, kube-system deletion, wildcard IAM,
  open security groups, terraform destroy in production.
- prescribe() returns `allow: false` with specific constraints.
- This is the current `baseline` protection level.
- Value: flight recorder + anomaly detection + catastrophic prevention.
- Use case: most teams.

**Tier 2: Extended ops rules (opt-in)**
- Full `ops-v0.1` bundle. Current Evidra behavior.
- Covers: mutable image tags, no resource limits, missing encryption,
  missing versioning, etc.
- Value: everything above + configuration quality guardrails.
- Use case: teams without Gatekeeper or other policy enforcement.

The signals engine works identically across all tiers. Signals are
behavioral — they don't depend on which policy tier is active.
Protocol violations, retry loops, behavior drift — all fire
regardless.

The default should be Tier 1 (catastrophic guardrails). This gives
prescriptions real teeth without requiring policy catalog maintenance.
Users who want more can enable Tier 2. Users who only want signals
can set Tier 0.

```bash
evidra-mcp                            # Tier 1 (default)
evidra-mcp --policy-tier 0            # protocol-only
evidra-mcp --policy-tier 2            # full ops rules
```

---

## 5. Signal Processing Architecture

The draft describes signals but not how they're computed. Here's
a concrete architecture.

### Event Flow

```
prescribe()/report()
       ↓
Protocol Entry (written to evidence chain)
       ↓
Signal Processor (in-process, synchronous)
       ↓
Signal Events (appended to signal log)
       ↓
Metrics Aggregator
       ↓
Prometheus /metrics  +  Scorecard computation
```

### Signal Processor

The signal processor runs after every protocol entry is written.
It's a pipeline of signal detectors, each checking one pattern:

```
protocol_entry
    → ProtocolViolationDetector    → signal or nil
    → ArtifactDriftDetector        → signal or nil
    → RetryLoopDetector            → signal or nil
    → BehaviorDriftDetector        → signal or nil (needs baseline)
    → BlastRadiusDetector          → signal or nil (needs baseline)
    → FrequencyAnomalyDetector     → signal or nil (needs baseline)
    → ReportLatencyDetector        → signal or nil
    → NewScopeDetector             → signal or nil
    → HighRiskActionDetector       → signal or nil
```

Each detector is stateless (reads from evidence chain / baseline)
and produces a signal event or nil.

### In-Memory State

Detectors that need history maintain in-memory state loaded from
the evidence chain on startup:

```go
type SignalState struct {
    // Action scope inventory (for behavior drift)
    KnownScopes     map[string]time.Time     // scope → first_seen

    // Frequency baseline (for frequency anomaly)
    HourlyRates      *RollingWindow           // operations per hour

    // Deny cache (for retry loop detection)
    RecentDenies     *LRUCache                // intent_key → deny_count

    // Blast radius baseline (for blast radius spike)
    ResourceCounts   *PercentileTracker        // resources per operation

    // Report latency baseline
    LatencyTracker   *PercentileTracker        // prescribe→report duration
}
```

State is rebuilt from evidence chain on startup (cold start).
For large evidence chains, only the last `baseline_window_days`
are loaded.

### Signal Event Format

```yaml
signal_event:
  signal_type: "retry_loop"
  severity: "warning"            # info | warning | critical
  agent_id: "claude-code"
  environment: "production"
  details:
    intent_key: "sha256:..."
    deny_count: 4
    window_seconds: 60
  protocol_entry_ref: "evt-..."
  timestamp: "..."
```

Signal events are written to a separate signal log (not the evidence
chain — they're derived data, not primary evidence). They drive
metrics and scorecards.

---

## 6. What the v1 Product Looks Like

Concrete deliverable for first usable version:

```
$ evidra scorecard

AGENT SCORECARD
───────────────────────────────────────
Agent:              claude-code
Period:             2026-02-02 — 2026-03-04 (30 days)
Status:             ACTIVE (learning complete)
Operations:         4,217

RELIABILITY SCORE:  99.97 / 100.00  [EXCELLENT]

SIGNALS
  Protocol Violations:     2    (0.05%)
  Artifact Drifts:         1    (0.02%)
  Retry Loops:             3    (0.07%)
  Behavior Drifts:         0
  Blast Radius Spikes:     0
  Frequency Anomalies:     1
  New Scopes:              0

SLO STATUS
  Deviation rate:     0.02%    ✓  (target: < 0.1%)
  Unreported rate:    0.12%    ✓  (target: < 0.5%)
  P95 report latency: 4.2s    ✓  (target: < 120s)
  High-risk ops/day:  0.4     ✓  (target: < 10)

TOP POLICY INTERACTIONS
  k8s.protected_namespace         7 denials
  terraform.sg_open_world         3 denials

RECENT SIGNALS
  2026-03-03 14:22  retry_loop      terraform.apply (denied 3x)
  2026-02-28 09:11  artifact_drift  kubectl.apply (digest mismatch)
  2026-02-15 11:45  protocol_violation  unreported prescription
───────────────────────────────────────
```

That's the product. Everything else — metrics endpoint, dashboards,
telemetry platform — builds on this.

---

## 7. Revised Architecture Summary

```
Evidra = Inspector Protocol + Signals Engine + Evidence Chain

Inspector Protocol:
  prescribe() → policy evaluation → signed prescription
  report()    → protocol entry → deviation check

Signals Engine:
  Protocol entries → signal detectors → signal events
  Signal events → metrics + scorecard

Evidence Chain:
  Append-only, hash-linked, Ed25519 signed
  Primary evidence: prescriptions + reports + protocol entries
  Derived: signal events (separate log)

Policy Layer:
  Tier 0: none (pure signals)
  Tier 1: catastrophic guardrails (default, embedded, 10-15 rules)
  Tier 2: extended ops rules (opt-in, full OPA bundle)
```

Flight recorder for AI infrastructure agents.
Signal engine detects behavioral anomalies.
Scorecard tells you if the agent is production-ready.
