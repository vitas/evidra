# Evidra Protocol

- Status: Normative
- Version: v1.0
- Canonical for: prescribe/report lifecycle, correlation, and delivery semantics
- Audience: public

This document defines the **minimal protocol contract** required for all Evidra integrations
(MCP, REST sidecar, CLI, or future SDKs).

The goal is to eliminate ambiguity across:

- session/run lifecycle
- event correlation
- environment scope
- external validator ingestion
- delivery semantics
- evidence compatibility

This protocol is designed to support integrations with systems such as:

- LangChain
- LangGraph
- AutoGen
- CrewAI
- CI/CD pipelines
- Terraform automation
- Kubernetes operators
- security scanners

---

# 1. Core Concepts

## 1.1 Session (Run)

A **session** represents the lifecycle of a single automation execution.

Examples:

- a LangChain agent run
- a CI pipeline execution
- a Terraform apply workflow
- a Kubernetes reconciliation loop

All events MUST belong to exactly one session.

Fields:

| Field | Requirement |
|------|-------------|
| session_id | MUST |
| started_at | MUST |
| ended_at | MAY |
| labels | MAY |

Properties:

- session_id MUST be globally unique (ULID recommended)
- sessions define the **boundary for metrics and scorecards**
- signals SHOULD aggregate within a session

---

# 2. Operation / Event Model

Each step of automation produces an **event** stored as an evidence entry.

## 2.1 Internal Entry Types (Normative)

The `EntryType` enum in code (`pkg/evidence/entry.go`) defines the canonical
set of entry types persisted in the evidence chain:

```
prescribe                  # pre-execution intent record
report                     # post-execution outcome report
finding                    # inspector/scanner-generated finding
signal                     # behavioral signal detection result
receipt                    # acknowledgement from a remote system
canonicalization_failure   # canonicalization error record
session_start              # session lifecycle: begin
session_end                # session lifecycle: end
annotation                 # human or system annotation
```

All evidence entries MUST use one of these values in the `type` field.

## 2.2 Conceptual Event Taxonomy

The protocol uses a higher-level conceptual taxonomy to describe automation workflows:

| Conceptual event | Internal `EntryType` | Notes |
|------------------|----------------------|-------|
| `operation.start` | `prescribe` | Pre-execution intent |
| `operation.end` | `report` (verdict=success) | Successful completion |
| `operation.error` | `report` (verdict=failure) | Failed/aborted operation |
| `validator.findings` | `finding` | External scanner results |
| `session.start` | `session_start` | Session lifecycle |
| `session.end` | `session_end` | Session lifecycle |
| `annotation` | `annotation` | Human/system annotation |

The `signal`, `receipt`, and `canonicalization_failure` entry types are
auxiliary entries and have no conceptual event counterpart. Core write paths no
longer require canonicalization; `canonicalization_failure` remains only for
legacy/companion tooling that attempts canonicalization explicitly.

External event-bus and standards mappings are intentionally outside the live
public protocol contract.

## 2.3 Reframe, Not Rename

The lifecycle pair is intentionally stable:

- `prescribe` means intent registered before execution
- `report` means outcome recorded after execution

This vocabulary applies to imperative commands, reconciliation loops, and
workflows. Integrations SHOULD prefer payload metadata on prescribe/report
payloads over introducing new primary lifecycle entry types.

v1 flavors:

- `imperative`
- `reconcile`
- `workflow`

`payload.flavor` answers what kind of execution this was.

`payload.evidence.kind` answers how Evidra obtained the lifecycle evidence:

- `declared`
- `observed`
- `translated`

`payload.source.system` identifies the producing system, for example `cli`,
`mcp`, `argocd`, or `agentgateway`.

The same correlation rules and the same signal pipeline apply across all
flavors.

The same lifecycle vocabulary is also exposed through the server-side external
ingest API:

- `POST /v1/evidence/ingest/prescribe`
- `POST /v1/evidence/ingest/report`

These routes are the typed external lifecycle surface. The request contract
keeps `flavor`, `evidence.kind`, and `source.system` explicit; persisted
entries then expose the same context as `payload.flavor`,
`payload.evidence.kind`, and `payload.source.system`. The webhook routes remain
compatibility wrappers over the shared ingest service.

---

# 3. Correlation Model

To support complex agent workflows, the protocol supports hierarchical tracing.

