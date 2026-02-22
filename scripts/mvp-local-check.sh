#!/usr/bin/env bash
set -euo pipefail

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

BASE_URL="${EVIDRA_BASE_URL:-http://localhost:8080}"
SUBJECT="${EVIDRA_SUBJECT:-payments-api:prod-eu:eu-1}"
FROM_TS="${EVIDRA_FROM_TS:-2026-02-16T00:00:00Z}"
TO_TS="${EVIDRA_TO_TS:-2026-02-16T23:59:59Z}"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"
}

require_cmd curl

curl -fsS "${BASE_URL}/healthz" >/tmp/evidra_mvp_health.json || fail "health endpoint"
pass "health endpoint"

curl -fsS -X POST "${BASE_URL}/v1/events" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/events/argocd_event_valid.json >/tmp/evidra_mvp_argo_valid.json || fail "argo event ingest"
pass "argo event ingest"

curl -fsS -X POST "${BASE_URL}/v1/events" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/events/argocd_event_degraded_supporting.json >/tmp/evidra_mvp_argo_degraded.json || fail "supporting event ingest"
pass "supporting event ingest"

curl -fsS "${BASE_URL}/v1/changes?subject=${SUBJECT}&from=${FROM_TS}&to=${TO_TS}" >/tmp/evidra_mvp_changes.json || fail "changes query"
grep -q "\"items\"" /tmp/evidra_mvp_changes.json || fail "changes response shape"
pass "changes query"

echo "MVP local check completed"
