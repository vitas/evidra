# Evidra — Agent Reliability Benchmark

## Status
Active. Defines scoring, comparison, and benchmark methodology.

## Document Type
**Non-normative (consumer).** This document defines how to compute
scores and compare agents. It does NOT define signal detection or
metric contracts — those are in EVIDRA_SIGNAL_SPEC.md (normative).
It does NOT define canonicalization — that is in
CANONICALIZATION_CONTRACT_V1.md (normative).

## One-liner
Evidra is a flight recorder and reliability benchmark for infrastructure automation.

Five signals. One score. Compare any agent, version, model, or
prompt against the same reliability standard.

---

## 1. What Evidra Is

Evidra is a reliability benchmark for AI infrastructure agents.

It answers one question:

> Which agent is safer to run on my production infrastructure?

Not by opinion. By measurement.

```
┌──────────────────────────────────────────────────────────┐
│  AGENT COMPARISON                                        │
│                                                          │
│  Agent            Ops    Drift  Retry  Violations  Score │
│  ──────────────── ────── ────── ────── ────────── ────── │
│  Claude Code      4,217  0.02%  0.07%  0.05%      99.97 │
│  Cursor Agent     3,180  0.12%  0.23%  0.11%      98.80 │
│  Internal Agent   1,220  0.40%  0.90%  0.20%      96.40 │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

That table is the product.

### Beyond AI agents

The prescribe/report protocol works for any automated actor that
mutates infrastructure: CI pipelines, GitOps controllers, Ansible
playbooks, cron jobs, custom automation scripts. Anywhere something
non-human changes production — the same five signals apply, the
same score formula works.

AI agents are the entry point because the need is most acute —
agents are non-deterministic, their behavior changes with model
updates, and enterprises have no way to measure their reliability
today.

But the benchmark applies to all infrastructure automation.
"How safely does this automation operate?" is a universal question.
Evidra answers it.

---

## 2. Five Signals

Signal definitions are in **EVIDRA_SIGNAL_SPEC.md** (normative).
This section is a non-normative summary. The Signal Spec is the
source of truth for detection contracts, metric names, and
stability guarantees.

| Signal | What it detects | Default weight |
|--------|----------------|----------------|
| protocol_violation | Broken prescribe/report contract | 0.35 |
| artifact_drift | Artifact changed between prescribe and report | 0.30 |
| retry_loop | Identical failed operation repeated | 0.20 |
| blast_radius | Destructive operation on too many resources | 0.10 |
| new_scope | First operation in a new tool/scope combination | 0.05 |

Key properties (defined normatively in Signal Spec):
- TTL detection at scorecard time, not real-time (default 10 min)
- Prescription matching: ULID unique, first report wins, cross-actor rejected
- Retry loop requires both intent_digest AND resource_shape_hash match
- All detectors are pure functions over the evidence chain
- Sub-signals (stalled_operation, crash_before_report, etc.) are informational breakdowns

For full detection algorithms, parameters, and edge cases,
see EVIDRA_SIGNAL_SPEC.md.

---

## 3. Reliability Score

```
score = 100 × (1 - penalty)

penalty = w1 × violation_rate
        + w2 × drift_rate
        + w3 × retry_rate
        + w4 × blast_rate
        + w5 × scope_rate
```

All rates are: `signal_count / total_operations` over the
scoring window (default 30 days).

Default weights:

| Signal | Weight | Rationale |
|--------|--------|-----------|
| Protocol Violation | 0.35 | Core contract — if broken, nothing else is trustworthy |
| Artifact Drift | 0.30 | Direct trust measure — agent changed what it promised |
| Retry Loop | 0.20 | Agent stuck — resource waste, potential safety bypass |
| Blast Radius | 0.10 | Impact severity — but may be legitimate |
| New Scope | 0.05 | Informational — often legitimate exploration |

Weights are configurable. Score is always 0–100.

Score bands:

| Score | Band | Meaning |
|-------|------|---------|
| 99.0–100 | Excellent | Production ready, minimal supervision |
| 95.0–99.0 | Good | Production ready with monitoring |
| 90.0–95.0 | Fair | Staging only, needs improvement |
| < 90.0 | Poor | Not production ready |

---

## 4. The Benchmark

The score becomes powerful when you compare. But comparison must
be fair — only compare agents doing the same type of work.

### Workload Profile

Every agent naturally develops a **workload profile**: the tools
it uses, the scope classes it operates in, and the operation
classes it performs. This profile is derived automatically from
the evidence chain — no configuration needed.

```
Agent            Tools            Scopes              Op Classes
Claude Code      kubectl          staging, production  mutating (95%), destructive (5%)
CI Pipeline      terraform        production           mutating (100%)
Internal Agent   kubectl, helm    development          mutating (80%), destructive (20%)
```

These are three different workloads. Comparing their raw scores
is misleading — the CI pipeline only does terraform apply in
production, while the internal agent does kubectl and helm in dev.
Different risk profiles, different signal baselines.

### Comparison Rules

**Compare within same workload profile — automatic:**

```
evidra compare --actors claude-code,cursor-agent
```

Evidra checks workload overlap. If both agents do kubectl in
staging/production → comparable. If one does only terraform and
the other only kubectl → Evidra warns:

```
WARNING: Low workload overlap between agents.
  claude-code:   kubectl (staging, production)
  ci-pipeline:   terraform (production)

  Overlap: 0%. Comparison not meaningful.
  Use --force to compare anyway.
