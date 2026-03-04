# Evidra — Inspector Model Architecture

## Status
Architecture proposal. Replaces Engine v4 execution-binding model.

## One-liner
Evidra issues signed prescriptions for AI agent actions and maintains
a tamper-evident protocol of what was prescribed vs what the agent
reported doing.

Evidra does not block. Does not execute. Does not verify.
Admission controllers, Gatekeeper, Sentinel — they enforce.
Evidra **prescribes and protocols**.

---

## 1. Why This Model

### What Evidra is NOT

- Not an admission controller (Gatekeeper, Kyverno do this)
- Not an execution proxy (Spacelift, Terraform Cloud do this)
- Not a runtime scanner (Trivy, Falco do this)
- Not a post-execution verifier (Config Connector, AWS Config do this)

All of these exist. Competing with them is a losing strategy.

### What Evidra IS

An **independent inspector** for AI agent infrastructure operations.

The customer is not the ops team. The customer is **the team building
the agent** — Anthropic (Claude Code), OpenAI (Codex), Cognition
(Devin), or any company building internal AI DevOps agents.

Their problem: "How do I prove my agent behaves correctly?"

Their current answer: "Trust me." That's not good enough for
enterprises evaluating whether to allow AI agents on production
infrastructure.

Evidra gives them: "Here's a cryptographically signed, tamper-evident
protocol of every prescription issued and every action the agent
reported. If the agent deviated, here's the evidence."

### The accountability chain

```
Agent wants to act
       ↓
Agent asks Evidra: "I want to do X"
       ↓
Evidra evaluates policy, issues signed PRESCRIPTION:
  "You may do X, under these constraints: ..."
       ↓
Agent acts (with its own tools, its own credentials)
       ↓
Agent reports back to Evidra: "I did Y"
       ↓
Evidra records REPORT, compares to PRESCRIPTION
       ↓
Evidence chain entry:
  prescription_id → prescription_digest → report_digest → match/deviation
```

If the agent lies in its report — that's the agent developer's
problem to fix. Evidra recorded what it prescribed. The deviation
between prescription and report is the bug report.

If the agent doesn't report back — that's recorded too. A
prescription without a corresponding report is a protocol violation.

---

## 2. Core Concepts

### 2.1 Prescription

A prescription is a signed statement from Evidra:

> "Given your stated intent and the current policy, you may proceed
> under these constraints."

Prescription contents:

```yaml
prescription:
  id: "prs-01JXXXXXX"
  timestamp: "2026-03-04T14:00:00Z"
  
  # What was requested
  intent:
    tool: kubectl
    operation: apply
    target:
      namespace: staging
      resource: deployment
      name: api-server
  
  # What raw artifact was evaluated (digest only)
  artifact_digest: "sha256:abc123..."
  
  # Policy evaluation result
  decision:
    allow: true
    risk_level: low
    policy_bundle: "ops-v0.1 rev:a1b2c3"
    rules_evaluated: 24
    rules_triggered: 0
  
  # Constraints the agent must follow
  constraints:
    - "namespace must be staging"
    - "no privileged containers"
    - "images must match registry.corp.com/*"
  
  # Cryptographic binding
  signature: "<ed25519-over-canonical-prescription>"
  signing_key_id: "evidra-prod-2026"
```

Key properties:
- **Signed.** Prescription cannot be forged or modified.
- **Constraint-bearing.** Not just allow/deny — states conditions.
- **Artifact-bound.** The digest ties the prescription to the exact
  manifest/plan that was evaluated.
- **Stateless.** Evidra issues it and forgets. No server-side storage.

### 2.2 Report

After acting, the agent reports back what it did:

```yaml
report:
  prescription_id: "prs-01JXXXXXX"
  timestamp: "2026-03-04T14:00:12Z"
  
  # What the agent says it did
  action_taken:
    tool: kubectl
    operation: apply
    exit_code: 0
    artifact_digest: "sha256:abc123..."  # should match prescription
  
  # Agent self-attestation
  actor:
    type: agent
    id: "claude-code-v3.5"
    origin: mcp
```

Key properties:
- **Self-reported.** Evidra trusts the agent's report at face value.
  This is by design — Evidra is not an enforcer.
- **Prescription-linked.** References the prescription it's fulfilling.
- **Digest-carrying.** The agent reports the artifact digest it
  actually used. If different from prescription → deviation.

### 2.3 Protocol Entry

Evidra combines prescription + report into a protocol entry:

