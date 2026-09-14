#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

[[ -f GOVERNANCE.md ]] || fail "GOVERNANCE.md should exist"
[[ -f OWNERS ]] || fail "OWNERS should exist"
[[ -f .github/PULL_REQUEST_TEMPLATE.md ]] || fail "PR template should exist"
[[ -f .github/workflows/dco.yml ]] || fail "DCO workflow should exist"
# The corpus promotion and artifact-layout guards were removed together with the
# pre-vNext acceptance corpus they protected (see docs/system-design/vnext-prune-record.md).
# What remains as the baseline is the surface that still makes public claims.
[[ -x tests/test_doc_trust_alignment.sh ]] || fail "doc trust alignment guard should exist"
[[ -x tests/test_public_claims.sh ]] || fail "public claims guard should exist"

grep -Fq "Developer Certificate of Origin" CONTRIBUTING.md \
  || fail "CONTRIBUTING.md should document the DCO policy"

grep -Fq "Signed-off-by:" CONTRIBUTING.md \
  || fail "CONTRIBUTING.md should explain commit sign-offs"

grep -Fq "Signed-off-by:" .github/PULL_REQUEST_TEMPLATE.md \
  || fail "PR template should remind contributors about sign-off"

grep -Fq "git interpret-trailers --parse" .github/workflows/dco.yml \
  || fail "DCO workflow should validate commit sign-off trailers"

echo "PASS: test_governance_baseline"
