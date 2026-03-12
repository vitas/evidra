# Evidra Session / Operation / Event Model v1
**Status:** Normative (v1)  
**Goal:** Define a cloud-native execution model compatible with **Kubernetes**, **OpenTelemetry**, and **CloudEvents**, while mapping cleanly to Evidra’s `prescribe/report/findings` concepts.

This model prevents protocol gaps in:
- session/run boundaries
- retries/deduplication
- per-run scoring and benchmarking
- multi-step agent workflows (LangChain/LangGraph/AutoGen/CrewAI)
- K8s controller reconciliation loops

---

## 1. Terminology

### 1.1 Session
A **Session** represents a single automation execution boundary (a “run” in the human sense).

Examples:
- one GitHub Actions workflow run
- one LangChain agent execution
- one CI pipeline execution
- one “reconciliation cycle” for a controller (optionally grouped)

**Session is the primary unit for scorecards and benchmark aggregation.**

### 1.2 Operation
An **Operation** is an atomic action within a Session that can succeed/fail and has a bounded scope.

Examples:
- `terraform.apply`
- `kubectl.apply`
- `helm.upgrade`
- `scanner.run` (optional modeling choice)
- “approve loan” (non-Ops domain)

Operations are the primary unit for:
- deduplication of retries (via operation_id + attempt)
- “intent → action → result” tracking
- safety/risk detectors

### 1.3 Event
An **Event** is a concrete fact emitted during execution.

Examples:
- tool started
- tool ended
- tool error
- validator findings received
- annotation (“human approved”)

Events are append-only evidence entries.

---

## 2. Normative Hierarchy

```
Session (session_id)
  └─ Operation (operation_id)
       └─ Events (event_id…)
```

**MUST rules**
- Every Event MUST belong to exactly one Session.
- Every `prescribe/report` Event MUST belong to exactly one Operation.
- A Session MAY contain many Operations.
- Operations MUST NOT cross Session boundaries.

---

## 3. Required IDs and Cardinality

### 3.1 IDs
| Field | Required | Notes |
|---|---:|---|
| session_id | MUST | Stable for the full run; ULID recommended |
| operation_id | MUST for operation events | Unique within a session; ULID or UUID |
| event_id | MUST | Globally unique; ULID/UUID |
| attempt | SHOULD | Integer retry counter per operation |
| trace_id | MUST | Correlation ID for related events |
| span_id | MAY | For OTel span hierarchy |

