#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/bump-version.sh"

fail() {
  echo "bump-version test failed: $*" >&2
  exit 1
}

assert_contains() {
  local pattern="$1"
  local file="$2"
  if ! grep -Fq -- "$pattern" "$file"; then
    fail "missing '$pattern' in $file"
  fi
}

assert_not_contains() {
  local pattern="$1"
  local file="$2"
  if grep -Fq -- "$pattern" "$file"; then
    fail "unexpected '$pattern' in $file"
  fi
}

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$tmpdir/pkg/version"
cp "$ROOT_DIR/pkg/version/version.go" "$tmpdir/pkg/version/version.go"
cp "$ROOT_DIR/server.json" "$tmpdir/server.json"
cp "$ROOT_DIR/CHANGELOG.md" "$tmpdir/CHANGELOG.md"

BUMP_VERSION_ROOT="$tmpdir" bash "$SCRIPT_PATH" 0.4.7

assert_contains 'Version = "0.4.7"' "$tmpdir/pkg/version/version.go"
assert_not_contains 'Version = "0.4.6"' "$tmpdir/pkg/version/version.go"

assert_contains '"version": "0.4.7"' "$tmpdir/server.json"
assert_contains 'ghcr.io/vitas/evidra-mcp:0.4.7' "$tmpdir/server.json"
assert_not_contains '"version": "0.4.5"' "$tmpdir/server.json"
assert_not_contains 'ghcr.io/vitas/evidra-mcp:0.4.5' "$tmpdir/server.json"

assert_contains '## v0.4.7 — 2026-03-11' "$tmpdir/CHANGELOG.md"

invalid_tmp="$(mktemp -d)"
trap 'rm -rf "$tmpdir" "$invalid_tmp"' EXIT
mkdir -p "$invalid_tmp/pkg/version"
cp "$ROOT_DIR/pkg/version/version.go" "$invalid_tmp/pkg/version/version.go"
cp "$ROOT_DIR/server.json" "$invalid_tmp/server.json"
cp "$ROOT_DIR/CHANGELOG.md" "$invalid_tmp/CHANGELOG.md"

if BUMP_VERSION_ROOT="$invalid_tmp" bash "$SCRIPT_PATH" foo >/dev/null 2>&1; then
  fail "expected invalid semver input to fail"
fi

echo "bump-version tests passed"
