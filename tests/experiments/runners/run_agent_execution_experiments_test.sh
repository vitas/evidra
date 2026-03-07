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
  "expected_risk_level": "low",
  "expected_risk_tags": []
}
EOF_SCN

fake_agent="$tmp_dir/fake-agent.sh"
cat >"$fake_agent" <<'EOF_AGENT'
#!/usr/bin/env bash
set -euo pipefail
cat >"${EVIDRA_AGENT_OUTPUT:?missing EVIDRA_AGENT_OUTPUT}" <<'JSON'
{
  "prescribe_ok": true,
  "report_ok": true,
  "exit_code": 0,
  "risk_level": "low",
  "risk_tags": [],
  "prescription_id": "rx-123",
  "report_id": "rp-123"
}
JSON
EOF_AGENT
chmod +x "$fake_agent"

out_dir="$tmp_dir/results"
mkdir -p "$out_dir"
echo "stale" >"$out_dir/stale.txt"

bash "$REPO_ROOT/scripts/run-agent-execution-experiments.sh" \
  --model-id test/model \
  --provider test \
  --prompt-version v1 \
  --scenarios-dir "$scenarios_dir" \
  --repeats 1 \
  --timeout-seconds 30 \
  --agent-cmd "$fake_agent" \
  --out-dir "$out_dir" \
  --clean-out-dir

summary="$out_dir/summary.jsonl"
[[ -s "$summary" ]]
[[ ! -f "$out_dir/stale.txt" ]]

result_json="$(jq -r '.result_json' "$summary" | head -n1)"
[[ -f "$result_json" ]]

jq -e '.schema_version == "evidra.exec-result.v1"' "$result_json" >/dev/null
jq -e '.evaluation.protocol_ok == true' "$result_json" >/dev/null
jq -e '.evaluation.exit_code_match == true' "$result_json" >/dev/null
jq -e '.evaluation.risk_level_match == true' "$result_json" >/dev/null

echo "run_agent_execution_experiments_test: PASS"