```yaml
protocol_entry:
  id: "evt-01JXXXXXX"
  prescription_id: "prs-01JXXXXXX"
  report_id: "rpt-01JXXXXXX"
  
  # Automated comparison
  verdict:
    status: "compliant"        # compliant | deviation | unreported
    deviations: []             # empty if compliant
  
  # Hash chain
  previous_hash: "sha256:..."
  entry_hash: "sha256:..."
  
  # Signed
  signature: "<ed25519>"
```

Possible verdicts:

| Verdict | Meaning |
|---------|---------|
| `compliant` | Report matches prescription. Artifact digests match. |
| `deviation` | Report differs from prescription. Details in `deviations[]`. |
| `unreported` | Prescription issued, no report received within TTL. |
| `unprescribed` | Report received with no matching prescription. |

Each of these is a **data point for the agent developer**, not a
security alert. Evidra doesn't remediate. It records.

---

## 3. What Evidra Checks In Prescriptions

Evidra's policy engine evaluates the raw artifact (manifest, plan)
and produces constraints. These are the same OPA rules from v2/v3:

| Domain | Example constraints in prescription |
|--------|-------------------------------------|
| Kubernetes | "no privileged containers", "namespace must not be kube-system", "no hostPath mounts" |
| Terraform | "destroy_count must be 0 for production", "no 0.0.0.0/0 security groups", "no wildcard IAM" |
| Helm | "namespace must match release namespace", "values must not override security context" |
| ArgoCD | "no auto-sync to production", "no wildcard destinations" |

The prescription carries these as human-readable constraint strings
AND as machine-readable rule_ids. The agent developer can use either.

On `deny`: prescription is still issued, but with `allow: false` and
the list of violated constraints. The agent developer decides how
their agent should handle denials.

---

## 4. MCP Interface

### 4.1 Tools

Two MCP tools:

**prescribe** — agent asks for a prescription before acting.

```
Tool: prescribe
Input:
  tool: "kubectl"
  operation: "apply"
  raw_artifact: "<full manifest>"
  actor: { type: "agent", id: "claude-code", origin: "mcp" }
  environment: "production"

Output:
  prescription_id: "prs-..."
  allow: true
  risk_level: "low"
  constraints: [...]
  rule_ids: [...]
  hints: [...]           # same as current v2 hints
  artifact_digest: "sha256:..."
  signature: "..."
```

**report** — agent reports what it did after acting.

```
Tool: report
Input:
  prescription_id: "prs-..."
  action_taken:
    tool: "kubectl"
    operation: "apply"
    exit_code: 0
    artifact_digest: "sha256:..."    # digest of what was actually applied

Output:
  protocol_entry_id: "evt-..."
  verdict: "compliant"
  deviations: []
```

### 4.2 Backward Compatibility

The existing `validate` tool becomes an alias for `prescribe`.
Same input, same output format (with additional prescription fields).
Zero breaking changes.

`report` is new. Agents that don't call `report` get protocol
entries with verdict `unreported`. This is fine — it's the current
v2/v3 behavior. The evidence chain records prescriptions without
reports.

### 4.3 Agent Contract

The agent contract (MCP instructions + resources) is updated:

```
1. Before any destructive operation: call `prescribe`.
2. If prescription says deny: STOP. Show constraints to user.
3. If prescription says allow: execute the operation.
4. After execution: call `report` with the prescription_id 
   and the artifact digest you actually used.
5. If you cannot report (crash, timeout): that's OK. 
   The missing report is itself recorded.
```

The contract is advisory. The agent may violate it. That's the point —
violations are the signal that agent developers use to improve their
agents.

---

## 5. Evidence Chain

### 5.1 Structure

Each protocol entry is appended to the hash-linked evidence chain
(same as current v2 model, extended):

```
Entry N:
  type: "prescription"
  prescription: { ... }
  previous_hash: hash(Entry N-1)
  hash: hash(this entry)

Entry N+1:
  type: "report"  
  report: { ... }
  prescription_ref: "prs-..."
  verdict: "compliant"
  previous_hash: hash(Entry N)
  hash: hash(this entry)
```

### 5.2 Queries

Evidence chain supports queries that agent developers care about:

```bash
# All deviations in the last 7 days
evidra evidence violations --since 7d

# All unreported prescriptions (agent didn't call report)
evidra evidence unreported --since 24h

# All prescriptions for a specific agent
evidra evidence list --actor claude-code --since 30d

# Compliance summary: compliant/deviation/unreported counts
evidra evidence summary --since 30d
```

### 5.3 Export

