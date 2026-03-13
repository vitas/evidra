#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAND_DIR="${STAND_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/teardown.sh

Destroy the stand environment:
  - stop the ArgoCD port-forward if running
  - delete the k3d cluster
  - stop Docker Compose and remove its volume
  - remove stand runtime state, logs, and cached results
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands docker k3d

stop_argocd_port_forward

if k3d cluster list 2>/dev/null | awk 'NR > 1 {print $1}' | grep -qx 'evidra-stand'; then
  k3d cluster delete evidra-stand >/dev/null
fi

compose down -v --remove-orphans >/dev/null 2>&1 || true
rm -f "$STAND_RUNTIME_ENV"
find "$STAND_RESULTS_DIR" -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +
find "$STAND_LOG_DIR" -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +

log "teardown complete"
