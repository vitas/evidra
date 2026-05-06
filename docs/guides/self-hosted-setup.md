# Self-Hosted Setup

- Status: Guide
- Version: current
- Canonical for: self-hosted deployment and operator setup
- Audience: public

This document covers deploying `evidra-api` with the web dashboard, key issuance, and evidence analytics.

CLI and MCP remain the primary local and agent-facing analytics surfaces. Self-hosted exposes the same scorecard and explain engine over centralized stored evidence, plus a web UI for onboarding and monitoring.

Self-hosted remains supported for centralized evidence collection.

GitOps note:

- Argo CD is controller-first in v1
- webhook ingestion remains supported, but it is not the whole Argo story
- webhook routes are compatibility wrappers over the shared lifecycle ingest service
- no Git provider access is required for the controller path

## Quick Start

### 1. Configure Environment

Create a `.env` file (or export variables) with your secrets:

```bash
# Required
export EVIDRA_API_KEY=my-secret-api-key        # Static API key for authenticated endpoints
export DATABASE_URL=postgres://evidra:evidra@localhost:5432/evidra?sslmode=disable

# Recommended — enables the onboarding flow and key issuance
export EVIDRA_INVITE_SECRET=my-invite-secret    # Gate for POST /v1/keys

# Optional
export EVIDRA_SIGNING_MODE=optional             # "strict" (default) or "optional"
export EVIDRA_SIGNING_KEY=                      # Base64 Ed25519 private key
export EVIDRA_WEBHOOK_SECRET_ARGOCD=            # Bearer secret for ArgoCD webhooks
export EVIDRA_WEBHOOK_SECRET_GENERIC=           # Bearer secret for generic webhooks
export EVIDRA_ARGOCD_CONTROLLER_ENABLED=false   # Enable in-cluster Argo CD controller integration
export EVIDRA_ARGOCD_APPLICATION_NAMESPACE=argocd
export EVIDRA_ARGOCD_TENANT_ID=default
export EVIDRA_KUBECONFIG=                       # Optional local-development kubeconfig override
export LISTEN_ADDR=:8080                        # HTTP listen address (default :8080)
```

### 2. Start with Docker Compose

```bash
export EVIDRA_API_KEY=my-secret-api-key
export EVIDRA_INVITE_SECRET=my-invite-secret
docker compose up --build -d
```

Verify:

```bash
curl http://localhost:8080/healthz   # → ok
curl http://localhost:8080/readyz    # → ok (once Postgres is ready)
```

### 3. Open the Dashboard

Navigate to `http://localhost:8080` in your browser. The embedded web UI provides:

- **Landing page** (`/`) — overview and quick start guide
- **Onboarding** (`/onboarding`) — guided API key generation wizard
- **Dashboard** (`/dashboard`) — reliability scorecard, signal breakdown, evidence timeline with pagination

## Key Issuance and Onboarding

The web UI includes a 4-step onboarding wizard at `/onboarding`:

1. **Invite secret** — enter the value of `EVIDRA_INVITE_SECRET`
2. **Label** — name the key (e.g. "ci-pipeline", "dev-laptop")
3. **Key reveal** — copy the generated API key (shown only once)
4. **MCP configuration** — copy-paste config for Claude Code, Cursor, Codex, or Gemini CLI with your key pre-filled

### Issuing keys via API

```bash
curl -X POST http://localhost:8080/v1/keys \
  -H "Content-Type: application/json" \
  -H "X-Invite-Secret: my-invite-secret" \
  -d '{"label": "my-pipeline"}'
```

Response:

```json
{
  "key": "ev1_abc123...",
  "prefix": "ev1_abc1",
  "tenant_id": "tnt_...",
  "created_at": "2025-01-15T10:30:00Z"
}
```

**Rate limit:** 3 keys per hour per IP address.

> **Important:** The `EVIDRA_INVITE_SECRET` environment variable must be set on the server for key issuance to work. Without it, `POST /v1/keys` returns `503 Service Unavailable`. The invite secret is shared with users who need to generate keys — it is not the same as `EVIDRA_API_KEY`.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `EVIDRA_API_KEY` | Yes | — | Static API key for Bearer auth on all authenticated endpoints |
| `EVIDRA_INVITE_SECRET` | No | — | Gate for key issuance (`POST /v1/keys`). Users enter this in the onboarding wizard or pass it as `X-Invite-Secret` header. Without it, key issuance is disabled. |
| `LISTEN_ADDR` | No | `:8080` | HTTP listen address |
| `EVIDRA_SIGNING_KEY` | No | — | Base64-encoded Ed25519 private key for evidence signing |
| `EVIDRA_SIGNING_KEY_PATH` | No | — | Path to PEM Ed25519 private key (alternative to `EVIDRA_SIGNING_KEY`) |
| `EVIDRA_SIGNING_MODE` | No | `strict` | `strict` requires signing key; `optional` allows unsigned evidence |
| `EVIDRA_WEBHOOK_SECRET_ARGOCD` | No | — | Bearer secret for `/v1/hooks/argocd` webhook receiver |
| `EVIDRA_WEBHOOK_SECRET_GENERIC` | No | — | Bearer secret for `/v1/hooks/generic` webhook receiver |
| `EVIDRA_ARGOCD_CONTROLLER_ENABLED` | No | `false` | Enable controller-observed Argo CD reconciliation evidence |
| `EVIDRA_ARGOCD_APPLICATION_NAMESPACE` | No | `argocd` | Namespace containing Argo `Application` objects |
| `EVIDRA_ARGOCD_TENANT_ID` | No | `default` | Tenant that receives controller-emitted evidence |
| `EVIDRA_KUBECONFIG` | No | — | Optional kubeconfig path for local development; in-cluster config is used by default |

