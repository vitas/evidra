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
out_json="$tmp_dir/out.json"
raw_stream="$tmp_dir/raw.jsonl"
config="$tmp_dir/mcp-config.json"

cat >"$artifact" <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
  - name: c
    image: nginx
YAML

cat >"$config" <<'JSON'
{"mcpServers":{"evidra":{"command":"evidra-mcp","args":[]}}}
JSON

mkdir -p "$tmp_dir/bin"

cat >"$tmp_dir/bin/npx" <<'EOF_NPX'
#!/usr/bin/env bash
set -euo pipefail
tool=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tool-name)
      tool="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ "$tool" == "prescribe" ]]; then
  cat <<'JSON'
{"structuredContent":{"ok":true,"prescription_id":"rx-123","risk_level":"critical","risk_tags":["k8s.privileged_container"]}}
JSON
  exit 0
fi

if [[ "$tool" == "report" ]]; then
  cat <<'JSON'
{"structuredContent":{"ok":true,"report_id":"rp-123"}}
JSON
  exit 0
fi

echo "unexpected tool name: $tool" >&2
exit 2
EOF_NPX
chmod +x "$tmp_dir/bin/npx"

cat >"$tmp_dir/bin/kubectl" <<'EOF_KUBECTL'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF_KUBECTL
chmod +x "$tmp_dir/bin/kubectl"

cat >"$tmp_dir/bin/evidra-mcp" <<'EOF_MCP'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF_MCP
chmod +x "$tmp_dir/bin/evidra-mcp"

export PATH="$tmp_dir/bin:$PATH"
export EVIDRA_MCP_CONFIG="$config"
export EVIDRA_MCP_INSPECTOR_BIN="$tmp_dir/bin/npx"
export EVIDRA_EXEC_TOOL="kubectl"
export EVIDRA_EXEC_OPERATION="apply"
export EVIDRA_EXEC_ARTIFACT="$artifact"
export EVIDRA_EXEC_COMMAND='true'
export EVIDRA_AGENT_OUTPUT="$out_json"
export EVIDRA_AGENT_RAW_STREAM="$raw_stream"
export EVIDRA_MODEL_ID="claude/sonnet"
export EVIDRA_PROMPT_CONTRACT_VERSION="v1.0.1"

bash "$REPO_ROOT/scripts/agent-cmd-mcp-kubectl.sh"

jq -e '.prescribe_ok == true' "$out_json" >/dev/null
jq -e '.report_ok == true' "$out_json" >/dev/null
jq -e '.risk_level == "critical"' "$out_json" >/dev/null
jq -e '.risk_tags | index("k8s.privileged_container") != null' "$out_json" >/dev/null
jq -e '.exit_code == 0' "$out_json" >/dev/null
grep -Eq '"phase"[[:space:]]*:[[:space:]]*"prescribe"' "$raw_stream"
grep -Eq '"phase"[[:space:]]*:[[:space:]]*"report"' "$raw_stream"

echo "agent_cmd_mcp_kubectl_test: PASS"
