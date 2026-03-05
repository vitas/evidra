# Evidra — CNCF Standards Alignment

## Status
**Non-normative.** Strategic alignment document. Defines how Evidra relates
to CNCF and industry standards. Does not change existing wire formats or
data models.

## Principle

Evidra is a **compatible layer**, not a parallel universe.

Where mature standards exist for events, tracing, findings, and supply chain,
Evidra aligns with them rather than inventing proprietary alternatives.

Evidra remains an **inspector** — it observes and measures. It does not
enforce policy, block deployments, or act as a gate.

---

## 1. Standards Alignment Matrix

| Layer | Standard | Evidra stance | Phase |
|-------|----------|---------------|-------|
| Events | CloudEvents | Compatible envelope mapping | 1 |
| Tracing | OpenTelemetry | Semantic attribute mapping | 1 |
| Scanner findings | SARIF | Ingestion format with lossy projection | 1 |
| Supply chain | in-toto | Export-compatible (evidence as predicate) | 1 |
| Policy | OPA | Downstream consumer only | 1 |
| Kubernetes | Labels / CRDs | Labels in v1, CRDs in v2+ | 1 |

Phase 1 = this document (alignment map, no code changes).
Phase 2 = compatibility adapters (`pkg/adapters/`).
Phase 3 = optional native adoption.

---

## 2. CloudEvents Compatibility

