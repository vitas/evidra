#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_SCENARIOS_DIR="$ROOT_DIR/tests/experiments/execution-scenarios"
DEFAULT_PROMPT_FILE="$ROOT_DIR/prompts/experiments/runtime/system_instructions.txt"

usage() {
  cat <<'USAGE'
Usage:
  scripts/run-agent-execution-experiments.sh [options]

Required:
  --model-id <id>            Model id label (example: claude/sonnet)
  --agent-cmd <command>      Command executed per run via bash -lc

Optional:
  --provider <name>          Provider label (default: unknown)
  --prompt-version <ver>     Prompt version label (default: from prompt file header, else v1)
  --prompt-file <path>       Prompt contract file path (default: prompts/experiments/runtime/system_instructions.txt)
  --scenarios-dir <path>     Scenario JSON directory (default: tests/experiments/execution-scenarios)
  --mode <name>              Transport label (default: local-mcp)
  --repeats <n>              Repeats per scenario (default: 1)
  --timeout-seconds <n>      Per-run timeout in seconds (default: 600)
  --scenario-filter <regex>  Regex filter on scenario_id
  --max-scenarios <n>        Max selected scenarios after filtering
  --out-dir <path>           Output directory (default: experiments/results/<timestamp>-execution)
  --clean-out-dir            Remove existing files in output dir before run
  --dry-run                  Skip agent command and write synthetic output
  -h, --help                 Show help

Expected scenario JSON fields:
  scenario_id, tool, operation, artifact_path, execute_cmd

Agent command contract:
  - read env vars (EVIDRA_EXEC_* and EVIDRA_AGENT_*)
  - write JSON object to EVIDRA_AGENT_OUTPUT
USAGE
}

fail() {
  echo "run-agent-execution-experiments: FAIL $*" >&2
  exit 1
}

