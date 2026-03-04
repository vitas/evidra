# Evidra Telemetry Plane — Architect Review

## Status
Review of EVIDRA_TELEMETRY_PLANE.md draft RFC.

Review scope: architectural fitness, data model gaps, operational
realism, market positioning.

---

## Review Summary

The telemetry plane is the right commercial layer. It shifts the
buyer from agent developers (small market, long sales) to enterprises
deploying agents (large market, urgent need). The "Prometheus for
AI agent operations" positioning is clear and defensible.

This review addresses five gaps in the current draft:

1. Label provenance — where do agent metadata labels come from?
2. Intent fingerprinting — current hash approach doesn't capture drift
3. Data volume and cardinality — metrics will explode at scale
4. The "aha moment" — what single view sells the product?
5. Relationship between inspector and telemetry — what's free, what's paid?

---

## 1. Label Provenance

### The problem

The draft references labels like `agent_build_id`, `prompt_version`,
`model_id` in regression metrics. These are powerful — but who
provides them?

Evidra currently receives `actor: { type, id, origin }` from the
agent. That's three strings. Regression analysis needs much richer
metadata.

### Proposal: Actor Metadata Extension

The `prescribe` tool should accept an optional `actor_meta` field:

```yaml
prescribe:
  tool: kubectl
  operation: apply
  raw_artifact: "..."
  actor:
    type: agent
    id: claude-code
    origin: mcp
  actor_meta:                    # NEW — optional, free-form labels
    agent_version: "3.5.2"
    model_id: "claude-sonnet-4-5-20250929"
    prompt_version: "infra-v12"
    session_id: "ses-abc123"
    pipeline_id: "gh-actions-run-456"
```

Rules:

- `actor_meta` is optional. If absent, telemetry works with base
  labels only (`agent_id`, `tool`, `env`). Regression analysis
  is unavailable, but reliability metrics work fine.
- All values are strings. Evidra does not interpret them.
- Keys are restricted to `[a-z0-9_]`, max 32 chars. Values max 128
  chars. Max 10 keys. This prevents label cardinality explosion from
  agent-side.
