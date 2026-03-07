#!/usr/bin/env bash
# helpers.sh — source before running any signal validation scenario
set -euo pipefail

# Dependency check
for cmd in evidra jq; do
  command -v "$cmd" >/dev/null || { echo "FATAL: $cmd required but not found"; exit 1; }
done

export EVIDRA_SIGNING_MODE=optional
FAULT_TTL="${FAULT_TTL:-1s}"

# Workspace — override with WORKSPACE env var
export WORKSPACE="${WORKSPACE:-/tmp/evidra-signal-validation}"
mkdir -p "$WORKSPACE"

new_session() {
  export EV_DIR="$WORKSPACE/evidence-$(date +%s)-$RANDOM"
  mkdir -p "$EV_DIR"
  export SESSION_ID="sig-$(date +%s)-$RANDOM"
  echo "[session] $SESSION_ID → $EV_DIR"
}

prescribe() {
  local tool="$1" op="$2" artifact="$3"
  local output
  output=$(evidra prescribe \
    --tool "$tool" \
    --operation "$op" \
    --artifact "$artifact" \
    --evidence-dir "$EV_DIR" \
    --session-id "$SESSION_ID" \
    --signing-mode optional 2>/dev/null) || true
  LAST_PRESCRIPTION_ID=$(echo "$output" | jq -r '.prescription_id // empty')
  LAST_ARTIFACT_DIGEST=$(echo "$output" | jq -r '.artifact_digest // empty')
  if [ -z "$LAST_PRESCRIPTION_ID" ]; then
    echo "FATAL: prescribe failed"
    echo "$output"
    return 1
  fi
}

report() {
  local prescription_id="$1" exit_code="$2"
  local artifact_digest="${3:-${LAST_ARTIFACT_DIGEST:-}}"
  if [ -z "$prescription_id" ]; then
    echo "FATAL: empty prescription_id"
    return 1
  fi
  local -a args=(
    report
    --prescription "$prescription_id"
    --exit-code "$exit_code"
    --evidence-dir "$EV_DIR"
    --session-id "$SESSION_ID"
    --signing-mode optional
  )
  if [ -n "$artifact_digest" ]; then
    args+=(--artifact-digest "$artifact_digest")
  fi
  evidra "${args[@]}" 2>/dev/null || true
}

get_signals() {
  local output
  output=$(evidra explain \
    --evidence-dir "$EV_DIR" \
    --session-id "$SESSION_ID" \
    --min-operations "${SCORECARD_MIN_OPERATIONS:-1}" \
    --ttl "$FAULT_TTL" 2>/dev/null) || true
  if [ -z "$output" ]; then
    echo '{"signals":[]}'
  else
    echo "$output"
  fi
}

get_score() {
  evidra scorecard \
    --evidence-dir "$EV_DIR" \
    --session-id "$SESSION_ID" \
    --ttl "$FAULT_TTL" \
    --min-operations "${SCORECARD_MIN_OPERATIONS:-1}" 2>/dev/null || echo '{"score":null,"band":"error"}'
}

print_signals() {
  get_signals | jq -r '.signals[] | select(.count > 0) | "  \(.signal): \(.count)"' 2>/dev/null || echo "  (none)"
}

print_score() {
  local sc
  sc=$(get_score)
  local score band
  score=$(echo "$sc" | jq -r '.score // "N/A"')
  band=$(echo "$sc" | jq -r '.band // "N/A"')
  echo "  score=$score band=$band"
}