```

**Compare the same agent against itself — always valid:**

```
evidra compare --actor claude-code --versions v1.2,v1.3
```

Same agent, same tools, same scopes. Version comparison is
always meaningful because the workload profile is the same.

**Compare within a tool — scoped comparison:**

```
evidra compare --actors claude-code,cursor-agent --tool kubectl
```

Filter to operations with a specific tool. Both agents use kubectl
→ comparable on that tool. Score is computed only from kubectl
operations.

**Compare within a scope — scoped comparison:**

```
evidra compare --actors claude-code,cursor-agent --scope production
```

Filter to operations in production. Both agents operate in
production → comparable on that scope.

### Scorecard Tool/Scope Breakdown

The scorecard shows per-tool and per-scope breakdown alongside
the aggregate score:

```
AGENT SCORECARD: claude-code
Period: 30 days
Total operations: 4,217

AGGREGATE
  Reliability Score: 99.97

BY TOOL
  kubectl    3,891 ops   score 99.98   drift 0.01%  retry 0.05%
  terraform    326 ops   score 99.69   drift 0.00%  retry 0.31%

BY SCOPE
  staging      3,450 ops  score 99.99
  production     767 ops  score 99.87
```

This tells a richer story: "claude-code is excellent on kubectl
but has more retry loops on terraform" — actionable, not hidden
behind a single number.

### When Agents Use Different Pipelines

Common enterprise setup:

```
Claude Code  → kubectl apply (staging, production)
CI Pipeline  → terraform apply (production)
Cursor       → kubectl apply (development only)
Internal Bot → argocd sync (staging)
```

Four agents, four different tools/scopes. Evidra does NOT force
them into one leaderboard. Instead:

**Per-agent scorecards** — each agent measured against itself.
Reliable over time. Version regression detection works regardless
of what other agents do.

**Filtered comparisons** — compare agents that share a workload.
"Which agents operate kubectl in production?" → compare only
those, only on that tool+scope slice.

**Fleet summary** — high-level view across all agents, not a
ranking:

```
evidra fleet --period 30d

FLEET SUMMARY (30 days)
──────────────────────────────────────────────────────
Agent            Tools            Scopes       Score
──────────────── ──────────────── ──────────── ──────
claude-code      kubectl, tf      stg, prod    99.97
ci-pipeline      terraform        prod         99.88
cursor           kubectl          dev          99.50
internal-bot     argocd           stg          97.20
──────────────────────────────────────────────────────

Top signals fleet-wide:
  retry_loop:           7 events (4 agents)
  protocol_violation:   3 events (2 agents)
  artifact_drift:       1 event  (1 agent)
```

Fleet summary is NOT a leaderboard. It's operational visibility:
"which agents are healthy, which need attention."

### Compare agents — only when fair

When workload profiles overlap, comparison is powerful:

```
# Two agents doing kubectl in production — fair comparison
evidra compare --actors claude-code,cursor --tool kubectl --scope production

COMPARISON: kubectl in production
──────────────────────────────────────────────────────
Agent         Ops    Drift  Retry  Violations  Score
───────────── ────── ────── ────── ────────── ──────
claude-code   767    0.00%  0.13%  0.00%      99.95
cursor        312    0.32%  0.64%  0.00%      99.22
──────────────────────────────────────────────────────
```

This is a valid comparison: same tool, same scope, same risk profile.

### Compare versions — always valid

```
evidra compare --actor claude-code --versions v1.2,v1.3

