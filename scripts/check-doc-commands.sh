#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "doc check failed: $*" >&2
  exit 1
}

has_pattern() {
  local pattern="$1"
  local file="$2"
  if command -v rg >/dev/null 2>&1; then
    rg -q --fixed-strings -- "$pattern" "$file"
    return
  fi
  grep -Fq -- "$pattern" "$file"
}

require_pattern() {
  local file="$1"
  local pattern="$2"
  if ! has_pattern "$pattern" "$file"; then
    fail "missing pattern '$pattern' in $file"
  fi
}

require_pattern "README.md" "EVIDRA_SIGNING_MODE"
require_pattern "README.md" "make test-mcp-inspector"
require_pattern "README.md" "docs/integrations/CLI_REFERENCE.md"
require_pattern "docs/integrations/CLI_REFERENCE.md" "evidra-exp artifact run"
require_pattern "docs/integrations/CLI_REFERENCE.md" "--delay-between-runs"
require_pattern "docs/integrations/CLI_REFERENCE.md" "evidra-mcp"
require_pattern "docs/integrations/CLI_REFERENCE.md" "evidra prescribe"
require_pattern "docs/integrations/SCANNER_SARIF_QUICKSTART.md" "--signing-mode optional"
require_pattern "tests/inspector/README.md" "EVIDRA_LOCAL_API_URL"
require_pattern "tests/inspector/README.md" "EVIDRA_SIGNING_MODE=optional"
require_pattern "server.json" "\"name\": \"EVIDRA_SIGNING_MODE\""
require_pattern "server.json" "\"name\": \"EVIDRA_SIGNING_KEY\""
require_pattern "server.json" "\"name\": \"EVIDRA_SIGNING_KEY_PATH\""

# Smoke-check documented command paths with local optional signing mode.
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

cat >"$tmpdir/artifact.json" <<'JSON'
{"noop":true}
JSON

go run ./cmd/evidra-mcp --help >/dev/null
go run ./cmd/evidra-exp --help >/dev/null
go run ./cmd/evidra-exp artifact --help >/dev/null
go run ./cmd/evidra-exp execution --help >/dev/null

prescribe_out="$(go run ./cmd/evidra prescribe \
  --tool terraform \
  --artifact "$tmpdir/artifact.json" \
  --canonical-action '{"tool":"terraform","operation":"apply","operation_class":"mutate","scope_class":"production","resource_count":1,"resource_shape_hash":"sha256:test"}' \
  --signing-mode optional \
  --evidence-dir "$tmpdir/evidence")"

prescription_id="$(
  printf '%s\n' "$prescribe_out" \
    | sed -nE 's/.*"prescription_id"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' \
    | head -n1
)"
if [[ -z "${prescription_id:-}" ]]; then
  fail "could not parse prescription_id from prescribe output"
fi

go run ./cmd/evidra report \
  --prescription "$prescription_id" \
  --verdict success \
  --exit-code 0 \
  --signing-mode optional \
  --evidence-dir "$tmpdir/evidence" >/dev/null

echo "doc checks passed"
