#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAND_DIR="${STAND_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/argocd/install.sh

Install or upgrade ArgoCD into the stand cluster via Helm.
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands helm kubectl
create_or_update_namespace argocd

helm repo add argo https://argoproj.github.io/argo-helm >/dev/null 2>&1 || true
helm repo update >/dev/null
helm upgrade --install argocd argo/argo-cd \
  --namespace argocd \
  --create-namespace \
  -f "$STAND_DIR/argocd/values.yaml" \
  --wait \
  --timeout 10m >/dev/null