| Field | Requirement |
|------|-------------|
| session_id | MUST |
| event_id | MUST |
| trace_id | MUST |
| span_id | SHOULD |
| parent_span_id | MAY |
| operation_id | SHOULD |
| attempt | MAY |

Rules:

- `event_id` MUST be globally unique
- `trace_id` groups related events
- `span_id` identifies a step in execution
- `parent_span_id` allows hierarchical workflows
- `report` without explicit `session_id` MUST inherit `session_id` from the referenced `prescribe`
- `report` with explicit `session_id` that does not match the referenced `prescribe` MUST be rejected (`invalid_input`)
- `canonicalization_failure` and `signal` entries SHOULD inherit `session_id`/`trace_id` from the originating operation when available
- when no originating trace exists, integrations SHOULD generate a new `trace_id` instead of leaving it empty

This model allows compatibility with **OpenTelemetry** style tracing.

## 3.1 Correlation Defaults (Normative)

Unless a caller explicitly provides correlation fields:

- `session_id` MUST be generated by the integration layer before persistence
- `trace_id` MUST default to `session_id`
- `span_id` is OPTIONAL and SHOULD be omitted when no hierarchical span model is needed
- `parent_span_id` MUST only be set when `span_id` is set

---

# 4. Actor Identity

Actors represent the automation entity performing the action.

Examples:

- CI bot
- Kubernetes controller
- AI agent
- Terraform automation

Fields:

| Field | Requirement |
|------|-------------|
| actor.id | MUST |
| actor.type | SHOULD |
| actor.instance_id | MAY |
| actor.version | MAY |
| actor.skill_version | MAY |

Rules:

- `actor.id` MUST be stable and low-cardinality
- `actor.instance_id` MUST NOT be used as a metrics label
- `actor.skill_version` SHOULD be set by agent integrations to track contract/prompt version used during execution

Example:

```
actor:
  id: ci-bot
  type: automation
  instance_id: runner-234
  version: 1.4.2
  skill_version: 1.0.0
```

---

# 5. Environment Scope Model

Automation often operates across different infrastructure scopes.

To maintain stable metrics, Evidra separates:

- **scope_class** (low cardinality)
- **scope.dimensions** (detailed metadata)

## 5.1 Scope Class

Canonical persisted enum values:

```
production
staging
development
unknown
```

Rules:

- `scope_class` MUST be low cardinality
- `scope_class` MUST be derived deterministically by canonicalization rules
- persisted values MUST use only the canonical enum values above
- ingress aliases MUST normalize before persistence as:
  - `prod -> production`
  - `dev -> development`
  - `test -> development`
  - `sandbox -> development`
- integrations SHOULD validate `scope_class` at ingress (CLI/MCP/REST) and reject unknown non-alias strings with `invalid_input`

## 5.2 Scope Dimensions

Optional metadata describing the environment.

Example:

```
scope:
  class: production
  dimensions:
    cluster: prod-cluster-1
    namespace: payments
    account: aws-prod
    region: eu-central-1
```

Dimensions MUST NOT be used as metrics labels.

---

# 6. Artifact Identity

Automation actions often reference artifacts.

Examples:

- terraform plan
- container image
- deployment manifest

Fields:

| Field | Requirement |
|------|-------------|
| artifact.digest | SHOULD |
| artifact.type | MAY |
| artifact.uri | MAY |

Digest MUST use cryptographic hashing (SHA256 recommended).

Example:

```
artifact:
  digest: sha256:abcd1234
  type: terraform-plan
```

---

# 7. Validator Findings Ingestion

External security or compliance scanners may submit findings.

Supported sources:

- SAST
- IaC scanners
- container scanners
- policy engines

## Endpoint

```
POST /v1/evidence/findings
```

## Required fields

| Field | Requirement |
|------|-------------|
| tool | MUST |
| tool_version | MAY |
| artifact_digest | MUST |
| rule_id | MUST |
| severity | MUST |
| resource | MAY |
| message | MAY |
| external_refs | MAY |
| operation_id | MAY |
| timestamp | MUST |

Example:

```
{
  "tool": "trivy",
  "artifact_digest": "sha256:abcd",
  "rule_id": "CVE-2023-1234",
  "severity": "high",
  "resource": "container:api",
  "timestamp": "2026-03-05T10:00:00Z"
}
```

## 7.1 Finding Correlation Rules (Normative)