- `actor_meta` is stored in the protocol entry and propagated to
  telemetry labels. It is NOT part of the prescription signature
  (it's metadata, not policy-relevant input).

### Why this matters

Without `actor_meta`, the regression dashboard is impossible.
With it, you can answer: "after we upgraded from claude-sonnet-4-5-20250929 to
claude-opus-4-5-20250918, deviation rate went from 0.01% to 0.3% — rollback."

This is the query that makes the telemetry plane worth paying for.

---

## 2. Intent Fingerprinting — Revised Model

### The problem

The draft defines: `intent_fingerprint = hash(canonical_actions)`.

This is too granular. Two deployments of the same app with different
image tags produce different hashes. That's not drift — that's
normal CI/CD. Drift is when the agent starts doing operations it
didn't do before, or targets namespaces it didn't target before.

### Proposal: Hierarchical Intent Classification

Instead of hashing the full canonical action, define intent at
three levels:

```
Level 1: Action Class (coarsest)
  = tool + operation
  Example: "kubectl.apply", "terraform.apply"

Level 2: Action Scope
  = tool + operation + resource_type + namespace/environment
  Example: "kubectl.apply/deployment/production"

Level 3: Action Identity (finest)
  = hash(full canonical action)
  Example: "sha256:abc123..."
```

Telemetry uses Level 2 for drift detection. Level 3 is for
deduplication (deny-loop cache, already exists in v2).

```
# Drift detection: new action scopes appearing
evidra_action_scope_first_seen{agent, scope, env}

# Behavioral profile: distribution of action scopes
evidra_action_scope_distribution{agent, scope, env}

# Drift alert: agent doing something it's never done before
# (new scope not in baseline window)
```

### Drift detection algorithm

1. Build baseline: action scope distribution over trailing 30 days.
2. On each new operation: check if scope exists in baseline.
3. New scope → `drift_event` with severity based on risk_level
   of the new scope.
4. Metric: `evidra_intent_drift_events_total{agent, env, severity}`

This catches: "agent started deleting resources in production for
the first time" without false-alerting on "agent deployed a new
image tag."

---

## 3. Data Volume and Cardinality

### The problem

Label combinations: `{agent_id, tool, operation, env, rule_id,
verdict, risk_level, agent_version, model_id}`. With 10 agents,
5 tools, 4 envs, 30 rules, 4 verdicts, 3 risk levels, 5 agent
versions — that's potentially 900,000 series. Prometheus dies.

### Proposal: Tiered Metrics

**Tier 1: Real-time counters (low cardinality, always on)**

```
evidra_prescriptions_total{agent, tool, decision}
evidra_reports_total{agent, tool, exit_code_class}
evidra_verdicts_total{agent, verdict}
evidra_risk_actions_total{risk_level}
```

Labels: agent (10), tool (5), decision (2), verdict (4),
exit_code_class (3: success/failure/timeout), risk_level (3).
Max series: ~600. Fine for Prometheus.

exit_code_class instead of raw exit_code: 0 → success,
1-125 → failure, 126+ → timeout/signal. Prevents cardinality
explosion from diverse exit codes.

**Tier 2: Dimensional analytics (higher cardinality, sampled)**

```
evidra_policy_triggers_total{rule_id, env}
evidra_deny_reasons_total{rule_id, agent}
evidra_action_scope_total{agent, scope}
```

Labels include rule_id (30) and scope (variable). These are
aggregated hourly, not kept as raw Prometheus counters. Stored
in the telemetry backend (commercial), not in the OSS
Prometheus endpoint.

**Tier 3: Regression analysis (event-level, stored)**

Full protocol entries with `actor_meta` labels. Not metrics —
events stored in the evidence chain, queryable via evidence API.
Aggregated into regression dashboards on read, not write.

### Retention

| Tier | Retention | Storage |
|------|-----------|---------|
| Tier 1 (counters) | 15 days raw, 1 year aggregated | Prometheus |
| Tier 2 (dimensional) | 90 days | Telemetry backend |
| Tier 3 (events) | 1 year raw | Evidence chain |

---

## 4. The "Aha Moment" — Agent Scorecard

### The problem

The draft lists four dashboards. None of them is the one view that
sells the product.

### Proposal: Agent Scorecard

One page per agent. This is what the VP of Engineering shows the
CISO when asking "can we use this agent in production?"

```
┌─────────────────────────────────────────────────┐
│  AGENT SCORECARD: claude-code                   │
│  Period: 2026-02-01 — 2026-03-04                │
│  Operations: 4,217                              │
├─────────────────────────────────────────────────┤
│                                                 │
│  COMPLIANCE                      SLO STATUS     │
│  ─────────                       ──────────     │
│  Deviation rate:    0.02%        ✅ < 0.1%      │
│  Unreported rate:   0.12%        ✅ < 0.5%      │
│  P95 time-to-report: 4.2s       ✅ < 120s      │
│                                                 │
│  RISK PROFILE                                   │
│  ────────────                                   │
│  High-risk ops:     12           ⚠️  < 10/mo    │
│  Destructive ops:   34                          │
│  Read-only ops:     3,891                       │
│                                                 │
│  TOP POLICY INTERACTIONS                        │
│  ───────────────────────                        │
│  k8s.protected_namespace     7 denials          │
│  terraform.sg_open_world     3 denials          │
│  k8s.privileged_container    2 denials          │
│                                                 │
│  DRIFT                                          │
│  ─────                                          │
│  New action scopes this period: 0               │
│  Action scope consistency: 99.8%                │
│                                                 │
│  TREND (30d)                                    │
│  ──────────                                     │
│  Deviation rate:  ▁▁▁▁▂▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁  │
│  Denials:         ▃▂▂▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁  │
│  Operations/day:  ▅▅▆▆▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇█▇▇  │
│                                                 │
│  VERDICT: PRODUCTION READY                      │
│  All SLOs met. No drift detected.               │
│                                                 │
└─────────────────────────────────────────────────┘
```

This scorecard is the product. Everything else — metrics,
dashboards, alerts — exists to produce this one artifact.

An enterprise customer evaluating Claude Code asks: "Show me
the scorecard." If all SLOs green → approved. If any red →
specific conversation about what failed and whether it's
acceptable.

### Scorecard as PDF/API

```bash
# Generate scorecard for agent
evidra scorecard --agent claude-code --period 30d --format pdf

# API endpoint (commercial)
GET /v1/scorecard?agent=claude-code&period=30d
```

The PDF scorecard is the artifact that goes into the compliance
folder. Signed with Ed25519. Tamper-evident. Auditor-friendly.

---

## 5. OSS vs Commercial Boundary

### The problem

The draft says: "OSS: inspector + basic metrics. Commercial:
dashboards + regression + compliance." This is too vague.
The boundary determines adoption (too little free → no users)
and revenue (too much free → no buyers).

### Proposal: Clear Boundary

**OSS (evidra binary, self-hosted)**

- Inspector model (prescribe/report/protocol)
- Evidence chain (JSONL, hash-linked, Ed25519 signed)
- Prometheus /metrics endpoint (Tier 1 counters only)
- CLI commands: `evidra evidence violations`, `evidra evidence
  unreported`, `evidra evidence summary`
- Basic scorecard via CLI: `evidra scorecard --agent X --period 30d`
  (text output, no PDF, no signature)

This is enough for a team to evaluate Evidra, integrate with
their agent, and see basic reliability metrics. It's also enough
to prove value during a pilot.

**Commercial (hosted platform)**

- Hosted telemetry ingestion (protocol entries → telemetry backend)
- Tier 2 dimensional analytics (rule_id breakdown, scope analysis)
- Tier 3 regression analysis (agent_version / model_id comparison)
- Agent Scorecard (web UI + signed PDF export + API)
- Drift detection with alerting
- SLO configuration and monitoring
- Multi-agent comparison view
- Webhook/SIEM integrations (Datadog, Splunk, PagerDuty)
- Compliance report generation (SOC2, PCI DSS, CIS mapping)
- SSO/OIDC for team access

### Why this boundary works

The OSS version answers: "Is my agent behaving?" (yes/no).
The commercial version answers: "How is my agent behaving
compared to last month, compared to other agents, compared
to our SLOs, and can I show this to an auditor?" (analytics).

Teams start with OSS. When they have 3+ agents in production,
they need the commercial layer because managing scorecards
and regression analysis across agents via CLI doesn't scale.

---

## 6. Missing Piece: How Does Telemetry Reach the Platform?

The draft shows the telemetry plane as a local component. But the
commercial value requires centralized telemetry. How does protocol
data get from the local Evidra instance to the hosted platform?

### Proposal: Telemetry Forwarder

Two modes:

**Push mode (recommended for production)**

Evidra MCP/CLI is configured with `EVIDRA_TELEMETRY_URL` and
`EVIDRA_TELEMETRY_KEY`. After every protocol entry is written
locally, it is also forwarded to the hosted platform via HTTPS POST.

```
EVIDRA_TELEMETRY_URL=https://telemetry.evidra.com/v1/ingest
EVIDRA_TELEMETRY_KEY=etk-...
```

What is sent: the protocol entry (prescription + report + verdict +
actor_meta). Raw artifacts are NOT sent — only digests. This is
important: the telemetry platform never sees the customer's
infrastructure manifests. Privacy by design.

**Pull mode (for air-gapped environments)**

Evidence chain is exported and uploaded manually:

```bash
evidra evidence export --format jsonl --since 30d > export.jsonl
# Upload to platform via web UI or API
```

### Data privacy

The telemetry platform receives:

- Tool, operation, environment (categorical)
- Risk level, verdict (categorical)
- Rule IDs triggered (categorical)
- Actor metadata (agent_id, version, model_id)
- Artifact digests (hashes only, no content)
- Timestamps

The platform does NOT receive:

- Raw manifests, plans, or any infrastructure configuration
- Credentials or secrets
- Cluster names, IP addresses, or endpoint URLs (unless in actor_meta,
  which the customer controls)

This is critical for enterprise adoption. The telemetry platform
knows "agent X applied a kubectl.apply in production and it was
compliant." It does NOT know what was applied.

---

## 7. Competitive Positioning

The draft compares to Prometheus. More precise positioning:

| Layer | Tools | Evidra's relationship |
|-------|-------|----------------------|
| Infrastructure monitoring | Prometheus, Datadog, CloudWatch | Orthogonal. They monitor servers. Evidra monitors agents. |
| Security enforcement | Gatekeeper, Kyverno, Sentinel | Complementary. They enforce. Evidra prescribes and observes. |
| CI/CD observability | Argo Workflows, Tekton metrics | Adjacent. They monitor pipeline health. Evidra monitors agent decision quality. |
| AI observability | LangSmith, Langfuse, Braintrust | Closest competitors. They monitor LLM calls (token usage, latency, hallucination). Evidra monitors infrastructure actions (policy compliance, deviation, drift). Different layer. |

The most important row is the last one. LangSmith/Langfuse monitor
the AI's thinking (prompts, completions, token usage). Evidra
monitors the AI's doing (infrastructure actions, policy compliance,
operational reliability). These are complementary layers:

