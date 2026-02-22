#!/usr/bin/env bash
set -euo pipefail

NS="${EVIDRA_NAMESPACE:-evidra}"
DEPLOYMENT="${EVIDRA_DEPLOYMENT:-evidra-trial}"
SERVICE="${EVIDRA_SERVICE:-evidra-trial}"
LOCAL_PORT="${EVIDRA_LOCAL_PORT:-18080}"
BASE_URL="http://127.0.0.1:${LOCAL_PORT}"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; exit 1; }

kubectl -n "${NS}" rollout status "deployment/${DEPLOYMENT}" --timeout=180s || fail "trial deployment rollout"
pass "trial deployment rollout"

kubectl -n "${NS}" port-forward "svc/${SERVICE}" "${LOCAL_PORT}:80" >/tmp/evidra_trial_pf.log 2>&1 &
PF_PID=$!
trap 'kill ${PF_PID} >/dev/null 2>&1 || true' EXIT
sleep 2

curl -fsS "${BASE_URL}/healthz" >/tmp/evidra_trial_healthz.json || fail "health endpoint"
pass "health endpoint"

curl -fsS "${BASE_URL}/v1/subjects" >/tmp/evidra_trial_subjects.json || fail "subjects endpoint"
pass "subjects endpoint"

echo "trial smoke check completed"