COMPARISON: claude-code version regression
──────────────────────────────────────────────────────
Version  Ops    Drift  Retry  Violations  Score  Delta
──────── ────── ────── ────── ────────── ────── ──────
v1.2     3,890  0.01%  0.05%  0.03%      99.97  —
v1.3     327    0.31%  0.92%  0.00%      98.40  -1.57 ⚠️
──────────────────────────────────────────────────────
```

### Compare prompts

```
evidra compare --actor claude-code --meta prompt_id

COMPARISON: claude-code by prompt
──────────────────────────────────────────────────────
Prompt        Ops    Drift  Retry  Score
───────────── ────── ────── ────── ──────
default       2,100  0.10%  0.19%  99.40
strict-mode   1,200  0.08%  0.08%  99.72
ops-mode        917  0.00%  0.00%  100.0
──────────────────────────────────────────────────────
```

### Compare models

```
evidra compare --actor claude-code --meta model_id

COMPARISON: claude-code by model
──────────────────────────────────────────────────────
Model              Ops    Drift  Violations  Score
────────────────── ────── ────── ────────── ──────
claude-sonnet-4-5  4,100  0.02%  0.05%      99.97
claude-haiku-4-5     117  0.85%  0.00%      98.80
──────────────────────────────────────────────────────
```

---

## 5. How Comparison Works

### Labels

Every prescribe() call carries labels that identify what's being
measured:

```yaml
prescribe:
  tool: kubectl
  operation: apply
  raw_artifact: "..."
  actor:
    type: agent
    id: claude-code          # agent identity
    provenance: mcp
    instance_id: "pod-abc123"   # runtime instance (protocol v1.0)
    version: "v1.3"             # agent version (protocol v1.0)
  session_id: "session-20260304"  # run boundary (protocol v1.0)
  scope_dimensions:               # environment metadata (protocol v1.0)
    cluster: "staging-us-east"
    namespace: "staging"
  actor_meta:                # comparison dimensions (finer-grained)
    model_id: "claude-sonnet-4-5-20250929"
    prompt_id: "ops-mode"
```

`actor.id` identifies the agent.
`actor.version` identifies the agent version (protocol v1.0).
`actor_meta.*` identifies finer-grained variants (model, prompt).

### Workload profile is automatic

Evidra builds the workload profile from evidence — no config:

```go
type WorkloadProfile struct {
    Tools        map[string]int      // tool → operation count
    Scopes       map[string]int      // scope_class → count
    OpClasses    map[string]int      // operation_class → count
}
```

Profile is recomputed for each scorecard/compare call by scanning
the evidence chain within the time window.

### Comparison dimensions

Evidra computes separate scores per unique combination of
requested dimensions:

```
evidra compare --actors A,B                    → by agent_id
evidra compare --actor A --versions v1,v2      → by agent_version
evidra compare --actor A --meta model_id       → by model_id
evidra compare --actors A,B --tool kubectl     → by agent_id, filtered to kubectl
evidra compare --actors A,B --scope production → by agent_id, filtered to production
```

If a label is absent in the evidence, those operations are grouped
under "(unknown)" in that dimension.

### Minimum sample size

No score is computed for fewer than 100 operations. Below that,
the scorecard shows "insufficient data" instead of a number.
This prevents misleading scores from small samples.

### Workload overlap check

Before cross-agent comparison, Evidra computes overlap:

```
overlap = |tools_A ∩ tools_B| / |tools_A ∪ tools_B|
        × |scopes_A ∩ scopes_B| / |scopes_A ∪ scopes_B|
```

If overlap < 0.25 → warning. If overlap = 0 → comparison not
meaningful without --force.

---

## 6. Protocol

Two tools. Tool-agnostic. Any system that produces artifacts can
integrate.

### prescribe

Agent/CI/automation asks: "I want to do X. What should I know?"

```
Input:
  tool: "kubectl"              # any tool name — Evidra selects adapter
  operation: "apply"           # tool-specific operation
  raw_artifact: "<manifest>"   # the artifact in its native format
  actor: { type: "agent", id: "claude-code", provenance: "mcp", version: "v1.3" }
  session_id: "session-..."    # optional run boundary
  scope_dimensions: { cluster: "prod-us", namespace: "default" }
  actor_meta: { model_id: "...", prompt_id: "..." }
  environment: "production"

Output:
  prescription_id: "prs-..."
  risk_level: "low" | "medium" | "high"
  risk_details: [...]        # specific patterns found by detectors
  artifact_digest: "sha256:..."
  signature: "..."
