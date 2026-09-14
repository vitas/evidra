#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

legacy_term='g'"olden"
legacy_env='EVIDRA_UPDATE_G'"OLDEN"
legacy_test='TestG'"olden"
legacy_dir_var="${legacy_term}Dir"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

if [[ -d "tests/${legacy_term}" ]]; then
  fail "legacy canonicalization fixture directory still exists"
fi

# Patterns are POSIX ERE without `\b`: the runner's grep is GNU and the development one is BSD,
# and `\b` exists only on the former. Boundaries are spelled as character classes instead, which
# both accept.
word_pat="(^|[^_[:alnum:]])${legacy_term}([^_[:alnum:]]|\$)"
path_pat="tests/${legacy_term}|/${legacy_term}/|${legacy_dir_var}|${legacy_env}|${legacy_test}"
pattern="${word_pat}|${path_pat}"

# The active set has to be present, and has to be named explicitly. This guard used to list
# `internal` and `scripts` as scan roots; the §43 prune deleted both, and grep's non-zero exit for
# "no such directory" read as "no matches", so the check that still catches a legacy name in a
# README has been quietly checking nothing on every run since. A scan root that can disappear
# without failing is not a scan.
targets=(README.md docs tests .github Makefile)
for t in "${targets[@]}"; do
  [[ -e "$t" ]] || fail "scan root '$t' is gone; this guard would search nothing"
done
# Historical trees, scanned when they exist.
for t in internal scripts; do
  [[ -e "$t" ]] && targets+=("$t")
done

set +e
hits="$(grep -rEn --binary-files=without-match \
  --exclude-dir=plans --exclude-dir=done \
  --exclude="$(basename "${BASH_SOURCE[0]}")" \
  -- "$pattern" "${targets[@]}")"
rc=$?
set -e
[[ $rc -le 1 ]] || fail "scan failed with rc=$rc (a grep error is not a clean result)"

if [[ -n "$hits" ]]; then
  echo "$hits" | head -10 >&2
  fail "legacy fixture naming remains in active repo files"
fi

echo "PASS: fixture and snapshot naming is consistent"
