#!/usr/bin/env bash
set -euo pipefail

NS="${EVIDRA_NAMESPACE:-evidra}"
EVIDRA_DEPLOY="${EVIDRA_DEPLOYMENT:-evidra-prod-secure}"
PROXY_DEPLOY="${EVIDRA_PROXY_DEPLOYMENT:-oauth2-proxy-prod-secure}"
EVIDRA_SVC="${EVIDRA_SERVICE:-evidra-prod-secure}"
LOCAL_PORT="${EVIDRA_LOCAL_PORT:-18080}"
BASE_URL="http://127.0.0.1:${LOCAL_PORT}"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; exit 1; }

kubectl -n "${NS}" rollout status "deployment/${EVIDRA_DEPLOY}" --timeout=180s || fail "evidra deployment rollout"
pass "evidra deployment rollout"

kubectl -n "${NS}" rollout status "deployment/${PROXY_DEPLOY}" --timeout=180s || fail "oauth2-proxy deployment rollout"
pass "oauth2-proxy deployment rollout"

kubectl -n "${NS}" port-forward "svc/${EVIDRA_SVC}" "${LOCAL_PORT}:80" >/tmp/evidra_prod_secure_pf.log 2>&1 &
PF_PID=$!
trap 'kill ${PF_PID} >/dev/null 2>&1 || true' EXIT
sleep 2

code=$(curl -s -o /tmp/evidra_ps_unauth.json -w "%{http_code}" "${BASE_URL}/v1/subjects")
[[ "${code}" == "401" ]] || fail "unauthorized read without role header"
pass "unauthorized read without role header"

code=$(curl -s -o /tmp/evidra_ps_reader.json -w "%{http_code}" -H "X-Forwarded-Groups: reader" "${BASE_URL}/v1/subjects")
[[ "${code}" == "200" ]] || fail "reader role can read subjects"
pass "reader role can read subjects"

code=$(curl -s -o /tmp/evidra_ps_export_reader.json -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -H "X-Forwarded-Groups: reader" \
  "${BASE_URL}/v1/exports" \
  -d '{"format":"json","filter":{}}')
[[ "${code}" == "401" ]] || fail "reader role cannot create export"
pass "reader role cannot create export"

code=$(curl -s -o /tmp/evidra_ps_export_exporter.json -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -H "X-Forwarded-Groups: exporter" \
  "${BASE_URL}/v1/exports" \
  -d '{"format":"json","filter":{}}')
[[ "${code}" == "202" ]] || fail "exporter role can create export"
pass "exporter role can create export"

read -r -d '' EVENT_PAYLOAD <<'JSON' || true
{
  "id":"evt_smoke_prod_secure_1",
  "source":"git",
  "type":"push",
  "timestamp":"2026-02-17T10:00:00Z",
  "subject":{"app":"payments-api","environment":"prod","cluster":"eu-1"},
  "actor":{"kind":"user","id":"smoke"},
  "metadata":{"commit_sha":"smoke123"},
  "raw_ref":{"kind":"external","ref":"smoke:test"},
  "event_schema_version":1
}
JSON

code=$(curl -s -o /tmp/evidra_ps_ingest_reader.json -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -H "X-Forwarded-Groups: reader" \
  "${BASE_URL}/v1/events" \
  -d "${EVENT_PAYLOAD}")
[[ "${code}" == "401" ]] || fail "reader role cannot ingest"
pass "reader role cannot ingest"

code=$(curl -s -o /tmp/evidra_ps_ingest_admin.json -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -H "X-Forwarded-Groups: admin" \
  "${BASE_URL}/v1/events" \
  -d "${EVENT_PAYLOAD}")
[[ "${code}" == "202" ]] || fail "admin role can ingest"
pass "admin role can ingest"

echo "prod-secure smoke check completed"