## Argo CD Controller Mode

When `EVIDRA_ARGOCD_CONTROLLER_ENABLED=true`, `evidra-api` watches Argo
`Application` objects and emits reconciliation evidence.

- zero-touch mode records mapped reconcile-flavor prescribe/report pairs
- explicit mode links reconcile completion back to an existing prescription via
  `evidra.cc/*` annotations
- this path is intended for in-cluster deployment
- direct Argo CLI capture is optional and non-strategic

Guide: [Argo CD GitOps integration](argocd-gitops-integration.md)

## Supported Endpoints

### Public (no auth)
- `GET /healthz` — liveness probe
- `GET /readyz` — readiness probe (checks database)
- `GET /v1/evidence/pubkey` — Ed25519 public key (when signing configured)

### Key management (invite-gated)
- `POST /v1/keys` — issue API key (requires `X-Invite-Secret` header)

### Evidence ingestion (Bearer auth)
- `POST /v1/evidence/forward` — raw single-entry forwarding
- `POST /v1/evidence/batch` — raw batch forwarding
- `POST /v1/evidence/ingest/prescribe` — typed external lifecycle prescribe ingest
- `POST /v1/evidence/ingest/report` — typed external lifecycle report ingest
- `POST /v1/evidence/findings` — SARIF findings ingestion

`/v1/evidence/forward` and `/v1/evidence/batch` remain the raw transport
paths. The `/v1/evidence/ingest/prescribe` and `/v1/evidence/ingest/report`
routes are the typed external lifecycle surface for adapters and controllers.
The request contract carries the explicit taxonomy:

- `flavor` for execution shape, including `workflow`
- `evidence.kind` for acquisition mode, including `declared`, `observed`,
  and `translated`
- `source.system` for the producing adapter or upstream system

Persisted evidence entries expose those same fields under `payload.flavor`,
`payload.evidence.kind`, and `payload.source.system`.

### Evidence queries (Bearer auth)
- `GET /v1/evidence/entries` — paginated entry listing with filters
- `GET /v1/evidence/entries/{id}` — single entry by ID

### Analytics (Bearer auth)
- `GET /v1/evidence/scorecard` — reliability scorecard
- `GET /v1/evidence/explain` — signal-level breakdown

### Webhooks
- `POST /v1/hooks/argocd` — ArgoCD sync events
- `POST /v1/hooks/generic` — generic operation events

Webhook ingestion remains available, but for Argo CD it is now the adjacent
path, not the primary GitOps integration story. Both webhook routes are
compatibility wrappers over the shared lifecycle ingest service.

Webhook ingestion is tenant-aware:
- `Authorization: Bearer <webhook-secret>` gates the route
- `X-Evidra-API-Key: <tenant-api-key>` selects the tenant that receives the mapped evidence

Full endpoint documentation: [API Reference](../api-reference.md)

## Pagination

The `GET /v1/evidence/entries` endpoint supports limit/offset pagination:

```bash
# Page 1 (first 20 entries)
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/v1/evidence/entries?limit=20&offset=0"

# Page 2
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/v1/evidence/entries?limit=20&offset=20"
```

Response includes `total` for computing page count:

```json
{ "entries": [...], "total": 47, "limit": 20, "offset": 0 }
```

The web dashboard uses this pagination automatically.

## Analytics Contract

- Default scope is tenant-wide over stored evidence.
- Optional narrowing filters on scorecard and explain:
  - `actor`, `tool`, `scope`, `session_id`, `period`, `min_operations`
- Webhook mapping support:
  - controller-observed Argo CD reconciliation evidence
  - ArgoCD `sync_started` / `sync_completed`
  - Generic `operation_started` / `operation_completed`
  - generic webhook payloads must include `operation_id` on both start and completion events
  - `idempotency_key` on generic completion is for duplicate suppression, not lifecycle identity
  - webhook requests must include `X-Evidra-API-Key` so mapped evidence lands in the correct tenant
- Hosted `compare` is not part of this contract yet.

## Connecting CLI and MCP

### CLI forwarding

Point the CLI at the API backend to forward evidence centrally:

```bash
evidra record \
  --url http://localhost:8080 \
  --api-key $EVIDRA_API_KEY \
  -f deploy.yaml -- kubectl apply -f deploy.yaml
```

### MCP server with API forwarding

Set `EVIDRA_URL` and `EVIDRA_API_KEY` to forward MCP evidence to the API:

```bash
EVIDRA_URL=http://localhost:8080 \
EVIDRA_API_KEY=my-secret-api-key \
evidra-mcp --evidence-dir ~/.evidra/evidence
```

## Practical Guidance

- Use self-hosted when you want centralized evidence collection, API keys, entry browsing, dashboard monitoring, and tenant-wide analytics over forwarded evidence.
- Use the Argo CD controller path when you want centralized GitOps reconciliation evidence without requiring Git access.
- Use CLI or MCP when you want local-first workflows, immediate command assessment, or agent-native tool invocation.
- Treat hosted `scorecard` and `explain` as the supported analytics surface for stored evidence. Hosted `compare` is still future work.
