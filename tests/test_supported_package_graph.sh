#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# The module's package graph is declared in tests/vnext-packages.txt and must match
# `go list ./...` exactly, in both directions.
#
# This replaces a count floor (VNEXT_MIN_PACKAGES=8) that passed while the module had 9
# packages - one of which was a dead root package nothing imported. A floor has the wrong
# shape for the thing it is protecting: it catches "the graph collapsed to nothing" but not
# "a package left the graph and nobody thought about it", and it silently becomes a lie
# several deletions later. An exact set makes every removal a deliberate edit to this file.

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

[[ -f tests/vnext-packages.txt ]] || fail "tests/vnext-packages.txt is missing"

# node_modules is third-party JavaScript that sometimes ships Go source of its own
# (flatted ships golang/pkg/flatted). That code is not part of this module's graph,
# and a guard whose verdict depends on which npm package was installed today is not
# a guard. Everything else in the module must match the declared set exactly.
actual="$(go list ./... 2>/dev/null | grep -v '/node_modules/' | sort -u)"
[[ -n "$actual" ]] || fail "go list ./... returned nothing (broken module?)"

declared="$(sort -u tests/vnext-packages.txt)"

missing="$(comm -13 <(printf '%s\n' "$actual") <(printf '%s\n' "$declared"))"
extra="$(comm -23 <(printf '%s\n' "$actual") <(printf '%s\n' "$declared"))"

if [[ -n "$missing" ]]; then
  echo "declared in tests/vnext-packages.txt but absent from the module:" >&2
  printf '  %s\n' $missing >&2
  fail "the declared package graph is stale"
fi
if [[ -n "$extra" ]]; then
  echo "present in the module but not declared:" >&2
  printf '  %s\n' $extra >&2
  fail "a package joined the graph without being declared (add it deliberately, or delete it)"
fi

# Every declared package must exist as a directory with Go files in it.
for pkg in $declared; do
  dir="${pkg#samebits.com/evidra/}"
  [[ -d "$dir" ]] || fail "declared package $pkg has no directory $dir/"
  comp="$(go list -e -f '{{if .Error}}BAD{{end}}' "$pkg" 2>/dev/null)"
  [[ "$comp" == "BAD" ]] && fail "go list reports an error for $pkg"
done

echo "PASS: package graph matches tests/vnext-packages.txt ($(printf '%s\n' "$declared" | grep -c .) packages)"