- findings MAY omit `operation_id` to support scanner-only ingestion flows
- findings without `operation_id` MUST include `artifact_digest` (primary correlation key)
- findings SHOULD include `external_refs` when available (for example ticket/change/run references)
- when `operation_id` is present, integrations SHOULD validate it against known session/operation context when available

Deduplication key:

```
artifact_digest + tool + rule_id + resource
```

---

# 8. Delivery Guarantees

Protocol uses **At-Least-Once delivery**.

Rules:

- events MAY be delivered multiple times
- backend MUST deduplicate using `event_id`
- replay MUST preserve original event_id

---

# 9. Backend Delivery Modes

Two operating modes exist.

## offline-first (default)

```
event -> sidecar -> local evidence
                     |
                     v
                   outbox -> backend
```

Properties:

- sidecar always writes locally
- backend delivery happens asynchronously
- replay guarantees eventual delivery

## backend-required

```
event -> backend required
```

If backend is unavailable, event submission fails.

---

# 10. Evidence Entry Requirements

Every stored evidence entry MUST include:

| Field |
|------|
| spec_version |
| canon_version |
| scoring_version |
| event_id |
| session_id |
| timestamp |
| hash |
| previous_hash |

Protocol invariants for write paths:

- every persisted entry MUST have session linkage (`session_id`)
- every persisted entry MUST include a non-empty `trace_id`
- validator/internal auxiliary entries (`signal`, `canonicalization_failure`) MUST preserve originating correlation fields when known

Optional:

| Field |
|------|
| signature |
| encrypted_payload |

---

# 11. JSON Schema Shapes for Write APIs

The live API does not expose a generic `/v1/events` route. Evidence is written
through three surfaces:

```
POST /v1/evidence/forward
POST /v1/evidence/batch
POST /v1/evidence/ingest/prescribe
POST /v1/evidence/ingest/report
POST /v1/evidence/findings
```

`/v1/evidence/forward` and `/v1/evidence/batch` accept already-shaped evidence
entries. `/v1/evidence/ingest/prescribe` and `/v1/evidence/ingest/report` are
typed external adapter routes; the server builds and signs the final evidence
entries from those requests.

Representative raw entry shape for `/v1/evidence/forward`:

```
{
  "type": "object",
  "required": [
    "event_id",
    "session_id",
    "event_type",
    "timestamp"
  ],
  "properties": {
    "event_id": { "type": "string" },
    "session_id": { "type": "string" },
    "event_type": { "type": "string" },
    "timestamp": { "type": "string" },

    "trace_id": { "type": "string" },
    "span_id": { "type": "string" },
    "parent_span_id": { "type": "string" },

    "actor": { "type": "object" },

    "scope": { "type": "object" },

    "artifact": { "type": "object" },

    "payload": { "type": "object" }
  }
}
```

Representative typed lifecycle ingest shape for `/v1/evidence/ingest/prescribe`:

```
{
  "type": "object",
  "required": [
    "actor",
    "flavor",
    "evidence",
    "source"
  ],
  "properties": {
    "claim": { "type": "object" },
    "actor": { "type": "object" },
    "session_id": { "type": "string" },
    "operation_id": { "type": "string" },
    "trace_id": { "type": "string" },
    "span_id": { "type": "string" },
    "parent_span_id": { "type": "string" },
    "scope_dimensions": { "type": "object" },
    "flavor": { "type": "string" },
    "evidence": {
      "type": "object",
      "required": ["kind"]
    },
    "source": {
      "type": "object",
      "required": ["system"]
    },
    "intent": { "type": "object" },
    "canonical_action": { "type": "object" },
    "assessment": { "type": "object" },
    "smart_target": { "type": "object" },
    "payload_override": { "type": "object" }
  }
}
```

---

# 12. Compatibility Guarantees

The protocol guarantees:

- forward compatibility through optional fields
- deterministic hashing of recorded intent and entries
- immutable evidence chains
- low-cardinality metric dimensions

Breaking changes require **Protocol v2**.

---

# 13. Summary

This protocol ensures:

- deterministic evidence
- consistent scope classification
- robust automation tracing
- safe validator ingestion
- scalable telemetry for infrastructure automation (including AI)

It establishes a stable foundation for:

- LangChain
- LangGraph
- AutoGen
- CrewAI
- CI/CD integrations
- security scanners
