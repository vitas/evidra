#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "cli rebranding check failed: $*" >&2
  exit 1
}

assert_contains() {
  local pattern="$1"
  local path="${2:-README.md}"
  if ! rg -q --fixed-strings -- "$pattern" "$path"; then
    fail "missing '$pattern' in $path"
  fi
}

assert_not_contains() {
  local pattern="$1"
  shift
  if rg -n --fixed-strings \
    -g '!docs/plans/**' \
    -g '!docs/research/**' \
    -g '!docs/system-design/backlog/**' \
    -g '!docs/system-design/internal/**' \
    -g '!tests/test_cli_command_rebranding.sh' \
    -- "$pattern" "$@" >/dev/null; then
    fail "found forbidden '$pattern' in $*"
  fi
}

assert_contains "evidra record" "README.md"
assert_contains "evidra import" "README.md"
assert_contains "evidra import-findings" "docs/integrations/scanner-sarif-quickstart.md"
assert_contains "evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml" "README.md"
assert_contains "evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml" "ui/src/pages/Landing.tsx"
assert_contains "evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml" "cmd/evidra-api/static/index.html"
assert_contains "evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml" "docs/integrations/cli-reference.md"

assert_not_contains "evidra run" README.md docs ui tests cmd
assert_not_contains "evidra ingest-findings" README.md docs ui tests cmd

echo "cli rebranding checks passed"
