#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

ALIGN_DOC="docs/system-design/EVIDRA_CNCF_STANDARDS_ALIGNMENT.md"

grep -Fq "Documented mapping only; CloudEvents adapter not implemented on \`main\`." "$ALIGN_DOC" \
  || fail "CloudEvents section should state adapter status precisely"

grep -Fq "OTLP/HTTP metrics export exists today, but trace/span export is not implemented on \`main\`." "$ALIGN_DOC" \
  || fail "OpenTelemetry section should distinguish metrics export from trace export"

grep -Fq "Documented export mapping only; no in-toto adapter is implemented on \`main\`." "$ALIGN_DOC" \
  || fail "in-toto section should state adapter status precisely"

grep -Fq "DB-backed \`scorecard\` and" docs/ROAD_MAP.md \
  || fail "roadmap should describe supported hosted analytics precisely"

grep -Fq "Hosted \`compare\` is not part of this contract yet." docs/guides/self-hosted-experimental-status.md \
  || fail "self-hosted status doc should keep hosted compare boundaries explicit"

grep -Fq "X-Evidra-API-Key" docs/guides/self-hosted-experimental-status.md \
  || fail "self-hosted status doc should document tenant-aware webhook routing"

echo "PASS: test_public_claims"
