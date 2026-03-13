#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_ID="01-happy-path"
STAND_DIR="${STAND_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
SCENARIO_DIR="${SCENARIO_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/scenarios/01-happy-path/run.sh

Run a successful Evidra-wrapped kubectl apply against nginx-dev.yaml and assert
local evidence counts, verdict, risk level, and validate output.
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
ACTOR="stand-happy-path"
SESSION_ID="${SCENARIO_ID}-${SCENARIO_TIMESTAMP}"
ONLINE_ARGS=()

mkdir -p "$OUTPUT_DIR" "$EVIDENCE_DIR"
CHECKS_FILE="$(make_checks_file "$OUTPUT_DIR")"

if [[ "${STAND_ONLINE:-0}" == "1" ]]; then
  [[ -n "${EVIDRA_TENANT_API_KEY:-}" ]] || die "STAND_ONLINE requires EVIDRA_TENANT_API_KEY"
  ONLINE_ARGS=(--url "$EVIDRA_API_BASE" --api-key "$EVIDRA_TENANT_API_KEY")
fi

record_json="$(
  evidra record \
    --artifact "$STAND_DIR/workloads/nginx-dev.yaml" \
    --environment development \
    --actor "$ACTOR" \
    --session-id "$SESSION_ID" \
    --evidence-dir "$EVIDENCE_DIR" \
    --signing-mode optional \
    "${ONLINE_ARGS[@]}" \
    -- kubectl apply -f "$STAND_DIR/workloads/nginx-dev.yaml"
)"
printf '%s\n' "$record_json" >"$OUTPUT_DIR/record.json"

prescribe_count="$(count_local_entries "$EVIDENCE_DIR" prescribe)"
report_count="$(count_local_entries "$EVIDENCE_DIR" report)"
verdict="$(printf '%s' "$record_json" | jq -r '.verdict')"
risk_level="$(printf '%s' "$record_json" | jq -r '.risk_level')"

validate_pass=true
if ! evidra validate --evidence-dir "$EVIDENCE_DIR" >"$OUTPUT_DIR/validate.txt"; then
  validate_pass=false
fi

prescribe_pass=false
[[ "$prescribe_count" == "1" ]] && prescribe_pass=true
append_check "$CHECKS_FILE" "prescribe_count" '1' "$prescribe_count" "$prescribe_pass"

report_pass=false
[[ "$report_count" == "1" ]] && report_pass=true
append_check "$CHECKS_FILE" "report_count" '1' "$report_count" "$report_pass"

verdict_pass=false
[[ "$verdict" == "success" ]] && verdict_pass=true
append_check "$CHECKS_FILE" "verdict" '"success"' "$(json_string "$verdict")" "$verdict_pass"

risk_pass=false
case "$risk_level" in
  low|medium) risk_pass=true ;;
esac
append_check "$CHECKS_FILE" "risk_level" '["low","medium"]' "$(json_string "$risk_level")" "$risk_pass"

append_check "$CHECKS_FILE" "validate" '"pass"' "$(json_string "$([[ "$validate_pass" == true ]] && echo pass || echo fail)")" "$validate_pass"

write_result "$SCENARIO_ID" "$OUTPUT_DIR" "$STARTED_AT" "$CHECKS_FILE"
jq -e '.passed == true' "$OUTPUT_DIR/result.json" >/dev/null