warn() {
  echo "run-agent-execution-experiments: WARN $*" >&2
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

clean_out_dir() {
  local out_dir="$1"
  [[ -n "$out_dir" ]] || fail "--clean-out-dir requires non-empty --out-dir"
  [[ "$out_dir" != "/" ]] || fail "--clean-out-dir refuses out-dir '/'"
  [[ "$out_dir" != "." ]] || fail "--clean-out-dir refuses out-dir '.'"
  [[ "$out_dir" != ".." ]] || fail "--clean-out-dir refuses out-dir '..'"
  [[ -d "$out_dir" ]] || return 0

  find "$out_dir" -mindepth 1 -exec rm -rf -- {} +
}

iso_utc_now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

safe_model_id() {
  printf '%s' "$1" | tr '/: ' '---' | tr -cd '[:alnum:]_.-'
}

parse_contract_version() {
  local path="$1"
  [[ -f "$path" ]] || return 1
  awk '
    {
      line=$0
      gsub(/^[ \t]+|[ \t]+$/, "", line)
      if (line == "") next
      if (line ~ /^<!--/ && line ~ /-->$/) {
        sub(/^<!--[ \t]*/, "", line)
        sub(/[ \t]*-->$/, "", line)
      }
      if (line ~ /^#/) {
        sub(/^#[ \t]*/, "", line)
      }
      low=tolower(line)
      if (index(low, "contract:") != 1) {
        exit 1
      }
      val=line
      sub(/^[^:]*:[ \t]*/, "", val)
      if (val == "") exit 1
      print val
      exit 0
    }
    END { if (NR == 0) exit 1 }
  ' "$path"
}

resolve_path() {
  local base_dir="$1" candidate="$2"
  if [[ "$candidate" = /* ]]; then
    printf '%s' "$candidate"
    return 0
  fi
  if [[ -f "$base_dir/$candidate" ]]; then
    printf '%s' "$base_dir/$candidate"
    return 0
  fi
  if [[ -f "$ROOT_DIR/$candidate" ]]; then
    printf '%s' "$ROOT_DIR/$candidate"
    return 0
  fi
  return 1
}

MODEL_ID=""
PROVIDER="unknown"
PROMPT_VERSION=""
PROMPT_VERSION_SET=0
PROMPT_FILE="$DEFAULT_PROMPT_FILE"
PROMPT_CONTRACT_VERSION=""
SCENARIOS_DIR="$DEFAULT_SCENARIOS_DIR"
MODE="local-mcp"
REPEATS=1
TIMEOUT_SECONDS=600
SCENARIO_FILTER=""
MAX_SCENARIOS=0
OUT_DIR=""
AGENT_CMD=""
DRY_RUN=0
CLEAN_OUT_DIR=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --model-id)
      [[ $# -ge 2 ]] || fail "--model-id requires a value"
      MODEL_ID="$2"
      shift 2
      ;;
    --provider)
      [[ $# -ge 2 ]] || fail "--provider requires a value"
      PROVIDER="$2"
      shift 2
      ;;
    --prompt-version)
      [[ $# -ge 2 ]] || fail "--prompt-version requires a value"
      PROMPT_VERSION="$2"
      PROMPT_VERSION_SET=1
      shift 2
      ;;
    --prompt-file)
      [[ $# -ge 2 ]] || fail "--prompt-file requires a value"
      PROMPT_FILE="$2"
      shift 2
      ;;
    --scenarios-dir)
      [[ $# -ge 2 ]] || fail "--scenarios-dir requires a value"
      SCENARIOS_DIR="$2"
      shift 2
      ;;
    --mode)
      [[ $# -ge 2 ]] || fail "--mode requires a value"
      MODE="$2"
      shift 2
      ;;
    --repeats)
      [[ $# -ge 2 ]] || fail "--repeats requires a value"
      REPEATS="$2"
      shift 2
      ;;
    --timeout-seconds)
      [[ $# -ge 2 ]] || fail "--timeout-seconds requires a value"
      TIMEOUT_SECONDS="$2"
      shift 2
      ;;
    --scenario-filter)
      [[ $# -ge 2 ]] || fail "--scenario-filter requires a value"
      SCENARIO_FILTER="$2"
      shift 2
      ;;
    --max-scenarios)
      [[ $# -ge 2 ]] || fail "--max-scenarios requires a value"
      MAX_SCENARIOS="$2"
      shift 2
      ;;
    --out-dir)
      [[ $# -ge 2 ]] || fail "--out-dir requires a value"
      OUT_DIR="$2"
      shift 2
      ;;
    --clean-out-dir)
      CLEAN_OUT_DIR=1
      shift
      ;;
    --agent-cmd)
      [[ $# -ge 2 ]] || fail "--agent-cmd requires a value"
      AGENT_CMD="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

require_cmd jq
[[ -n "$MODEL_ID" ]] || fail "--model-id is required"
[[ -n "$AGENT_CMD" || "$DRY_RUN" -eq 1 ]] || fail "--agent-cmd is required unless --dry-run is set"
[[ -d "$SCENARIOS_DIR" ]] || fail "scenarios dir not found: $SCENARIOS_DIR"

[[ "$REPEATS" =~ ^[0-9]+$ ]] || fail "--repeats must be integer"
(( REPEATS >= 1 )) || fail "--repeats must be >= 1"
[[ "$TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || fail "--timeout-seconds must be integer"
(( TIMEOUT_SECONDS >= 1 )) || fail "--timeout-seconds must be >= 1"
[[ "$MAX_SCENARIOS" =~ ^[0-9]+$ ]] || fail "--max-scenarios must be integer"

if [[ -n "$PROMPT_FILE" ]]; then
  [[ -f "$PROMPT_FILE" ]] || fail "prompt file not found: $PROMPT_FILE"
  if PROMPT_CONTRACT_VERSION="$(parse_contract_version "$PROMPT_FILE" 2>/dev/null)"; then
    :
  else
    PROMPT_CONTRACT_VERSION="unknown"
  fi
fi

if [[ "$PROMPT_VERSION_SET" -eq 0 ]]; then
  if [[ -n "$PROMPT_CONTRACT_VERSION" && "$PROMPT_CONTRACT_VERSION" != "unknown" ]]; then
    PROMPT_VERSION="$PROMPT_CONTRACT_VERSION"
  else
    PROMPT_VERSION="v1"
  fi
fi

run_stamp="$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="$ROOT_DIR/experiments/results/${run_stamp}-execution"
fi
if [[ "$CLEAN_OUT_DIR" -eq 1 ]]; then
  clean_out_dir "$OUT_DIR"
fi
mkdir -p "$OUT_DIR"
SUMMARY_PATH="$OUT_DIR/summary.jsonl"
: > "$SUMMARY_PATH"

selected_scenarios=()
while IFS= read -r scenario_json; do
  scenario_id="$(jq -r '.scenario_id // empty' "$scenario_json")"
  [[ -n "$scenario_id" ]] || {
    warn "skip $scenario_json: missing scenario_id"
    continue
  }

  if [[ -n "$SCENARIO_FILTER" ]]; then
    if ! [[ "$scenario_id" =~ $SCENARIO_FILTER ]]; then
      continue
    fi
  fi

  selected_scenarios+=("$scenario_json")
done < <(find "$SCENARIOS_DIR" -type f -name '*.json' | sort)

if (( MAX_SCENARIOS > 0 && ${#selected_scenarios[@]} > MAX_SCENARIOS )); then
  selected_scenarios=("${selected_scenarios[@]:0:MAX_SCENARIOS}")
fi

(( ${#selected_scenarios[@]} > 0 )) || fail "no scenarios selected"

timeout_bin=""
if command -v timeout >/dev/null 2>&1; then
  timeout_bin="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
  timeout_bin="gtimeout"
else
  warn "timeout binary not found (timeout/gtimeout); runs will not be time-limited"
fi

runs_total=0
runs_success=0
runs_failure=0
runs_timeout=0
runs_dry=0
runs_eval_pass=0
runs_eval_fail=0

echo "run-agent-execution-experiments: selected_scenarios=${#selected_scenarios[@]} repeats=$REPEATS out_dir=$OUT_DIR mode=$MODE"

for scenario_json in "${selected_scenarios[@]}"; do
  scenario_dir="$(dirname "$scenario_json")"
  scenario_id="$(jq -r '.scenario_id' "$scenario_json")"
  category="$(jq -r '.category // "unknown"' "$scenario_json")"
  difficulty="$(jq -r '.difficulty // "unknown"' "$scenario_json")"
  tool="$(jq -r '.tool // empty' "$scenario_json")"
  operation="$(jq -r '.operation // empty' "$scenario_json")"
  artifact_ref="$(jq -r '.artifact_path // empty' "$scenario_json")"
  execute_cmd="$(jq -r '.execute_cmd // empty' "$scenario_json")"
  expected_exit_code="$(jq -c '.expected_exit_code // null' "$scenario_json")"
  expected_risk_level="$(jq -r '.expected_risk_level // ""' "$scenario_json")"
  expected_risk_tags="$(jq -c '.expected_risk_tags // [] | map(tostring) | unique | sort' "$scenario_json" 2>/dev/null || echo '[]')"

  [[ -n "$tool" ]] || { warn "skip $scenario_id: missing tool"; continue; }
  [[ -n "$operation" ]] || { warn "skip $scenario_id: missing operation"; continue; }
  [[ -n "$artifact_ref" ]] || { warn "skip $scenario_id: missing artifact_path"; continue; }
  [[ -n "$execute_cmd" ]] || { warn "skip $scenario_id: missing execute_cmd"; continue; }

  artifact_path="$(resolve_path "$scenario_dir" "$artifact_ref" || true)"
  [[ -n "$artifact_path" ]] || { warn "skip $scenario_id: cannot resolve artifact_path '$artifact_ref'"; continue; }
  [[ -f "$artifact_path" ]] || { warn "skip $scenario_id: artifact not found '$artifact_path'"; continue; }

  for repeat_idx in $(seq 1 "$REPEATS"); do
    runs_total=$((runs_total + 1))

    run_id="${run_stamp}-$(safe_model_id "$MODEL_ID")-${scenario_id}-r${repeat_idx}"
    run_dir="$OUT_DIR/$run_id"
    mkdir -p "$run_dir"

    stdout_log="$run_dir/agent_stdout.log"
    stderr_log="$run_dir/agent_stderr.log"
    agent_output="$run_dir/agent_output.json"
    agent_raw_stream="$run_dir/agent_raw_stream.jsonl"
    result_json="$run_dir/result.json"

    start_epoch="$(date +%s)"
    started_at="$(iso_utc_now)"

    export EVIDRA_RUN_ID="$run_id"
    export EVIDRA_MODEL_ID="$MODEL_ID"
    export EVIDRA_PROVIDER="$PROVIDER"
    export EVIDRA_REPEAT_INDEX="$repeat_idx"
    export EVIDRA_PROMPT_FILE="$PROMPT_FILE"
    export EVIDRA_PROMPT_VERSION="$PROMPT_VERSION"
    export EVIDRA_PROMPT_CONTRACT_VERSION="$PROMPT_CONTRACT_VERSION"
    export EVIDRA_AGENT_OUTPUT="$agent_output"
    export EVIDRA_AGENT_RAW_STREAM="$agent_raw_stream"
    export EVIDRA_EXEC_SCENARIO_ID="$scenario_id"
    export EVIDRA_EXEC_TOOL="$tool"
    export EVIDRA_EXEC_OPERATION="$operation"
    export EVIDRA_EXEC_ARTIFACT="$artifact_path"
    export EVIDRA_EXEC_COMMAND="$execute_cmd"
    export EVIDRA_EXEC_EXPECTED_EXIT_CODE="$expected_exit_code"

    exit_code=0
    status="success"

    if [[ "$DRY_RUN" -eq 1 ]]; then
      printf 'dry-run: %s\n' "$run_id" >"$stdout_log"
      : >"$stderr_log"
      : >"$agent_raw_stream"
      jq -n '{prescribe_ok:true,report_ok:true,exit_code:0,risk_level:"",risk_tags:[]}' >"$agent_output"
      status="dry_run"
      runs_dry=$((runs_dry + 1))
    else
      set +e
      if [[ -n "$timeout_bin" ]]; then
        "$timeout_bin" "$TIMEOUT_SECONDS" bash -lc "$AGENT_CMD" >"$stdout_log" 2>"$stderr_log"
      else
        bash -lc "$AGENT_CMD" >"$stdout_log" 2>"$stderr_log"
      fi
      exit_code=$?
      set -e

      if [[ "$exit_code" -eq 124 || "$exit_code" -eq 137 ]]; then
        status="timeout"
        runs_timeout=$((runs_timeout + 1))
      elif [[ "$exit_code" -ne 0 ]]; then
        status="failure"
        runs_failure=$((runs_failure + 1))
      else
        status="success"
        runs_success=$((runs_success + 1))
      fi

      if [[ ! -s "$agent_output" ]]; then
        jq -n --arg status "$status" --argjson ec "$exit_code" '{prescribe_ok:false,report_ok:false,exit_code:null,status:$status,agent_exit_code:$ec}' >"$agent_output"
      fi
    fi

    if ! jq -e 'type == "object"' "$agent_output" >/dev/null 2>&1; then
      raw_output="$run_dir/agent_output.raw"
      cp "$agent_output" "$raw_output" 2>/dev/null || true
      jq -n --arg status "$status" --arg raw "$(cat "$agent_output" 2>/dev/null || true)" '{prescribe_ok:false,report_ok:false,status:$status,raw_output:$raw}' >"$agent_output"
    fi

    agent_result_json="$(jq -c '.' "$agent_output")"
    observed_exit_code="$(echo "$agent_result_json" | jq -c '.exit_code // null')"
    observed_risk_level="$(echo "$agent_result_json" | jq -r '.risk_level // .predicted_risk_level // ""')"
    observed_risk_tags="$(echo "$agent_result_json" | jq -c '(.risk_tags // .predicted_risk_details // []) | map(tostring) | unique | sort')"
    prescribe_ok="$(echo "$agent_result_json" | jq -c '.prescribe_ok // false')"
    report_ok="$(echo "$agent_result_json" | jq -c '.report_ok // false')"

    evaluation_json="$(
      jq -n \
        --argjson prescribe_ok "$prescribe_ok" \
        --argjson report_ok "$report_ok" \
        --argjson expected_exit "$expected_exit_code" \
        --argjson observed_exit "$observed_exit_code" \
        --arg expected_level "$expected_risk_level" \
        --arg observed_level "$observed_risk_level" \
        --argjson expected_tags "$expected_risk_tags" \
        --argjson observed_tags "$observed_risk_tags" \
        '
        def safe_div($a; $b): if $b == 0 then null else ($a / $b) end;
        def tp($exp; $obs): [ $obs[] | select($exp | index(.) != null) ] | length;
        def fp($exp; $obs): [ $obs[] | select($exp | index(.) == null) ] | length;
        def fn($exp; $obs): [ $exp[] | select($obs | index(.) == null) ] | length;
        ($expected_tags | unique) as $e
        | ($observed_tags | unique) as $o
        | tp($e; $o) as $tp
        | fp($e; $o) as $fp
        | fn($e; $o) as $fn
        | safe_div($tp; ($tp + $fp)) as $precision
        | safe_div($tp; ($tp + $fn)) as $recall
        | (if $expected_level == "" then null else ($expected_level == $observed_level) end) as $risk_level_match
        | {
            prescribe_ok: $prescribe_ok,
            report_ok: $report_ok,
            protocol_ok: ($prescribe_ok and $report_ok),
            command_exit_code: $observed_exit,
            exit_code_match: (if $expected_exit == null then true else ($observed_exit == $expected_exit) end),
            observed_risk_level: $observed_level,
            observed_risk_tags: $o,
            risk_level_match: $risk_level_match,
            true_positive: $tp,
            false_positive: $fp,
            false_negative: $fn,
            precision: $precision,
            recall: $recall,
            f1: (if ($precision == null or $recall == null or ($precision + $recall) == 0) then null else (2 * $precision * $recall / ($precision + $recall)) end),
            pass: (($prescribe_ok and $report_ok) and (if $expected_exit == null then true else ($observed_exit == $expected_exit) end) and (if $risk_level_match == null then true else $risk_level_match end))
          }
        '
    )"

    eval_pass="$(echo "$evaluation_json" | jq -r '.pass')"
    if [[ "$eval_pass" == "true" ]]; then
      runs_eval_pass=$((runs_eval_pass + 1))
    else
      runs_eval_fail=$((runs_eval_fail + 1))
    fi

    end_epoch="$(date +%s)"
    finished_at="$(iso_utc_now)"
    duration_seconds=$((end_epoch - start_epoch))

    jq -n \
      --arg schema_version "evidra.exec-result.v1" \
      --arg run_id "$run_id" \
      --arg started_at "$started_at" \
      --arg finished_at "$finished_at" \
      --argjson duration_seconds "$duration_seconds" \
      --arg mode "$MODE" \
      --arg model_id "$MODEL_ID" \
      --arg provider "$PROVIDER" \
      --arg prompt_version "$PROMPT_VERSION" \
      --arg prompt_file "$PROMPT_FILE" \
      --arg prompt_contract_version "$PROMPT_CONTRACT_VERSION" \
      --argjson repeat_index "$repeat_idx" \
      --arg scenario_id "$scenario_id" \
      --arg category "$category" \
      --arg difficulty "$difficulty" \
      --arg tool "$tool" \
      --arg operation "$operation" \
      --arg artifact_path "$artifact_path" \
      --arg execute_cmd "$execute_cmd" \
      --argjson expected_exit_code "$expected_exit_code" \
      --arg expected_risk_level "$expected_risk_level" \
      --argjson expected_risk_tags "$expected_risk_tags" \
      --arg source_json_path "$scenario_json" \
      --arg agent_cmd "$AGENT_CMD" \
      --argjson timeout_seconds "$TIMEOUT_SECONDS" \
      --arg status "$status" \
      --argjson agent_exit_code "$exit_code" \
      --arg run_dir "$run_dir" \
      --arg stdout_log "$stdout_log" \
      --arg stderr_log "$stderr_log" \
      --arg agent_output "$agent_output" \
      --arg agent_raw_stream "$agent_raw_stream" \
      --arg result_json "$result_json" \
      --argjson agent_result "$agent_result_json" \
      --argjson evaluation "$evaluation_json" \
      '{
        schema_version: $schema_version,
        run_id: $run_id,
        started_at: $started_at,
        finished_at: $finished_at,
        duration_seconds: $duration_seconds,
        mode: $mode,
        model: {
          id: $model_id,
          provider: $provider,
          prompt_version: $prompt_version,
          prompt_file: $prompt_file,
          prompt_contract_version: $prompt_contract_version,
          repeat_index: $repeat_index
        },
        scenario: {
          id: $scenario_id,
          category: $category,
          difficulty: $difficulty,
          tool: $tool,
          operation: $operation,
          artifact_path: $artifact_path,
          execute_cmd: $execute_cmd,
          expected_exit_code: $expected_exit_code,
          expected_risk_level: $expected_risk_level,
          expected_risk_tags: $expected_risk_tags,
          source_json_path: $source_json_path
        },
        execution: {
          agent_cmd: $agent_cmd,
          timeout_seconds: $timeout_seconds,
          status: $status,
          agent_exit_code: $agent_exit_code
        },
        artifacts: {
          run_dir: $run_dir,
          stdout_log: $stdout_log,
          stderr_log: $stderr_log,
          agent_output: $agent_output,
          agent_raw_stream: $agent_raw_stream,
          result_json: $result_json
        },
        agent_result: $agent_result,
        evaluation: $evaluation
      }' >"$result_json"

    jq -e '.schema_version == "evidra.exec-result.v1" and (.run_id | length > 0) and (.scenario.id | length > 0)' "$result_json" >/dev/null \
      || fail "result JSON validation failed for $run_id"

    jq -n \
      --arg run_id "$run_id" \
      --arg scenario_id "$scenario_id" \
      --arg status "$status" \
      --argjson pass "$eval_pass" \
      --arg result_json "$result_json" \
      '{run_id:$run_id,scenario_id:$scenario_id,status:$status,pass:$pass,result_json:$result_json}' >>"$SUMMARY_PATH"

    echo "run-agent-execution-experiments: run_id=$run_id status=$status pass=$eval_pass result=$result_json"
  done
done

echo ""
echo "run-agent-execution-experiments: summary"
echo "  total:       $runs_total"
echo "  success:     $runs_success"
echo "  failure:     $runs_failure"
echo "  timeout:     $runs_timeout"
echo "  dry_run:     $runs_dry"
echo "  eval_pass:   $runs_eval_pass"
echo "  eval_fail:   $runs_eval_fail"
echo "  index:       $SUMMARY_PATH"

echo ""
echo "run-agent-execution-experiments: top statuses from summary"
jq -s 'group_by(.status) | map({status: .[0].status, count: length})' "$SUMMARY_PATH"
