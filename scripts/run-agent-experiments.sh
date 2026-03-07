#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CASES_DIR="$ROOT_DIR/tests/benchmark/cases"
RESULT_SCHEMA_PATH="$ROOT_DIR/docs/experimental/RESULT_SCHEMA.json"

usage() {
  cat <<'USAGE'
Usage:
  scripts/run-agent-experiments.sh [options]

Required:
  --model-id <id>            Model id (example: anthropic/claude-3-5-haiku)
  --agent-cmd <command>      Command executed per run via bash -lc

Optional:
  --provider <name>          Provider label (default: unknown)
  --prompt-version <ver>     Prompt version label for metadata (default: from prompt file header, else v1)
  --prompt-file <path>       Prompt file path exported to agent command (default: prompts/experiments/runtime/system_instructions.txt)
  --temperature <num>        Temperature recorded in result metadata
  --mode <name>              Execution mode label (default: custom)
  --repeats <n>              Repeats per case (default: 3)
  --timeout-seconds <n>      Per-run timeout in seconds (default: 300)
  --case-filter <regex>      Regex filter on case_id
  --max-cases <n>            Max selected cases after filtering
  --out-dir <path>           Output dir (default: experiments/results/<timestamp>)
  --clean-out-dir            Remove existing files in output dir before run
  --dry-run                  Do not execute agent command; write synthetic output
  -h, --help                 Show help

Environment variables exported to each agent command:
  EVIDRA_RUN_ID
  EVIDRA_CASE_ID
  EVIDRA_REPEAT_INDEX
  EVIDRA_MODEL_ID
  EVIDRA_PROVIDER
  EVIDRA_EXPECTED_JSON
  EVIDRA_ARTIFACT_PATH
  EVIDRA_AGENT_OUTPUT        (agent should write JSON here)
  EVIDRA_AGENT_RAW_STREAM    (optional raw model stream output path)
  EVIDRA_PROMPT_FILE
  EVIDRA_PROMPT_VERSION
  EVIDRA_PROMPT_CONTRACT_VERSION

Agent output JSON should ideally contain:
  {
    "predicted_risk_level": "medium",
    "predicted_risk_details": ["k8s.privileged_container"]
  }
USAGE
}

fail() {
  echo "run-agent-experiments: FAIL $*" >&2
  exit 1
}

