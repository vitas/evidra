#!/usr/bin/env bash

STAND_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${STAND_DIR:=$STAND_LIB_DIR}"
: "${REPO_ROOT:=$(cd "$STAND_DIR/../.." && pwd)}"
: "${STAND_RUNTIME_ENV:=$STAND_DIR/runtime.env}"
: "${STAND_LOG_DIR:=$STAND_DIR/logs}"
: "${STAND_RESULTS_DIR:=$STAND_DIR/results}"
: "${STAND_ARGOCD_PORT_FORWARD_PORT:=8088}"
: "${EVIDRA_API_BASE:=http://localhost:8090}"

log() {
  printf '[stand] %s\n' "$*" >&2
}

die() {
  log "ERROR: $*"
  exit 1
}

ensure_dir() {
  mkdir -p "$1"
}

timestamp_utc() {
  date -u +%Y%m%dT%H%M%SZ
}

iso_timestamp_utc() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

load_stand_env() {
  ensure_dir "$STAND_LOG_DIR"
  ensure_dir "$STAND_RESULTS_DIR"
  # shellcheck disable=SC1091
  source "$STAND_DIR/evidra/env.sh"
  if [[ -f "$STAND_RUNTIME_ENV" ]]; then
    # shellcheck disable=SC1091
    source "$STAND_RUNTIME_ENV"
  fi
  export PATH="$REPO_ROOT/bin:$PATH"
}

save_runtime_var() {
  local key="$1"
  local value="$2"
  local tmp

  tmp="$(mktemp)"
  if [[ -f "$STAND_RUNTIME_ENV" ]]; then
    grep -v "^export ${key}=" "$STAND_RUNTIME_ENV" >"$tmp" || true
  fi
  printf 'export %s=%q\n' "$key" "$value" >>"$tmp"
  mv "$tmp" "$STAND_RUNTIME_ENV"
}

require_commands() {
  local missing=()
  local cmd
  for cmd in "$@"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing+=("$cmd")
    fi
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    die "missing required commands: ${missing[*]}"
  fi
}

retry_until() {
  local attempts="$1"
  local delay="$2"
  shift 2

  local i=1
  until "$@"; do
    if (( i >= attempts )); then
      return 1
    fi
    sleep "$delay"
    ((i++))
  done
}

compose() {
  docker compose -f "$STAND_DIR/evidra/docker-compose.yaml" "$@"
}

create_or_update_namespace() {
  kubectl create namespace "$1" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
}

delete_and_recreate_namespace() {
  kubectl delete namespace "$1" --ignore-not-found --wait=true >/dev/null 2>&1 || true
  create_or_update_namespace "$1"
}

render_notifications_config() {
  local destination="$1"
  sed \
    -e "s|__EVIDRA_WEBHOOK_SECRET_ARGOCD__|${EVIDRA_WEBHOOK_SECRET_ARGOCD}|g" \
    -e "s|__EVIDRA_TENANT_API_KEY__|${EVIDRA_TENANT_API_KEY}|g" \
    "$STAND_DIR/argocd/notifications-cm.yaml" >"$destination"
}

apply_notifications_config() {
  local rendered="$STAND_LOG_DIR/argocd-notifications-cm.rendered.yaml"
  render_notifications_config "$rendered"
  kubectl apply -f "$rendered" >/dev/null
}

apply_baseline_workloads() {
  kubectl apply -f "$STAND_DIR/workloads/nginx-dev.yaml" >/dev/null
  kubectl apply -f "$STAND_DIR/workloads/nginx-prod.yaml" >/dev/null
  kubectl apply -f "$STAND_DIR/workloads/nginx-dev-application.yaml" >/dev/null
}

