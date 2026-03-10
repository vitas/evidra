#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

workflow=".github/workflows/dco.yml"
[[ -f "$workflow" ]] || fail "DCO workflow should exist"

grep -Fq 'GITHUB_EVENT_PATH' "$workflow" \
  || fail "DCO workflow should read push commits from GITHUB_EVENT_PATH"

grep -Fq '.commits[].id' "$workflow" \
  || fail "DCO workflow should derive push commit IDs from the event payload"

if grep -Fq 'range="${BEFORE_SHA}..${HEAD_SHA}"' "$workflow"; then
  fail "DCO workflow should not build push commit ranges from BEFORE_SHA"
fi

echo "PASS: test_dco_force_push_handling"