warn() {
  echo "run-agent-experiments: WARN $*" >&2
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

MODEL_ID=""
PROVIDER="unknown"
PROMPT_VERSION=""
PROMPT_VERSION_SET=0
PROMPT_FILE="$ROOT_DIR/prompts/experiments/runtime/system_instructions.txt"
PROMPT_CONTRACT_VERSION=""
TEMPERATURE=""
MODE="custom"
REPEATS=3
TIMEOUT_SECONDS=300
CASE_FILTER=""
MAX_CASES=0
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
    --temperature)
      [[ $# -ge 2 ]] || fail "--temperature requires a value"
      TEMPERATURE="$2"
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
    --case-filter)
      [[ $# -ge 2 ]] || fail "--case-filter requires a value"
      CASE_FILTER="$2"
      shift 2
      ;;
    --max-cases)
      [[ $# -ge 2 ]] || fail "--max-cases requires a value"
      MAX_CASES="$2"
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
[[ -d "$CASES_DIR" ]] || fail "missing cases dir: $CASES_DIR"
[[ -f "$RESULT_SCHEMA_PATH" ]] || fail "missing result schema: $RESULT_SCHEMA_PATH"

[[ -n "$MODEL_ID" ]] || fail "--model-id is required"
[[ -n "$AGENT_CMD" || "$DRY_RUN" -eq 1 ]] || fail "--agent-cmd is required unless --dry-run is set"
if [[ "$DRY_RUN" -ne 1 && "$AGENT_CMD" == *"...your harness command..."* ]]; then
  fail "--agent-cmd contains placeholder text; use a real command (example: --agent-cmd 'bash scripts/agent-cmd-bifrost.sh')"
fi

[[ "$REPEATS" =~ ^[0-9]+$ ]] || fail "--repeats must be integer"
(( REPEATS >= 1 )) || fail "--repeats must be >= 1"

[[ "$TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || fail "--timeout-seconds must be integer"
(( TIMEOUT_SECONDS >= 1 )) || fail "--timeout-seconds must be >= 1"

[[ "$MAX_CASES" =~ ^[0-9]+$ ]] || fail "--max-cases must be integer"

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

temperature_json="null"
if [[ -n "$TEMPERATURE" ]]; then
  if ! temperature_json="$(jq -n --arg t "$TEMPERATURE" '$t|tonumber' 2>/dev/null)"; then
    fail "--temperature must be numeric"
  fi
fi

run_stamp="$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="$ROOT_DIR/experiments/results/$run_stamp"
fi
if [[ "$CLEAN_OUT_DIR" -eq 1 ]]; then
  clean_out_dir "$OUT_DIR"
fi
mkdir -p "$OUT_DIR"
SUMMARY_PATH="$OUT_DIR/summary.jsonl"
: > "$SUMMARY_PATH"

selected_expected=()
while IFS= read -r expected; do
  case_id="$(jq -r '.case_id // empty' "$expected")"
  [[ -n "$case_id" ]] || { warn "skip $expected: missing case_id"; continue; }

  if [[ -n "$CASE_FILTER" ]]; then
    if ! [[ "$case_id" =~ $CASE_FILTER ]]; then
      continue
    fi
  fi

  selected_expected+=("$expected")
done < <(find "$CASES_DIR" -type f -name "expected.json" | sort)

if (( MAX_CASES > 0 && ${#selected_expected[@]} > MAX_CASES )); then
  selected_expected=("${selected_expected[@]:0:MAX_CASES}")
fi

(( ${#selected_expected[@]} > 0 )) || fail "no cases selected"

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

echo "run-agent-experiments: selected_cases=${#selected_expected[@]} repeats=$REPEATS out_dir=$OUT_DIR prompt_version=$PROMPT_VERSION prompt_file=$PROMPT_FILE"

for expected in "${selected_expected[@]}"; do
  case_dir="$(dirname "$expected")"
  case_id="$(jq -r '.case_id' "$expected")"
  category="$(jq -r '.category // "unknown"' "$expected")"
  difficulty="$(jq -r '.difficulty // "unknown"' "$expected")"
  ground_truth_pattern="$(jq -r '.ground_truth_pattern // ""' "$expected")"
  expected_risk_level="$(jq -r '.risk_level // ""' "$expected")"
  expected_risk_details="$(jq -c '.risk_details_expected // [] | map(tostring) | unique | sort' "$expected" 2>/dev/null || echo '[]')"
  [[ -n "$expected_risk_details" ]] || expected_risk_details='[]'

  artifact_ref="$(jq -r '.artifact_ref // empty' "$expected")"
  [[ -n "$artifact_ref" ]] || { warn "skip $case_id: missing artifact_ref"; continue; }
  artifact_path="$case_dir/$artifact_ref"
  [[ -f "$artifact_path" ]] || { warn "skip $case_id: artifact not found: $artifact_path"; continue; }

  for repeat_idx in $(seq 1 "$REPEATS"); do
    runs_total=$((runs_total + 1))

    run_id="${run_stamp}-$(safe_model_id "$MODEL_ID")-${case_id}-r${repeat_idx}"
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
    export EVIDRA_CASE_ID="$case_id"
    export EVIDRA_REPEAT_INDEX="$repeat_idx"
    export EVIDRA_MODEL_ID="$MODEL_ID"
    export EVIDRA_PROVIDER="$PROVIDER"
    export EVIDRA_EXPECTED_JSON="$expected"
    export EVIDRA_ARTIFACT_PATH="$artifact_path"
    export EVIDRA_AGENT_OUTPUT="$agent_output"
    export EVIDRA_AGENT_RAW_STREAM="$agent_raw_stream"
    export EVIDRA_PROMPT_FILE="$PROMPT_FILE"
    export EVIDRA_PROMPT_VERSION="$PROMPT_VERSION"
    export EVIDRA_PROMPT_CONTRACT_VERSION="$PROMPT_CONTRACT_VERSION"

    exit_code=0
    status="success"

    if [[ "$DRY_RUN" -eq 1 ]]; then
      printf 'dry-run: %s\n' "$run_id" >"$stdout_log"
      : >"$stderr_log"
      jq -n '{predicted_risk_level:"",predicted_risk_details:[]}' >"$agent_output"
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
        if [[ -s "$stdout_log" ]] && jq -e 'type == "object"' "$stdout_log" >/dev/null 2>&1; then
          cp "$stdout_log" "$agent_output"
        else
          jq -n --arg s "$status" --argjson ec "$exit_code" '{predicted_risk_level:"",predicted_risk_details:[],status:$s,exit_code:$ec}' >"$agent_output"
        fi
      fi
    fi

    if ! jq -e 'type == "object"' "$agent_output" >/dev/null 2>&1; then
      raw_output="$run_dir/agent_output.raw"
      cp "$agent_output" "$raw_output" 2>/dev/null || true
      jq -n --arg status "$status" --arg raw "$(cat "$agent_output" 2>/dev/null || true)" '{predicted_risk_level:"",predicted_risk_details:[],status:$status,raw_output:$raw}' >"$agent_output"
    fi

    predicted_risk_level="$(jq -r '.predicted_risk_level // .risk_level // ""' "$agent_output")"
    predicted_risk_details="$(jq -c '(.predicted_risk_details // .predicted_risk_tags // .risk_details // .risk_tags // []) | map(tostring) | unique | sort' "$agent_output" 2>/dev/null || echo '[]')"
    [[ -n "$predicted_risk_details" ]] || predicted_risk_details='[]'

    evaluation_json="$(
      jq -n \
        --arg expected_level "$expected_risk_level" \
        --arg predicted_level "$predicted_risk_level" \
        --argjson expected_tags "$expected_risk_details" \
        --argjson predicted_tags "$predicted_risk_details" \
        '
        def safe_div($a; $b): if $b == 0 then null else ($a / $b) end;
        def tp($exp; $pred): [ $pred[] | select($exp | index(.) != null) ] | length;
        def fp($exp; $pred): [ $pred[] | select($exp | index(.) == null) ] | length;
        def fn($exp; $pred): [ $exp[] | select($pred | index(.) == null) ] | length;

        ($expected_tags | unique) as $e
        | ($predicted_tags | unique) as $p
        | tp($e; $p) as $tp
        | fp($e; $p) as $fp
        | fn($e; $p) as $fn
        | safe_div($tp; ($tp + $fp)) as $precision
        | safe_div($tp; ($tp + $fn)) as $recall
        | {
            predicted_risk_level: $predicted_level,
            predicted_risk_details: $p,
            risk_level_match: ($expected_level != "" and $predicted_level != "" and ($expected_level == $predicted_level)),
            true_positive: $tp,
            false_positive: $fp,
            false_negative: $fn,
            precision: $precision,
            recall: $recall,
            f1: (if ($precision == null or $recall == null or ($precision + $recall) == 0) then null else (2 * $precision * $recall / ($precision + $recall)) end)
          }
        '
    )"

    end_epoch="$(date +%s)"
    finished_at="$(iso_utc_now)"
    duration_seconds=$((end_epoch - start_epoch))

    jq -n \
      --arg schema_version "evidra.result.v1" \
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
      --argjson temperature "$temperature_json" \
      --argjson repeat_index "$repeat_idx" \
      --arg case_id "$case_id" \
      --arg category "$category" \
      --arg difficulty "$difficulty" \
      --arg ground_truth_pattern "$ground_truth_pattern" \
      --arg expected_risk_level "$expected_risk_level" \
      --argjson expected_risk_details "$expected_risk_details" \
      --arg artifact_path "$artifact_path" \
      --arg expected_json_path "$expected" \
      --arg agent_cmd "$AGENT_CMD" \
      --argjson timeout_seconds "$TIMEOUT_SECONDS" \
      --arg status "$status" \
      --argjson exit_code "$exit_code" \
      --arg run_dir "$run_dir" \
      --arg stdout_log "$stdout_log" \
      --arg stderr_log "$stderr_log" \
      --arg agent_output "$agent_output" \
      --arg agent_raw_stream "$agent_raw_stream" \
      --arg result_json "$result_json" \
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
          temperature: $temperature,
          repeat_index: $repeat_index
        },
        case: {
          id: $case_id,
          category: $category,
          difficulty: $difficulty,
          ground_truth_pattern: $ground_truth_pattern,
          expected_risk_level: $expected_risk_level,
          expected_risk_details: $expected_risk_details,
          artifact_path: $artifact_path,
          expected_json_path: $expected_json_path
        },
        execution: {
          agent_cmd: $agent_cmd,
          timeout_seconds: $timeout_seconds,
          status: $status,
          exit_code: $exit_code
        },
        artifacts: {
          run_dir: $run_dir,
          stdout_log: $stdout_log,
          stderr_log: $stderr_log,
          agent_output: $agent_output,
          agent_raw_stream: $agent_raw_stream,
          result_json: $result_json
        },
        evaluation: $evaluation
      }' >"$result_json"

    jq -e '.schema_version == "evidra.result.v1" and (.run_id | length > 0) and (.case.id | length > 0)' "$result_json" >/dev/null \
      || fail "result JSON validation failed for $run_id"

    jq -n \
      --arg run_id "$run_id" \
      --arg case_id "$case_id" \
      --arg status "$status" \
      --arg result_json "$result_json" \
      '{run_id:$run_id,case_id:$case_id,status:$status,result_json:$result_json}' >>"$SUMMARY_PATH"

    echo "run-agent-experiments: run_id=$run_id status=$status result=$result_json"
  done
done

echo ""
echo "run-agent-experiments: summary"
echo "  total:    $runs_total"
echo "  success:  $runs_success"
echo "  failure:  $runs_failure"
echo "  timeout:  $runs_timeout"
echo "  dry_run:  $runs_dry"
echo "  index:    $SUMMARY_PATH"

echo ""
echo "run-agent-experiments: top statuses from summary"
jq -s 'group_by(.status) | map({status: .[0].status, count: length})' "$SUMMARY_PATH"
