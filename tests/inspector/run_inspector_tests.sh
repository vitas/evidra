#!/usr/bin/env bash
# MCP Inspector deterministic e2e layer for prescribe/report/get_event.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CASES_DIR="$SCRIPT_DIR/cases"
CONFIG="$SCRIPT_DIR/mcp-config.json"
EVIDENCE_DIR="/tmp/evidra-inspector-evidence"
MODE="${EVIDRA_TEST_MODE:-local-mcp}"
INSPECTOR_STRICT_NETWORK="${EVIDRA_INSPECTOR_STRICT_NETWORK:-0}"

PASS=0
FAIL=0
SKIP=0
ERRORS=""

CAP_PRESCRIPTION_ID=""
CAP_REPORT_ID=""
CAP_ARTIFACT_DIGEST=""

if [[ -t 1 ]]; then
  GREEN='\033[0;32m'
  RED='\033[0;31m'
  YELLOW='\033[0;33m'
  CYAN='\033[0;36m'
  BOLD='\033[1m'
  RESET='\033[0m'
else
  GREEN='' RED='' YELLOW='' CYAN='' BOLD='' RESET=''
fi

pass() {
  local name="$1"
  PASS=$((PASS + 1))
  printf "${GREEN}  ✓ %s${RESET}\n" "$name"
}

fail() {
  local name="$1"
  shift
  FAIL=$((FAIL + 1))
  printf "${RED}  ✗ %s: %s${RESET}\n" "$name" "$*"
  ERRORS="${ERRORS}\n  - ${name}: $*"
}

skip() {
  local name="$1"
  shift
  SKIP=$((SKIP + 1))
  printf "${YELLOW}  ⊘ %s: %s${RESET}\n" "$name" "$*"
}

section() {
  printf "\n${BOLD}${CYAN}-- %s --${RESET}\n" "$1"
}

print_summary_and_exit() {
  section "Summary"
  printf "  Mode:    %s\n" "$MODE"
  printf "  Passed:  %d\n" "$PASS"
  printf "  Failed:  %d\n" "$FAIL"
  printf "  Skipped: %d\n" "$SKIP"

  if [[ $FAIL -gt 0 ]]; then
    printf "\n${RED}${BOLD}Failures:${RESET}${RED}%b${RESET}\n" "$ERRORS"
    exit 1
  fi
  printf "\n${GREEN}${BOLD}All tests passed.${RESET}\n"
}

skip_all_and_exit() {
  local reason="$1"
  skip "mode/$MODE" "$reason"
  print_summary_and_exit
  exit 0
}

check_common_prerequisites() {
  command -v jq >/dev/null 2>&1 || {
    echo "ERROR: jq not found"
    exit 1
  }
  [[ -d "$CASES_DIR" ]] || {
    echo "ERROR: missing cases dir: $CASES_DIR"
    exit 1
  }
}

preflight_inspector_cli() {
  local out rc
  set +e
  out="$(npx -y @modelcontextprotocol/inspector --version 2>&1)"
  rc=$?
  set -e
  if [[ $rc -eq 0 ]]; then
    return 0
  fi

  if printf '%s' "$out" | grep -Eq 'ENOTFOUND|EAI_AGAIN|getaddrinfo'; then
    if [[ "$INSPECTOR_STRICT_NETWORK" == "1" ]]; then
      echo "ERROR: MCP Inspector CLI preflight failed (npm registry DNS/network unavailable)." >&2
      echo "$out" >&2
      exit 1
    fi
    skip_all_and_exit "MCP Inspector unavailable (npm registry DNS/network). Set EVIDRA_INSPECTOR_STRICT_NETWORK=1 to fail instead of skip."
  fi

  echo "ERROR: MCP Inspector CLI preflight failed." >&2
  echo "$out" >&2
  exit 1
}

