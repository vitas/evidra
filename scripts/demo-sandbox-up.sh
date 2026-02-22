#!/usr/bin/env bash
set -euo pipefail

ARGOCD_NS="${ARGOCD_NAMESPACE:-argocd}"
GITEA_NS="${EVIDRA_DEMO_GITEA_NAMESPACE:-evidra-demo-gitops}"
APP_NS="${EVIDRA_DEMO_APP_NAMESPACE:-demo}"
APP_NAME="${EVIDRA_DEMO_APP_NAME:-guestbook-demo}"
ARGO_URL="${EVIDRA_DEMO_ARGO_URL:-https://localhost:8081}"
EVIDRA_URL="${EVIDRA_DEMO_EVIDRA_URL:-http://localhost:8080/ui/}"
GITEA_URL="${EVIDRA_DEMO_GITEA_URL:-http://localhost:13000}"

pass() { echo "PASS: $1"; }

make evidra-ui-sync

docker compose up -d --build postgres evidra
pass "local Evidra API is up"

bash scripts/demo-kind-argocd-up.sh
bash scripts/demo-kind-collector-up.sh
bash scripts/demo-gitea-setup.sh

echo
if admin_pw="$(kubectl -n "${ARGOCD_NS}" get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null)"; then
  :
else
  admin_pw=""
fi

if gitea_user_b64="$(kubectl -n "${GITEA_NS}" get secret gitea-admin -o jsonpath='{.data.GITEA_ADMIN_USERNAME}' 2>/dev/null)"; then
  gitea_user="$(printf '%s' "${gitea_user_b64}" | base64 -d 2>/dev/null || true)"
else
  gitea_user=""
fi
if gitea_password_b64="$(kubectl -n "${GITEA_NS}" get secret gitea-admin -o jsonpath='{.data.GITEA_ADMIN_PASSWORD}' 2>/dev/null)"; then
  gitea_password="$(printf '%s' "${gitea_password_b64}" | base64 -d 2>/dev/null || true)"
else
  gitea_password=""
fi

echo "Demo environment is ready."
echo
echo "Gitea:  ${GITEA_URL}   (user: ${gitea_user:-<from-secret>} pass: ${gitea_password:-<from-secret>})"
echo "ArgoCD: ${ARGO_URL}   (user: admin, pass: ${admin_pw:-<run: kubectl -n ${ARGOCD_NS} get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d>})"
echo "Evidra: ${EVIDRA_URL}"
echo
echo "Namespaces:"
echo "- gitea: ${GITEA_NS}"
echo "- argocd: ${ARGOCD_NS}"
echo "- app target: ${APP_NS}"
echo "Argo CD Application: ${APP_NAME}"
echo
echo "Try it manually in 3 steps:"
echo "1) Open Gitea (${GITEA_URL}) -> repo demo-app -> edit a value (for example ConfigMap) -> commit."
echo "2) Open Argo CD (${ARGO_URL}) -> Applications -> ${APP_NAME} -> click Sync."
echo "3) Open Evidra (${EVIDRA_URL}) -> select subject ${APP_NAME} -> click Find changes -> open a failed/degraded Change permalink."
echo
echo "Port-forwards:"
echo "- Argo CD: bash scripts/argocd-port-forward.sh start"
echo "- Gitea:   kubectl -n ${GITEA_NS} port-forward svc/gitea-http 13000:3000"
echo
echo "Cleanup:"
echo "- make evidra-demo-clean"