For agent developers who want to analyze patterns:

```bash
# JSONL export for data pipeline
evidra evidence export --format jsonl --since 90d

# Deviation-only export (bug reports for agent team)
evidra evidence export --format jsonl --verdict deviation
```

---

## 6. Who Cares About This Data

### Agent developers (primary customer)

"My agent got 3 deviations this week — it's reporting artifact X
but prescriptions were issued for artifact Y. The agent is
regenerating the manifest between prescribe and apply. Bug filed."

"My agent has 12 unreported prescriptions. It's crashing between
prescribe and report. Need to add error recovery."

"My agent has 0 deviations across 2000 operations this month.
Here's the evidence for the enterprise customer's security review."

### Enterprise security teams (secondary customer)

"We're evaluating Claude Code for production use. Show us the
protocol for the last 90 days of the pilot. How many deviations?
How many unreported? What was the deviation pattern?"

This is the conversation that gets AI agents approved for production
in regulated companies.

### Compliance auditors (tertiary customer)

"Show me evidence that every production change by an AI agent was
prescribed and reported. Show me the signed prescription and the
signed protocol entry. I can verify signatures independently."

---

## 7. What Evidra Does NOT Do

| Concern | Who handles it | Why not Evidra |
|---------|---------------|----------------|
| Block bad deployments at API level | Gatekeeper, Kyverno | They sit in the API path. Evidra doesn't. |
| Verify agent actually did what it reported | Kubernetes audit log, CloudTrail | They have API-level visibility. Evidra doesn't. |
| Remediate violations | Agent framework, gitops controller | Evidra has no write access. By design. |
| Enforce prescription compliance | Nobody — it's advisory | Agent developers fix their agents. |
| Scan running workloads | Falco, Trivy | Runtime security is a different domain. |

Evidra's value is precisely that it does NONE of these things.
It's an independent third party. It has no power to block or fix.
Its only power is **the signed record**.

This is what makes it credible as evidence. An enforcer's logs are
self-serving ("I blocked it, trust me"). An independent inspector's
protocol is not.

---

## 8. Architecture

### 8.1 Components

```
                    ┌──────────────────┐
                    │   AI Agent       │
                    │ (Claude Code,    │
                    │  Codex, Devin)   │
                    └──────┬───────────┘
                           │ MCP (stdio)
                    ┌──────▼───────────┐
                    │  Evidra MCP      │
                    │  Server          │
                    │                  │
                    │  prescribe tool  │
                    │  report tool     │
                    │  get_event tool  │
                    └──────┬───────────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
      ┌──────────┐  ┌───────────┐  ┌──────────┐
      │ Domain   │  │ OPA       │  │ Evidence  │
      │ Adapters │  │ Policy    │  │ Store     │
      │          │  │ Engine    │  │           │
      │ kubectl  │  │ ops-v0.1  │  │ JSONL     │
      │ terraform│  │ baseline  │  │ hash-link │
      │ helm     │  │ (embedded)│  │ Ed25519   │
      │ argocd   │  │           │  │           │
      └──────────┘  └───────────┘  └──────────┘
```

### 8.2 Privilege Model

Evidra has **zero infrastructure privileges**.

- No kubeconfig needed
- No AWS credentials needed  
- No terraform state access
- No cluster access
- No cloud API access

Evidra reads the artifact the agent sends, evaluates policy, writes
local evidence. That's it. The entire attack surface is:

- MCP stdio (local process communication)
- Local filesystem (~/.evidra/evidence)
- Signing key (in memory, from env var)

This is orders of magnitude smaller than any proxy or enforcement
model.

### 8.3 Online Mode (API)

Same model, HTTP transport:

```
POST /v1/prescribe   →  prescription (signed)
POST /v1/report      →  protocol entry (signed)
GET  /v1/evidence/*  →  evidence queries
```

API server is stateless. Prescriptions and protocol entries are
returned to the caller. Server signs but does not store.

---

## 9. Prescription vs Current Validate

The prescription model is a superset of the current validate response:

| Field | validate (v2) | prescribe (v4) |
|-------|---------------|----------------|
| allow | ✓ | ✓ |
| risk_level | ✓ | ✓ |
| rule_ids | ✓ | ✓ |
| hints | ✓ | ✓ |
| reasons | ✓ | ✓ |
| prescription_id | ✗ | ✓ |
| constraints (human-readable) | ✗ | ✓ |
| artifact_digest | ✗ | ✓ |
| signature | ✗ (evidence only) | ✓ (on prescription itself) |

