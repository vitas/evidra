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

run_evidra_json() {
  local output
  if ! output=$(evidra "$@" 2>/dev/null); then
    echo "FATAL: evidra $1 failed"
    return 1
  fi
  if ! echo "$output" | jq -e . >/dev/null 2>&1; then
    echo "FATAL: evidra $1 returned invalid JSON"
    echo "$output"
    return 1
  fi
  printf '%s\n' "$output"
}

prescribe() {
  local tool="$1" op="$2" artifact="$3"
  shift 3
  local output
  output=$(run_evidra_json prescribe \
    --tool "$tool" \
    --operation "$op" \
    --artifact "$artifact" \
    --evidence-dir "$EV_DIR" \
    --session-id "$SESSION_ID" \
    --signing-mode optional \
    "$@")
  LAST_PRESCRIPTION_ID=$(echo "$output" | jq -er '.prescription_id')
  LAST_ARTIFACT_DIGEST=$(echo "$output" | jq -er '.artifact_digest')
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
  local output
  output=$(run_evidra_json "${args[@]}")
  LAST_REPORT_ID=$(echo "$output" | jq -er '.report_id')
}

get_signals() {
  run_evidra_json explain \
    --evidence-dir "$EV_DIR" \
    --session-id "$SESSION_ID" \
    --min-operations "${SCORECARD_MIN_OPERATIONS:-1}" \
    --ttl "$FAULT_TTL"
}

get_score() {
  run_evidra_json scorecard \
    --evidence-dir "$EV_DIR" \
    --session-id "$SESSION_ID" \
    --ttl "$FAULT_TTL" \
    --min-operations "${SCORECARD_MIN_OPERATIONS:-1}"
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

backdate_evidence_entries() {
  local evidence_dir="$1"
  local seconds_ago="${2:-120}"
  local base_epoch
  local offset=0
  base_epoch=$(( $(date +%s) - seconds_ago ))

  while IFS= read -r file; do
    local tmp="${file}.tmp"
    : > "$tmp"
    while IFS= read -r line; do
      local ts epoch
      epoch=$(( base_epoch + offset ))
      ts=$(jq -nr --argjson epoch "$epoch" '$epoch | strftime("%Y-%m-%dT%H:%M:%SZ")')
      echo "$line" | jq -c --arg ts "$ts" '.timestamp = $ts' >> "$tmp"
      offset=$((offset + 1))
    done < "$file"
    mv "$tmp" "$file"
  done < <(find "$evidence_dir" -name "*.jsonl" | sort)
}
