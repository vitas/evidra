#!/usr/bin/env bash
set -euo pipefail

ARGOCD_NS="${ARGOCD_NAMESPACE:-argocd}"
ARGO_BASE_URL="${ARGO_BASE_URL:-https://localhost:8081}"
EVIDRA_ARGO_API_URL="${EVIDRA_ARGO_API_URL:-https://host.docker.internal:8081}"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"
}

require_cmd kubectl
require_cmd curl
require_cmd jq

bash scripts/argocd-port-forward.sh start >/dev/null

admin_password="$(kubectl -n "${ARGOCD_NS}" get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"
[[ -n "${admin_password}" ]] || fail "unable to read Argo CD admin password"

session_payload="$(jq -nc --arg u admin --arg p "${admin_password}" '{username:$u,password:$p}')"
api_token="$(curl -ksS -X POST "${ARGO_BASE_URL}/api/v1/session" -H 'Content-Type: application/json' -d "${session_payload}" | jq -r '.token // empty')"
[[ -n "${api_token}" ]] || fail "unable to obtain Argo CD API token"

EVIDRA_ARGO_COLLECTOR_ENABLED=true \
EVIDRA_ARGO_API_URL="${EVIDRA_ARGO_API_URL}" \
EVIDRA_ARGO_API_TOKEN="${api_token}" \
EVIDRA_ARGO_COLLECTOR_INTERVAL="5s" \
EVIDRA_ARGO_CHECKPOINT_FILE="/tmp/evidra-argo-checkpoint.json" \
EVIDRA_DEV_INSECURE=true \
  docker compose up -d --build postgres evidra >/dev/null

for _ in $(seq 1 30); do
  if curl -fsS "http://localhost:8080/healthz" >/dev/null 2>&1; then
    pass "Evidra restarted with Argo collector enabled"
    echo "ARGO_BASE_URL=${ARGO_BASE_URL}"
    echo "EVIDRA_ARGO_API_URL=${EVIDRA_ARGO_API_URL}"
    exit 0
  fi
  sleep 1
done

fail "Evidra did not become healthy after collector configuration"
