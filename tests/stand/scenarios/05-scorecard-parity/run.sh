#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_ID="05-scorecard-parity"
STAND_DIR="${STAND_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
SCENARIO_DIR="${SCENARIO_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/scenarios/05-scorecard-parity/run.sh

Generate the same evidence locally and in evidra-api via online-mode CLI flags,
then compare local scorecard fields with the hosted API scorecard for one actor.
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands evidra kubectl curl jq
[[ -n "${EVIDRA_TENANT_API_KEY:-}" ]] || die "EVIDRA_TENANT_API_KEY is required"

RESULTS_DIR="${RESULTS_DIR:-$STAND_RESULTS_DIR}"
SCENARIO_TIMESTAMP="${SCENARIO_TIMESTAMP:-$(timestamp_utc)}"
OUTPUT_DIR="$RESULTS_DIR/$SCENARIO_ID/$SCENARIO_TIMESTAMP"
EVIDENCE_DIR="${EVIDRA_EVIDENCE_DIR:-$OUTPUT_DIR/evidence}"
CHECKS_FILE=
STARTED_AT="$(date +%s)"
ACTOR="stand-scorecard-parity"
ONLINE_ARGS=(--url "$EVIDRA_API_BASE" --api-key "$EVIDRA_TENANT_API_KEY")

mkdir -p "$OUTPUT_DIR" "$EVIDENCE_DIR"
CHECKS_FILE="$(make_checks_file "$OUTPUT_DIR")"

session_happy="parity-happy-${SCENARIO_TIMESTAMP}"
session_retry="parity-retry-${SCENARIO_TIMESTAMP}"
session_declined="parity-declined-${SCENARIO_TIMESTAMP}"

happy_json="$(
  evidra record \
    --artifact "$STAND_DIR/workloads/nginx-dev.yaml" \
    --environment development \
    --actor "$ACTOR" \
    --session-id "$session_happy" \
    --evidence-dir "$EVIDENCE_DIR" \
    --signing-mode optional \
    "${ONLINE_ARGS[@]}" \
    -- kubectl apply -f "$STAND_DIR/workloads/nginx-dev.yaml"
)"
printf '%s\n' "$happy_json" >"$OUTPUT_DIR/parity-happy.json"

for attempt in 1 2 3 4; do
  set +e
  retry_json="$(
    evidra record \
      --artifact "$STAND_DIR/workloads/broken-deploy.yaml" \
      --environment development \
      --actor "$ACTOR" \
      --session-id "$session_retry" \
      --attempt "$attempt" \
      --evidence-dir "$EVIDENCE_DIR" \
      --signing-mode optional \
      "${ONLINE_ARGS[@]}" \
      -- kubectl apply -f "$STAND_DIR/workloads/broken-deploy.yaml"
  )"
  retry_status=$?
  set -e
  printf '%s\n' "$retry_json" >"$OUTPUT_DIR/parity-retry-$attempt.json"
  if (( retry_status == 0 )); then
    die "expected retry-loop attempt $attempt to fail"
  fi
done

prescribe_json="$(
  evidra prescribe \
    --tool kubectl \
    --operation apply \
    --artifact "$STAND_DIR/workloads/risky-deploy.yaml" \
    --environment development \
    --actor "$ACTOR" \
    --session-id "$session_declined" \
    --evidence-dir "$EVIDENCE_DIR" \
    --signing-mode optional \
    "${ONLINE_ARGS[@]}"
)"
printf '%s\n' "$prescribe_json" >"$OUTPUT_DIR/parity-declined-prescribe.json"
prescription_id="$(printf '%s' "$prescribe_json" | jq -r '.prescription_id')"

declined_json="$(
  evidra report \
    --prescription "$prescription_id" \
    --verdict declined \
    --decline-trigger risk_threshold_exceeded \
    --decline-reason "privileged container rejected" \
    --actor "$ACTOR" \
    --session-id "$session_declined" \
    --evidence-dir "$EVIDENCE_DIR" \
    --signing-mode optional \
    "${ONLINE_ARGS[@]}"
)"
printf '%s\n' "$declined_json" >"$OUTPUT_DIR/parity-declined-report.json"

validate_pass=true
if ! evidra validate --evidence-dir "$EVIDENCE_DIR" >"$OUTPUT_DIR/validate.txt"; then
  validate_pass=false
fi

local_scorecard="$(evidra scorecard --evidence-dir "$EVIDENCE_DIR" --actor "$ACTOR")"
api_scorecard_json="$(api_scorecard "$ACTOR")"
printf '%s\n' "$local_scorecard" >"$OUTPUT_DIR/local-scorecard.json"
printf '%s\n' "$api_scorecard_json" >"$OUTPUT_DIR/api-scorecard.json"

local_score="$(printf '%s' "$local_scorecard" | jq -r '.score')"
api_score="$(printf '%s' "$api_scorecard_json" | jq -r '.score')"
local_band="$(printf '%s' "$local_scorecard" | jq -r '.band')"
api_band="$(printf '%s' "$api_scorecard_json" | jq -r '.band')"
local_total="$(printf '%s' "$local_scorecard" | jq -r '.total_operations')"
api_total="$(printf '%s' "$api_scorecard_json" | jq -r '.total_entries')"
local_signals="$(printf '%s' "$local_scorecard" | jq -c '(.signals // {}) | with_entries(select(.value > 0))')"
api_signals="$(printf '%s' "$api_scorecard_json" | jq -c '.signal_summary | map_values(.count) | with_entries(select(.value > 0))')"

score_pass=false
[[ "$local_score" == "$api_score" ]] && score_pass=true
append_check "$CHECKS_FILE" "score" "$local_score" "$api_score" "$score_pass"

band_pass=false
[[ "$local_band" == "$api_band" ]] && band_pass=true
append_check "$CHECKS_FILE" "band" "$(json_string "$local_band")" "$(json_string "$api_band")" "$band_pass"

total_pass=false
[[ "$local_total" == "$api_total" ]] && total_pass=true
append_check "$CHECKS_FILE" "total_operations" "$local_total" "$api_total" "$total_pass"

signals_pass=false
[[ "$local_signals" == "$api_signals" ]] && signals_pass=true
append_check "$CHECKS_FILE" "signal_counts" "$local_signals" "$api_signals" "$signals_pass"

append_check "$CHECKS_FILE" "validate" '"pass"' "$(json_string "$([[ "$validate_pass" == true ]] && echo pass || echo fail)")" "$validate_pass"

write_result "$SCENARIO_ID" "$OUTPUT_DIR" "$STARTED_AT" "$CHECKS_FILE"
jq -e '.passed == true' "$OUTPUT_DIR/result.json" >/dev/null
