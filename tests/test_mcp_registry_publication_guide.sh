#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "mcp registry publication guide check failed: $*" >&2
  exit 1
}

# Both helpers check the file's existence before its content. The first version of this guard
# searched with `rg`, which the GitHub runner does not have: `assert_contains` then failed loudly
# ("missing 'Docker MCP Registry'"), while `assert_not_contains` read "command not found" as "the
# forbidden text is absent" and passed. A guard whose negative half can pass because a binary is
# missing is worse than the checks it replaced, so the tool is grep and an unreadable file is a
# failure in both directions.
assert_contains() {
  local pattern="$1"
  local path="$2"
  [[ -f "$path" ]] || fail "missing $path (cannot look for '$pattern')"
  if ! grep -qF -- "$pattern" "$path"; then
    fail "missing '$pattern' in $path"
  fi
}

assert_not_contains() {
  local pattern="$1"
  local path="$2"
  [[ -f "$path" ]] || fail "missing $path (cannot confirm '$pattern' is absent)"
  if grep -qF -- "$pattern" "$path"; then
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