**Standard:** [CloudEvents v1.0](https://github.com/cloudevents/spec) (CNCF graduated)

**Alignment:** Evidra events can be represented as CloudEvents.
The mapping is defined in
[evidra-session-operation-event-model-v1.md, Section 9](evidra-session-operation-event-model-v1.md#9-cloudevents-mapping-recommended).

Summary of the mapping:

| CloudEvents field | Evidra source |
|-------------------|---------------|
| `specversion` | `"1.0"` |
| `id` | `event_id` |
| `type` | Evidra event type (e.g. `evidra.operation.start`) |
| `source` | Actor identifier (e.g. `ci://github/actions`) |
| `subject` | `session_id` |
| `time` | Event timestamp |
| `datacontenttype` | `application/json` |
| `data` | Evidra event payload |

### Evidra event type taxonomy (CloudEvents `type` field)

The `evidra.` prefix is used in CloudEvents `type` to namespace Evidra events.

The CloudEvents type taxonomy follows the session/operation event model.
It is a **separate namespace** from the internal entry type enum defined in
[EVIDRA_CORE_DATA_MODEL.md, §5 Entry Types](EVIDRA_CORE_DATA_MODEL.md#5-evidenceentry).

| CloudEvents `type` | Internal entry type | Notes |
|---------------------|---------------------|-------|
| `evidra.operation.start` | `prescribe` | Maps 1:1 |
| `evidra.operation.end` | `report` (verdict=success) | Maps 1:1 |
| `evidra.operation.error` | `report` (verdict=failure\|error) | Maps 1:1 |
| `evidra.validator.findings` | `finding` | Maps 1:1 |
| `evidra.annotation` | — | No internal entry type exists yet |
| `evidra.session.start` | — | No internal entry type exists yet |
| `evidra.session.end` | — | No internal entry type exists yet |

**Important:** `evidra.session.start`, `evidra.session.end`, and
`evidra.annotation` are defined in the CloudEvents taxonomy for external
interoperability, but do **not** have corresponding internal entry types
in the current implementation. They are future additions — the internal
entry type enum is a closed set that requires a spec version bump to extend
([EVIDRA_CORE_DATA_MODEL.md, §5 EvidenceEntry](EVIDRA_CORE_DATA_MODEL.md#5-evidenceentry)).

The mapping for `prescribe`, `report`, and `findings` is defined in
[evidra-session-operation-event-model-v1.md, Section 6](evidra-session-operation-event-model-v1.md#6-mapping-to-evidra-concepts).
The session lifecycle types (`evidra.session.start/end`) and `evidra.annotation`
are defined in the event taxonomy (§4) of that document but do not yet have
a §6-style mapping — they will be specified when the corresponding internal
entry types are added.

**Ambiguity note:** The session/operation event model informally lists
`evidra.operation.start|end|error|finding` as CloudEvents types. This
alignment document normalizes the canonical CloudEvents type for findings
to `evidra.validator.findings` (matching event taxonomy §4.3 of the
session/operation model).

### What Evidra does NOT do

- Evidra does not use CloudEvents as its internal storage format.
- Evidra does not require a CloudEvents-compatible event bus for operation.
- CloudEvents is an **export format**, not a transport dependency.

### Phase 2

`pkg/adapters/cloudevents/` — converts `EvidenceEntry` to/from CloudEvents
structured content mode.

---

## 3. OpenTelemetry Compatibility

**Standard:** [OpenTelemetry](https://opentelemetry.io/) (CNCF graduated)

**Alignment:** Evidra's correlation model maps to OpenTelemetry traces and spans.
The mapping is defined in
[evidra-session-operation-event-model-v1.md, Section 8](evidra-session-operation-event-model-v1.md#8-opentelemetry-mapping-recommended).

Summary of the mapping:

| OTel concept | Evidra concept |
|--------------|----------------|
| `trace_id` | `session_id` (or deterministic transform) |
| Root span | Session / run |
| Child span | Operation (`operation_id`) |
| Span event | Evidence entry (finding, annotation) |

### Semantic attributes

```
evidra.session_id
evidra.operation_id
evidra.attempt
evidra.scope.class
evidra.artifact.sha256
evidra.actor.id
evidra.actor.type
```

### Privacy

Prompts and tool outputs SHOULD NOT be exported to OTel by default.

### What Evidra does NOT do

- Evidra does not depend on an OTel collector for operation.
- Evidra does not emit spans natively. Export is opt-in.

### Phase 2

`pkg/adapters/otel/` — exports Evidra sessions as OTel traces with the
semantic attributes above.

---

## 4. SARIF Ingestion

**Standard:** [SARIF v2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)

**Alignment:** Evidra accepts SARIF as an ingestion format for scanner
findings. SARIF results are normalized into the internal `ValidatorFinding`
model. This mapping is intentionally lossy: only fields relevant to
operational risk signals are retained.

### Mapping table

| SARIF field | Evidra `ValidatorFinding` field |
|-------------|-------------------------------|
| `run.tool.driver.name` | `tool` |
| `run.tool.driver.version` | `tool_version` |
| `result.ruleId` | `rule_id` |
| `result.level` | `severity` (see severity mapping) |
| `result.message.text` | `message` |
| `result.locations[0].physicalLocation.artifactLocation.uri` | `resource` |

### Severity mapping

| SARIF `level` | Evidra `severity` |
|---------------|-------------------|
| `error` | `high` |
| `warning` | `medium` |
| `note` | `low` |
| `none` | `info` |

### Explicitly dropped fields

Evidra intentionally ignores the following SARIF structures:

- `codeFlows`
- `threadFlows`
- `stacks`
- `graphs`
- `fixes`
- `relatedLocations`
- `fingerprints` (Evidra uses `artifact_digest` for deduplication)

**Rationale:** These fields are primarily used for IDE debugging and static
analysis UX. They are not required for operational automation risk analysis.

### artifact_digest requirement

SARIF does not typically include an artifact digest. However, Evidra requires
`artifact_digest` to link findings to evidence entries.

Rule:

> If `artifact_digest` is not present in the SARIF document, it MUST be
> supplied by the caller or derived from the scanned artifact at ingestion
> time.

Without `artifact_digest`, findings cannot be correlated with operations
and will be rejected.

### Usage (Phase 2)

```
evidra ingest-findings --format sarif --file trivy.sarif --artifact-digest sha256:abcd...
```

### Phase 2

`pkg/adapters/sarif/` — parses SARIF v2.1.0, extracts results, normalizes
into `[]ValidatorFinding`.

---

## 5. in-toto Export Compatibility

**Standard:** [in-toto attestation framework](https://github.com/in-toto/attestation) (used by SLSA, Sigstore)

**Alignment:** Evidra evidence entries can be exported as in-toto attestations.
Evidra does not use in-toto internally.

### Current evidence model

Evidra's evidence chain already provides:

- Append-only entries
- Hash chaining (`previous_hash` linkage)
- Ed25519 signatures
- Canonicalized payloads

These properties make Evidra evidence suitable as an in-toto predicate payload
without any internal format changes.

**Note on signing:** The normative data model
([EVIDRA_CORE_DATA_MODEL.md, §5](EVIDRA_CORE_DATA_MODEL.md#5-evidenceentry))
specifies `signature` as a MUST field on every `EvidenceEntry`. The current
runtime implementation treats signing as opt-in (entries are unsigned when
no `Signer` is configured). This is a known conformance gap. For in-toto
export, signing MUST be enabled to produce verifiable attestations —
unsigned entries cannot satisfy supply chain verification requirements.

### Export mapping

```
in-toto attestation
├── subject:
│     - digest: sha256:<artifact_digest>
├── predicateType: "evidra.dev/operationEvidence/v1"
└── predicate:
      session_id: "ses_01J..."
      operation_id: "op_01J..."
      tool: "terraform.apply"
      evidence_hash: "sha256:..."
      risk_level: "high"
      signals: ["artifact_drift", "blast_radius"]
      scoring_version: "v1"
```

### Architecture

```
artifact
  |
  v
Evidra evidence chain (internal, unchanged)
  |
  v
in-toto attestation (export adapter)
  |
  v
cosign / Sigstore / SLSA verification
```

### What Evidra does NOT do

- Evidra does not replace its evidence chain with in-toto.
- Evidra does not depend on Sigstore or cosign for operation.
- in-toto is an **export format** for interoperability with supply chain tooling.

### Phase 2

`pkg/adapters/intoto/` — wraps Evidra evidence entries into in-toto
attestation envelopes.

---

## 6. Policy Engine Compatibility

**Evidra does not implement policy evaluation.**

Evidra is an inspector. It observes automation behavior and emits signals
and scorecards. It does not make pass/fail decisions, block deployments,
or enforce compliance rules.

Instead, Evidra signals and scorecards can be **consumed by external
policy engines**, compliance frameworks, or automation gates.

### Examples of downstream consumers

| Consumer | How it uses Evidra output |
|----------|--------------------------|
| OPA / Gatekeeper | Scorecard as OPA input for admission decisions |
| Terraform Cloud Sentinel | Signal rates as policy inputs |
| CI/CD gates | Score band check (e.g. fail pipeline if band = "poor") |
| Compliance dashboards | Signal rates and evidence refs for audit |
| SIEM | Evidence entries forwarded as security events |

### Example: OPA policy consuming Evidra scorecard

The scorecard JSON structure used below matches the current runtime output
(see `internal/score/score.go`). `rates` is a map keyed by signal name.
`confidence` is computed separately via `ComputeConfidence` in the current
runtime implementation (`internal/score/score.go`), though the normative
data model ([EVIDRA_CORE_DATA_MODEL.md, §7](EVIDRA_CORE_DATA_MODEL.md#7-scorecard))
specifies `confidence` as a MUST field on Scorecard. This gap exists because
the runtime has not yet been updated to embed confidence in the scorecard
JSON output.

**Known gap for external consumers:** The inputs to `ComputeConfidence`
(`externalPct`, `violationRate`) are not currently exposed in the scorecard
JSON. Until the runtime embeds confidence into the scorecard output (as
required by the normative model), external consumers such as OPA cannot
evaluate confidence. The OPA example below intentionally omits confidence
checks for this reason. When the runtime is updated, consumers should add:
`input.scorecard.confidence.level != "low"`.

```rego
package evidra.gate

default allow = false

allow {
    input.scorecard.band != "poor"
    input.scorecard.sufficient == true
    input.scorecard.rates.protocol_violation < 0.05
}
```

### What Evidra does NOT do

- Evidra does not embed OPA or any policy engine.
- Evidra does not evaluate Rego policies.
- Evidra does not provide a policy DSL.
- There is no `pkg/adapters/opa/` — policy integration is the consumer's
  responsibility.

---

## 7. Kubernetes Integration

**Alignment:** Evidra supports Kubernetes-native integration patterns.
The full mapping is defined in
[evidra-session-operation-event-model-v1.md, Section 10](evidra-session-operation-event-model-v1.md#10-kubernetes-native-integration-recommended).

### v1: Labels and Annotations

When recording Kubernetes operations, producers SHOULD attach:

```yaml
metadata:
  labels:
    evidra.io/session-id: "ses_01J..."
    evidra.io/operation-id: "op_01J..."
  annotations:
    evidra.io/artifact-digest: "sha256:..."
```

This provides integration without requiring CRDs or cluster-level components.

### v2+: Custom Resource Definitions (optional)

Future CRDs may include:

- `EvidraSession` — session lifecycle and metadata
- `EvidraOperation` — operation status and evidence refs
- `EvidraScorecard` — computed reliability scores

CRDs MUST follow Kubernetes API conventions and avoid high-cardinality
fields in status.

### What Evidra does NOT do

- Evidra does not require Kubernetes to operate.
- CRDs are optional and deferred to v2+.

---

## 8. Adoption Roadmap

### Phase 1 — Alignment (current)

This document. Answers "how does Evidra relate to each standard?" with
explicit mappings, boundaries, and field-level projections.

**No code changes.** No new dependencies. No wire format modifications.

### Phase 2 — Compatibility Layer

Add import/export adapters:

```
pkg/adapters/
  cloudevents/    EvidenceEntry <-> CloudEvent
  sarif/          SARIF -> []ValidatorFinding
  otel/           Session -> OTel trace
  intoto/         EvidenceEntry -> in-toto attestation
```

Each adapter is:

- A pure Go package with no side effects
- Independently testable
- Optional — Evidra operates without any adapter loaded

CLI additions:

```
evidra ingest-findings --format sarif --file <path> --artifact-digest <digest>
evidra export --format cloudevents --session <id>
evidra export --format intoto --session <id>
```

### Phase 3 — Native Adoption (optional)

Only if real adoption demand requires it:

- Native CloudEvents transport (emit directly to event bus)
- Native OTel exporter (emit spans without adapter)
- Native in-toto attestation signing

Phase 3 may never be needed. The compatibility layer (Phase 2) may be
sufficient for all practical integrations.

---

## 9. Positioning

With this alignment, Evidra can be described as:

> Evidra is the standard signal and metrics layer for infrastructure
> automation — built on CloudEvents, SARIF, in-toto, and OpenTelemetry
> for cloud-native interoperability.

This positions Evidra as a natural part of the CNCF ecosystem rather than
a standalone tool.

---

## References

- [EVIDRA_PROTOCOL.md](EVIDRA_PROTOCOL.md) — integration contract
- [EVIDRA_CORE_DATA_MODEL.md](EVIDRA_CORE_DATA_MODEL.md) — normative data model
- [evidra-session-operation-event-model-v1.md](evidra-session-operation-event-model-v1.md) — session/operation model with CloudEvents, OTel, K8s mappings
- [EVIDRA_SIGNAL_SPEC.md](EVIDRA_SIGNAL_SPEC.md) — signal definitions
- [CloudEvents v1.0 spec](https://github.com/cloudevents/spec)
- [SARIF v2.1.0 spec](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
- [in-toto attestation framework](https://github.com/in-toto/attestation)
- [OpenTelemetry specification](https://opentelemetry.io/docs/specs/)
