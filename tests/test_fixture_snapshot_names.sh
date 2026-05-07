#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

legacy_term='g'"olden"
legacy_env='EVIDRA_UPDATE_G'"OLDEN"
legacy_test='TestG'"olden"
legacy_dir_var="${legacy_term}Dir"
legacy_word_pattern="\\b${legacy_term}\\b"
legacy_path_pattern="tests/${legacy_term}|/${legacy_term}/|${legacy_dir_var}|${legacy_env}|${legacy_test}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

if [[ -d "tests/${legacy_term}" ]]; then
  fail "legacy canonicalization fixture directory still exists"
fi

if rg -n "${legacy_word_pattern}|${legacy_path_pattern}" README.md docs tests internal scripts .github Makefile \
  --glob '!docs/plans/**' \
  --glob '!docs/system-design/done/**' \
  --glob '!tests/test_fixture_snapshot_names.sh' >/dev/null; then
  fail "legacy fixture naming remains in active repo files"
fi

echo "PASS: fixture and snapshot naming is consistent"
