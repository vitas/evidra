#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

scenarios_dir="$tmp_dir/scenarios"
artifacts_dir="$tmp_dir/artifacts"
mkdir -p "$scenarios_dir" "$artifacts_dir"

artifact="$artifacts_dir/deploy.yaml"
cat >"$artifact" <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: app
  template:
    metadata:
      labels:
        app: app
    spec:
      containers:
      - name: app
        image: nginx:1.25
YAML

cat >"$scenarios_dir/k8s-safe-apply.json" <<EOF_SCN
{
  "scenario_id": "k8s-safe-apply",
  "category": "kubernetes",
  "difficulty": "low",
  "tool": "kubectl",
  "operation": "apply",
  "artifact_path": "$artifact",
  "execute_cmd": "kubectl apply -f \"$artifact\"",
  "expected_exit_code": 0,
  "expected_risk_level": "",
  "expected_risk_tags": []
}
EOF_SCN

out_dir="$tmp_dir/results"
mkdir -p "$out_dir"
echo "stale" >"$out_dir/stale.txt"

(
  cd "$REPO_ROOT"
  go run ./cmd/evidra-exp execution run \
    --model-id test/model \
    --provider test \
    --prompt-version v1 \
    --agent dry-run \
    --scenarios-dir "$scenarios_dir" \
    --repeats 1 \
    --timeout-seconds 30 \
    --out-dir "$out_dir" \
    --clean-out-dir
)

summary="$out_dir/summary.jsonl"
[[ -s "$summary" ]]
[[ ! -f "$out_dir/stale.txt" ]]

result_json="$(jq -r '.result_json' "$summary" | head -n1)"
[[ -f "$result_json" ]]

jq -e '.schema_version == "evidra.exec-result.v1"' "$result_json" >/dev/null
jq -e '.evaluation.protocol_ok == true' "$result_json" >/dev/null
jq -e '.evaluation.exit_code_match == true' "$result_json" >/dev/null
jq -e '.evaluation.risk_level_match == null' "$result_json" >/dev/null

echo "run_agent_execution_experiments_test: PASS"
