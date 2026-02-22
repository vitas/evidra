# API and Change Contracts v1

## Change identity
- `ChangeID` format: `chg_<sha256...>`.
- Deterministic and URL-safe.
- Derived from normalized subject + primary provider (`argo`) + stable reference.
- `GET /v1/changes/{id}` is the stable retrieval endpoint for permalink usage.

## Change summary contract
`GET /v1/changes` and `GET /v1/changes/{id}` expose summary fields used by Explorer:

- `change_id`, `permalink`
- `application`, `project`, `target_cluster`, `namespace`
- `revision`, `initiator`
- `started_at`, `completed_at`, `result_status`
- `health_at_operation_start`, `health_after_deploy`
- `external_change_id`, `ticket_id`
- `evidence_last_updated_at`, `evidence_window_seconds`, `evidence_may_be_incomplete`

## Post-deploy degradation semantics

- Field: `post_deploy_degradation`
- Shape: `{ observed: boolean, first_timestamp?: RFC3339 }`
- Rule: observational only. It indicates temporal sequence and does not claim causality.

## Normalized status fields
- Result status: `succeeded`, `failed`, `unknown`.
- Health status: `healthy`, `degraded`, `progressing`, `missing`, `unknown`.

## Runtime API (v1)
Operational:
- `GET /healthz`

Ingestion:
- `POST /v1/events` (Consumes `application/cloudevents+json` and `application/cloudevents-batch+json`)

Query:
- `GET /v1/timeline`
- `GET /v1/subjects`
- `GET /v1/events/{id}`
- `GET /v1/correlations/{key}`
- `GET /v1/changes`
- `GET /v1/changes/{id}`
- `GET /v1/changes/{id}/timeline`
- `GET /v1/changes/{id}/evidence`

Exports:
- `POST /v1/exports`
- `GET /v1/exports/{id}`
- `GET /v1/exports/{id}/download`

## `/v1/changes` filter contract
Required:
- `subject`
- `from`
- `to`

Optional:
- `q`
- `result_status`
- `health_status`
- `external_change_id`
- `ticket_id`
- `approval_reference`
- `limit`
- `cursor`

## Export reproducibility
Repeated export over the same stored evidence must produce the same logical content and deterministic ordering.

## Export metadata contract
Downloaded export JSON includes top-level fields:

- `change_id`
- `generated_at`
- `source` (`argocd`)
- `application`, `cluster`, `namespace`
- `revision`, `initiator`, `result`
- `post_deploy_degradation`
- `timeline`
- `external_change_id` and `ticket_id` when present
- `deterministic_hash_sha256`