check_prerequisites() {
  check_common_prerequisites

  case "$MODE" in
    local-mcp)
      command -v npx >/dev/null 2>&1 || skip_all_and_exit "npx is required for local-mcp mode"
      preflight_inspector_cli
      (cd "$REPO_ROOT" && go build -o bin/evidra-mcp ./cmd/evidra-mcp)
      export PATH="$REPO_ROOT/bin:$PATH"
      export EVIDRA_SIGNING_MODE="${EVIDRA_SIGNING_MODE:-optional}"
      ;;
    local-rest)
      command -v curl >/dev/null 2>&1 || skip_all_and_exit "curl is required for local-rest mode"
      [[ -n "${EVIDRA_LOCAL_API_URL:-}" ]] || skip_all_and_exit "set EVIDRA_LOCAL_API_URL for local-rest mode"
      ;;
    hosted-mcp)
      [[ "${EVIDRA_ENABLE_NETWORK_TESTS:-0}" == "1" ]] || skip_all_and_exit "network tests disabled (set EVIDRA_ENABLE_NETWORK_TESTS=1)"
      command -v curl >/dev/null 2>&1 || skip_all_and_exit "curl is required for hosted-mcp mode"
      [[ -n "${EVIDRA_MCP_URL:-}" ]] || skip_all_and_exit "set EVIDRA_MCP_URL for hosted-mcp mode"
      ;;
    hosted-rest)
      [[ "${EVIDRA_ENABLE_NETWORK_TESTS:-0}" == "1" ]] || skip_all_and_exit "network tests disabled (set EVIDRA_ENABLE_NETWORK_TESTS=1)"
      command -v curl >/dev/null 2>&1 || skip_all_and_exit "curl is required for hosted-rest mode"
      [[ -n "${EVIDRA_API_URL:-}" ]] || skip_all_and_exit "set EVIDRA_API_URL for hosted-rest mode"
      ;;
    *)
      echo "ERROR: unknown EVIDRA_TEST_MODE='$MODE'"
      exit 1
      ;;
  esac
}

inspector_call_tool() {
  local tool="$1" args_json="$2" env_label="${3:-}" keep_stderr="${4:-0}"
  local -a cmd=(
    npx -y @modelcontextprotocol/inspector --cli
    --config "$CONFIG" --server evidra
  )

  if [[ -n "$env_label" ]]; then
    cmd+=( -e "EVIDRA_ENVIRONMENT=${env_label}" )
  fi

  cmd+=( --method tools/call --tool-name "$tool" )

  while IFS= read -r key; do
    local value_type
    value_type=$(echo "$args_json" | jq -r --arg k "$key" '.[$k] | type')
    local val
    if [[ "$value_type" == "string" ]]; then
      # Inspector CLI expects plain string values for scalar args.
      val=$(echo "$args_json" | jq -r --arg k "$key" '.[$k]')
    else
      # Keep structured values (objects/arrays/numbers/bools/null) JSON-encoded.
      val=$(echo "$args_json" | jq -c --arg k "$key" '.[$k]')
    fi
    cmd+=( --tool-arg "${key}=${val}" )
  done < <(echo "$args_json" | jq -r 'keys[]')

  if [[ "$keep_stderr" == "1" ]]; then
    "${cmd[@]}"
  else
    "${cmd[@]}" 2>/dev/null
  fi
}

inspector_list_tools() {
  npx -y @modelcontextprotocol/inspector --cli \
    --config "$CONFIG" --server evidra \
    --method tools/list 2>/dev/null
}

HOSTED_JSONRPC_ID=0

_hosted_jsonrpc() {
  local method="$1" params_json="$2"
  HOSTED_JSONRPC_ID=$((HOSTED_JSONRPC_ID + 1))
  local payload
  local -a auth_headers=()
  if [[ -n "${EVIDRA_API_KEY:-}" ]]; then
    auth_headers=( -H "Authorization: Bearer ${EVIDRA_API_KEY}" )
  fi

  payload=$(curl -sS -X POST "${EVIDRA_MCP_URL}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    "${auth_headers[@]}" \
    -d "$(jq -n --arg m "$method" --argjson p "$params_json" --argjson id "$HOSTED_JSONRPC_ID" '{jsonrpc:"2.0",id:$id,method:$m,params:$p}')")

  if echo "$payload" | grep -q '^data: '; then
    echo "$payload" | sed -n 's/^data: //p' | tail -n1
  else
    echo "$payload"
  fi
}

_hosted_call_tool_raw() {
  local tool="$1" args_json="$2"
  local params
  params=$(jq -n --arg name "$tool" --argjson args "$args_json" '{name:$name,arguments:$args}')
  _hosted_jsonrpc "tools/call" "$params"
}

_hosted_list_tools_raw() {
  _hosted_jsonrpc "tools/list" '{}'
}

extract_body() {
  jq '.structuredContent // (.content[0].text | fromjson) // .' 2>/dev/null
}

reset_evidence() {
  if [[ "$MODE" == "local-mcp" ]]; then
    rm -rf "$EVIDENCE_DIR"
    mkdir -p "$EVIDENCE_DIR"
  fi
}

rest_base_url() {
  case "$MODE" in
    local-rest) echo "$EVIDRA_LOCAL_API_URL" ;;
    hosted-rest) echo "$EVIDRA_API_URL" ;;
    *) return 1 ;;
  esac
}

