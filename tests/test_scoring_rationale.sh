#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

RationaleDoc="docs/system-design/scoring/default.v1.1.0.md"

[[ -f "$RationaleDoc" ]] || fail "missing scoring rationale document: $RationaleDoc"

grep -Fq "default.v1.1.0" "$RationaleDoc" \
  || fail "scoring rationale should name the active default profile"

grep -Fq "scoring/default.v1.1.0.md" README.md \
  || fail "README should link to the scoring rationale"

grep -Fq "heuristic" "$RationaleDoc" \
  || fail "scoring rationale should distinguish heuristic choices from normative ones"

echo "PASS: test_scoring_rationale"
