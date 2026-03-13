#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_ID="02-retry-loop"
STAND_DIR="${STAND_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
SCENARIO_DIR="${SCENARIO_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/scenarios/02-retry-loop/run.sh

Run the same broken kubectl apply four times through Evidra and assert that the
retry_loop signal fires while local evidence remains internally valid.
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands evidra kubectl jq

RESULTS_DIR="${RESULTS_DIR:-$STAND_RESULTS_DIR}"
SCENARIO_TIMESTAMP="${SCENARIO_TIMESTAMP:-$(timestamp_utc)}"
OUTPUT_DIR="$RESULTS_DIR/$SCENARIO_ID/$SCENARIO_TIMESTAMP"
EVIDENCE_DIR="${EVIDRA_EVIDENCE_DIR:-$OUTPUT_DIR/evidence}"
CHECKS_FILE=
STARTED_AT="$(date +%s)"
ACTOR="stand-retry-loop"
SESSION_ID="${SCENARIO_ID}-${SCENARIO_TIMESTAMP}"
ONLINE_ARGS=()

mkdir -p "$OUTPUT_DIR" "$EVIDENCE_DIR"
CHECKS_FILE="$(make_checks_file "$OUTPUT_DIR")"

if [[ "${STAND_ONLINE:-0}" == "1" ]]; then
  [[ -n "${EVIDRA_TENANT_API_KEY:-}" ]] || die "STAND_ONLINE requires EVIDRA_TENANT_API_KEY"
  ONLINE_ARGS=(--url "$EVIDRA_API_BASE" --api-key "$EVIDRA_TENANT_API_KEY")
fi

failure_attempts=0
last_record_json='{}'
for attempt in 1 2 3 4; do
  set +e
  record_json="$(
    evidra record \
      --artifact "$STAND_DIR/workloads/broken-deploy.yaml" \
      --environment development \
      --actor "$ACTOR" \
      --session-id "$SESSION_ID" \
      --attempt "$attempt" \
      --evidence-dir "$EVIDENCE_DIR" \
      --signing-mode optional \
      "${ONLINE_ARGS[@]}" \
      -- kubectl apply -f "$STAND_DIR/workloads/broken-deploy.yaml"
  )"
  status=$?
  set -e

  printf '%s\n' "$record_json" >"$OUTPUT_DIR/record-attempt-$attempt.json"
  last_record_json="$record_json"
  if (( status != 0 )); then
    ((failure_attempts += 1))
  fi
done

explain_json="$(evidra explain --evidence-dir "$EVIDENCE_DIR" --actor "$ACTOR" --session-id "$SESSION_ID")"
printf '%s\n' "$explain_json" >"$OUTPUT_DIR/explain.json"

prescribe_count="$(count_local_entries "$EVIDENCE_DIR" prescribe)"
report_count="$(count_local_entries "$EVIDENCE_DIR" report)"
retry_count="$(printf '%s' "$explain_json" | jq -r '[.signals[] | select(.signal == "retry_loop") | .count][0] // 0')"
verdict="$(printf '%s' "$last_record_json" | jq -r '.verdict')"

validate_pass=true
if ! evidra validate --evidence-dir "$EVIDENCE_DIR" >"$OUTPUT_DIR/validate.txt"; then
  validate_pass=false
fi

prescribe_pass=false
[[ "$prescribe_count" == "4" ]] && prescribe_pass=true
append_check "$CHECKS_FILE" "prescribe_count" '4' "$prescribe_count" "$prescribe_pass"

report_pass=false
[[ "$report_count" == "4" ]] && report_pass=true
append_check "$CHECKS_FILE" "report_count" '4' "$report_count" "$report_pass"

attempts_pass=false
[[ "$failure_attempts" == "4" ]] && attempts_pass=true
append_check "$CHECKS_FILE" "failed_attempts" '4' "$failure_attempts" "$attempts_pass"

verdict_pass=false
[[ "$verdict" == "failure" ]] && verdict_pass=true
append_check "$CHECKS_FILE" "verdict" '"failure"' "$(json_string "$verdict")" "$verdict_pass"

retry_pass=false
if (( retry_count >= 3 )); then
  retry_pass=true
fi
append_check "$CHECKS_FILE" "retry_loop" '3' "$retry_count" "$retry_pass"

append_check "$CHECKS_FILE" "validate" '"pass"' "$(json_string "$([[ "$validate_pass" == true ]] && echo pass || echo fail)")" "$validate_pass"

write_result "$SCENARIO_ID" "$OUTPUT_DIR" "$STARTED_AT" "$CHECKS_FILE"
jq -e '.passed == true' "$OUTPUT_DIR/result.json" >/dev/null
