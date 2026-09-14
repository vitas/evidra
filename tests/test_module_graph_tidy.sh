#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Is go.mod the tidy form of the code that is actually here?
#
# This went unguarded through the whole §43 prune. `make tidy` existed, docs/ARCHITECTURE.md named
# `modelcontextprotocol/go-sdk` as a dependency, and go.mod still required go-sdk, jsonschema-go,
# terraform-json, yaml, protobuf and five otel modules - none of which any surviving file imports.
# The consequence was not untidiness, it was misinformation: dependabot opened bumps against those
# packages, they merged green, and each looked like a change to the shipped binary that the shipped
# binary never linked. `go mod tidy` is also how a deleted package's dependencies leave the
# repository; with no check, they never do.
#
# The originals are saved aside and restored afterwards, so running this guard is not an edit: a
# check that dirties the tree it audits gets run only when someone is prepared to lose work.

status=0
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

cp go.mod "$workdir/go.mod.orig"
cp go.sum "$workdir/go.sum.orig"

if ! out="$(go mod tidy 2>&1)"; then
  echo "FAIL: go mod tidy did not run cleanly:"
  echo "$out" | sed 's/^/        /'
  status=1
fi

for f in go.mod go.sum; do
  if ! diff -q "$workdir/$f.orig" "$f" >/dev/null 2>&1; then
    echo "FAIL: $f is not in tidy form for the code in this tree"
    diff "$workdir/$f.orig" "$f" | sed 's/^/        /' | head -30
    status=1
  fi
done

cp "$workdir/go.mod.orig" go.mod
cp "$workdir/go.sum.orig" go.sum

if [[ $status -ne 0 ]]; then
  echo "  fix: run 'go mod tidy' and commit go.mod/go.sum in the change that moved them."
  exit 1
fi
echo "PASS: test_module_graph_tidy.sh"
