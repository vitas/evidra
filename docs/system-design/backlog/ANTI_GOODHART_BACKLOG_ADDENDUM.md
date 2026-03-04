
# Addendum: Anti-Goodhart Measures (Why + Backlog)
Status: Draft

## Why we need this

Evidra introduces an **Agent Reliability Score**. The moment this score becomes a target
(for promotion, rollout, gating, or vendor comparison), agents and their builders will
optimize for the score rather than for operational outcomes.

This is a direct application of **Goodhart’s Law**:
> When a measure becomes a target, it ceases to be a good measure.

### What can go wrong if we do nothing
Even with perfect canonicalization and stable signals, a single score can be “gamed”:

- **Safe-but-useless behavior**: agent avoids doing hard work, escalates to humans, or stalls.
- **Micro-splitting**: agent breaks changes into tiny steps to avoid blast-radius triggers.
- **Protocol theater**: agent reports “success” and reuses digests without real corroboration.
- **Metric drift**: score improves while real ops pain worsens (longer MTTR, lower throughput).

If ops teams see “99.9 score but nothing gets done / incidents still happen”, trust is lost
and the tool becomes shelfware.

## Design principle
Keep the system simple, but make gaming hard by measuring **two axes**:

1) **Safety** — “does it behave safely?” (signals)
2) **Effectiveness** — “does it actually complete tasks?” (outcomes)

And optionally attach **independent evidence pointers** (not full verification).

---

## Minimal patch to EVIDRA_AGENT_RELIABILITY_BENCHMARK.md

### Add a section near scoring (recommended location: after score definition)

#### Anti-Goodhart: Safety vs Effectiveness

Evidra splits scoring into two independent components:

- **Safety Score**: derived from the five core signals (protocol violations, artifact drift,
  retry loops, blast radius, new scope). This measures operational risk and instability.
- **Effectiveness Score**: derived from outcome metrics (completion, latency, escalation).
  This prevents “safe-but-useless” agents from ranking high.

The overall evaluation MUST present both scores side-by-side. Do not collapse them into a
single number for gating decisions.

Optional: `report.external_refs[]` allows attaching pointers to independent evidence
(CI job IDs, Kubernetes audit event IDs, CloudTrail event IDs) to reduce self-report gaming.
These references enrich evidence but do not change the protocol verdict.

---

## Backlog (phased delivery)

### P0 (v1) — Ship without overbuilding
Goal: protect against the most obvious gaming with minimal scope.

- **P0.1 Add Effectiveness Score (simple)**
  - Inputs:
    - completion_rate (success reports / prescriptions)
    - p95_time_to_report (or time_to_outcome if you track retries)
    - escalation_rate (agent asked for human / required manual step)
  - Output:
    - effectiveness_score in [0..100]
  - Rationale: prevents “do nothing / always escalate” from looking good.

- **P0.2 Present Safety + Effectiveness in scorecard**
  - Scorecard outputs:
    - safety_score
    - effectiveness_score
    - top 3 penalties (most impactful signals)
  - Rationale: ops can see tradeoffs instantly.

- **P0.3 Add `report.external_refs[]` field (optional, no verification)**
  - Examples:
    - github_actions_run_id
    - gitlab_job_id
    - k8s_audit_event_id
    - cloudtrail_event_id
  - Rationale: raises cost of “protocol theater” without building integrations.

### P1 — Make it robust for real ops use
Goal: improve outcome modeling without turning into a platform.

- **P1.1 Task boundary / session model (lightweight)**
  - Add `task_id` / `session_id` to prescriptions and reports.
  - Allows measuring “time to resolution” across retries.
  - Rationale: effectiveness becomes meaningful per task, not per command.

- **P1.2 Stalled-task detector**
  - If prescription exists and no terminal outcome within threshold → stalled.
  - Rationale: catches “safe but stuck” patterns.

- **P1.3 Micro-splitting heuristic**
  - Detect suspicious patterns of many tiny operations vs baseline per task.
  - Rationale: reduces blast-radius gaming by atomizing changes.

### P2 — Optional corroboration (still not enforcement)
Goal: add trust without requiring wide permissions.

- **P2.1 Pluggable corroboration adapters (read-only)**
  - K8s audit log correlator
  - CloudTrail correlator
  - CI correlator
  - Output: corroboration_coverage metric + mismatches
  - Rationale: improves confidence in reports; still “enrich evidence, not verdict”.

---

## Notes on simplicity
- Do NOT add ML/baselines for v1.
- Keep labels low-cardinality; keep detailed breakdowns in scorecard JSON, not Prometheus.
- Keep canonicalization versioning strict; scoring should be version-aware.