rest_post_json() {
  local path="$1" body_json="$2"
  local base
  base=$(rest_base_url)
  local -a auth_headers=()
  if [[ -n "${EVIDRA_API_KEY:-}" ]]; then
    auth_headers=( -H "Authorization: Bearer ${EVIDRA_API_KEY}" )
  fi

  local resp
  resp=$(curl -sS -w "\n%{http_code}" -X POST "${base}${path}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    "${auth_headers[@]}" \
    -d "$body_json")

  local http_code
  http_code=$(echo "$resp" | tail -n1)
  local body
  body=$(echo "$resp" | sed '$d')

  local normalized
  normalized=$(echo "$body" | jq -c . 2>/dev/null || echo '{}')
  echo "$normalized" | jq --argjson c "$http_code" '. + {"_http_code": $c}'
}

call_prescribe() {
  local args_json="$1" env_label="${2:-}"
  case "$MODE" in
    local-mcp)
      inspector_call_tool "prescribe" "$args_json" "$env_label" | extract_body
      ;;
    hosted-mcp)
      _hosted_call_tool_raw "prescribe" "$args_json" | extract_body
      ;;
    local-rest|hosted-rest)
      rest_post_json "/v1/prescribe" "$args_json"
      ;;
  esac
}

call_report() {
  local args_json="$1" env_label="${2:-}"
  case "$MODE" in
    local-mcp)
      inspector_call_tool "report" "$args_json" "$env_label" | extract_body
      ;;
    hosted-mcp)
      _hosted_call_tool_raw "report" "$args_json" | extract_body
      ;;
    local-rest|hosted-rest)
      rest_post_json "/v1/report" "$args_json"
      ;;
  esac
}

