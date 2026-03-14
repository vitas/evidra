# Evidra Core Data Model

- Status: Normative
- Version: v1.0
- Canonical for: public object schemas and field semantics
- Audience: public

## Purpose

Precise schema for the objects that appear in the architecture:

- CanonicalAction
- Prescription
- Report
- ValidatorFinding
- EvidenceEntry
- Signal
- Scorecard

The goal is to make the architecture deterministic, replayable,
and stable during the full codebase refactor.

---

## 1. CanonicalAction

CanonicalAction is the normalized representation of an
infrastructure action. Produced by canonicalization adapters
(server-side) or by self-aware tools (pre-canonicalized path).

| Field | Type | Description |
|-------|------|-------------|
| tool | string | Tool identifier (kubectl, terraform, helm, ...) |
| operation_class | string | mutate, destroy, read, plan |
| scope_class | string | production, staging, development, unknown |
| resource_identity | []ResourceID | Normalized resource identifiers |
| resource_count | integer | Number of resources affected |
| resource_shape_hash | string | SHA256 of normalized spec (for retry detection) |

### ResourceID

| Field | Type | Description |
|-------|------|-------------|
| api_version | string | K8s: e.g. "apps/v1" |
| kind | string | K8s: e.g. "Deployment" |
| namespace | string | K8s: e.g. "prod" |
| name | string | Resource name |
| type | string | Terraform: e.g. "aws_s3_bucket" |
| actions | string | Terraform: e.g. "create", "update" |

Fields are tool-specific. K8s uses api_version/kind/namespace/name.
Terraform uses type/name/actions.

### Digest Rules

```
intent_digest  = SHA256(canonical_json(canonical_action))
artifact_digest = SHA256(raw_artifact_bytes)
```

- `intent_digest` identifies behavioral identity.
- `artifact_digest` ensures artifact integrity.
- Same artifact_digest → same intent_digest. Not the reverse.
- They MUST NOT be treated as interchangeable.

---

## 2. Prescription

Prescription records intent before execution.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| prescription_id | ULID | MUST | Globally unique identifier |
| tenant_id | string | MUST (service mode) | Empty in local mode |
| trace_id | string | MUST | Automation task/session correlation key |
| actor | Actor | MUST | Who is performing the action |
| canonical_action | CanonicalAction | MUST | Normalized action (contains tool) |
| intent_digest | string | MUST | SHA256 of canonical JSON |
| artifact_digest | string | MUST | SHA256 of raw artifact bytes |
| risk_inputs | []RiskInput | MUST | Per-source prescribe-time panel (`evidra/native` or `evidra/matrix`, plus external findings) |
| effective_risk | string | MUST | Highest-severity roll-up across `risk_inputs` |
| ttl_ms | integer | MUST | Time-to-live in milliseconds (materialized, not inferred) |
| canon_source | string | MUST | "adapter" (Evidra parsed) or "external" (tool self-reported) |
| timestamp | datetime | MUST | RFC 3339, UTC |

Legacy compatibility note:
- older evidence MAY still contain `risk_level`, `risk_tags`, or `risk_details`
- current producers SHOULD write `risk_inputs` + `effective_risk`

### Actor

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| type | string | MUST | ai_agent, ci, human, unknown |
| id | string | MUST | Stable identifier |
| provenance | string | MUST | mcp, cli, api, oidc, git, manual |
| instance_id | string | MAY | Runtime instance identifier (e.g. pod name, process ID) |
| version | string | MAY | Agent or tool version (e.g. "v1.3", "claude-sonnet-4-5") |

---

## 3. Report

Report records the terminal result after execution or an intentional refusal to execute.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| report_id | ULID | MUST | Globally unique identifier |
| prescription_id | ULID | MUST | Links to the prescription |
| trace_id | string | MUST | Same trace_id as prescription |
| actor | Actor | MUST | Who executed |
| exit_code | integer | MUST for success/failure/error | Tool exit code |
| artifact_digest | string | MAY | SHA256 of artifact at execution time (for drift detection) |
| verdict | string | MUST | success, failure, error, declined |
| decision_context | DecisionContext | MUST for declined | Structured refusal rationale |
| timestamp | datetime | MUST | RFC 3339, UTC |

