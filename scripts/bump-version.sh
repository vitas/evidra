#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "bump-version.sh: $*" >&2
  exit 1
}

if [[ $# -ne 1 ]]; then
  fail "usage: $0 <semver>"
fi

version="$1"
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  fail "version must match X.Y.Z"
fi

if [[ -n "${BUMP_VERSION_ROOT:-}" ]]; then
  root_dir="$BUMP_VERSION_ROOT"
else
  root_dir="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi

version_go="$root_dir/pkg/version/version.go"
changelog="$root_dir/CHANGELOG.md"
today="$(date +%F)"

[[ -f "$version_go" ]] || fail "missing $version_go"
[[ -f "$changelog" ]] || fail "missing $changelog"

if grep -Eq 'BaseVersion = "[0-9]+\.[0-9]+\.[0-9]+"' "$version_go"; then
  version_symbol='BaseVersion'
elif grep -Eq 'Version = "[0-9]+\.[0-9]+\.[0-9]+"' "$version_go"; then
  version_symbol='Version'
else
  fail "could not find BaseVersion or Version assignment in $version_go"
fi

if ! grep -Fq '## Unreleased' "$changelog"; then
  tmp_file="$(mktemp)"
  awk '
    NR == 1 {
      print
      print ""
      print "## Unreleased"
      print ""
      next
    }
    { print }
  ' "$changelog" >"$tmp_file"
  mv "$tmp_file" "$changelog"
fi

perl -0pi -e 's/'"$version_symbol"' = "[0-9]+\.[0-9]+\.[0-9]+"/'"$version_symbol"' = "'"$version"'"/' "$version_go"

heading="## v$version — $today"
if ! grep -Fq "$heading" "$changelog"; then
  tmp_file="$(mktemp)"
  awk -v heading="$heading" '
    {
      print
      if (!inserted && $0 == "## Unreleased") {
        print ""
        print heading
        inserted = 1
      }
    }
  ' "$changelog" >"$tmp_file"
  mv "$tmp_file" "$changelog"
fi

grep -Fq "$version_symbol = \"$version\"" "$version_go" \
  || fail "failed to update $version_go"
