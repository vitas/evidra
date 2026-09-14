#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "mcp registry publication guide check failed: $*" >&2
  exit 1
}

assert_contains() {
  local pattern="$1"
  local path="$2"
  if ! rg -q --fixed-strings -- "$pattern" "$path"; then
    fail "missing '$pattern' in $path"
  fi
}

assert_not_contains() {
  local pattern="$1"
  local path="$2"
  if rg -q --fixed-strings -- "$pattern" "$path"; then
    fail "found forbidden '$pattern' in $path"
  fi
}

GUIDE="docs/guides/mcp-registry-publication.md"

[[ -f "$GUIDE" ]] || fail "missing $GUIDE"

assert_contains "Docker MCP Registry" "$GUIDE"
assert_contains "MCP Registry" "$GUIDE"
assert_contains "docker/mcp-registry" "$GUIDE"
assert_contains "server.json" "$GUIDE"
assert_contains "io.github.vitas/evidra" "$GUIDE"
assert_contains "ghcr.io/vitas/evidra-mcp" "$GUIDE"
assert_contains "prescribe" "$GUIDE"
assert_contains "report" "$GUIDE"
assert_contains "get_event" "$GUIDE"
assert_contains "local-first" "$GUIDE"
assert_contains "EVIDRA_URL" "$GUIDE"
assert_contains "EVIDRA_API_KEY" "$GUIDE"

assert_not_contains "Evidra-Lock" "$GUIDE"
assert_not_contains "Embedded OPA bundle" "$GUIDE"
assert_not_contains "deny-cache" "$GUIDE"
assert_not_contains "validate before destructive" "$GUIDE"

assert_contains "MCP Registry Publication Guide" "README.md"
assert_contains "docs/guides/mcp-registry-publication.md" "README.md"

echo "mcp registry publication guide checks passed"
