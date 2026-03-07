
# Evidra — End-to-End Example (v2)
## Status
Worked example. Updated to match **Canonicalization Contract v1**, the current
**Benchmark** model, and the v0.3.x codebase.

---

## Scenario

Company: FinTech Corp, ~200 engineers.
Actors:

- **AI agent**: Claude Code via MCP, operating on Kubernetes (staging).
- **CI**: GitHub Actions running Terraform + kubectl apply.
- **Evidra**:
  - MCP tool endpoint for agent calls (`prescribe`, `report`)
  - CLI wrapper in CI (`evidra prescribe`, `evidra report`)
  - Append-only evidence chain (JSONL, hash-linked)

Goal:
- Record actions, compute the seven signals, produce a comparable scorecard.

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
  "operation_class": "mutate",
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
- `operation_class` uses `mutate` (not `mutating`) — consistent with risk matrix enum.

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
- actor: `{"type":"ai_agent","id":"claude-code","provenance":"mcp","instance_id":"pod-abc123","version":"v1.3"}`
- session_id: `"session-20260304-staging"` (optional, auto-generated if omitted)
- scope_dimensions: `{"cluster":"staging-us-east","namespace":"staging"}`

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
  "operation_class": "mutate",
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

Evidence JSONL entry (actual `EvidenceEntry` envelope):

```json
{
  "entry_id": "01JD7KX9M2...",
  "previous_hash": "sha256:...",
  "hash": "sha256:...",
  "signature": "base64-ed25519-signature",
  "type": "prescribe",
  "session_id": "session-20260304-staging",
  "trace_id": "01JD7KX9M1...",
  "span_id": "span-prescribe-001",
  "actor": {"type":"ai_agent","id":"claude-code","provenance":"mcp","instance_id":"pod-abc123","version":"v1.3"},
  "timestamp": "2026-03-04T10:12:10Z",
  "intent_digest": "sha256:...",
  "artifact_digest": "sha256:...",
  "payload": {
    "prescription_id": "01JD7KX9M2...",
    "canonical_action": {"tool":"kubectl","operation":"apply","operation_class":"mutate","...":"..."},
    "risk_level": "medium",
    "risk_tags": [],
    "ttl_ms": 300000,
    "canon_source": "adapter"
  },
  "scope_dimensions": {"cluster":"staging-us-east","namespace":"staging"},
  "spec_version": "0.3.1",
  "canonical_version": "k8s/v1",
  "adapter_version": "0.3.1"
}
```

Note: v0.3.1 writes signed entries. Strict mode requires configured keys;
optional mode uses an ephemeral in-process key for local/test runs.

Evidra returns the prescription to the agent. No allow/deny —
just risk assessment and the recorded intent:

```json
{
  "ok": true,
  "prescription_id": "01JD7KX9M2...",
  "risk_level": "medium",
  "risk_tags": [],
  "risk_details": [],
  "artifact_digest": "sha256:...",
  "intent_digest": "sha256:...",
  "resource_shape_hash": "sha256:...",
  "resource_count": 1,
  "operation_class": "mutate",
  "scope_class": "staging",
  "canon_version": "k8s/v1"
}
```

risk_level = medium (mutate × staging from the risk matrix).
No risk_tags because no catastrophic patterns found.

## Step 6: Agent executes kubectl apply

The agent runs the real command in its environment.

## Step 7: Agent calls report()

The agent returns:

- prescription_id: the ID from prescribe
- exit_code: 0
- artifact_digest (optional): for drift detection
- actor (optional): falls back to prescribe actor if omitted

Report entry:

```json
{
  "entry_id": "01JD7KZ1A3...",
  "previous_hash": "sha256:...",
  "hash": "sha256:...",
  "signature": "base64-ed25519-signature",
  "type": "report",
  "session_id": "session-20260304-staging",
  "trace_id": "01JD7KZ1A2...",
  "span_id": "span-report-001",
  "parent_span_id": "span-prescribe-001",
  "actor": {"type":"ai_agent","id":"claude-code","provenance":"mcp","instance_id":"pod-abc123","version":"v1.3"},
  "timestamp": "2026-03-04T10:12:30Z",
  "artifact_digest": "sha256:...",
  "payload": {
    "report_id": "01JD7KZ1A3...",
    "prescription_id": "01JD7KX9M2...",
    "exit_code": 0,
    "verdict": "success"
  },
  "scope_dimensions": {"cluster":"staging-us-east","namespace":"staging"},
  "spec_version": "0.3.1",
  "adapter_version": "0.3.1"
}
```