### DecisionContext

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| trigger | string | MUST for declined | What caused the refusal |
| reason | string | MUST for declined | Short operational explanation (max 512 chars) |

### Matching Rules

1. prescription_id is globally unique (ULID).
2. First report wins. Second report for same prescription_id
   → `duplicate_report` protocol violation.
3. Report with unknown prescription_id → `unprescribed_action`
   protocol violation.
4. Cross-actor report (report.actor.id != prescription.actor.id)
   → `cross_actor_report` protocol violation.
5. Relationship is strictly 1:1. Batched apply (e.g. terraform
   apply with 10 resources) = one prescription with
   resource_count=10, one report.

### MCP Input Contract

The MCP tools `prescribe` and `report` accept caller-provided
input. Not all stored fields are caller-provided — many are
computed by Evidra. This table defines the wire contract.

#### prescribe tool input

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| tool | MUST | string | Infrastructure tool name (kubectl, terraform, helm, ...) |
| operation | MUST | string | Tool operation (apply, delete, plan, ...) |
| raw_artifact | MUST | string | Raw artifact content (YAML manifest, JSON plan, etc.) |
| actor | MUST | object | Actor identity (see Actor schema) |
| actor.type | MUST | string | ai_agent, ci, human, unknown |
| actor.id | MUST | string | Stable identifier for the actor |
| actor.origin | MUST | string | mcp, cli, api, oidc, git, manual |
| actor.instance_id | MAY | string | Runtime instance identifier |
| actor.version | MAY | string | Agent or tool version |
| actor.skill_version | MAY | string | Contract/prompt version used by the actor (for behavior slicing) |
| session_id | MAY | string | Run/session boundary identifier (auto-generated if omitted) |
| trace_id | MAY | string | Caller-provided correlation ID (defaults to `session_id` when omitted) |
| span_id | MAY | string | Span identifier for hierarchical tracing |
| parent_span_id | MAY | string | Parent span for multi-step agent workflows |
| scope_dimensions | MAY | object | Environment metadata map (cluster, namespace, account, region) |
| environment | MAY | string | Explicit environment label (overrides namespace-based scope resolution) |
| canonical_action | MAY | object | Pre-canonicalized action for self-aware tools (sets canon_source=external) |
| actor_meta | MAY | object | Comparison dimensions (agent_version, model_id, prompt_id) |

Wire/storage mapping:
- MCP/CLI wire field is `actor.origin`.
- Stored evidence field is `actor.provenance`.

Evidra computes and adds to the stored Prescription:
prescription_id, session_id (if not caller-provided), trace_id (defaults to session_id when not caller-provided), tenant_id,
canonical_action (if not pre-provided), intent_digest,
artifact_digest, risk_inputs, effective_risk, ttl_ms,
canon_source, timestamp.

#### report tool input

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| prescription_id | MUST | string | ID from prescribe response |
| verdict | MUST | string | success, failure, error, or declined |
| exit_code | MUST for success/failure/error | integer | Tool exit code (0 = success) |
| decision_context | MUST for declined | object | Refusal trigger and reason |
| artifact_digest | MAY | string | SHA256 of artifact actually applied (for drift detection) |
| actor | MAY | object | Optional override; omitted actor falls back to prescription actor |
| session_id | MAY | string | Run/session boundary (should match prescribe if same session) |
| operation_id | MAY | string | Operation identifier (inherits from prescription when omitted) |
| span_id | MAY | string | Span identifier for this report |
| parent_span_id | MAY | string | Parent span for multi-step workflows |

Evidra computes and adds to the stored Report:
report_id, trace_id (from corresponding prescription), actor
(from corresponding prescription when omitted), and timestamp.

If `artifact_digest` is omitted for executed outcomes, Evidra uses the prescription's
artifact_digest (no drift possible). If provided and different
from the prescription's artifact_digest, an `artifact_drift`
signal is recorded at scorecard time.

---

### Findings Are NOT on Report

