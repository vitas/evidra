#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${EVIDRA_BASE_URL:-http://localhost:8080}"

curl -fsS "${BASE_URL}/healthz" >/tmp/evidra_health.json

event_payload='{
  "id": "evt_smoke_001",
  "source": "argocd",
  "type": "argo.sync.finished",
  "timestamp": "2026-02-16T12:34:56Z",
  "subject": {
    "app": "payments-api",
    "environment": "prod-eu",
    "cluster": "eu-1"
  },
  "actor": {
    "kind": "user",
    "id": "smoke-user"
  },
  "metadata": {
    "argocd_app": "payments-api",
    "sync_revision": "abc123",
    "operation_id": "op-smoke-1"
  },
  "raw_ref": {
    "kind": "argocd",
    "ref": "history/42"
  },
  "event_schema_version": 1
}'

curl -fsS -X POST "${BASE_URL}/v1/events" \
  -H 'Content-Type: application/json' \
  -d "${event_payload}" >/tmp/evidra_ingest.json

curl -fsS "${BASE_URL}/v1/timeline?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&limit=10" >/tmp/evidra_timeline.json

echo "smoke check passed"
