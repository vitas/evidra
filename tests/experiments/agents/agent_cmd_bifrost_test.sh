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
fake_py="$tmp_dir/fake-python3"

cat >"$artifact" <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: demo
spec: {}
EOF

cat >"$expected" <<'EOF'
{"case_id":"demo-case","category":"kubernetes","difficulty":"low"}
EOF

cat >"$prompt" <<'EOF'
# contract: v1.0.1
Return strict JSON.
EOF

cat >"$fake_py" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "-" ]]; then
  # import check path in wrapper
  exit 0
fi

out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      out="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -z "$out" ]]; then
  echo "missing --output in fake python shim" >&2
  exit 2
fi

cat >"$out" <<'JSON'
{
  "predicted_risk_level": "low",
  "predicted_risk_details": []
}
JSON
EOF
chmod +x "$fake_py"

export EVIDRA_MODEL_ID="anthropic/claude-3-5-haiku"
export EVIDRA_BIFROST_BASE_URL="http://localhost:8080/openai"
export EVIDRA_ARTIFACT_PATH="$artifact"
export EVIDRA_EXPECTED_JSON="$expected"
export EVIDRA_AGENT_OUTPUT="$out_json"
export EVIDRA_PROMPT_FILE="$prompt"
export BIFROST_PYTHON_BIN="$fake_py"

bash "$REPO_ROOT/scripts/agent-cmd-bifrost.sh"

jq -e '.predicted_risk_level == "low"' "$out_json" >/dev/null
echo "agent_cmd_bifrost_test: PASS"