```
LangSmith: "The agent made 47 LLM calls, spent 12K tokens,
            and produced this plan."

Evidra:    "The agent prescribed 3 kubectl applies, 1 was denied
            (privileged container), 2 were compliant, deviation
            rate 0%, SLO met."
```

An enterprise running AI agents in production needs both. LangSmith
for model-level debugging. Evidra for ops-level accountability.

---

## 8. Amended Implementation Plan

Replaces Section 10 of the draft and aligns with inspector model
implementation plan.

### v0.3.0 — Foundation (Engine v3)
- Domain adapters + raw artifact input
- Current validate tool unchanged

### v0.4.0 — Inspector Model
- `prescribe` / `report` MCP tools
- Protocol entries in evidence chain
- Deviation detection (artifact_digest comparison)
- `actor_meta` field (optional, free-form labels)
- CLI: `evidra evidence violations`, `evidra evidence summary`

### v0.4.x — OSS Telemetry
- Prometheus /metrics endpoint (Tier 1 counters)
- `evidra scorecard --agent X --period 30d` (CLI, text output)
- Hierarchical intent classification (Level 1/2/3)
- Drift detection events in evidence chain

### v0.5.0 — Commercial Telemetry Platform (hosted)
- Telemetry forwarder (push mode)
- Tier 2 dimensional analytics
- Agent Scorecard (web UI + signed PDF)
- SLO configuration and monitoring
- Drift alerting

