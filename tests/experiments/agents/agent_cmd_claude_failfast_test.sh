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
{"type":"text","text":"not a json object"}
STREAM
EOF_CLAUDE
chmod +x "$tmp_dir/bin/claude"

export PATH="$tmp_dir/bin:$PATH"
export EVIDRA_MODEL_ID="claude/haiku"
export EVIDRA_ARTIFACT_PATH="$artifact"
export EVIDRA_EXPECTED_JSON="$expected"
export EVIDRA_AGENT_OUTPUT="$out_json"
export EVIDRA_PROMPT_FILE="$prompt"
export EVIDRA_AGENT_RAW_STREAM="$raw_stream"

set +e
bash "$REPO_ROOT/scripts/agent-cmd-claude.sh" >"$tmp_dir/stdout.log" 2>"$tmp_dir/stderr.log"
rc=$?
set -e

if [[ "$rc" -eq 0 ]]; then
  echo "agent_cmd_claude_failfast_test: expected non-zero exit when JSON parsing fails" >&2
  exit 1
fi

[[ -s "$raw_stream" ]]
grep -q "not a json object" "$raw_stream"
grep -q "could not parse JSON object" "$tmp_dir/stderr.log"

echo "agent_cmd_claude_failfast_test: PASS"
