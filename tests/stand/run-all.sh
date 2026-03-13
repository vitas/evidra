#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAND_DIR="${STAND_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/run-all.sh [scenario-id ...]

Run all stand scenarios sequentially with isolated evidence directories and
collect per-scenario result.json files plus results/summary.json.
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands bash jq curl kubectl
retry_until 30 2 curl -sf "$EVIDRA_API_BASE/healthz" >/dev/null

if [[ $# -gt 0 ]]; then
  SCENARIOS=("$@")
else
  SCENARIOS=(
    01-happy-path
    02-retry-loop
    03-declined-decision
    04-argocd-sync
    05-scorecard-parity
  )
fi

summary_lines="$(mktemp)"
trap 'rm -f "$summary_lines"' EXIT

passed=0
failed=0
for scenario_id in "${SCENARIOS[@]}"; do
  scenario_ts="$(timestamp_utc)"
  export RESULTS_DIR="$STAND_RESULTS_DIR"
  export SCENARIO_DIR="$STAND_DIR/scenarios/$scenario_id"
  export SCENARIO_TIMESTAMP="$scenario_ts"
  export EVIDRA_EVIDENCE_DIR="$RESULTS_DIR/$scenario_id/$scenario_ts/evidence"

  log "resetting stand for $scenario_id"
  bash "$STAND_DIR/reset.sh" --keep-results

  log "running $scenario_id"
  if bash "$SCENARIO_DIR/run.sh"; then
    ((passed += 1))
  else
    ((failed += 1))
  fi

  if [[ -f "$RESULTS_DIR/$scenario_id/$scenario_ts/result.json" ]]; then
    jq '{scenario_id, passed}' "$RESULTS_DIR/$scenario_id/$scenario_ts/result.json" >>"$summary_lines"
  else
    jq -nc --arg scenario_id "$scenario_id" '{scenario_id: $scenario_id, passed: false}' >>"$summary_lines"
  fi
done

stand_version="$(sed -n 's/^version: "\(.*\)"/\1/p' "$STAND_DIR/stand.yaml" | head -n1)"
evidra_version="$(evidra version | head -n1)"
jq -n \
  --arg stand_version "$stand_version" \
  --arg evidra_version "$evidra_version" \
  --arg timestamp "$(iso_timestamp_utc)" \
  --argjson total "${#SCENARIOS[@]}" \
  --argjson passed "$passed" \
  --argjson failed "$failed" \
  --argjson results "$(jq -s '.' "$summary_lines")" \
  '{
    stand_version: $stand_version,
    evidra_version: $evidra_version,
    timestamp: $timestamp,
    total: $total,
    passed: $passed,
    failed: $failed,
    results: $results
  }' >"$STAND_RESULTS_DIR/summary.json"

if (( failed > 0 )); then
  exit 1
fi
