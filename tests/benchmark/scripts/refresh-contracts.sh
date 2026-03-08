#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tests/benchmark/scripts/refresh-contracts.sh [options]

Options:
  --case <case-id>       Refresh a single case only
  --evidra-bin <path>    Explicit evidra binary passed to process-artifact
  --operation <name>     Operation to pass to process-artifact (default: apply)
  --check                Verify golden contracts are in sync (no writes)
  -h, --help             Show this help
EOF
}

fail() {
  echo "refresh-contracts: $*" >&2
  exit 1
}

CASE_FILTER=""
EVIDRA_BIN=""
OPERATION="apply"
CHECK_MODE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --case)
      [[ $# -ge 2 ]] || fail "--case requires a value"
      CASE_FILTER="$2"
      shift 2
      ;;
    --evidra-bin)
      [[ $# -ge 2 ]] || fail "--evidra-bin requires a value"
      EVIDRA_BIN="$2"
      shift 2
      ;;
    --operation)
      [[ $# -ge 2 ]] || fail "--operation requires a value"
      OPERATION="$2"
      shift 2
      ;;
    --check)
      CHECK_MODE=1
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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

PROCESS_SCRIPT="tests/benchmark/scripts/process-artifact.sh"
CASES_DIR="tests/benchmark/cases"

[[ -x "$PROCESS_SCRIPT" ]] || fail "missing executable $PROCESS_SCRIPT"
[[ -d "$CASES_DIR" ]] || fail "missing $CASES_DIR"

expected_files=()
while IFS= read -r file; do
  expected_files+=("$file")
done < <(find "$CASES_DIR" -type f -name "expected.json" | sort)

[[ ${#expected_files[@]} -gt 0 ]] || fail "no expected.json files found"

ok_count=0
skip_count=0
fail_count=0

for expected in "${expected_files[@]}"; do
  case_dir="$(dirname "$expected")"
  case_id="$(jq -r '.case_id // empty' "$expected")"
  if [[ -z "$case_id" ]]; then
    echo "refresh-contracts: WARN skip $expected (missing case_id)" >&2
    skip_count=$((skip_count + 1))
    continue
  fi

  if [[ -n "$CASE_FILTER" && "$case_id" != "$CASE_FILTER" ]]; then
    continue
  fi

  artifact_ref="$(jq -r '.artifact_ref // empty' "$expected")"
  if [[ -z "$artifact_ref" || "$artifact_ref" == "TODO" ]]; then
    echo "refresh-contracts: WARN skip $case_id (artifact_ref missing/TODO)" >&2
    skip_count=$((skip_count + 1))
    continue
  fi

  artifact_path="$case_dir/$artifact_ref"
  if [[ ! -f "$artifact_path" ]]; then
    echo "refresh-contracts: WARN skip $case_id (artifact missing: $artifact_path)" >&2
    skip_count=$((skip_count + 1))
    continue
  fi

  category="$(jq -r '.category // "unknown"' "$expected")"
  tool="generic"
  case "$category" in
    kubernetes) tool="kubectl" ;;
    terraform) tool="terraform" ;;
    helm) tool="helm" ;;
    argocd) tool="argocd" ;;
  esac

  out_path="$case_dir/golden/contract.json"
  tmp_out="$(mktemp)"
  log_out="$(mktemp)"
  process_cmd=(bash "$PROCESS_SCRIPT" --artifact "$artifact_path" --tool "$tool" --operation "$OPERATION" --out "$tmp_out")
  if [[ -n "$EVIDRA_BIN" ]]; then
    process_cmd+=(--evidra-bin "$EVIDRA_BIN")
  fi

  if ! "${process_cmd[@]}" >"$log_out" 2>&1; then
    # Fallback for artifacts that do not parse for category-specific tool.
    if [[ "$tool" != "generic" ]]; then
      process_cmd=(bash "$PROCESS_SCRIPT" --artifact "$artifact_path" --tool generic --operation "$OPERATION" --out "$tmp_out")
      if [[ -n "$EVIDRA_BIN" ]]; then
        process_cmd+=(--evidra-bin "$EVIDRA_BIN")
      fi
      if ! "${process_cmd[@]}" >"$log_out" 2>&1; then
        echo "refresh-contracts: FAIL $case_id (tool=$tool + fallback generic failed)" >&2
        fail_count=$((fail_count + 1))
        rm -f "$tmp_out" "$log_out"
        continue
      else
        echo "refresh-contracts: INFO $case_id used fallback tool=generic" >&2
      fi
    else
      echo "refresh-contracts: FAIL $case_id (tool=generic failed)" >&2
      fail_count=$((fail_count + 1))
      rm -f "$tmp_out" "$log_out"
      continue
    fi
  fi

  jq --arg case_id "$case_id" '. + {case_id: $case_id}' "$tmp_out" > "$tmp_out.with_case_id"
  mv "$tmp_out.with_case_id" "$tmp_out"
  rm -f "$log_out"

  if [[ "$CHECK_MODE" -eq 1 ]]; then
    if [[ ! -f "$out_path" ]]; then
      echo "refresh-contracts: FAIL $case_id missing $out_path (run refresh to create it)" >&2
      fail_count=$((fail_count + 1))
      rm -f "$tmp_out"
      continue
    fi

    tmp_norm="$(mktemp)"
    out_norm="$(mktemp)"
    if ! jq -S 'del(.processing.processed_at, .prescription_id, .evidra_version)' "$tmp_out" >"$tmp_norm" 2>/dev/null; then
      echo "refresh-contracts: FAIL $case_id generated contract is not valid JSON" >&2
      fail_count=$((fail_count + 1))
      rm -f "$tmp_out" "$tmp_norm" "$out_norm"
      continue
    fi
    if ! jq -S 'del(.processing.processed_at, .prescription_id, .evidra_version)' "$out_path" >"$out_norm" 2>/dev/null; then
      echo "refresh-contracts: FAIL $case_id existing contract is not valid JSON: $out_path" >&2
      fail_count=$((fail_count + 1))
      rm -f "$tmp_out" "$tmp_norm" "$out_norm"
      continue
    fi

    if ! diff -u "$out_norm" "$tmp_norm" >/dev/null 2>&1; then
      echo "refresh-contracts: FAIL $case_id drift detected in $out_path (run benchmark-refresh-contracts)" >&2
      fail_count=$((fail_count + 1))
      rm -f "$tmp_out" "$tmp_norm" "$out_norm"
      continue
    fi
    rm -f "$tmp_out" "$tmp_norm" "$out_norm"
    ok_count=$((ok_count + 1))
    echo "refresh-contracts: OK $case_id (no drift)"
    continue
  fi

  mkdir -p "$case_dir/golden"
  mv "$tmp_out" "$out_path"
  ok_count=$((ok_count + 1))
  echo "refresh-contracts: OK $case_id -> $out_path"
done

if [[ -n "$CASE_FILTER" && "$ok_count" -eq 0 && "$fail_count" -eq 0 ]]; then
  fail "case filter '$CASE_FILTER' matched no cases"
fi

mode="refresh"
if [[ "$CHECK_MODE" -eq 1 ]]; then
  mode="check"
fi
echo "refresh-contracts: summary mode=$mode ok=$ok_count skipped=$skip_count failed=$fail_count"
[[ "$fail_count" -eq 0 ]] || exit 1
