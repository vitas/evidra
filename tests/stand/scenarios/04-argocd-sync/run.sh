#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_ID="04-argocd-sync"
STAND_DIR="${STAND_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
SCENARIO_DIR="${SCENARIO_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/scenarios/04-argocd-sync/run.sh

Force an ArgoCD sync for the stand Application, wait for mapped webhook
evidence in evidra-api, and assert prescribe/report counts plus success verdict.
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands argocd kubectl curl jq
[[ -n "${EVIDRA_TENANT_API_KEY:-}" ]] || die "EVIDRA_TENANT_API_KEY is required"

RESULTS_DIR="${RESULTS_DIR:-$STAND_RESULTS_DIR}"
SCENARIO_TIMESTAMP="${SCENARIO_TIMESTAMP:-$(timestamp_utc)}"
OUTPUT_DIR="$RESULTS_DIR/$SCENARIO_ID/$SCENARIO_TIMESTAMP"
CHECKS_FILE=
STARTED_AT="$(date +%s)"

mkdir -p "$OUTPUT_DIR"
CHECKS_FILE="$(make_checks_file "$OUTPUT_DIR")"

apply_notifications_config
kubectl apply -f "$STAND_DIR/workloads/nginx-dev-application.yaml" >/dev/null
argocd_login
argocd app sync nginx-dev --force --grpc-web >/dev/null
argocd app wait nginx-dev --sync --health --timeout 180 --grpc-web >/dev/null

revision="$(
  argocd app get nginx-dev -o json | jq -r '.status.operationState.operation.sync.revision // .status.sync.revision // empty'
)"
printf '%s\n' "$revision" >"$OUTPUT_DIR/operation-id.txt"
[[ -n "$revision" ]] || die "failed to resolve ArgoCD revision"

wait_for_api_entries 1 prescribe "$revision"
wait_for_api_entries 1 report "$revision"

prescribe_count="$(api_entry_total prescribe "$revision")"
report_count="$(api_entry_total report "$revision")"
report_verdict="$(api_report_verdict "$revision")"

prescribe_pass=false
[[ "$prescribe_count" == "1" ]] && prescribe_pass=true
append_check "$CHECKS_FILE" "prescribe_count" '1' "$prescribe_count" "$prescribe_pass"

report_pass=false
[[ "$report_count" == "1" ]] && report_pass=true
append_check "$CHECKS_FILE" "report_count" '1' "$report_count" "$report_pass"

verdict_pass=false
[[ "$report_verdict" == "success" ]] && verdict_pass=true
append_check "$CHECKS_FILE" "verdict" '"success"' "$(json_string "$report_verdict")" "$verdict_pass"

operation_pass=false
[[ -n "$revision" ]] && operation_pass=true
append_check "$CHECKS_FILE" "operation_id" '"non-empty"' "$(json_string "$revision")" "$operation_pass"

write_result "$SCENARIO_ID" "$OUTPUT_DIR" "$STARTED_AT" "$CHECKS_FILE"
jq -e '.passed == true' "$OUTPUT_DIR/result.json" >/dev/null
