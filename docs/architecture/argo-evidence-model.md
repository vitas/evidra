# Argo Evidence Model (v1)

This document defines what Argo CD evidence is required for incident investigation and deployment traceability in v1.

## Required Argo evidence
- Application identity and target:
  - application name
  - destination cluster
  - destination namespace
- Revision/source context:
  - sync/history revision
  - source repo URL
  - source path/chart
  - target revision intent
- Operation lifecycle:
  - operation/history identifier
  - initiator
  - started and finished timestamps
  - terminal phase/result
- Health/sync transitions:
  - sync state changes
  - health state changes
  - ordering relative to deployment completion
- External traceability via annotations:
  - `evidra.io/change-id`
  - `evidra.io/ticket`
  - `evidra.io/approvals-ref`
  - `evidra.io/approvals-json` (optional)

## Non-goals in v1
- Full manifest/resource diff rendering.
- Deep AppProject policy analysis.
- Raw Kubernetes event mirroring as primary timeline source.

## CloudEvents Mapping
- Extracted Argo evidence is normalized into `StoredEvent` structs that follow the CloudEvents 1.0 specification (`specversion`, `id`, `source`, `type`, `subject`, `time`, `data`). All domain context fields (cluster, namespace, initiator, commit_sha, ticket_id, etc.) are stored as CloudEvents extensions in `Extensions map[string]interface{}`. Serialization and integrity hashing use a custom `internal/cloudevents` package.
