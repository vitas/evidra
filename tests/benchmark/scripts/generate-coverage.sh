#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

CASES_DIR="tests/benchmark/cases"
DATASET_JSON="tests/benchmark/dataset.json"
CORPUS_DIR="tests/benchmark/corpus"

if ! command -v jq >/dev/null 2>&1; then
  echo "coverage: jq is required" >&2
  exit 2
fi

tmp_expected="$(mktemp)"
tmp_signals="$(mktemp)"
tmp_gaps="$(mktemp)"
tmp_categories="$(mktemp)"
tmp_patterns="$(mktemp)"
tmp_difficulties="$(mktemp)"
trap 'rm -f "$tmp_expected" "$tmp_signals" "$tmp_gaps" "$tmp_categories" "$tmp_patterns" "$tmp_difficulties"' EXIT

if [[ -d "$CASES_DIR" ]]; then
  find "$CASES_DIR" -type f -name "expected.json" | sort > "$tmp_expected"
fi

total_cases="$(wc -l < "$tmp_expected" | tr -d ' ')"
if [[ -d "$CORPUS_DIR" ]]; then
  corpus_files="$(find "$CORPUS_DIR" -type f | wc -l | tr -d ' ')"
else
  corpus_files="0"
fi

dataset_label="unknown"
dataset_scope="unknown"
if [[ -f "$DATASET_JSON" ]]; then
  dataset_label="$(jq -r '.dataset_label // "unknown"' "$DATASET_JSON")"
  dataset_scope="$(jq -r '.dataset_scope // "unknown"' "$DATASET_JSON")"
fi

case_role() {
  local expected_file="$1"
  local case_id
  case_id="$(jq -r '.case_id // ""' "$expected_file")"
  case "$case_id" in
    *-fail) echo "fail" ;;
    *-pass) echo "pass" ;;
    *) echo "other" ;;
  esac
}

echo "# Evidra Benchmark Dataset - Coverage Report"
echo ""
echo "Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""
echo "**Dataset label:** \`$dataset_label\`  "
echo "**Dataset scope:** \`$dataset_scope\`  "
echo "**Cases:** $total_cases | **Corpus artifacts:** $corpus_files"
echo ""

echo "## Signal Coverage"
echo ""
echo "| Signal / Risk Tag | FAIL Cases | PASS Cases | Total |"
echo "|-------------------|-----------|-----------|-------|"

if [[ "$total_cases" -gt 0 ]]; then
  while IFS= read -r expected_file; do
    jq -r '
      (.ground_truth_pattern // empty),
      ((.risk_details_expected // [])[]?),
      (((.signals_expected // {}) | keys[])[]?)
    ' "$expected_file"
  done < "$tmp_expected" | awk 'NF > 0' | sort -u > "$tmp_signals"
fi

if [[ ! -s "$tmp_signals" ]]; then
  echo "| _none_ | 0 | 0 | 0 |"
else
  while IFS= read -r signal; do
    fail_count=0
    pass_count=0
    total_count=0

    while IFS= read -r expected_file; do
      matched="$(jq -r --arg s "$signal" '
        ((.ground_truth_pattern // "") == $s) or
        (((.risk_details_expected // []) | index($s)) != null) or
        (((.signals_expected // {})[$s]) != null)
      ' "$expected_file")"
      if [[ "$matched" == "true" ]]; then
        total_count=$((total_count + 1))
        role="$(case_role "$expected_file")"
        if [[ "$role" == "fail" ]]; then
          fail_count=$((fail_count + 1))
        elif [[ "$role" == "pass" ]]; then
          pass_count=$((pass_count + 1))
        fi
      fi
    done < "$tmp_expected"

    echo "| \`$signal\` | $fail_count | $pass_count | $total_count |"
    if [[ "$fail_count" -lt 2 ]]; then
      echo "- \`$signal\`: FAIL cases=$fail_count" >> "$tmp_gaps"
    fi
  done < "$tmp_signals"
fi

echo ""
echo "## Gaps (signals with < 2 FAIL cases)"
echo ""
if [[ -s "$tmp_gaps" ]]; then
  cat "$tmp_gaps"
else
  echo "- none"
fi

echo ""
echo "## By Category"
echo ""
echo "| Category | Cases |"
echo "|----------|-------|"

if [[ "$total_cases" -eq 0 ]]; then
  echo "| _none_ | 0 |"
else
  while IFS= read -r expected_file; do
    jq -r '.category // "unknown"' "$expected_file"
  done < "$tmp_expected" > "$tmp_categories"

  sort "$tmp_categories" | uniq -c | sort -rn | while read -r count category; do
    echo "| $category | $count |"
  done
fi

echo ""
echo "## By Ground Truth Pattern"
echo ""
echo "| Pattern | Cases |"
echo "|---------|-------|"

if [[ "$total_cases" -eq 0 ]]; then
  echo "| _none_ | 0 |"
else
  while IFS= read -r expected_file; do
    jq -r '.ground_truth_pattern // "unknown"' "$expected_file"
  done < "$tmp_expected" > "$tmp_patterns"

  sort "$tmp_patterns" | uniq -c | sort -rn | while read -r count pattern; do
    echo "| \`$pattern\` | $count |"
  done
fi

echo ""
echo "## By Difficulty"
echo ""
echo "| Difficulty | Cases | % |"
echo "|-----------|-------|---|"

if [[ "$total_cases" -eq 0 ]]; then
  echo "| _none_ | 0 | 0% |"
else
  while IFS= read -r expected_file; do
    jq -r '.difficulty // "unknown"' "$expected_file"
  done < "$tmp_expected" > "$tmp_difficulties"

  for diff in easy medium hard catastrophic unknown; do
    count="$(awk -v d="$diff" '$0==d{c++} END{print c+0}' "$tmp_difficulties")"
    if [[ "$count" -eq 0 && "$diff" == "unknown" ]]; then
      continue
    fi
    pct="$(awk -v n="$count" -v d="$total_cases" 'BEGIN { if (d>0) printf "%.0f", (n/d)*100; else printf "0" }')"
    echo "| $diff | $count | ${pct}% |"
  done
fi
