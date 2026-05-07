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

if [ -d tests/benchmark ]; then
  fail_found "legacy benchmark corpus still present in core"
fi

if [ -e scripts/bench-add.sh ]; then
  fail_found "legacy benchmark corpus helper still present in core"
fi

if grep -nE '^[[:space:]]*(benchmark-[A-Za-z0-9_-]+|bench-add):' Makefile 2>/dev/null; then
  fail_found "legacy benchmark Make targets still present in core"
fi

if grep -rnE 'tests/benchmark|benchmark-validate|benchmark-check-contracts|benchmark-coverage|bench-add' .github/workflows 2>/dev/null; then
  fail_found "legacy benchmark CI/release hooks still present in core"
fi

if rg -n 'infra-bench|evidra-infra-bench|lab\.evidra\.cc|/v1/bench|/v1/benchmark|BenchmarkService' \
  README.md docs cmd internal pkg scripts tests .github ui Makefile \
  --glob '!docs/plans/**' \
  --glob '!docs/product/**' \
  --glob '!tests/test_no_legacy_benchmark_surface.sh' \
  --glob '!tests/test_unified_artifact_layout.sh' \
  --glob '!tests/test_acceptance_corpus_promotion.sh' \
  --glob '!tests/test_module_path_refs.sh' \
  --glob '!cmd/evidra/command_registry_test.go' \
  --glob '!internal/db/db_test.go' \
  --glob '!ui/package-lock.json' 2>/dev/null; then
  fail_found "core active tree still has bench/lab-specific references"
fi

if find internal/db/migrations -maxdepth 1 -type f \( -name '*bench*' -o -name '*benchmark*' \) -print -quit | grep -q .; then
  find internal/db/migrations -maxdepth 1 -type f \( -name '*bench*' -o -name '*benchmark*' \) -print >&2
  fail_found "core migration filenames should not carry bench-specific names"
fi

if find internal/db/migrations -maxdepth 1 -type f \( -name '*core_compatibility*' -o -name '*drop_legacy*' -o -name '*legacy*' \) -print -quit | grep -q .; then
  find internal/db/migrations -maxdepth 1 -type f \( -name '*core_compatibility*' -o -name '*drop_legacy*' -o -name '*legacy*' \) -print >&2
  fail_found "core migrations should describe only the current evidra schema"
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