**Implementation status:** `operation_id` and `attempt` are fields on
`EvidenceEntry` (see [EVIDRA_CORE_DATA_MODEL_V1.md, §5](EVIDRA_CORE_DATA_MODEL_V1.md#5-evidenceentry)).
They are available via CLI (`--operation-id`, `--attempt`) and MCP server
input (`operation_id`, `attempt` in prescribe/report). Session lifecycle
entry types (`session_start`, `session_end`) and `annotation` are
implemented as internal entry types.

### 3.2 Stability rules
- `session_id` MUST NOT change during a run.
- `operation_id` MUST remain stable across retries of the same logical action.
- `attempt` increments on each retry (0 or 1-based; choose one and document).

---

## 4. Event Taxonomy v1 (Normative)

### 4.1 Session lifecycle
- `session.start`
- `session.end`

Implementation note (v0.3.x): explicit `session.start`/`session.end`
entries are OPTIONAL. Session boundaries are still required through
persisted `session_id` on all entries.

### 4.2 Operation lifecycle
- `operation.start`  (maps to prescribe)
- `operation.end`    (maps to report success)
- `operation.error`  (maps to report failure or error)

### 4.3 Findings and annotations
- `validator.findings`
- `annotation`

### 4.4 Optional (v1.1+)
- `operation.progress`
- `llm.start/end/error` (privacy-sensitive; disabled by default)

### 4.5 Canonical Agent Lifecycle (Recommended)

```
session.start (optional in v0.3.x)
  -> operation.start
  -> validator.findings (0..N)
  -> operation.end | operation.error
  -> report(verdict=declined) when execution is intentionally not started
session.end (optional in v0.3.x)
```

**MUST rules**
- `operation.end` and `operation.error` MUST close an `operation.start`.
- If an operation is abandoned, emit `operation.error` with `status=aborted`.

---

## 5. Core Event Fields (v1)

All events MUST contain:

```json
{
  "event_id": "evt_...",
  "session_id": "ses_...",
  "type": "operation.start",
  "time": "2026-03-05T12:00:00Z"
}
```

Operation-scoped events MUST also contain:

```json
{
  "operation_id": "op_...",
  "attempt": 0,
  "operation": {
    "name": "terraform.apply",
    "kind": "cli|api|k8s|scanner"
  }
}
```

Recommended common fields:

```json
{
  "actor": { "id": "ci-bot", "type": "automation" },
  "scope": {
    "class": "production",
    "dimensions": { "cluster": "prod-1", "namespace": "payments" }
  },
  "artifact": { "digest": "sha256:...", "type": "terraform-plan" }
}
```

---

## 6. Mapping to Evidra Concepts

### 6.1 `prescribe`
Maps to:
- `operation.start`

Carries:
- intent summary (redacted by default)
- tool/action name
- artifact digests and scope

### 6.2 `report`
Maps to:
- `operation.end` (success)
- `operation.error` (failure/aborted/error)

For v1 decision tracking, `report` also carries `verdict=declined` for an
operation that was intentionally not started after assessment. This remains the
same terminal report event rather than a new entry type.

Carries:
- verdict / status
- exit_code (if applicable)
- decision_context (if declined)
- output digests / side-effects summary (redacted by default)

### 6.3 Findings
Maps to:
- `validator.findings`

Findings MUST be attachable:
- before operation.start
- between start and end/error
- after end/error

All findings MUST include:
- `artifact.digest`
- `validator.tool` (name/version)
- severity/level
- timestamp

---

## 7. Delivery, Deduplication, Ordering

### 7.1 Delivery guarantee (v1)
- **At-least-once** delivery is normative.

### 7.2 Deduplication keys
Backends MUST deduplicate by:
- `(tenant_id, session_id, event_id)` OR `event_id` globally

Operations SHOULD be deduplicated by:
- `(session_id, operation_id, attempt, type)`

### 7.3 Ordering
- Sidecar/engine is the ordering authority within a session.
- Events MAY arrive out of order; the writer SHOULD store them in canonical order when possible.
- If ordering cannot be guaranteed, events MUST still be hash-chained in received order, and higher-level views must use timestamps + correlation to reconstruct.

---

## 8. OpenTelemetry Mapping (Recommended)

### 8.1 Trace/Span mapping
- `trace_id` SHOULD equal `session_id` (or a deterministic transform)
- A “run root span” SHOULD be created:
  - span name: `run`
  - attributes: `evidra.session_id`
- Each Operation SHOULD be a child span:
  - span name: `operation.name`
  - attributes: `evidra.operation_id`, `evidra.attempt`, `evidra.scope.class`

### 8.2 Span Events
Events such as findings can be represented as span events or log records with:
- `evidra.event_id`
- `evidra.type`
- `evidra.validator.tool`

**Privacy note:** prompts/tool outputs SHOULD NOT be exported by default.

---

## 9. CloudEvents Mapping (Recommended)

Use CloudEvents envelope for transport interoperability.

Mapping:
- CloudEvents `id` = `event_id`
- `type` = Evidra `type` (e.g., `evidra.operation.start`)
- `source` = stable actor identifier (e.g., `ci://github/actions`)
- `subject` = `session_id` (or `operation_id` for operation-scoped streams)
- `time` = `time`
- `data` = Evidra event payload

Example:
```json
{
  "specversion": "1.0",
  "type": "evidra.operation.start",
  "source": "ci://github/actions",
  "id": "evt_01J...",
  "subject": "ses_01J...",
  "time": "2026-03-05T12:00:00Z",
  "datacontenttype": "application/json",
  "data": {
    "session_id": "ses_01J...",
    "operation_id": "op_01J...",
    "attempt": 0,
    "operation": { "name": "terraform.apply", "kind": "cli" },
    "artifact": { "digest": "sha256:..." }
  }
}
```

---

## 10. Kubernetes-Native Integration (Recommended)

### 10.1 Labels/Annotations (v1)
When applying manifests or recording object changes, producers SHOULD attach:
- `evidra.io/session-id`
- `evidra.io/operation-id`
- `evidra.io/artifact-digest`

This provides a low-friction path that works without CRDs.

### 10.2 CRDs (v2+)
Optional future CRDs:
- `EvidraSession`
- `EvidraOperation`
- `EvidraScorecard`

CRDs MUST follow Kubernetes API conventions and avoid high-cardinality fields in status.

---

## 11. Benchmark Implications (v1)

- Per-case validation SHOULD validate signals and evidence integrity.
- Suite-level aggregation MAY treat the entire suite as one Session to satisfy MinOperations constraints for scoring.
- `session_id` is the natural boundary for benchmark results and reproducibility.

---

## 12. Minimal Examples (v1)

### 12.1 Terraform apply operation
1) `session.start`
2) `operation.start` (`terraform.plan`)
3) `operation.end`
4) `validator.findings` (Checkov SARIF)
5) `operation.start` (`terraform.apply`)
6) `operation.end`
7) `session.end`

### 12.2 K8s deploy with security violation
1) `session.start`
2) `operation.start` (`kubectl.apply`)
3) `validator.findings` (Kubescape SARIF)
4) `operation.end`
5) `session.end`

---

## 13. Summary (Normative)
To be cloud-native and community-aligned, Evidra v1 MUST standardize:
- Session as the primary run boundary
- Operation as the atomic unit of action
- Event taxonomy as above
- At-least-once delivery with dedup on event_id
- Optional OTel + CloudEvents mappings for interoperability
