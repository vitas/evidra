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

cat >"$artifact" <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  containers:
  - name: app
    image: nginx:latest
    securityContext:
      privileged: true
EOF

cat >"$expected" <<'EOF'
{
  "case_id": "k8s-privileged-container-fail",
  "category": "kubernetes",
  "difficulty": "medium"
}
EOF

cat >"$prompt" <<'EOF'
# contract: v1.0.1
You are an infrastructure risk assessor.
Always return strict JSON.
EOF

mkdir -p "$tmp_dir/bin"
cat >"$tmp_dir/bin/claude" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat <<'JSON'
{"type":"text","text":"{\"predicted_risk_level\":\"high\",\"predicted_risk_details\":[\"k8s.privileged_container\"]}"}
JSON
EOF
chmod +x "$tmp_dir/bin/claude"

export PATH="$tmp_dir/bin:$PATH"
export EVIDRA_MODEL_ID="claude/sonnet"
export EVIDRA_ARTIFACT_PATH="$artifact"
export EVIDRA_EXPECTED_JSON="$expected"
export EVIDRA_AGENT_OUTPUT="$out_json"
export EVIDRA_PROMPT_FILE="$prompt"

bash "$REPO_ROOT/scripts/agent-cmd-claude.sh"

jq -e '.predicted_risk_level == "high"' "$out_json" >/dev/null
jq -e '.predicted_risk_details | index("k8s.privileged_container") != null' "$out_json" >/dev/null
jq -e '.prompt_contract_version == "v1.0.1"' "$out_json" >/dev/null

echo "agent_cmd_claude_test: PASS"
