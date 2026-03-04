
# Evidra — End-to-End Example (v2)
## Status
Worked example. Updated to match **Canonicalization Contract v1** and the current **Benchmark** model.

---

## Scenario

Company: FinTech Corp, ~200 engineers.  
Actors:

- **AI agent**: Claude Code via MCP, operating on Kubernetes (staging).
- **CI**: GitHub Actions running Terraform + kubectl apply.
- **Evidra**:
  - MCP tool endpoint for agent calls (`prescribe`, `report`)
  - CLI wrapper in CI (`evidra prescribe`, `evidra report`)
  - Append-only evidence chain (JSONL, hash-linked, signed)

Goal:
- Record actions, compute the five signals, produce a comparable scorecard.

---

## Canonicalization Fundamentals (Contract v1)

Two digests are always produced:

```
artifact_digest = SHA256(raw bytes as received)
intent_digest   = SHA256(canonical JSON of canonical_action)
```

- `artifact_digest` detects **raw artifact modification** (protocol integrity).
- `intent_digest` identifies **behavioral identity** (same action intent across formatting noise).

Canonical action schema (simplified):

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "operation_class": "mutating",
  "resource_identity": [
    {"api_version":"apps/v1","kind":"Deployment","namespace":"staging","name":"api-server"}
  ],
  "scope_class": "staging",
  "resource_count": 1,
  "resource_shape_hash": "sha256:...",
  "risk_tags": []
}
```

Notes:
- `resource_identity` is stable identity (what resources).
- `resource_shape_hash` captures normalized spec shape for detectors (not part of `intent_digest`), used to reduce false retry-loop positives when the same resource is intentionally modified.

---

# Part 1 — MCP Agent Flow (Kubernetes)

## Step 1: Agent produces a manifest (raw artifact)

The agent plans to deploy a new image to staging.

Raw artifact bytes (multi-doc YAML possible; here single object):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: staging
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: "..."   # NOISE
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
      - name: api
        image: registry.corp.com/api-server:v2.4.1
```

## Step 2: Agent calls prescribe()

Request (MCP → Evidra):

- tool: `kubectl`
- operation: `apply`
- raw_artifact: the YAML bytes above
- scope hint: `staging` (optional; adapter can derive via namespace mapping)

Evidra computes `artifact_digest` immediately from raw bytes:

```
artifact_digest = SHA256(raw_yaml_bytes)
```

## Step 3: Evidra canonicalizes the artifact (k8s adapter)

Evidra parses YAML using Kubernetes unstructured decoding and applies **noise filtering**:

Noise removed / ignored:
- `metadata.managedFields`
- `metadata.resourceVersion`
- `metadata.uid`
- `metadata.creationTimestamp`
- `status`
- known noisy annotations (example: `kubectl.kubernetes.io/last-applied-configuration`)

Objects are sorted by (apiVersion, kind, namespace, name). (No effect here: 1 object.)

