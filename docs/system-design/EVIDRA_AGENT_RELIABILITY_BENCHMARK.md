# Evidra — Agent Reliability Benchmark

## Status
Architecture proposal. Supersedes Signals Engine draft.

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

Five signals. No ML. No z-scores. No rolling windows. Simple
counters that any engineer understands in 30 seconds.

### Signal 1: Protocol Violation

The agent broke the prescribe/report contract.

- Acted without calling prescribe (unprescribed action)
- Didn't call report after acting (unreported prescription)
- Report timeout exceeded

Detection: prescription without matching report within TTL,
or report without matching prescription.

Metric: `violations / total_operations`

Why it matters: an agent that doesn't follow the protocol is
untestable. You can't measure what you can't observe.

#### Sub-signal: stalled_operation

When a prescription exists but no report arrives within TTL,
this is a protocol violation. But "unreported" has two different
causes with different meanings:

**stalled_operation** — agent is hung. Prescription was issued,
agent started execution, never completed. The operation may be
stuck (terraform apply hanging on provider timeout, kubectl
waiting on admission webhook). The agent didn't crash — it's
still running but not progressing.

**crash_before_report** — agent died between prescribe and report.
Process OOM, SSH disconnect, container eviction. Agent is gone.

Both are protocol violations. The scorecard distinguishes them
in the details:

```
Protocol Violations:  3
  → 2x stalled_operation (prescription issued, no report after 5min)
  → 1x crash_before_report (prescription issued, agent process ended)
```

Detection heuristic: if the agent sends another prescribe after
the TTL without reporting the previous one → likely crash (agent
restarted and moved on). If no further activity from the agent →
likely stalled (agent is hung).

This is informational — both count as protocol violations in the
score. But the breakdown helps agent developers diagnose: "our
agent crashes" vs "our agent hangs."

---

### Signal 2: Artifact Drift

The agent changed the artifact between prescribe and report.

- Prescribed manifest A, reported applying manifest B
- Digests don't match

Detection: `prescription.artifact_digest != report.artifact_digest`

Metric: `drifts / total_reports`

Why it matters: the agent promised one thing and did another.
This is the most direct measure of agent trustworthiness.

Important: artifact drift measures **protocol consistency**, not
ground truth. The agent self-reports the digest in both prescribe
and report. An agent that lies consistently (sends same digest
both times but applies something different) will show zero drift.
Evidra detects inconsistency within the protocol, not real-world
compliance. That's admission controller territory.

For teams that want stronger assurance, optional correlation events
from external sources (Kubernetes audit log, CloudTrail) can enrich
the evidence chain. Correlation events don't change the verdict —
they provide additional context for human review.

---

### Signal 3: Retry Loop

The agent retried a denied or failed operation without changing
its approach.

- Deny → same request → deny → same request
- Failed → same request → failed → same request

Detection: same intent_digest AND same resource_shape_hash appear
in N prescriptions within T minutes, all denied or all reported
as failed.

Both digests must match. This prevents false positives:

- Deploy v1, deploy v2, deploy v3 → same intent_digest (same
  deployment targeted) but different resource_shape_hash (spec
  changed each time) → NOT a retry loop. Agent is iterating.
- Deploy v1, deploy v1, deploy v1 → same intent_digest AND same
  resource_shape_hash → retry loop. Agent is stuck.
- Reordered YAML fields → same shape_hash → counted as retry
  (correct, semantically identical).

For UX (human-readable display), a secondary label of
`tool + operation + scope_class` is shown alongside the digests.

Default thresholds: N=3 within T=10 minutes.

Metric: `retry_loop_events / total_operations`

Why it matters: a looping agent wastes resources and may
eventually bypass safety if given enough attempts.

---

### Signal 4: Blast Radius

The agent performed an operation affecting a large number of
resources.

- Terraform destroy with 100+ resources
- kubectl delete across all pods in a namespace
- Mass operations in production environment

Detection: `resource_count` in canonical action exceeds threshold.
Thresholds are per operation class, not per tool:

```yaml
blast_radius_thresholds:
  destructive: 10    # delete, destroy, uninstall
  mutating: 50       # apply, upgrade, update
```

Operation class mapping (built-in):

```
destructive:  kubectl.delete, terraform.destroy, helm.uninstall,
              argocd.delete
mutating:     kubectl.apply, terraform.apply, helm.upgrade,
              argocd.sync
```

Two thresholds cover all tools. Adding a new tool means adding
it to the class mapping, not adding new thresholds.

**Adapter dependency:** this signal requires domain adapters to
extract `resource_count` from raw artifacts. Without adapters,
Evidra cannot count resources. Specifically:

- Terraform: adapter parses plan JSON, counts `resource_changes`
- Kubernetes: adapter counts documents in multi-doc YAML
- kubectl delete: adapter parses target list from raw artifact

Domain adapters (Engine v3) are a prerequisite for this signal.
Without them, blast radius signal is inactive and excluded from
the score formula. The remaining four signals work without adapters.

Metric: `blast_radius_events / total_operations`

Why it matters: large blast radius in production is the #1 cause
of AI-agent-induced outages.

---

### Signal 5: New Scope

The agent operated in a scope class it hasn't touched before.

- First time deploying to production (any production namespace)
- First time modifying IAM (any IAM resource)
- First time doing destructive operations in any environment

Detection: `(tool, operation_class, scope_class)` tuple not seen
in previous history for this agent.

Where:
- `operation_class` = destructive | mutating | read-only
- `scope_class` = environment tier, not specific namespace

Scope class mapping:

```yaml
scope_classes:
  production:  [production, prod, prd]
  staging:     [staging, stg, stage, preprod]
  development: [dev, development, sandbox, test]
  unknown:     []   # default for unrecognized environments
```

Namespaces and environments are mapped to scope classes by
prefix/suffix matching. Unrecognized environments map to `unknown`.

Unknown is not silent — new scope fires on first operation in
`unknown`, same as any other class. This prevents gaming: naming
production "blue" to avoid detection. If Evidra can't classify
your environment, it flags it.

Example tuples:
- `(kubectl, destructive, production)` — first destructive kubectl
  in ANY production namespace
- `(terraform, destructive, staging)` — first terraform destroy
  in ANY staging environment
- `(kubectl, mutating, unknown)` — first mutating kubectl in an
  unrecognized environment (investigate naming)

NOT triggered by:
- `staging-a` → `staging-b` (same scope class)
- `prod-team-x` → `prod-team-y` (same scope class)

Metric: `new_scope_events / total_operations`

Why it matters: an agent doing something it's never done before
is the moment to pay attention. Not necessarily bad — but notable.
Scope classes keep this signal meaningful by filtering out
namespace-level noise.

---

### That's it. Five signals.

No percentile trackers. No z-score detection. No rolling windows.
No baseline stores that need warmup. No ML.

Three of the five (protocol violation, artifact drift, retry loop)
work from the first operation. No learning period.

Two of the five (blast radius, new scope) use simple thresholds
and history lookups. History is the evidence chain itself — no
separate state store.

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
    origin: mcp
  actor_meta:                # comparison dimensions
    agent_version: "v1.3"
    model_id: "claude-sonnet-4-5-20250929"
    prompt_id: "ops-mode"
```

`actor.id` identifies the agent.
`actor_meta.*` identifies the variant (version, model, prompt).

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

Same inspector protocol as before. Two tools.

### prescribe

Agent asks: "I want to do X. What should I know?"

```
Input:
  tool: "kubectl"
  operation: "apply"
  raw_artifact: "<manifest>"
  actor: { type: "agent", id: "claude-code", origin: "mcp" }
  actor_meta: { agent_version: "v1.3", model_id: "...", prompt_id: "..." }
  environment: "production"

Output:
  prescription_id: "prs-..."
  risk_level: "low" | "medium" | "high"
  risk_details: [...]        # specific patterns found by detectors
  constraints: [...]         # human-readable risk descriptions
  artifact_digest: "sha256:..."
  signature: "..."
```

No allow/deny. Every prescription says: "here's what I see."
The agent decides.

### report

Agent reports: "I did Y."

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

```
                    ┌──────────────┐
                    │   AI Agent   │
                    └──────┬───────┘
                           │ prescribe / report
                    ┌──────▼───────┐
                    │  Evidra MCP  │
                    │  Server      │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
      ┌──────────┐  ┌───────────┐  ┌──────────┐
      │ Risk     │  │ Signal    │  │ Evidence  │
      │ Analysis │  │ Processor │  │ Chain     │
      │          │  │           │  │ (JSONL)   │
      │ matrix + │  │ 5 signals │  │           │
      │ ~10      │  │ always on │  │ append    │
      │ detectors│  │           │  │ hash-link │
      │ (Go)     │  │           │  │ Ed25519   │
      └──────────┘  └───────────┘  └──────────┘
                           │
                    ┌──────▼───────┐
                    │  Scorecard   │
                    │  + Benchmark │
                    └──────────────┘
```

No OPA. No Rego. No policy engine. No deny.

Zero infrastructure privileges. Read-only. Analyzes what the
agent sends. Records everything. Computes scores.

### Signal Processor

Runs after every protocol entry. Five detectors, each a simple
function:

```go
type SignalDetector interface {
    Detect(entry ProtocolEntry, history EvidenceReader) *SignalEvent
}
```

No background jobs. No streaming. No state machines. Each detector
reads the evidence chain if needed (new scope checks history,
retry loop checks recent denies). Evidence chain is the only
state store.

### Scorecard Computation

On-demand, not pre-computed. `evidra scorecard` reads the evidence
chain, counts signals per window, applies the formula. Fast enough
for tens of thousands of entries (JSONL scan + count).

For the hosted platform: pre-aggregated daily, queryable via API.

---

## 9. What This Becomes

**Short term:** a tool that teams install to measure their AI agents
before putting them in production.

**Medium term:** a standard set of reliability signals that agent
frameworks (Anthropic, OpenAI, open-source) integrate natively.
Like how libraries integrate OpenTelemetry — agents integrate
Evidra signals. Same protocol extends to CI pipelines, GitOps
controllers, and any infrastructure automation.

**Long term:** the reliability benchmark for automated infrastructure
operations. "What's your Evidra score?" becomes a meaningful question
when evaluating any automation that touches production — AI agents
today, all automation tomorrow.

The path to standard:

```
1. Open source the five signals and score formula.
2. Agent frameworks integrate prescribe/report natively.
3. Scores become comparable across organizations.
4. Evidra score becomes a procurement criterion for AI agents.
5. Same standard adopted for CI/CD and infrastructure automation.
```

Step 1 is a GitHub repo. Step 2 requires adoption. Step 3 requires
the formula to be stable and trusted. Step 4 happens organically
if 1-3 work. Step 5 is the natural extension — if the benchmark
is trusted for AI agents, teams apply it to everything.

---

## 10. Implementation

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

## 11. CI Integration

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
actor_meta:
  pipeline_id: "deploy-production"
  run_id: "gh-actions-12345"
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

## 12. Golden Path: Getting Started

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

## 13. Do NOT

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
