
# Evidra Protocol v1.0 — Integration Contract
Status: Draft (Normative for v1 Integrations)

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

Each step of automation produces an **event**.

Examples:

- agent started
- tool invoked
- tool finished
- error occurred
- validator findings

Event types:

```
session_start
session_end
agent_start
agent_end
tool_start
tool_end
tool_error
annotation
validator_findings
```

---

# 3. Correlation Model

To support complex agent workflows, the protocol supports hierarchical tracing.

| Field | Requirement |
|------|-------------|
| session_id | MUST |
| event_id | MUST |
| trace_id | SHOULD |
| span_id | SHOULD |
| parent_span_id | MAY |
| operation_id | SHOULD |
| attempt | MAY |

Rules:

- `event_id` MUST be globally unique
- `trace_id` groups related events
- `span_id` identifies a step in execution
- `parent_span_id` allows hierarchical workflows

This model allows compatibility with **OpenTelemetry** style tracing.

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

Rules:

- `actor.id` MUST be stable and low-cardinality
- `actor.instance_id` MUST NOT be used as a metrics label

Example:

```
actor:
  id: ci-bot
  type: automation
  instance_id: runner-234
  version: 1.4.2
```

---

# 5. Environment Scope Model

Automation often operates across different infrastructure scopes.

To maintain stable metrics, Evidra separates:

- **scope_class** (low cardinality)
- **scope.dimensions** (detailed metadata)

## 5.1 Scope Class

```
prod
staging
dev
test
sandbox
unknown
```

Rules:

- scope_class MUST be low cardinality
- derived deterministically by canonicalization rules

## 5.2 Scope Dimensions

Optional metadata describing the environment.

Example:

```
scope:
  class: prod
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
POST /v1/findings
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

Optional:

| Field |
|------|
| signature |
| encrypted_payload |

---

# 11. JSON Schema for Event API

Example schema for `/v1/events`

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

---

# 12. Compatibility Guarantees

The protocol guarantees:

- forward compatibility through optional fields
- deterministic canonicalization
- immutable evidence chains
- low-cardinality metric dimensions

Breaking changes require **Protocol v2**.

---

# 13. Summary

This protocol ensures:

- deterministic evidence
- consistent scope classification
- robust agent tracing
- safe validator ingestion
- scalable telemetry for AI automation

It establishes a stable foundation for:

- LangChain
- LangGraph
- AutoGen
- CrewAI
- CI/CD integrations
- security scanners
