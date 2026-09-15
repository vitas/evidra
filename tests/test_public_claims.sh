#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# CHANGELOG.md is historical by design. Public orientation, security, canonical docs, and
# shipped UI copy must describe only the current Core surface.
public_files=(README.md SECURITY.md docs/*.md ui/index.html)
forbidden='DevOps MCP Server|reliability scoring|scorecards?|hosted API|X-Evidra-API-Key|EVIDRA_SIGNING_MODE|EVIDRA_SIGNING_KEY|EVIDRA_ENVIRONMENT'

if hits="$(grep -InEi -- "$forbidden" "${public_files[@]}" 2>/dev/null)"; then
  printf '%s\n' "$hits" >&2
  fail "public documents contain obsolete product claims or variables"
fi

# Type names and legacy request fields in the soon-to-be-removed application are not prose
# claims. Scan all UI source for the unsupported product phrases and the user-facing scoring
# spellings that appeared in rendered strings.
ui_forbidden='DevOps MCP Server|reliability scoring|hosted API|X-Evidra-API-Key|EVIDRA_SIGNING_MODE|EVIDRA_SIGNING_KEY|EVIDRA_ENVIRONMENT|Scoring Engine|Scoring 0-100|View the scorecard|evidra scorecard|hosted scorecard|scorecard \(0-100'
if hits="$(grep -RInEi -- "$ui_forbidden" ui/src 2>/dev/null)"; then
  printf '%s\n' "$hits" >&2
  fail "UI source contains obsolete current-state product claims or variables"
fi

# Stale distribution metadata must not advertise a product that no longer exists.
[[ ! -e server.json ]] || fail "stale MCP Registry manifest must not advertise the pre-vNext image"

if grep -Fq "cd ui && npm install && npm run build" .goreleaser.yaml; then
  fail "binary release must not build an unembedded marketing site"
fi

echo "PASS: test_public_claims"
