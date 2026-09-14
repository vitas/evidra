#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

if find . -name '.DS_Store' -print -quit | grep -q .; then
  fail "repo should not contain Finder junk files"
fi

if [[ -d docs/plans/done/archive ]]; then
  fail "docs/plans/done/archive should be deleted"
fi

echo "PASS: test_repo_cleanup_hygiene"