### v0.5.x — Enterprise
- Tier 3 regression analysis (agent_version, model_id comparison)
- Multi-agent comparison view
- SIEM/webhook integrations
- Compliance report generation
- Pull mode for air-gapped environments
- SSO/OIDC

### v0.6.0 — Ecosystem
- LangSmith/Langfuse integration (link LLM trace → Evidra protocol)
- OpenTelemetry exporter (OTLP)
- Grafana plugin
- Public agent scorecard (opt-in, for agent marketplaces)

---

## 9. Summary of Review

The telemetry plane is the right commercial layer for Evidra. The
key additions from this review:

1. **actor_meta** — without it, regression analysis is impossible.
   Structured optional labels with cardinality limits.

2. **Hierarchical intent** — three levels instead of flat hash.
   Level 2 (scope) for drift detection. Level 3 (identity) for
   dedup.

3. **Tiered metrics** — Tier 1 (Prometheus, low cardinality),
   Tier 2 (backend, dimensional), Tier 3 (events, stored).
   Prevents cardinality explosion.

4. **Agent Scorecard** — the product's "aha moment." One page per
   agent. Signed PDF for compliance. This is what sells.

5. **OSS/commercial boundary** — inspector + Tier 1 metrics free.
   Analytics + scorecard UI + regression + integrations paid.

6. **Telemetry forwarder** — how data gets from local to hosted.
   Privacy by design: digests only, no raw artifacts.

7. **Competitive positioning** — LangSmith monitors AI thinking.
   Evidra monitors AI doing. Complementary, not competing.