The prescription is the validate response + a signed commitment
that can be independently verified and linked to a report.

---

## 10. Deviation Detection

Evidra compares prescription and report on these dimensions:

| Check | How | Deviation type |
|-------|-----|----------------|
| Artifact changed | artifact_digest mismatch | `artifact_modified` |
| Tool changed | tool field mismatch | `tool_changed` |
| Operation changed | operation field mismatch | `operation_changed` |
| Unprescribed action | report with no prescription | `unprescribed` |
| Missing report | prescription with no report within TTL | `unreported` |

Evidra does NOT check whether the agent actually executed the
artifact on the real cluster. That's admission controller territory.
Evidra checks **internal consistency of what the agent told it**.

If the agent says "I applied manifest X" and the prescription was
for manifest X — verdict is `compliant`. Whether the agent lied is
not Evidra's problem. The admission controller will catch the actual
API call. Evidra catches the agent's inconsistency in its own
reporting.

---

## 11. Value Proposition For Agent Frameworks

### For Anthropic (Claude Code)

"Claude Code integrates with Evidra. Every infrastructure operation
is prescribed and protocolled. Enterprises can audit Claude Code's
behavior against signed evidence. Here's our deviation rate: 0.01%
across 100,000 operations."

### For OpenAI (Codex)

Same story. Evidra is agent-agnostic. Any MCP-compatible agent can
integrate.

### For enterprises evaluating AI agents

"Before approving Agent X for production, we ran a 90-day pilot
with Evidra. Protocol shows 3 deviations (all artifact_modified,
all in the first week — agent bug was fixed). Zero deviations in
the last 60 days. Zero unreported prescriptions. Approved."

### For regulated industries

"Every AI agent operation on our infrastructure is independently
prescribed and protocolled by Evidra. Prescriptions are signed
with Ed25519. Evidence chain is hash-linked and tamper-evident.
Auditor can verify offline with the public key."

---

## 12. Relationship to Previous Engine Versions

| Engine | What it solved |
|--------|---------------|
| v2 | Deterministic policy evaluation, fail-closed, evidence chain |
| v3 | Input trust (raw artifacts, domain adapters) |
| v4 (execution binding, superseded) | Execution trust via capability tokens |
| v4 (inspector model, this document) | Accountability via prescriptions and protocol |

The inspector model supersedes the execution-binding model because:

1. Zero privilege increase (no infra credentials needed)
2. No execution path complexity (no proxy, no wrapper, no crash recovery)
3. Clear market positioning (independent inspector, not another enforcer)
4. Complementary to enforcers (Gatekeeper + Evidra, not Gatekeeper vs Evidra)
5. Agent developer as customer (not ops team — different buyer, less competition)

---

## 13. Implementation Plan

### v0.3.0 — Foundation (Engine v3)
- Domain adapters
- Raw artifact input
- k8s.io/apimachinery for manifest parsing
- Current validate tool unchanged

### v0.4.0 — Inspector Model
- `prescribe` MCP tool (validate + prescription issuance)
- `report` MCP tool
- Prescription signing (Ed25519, same key as evidence)
- Deviation detection (artifact_digest comparison)
- Protocol entry in evidence chain (prescription + report + verdict)
- `validate` becomes alias for `prescribe` (backward compat)
- `evidra evidence violations` CLI command
- `evidra evidence unreported` CLI command
- Agent contract v2 with prescribe/report flow

### v0.4.x — Enrichment
- Constraint generation from OPA rules (human-readable)
- Compliance mapping (rule_id → CIS/PCI DSS/SOC2 ref)
- Evidence export (JSONL, filtered by verdict/actor/date)
- Dashboard in UI (deviation rate, unreported rate, top violations)

### v0.5.0 — Enterprise
- OIDC-verified actor.id
- Approval workflows (signed approval tokens in prescriptions)
- Evidence query API (for SIEM/audit tool integration)
- Multi-agent protocol (prescription issued to Agent A,
  report from Agent B → deviation)

---

## 14. Do NOT

- Do not block execution. Evidra is not an enforcer.
- Do not verify execution against live infrastructure. That's
  admission controller / audit log territory.
- Do not store infrastructure credentials. Zero-privilege.
- Do not compete with Gatekeeper/Kyverno/Sentinel. Complement them.
- Do not target ops teams as primary buyer. Target agent developers.
- Do not add execution capability. The moment Evidra executes,
  it's no longer an independent inspector.
- Do not make report mandatory in the protocol. Missing reports
  are a signal, not an error.