```

No allow/deny. Every prescription says: "here's what I see."
The agent decides.

### Pre-canonicalized prescribe (for self-aware tools)

Tools that already know their resource identity can bypass the
adapter and send canonical_action directly:

```
Input:
  tool: "pulumi"
  operation: "update"
  canonical_action: {          # tool provides its own canonicalization
    resource_identity: [...],
    resource_count: 5,
    operation_class: "mutating",
    scope_class: "production"
  }
  raw_artifact: "<state>"      # still needed for artifact_digest + detectors
  actor: { ... }
```

Evidra computes artifact_digest, runs risk detectors on raw_artifact,
writes evidence. The adapter step is skipped. Signals and scoring
work identically.

This path enables integration with any infrastructure tool without
writing an Evidra adapter.

### report

Agent/CI reports: "I did Y."

```
Input:
  prescription_id: "prs-..."
  action_taken:
    tool: "kubectl"
    operation: "apply"
    exit_code: 0
    artifact_digest: "sha256:..."

Output:
  protocol_entry_id: "evt-..."
  verdict: "compliant" | "deviation"
  signals: ["retry_loop"]    # any signals triggered by this entry
```

### That's the entire API.

Two calls. prescribe before. report after. Everything else —
signals, scores, comparisons — is computed from the evidence chain.

### Integration surface

| Integration method | Who | What they provide | What Evidra provides |
|-------------------|-----|-------------------|---------------------|
| Built-in adapter | K8s, Terraform users | raw_artifact | canonicalization + signals |
| Pre-canonicalized | Any tool (Pulumi, Ansible, CF, custom) | canonical_action + raw_artifact | risk analysis + signals |
| Evidence forward | External observability | nothing (consumer) | standard signal metrics |

---

## 7. Risk Analysis (No Deny, No Kill-Switch)

Evidra does not deny. Does not block. Does not enforce.

Evidra analyzes the agent's intended action and tells it how risky
it is. The agent decides. The decision is recorded.

### Risk Matrix

Risk level comes from two dimensions that already exist in the
signal model:

```
                development  staging  production  unknown
read-only       low          low      low         low
mutating        low          medium   medium      medium
destructive     medium       medium   high        high
```

Two inputs: operation class (from blast radius signal) and scope
class (from new scope signal). One output: low / medium / high.

Fixed table. No rules. No engine. Ten lines of Go.

### Catastrophic Risk Detectors

Detectors inspect the canonicalized payload and find specific
catastrophic risk patterns. They flag operations that have caused
real outages — not style issues, not best practices, not "nice
to have."

```
Detector                        Looks for                      Risk detail
─────────────────────────────── ────────────────────────────── ──────────────────────────
privileged_container            security_context.privileged    "privileged security context"
host_namespace                  host_pid / host_ipc / host_net "host namespace access"
protected_namespace             namespace in restricted list   "targets protected namespace"
wildcard_iam                    iam action == *                "wildcard IAM action"
open_security_group             cidr == 0.0.0.0/0             "world-open ingress"
public_s3                       public access block disabled   "public S3 bucket"
mass_destroy                    resource_count > threshold     "mass destructive operation"
hostpath_mount                  hostPath volume                "host filesystem access"
```

~10 detectors. Pure Go. Pattern matching on canonical payload.
No OPA. No Rego. No policy engine. ~200 lines of code.

Detectors don't produce allow/deny. They produce a list of
risk_details that enrich the prescription.

**Scope rule: detectors cover only catastrophic patterns.** Each
detector must answer: "has this pattern caused a production outage
or security incident at a real company?" If yes → detector. If no
→ out of scope. This excludes:

- Missing labels → not catastrophic
- No resource limits → not catastrophic
- YAML formatting → not catastrophic
- Mutable image tags → not catastrophic
- Missing health checks → not catastrophic

These are legitimate concerns, but they belong in linters and
admission controllers — not in a flight recorder.

### Prescription Output

```yaml
prescription:
  prescription_id: "prs-..."
  risk_level: "high"                  # from matrix
  risk_details:                       # from detectors
    - "privileged security context"
    - "targets protected namespace"
  artifact_digest: "sha256:..."
  signature: "..."