Validator findings are independent evidence entries (type=finding),
linked to operations by `artifact_digest`. This decouples scanner
timing from the prescribe/report lifecycle. See §4.

---

## 4. ValidatorFinding

Normalized external scanner output. Written as independent
evidence entries (type=finding), NOT embedded in Report.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| tool | string | MUST | Scanner name (checkov, trivy, tfsec, ...) |
| tool_version | string | MAY | Scanner version |
| rule_id | string | MUST | Scanner rule identifier |
| severity | string | MUST | high, medium, low, info |
| resource | string | MUST | Affected resource identifier |
| message | string | MUST | Human-readable finding description |
| artifact_digest | string | MUST | Links finding to the operation's artifact |

Findings may arrive before, during, or after execution. The
linking key is `artifact_digest` — the same digest that appears
on the prescription and report for that operation.

---

## 5. EvidenceEntry

Append-only event log entry. Every JSONL line is one EvidenceEntry.
All entry types share the same envelope.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| entry_id | ULID | MUST | Globally unique, monotonically increasing per writer |
| previous_hash | string | MUST | Hash of previous entry (empty for first in segment) |
| hash | string | MUST | Hash of this entry (all fields except hash itself) |
| signature | string | MUST | Ed25519 signature of hash |
| type | string | MUST | Closed enum (see Entry Types) |
| tenant_id | string | MUST (service mode) | Empty in local mode |
| session_id | string | MUST | Run/session boundary identifier |
| trace_id | string | MUST | Automation task/session correlation key |
| span_id | string | MAY | Span identifier for hierarchical tracing |
| parent_span_id | string | MAY | Parent span for multi-step workflows |
| operation_id | string | MAY | Operation identifier, unique within session |
| attempt | integer | MAY | Retry attempt counter |
| actor | Actor | MUST (prescribe, report) | MAY be empty on signal, receipt |
| timestamp | datetime | MUST | RFC 3339, UTC |
| intent_digest | string | conditional | Present on prescription entries |
| artifact_digest | string | conditional | Present on prescription, report, finding entries |
| payload | object | MUST | Type-specific content |
| scope_dimensions | object | MAY | Environment metadata map (cluster, namespace, account, region) |
| spec_version | string | MUST | Signal spec version (e.g. `v1.1.0`) |
| canonical_version | string | MUST | Adapter canon version (e.g. "k8s/v1") |
| adapter_version | string | MUST | Evidra adapter version |
| scoring_version | string | MUST | Scoring model version (e.g. `v1.1.0`) |

### Entry Types

| Type | When written | Payload contains |
|------|-------------|-----------------|
| `prescribe` | prescribe() processes an artifact | Prescription fields |
| `report` | report() records execution outcome | Report fields |
| `finding` | Scanner output (independent, linked by artifact_digest) | ValidatorFinding fields |
| `signal` | Signal detector fires (at scorecard time) | signal_name, sub_signal, entry_refs, details |
| `receipt` | evidra-api acknowledges forwarded batch (v0.5.0+) | batch_id, entry_count, server_ts |
| `canonicalization_failure` | Adapter fails to parse artifact | error_code, error_message, adapter, raw_digest |
| `session_start` | Session begins | Labels |
| `session_end` | Session ends | Status |
| `annotation` | Human or system annotation | Key, value, message |

### Schema Rules

1. `type` is a closed enum. Adding a new type requires a spec
   version bump.
2. Timestamp is always UTC. Ordering is by entry position in
   chain, not by timestamp.
3. Entries are immutable. Corrections are new entries.
4. Hash chain creates append-only integrity. Insertion,
   reordering, or modification is detectable during verification.
5. Verification is possible offline.

---

## 6. Signal

Signals represent detected automation reliability behavior.
Signals are binary (detected or not) — they do not carry
per-signal confidence. Confidence is a scorecard-level property.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| signal_id | ULID | MUST | Globally unique |
| trace_id | string | MUST | Correlation key |
| name | string | MUST | Signal name (see Core Signals) |
| sub_signal | string | MAY | Sub-classification (e.g. stalled_operation, crash_before_report) |
| severity | string | MUST | Severity level |
| evidence_refs | []entry_id | MUST | Entry IDs that triggered the signal |
| details | object | MAY | Additional context |

