#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

FAILED=0

fail_found() {
  echo "ERROR: $1" >&2
  FAILED=1
}

# Check for legacy benchmark routes in active non-test Go code.
if grep -rn --include='*.go' --exclude='*_test.go' '/v1/benchmark/' cmd/evidra-api internal/api pkg/client 2>/dev/null; then
  fail_found "legacy benchmark route still present in code"
fi

# Check for dead benchmark CLI stub
if [ -e cmd/evidra/benchmark.go ]; then
  fail_found "dead benchmark CLI stub still present"
fi

if [ -d pkg/signalaudit ]; then
  fail_found "signal audit package belongs in evidra-bench, not core"
fi

if find internal/db/migrations -maxdepth 1 -type f \( -name '*bench*' -o -name '*benchmark*' \) -print -quit | grep -q .; then
  find internal/db/migrations -maxdepth 1 -type f \( -name '*bench*' -o -name '*benchmark*' \) -print >&2
  fail_found "core migration filenames should not carry bench-specific names"
fi

if find docs/plans docs/product -type f \( -name '*bench*' -o -name '*benchmark*' \) -print -quit | grep -q .; then
  find docs/plans docs/product -type f \( -name '*bench*' -o -name '*benchmark*' \) -print >&2
  fail_found "bench planning docs belong in evidra-bench"
fi

if grep -rnE 'evidrabenchmark|Evidra Benchmark binaries|benchmark MCP server' \
  uiembed.go uiembed_embed.go cmd/evidra-api/main.go pkg/mcpserver/server.go pkg/version/version.go 2>/dev/null; then
  fail_found "core code still has benchmark naming leftovers"
fi

exit $FAILED
