#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAND_DIR="${STAND_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/setup.sh

Build the Evidra binaries, create the k3d cluster, install ArgoCD, start the
hosted API/Postgres stack, provision a tenant API key, render ArgoCD webhook
notifications, and apply the baseline stand workloads.

Prerequisites:
  make, docker, k3d, kubectl, helm, argocd, curl, jq
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands make docker k3d kubectl helm argocd curl jq

log "building binaries"
(cd "$REPO_ROOT" && make build >/dev/null)
ensure_signing_key

if ! k3d cluster list 2>/dev/null | awk 'NR > 1 {print $1}' | grep -qx 'evidra-stand'; then
  log "creating k3d cluster"
  k3d cluster create --config "$STAND_DIR/cluster/k3d-config.yaml" >/dev/null
fi

create_or_update_namespace argocd
create_or_update_namespace workloads-dev
create_or_update_namespace workloads-prod

log "installing ArgoCD"
bash "$STAND_DIR/argocd/install.sh"

log "starting compose stack"
compose up --build -d >/dev/null
retry_until 60 2 curl -sf "$EVIDRA_API_BASE/healthz" >/dev/null
retry_until 60 2 curl -sf "$EVIDRA_API_BASE/readyz" >/dev/null

log "provisioning tenant key"
provision_tenant_key

log "applying ArgoCD notifications config"
apply_notifications_config

log "applying baseline workloads"
apply_baseline_workloads

log "setup complete"
log "export PATH=\"$REPO_ROOT/bin:\$PATH\""
log "tenant key stored in $STAND_RUNTIME_ENV"
