#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

expected="$({
  printf '%s\n' \
    docs/architecture.md \
    docs/cli-reference.md \
    docs/evidence-format.md \
    docs/getting-started.md \
    docs/validation.md
})"

actual="$({
  git ls-files docs |
    grep -E '\.md$' |
    grep -v '^docs/plans/' |
    sort
})"

if [[ "$actual" != "$expected" ]]; then
  echo "Expected live public docs:" >&2
  printf '%s\n' "$expected" >&2
  echo "Actual live public docs:" >&2
  printf '%s\n' "$actual" >&2
  fail "public documentation tree is not canonical"
fi

while IFS= read -r path; do
  [[ -s "$path" ]] || fail "$path is missing or empty"
done <<<"$expected"

docs_root='docs'
stale_pattern="${docs_root}/(ARCHITECTURE|guides|integrations|proposals|system-design)(/|\\.md)|${docs_root}/external-evidence-bundle-v1\\.md|${docs_root}/supported-tools\\.md"
stale_refs="$({
  git grep -nE \
    "$stale_pattern" \
    -- . \
    ':!CHANGELOG.md' \
    ':!docs/plans/**' || true
})"
if [[ -n "$stale_refs" ]]; then
  printf '%s\n' "$stale_refs" >&2
  fail "tracked current-state files still reference removed documentation"
fi

echo "PASS: test_public_docs_structure"
