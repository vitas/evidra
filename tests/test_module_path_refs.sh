#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

canonical_module="github.com/vitas/evidra"
legacy_module="samebits.com/evidra"

if git grep -n -F "$legacy_module" -- . \
  ":(exclude)CHANGELOG.md" \
  ":(exclude)docs/plans/**" \
  ":(exclude)tests/test_module_path_refs.sh" >/tmp/test-module-path-refs.out 2>/dev/null; then
  cat /tmp/test-module-path-refs.out >&2
  fail "old module path references remain"
fi

IFS= read -r module_line <go.mod
[[ "$module_line" == "module $canonical_module" ]] \
  || fail "go.mod first line must be: module $canonical_module"

grep -Fq '"github.com/vitas/evidra/pkg/proxy"' cmd/evidra-mcp/main.go \
  || fail "evidra-mcp should import proxy from the current module"

for public_package in pkg/evidence pkg/report; do
  git grep -Fq "github.com/vitas/evidra/$public_package" -- README.md docs/architecture.md \
    || fail "$public_package import path must be documented in README and Architecture"
done

grep -Fq '`pkg/proxy` is an internal implementation package' docs/architecture.md \
  || fail "Architecture must describe the pkg/proxy stability boundary"

grep -Fq 'main: ./cmd/evidra-mcp' .goreleaser.yaml \
  || fail "release configuration should build the current evidra-mcp command"

echo "PASS: test_module_path_refs"
