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
# Historical corpus guards were removed with the obsolete product surface they protected.
# What remains as the baseline is the surface that still makes current public claims.
[[ -x tests/test_doc_trust_alignment.sh ]] || fail "doc trust alignment guard should exist"
[[ -x tests/test_public_claims.sh ]] || fail "public claims guard should exist"

# A green check that points at deleted things is worse than a red one, so the checks over the
# checks are part of the baseline: every workflow declared, every reference resolved, the
# package graph stated as a set rather than a floor, and one runner with no exclusion list.
[[ -x tests/run_guards.sh ]] || fail "guard runner should exist and be executable"
[[ -x tests/test_ci_workflows_resolve.sh ]] || fail "CI reference resolver guard should exist"
[[ -x tests/test_supported_package_graph.sh ]] || fail "package graph guard should exist"
[[ -f tests/vnext-workflows.txt ]] || fail "workflow declaration list should exist"
[[ -f tests/vnext-packages.txt ]] || fail "declared package set should exist"
while IFS=$'\t' read -r wf state _; do
  [[ -n "${wf:-}" && ! "$wf" =~ ^# ]] || continue
  [[ -f ".github/workflows/$wf" ]] || fail "tests/vnext-workflows.txt declares $wf, which does not exist"
  grep -qE "^(enabled|disabled)$" <<<"$state" || fail "$wf has an invalid state: $state"
done < tests/vnext-workflows.txt

grep -Fq "Developer Certificate of Origin" CONTRIBUTING.md \
  || fail "CONTRIBUTING.md should document the DCO policy"

grep -Fq "Signed-off-by:" CONTRIBUTING.md \
  || fail "CONTRIBUTING.md should explain commit sign-offs"

grep -Fq "Signed-off-by:" .github/PULL_REQUEST_TEMPLATE.md \
  || fail "PR template should remind contributors about sign-off"

grep -Fq "git interpret-trailers --parse" .github/workflows/dco.yml \
  || fail "DCO workflow should validate commit sign-off trailers"

echo "PASS: test_governance_baseline"
