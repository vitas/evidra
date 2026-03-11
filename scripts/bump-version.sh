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
server_json="$root_dir/server.json"
changelog="$root_dir/CHANGELOG.md"
today="$(date +%F)"

[[ -f "$version_go" ]] || fail "missing $version_go"
[[ -f "$server_json" ]] || fail "missing $server_json"
[[ -f "$changelog" ]] || fail "missing $changelog"

grep -Eq 'Version = "[0-9]+\.[0-9]+\.[0-9]+"' "$version_go" \
  || fail "could not find Version assignment in $version_go"
grep -Eq '"version": "[0-9]+\.[0-9]+\.[0-9]+"' "$server_json" \
  || fail "could not find top-level version in $server_json"
grep -Eq 'ghcr\.io/vitas/evidra-mcp:[0-9]+\.[0-9]+\.[0-9]+' "$server_json" \
  || fail "could not find MCP image tag in $server_json"
grep -Fq '## Unreleased' "$changelog" \
  || fail "could not find '## Unreleased' in $changelog"

perl -0pi -e 's/Version = "[0-9]+\.[0-9]+\.[0-9]+"/Version = "'"$version"'"/' "$version_go"
perl -0pi -e 's/"version": "[0-9]+\.[0-9]+\.[0-9]+"/"version": "'"$version"'"/' "$server_json"
perl -0pi -e 's#(ghcr\.io/vitas/evidra-mcp:)[0-9]+\.[0-9]+\.[0-9]+#${1}'"$version"'#' "$server_json"

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

grep -Fq "Version = \"$version\"" "$version_go" \
  || fail "failed to update $version_go"
grep -Fq "\"version\": \"$version\"" "$server_json" \
  || fail "failed to update version in $server_json"
grep -Fq "ghcr.io/vitas/evidra-mcp:$version" "$server_json" \
  || fail "failed to update MCP image tag in $server_json"
