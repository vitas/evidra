#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

legacy_module="samebits.com/evidra""-benchmark"

if git grep -n "$legacy_module" -- . ":(exclude)docs/plans/**" >/tmp/test-module-path-refs.out 2>/dev/null; then
  cat /tmp/test-module-path-refs.out >&2
  fail "old module path references remain"
fi

grep -Eq '^module samebits.com/evidra$' go.mod \
  || fail "go.mod missing module samebits.com/evidra"

grep -Fq '"samebits.com/evidra/pkg/proxy"' cmd/evidra-mcp/main.go \
  || fail "evidra-mcp should import proxy from the current module"

grep -Fq 'main: ./cmd/evidra-mcp' .goreleaser.yaml \
  || fail "release configuration should build the current evidra-mcp command"

echo "PASS: test_module_path_refs"
