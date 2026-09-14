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

# Tracked files must not be ignored files. This is how committed binaries happen: a .gitignore
# entry is added, and a later blanket `git add -A` of the same paths re-stages them - tracked
# wins over ignore, so the entry looks effective (status stays clean) while the file is still
# in the index and in the next commit. One commit here claimed to untrack two binaries and
# changed only .gitignore.
tracked_ignored="$(git ls-files -i -c --exclude-standard)"
if [[ -n "$tracked_ignored" ]]; then
  printf '%s\n' "$tracked_ignored" | sed 's/^/  /' >&2
  fail "tracked files that .gitignore says to ignore (git rm --cached them, or drop the pattern)"
fi

# An untracked executable at the repository root is how a tracked binary starts. Both
# committed binaries below were produced by a bare `go build ./cmd/X`, which writes the
# executable next to go.mod; nothing in the tree needs an executable there, bin/ is ignored and
# is where builds belong, and every guard here is about what reaches history - but a root binary
# is one `git add` away from it, and the two that got in were added exactly that way.
while IFS= read -r -d '' entry; do
  path="${entry:3}"
  [[ "$path" == */* ]] && continue
  [[ -f "$path" && -x "$path" ]] || continue
  fail "untracked executable '$path' in the repository root: build into bin/ instead (go build -o bin/${path} ./cmd/${path}/)"
done < <(git status --porcelain --untracked-files=all -z)

# No large binaries in the index: 1 MiB per file is the report threshold, and both history
# offenders found so far were 9.x MB compiled Go binaries.
while read -r size path; do
  if (( size > 1048576 )); then
    echo "  $path: $size bytes" >&2
    fail "a tracked file exceeds 1 MiB - build artifacts belong in bin/, which is ignored"
  fi
done < <(git ls-files -s | awk '{print $2}' | git cat-file --batch-check='%(objectsize) %(rest)' | awk '$1 > 1048576')

if [[ -d docs/plans/done/archive ]]; then
  fail "docs/plans/done/archive should be deleted"
fi

echo "PASS: test_repo_cleanup_hygiene"
