# API Reference

All endpoints are served by `evidra-api` (default `:8080`).

## Authentication

Authenticated endpoints require a Bearer token in the `Authorization` header:

```
Authorization: Bearer <api-key>
```

Keys are issued via `POST /v1/keys` (see below) or set statically with the `EVIDRA_API_KEY` environment variable.

---

## Public Endpoints

### `GET /healthz`

Liveness probe. Returns `200 OK` with body `ok`.

### `GET /readyz`

Readiness probe. Returns `200 OK` when the database connection is healthy.

### `GET /v1/evidence/pubkey`

Returns the Ed25519 public key (PEM-encoded) when signing is configured.

---

## Key Management

### `POST /v1/keys`

Issue a new API key. Gated by an invite secret, not by standard Bearer auth.

**Headers:**

| Header | Required | Description |
|---|---|---|
| `X-Invite-Secret` | Yes | Must match the server's `EVIDRA_INVITE_SECRET` value |

**Request body:**

```json
{ "label": "my-ci-pipeline" }
```

- `label` — optional, max 128 characters.

**Response** (`201 Created`):

```json
{
  "key": "ev1_abc123...",
  "prefix": "ev1_abc1",
  "tenant_id": "tnt_...",
  "created_at": "2025-01-15T10:30:00Z"
}
```

**Rate limit:** 3 keys per hour per IP.

**Errors:**
- `403` — missing or invalid invite secret
- `429` — rate limit exceeded
- `503` — invite secret not configured on server

---

## Evidence Ingestion

All ingestion endpoints require Bearer auth.

### `POST /v1/evidence/forward`

Forward a single evidence entry (raw JSON).

**Request body:** Any valid JSON evidence entry.

**Response:**

```json
{ "receipt_id": "01JD...", "status": "accepted" }
```

### `POST /v1/evidence/batch`

Ingest multiple entries in one request.

**Request body:**

```json
{ "entries": [ { ... }, { ... } ] }
```

**Response:**

```json
{ "accepted": 5, "errors": [] }
```

### `POST /v1/evidence/findings`

Ingest SARIF findings as evidence entries.

---

## Evidence Queries

All query endpoints require Bearer auth.

### `GET /v1/evidence/entries`

List evidence entries with pagination and optional filters.

**Query parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | integer | `100` | Page size (max 1000) |
| `offset` | integer | `0` | Number of entries to skip |
| `type` | string | — | Filter by entry type (`prescribe`, `report`, `finding`, etc.) |
| `period` | string | — | Time window (`7d`, `30d`, `90d`) |
| `session_id` | string | — | Filter by session ID |

**Response:**

```json
{
  "entries": [
    {
      "id": "01JD1A2B3C",
      "type": "prescribe",
      "tool": "kubectl",
      "operation": "apply",
      "scope": "namespace",
      "risk_level": "medium",
      "actor": "alice",
      "created_at": "2025-01-15T10:30:00Z"
    },
    {
      "id": "01JD1A2B3D",
      "type": "report",
      "tool": "kubectl",
      "operation": "apply",
      "scope": "namespace",
      "risk_level": "medium",
      "actor": "alice",
      "verdict": "success",
      "exit_code": 0,
      "created_at": "2025-01-15T10:30:05Z"
    }
  ],
  "total": 47,
  "limit": 20,
  "offset": 0
}
```

**Pagination example:**

```bash
# First page (20 entries)
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/v1/evidence/entries?limit=20&offset=0"

# Second page
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/v1/evidence/entries?limit=20&offset=20"

# Filter by type and period
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/v1/evidence/entries?type=prescribe&period=7d&limit=50"
```

### `GET /v1/evidence/entries/{id}`

Retrieve a single entry by ID.

**Response:** Same shape as a single entry in the list response above.

---

## Analytics

All analytics endpoints require Bearer auth.

### `GET /v1/evidence/scorecard`

Compute a reliability scorecard from stored evidence.

**Query parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `period` | string | `30d` | Time window (`7d`, `24h`, `30d`, `90d`) |
| `actor` | string | — | Filter by actor ID |
| `tool` | string | — | Filter by tool name |
| `scope` | string | — | Filter by scope |
| `session_id` | string | — | Filter by session |
| `min_operations` | integer | — | Minimum operation count threshold |

**Response:**

```json
{
  "score": 96.5,
  "band": "good",
  "basis": "sufficient",
  "confidence": "high",
  "total_entries": 47,
  "signal_summary": {
    "protocol_violation": { "detected": false, "weight": 0.30, "count": 0 },
    "artifact_drift": { "detected": true, "weight": 0.25, "count": 2 },
    "retry_loop": { "detected": true, "weight": 0.15, "count": 1 },
    "thrashing": { "detected": false, "weight": 0.10, "count": 0 },
    "blast_radius": { "detected": false, "weight": 0.10, "count": 0 },
    "risk_escalation": { "detected": false, "weight": 0.10, "count": 0 },
    "new_scope": { "detected": true, "weight": 0.05, "count": 3 },
    "repair_loop": { "detected": false, "weight": -0.05, "count": 0 }
  },
  "period": "30d",
  "scoring_version": "v1.1.0",
  "generated_at": "2025-01-15T10:30:00Z"
}
```

### `GET /v1/evidence/explain`

Signal-level breakdown of detected behavioral patterns.

**Query parameters:** Same as `/v1/evidence/scorecard`.

---

## Webhooks

### `POST /v1/hooks/argocd`

ArgoCD webhook receiver. Requires:
- `Authorization: Bearer <EVIDRA_WEBHOOK_SECRET_ARGOCD>`
- `X-Evidra-API-Key: <tenant-api-key>`

Maps `sync_started` / `sync_completed` events to prescribe/report entries.

### `POST /v1/hooks/generic`

Generic webhook receiver. Requires:
- `Authorization: Bearer <EVIDRA_WEBHOOK_SECRET_GENERIC>`
- `X-Evidra-API-Key: <tenant-api-key>`

Maps `operation_started` / `operation_completed` events.

Contract:
- `operation_id` is required on both start and completion events and is the stable lifecycle identity used to correlate prescribe/report entries.
- `idempotency_key` remains required on `operation_completed`, but only for duplicate suppression.

---

## Benchmark

All benchmark endpoints require Bearer auth.

### `POST /v1/benchmark/run`

Submit a benchmark run.

### `GET /v1/benchmark/runs`

List benchmark runs.

### `GET /v1/benchmark/compare`

Compare actor reliability across benchmark runs.

---

## Auth Check

### `GET /auth/check`

Validate a Bearer token. Returns `200` if valid, `401` if not. Useful as a forward-auth target for reverse proxies.

---

## Error Format

All errors return JSON:

```json
{ "error": "human-readable message" }
```

Common status codes:
- `400` — bad request (invalid params or body)
- `401` — missing or invalid auth token
- `403` — forbidden (invalid invite secret)
- `404` — resource not found
- `429` — rate limit exceeded
- `500` — internal server error