count_local_entries() {
  local evidence_dir="$1"
  local entry_type="$2"
  local segment_dir="$evidence_dir/segments"
  local files=()
  local file

  if [[ ! -d "$segment_dir" ]]; then
    printf '0\n'
    return
  fi

  while IFS= read -r -d '' file; do
    files+=("$file")
  done < <(find "$segment_dir" -type f -name '*.jsonl' -print0 | sort -z)

  if [[ ${#files[@]} -eq 0 ]]; then
    printf '0\n'
    return
  fi

  jq -s --arg entry_type "$entry_type" '[.[] | select(.type == $entry_type)] | length' "${files[@]}"
}

json_string() {
  jq -Rn --arg value "$1" '$value'
}

ensure_signing_key() {
  if [[ -n "${EVIDRA_SIGNING_KEY:-}" ]]; then
    return
  fi

  local output
  local key
  output="$("$REPO_ROOT/bin/evidra" keygen)"
  key="$(printf '%s\n' "$output" | sed -n 's/^EVIDRA_SIGNING_KEY=//p' | head -n1)"
  [[ -n "$key" ]] || die "failed to generate EVIDRA_SIGNING_KEY"

  export EVIDRA_SIGNING_KEY="$key"
  save_runtime_var EVIDRA_SIGNING_KEY "$key"
}

tenant_key_valid() {
  [[ -n "${EVIDRA_TENANT_API_KEY:-}" ]] || return 1
  curl -sf \
    -H "Authorization: Bearer $EVIDRA_TENANT_API_KEY" \
    "$EVIDRA_API_BASE/v1/evidence/entries?limit=1" >/dev/null
}

provision_tenant_key() {
  if tenant_key_valid; then
    log "reusing existing tenant key"
    return
  fi

  local response
  response="$(
    curl -sf \
      -X POST \
      -H "Content-Type: application/json" \
      -H "X-Invite-Secret: $EVIDRA_INVITE_SECRET" \
      -d '{"label":"test-stand"}' \
      "$EVIDRA_API_BASE/v1/keys"
  )"
  EVIDRA_TENANT_API_KEY="$(printf '%s' "$response" | jq -r '.key')"
  [[ -n "$EVIDRA_TENANT_API_KEY" && "$EVIDRA_TENANT_API_KEY" != "null" ]] || die "failed to provision tenant key"
  export EVIDRA_TENANT_API_KEY
  save_runtime_var EVIDRA_TENANT_API_KEY "$EVIDRA_TENANT_API_KEY"
}

clear_webhook_events() {
  compose exec -T postgres psql -U evidra -d evidra -c 'TRUNCATE webhook_events;' >/dev/null
}

api_entry_total() {
  local entry_type="$1"
  local session_id="$2"
  local actor="${3:-}"
  local args=(
    -sfG
    -H "Authorization: Bearer $EVIDRA_TENANT_API_KEY"
    --data-urlencode "type=${entry_type}"
    --data-urlencode "session_id=${session_id}"
    --data-urlencode "limit=100"
  )

  if [[ -n "$actor" ]]; then
    args+=(--data-urlencode "actor=${actor}")
  fi

  curl "${args[@]}" "$EVIDRA_API_BASE/v1/evidence/entries" | jq -r '.total'
}

api_scorecard() {
  local actor="$1"
  curl -sfG \
    -H "Authorization: Bearer $EVIDRA_TENANT_API_KEY" \
    --data-urlencode "actor=${actor}" \
    --data-urlencode "period=30d" \
    "$EVIDRA_API_BASE/v1/evidence/scorecard"
}

api_report_verdict() {
  local session_id="$1"
  curl -sfG \
    -H "Authorization: Bearer $EVIDRA_TENANT_API_KEY" \
    --data-urlencode "type=report" \
    --data-urlencode "session_id=${session_id}" \
    --data-urlencode "limit=1" \
    "$EVIDRA_API_BASE/v1/evidence/entries" | jq -r '.entries[0].verdict // empty'
}

wait_for_api_entries() {
  local expected="$1"
  local entry_type="$2"
  local session_id="$3"
  local actor="${4:-}"
  local attempt
  local actual

  for attempt in $(seq 1 30); do
    actual="$(api_entry_total "$entry_type" "$session_id" "$actor" 2>/dev/null || true)"
    if [[ "$actual" == "$expected" ]]; then
      return 0
    fi
    sleep 2
  done
  return 1
}

start_argocd_port_forward() {
  local pid_file="$STAND_LOG_DIR/argocd-port-forward.pid"
  local pid

  if [[ -f "$pid_file" ]]; then
    pid="$(cat "$pid_file")"
    if kill -0 "$pid" >/dev/null 2>&1; then
      return
    fi
  fi

  kubectl port-forward svc/argocd-server -n argocd "${STAND_ARGOCD_PORT_FORWARD_PORT}:80" \
    >"$STAND_LOG_DIR/argocd-port-forward.log" 2>&1 &
  pid=$!
  printf '%s\n' "$pid" >"$pid_file"
  retry_until 30 2 curl -sf "http://127.0.0.1:${STAND_ARGOCD_PORT_FORWARD_PORT}/healthz" >/dev/null
}

stop_argocd_port_forward() {
  local pid_file="$STAND_LOG_DIR/argocd-port-forward.pid"
  local pid

  if [[ ! -f "$pid_file" ]]; then
    return
  fi
  pid="$(cat "$pid_file")"
  if kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
  fi
  rm -f "$pid_file"
}

argocd_login() {
  local password

  start_argocd_port_forward
  password="$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 --decode)"
  argocd login "127.0.0.1:${STAND_ARGOCD_PORT_FORWARD_PORT}" \
    --grpc-web \
    --insecure \
    --username admin \
    --password "$password" >/dev/null
}

make_checks_file() {
  local output_dir="$1"
  local checks_file="$output_dir/checks.jsonl"
  : >"$checks_file"
  printf '%s\n' "$checks_file"
}

append_check() {
  local checks_file="$1"
  local name="$2"
  local expected_json="$3"
  local actual_json="$4"
  local passed_json="$5"

  jq -nc \
    --arg name "$name" \
    --argjson expected "$expected_json" \
    --argjson actual "$actual_json" \
    --argjson passed "$passed_json" \
    '{name: $name, expected: $expected, actual: $actual, passed: $passed}' >>"$checks_file"
}

write_result() {
  local scenario_id="$1"
  local output_dir="$2"
  local started_epoch="$3"
  local checks_file="$4"
  local checks_json='[]'
  local passed_json

  if [[ -s "$checks_file" ]]; then
    checks_json="$(jq -s '.' "$checks_file")"
  fi
  passed_json="$(printf '%s\n' "$checks_json" | jq 'all(.[]?; .passed == true)')"

  jq -n \
    --arg scenario_id "$scenario_id" \
    --arg timestamp "$(iso_timestamp_utc)" \
    --argjson duration_seconds "$(( $(date +%s) - started_epoch ))" \
    --argjson passed "$passed_json" \
    --argjson checks "$checks_json" \
    '{
      scenario_id: $scenario_id,
      passed: $passed,
      timestamp: $timestamp,
      duration_seconds: $duration_seconds,
      checks: $checks
    }' >"$output_dir/result.json"
}
