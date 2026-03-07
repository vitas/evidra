#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

out_dir="$tmp_dir/results"
mkdir -p "$out_dir"
echo "stale" >"$out_dir/stale.txt"

bash "$REPO_ROOT/scripts/run-agent-experiments.sh" \
  --model-id test/model \
  --provider test \
  --prompt-version v1 \
  --dry-run \
  --repeats 1 \
  --max-cases 1 \
  --out-dir "$out_dir" \
  --clean-out-dir

summary="$out_dir/summary.jsonl"
[[ -s "$summary" ]]
[[ ! -f "$out_dir/stale.txt" ]]

echo "run_agent_experiments_clean_out_dir_test: PASS"
