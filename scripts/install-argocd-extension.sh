#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARGOCD_NS="${ARGOCD_NAMESPACE:-argocd}"

EVIDRA_EXTENSION_BASE_URL="${EVIDRA_EXTENSION_BASE_URL:-http://localhost:8080}"
EVIDRA_EXTENSION_API_BASE_URL="${EVIDRA_EXTENSION_API_BASE_URL:-${EVIDRA_EXTENSION_BASE_URL}}"
EVIDRA_EXTENSION_AUTH_MODE="${EVIDRA_EXTENSION_AUTH_MODE:-none}"
EVIDRA_EXTENSION_AUTH_TOKEN="${EVIDRA_EXTENSION_AUTH_TOKEN:-}"
EVIDRA_EXTENSION_TITLE="${EVIDRA_EXTENSION_TITLE:-Evidra}"
EVIDRA_EXTENSION_PATH="${EVIDRA_EXTENSION_PATH:-/evidra-evidence}"
EVIDRA_EXTENSION_ICON="${EVIDRA_EXTENSION_ICON:-fa fa-file-alt}"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"
}

require_cmd kubectl
require_cmd bash

bash "${ROOT_DIR}/scripts/prepare-argocd-extension.sh" >/dev/null

js_quote() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\'/\\\'}"
  printf "'%s'" "$s"
}

cat > "${ROOT_DIR}/tmp/argocd/extensions/evidra/extension-config.js" <<EOF
window.__EVIDRA_EXTENSION_CONFIG__ = {
  evidraBaseURL: $(js_quote "${EVIDRA_EXTENSION_BASE_URL}"),
  apiBaseURL: $(js_quote "${EVIDRA_EXTENSION_API_BASE_URL}"),
  authMode: $(js_quote "${EVIDRA_EXTENSION_AUTH_MODE}"),
  authToken: $(js_quote "${EVIDRA_EXTENSION_AUTH_TOKEN}"),
  title: $(js_quote "${EVIDRA_EXTENSION_TITLE}"),
  path: $(js_quote "${EVIDRA_EXTENSION_PATH}"),
  icon: $(js_quote "${EVIDRA_EXTENSION_ICON}")
};
EOF

kubectl -n "${ARGOCD_NS}" create configmap evidra-extension \
  --from-file=config.js="${ROOT_DIR}/tmp/argocd/extensions/evidra/extension-config.js" \
  --from-file=extension.js="${ROOT_DIR}/tmp/argocd/extensions/evidra/extension-evidra.js" \
  --dry-run=client -o yaml | kubectl apply --server-side=true -f - >/dev/null
pass "extension configmap applied"

kubectl -n "${ARGOCD_NS}" patch deployment argocd-server --type strategic -p '
{
  "spec": {
    "template": {
      "spec": {
        "volumes": [
          {
            "name": "evidra-extension",
            "configMap": { "name": "evidra-extension" }
          }
        ],
        "containers": [
          {
            "name": "argocd-server",
            "volumeMounts": [
              {
                "name": "evidra-extension",
                "mountPath": "/tmp/extensions/evidra"
              }
            ]
          }
        ]
      }
    }
  }
}' >/dev/null

kubectl -n "${ARGOCD_NS}" rollout restart deployment/argocd-server >/dev/null
kubectl -n "${ARGOCD_NS}" rollout status deployment/argocd-server --timeout=420s >/dev/null
pass "argocd-server rollout complete"

echo "Applied extension config:"
echo "  namespace=${ARGOCD_NS}"
echo "  evidraBaseURL=${EVIDRA_EXTENSION_BASE_URL}"
echo "  apiBaseURL=${EVIDRA_EXTENSION_API_BASE_URL}"
echo "  authMode=${EVIDRA_EXTENSION_AUTH_MODE}"
echo "  title=${EVIDRA_EXTENSION_TITLE}"
echo "  path=${EVIDRA_EXTENSION_PATH}"
