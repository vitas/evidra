#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Tracked files, not the working tree. Finder writes .DS_Store into directories whenever
# someone browses them - including ignored ones like output/ - and a repository guard that
# fails on local noise trains people to disregard repository guards. What must be true is
# that no junk is tracked or staged, because that is what leaves the machine on a push.
if git ls-files | grep -q -E '(^|/)\.DS_Store$'; then
  fail "tracked Finder junk files present (git ls-files)"
fi
if git diff --cached --name-only | grep -q -E '(^|/)\.DS_Store$'; then
  fail "staged Finder junk files present"
fi
if git ls-files | grep -q -i '__MACOSX\|~\$'; then
  fail "archive or office lock junk tracked"
fi

if [[ -d docs/plans/done/archive ]]; then
  fail "docs/plans/done/archive should be deleted"
fi

echo "PASS: test_repo_cleanup_hygiene"
