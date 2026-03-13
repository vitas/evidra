#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAND_DIR="${STAND_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

KEEP_RESULTS=0

usage() {
  cat <<'EOF'
Usage: tests/stand/reset.sh [--keep-results]

Reset the stand between scenario runs:
  - delete and recreate workload namespaces
  - reapply baseline workloads and the ArgoCD Application
  - clear webhook_events from Postgres
  - optionally clean results/ (default behavior)
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "${1:-}" == "--keep-results" ]]; then
  KEEP_RESULTS=1
elif [[ -n "${1:-}" ]]; then
  usage >&2
  exit 2
fi

load_stand_env
require_commands kubectl docker

delete_and_recreate_namespace workloads-dev
delete_and_recreate_namespace workloads-prod
apply_baseline_workloads
clear_webhook_events

if (( KEEP_RESULTS == 0 )); then
  find "$STAND_RESULTS_DIR" -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +
fi

log "reset complete"