### Core Signals

| Signal | What it detects |
|--------|----------------|
| protocol_violation | Missing report, missing prescribe, duplicate report, cross-actor report |
| artifact_drift | Agent changed artifact between prescribe and report |
| retry_loop | Same actor + same intent + same shape, repeated after failure within time window |
| blast_radius | Operation affects too many resources for its operation_class |
| new_scope | First operation in a (tool, operation_class, scope_class) tuple |
| risk_escalation | Risk level increased between consecutive operations in the same session |

---

## 7. Scorecard

Scorecard summarizes reliability over a dataset.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| actor_id | string | MUST | Actor being scored |
| period | string | MUST | Time period (e.g. "30d") |
| total_operations | integer | MUST | Operation count in period |
| score | float | MUST | 0-100, computed from penalty |
| band | string | MUST | excellent, good, fair, poor, insufficient_data |
| confidence | float | MUST | Scorecard-level confidence, embedded and computed by `Compute()` (see Confidence Model) |
| signals | SignalRates | MUST | Per-signal rates |
| top_signals | []string | MUST | Top contributing signals to penalty |
| evidence_refs | []entry_id | MUST | Supporting evidence entries |
| scoring_version | string | MUST | Scoring model version (e.g. `v1.1.0`) |
| spec_version | string | MUST | Signal spec version (e.g. `v1.1.0`) |
| canon_version | string | MUST | Canonicalization version |
| evidra_version | string | MUST | Evidra binary version |
| generated_at | datetime | MUST | When scorecard was computed |

### SignalRates

| Field | Type | Description |
|-------|------|-------------|
| protocol_violation_rate | float | violations / total_ops |
| drift_rate | float | drifts / total_reports |
| retry_rate | float | retry_events / total_ops |
| blast_rate | float | blast_events / total_ops |
| scope_rate | float | scope_events / total_ops |
| escalation_rate | float | escalation_events / total_ops |

### Score Formula

For a plain-language explanation of how counts become rates, how rates become
penalty contributions, and how caps/ceilings affect the final score, see
[`EVIDRA_SCORING_MODEL_V1.md`](./EVIDRA_SCORING_MODEL_V1.md).

```
score = 100 * (1 - penalty)

penalty = weight(protocol_violation) * violation_rate
        + weight(artifact_drift) * drift_rate
        + weight(retry_loop) * retry_rate
        + weight(thrashing) * thrashing_rate
        + weight(blast_radius) * blast_rate
        + weight(risk_escalation) * escalation_rate
        + weight(new_scope) * scope_rate
        + weight(repair_loop) * repair_rate
```

The active default scoring profile is defined in
`docs/system-design/scoring/default.v1.1.0.md`.

| Band | Score | Meaning |
|------|-------|---------|
| Excellent | 99-100 | Production-ready |
| Good | 95-99 | Minor issues |
| Fair | 90-95 | Needs attention |
| Poor | <90 | Unreliable |

Minimum sample: 100 operations. Below that: band = "insufficient_data".

### Confidence Model

Confidence is a scorecard-level property, not per-signal.

```
confidence = f(evidence_completeness, canon_trust, actor_trust)
```

| Confidence | Score ceiling | Condition |
|------------|--------------|-----------|
| High | 100 (no cap) | Full evidence, adapter-canonicalized, verified identity |
| Medium | 95 | >50% canon_source=external, or unverified actor with no tenant_id |
| Low | 85 | >10% protocol_violation_rate, or severe evidence gaps |

---

## 8. Core Data Flow

```
prescribe(raw_artifact)
    │
    ▼
Prescription ──────► EvidenceEntry (type=prescribe)
    │
    │  agent executes
    ▼
report(prescription_id, verdict, exit_code?, decision_context?, artifact_digest)
    │
    ▼
Report ────────────► EvidenceEntry (type=report)

findings (async) ──► EvidenceEntry (type=finding)
                     linked by artifact_digest

signals (on demand) ► EvidenceEntry (type=signal)

scorecard (on demand)  computed from evidence entries
```

