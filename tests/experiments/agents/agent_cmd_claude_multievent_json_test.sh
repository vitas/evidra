#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

artifact="$tmp_dir/artifact.yaml"
expected="$tmp_dir/expected.json"
prompt="$tmp_dir/system_instructions.txt"
out_json="$tmp_dir/out.json"
raw_stream="$tmp_dir/raw_stream.jsonl"

cat >"$artifact" <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  containers:
  - name: app
    image: nginx:latest
YAML

cat >"$expected" <<'JSON'
{"case_id":"demo-case","category":"kubernetes","difficulty":"low"}
JSON

cat >"$prompt" <<'TXT'
# contract: v1.0.1
Output strict JSON.
TXT

mkdir -p "$tmp_dir/bin"
cat >"$tmp_dir/bin/claude" <<'EOF_CLAUDE'
#!/usr/bin/env bash
set -euo pipefail
cat <<'STREAM'
{"type":"assistant","message":{"content":[{"type":"text","text":"{\"predicted_risk_level\":\"high\",\"predicted_risk_details\":[\"k8s.hostpath_mount\"]}"}]}}
{"type":"result","result":"{\"predicted_risk_level\":\"high\",\"predicted_risk_details\":[\"k8s.hostpath_mount\"]}"}
STREAM
EOF_CLAUDE
chmod +x "$tmp_dir/bin/claude"

export PATH="$tmp_dir/bin:$PATH"
export EVIDRA_MODEL_ID="claude/sonnet"
export EVIDRA_ARTIFACT_PATH="$artifact"
export EVIDRA_EXPECTED_JSON="$expected"
export EVIDRA_AGENT_OUTPUT="$out_json"
export EVIDRA_PROMPT_FILE="$prompt"
export EVIDRA_AGENT_RAW_STREAM="$raw_stream"

bash "$REPO_ROOT/scripts/agent-cmd-claude.sh"

jq -e '.predicted_risk_level == "high"' "$out_json" >/dev/null
jq -e '.predicted_risk_details | index("k8s.hostpath_mount") != null' "$out_json" >/dev/null
[[ -s "$raw_stream" ]]

echo "agent_cmd_claude_multievent_json_test: PASS"
