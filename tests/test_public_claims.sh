#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

grep -Fq "Hosted \`compare\` is not part of this contract yet." docs/guides/self-hosted-setup.md \
  || fail "self-hosted status doc should keep hosted compare boundaries explicit"

grep -Fq "X-Evidra-API-Key" docs/guides/self-hosted-setup.md \
  || fail "self-hosted status doc should document tenant-aware webhook routing"

echo "PASS: test_public_claims"
