#!/usr/bin/env bash
set -euo pipefail

GITEA_NS="${EVIDRA_DEMO_GITEA_NAMESPACE:-evidra-demo-gitops}"
ARGOCD_NS="${ARGOCD_NAMESPACE:-argocd}"
APP_NS="${EVIDRA_DEMO_APP_NAMESPACE:-demo}"
APP_NAME="${EVIDRA_DEMO_APP_NAME:-guestbook-demo}"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"
}

require_cmd kubectl
require_cmd jq
require_cmd go
require_cmd curl

kubectl apply -k deploy/k8s/addons/gitea >/dev/null
kubectl -n "${GITEA_NS}" rollout status statefulset/gitea --timeout=300s >/dev/null
pass "gitea is ready"

kubectl create namespace "${APP_NS}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

admin_user_b64="$(kubectl -n "${GITEA_NS}" get secret gitea-admin -o jsonpath='{.data.GITEA_ADMIN_USERNAME}')"
admin_password_b64="$(kubectl -n "${GITEA_NS}" get secret gitea-admin -o jsonpath='{.data.GITEA_ADMIN_PASSWORD}')"
admin_email_b64="$(kubectl -n "${GITEA_NS}" get secret gitea-admin -o jsonpath='{.data.GITEA_ADMIN_EMAIL}')"

admin_user="$(printf '%s' "${admin_user_b64}" | base64 -d)"
admin_password="$(printf '%s' "${admin_password_b64}" | base64 -d)"
admin_email="$(printf '%s' "${admin_email_b64}" | base64 -d)"

pod="$(kubectl -n "${GITEA_NS}" get pods -l app.kubernetes.io/name=gitea -o jsonpath='{.items[0].metadata.name}')"
[[ -n "${pod}" ]] || fail "unable to find gitea pod"

gitea_admin_cmd() {
  kubectl -n "${GITEA_NS}" exec "${pod}" -- sh -c "su git -s /bin/sh -c \"$1\""
}

if ! gitea_admin_cmd "gitea admin user list | grep -q '${admin_user}'"; then
  gitea_admin_cmd "gitea admin user create --username '${admin_user}' --password '${admin_password}' --email '${admin_email}' --admin --must-change-password=false" >/dev/null
fi
gitea_admin_cmd "gitea admin user change-password --username '${admin_user}' --password '${admin_password}'" >/dev/null
gitea_admin_cmd "gitea admin user must-change-password --all --unset" >/dev/null
pass "gitea admin user is ready"

kubectl apply -f deploy/k8s/addons/gitea/argocd-repo-secret.yaml >/dev/null
kubectl apply -f deploy/k8s/addons/gitea/argocd-application.yaml >/dev/null
pass "argo cd demo repository and application are configured"

repo_local_url="http://127.0.0.1:13000/${admin_user}/demo-app.git"
gitea_pf_pid=""
if ! curl -fsS http://127.0.0.1:13000/ >/dev/null 2>&1; then
  kubectl -n "${GITEA_NS}" port-forward svc/gitea-http 13000:3000 >/tmp/evidra_demo_gitea_pf.log 2>&1 &
  gitea_pf_pid=$!
  trap '[[ -n "${gitea_pf_pid}" ]] && kill "${gitea_pf_pid}" >/dev/null 2>&1 || true' EXIT
  sleep 2
fi

repo_status="$(curl -sS -o /tmp/evidra_demo_repo_check.json -w '%{http_code}' -u "${admin_user}:${admin_password}" "http://127.0.0.1:13000/api/v1/repos/${admin_user}/demo-app")"
if [[ "${repo_status}" == "404" ]]; then
  create_payload="$(jq -nc '{name:"demo-app",private:true,auto_init:false}')"
  create_status="$(curl -sS -o /tmp/evidra_demo_repo_create.json -w '%{http_code}' -u "${admin_user}:${admin_password}" -H 'Content-Type: application/json' -d "${create_payload}" http://127.0.0.1:13000/api/v1/user/repos)"
  [[ "${create_status}" == "201" ]] || fail "failed to create demo-app repository via Gitea API (status=${create_status})"
elif [[ "${repo_status}" != "200" ]]; then
  fail "failed to query demo-app repository via Gitea API (status=${repo_status})"
fi
pass "demo repository is ready"

go run ./cmd/evidra-demo seed-commit --repo-url "${repo_local_url}" --username "${admin_user}" --password "${admin_password}" --case A >/dev/null
if [[ -n "${gitea_pf_pid}" ]]; then
  kill "${gitea_pf_pid}" >/dev/null 2>&1 || true
  trap - EXIT
fi
pass "demo repository seeded with commit A"

echo "Gitea URL: http://localhost:13000 (run: kubectl -n ${GITEA_NS} port-forward svc/gitea-http 13000:3000)"
echo "Gitea credentials: ${admin_user} / ${admin_password}"
echo "Demo Application: ${ARGOCD_NS}/${APP_NAME}"