## Step 8: Signal detection at scorecard time

Signals are computed **batch** at `evidra scorecard` time, not at
report() time. The scorecard reads the full evidence chain and
evaluates all seven signal detectors across all entries:

- **Protocol Violation**: prescriptions without reports (TTL-based),
  reports without prescriptions, duplicate reports, cross-actor reports
- **Artifact Drift**: report.artifact_digest ≠ prescribe.artifact_digest
- **Retry Loop**: same (actor, intent_digest, shape_hash) repeated N
  times after failure within 30-minute window
- **Blast Radius**: destroy operations with resource_count > 5
- **New Scope**: first time an actor operates in a given tool+scope combination
- **Repair Loop**: repeated fix attempts after failures on the same resource
- **Thrashing**: rapid create/delete cycles on the same resource

In this example, all signals are zero:

```
Protocol Violation: 0
Artifact Drift: 0
Retry Loop: 0
Blast Radius: 0
New Scope: 0
Repair Loop: 0
Thrashing: 0
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

```bash
PRESCRIBE_OUT=$(evidra prescribe --tool terraform --operation apply \
  --artifact plan.json --environment production \
  --evidence-dir /tmp/evidra --actor ci-pipeline-123)
PRESCRIPTION_ID=$(echo "$PRESCRIBE_OUT" | jq -r '.prescription_id')
```

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
  "operation_class": "mutate",
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
it is recorded in `risk_tags` AND elevates `risk_level` above the
matrix value. Evidra remains an inspector — no blocking. The risk
is visible in the prescription and in the scorecard.

Prescription returned:

```json
{
  "ok": true,
  "prescription_id": "01JD8ABC12...",
  "risk_level": "critical",
  "risk_tags": ["world-open ingress"],
  "artifact_digest": "sha256:...",
  "intent_digest": "sha256:...",
  "canon_version": "tf/v1"
}
```

risk_level = critical because risk_tags elevated it above the matrix
value (mutate × production = high from matrix, but catastrophic
detector found open ingress → critical).

## Step 3: CI executes terraform apply and reports

After apply, CI captures the exit code and reports using the
prescription_id from step 2:

```bash
terraform apply -auto-approve plan.out
EXIT_CODE=$?

evidra report --prescription "$PRESCRIPTION_ID" --exit-code "$EXIT_CODE" \
  --evidence-dir /tmp/evidra --actor ci-pipeline-123
```

Report entry written to evidence:

```json
{
  "entry_id": "01JD8ABD34...",
  "type": "report",
  "actor": {"type":"cli","id":"ci-pipeline-123","provenance":"cli"},
  "payload": {
    "report_id": "01JD8ABD34...",
    "prescription_id": "01JD8ABC12...",
    "exit_code": 1,
    "verdict": "failure"
  }
}
```

At scorecard time, this may produce signals such as:
- Retry Loop (if CI re-runs the same intent repeatedly after failure)
- Protocol Violation (if report missing past TTL)
- Artifact Drift (if report digest differs from prescribed digest)

---

# Part 3 — Scorecard Output

A scorecard is computed over a selected window (e.g., last 30 days)
for each actor.

```bash
evidra scorecard --actor claude-code --period 30d --evidence-dir /tmp/evidra
```

Output (JSON):

```json
{
  "score": 99.97,
  "band": "excellent",
  "total_operations": 4217,
  "signals": {
    "protocol_violation": {"count": 2, "rate": 0.0005},
    "artifact_drift": {"count": 1, "rate": 0.0002},
    "retry_loop": {"count": 3, "rate": 0.0007},
    "blast_radius": {"count": 0, "rate": 0},
    "new_scope": {"count": 0, "rate": 0}
  },
  "actor_id": "claude-code",
  "period": "30d",
  "scoring_version": "0.3.1",
  "spec_version": "0.3.1",
  "generated_at": "2026-03-04T12:00:00Z"
}
```

Use `evidra explain` for signal detail:

```bash
evidra explain --actor claude-code --period 30d --evidence-dir /tmp/evidra
```

Use `evidra compare` for cross-actor comparison:

```bash
evidra compare --actors claude-code,ci-pipeline --period 30d \
  --evidence-dir /tmp/evidra
