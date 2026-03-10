#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

go_floor="$(
  sed -nE 's/^go ([0-9]+\.[0-9]+).*/\1/p' go.mod | head -n1
)"
if [[ -z "${go_floor:-}" ]]; then
  fail "could not parse Go version from go.mod"
fi

grep -Fq "Requires Go ${go_floor}+." CONTRIBUTING.md \
  || fail "CONTRIBUTING.md should advertise the same Go floor as go.mod"

grep -Fq "| 0.4.x | Yes |" SECURITY.md \
  || fail "SECURITY.md should list the maintained 0.4.x release line"

for file in README.md SECURITY.md docs/integrations/CLI_REFERENCE.md; do
  grep -Fq "Evidra does not sandbox the wrapped command" "$file" \
    || fail "$file should document the run command execution boundary"
done

grep -Fq "same trust model as direct shell execution" docs/integrations/CLI_REFERENCE.md \
  || fail "CLI reference should explain the trust boundary for run"

echo "PASS: test_doc_trust_alignment"
