#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

for path in docs/protocol.md skills/evidra/SKILL.md prompts/evidra-agent.md; do
  [[ -s "$path" ]] || fail "missing protocol asset: $path"
done

for path in scripts/prompts-generate.sh scripts/prompts-verify.sh; do
  [[ ! -e "$path" ]] || fail "removed prompt factory script remains: $path"
done

for path in skills/evidra/SKILL.md prompts/evidra-agent.md; do
  for removed in prescribe_smart prescribe_full run_command prescription_id; do
    if grep -Fq "$removed" "$path"; then
      fail "$path instructs agents to use removed protocol field or tool: $removed"
    fi
  done
done

grep -Fq 'docs/protocol.md' README.md || fail "README does not link the protocol reference"
grep -Fq 'skills/evidra/SKILL.md' docs/getting-started.md || fail "Getting Started does not link the skill"
grep -Fq 'prompts/evidra-agent.md' docs/getting-started.md || fail "Getting Started does not link the portable prompt"

echo "PASS: test_protocol_assets"