```

### Scope-Aware Comparison

Comparing agents that do different work is misleading. Evidra computes
workload overlap (Jaccard similarity of tool×scope profiles):

```json
{
  "actors": [
    {"actor_id":"claude-code","score":99.97,"band":"excellent","total_operations":4217,
     "workload_profile":{"tools":{"kubectl":true},"scopes":{"staging":true,"production":true}}},
    {"actor_id":"ci-pipeline","score":99.85,"band":"excellent","total_operations":326,
     "workload_profile":{"tools":{"terraform":true},"scopes":{"production":true}}}
  ],
  "workload_overlap": 0.0,
  "generated_at": "2026-03-04T12:00:00Z"
}
```

Low overlap (0%) means comparison is not meaningful — agents operate
on different tools and scopes. Filter by shared dimensions for fair
comparison using `--tool` and `--scope` flags.

Version comparison (same agent, different versions) uses
`actor.version` (protocol v1.0) for variant tracking.

---

# Failure Cases (How the system behaves)

## Agent crashes before report
- Prescription exists, no report within TTL (default 5 min)
- Protocol Violation signal increments at scorecard time
- Sub-signal: **crash_before_report** (detected when the agent's last
  report had a non-zero exit code)

## Agent hangs during execution
- Prescription exists, no report within TTL, no further activity
- Protocol Violation signal increments at scorecard time
- Sub-signal: **stalled_operation** (no crash indicator — agent
  simply stopped working)
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
- Retry Loop signal fires (default: 3 attempts in 30 minutes)
- Important: if shape_hash differs (agent changed the artifact
  between attempts), it's NOT a retry loop — it's iteration.

## Agent acts without calling prescribe
- Report received with no matching prescription
- Protocol Violation signal increments (unprescribed action)

## Adapter cannot parse artifact
- canonical_action cannot be produced
- A `canonicalization_failure` evidence entry is written with error
  details, raw artifact digest, and adapter name
- No prescription is issued; the CLI returns an error
- The parse failure is recorded in evidence for auditability but
  does not affect the reliability score (not the agent's behavioral
  fault)

## Two agents, same cluster, different scores
- Same infrastructure, same time period, different workload profiles
- Evidra produces per-agent scorecards
- Cross-agent comparison only meaningful when workload overlaps
  (same tool + same scope)

---

# Appendix: Other Adapters (Spec Reserved)

### Helm

Helm template output is Kubernetes YAML. The K8s adapter handles it.

```
helm template my-chart -f values.yaml | evidra prescribe --tool helm --operation upgrade --artifact -
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

### Pre-Canonicalized Integration (Pulumi, Ansible, etc.)

Tools that already know their resource identity can bypass
the adapter entirely by providing a pre-built canonical action:

**MCP server:** Pass `canonical_action` field in prescribe input.

**CLI (v0.3.x+):** Use `--canonical-action` flag:

```bash
evidra prescribe \
  --tool pulumi \
  --operation update \
  --artifact state.json \
  --canonical-action '{
    "resource_identity": [
      {"type": "aws:ec2:Instance", "name": "web-server", "actions": ["update"]},
      {"type": "aws:rds:Instance", "name": "main-db", "actions": ["update"]}
    ],
    "resource_count": 2,
    "operation_class": "mutate",
    "scope_class": "production"
  }'
```

Evidra still computes artifact_digest from raw state.json,
runs risk detectors on the raw content, writes evidence,
and evaluates signals. The only difference: resource_identity
comes from the tool, not from an Evidra adapter. Entries are
marked with `canon_source=external` so scorecards can show
what percentage of data is self-reported.

---

End of worked example.