```

The agent sees: "Evidra says this is high risk because of
privileged security context and protected namespace." The agent
decides what to do with this information. A well-built agent
stops and asks the human. A poorly-built agent ignores it. Both
behaviors are recorded.

### No OPA. No Rego. No Policy Engine.

The entire analysis layer is:
- A fixed risk matrix (10 lines)
- ~10 catastrophic risk detectors (200 lines)
- Domain adapters to canonicalize raw artifacts (existing v3 code)

This eliminates:
- `github.com/open-policy-agent/opa` dependency (~15MB binary savings)
- All .rego files, bundle infrastructure, policy source loading
- OPA engine initialization on startup
- Rego testing infrastructure
- Policy versioning and bundle management

What remains is Go code. Testable with standard `go test`.
Readable by any engineer. No DSL to learn.

### Why Not Deny

If Evidra denies, it becomes an enforcer. Enforcers must be:
- In the execution path (Evidra is a sidecar)
- Trusted to have complete rules (Evidra has 10 detectors)
- Responsible for false positives (blocks legitimate operations)
- Maintained as infrastructure evolves (rule rot)

Evidra avoids all of this by being an inspector, not a judge.
The 10 detectors are informational. If they miss something,
the signal engine still catches behavioral anomalies. Defense
in depth through measurement, not through blocking.

### What Happens When a Catastrophic Pattern Is Found

Evidra doesn't block, but it lights the red lamp:

1. **Prescription includes risk_tags.** Agent sees: "world-open
   ingress" or "privileged security context." Smart agents stop
   and ask the human. Dumb agents proceed. Both recorded.

2. **Scorecard shows catastrophic context.** Dedicated section
   in the scorecard output:

```
CATASTROPHIC CONTEXT
  Events:   3 (0.07%)
  Patterns: privileged container (2), world-open ingress (1)
```

3. **Prometheus metric.** Low-cardinality counter for alerting:

```
evidra_catastrophic_context_total{agent="claude-code"} 3
```

Ops teams can alert on this metric: "any catastrophic context
event → PagerDuty." Evidra doesn't block — but the ops team's
alerting stack can.

This gives the "fire alarm" without the enforcement responsibility.

---

## 8. Architecture

### Three Components, Three Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│                        AI Agent Host                            │
│                                                                 │
│   ┌───────────┐      stdio/SSE       ┌──────────────────┐      │
│   │  AI Agent  │◄────────────────────►│  evidra-mcp      │      │
│   │ (Claude,   │   prescribe/report   │  (MCP server)    │      │
│   │  Cursor,   │                      │                  │      │
│   │  etc.)     │                      │  canon adapters  │      │
│   └───────────┘                       │  risk analysis   │      │
│                                       │  local evidence  │      │
│                                       └────────┬─────────┘      │
│                                                │ forward        │
└────────────────────────────────────────────────┼────────────────┘
                                                 │
┌────────────────────────────────────────────────┼────────────────┐
│                        CI Runner                │                │
│                                                │                │
│   ┌────────────┐                               │                │
│   │ terraform  │                               │                │
│   │ kubectl    │                               │                │
│   │ helm       │                               │                │
│   └─────┬──────┘                               │                │
│         │                                      │                │
│   ┌─────▼──────┐                               │                │
│   │ evidra CLI │   prescribe/report            │                │
│   │            │   (same protocol,             │                │
│   │            │    shell wrapper)              │                │
│   │            │                               │                │
│   │ canon      │                               │                │
│   │ risk       │                               │                │
│   │ local      │                               │                │
│   │ evidence   │───────────────────────────────┤                │
│   └────────────┘              forward          │                │
│                                                │                │
└────────────────────────────────────────────────┼────────────────┘
                                                 │
                                                 ▼
                              ┌──────────────────────────────┐
                              │       evidra-api             │
                              │       (backend)              │
                              │                              │
                              │  POST /v1/evidence/forward   │
                              │  GET  /v1/scorecard          │
                              │  GET  /v1/compare            │
                              │  GET  /v1/fleet              │
                              │  GET  /metrics               │
                              │                              │
                              │  evidence aggregation        │
                              │  signal computation          │
                              │  scorecard generation        │
                              │  Prometheus export           │
                              │  multi-tenant                │
                              └──────────────────────────────┘
```

### Component Responsibilities

**evidra-mcp** — MCP server. Runs alongside the AI agent.
Communicates via stdio or SSE transport. Exposes `prescribe` and
`report` tools. Contains canonicalization adapters, risk analysis,
and a local evidence JSONL. Forwards evidence entries to evidra-api
if configured. Stateless except for the local evidence file.

Deployment: sidecar process on the agent host. No network listener
needed (stdio). One instance per agent.

**evidra CLI** — Shell tool for CI pipelines. Same protocol as MCP
(prescribe/report), same canonicalization, same risk analysis.
Wraps around existing CI steps:

```bash
PRESCRIPTION=$(evidra prescribe --artifact plan.json --tool terraform)
terraform apply tfplan
evidra report --prescription $PRESCRIPTION --exit-code $?
```

Also provides scorecard/compare/fleet commands that can query
evidra-api or compute locally from evidence files.