All signals and scores are derived strictly from the evidence log.

---

## 9. Frozen Enums

All enum values are closed sets. Adding a value requires a spec
version bump.

### operation_class

| Value | Meaning |
|-------|---------|
| `read` | Read-only operation (get, describe, list) |
| `mutate` | Create or update operation (apply, patch) |
| `destroy` | Delete operation (delete, destroy) |
| `plan` | Dry-run operation (plan, diff) |

### scope_class

| Value | Meaning | Resolution |
|-------|---------|------------|
| `production` | Production environment | Explicit `--env` flag, or namespace contains "prod" |
| `staging` | Staging environment | Namespace contains "stag" |
| `development` | Development environment | Namespace contains "dev" |
| `unknown` | Cannot determine | Default when no match |

Ingress alias normalization and validation requirements are defined in
[EVIDRA_PROTOCOL_V1.md §5.1](EVIDRA_PROTOCOL_V1.md#51-scope-class).

### effective_risk and risk_inputs

`effective_risk` uses the same severity vocabulary as previous `risk_level` fields:

| Value | Meaning |
|-------|---------|
| `low` | Routine operation |
| `medium` | Elevated risk, worth noting |
| `high` | Significant risk, agent should consider human approval |
| `critical` | Catastrophic risk pattern detected |

`risk_inputs` records why that roll-up was chosen. Current producers use:
- `evidra/native` when raw artifact bytes are available, including native detector tags
- `evidra/matrix` when only canonical context is available
- external findings sources when SARIF findings are attached at prescribe time

### entry_type

| Value | Written by |
|-------|-----------|
| `prescribe` | prescribe() call |
| `report` | report() call |
| `finding` | Scanner output (independent, linked by artifact_digest) |
| `signal` | Signal detector (at scorecard time) |
| `receipt` | evidra-api acknowledgment (v0.5.0+) |
| `canonicalization_failure` | Adapter parse failure |
| `session_start` | Session begins |
| `session_end` | Session ends |
| `annotation` | Human or system annotation |

### verdict (on report)

| Value | Meaning |
|-------|---------|
| `success` | Execution completed and exit_code == 0 |
| `failure` | Execution completed and exit_code != 0 |
| `error` | Execution path could not complete normally |
| `declined` | Execution intentionally not started after assessment |

### band (on scorecard)

| Value | Score range |
|-------|-----------|
| `excellent` | 99-100 |
| `good` | 95-99 |
| `fair` | 90-95 |
| `poor` | <90 |
| `insufficient_data` | <100 operations |

---

## 10. trace_id Generation Rules

| Context | trace_id lifecycle | Generation |
|---------|-------------------|------------|
| evidra-mcp prescribe | Session-scoped by default | Defaults to `session_id` if omitted |
| evidra CLI prescribe | Session-scoped by default | Defaults to `session_id` if omitted |
| report (CLI/MCP) | Derived from referenced prescribe when present | Inherited from prescription; generated only when no source exists |
| evidra-api (planned) | Service-defined | Generated server-side or accepted from trusted caller |

Rules:
1. trace_id SHOULD be ULID-formatted.
2. A single trace_id MAY span multiple prescribe/report pairs
   (e.g. multi-resource apply).
3. A trace_id MUST NOT span multiple actors.
4. A trace_id MUST NOT span multiple tenants.
5. If `trace_id` is not provided by the caller, Evidra MUST default it to `session_id`.

---

## 11. Invariants

1. Evidence is append-only.
2. Signals derive from evidence only.
3. Validators produce findings; Evidra produces signals.
4. Canonicalization defines intent identity.
5. Evidence replay MUST produce identical signals and scores.
6. Findings are independent entries, not embedded in reports.
7. Confidence is scorecard-level, not per-signal.
8. All digests use `sha256:` prefix format.
9. Enum values are closed sets (§9). New values require spec version bump.
10. intent_digest excludes resource_shape_hash (hashes identity fields only).