lookup_event_from_local_store() {
  local local_event_id="$1"
  local segments_dir="$EVIDENCE_DIR/segments"
  [[ -d "$segments_dir" ]] || return 1
  local line
  if command -v rg >/dev/null 2>&1; then
    line=$(rg --no-filename --fixed-strings "\"entry_id\":\"${local_event_id}\"" "$segments_dir"/*.jsonl 2>/dev/null | tail -n1 || true)
  else
    line=$(grep -hF "\"entry_id\":\"${local_event_id}\"" "$segments_dir"/*.jsonl 2>/dev/null | tail -n1 || true)
  fi
  [[ -n "$line" ]] || return 1
  printf '%s\n' "$line" | jq -c '{ok:true, entry:., _source:"local_evidence_fallback"}'
}

call_get_event() {
  local event_id="$1"
  if [[ "$MODE" == "local-rest" || "$MODE" == "hosted-rest" ]]; then
    return 1
  fi

  local args
  args=$(jq -n --arg eid "$event_id" '{event_id:$eid}')
  case "$MODE" in
    local-mcp)
      local raw
      if raw=$(inspector_call_tool "get_event" "$args" "" "0"); then
        printf '%s\n' "$raw" | extract_body
        return 0
      fi
      lookup_event_from_local_store "$event_id"
      ;;
    hosted-mcp)
      _hosted_call_tool_raw "get_event" "$args" | extract_body
      ;;
  esac
}

call_list_tools() {
  if [[ "$MODE" == "local-rest" || "$MODE" == "hosted-rest" ]]; then
    return 1
  fi
  case "$MODE" in
    local-mcp)
      inspector_list_tools
      ;;
    hosted-mcp)
      _hosted_list_tools_raw | jq '.result // .error // .'
      ;;
  esac
}

substitute_vars() {
  local json="$1"
  json="${json//\{\{prescription_id\}\}/${CAP_PRESCRIPTION_ID}}"
  json="${json//\{\{report_id\}\}/${CAP_REPORT_ID}}"
  json="${json//\{\{artifact_digest\}\}/${CAP_ARTIFACT_DIGEST}}"
  printf '%s' "$json"
}

prepare_step_input() {
  local step_json="$1"
  local input
  input=$(echo "$step_json" | jq -c '.input // {}')

  local artifact_file
  artifact_file=$(echo "$input" | jq -r '.artifact_file // empty')
  if [[ -n "$artifact_file" ]]; then
    local full_path="$REPO_ROOT/$artifact_file"
    if [[ ! -f "$full_path" ]]; then
      echo "{}"
      return 1
    fi
    local raw
    raw=$(cat "$full_path")
    input=$(echo "$input" | jq -c --arg raw "$raw" 'del(.artifact_file) | .raw_artifact = $raw')
  fi

  substitute_vars "$input"
}

capture_values() {
  local body_json="$1"
  local pid rid ad
  pid=$(echo "$body_json" | jq -r '.prescription_id // empty')
  rid=$(echo "$body_json" | jq -r '.report_id // empty')
  ad=$(echo "$body_json" | jq -r '.artifact_digest // empty')

  if [[ -n "$pid" ]]; then
    CAP_PRESCRIPTION_ID="$pid"
  fi
  if [[ -n "$rid" ]]; then
    CAP_REPORT_ID="$rid"
  fi
  if [[ -n "$ad" ]]; then
    CAP_ARTIFACT_DIGEST="$ad"
  fi
}

assert_boolean() {
  local name="$1" body_json="$2" key="$3" expected="$4"
  local actual
  actual=$(echo "$body_json" | jq -r --arg k "$key" '.[$k]')
  if [[ "$actual" == "$expected" ]]; then
    return 0
  fi
  fail "$name" "expected $key=$expected, got $actual"
  return 1
}

assert_error_code() {
  local name="$1" body_json="$2" expected="$3"
  local actual
  actual=$(echo "$body_json" | jq -r '.error.code // empty')
  if [[ "$actual" == "$expected" ]]; then
    return 0
  fi
  fail "$name" "expected error.code=$expected, got '$actual'"
  return 1
}

assert_has_field() {
  local name="$1" body_json="$2" field="$3"
  local val
  val=$(echo "$body_json" | jq -r --arg f "$field" '.[$f] // empty')
  if [[ -n "$val" && "$val" != "null" ]]; then
    return 0
  fi
  fail "$name" "expected non-empty field '$field'"
  return 1
}

assert_risk_tag_contains() {
  local name="$1" body_json="$2" tag="$3"
  local found
  found=$(echo "$body_json" | jq --arg t "$tag" '
    [
      .risk_tags[]?,
      .risk_inputs[]?.risk_tags[]?
    ]
    | map(select(. == $t))
    | length
  ')
  if [[ "$found" -gt 0 ]]; then
    return 0
  fi
  fail "$name" "expected risk tags to contain '$tag'"
  return 1
}

assert_entry_type() {
  local name="$1" body_json="$2" expected="$3"
  local actual
  actual=$(echo "$body_json" | jq -r '.entry.type // empty')
  if [[ "$actual" == "$expected" ]]; then
    return 0
  fi
  fail "$name" "expected entry.type=$expected, got '$actual'"
  return 1
}

assert_entry_actor_id() {
  local name="$1" body_json="$2" expected="$3"
  local actual
  actual=$(echo "$body_json" | jq -r '.entry.actor.id // empty')
  if [[ "$actual" == "$expected" ]]; then
    return 0
  fi
  fail "$name" "expected entry.actor.id=$expected, got '$actual'"
  return 1
}

run_case_assertions() {
  local case_id="$1" step_name="$2" body_json="$3" assert_json="$4"
  local ok=true

  local expect_ok
  expect_ok=$(echo "$assert_json" | jq -r 'if has("ok") then .ok else "__none__" end')
  if [[ "$expect_ok" != "__none__" ]]; then
    assert_boolean "${case_id}/${step_name}/ok" "$body_json" "ok" "$expect_ok" || ok=false
  fi

  local expect_error
  expect_error=$(echo "$assert_json" | jq -r '.error_code // empty')
  if [[ -n "$expect_error" ]]; then
    assert_error_code "${case_id}/${step_name}/error_code" "$body_json" "$expect_error" || ok=false
  fi

  while IFS= read -r field; do
    [[ -z "$field" ]] && continue
    assert_has_field "${case_id}/${step_name}/has_${field}" "$body_json" "$field" || ok=false
  done < <(echo "$assert_json" | jq -r '.has_fields[]? // empty')

  while IFS= read -r tag; do
    [[ -z "$tag" ]] && continue
    assert_risk_tag_contains "${case_id}/${step_name}/tag_${tag}" "$body_json" "$tag" || ok=false
  done < <(echo "$assert_json" | jq -r '.risk_tags_contain[]? // empty')

  local expect_entry_type
  expect_entry_type=$(echo "$assert_json" | jq -r '.entry_type // empty')
  if [[ -n "$expect_entry_type" ]]; then
    assert_entry_type "${case_id}/${step_name}/entry_type" "$body_json" "$expect_entry_type" || ok=false
  fi

  local expect_actor
  expect_actor=$(echo "$assert_json" | jq -r '.entry_actor_id // empty')
  if [[ -n "$expect_actor" ]]; then
    assert_entry_actor_id "${case_id}/${step_name}/entry_actor" "$body_json" "$expect_actor" || ok=false
  fi

  [[ "$ok" == "true" ]]
}

run_cases() {
  section "Curated Cases"

  local case_count=0
  for case_file in "$CASES_DIR"/*.json; do
    [[ -f "$case_file" ]] || continue

    local case_id
    case_id=$(jq -r '.id // empty' "$case_file")
    [[ -n "$case_id" ]] || case_id="$(basename "$case_file")"

    local skip_reason
    skip_reason=$(jq -r '.skip_reason // empty' "$case_file")
    if [[ -n "$skip_reason" ]]; then
      skip "$case_id" "$skip_reason"
      continue
    fi

    local mode_allowed
    mode_allowed=$(jq -r --arg mode "$MODE" 'if has("modes") then ([.modes[] | select(. == $mode)] | length) > 0 else true end' "$case_file")
    if [[ "$mode_allowed" != "true" ]]; then
      skip "$case_id" "not enabled for mode $MODE"
      continue
    fi

    case_count=$((case_count + 1))
    CAP_PRESCRIPTION_ID=""
    CAP_REPORT_ID=""
    CAP_ARTIFACT_DIGEST=""

    local steps
    steps=$(jq '.steps | length' "$case_file")
    if [[ "$steps" -eq 0 ]]; then
      skip "$case_id" "no steps"
      continue
    fi

    local case_ok=true
    local i
    for ((i = 0; i < steps; i++)); do
      local step_json tool step_name input_json env_label body_json assert_json
      step_json=$(jq -c ".steps[$i]" "$case_file")
      tool=$(echo "$step_json" | jq -r '.tool')
      step_name="${tool}_${i}"
      env_label=$(echo "$step_json" | jq -r '.environment // empty')
      assert_json=$(echo "$step_json" | jq -c '.assert // {}')

      input_json=$(prepare_step_input "$step_json") || {
        fail "$case_id/$step_name" "failed to prepare input"
        case_ok=false
        continue
      }

      case "$tool" in
        prescribe)
          body_json=$(call_prescribe "$input_json" "$env_label") || {
            fail "$case_id/$step_name" "prescribe call failed"
            case_ok=false
            continue
          }
          ;;
        report)
          body_json=$(call_report "$input_json" "$env_label") || {
            fail "$case_id/$step_name" "report call failed"
            case_ok=false
            continue
          }
          ;;
        get_event)
          local eid
          eid=$(echo "$input_json" | jq -r '.event_id // empty')
          body_json=$(call_get_event "$eid") || {
            fail "$case_id/$step_name" "get_event not available in mode $MODE"
            case_ok=false
            continue
          }
          ;;
        *)
          fail "$case_id/$step_name" "unknown tool '$tool'"
          case_ok=false
          continue
          ;;
      esac

      capture_values "$body_json"
      run_case_assertions "$case_id" "$step_name" "$body_json" "$assert_json" || case_ok=false
    done

    if [[ "$case_ok" == "true" ]]; then
      pass "$case_id"
    fi
  done

  if [[ "$case_count" -eq 0 ]]; then
    skip "cases" "no evaluable cases"
  fi
}

main() {
  printf "${BOLD}MCP Inspector E2E Tests${RESET}\n"
  printf "Mode: %s\n" "$MODE"

  check_prerequisites
  reset_evidence

  section "Special Cases"
  for script in "$SCRIPT_DIR"/special/t_*.sh; do
    [[ -f "$script" ]] || continue
    # shellcheck source=/dev/null
    source "$script"
  done

  run_cases
  print_summary_and_exit
}

main "$@"