Evidra produces a canonical action:

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "operation_class": "mutating",
  "resource_identity": [
    {"api_version":"apps/v1","kind":"Deployment","namespace":"staging","name":"api-server"}
  ],
  "scope_class": "staging",
  "resource_count": 1,
  "resource_shape_hash": "sha256:9a1b...",
  "risk_tags": []
}
```

Then:

```
intent_digest = SHA256(canonical_json(canonical_action))
```

## Step 4: Risk detectors (Golden Disaster context)

Evidra runs **small catastrophic risk detectors** (context, not full compliance).

Examples it may detect:
- privileged container
- hostNetwork/hostPID
- hostPath mount
- cluster-admin RBAC
- public exposure patterns

In this example, no catastrophic patterns are found:

```
risk_tags = []
```

## Step 5: Evidra writes a prescription entry to the evidence chain

Evidence JSONL entry (illustrative fields):

```json
{
  "type": "prescription",
  "ts": "2026-03-04T10:12:10Z",
  "actor": {"id":"claude-code","kind":"mcp_agent","agent_version":"1.2.0","model_id":"claude"},
  "canonicalization_version": "k8s/v1",
  "adapter_versions": {"k8s_adapter":"0.1.0"},
  "tool": "kubectl",
  "operation": "apply",
  "artifact_digest": "sha256:...",
  "intent_digest": "sha256:...",
  "canonical_action": { "...": "..." },
  "chain": {"prev_hash":"sha256:...","entry_hash":"sha256:...","signature":"ed25519:..."}
}
```

Evidra returns the prescription to the agent. No allow/deny —
just risk assessment and the recorded intent:

```json
{
  "prescription_id": "prs-01JD7KX9M2",
  "risk_level": "medium",
  "risk_details": [],
  "artifact_digest": "sha256:...",
  "intent_digest": "sha256:...",
  "canonicalization_version": "k8s/v1",
  "signature": "ed25519:..."
}
```

risk_level = medium (mutating × staging from the risk matrix).
No risk_details because no catastrophic patterns found.

## Step 6: Agent executes kubectl apply

The agent runs the real command in its environment.

## Step 7: Agent calls report()

The agent returns:

- exit_code: 0
- raw_artifact_digest it claims it applied (or the same artifact bytes again)
- optional external_refs (if available): e.g., kubectl command log id

Report entry:

```json
{
  "type": "report",
  "ts": "2026-03-04T10:12:30Z",
  "actor": {"id":"claude-code","kind":"mcp_agent"},
  "tool": "kubectl",
  "operation": "apply",
  "exit_code": 0,
  "artifact_digest": "sha256:...",
  "external_refs": []
}
```

## Step 8: Protocol verdict + signals

Evidra compares the report to the latest open prescription for the same actor/task.

Possible verdicts:
- compliant
- deviation
- unreported
- unprescribed

In this example:
- report exists
- tool/operation match
- artifact_digest matches

Verdict:

```
protocol_verdict = compliant
```

Signals updated:
- Protocol Violation: 0
- Artifact Drift: 0
- Retry Loop: 0
- Blast Radius: 0 (resource_count=1, mutating threshold not exceeded)
- New Scope: 0 (staging already known)

A protocol entry is appended to evidence:

```json
{
  "type": "protocol_entry",
  "ts": "2026-03-04T10:12:31Z",
  "actor": {"id":"claude-code"},
  "intent_digest": "sha256:...",
  "verdict": "compliant",
  "signals_delta": {
    "protocol_violation": 0,
    "artifact_drift": 0,
    "retry_loop": 0,
    "blast_radius": 0,
    "new_scope": 0
  },
  "chain": {"prev_hash":"sha256:...","entry_hash":"sha256:...","signature":"ed25519:..."}
}
```

---

# Part 2 — CI Flow (Terraform + kubectl)

## Step 1: Terraform plan in CI (raw artifact)

CI runs:

```
terraform plan -out=plan.out
terraform show -json plan.out > plan.json
```

`plan.json` is the raw artifact bytes for prescribe.

## Step 2: CI calls evidra prescribe (terraform)

Evidra computes:

```
artifact_digest = SHA256(plan.json bytes)
```

Terraform adapter extracts resource changes and builds canonical action(s):

Example canonical action (simplified):

```json
{
  "tool": "terraform",
  "operation": "apply",
  "operation_class": "mutating",
  "resource_identity": [
    {"type":"aws_security_group","name":"web","actions":["update"]}
  ],
  "scope_class": "production",
  "resource_count": 1,
  "resource_shape_hash": "sha256:4c2e...",
  "risk_tags": ["world-open ingress"]
}
```

Note: `address` (e.g. `module.vpc.aws_security_group.web`) is NOT
used in identity — it's unstable across module refactors. Identity
uses `type + name + actions` only. Full address is preserved in
evidence for human readability.

If a catastrophic detector finds a pattern (e.g., 0.0.0.0/0 ingress),
it is recorded in `risk_tags`. Evidra remains an inspector — no
blocking. The risk is visible in the prescription and in the scorecard.

Prescription returned:

```json
{
  "prescription_id": "prs-01JD8ABC12",
  "risk_level": "high",
  "risk_details": ["world-open ingress"],
  "artifact_digest": "sha256:...",
  "intent_digest": "sha256:...",
  "canonicalization_version": "tf/v1",
  "signature": "ed25519:..."
}
```

risk_level = high because risk_tags elevated it above the matrix
value (mutating × production = medium, but catastrophic detector
found open ingress → high).

## Step 3: CI executes terraform apply and reports

After apply, CI reports exit_code and references:

```json
{
  "type": "report",
  "tool": "terraform",
  "operation": "apply",
  "exit_code": 1,
  "external_refs": [
    {"type":"github_actions_run","id":"123456789"},
    {"type":"terraform_apply_log","id":"job-step-7"}
  ]
}
```

This produces signals such as:
- Retry Loop (if CI re-runs the same intent repeatedly after failure/deny)
- Protocol Violation (if report missing)
- Artifact Drift (if report digest differs from prescribed digest)

---

# Part 3 — Scorecard Output

A scorecard is computed over a selected window (e.g., last 30 days)
for each actor. It includes tool and scope breakdowns.

```
AGENT SCORECARD: claude-code
Period: 30 days
Operations: 4,217