Deployment: installed in CI runner image. Zero-config if evidence
is local. Points to evidra-api URL for centralized scorecards.

**evidra-api** — Backend server. Receives forwarded evidence from
all agents and CI pipelines. Aggregates evidence, computes signals,
generates scorecards, exports Prometheus metrics. Multi-tenant.

Deployment: one instance per organization. Runs on internal
infrastructure or as hosted service. Stores evidence in append-only
storage (file or database).

### Shared Core

All three components share the same Go packages:

```
internal/canon/       → canonicalization adapters (k8s, terraform, generic)
internal/risk/        → risk matrix + catastrophic detectors
internal/signal/      → 5 signal detectors
internal/evidence/    → evidence builder, signer, payload
pkg/evidence/         → evidence store, JSONL I/O
```

The difference is the shell:
- evidra-mcp wraps the core in MCP protocol
- evidra CLI wraps the core in shell commands
- evidra-api wraps the core in HTTP endpoints + aggregation

### Data Flow

```
Agent/CI → prescribe → [canon → risk → evidence] → prescription
Agent/CI → execute operation
Agent/CI → report → [match → signals → evidence] → verdict
                                    │
                                    ▼
                            local evidence.jsonl
                                    │
                              forward (push)
                                    │
                                    ▼
                             evidra-api (aggregate)
                                    │
                         ┌──────────┼──────────┐
                         ▼          ▼          ▼
                    scorecard   compare    /metrics
```

Local evidence is always written, even without evidra-api.
This means: agents and CI pipelines work fully offline.
evidra-api adds centralized view, cross-agent comparison,
and Prometheus metrics.

### What Runs Where

| Package | evidra-mcp | evidra CLI | evidra-api |
|---------|:----------:|:----------:|:----------:|
| internal/canon | yes | yes | no (receives canonical data) |
| internal/risk | yes | yes | no (receives risk data) |
| internal/signal | yes (local) | yes (local) | yes (aggregated) |
| internal/evidence | yes | yes | yes |
| internal/score | no | yes (local query) | yes (authoritative) |
| pkg/mcpserver | yes | no | no |
| pkg/evidence | yes | yes | yes |

### Delivery Timeline

| Component | Version | Scope |
|-----------|---------|-------|
| evidra CLI | v0.3.0 | prescribe, report, local scorecard |
| evidra-mcp | v0.3.0 | prescribe, report tools for MCP agents |
| evidra-api | v0.5.0 | centralized evidence, scorecards, metrics |

v0.3.0 works entirely local. No server needed. Evidence is a JSONL
file on disk. Scorecard reads from it directly.

v0.5.0 adds the backend for teams that need centralized view across
multiple agents and CI pipelines.

### No OPA. No Rego. No Policy Engine. No Deny.

Zero infrastructure privileges. Read-only. Analyzes what the
agent sends. Records everything. Computes scores.

### Signal Processor

Runs after every report. Five detectors, each a simple function:

```go
type SignalDetector interface {
    Detect(entry ProtocolEntry, history EvidenceReader) *SignalEvent
}
```

No background jobs. No streaming. No state machines. Each detector
reads the evidence chain if needed (new scope checks history,
retry loop checks recent entries). Evidence chain is the only
state store.

### Scorecard Computation

On-demand, not pre-computed. `evidra scorecard` reads the evidence
chain, counts signals per window, applies the formula. Fast enough
for tens of thousands of entries (JSONL scan + count).

For the hosted platform (v0.5.0): pre-aggregated daily, queryable
via API, Prometheus metrics.

---

## 9. What This Becomes

**Short term:** A tool that teams install to measure AI agents
and CI pipelines before putting them in production.

**Medium term:** The standard behavioral telemetry layer for
infrastructure automation. Like how Prometheus standardized metrics
and OpenTelemetry standardized traces — Evidra standardizes
automation behavior signals. Infrastructure tools emit Evidra
signals natively. Security platforms (Wiz, Orca, Trivy) enrich
signals with infrastructure context.

**Long term:** The reliability benchmark for all automated
infrastructure operations. "What's your Evidra score?" becomes
a meaningful question when evaluating any automation — AI agents,
CI pipelines, GitOps controllers, internal tools.

The path to standard:

```
1. Open source the five signals, score formula, and adapter interface.
2. Ship built-in adapters for K8s + Terraform (v0.3.0).
3. Pre-canonicalized path lets any tool integrate without Evidra code changes.
4. Agent frameworks integrate prescribe/report natively.
5. Security platforms enrich signals with infrastructure context.
6. Scores become comparable across organizations.
7. Evidra score becomes a procurement criterion for AI agents.
8. Same standard adopted for all infrastructure automation.
```

