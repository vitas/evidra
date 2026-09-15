#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

go_floor="$(
  sed -nE 's/^go ([0-9]+\.[0-9]+).*/\1/p' go.mod | head -n1
)"
if [[ -z "${go_floor:-}" ]]; then
  fail "could not parse Go version from go.mod"
fi

grep -Fq "Requires Go ${go_floor}+." CONTRIBUTING.md \
  || fail "CONTRIBUTING.md should advertise the same Go floor as go.mod"

# CONTRIBUTING may only name commands that exist.
for cmd in \
  "make build" \
  "make test" \
  "make lint" \
  "go vet ./..." \
  "go test -race ./pkg/evidence/... ./pkg/proxy/... ./pkg/report/... ./cmd/evidra-fixture/... ./cmd/evidra-gatea/..." \
  "bash tests/run_guards.sh" \
  "cd ui && npm ci" \
  "npm run lint" \
  "npm test" \
  "npm run build"
do
  grep -Fq "$cmd" CONTRIBUTING.md \
    || fail "CONTRIBUTING.md should list the existing command: $cmd"
done

for cmd in "make e2e" "make test-signals"; do
  if grep -Fq "$cmd" CONTRIBUTING.md; then
    fail "CONTRIBUTING.md should not reference a nonexistent target: $cmd"
  fi
done

grep -Fq "actively developed, unreleased" SECURITY.md \
  || fail "SECURITY.md should identify main as the active unreleased Core line"

grep -Fq "legacy releases (0.5.x" SECURITY.md \
  || fail "SECURITY.md should label the published release series as legacy"

grep -Fq "no implied feature or security support" SECURITY.md \
  || fail "SECURITY.md should say legacy releases get no implied support from main"

# CLAUDE.md may link only the five canonical documents and the plan corpus.
while IFS= read -r link; do
  case "$link" in
    docs/architecture.md|docs/cli-reference.md|docs/evidence-format.md|docs/getting-started.md|docs/validation.md|docs/plans/*) ;;
    *) fail "CLAUDE.md links outside the canonical document set: $link" ;;
  esac
done < <(grep -oE '\]\(docs/[^)]+' CLAUDE.md | sed 's/](//')

grep -Fq "not a scoring platform" CLAUDE.md \
  || fail "CLAUDE.md should state that Core is an MCP execution-evidence recorder, not a scoring platform"

if grep -Fq "retained for a future relocation" CLAUDE.md; then
  fail "CLAUDE.md should not keep the ui/ 'future relocation' claim"
fi

for file in README.md docs/cli-reference.md; do
  grep -Fq "Evidra does not sandbox the wrapped command" "$file" \
    || fail "$file should document the upstream execution boundary"
done

grep -Fq "Chain validity, signature validity, and evidence coverage are" docs/cli-reference.md \
  || fail "CLI reference should keep integrity and coverage conclusions separate"

grep -Fq "A successful upstream response is not proof of the" docs/cli-reference.md \
  || fail "CLI reference should not upgrade an MCP response into outcome proof"

grep -Fq "These are separate conclusions." docs/evidence-format.md \
  || fail "evidence trust model should distinguish chain, signature, and coverage"

for stale_phrase in \
  "agents actually did" \
  "Nothing beyond calling the wrapped server" \
  "blocks the call and records a protocol_violation" \
  "guarantees is that the execution is recorded" \
  "results are fingerprinted, never stored raw"
do
  if grep -RInF -- "$stale_phrase" ui/src >/dev/null; then
    fail "public UI should not contain stale trust wording: $stale_phrase"
  fi
done

grep -Fq "attempts to append a protocol_violation" ui/src/pages/Landing.tsx \
  || fail "landing enforcement copy should describe protocol_violation recording as an attempt"

echo "PASS: test_doc_trust_alignment"