AGGREGATE
  Reliability Score: 99.97 / 100

SIGNALS
  Protocol Violations:     2    (0.05%)
  Artifact Drifts:         1    (0.02%)
  Retry Loops:             3    (0.07%)
  Blast Radius Spikes:     0
  New Scopes:              0

BY TOOL
  kubectl    3,891 ops   score 99.98   drift 0.01%  retry 0.05%
  terraform    326 ops   score 99.69   drift 0.00%  retry 0.31%

BY SCOPE
  staging      3,450 ops  score 99.99
  production     767 ops  score 99.87

PROTOCOL VIOLATION BREAKDOWN
  → 1x stalled_operation (prescription issued, no report after 5min)
  → 1x crash_before_report (agent sent new prescribe without prior report)

RECENT SIGNALS
  2026-03-03 14:22  retry_loop       kubectl.apply (3 attempts, same shape_hash)
  2026-02-28 09:11  artifact_drift   kubectl.apply (digest mismatch)
  2026-02-15 11:45  protocol_violation  stalled_operation (no report)
```

### Scope-Aware Comparison

Comparing agents that do different work is misleading. Evidra checks
workload overlap before comparing:

```
evidra compare --actors claude-code,ci-pipeline

WARNING: Low workload overlap between agents.
  claude-code:   kubectl (staging, production)
  ci-pipeline:   terraform (production)
  Overlap: 0%. Comparison not meaningful.
  Use --force to compare anyway.
```

Fair comparison filters by shared tool+scope:

```
evidra compare --actors claude-code,cursor-agent --tool kubectl --scope production

COMPARISON: kubectl in production
──────────────────────────────────────────────────────
Agent         Ops    Drift  Retry  Violations  Score
───────────── ────── ────── ────── ────────── ──────
claude-code   767    0.00%  0.13%  0.00%      99.95
cursor        312    0.32%  0.64%  0.00%      99.22
──────────────────────────────────────────────────────
```

Version comparison is always valid (same agent, same workload):

```
evidra compare --actor claude-code --versions v1.2,v1.3
```

---

# Failure Cases (How the system behaves)

## Agent crashes before report
- Prescription exists, no report within TTL (default 5 min)
- Protocol Violation signal increments
- Sub-signal: **crash_before_report** (detected when agent sends
  a new prescribe without reporting the previous one)

## Agent hangs during execution
- Prescription exists, no report within TTL, no further activity
- Protocol Violation signal increments
- Sub-signal: **stalled_operation** (agent is hung, not crashed)
- Both sub-signals count as protocol_violation in the score.
  The breakdown helps agent developers diagnose: "crashes" vs "hangs"

## Agent modifies artifact after prescribe
- prescribe.artifact_digest ≠ report.artifact_digest
- Artifact Drift signal increments
- Note: this is self-reported consistency. Agent could lie
  consistently (send same digest both times). Evidra catches
  inconsistency, not ground truth.

## Agent retries same failed operation
- Same intent_digest AND same resource_shape_hash, N times in T minutes
- Retry Loop signal fires
- Important: if shape_hash differs (agent changed the artifact
  between attempts), it's NOT a retry loop — it's iteration.

## Agent acts without calling prescribe
- Report received with no matching prescription
- Protocol Violation signal increments (unprescribed action)

## Adapter cannot parse artifact
- canonical_action cannot be produced
- Prescription still issued with:
  - risk_level: high
  - risk_details: ["artifact parse failure: ..."]
  - intent_digest: null
  - artifact_digest: computed (raw bytes always work)
- Recorded in evidence. Not in reliability score (not the
  agent's behavioral fault).

## Two agents, same cluster, different scores
- Same infrastructure, same time period, different workload profiles
- Evidra produces per-agent scorecards
- Cross-agent comparison only meaningful when workload overlaps
  (same tool + same scope)

---

# Appendix: Other Adapters (Spec Reserved)

### Helm

Helm template output is Kubernetes YAML. No separate adapter.

```
helm template my-chart -f values.yaml | evidra prescribe --tool helm --op upgrade --artifact -
```

The K8s adapter parses the YAML. tool="helm" in canonical_action
(distinguishes from kubectl in scorecard breakdowns). Everything
else is identical to the K8s flow above.

### ArgoCD

ArgoCD sync events produce Kubernetes manifests. Two paths:

```
argocd app sync → rendered manifests → K8s adapter
argocd app create → Application CRD YAML → K8s adapter (CRD as unstructured)
```

ArgoCD adapter (v0.5.0+) will add sync-specific metadata
(Application name, target revision) to actor_meta.

---

End of worked example.