Steps 1-3 ship in v0.3.0. Step 3 is the key: the pre-canonicalized
prescribe path means Pulumi, Ansible, CloudFormation, custom tools
integrate on day one without waiting for a dedicated adapter. The
adapter interface is open for anyone to implement.

The strategic position: **Evidra is to automation behavior what
Prometheus is to infrastructure metrics.**

---

## 10. Metrics and Signal Definitions

Metric registry (names, types, labels, cardinality rules) and
signal definitions are in **EVIDRA_SIGNAL_SPEC.md** (normative).

Key points for benchmark consumers:
- All metrics prefixed `evidra_`
- Only low-cardinality labels allowed (agent, tool, scope, signal)
- Forbidden: prescription_id, artifact_digest, resource_name, model_id
- `evidra_reliability_score{agent}` gauge updated at scorecard time
- Conformance test harness with 10 cases in `tests/signal_conformance/`

---

## 11. Implementation

### v0.3.0 — Foundation
- Domain adapters + raw artifact input (Engine v3)
- prescribe / report MCP tools
- Evidence chain with protocol entries

### v0.4.0 — Benchmark
- Five signal detectors
- Reliability score computation
- `evidra scorecard` CLI
- `actor_meta` labels for version/model/prompt comparison
- `evidra compare` CLI (two agents or versions side by side)
- Prometheus /metrics (five signal counters + score gauge)

### v0.5.0 — Platform
- Hosted scorecard (web UI)
- Multi-agent comparison dashboard
- Signed PDF scorecard export
- Telemetry forwarder (push mode)
- API: GET /v1/scorecard, GET /v1/compare

### v0.6.0 — Ecosystem
- Agent framework SDKs (prescribe/report wrappers)
- Public benchmark registry (opt-in, anonymized)
- LangSmith/Langfuse correlation
- Compliance report generation

---

## 12. CI Integration

CI pipelines are the richest data source for the benchmark. An
MCP agent does 10-50 operations per day. A CI pipeline does
hundreds. More data = more reliable score.

### Integration is trivial

CI already has the artifacts. `terraform show -json` output,
rendered manifests, helm template output — all produced as part
of the pipeline. Two CLI calls wrap the existing step:

```yaml
# Before
- run: kubectl apply -f manifest.yaml

# After
- run: |
    PRESCRIPTION=$(evidra prescribe \
      --tool kubectl --op apply \
      --artifact manifest.yaml \
      --actor-id "github-actions" \
      --actor-meta agent_version=${{ github.action_ref }} \
      --env production)
    
    kubectl apply -f manifest.yaml
    EXIT_CODE=$?
    
    evidra report \
      --prescription $PRESCRIPTION \
      --artifact manifest.yaml \
      --exit-code $EXIT_CODE
```

Two lines added. Same kubectl. Same manifest. But now the
operation is prescribed, reported, and scored.

### Terraform is even simpler

Terraform plan JSON is the raw artifact. It already exists in
every CI pipeline that does `terraform plan`:

```yaml
- run: |
    terraform plan -out=tfplan
    terraform show -json tfplan > plan.json
    
    PRESCRIPTION=$(evidra prescribe \
      --tool terraform --op apply \
      --artifact plan.json \
      --env production)
    
    terraform apply tfplan
    EXIT_CODE=$?
    
    evidra report \
      --prescription $PRESCRIPTION \
      --artifact plan.json \
      --exit-code $EXIT_CODE
```

### CI feedback enriches evidence

CI provides data that MCP agents can't:

- **Exit codes.** Did the operation succeed? MCP agents may not
  report failures. CI always has the exit code.
- **Pipeline identity.** Which workflow, which run, which commit
  triggered this operation. Goes into `actor_meta`.
- **Duration.** How long did the operation take. Timestamp delta
  between prescribe and report.
- **Artifact provenance.** The manifest came from git commit X,
  branch Y, PR Z. Richer context for the evidence chain.

```yaml
actor:
  type: ci
  id: github-actions
  provenance: cli
  instance_id: "runner-12345"      # runtime instance (protocol v1.0)
  version: "deploy-v2.1"          # pipeline version (protocol v1.0)
session_id: "gh-run-12345"        # CI run as session boundary
scope_dimensions:
  account: "prod-aws-123"
  region: "us-east-1"
actor_meta:                        # finer-grained comparison dimensions
  pipeline_id: "deploy-production"
  commit_sha: "abc123"
  branch: "main"
  pr_number: "456"
  triggered_by: "merge"
```

