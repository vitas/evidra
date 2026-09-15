#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# The module's package graph is declared in tests/core-packages.txt and must match
# `go list ./...` exactly, in both directions.
#
# This replaces a count floor (an old MIN_PACKAGES=8 environment gate) that passed while the module had 9
# packages - one of which was a dead root package nothing imported. A floor has the wrong
# shape for the thing it is protecting: it catches "the graph collapsed to nothing" but not
# "a package left the graph and nobody thought about it", and it silently becomes a lie
# several deletions later. An exact set makes every removal a deliberate edit to this file.

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

[[ -f tests/core-packages.txt ]] || fail "tests/core-packages.txt is missing"

# Strip comments before checking active command surfaces, then limit Markdown
# checks to fenced command blocks so explanatory prose cannot satisfy the guard.
active_make="$(sed -E 's/[[:space:]]*#.*$//' Makefile)"
active_ci="$(sed -E 's/[[:space:]]*#.*$//' .github/workflows/ci.yml)"
contributor_commands="$({
  awk '/^```/ { in_fence = !in_fence; next } in_fence { print }' CONTRIBUTING.md
} | sed -E 's/[[:space:]]*#.*$//')"
validation_commands="$({
  awk '/^```/ { in_fence = !in_fence; next } in_fence { print }' docs/validation.md
} | sed -E 's/[[:space:]]*#.*$//')"
claude_commands="$({
  awk '/^```/ { in_fence = !in_fence; next } in_fence { print }' CLAUDE.md
} | sed -E 's/[[:space:]]*#.*$//')"

scoped_core_test='^[[:space:]]*go[[:space:]]+test([[:space:]]+[^[:space:]]+)*[[:space:]]+\./cmd/\.\.\.[[:space:]]+\./pkg/\.\.\.([[:space:]]|$)'
grep -Eq "$scoped_core_test" <<< "$validation_commands" \
  || fail "docs/validation.md must include an executable Core-scoped go test command"
grep -Eq "$scoped_core_test" <<< "$claude_commands" \
  || fail "CLAUDE.md must include an executable Core-scoped go test command"

grep -Fxq 'GO_PACKAGES := ./cmd/... ./pkg/...' <<< "$active_make" \
  || fail "Makefile must define the exact Core package scope"
awk '
  $0 == "test:" { in_test = 1; next }
  in_test && /^[^[:space:]#][^:]*:/ { in_test = 0 }
  in_test && /^\tgo test \$\(GO_PACKAGES\)([[:space:]]|$)/ { found = 1 }
  END { exit(found ? 0 : 1) }
' <<< "$active_make" \
  || fail "the Makefile test recipe must use GO_PACKAGES"

grep -Eq '^[[:space:]]*run:[[:space:]]+go[[:space:]]+test[[:space:]]+\./cmd/\.\.\.[[:space:]]+\./pkg/\.\.\.([[:space:]]|$)' <<< "$active_ci" \
  || fail "CI must run the literal Core test scope"
grep -Eq '^[[:space:]]*go[[:space:]]+test[[:space:]]+\./cmd/\.\.\.[[:space:]]+\./pkg/\.\.\.([[:space:]]|$)' <<< "$contributor_commands" \
  || fail "CONTRIBUTING.md must include the literal Core test command"

bare_all_packages_test='(^|[[:space:]])go[[:space:]]+test([[:space:]]+[^[:space:]]+)*[[:space:]]+\./\.\.\.([[:space:]]|$)'
if grep -Eq "$bare_all_packages_test" <<< "$active_ci"; then
  fail ".github/workflows/ci.yml must not run go test against bare ./..."
fi
if grep -Eq "$bare_all_packages_test" <<< "$contributor_commands"; then
  fail "CONTRIBUTING.md must not run go test against bare ./..."
fi
if grep -Eq "$bare_all_packages_test" <<< "$validation_commands"; then
  fail "docs/validation.md must not run go test against bare ./..."
fi
if grep -Eq "$bare_all_packages_test" <<< "$claude_commands"; then
  fail "CLAUDE.md must not run go test against bare ./..."
fi

# node_modules is third-party JavaScript that sometimes ships Go source of its own
# (flatted ships golang/pkg/flatted). That code is not part of this module's graph,
# and a guard whose verdict depends on which npm package was installed today is not
# a guard. Everything else in the module must match the declared set exactly.
actual="$(go list ./... 2>/dev/null | grep -v '/node_modules/' | sort -u)"
[[ -n "$actual" ]] || fail "go list ./... returned nothing (broken module?)"

declared="$(sort -u tests/core-packages.txt)"

missing="$(comm -13 <(printf '%s\n' "$actual") <(printf '%s\n' "$declared"))"
extra="$(comm -23 <(printf '%s\n' "$actual") <(printf '%s\n' "$declared"))"

if [[ -n "$missing" ]]; then
  echo "declared in tests/core-packages.txt but absent from the module:" >&2
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
  dir="${pkg#github.com/vitas/evidra/}"
  [[ -d "$dir" ]] || fail "declared package $pkg has no directory $dir/"
  comp="$(go list -e -f '{{if .Error}}BAD{{end}}' "$pkg" 2>/dev/null)"
  [[ "$comp" == "BAD" ]] && fail "go list reports an error for $pkg"
done

echo "PASS: package graph matches tests/core-packages.txt ($(printf '%s\n' "$declared" | grep -c .) packages)"
