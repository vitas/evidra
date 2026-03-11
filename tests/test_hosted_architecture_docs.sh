#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "hosted architecture docs check failed: $*" >&2
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

assert_not_contains "analytics experimental" "ui/src/pages/Landing.tsx"
assert_not_contains "use CLI or MCP for authoritative analytics" "ui/src/pages/Landing.tsx"
assert_contains "ArgoCD / generic webhooks" "ui/src/pages/Landing.tsx"
assert_contains "decision_context" "ui/src/pages/Landing.tsx"

assert_contains "Protocol Sequence" "cmd/evidra-api/static/index.html"
assert_not_contains "H[\"Exit Code\"] --> I[\"Report\"]" "cmd/evidra-api/static/index.html"
assert_contains "ArgoCD / generic webhooks" "cmd/evidra-api/static/index.html"
assert_contains "decision_context" "cmd/evidra-api/static/index.html"

assert_contains "## Hosted Mode" "docs/ARCHITECTURE.md"
assert_contains "webhook ingestion" "docs/ARCHITECTURE.md"
assert_contains "## Self-Hosted Mode" "docs/system-design/V1_ARCHITECTURE.md"
assert_contains "deliberate refusal" "docs/system-design/V1_ARCHITECTURE.md"

echo "hosted architecture docs checks passed"
