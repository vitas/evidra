#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_ID="03-declined-decision"
STAND_DIR="${STAND_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
SCENARIO_DIR="${SCENARIO_DIR:-$SCRIPT_DIR}"
# shellcheck disable=SC1091
source "$STAND_DIR/lib.sh"

usage() {
  cat <<'EOF'
Usage: tests/stand/scenarios/03-declined-decision/run.sh

Record a risky prescription, emit a declined report with decision_context, and
assert local evidence counts plus validate output.
EOF
}

if [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

load_stand_env
require_commands evidra jq

RESULTS_DIR="${RESULTS_DIR:-$STAND_RESULTS_DIR}"
SCENARIO_TIMESTAMP="${SCENARIO_TIMESTAMP:-$(timestamp_utc)}"
OUTPUT_DIR="$RESULTS_DIR/$SCENARIO_ID/$SCENARIO_TIMESTAMP"
EVIDENCE_DIR="${EVIDRA_EVIDENCE_DIR:-$OUTPUT_DIR/evidence}"
CHECKS_FILE=
STARTED_AT="$(date +%s)"
ACTOR="stand-declined-decision"
SESSION_ID="${SCENARIO_ID}-${SCENARIO_TIMESTAMP}"
ONLINE_ARGS=()

mkdir -p "$OUTPUT_DIR" "$EVIDENCE_DIR"
CHECKS_FILE="$(make_checks_file "$OUTPUT_DIR")"

if [[ "${STAND_ONLINE:-0}" == "1" ]]; then
  [[ -n "${EVIDRA_TENANT_API_KEY:-}" ]] || die "STAND_ONLINE requires EVIDRA_TENANT_API_KEY"
  ONLINE_ARGS=(--url "$EVIDRA_API_BASE" --api-key "$EVIDRA_TENANT_API_KEY")
fi

prescribe_json="$(
  evidra prescribe \
    --tool kubectl \
    --operation apply \
    --artifact "$STAND_DIR/workloads/risky-deploy.yaml" \
    --environment development \
    --actor "$ACTOR" \
    --session-id "$SESSION_ID" \
    --evidence-dir "$EVIDENCE_DIR" \
    --signing-mode optional \
    "${ONLINE_ARGS[@]}"
)"
printf '%s\n' "$prescribe_json" >"$OUTPUT_DIR/prescribe.json"
prescription_id="$(printf '%s' "$prescribe_json" | jq -r '.prescription_id')"
risk_level="$(printf '%s' "$prescribe_json" | jq -r '.risk_level')"

report_json="$(
  evidra report \
    --prescription "$prescription_id" \
    --verdict declined \
    --decline-trigger risk_threshold_exceeded \
    --decline-reason "privileged container rejected" \
    --actor "$ACTOR" \
    --session-id "$SESSION_ID" \
    --evidence-dir "$EVIDENCE_DIR" \
    --signing-mode optional \
    "${ONLINE_ARGS[@]}"
)"
printf '%s\n' "$report_json" >"$OUTPUT_DIR/report.json"

prescribe_count="$(count_local_entries "$EVIDENCE_DIR" prescribe)"
report_count="$(count_local_entries "$EVIDENCE_DIR" report)"
verdict="$(printf '%s' "$report_json" | jq -r '.verdict')"
decision_trigger="$(printf '%s' "$report_json" | jq -r '.decision_context.trigger // empty')"

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
[[ "$verdict" == "declined" ]] && verdict_pass=true
append_check "$CHECKS_FILE" "verdict" '"declined"' "$(json_string "$verdict")" "$verdict_pass"

risk_pass=false
case "$risk_level" in
  high|critical) risk_pass=true ;;
esac
append_check "$CHECKS_FILE" "risk_level" '["high","critical"]' "$(json_string "$risk_level")" "$risk_pass"

decision_pass=false
[[ "$decision_trigger" == "risk_threshold_exceeded" ]] && decision_pass=true
append_check "$CHECKS_FILE" "decision_context" '"risk_threshold_exceeded"' "$(json_string "$decision_trigger")" "$decision_pass"

append_check "$CHECKS_FILE" "validate" '"pass"' "$(json_string "$([[ "$validate_pass" == true ]] && echo pass || echo fail)")" "$validate_pass"

write_result "$SCENARIO_ID" "$OUTPUT_DIR" "$STARTED_AT" "$CHECKS_FILE"
jq -e '.passed == true' "$OUTPUT_DIR/result.json" >/dev/null