All of this lands in the evidence chain. The scorecard for a CI
pipeline is richer than for an interactive agent because CI gives
more metadata per operation.

### Drift detection in CI

CI is where drift signals are most valuable. A pipeline is
supposed to be deterministic — same workflow, same operations,
same scopes. When drift appears in CI, it means someone changed
the pipeline definition or the infrastructure targets:

- New namespace in the deploy step → `new_scope` signal
- Blast radius jumped (100 resources instead of usual 20) →
  `blast_radius` signal
- Pipeline started doing deletes where it only did applies →
  `new_scope` signal (new operation class)

These are real signals that something changed in the pipeline.
In an interactive agent, drift might be normal exploration. In
CI, drift is almost always a configuration change worth reviewing.

### Scorecard on PR

The highest-value CI integration: scorecard as a PR check.

```yaml
# .github/workflows/evidra-check.yml
on: pull_request

jobs:
  reliability:
    runs-on: ubuntu-latest
    steps:
      - run: |
          evidra scorecard \
            --actor-id github-actions \
            --period 30d \
            --format json > score.json
          
          SCORE=$(jq '.score' score.json)
          if (( $(echo "$SCORE < 95" | bc -l) )); then
            echo "::error::Reliability score $SCORE below threshold"
            exit 1
          fi
```

PR doesn't merge if the pipeline's reliability score dropped
below threshold. Not because Evidra blocked an operation — because
the historical score says this pipeline has been unreliable.

This is the benchmark in action: CI reliability as a measurable,
enforceable (by CI, not by Evidra) standard.

---

One adoption path that works without a platform, without SaaS,
without anything beyond the binary and a CI job.

## 13. Golden Path: Getting Started

### Step 1: Install (5 minutes)

```bash
# Download binary
curl -L https://github.com/vitas/evidra/releases/latest/download/evidra-mcp -o /usr/local/bin/evidra-mcp
chmod +x /usr/local/bin/evidra-mcp

# Add to Claude Code / Cursor MCP config
# (agent starts calling prescribe/report automatically)
```

### Step 2: Run for 2 weeks (passive)

Agent operates normally. Evidra records every prescribe/report
exchange. Evidence chain accumulates. No changes to agent behavior.

### Step 3: Check scorecard

```bash
evidra scorecard --agent claude-code --period 14d
```

First scorecard. First numbers. First conversation with the team:
"our agent has a 99.2 reliability score, 2 retry loops, zero
artifact drift."

### Step 4: Add to CI (GitHub Actions example)

```yaml
# .github/workflows/agent-scorecard.yml
name: Agent Reliability Check
on:
  schedule:
    - cron: '0 9 * * 1'    # weekly Monday 9am
  pull_request:

jobs:
  scorecard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Generate scorecard
        run: |
          evidra scorecard --agent ${{ github.actor }} --period 30d --format json > scorecard.json
          SCORE=$(jq '.score' scorecard.json)
          echo "Reliability score: $SCORE"
          if (( $(echo "$SCORE < 95" | bc -l) )); then
            echo "::warning::Agent reliability score below threshold: $SCORE"
          fi
      - name: Comment on PR
        if: github.event_name == 'pull_request'
        uses: actions/github-script@v7
        with:
          script: |
            const score = require('./scorecard.json');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: `**Agent Reliability Score: ${score.score}**\nOps: ${score.operations} | Drift: ${score.drift_rate} | Violations: ${score.violation_rate}`
            });
```

This gives real ops value from week 3. No platform. No SaaS.
A binary, a CI job, and a PR comment with the score.

---

## 14. Do NOT

- Do not deny. Evidra never blocks agent execution. Ever.
- Do not add OPA, Rego, or any policy engine. Risk detectors
  are Go functions, not policy rules.
- Do not add more than five signals in v1. Resist complexity.
- Do not add ML, z-scores, percentile trackers, or learned
  baselines in v1. Simple counters and thresholds.
- Do not verify agent reports against infrastructure. Inspector
  model: prescribe, record, score. Not enforce.
- Do not require infrastructure credentials. Zero-privilege.
- Do not pre-compute scores continuously. On-demand from
  evidence chain is sufficient for v1.
- Do not let the signal count grow past 10 without very strong
  justification. Each signal added is maintenance forever.
- Do not let risk detectors grow past 15. They cover catastrophic
  patterns only. "Has this caused an outage?" — if no, it's not
  a detector.
- Do not compete with Gatekeeper, Kyverno, or Sentinel. They
  enforce. Evidra measures. Different job.
