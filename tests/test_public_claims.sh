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

grep -Fq "Self-hosted analytics endpoints remain experimental" docs/ROAD_MAP.md \
  || fail "roadmap should keep hosted analytics boundaries explicit"

grep -Fq "501 Not Implemented" docs/guides/self-hosted-experimental-status.md \
  || fail "self-hosted status doc should state current endpoint behavior"

echo "PASS: test_public_claims"
