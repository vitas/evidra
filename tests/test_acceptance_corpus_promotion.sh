#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

has_pattern() {
  local pattern="$1"
  local file="$2"
  if command -v rg >/dev/null 2>&1; then
    rg -q -e "$pattern" "$file"
    return
  fi
  grep -Eq -- "$pattern" "$file"
}

has_fixed_pattern() {
  local pattern="$1"
  local file="$2"
  if command -v rg >/dev/null 2>&1; then
    rg -Fq -- "$pattern" "$file"
    return
  fi
  grep -Fq -- "$pattern" "$file"
}

CATALOG="tests/artifacts/catalog.yaml"
E2E_TEST="tests/e2e/real_world_test.go"

[[ -f "$CATALOG" ]] || fail "missing $CATALOG"
[[ -f "$E2E_TEST" ]] || fail "missing $E2E_TEST"

if has_pattern 'k8s_app_stack.yaml|tf_infra_plan.json' "$CATALOG"; then
  fail "acceptance catalog still references low-provenance k8s/terraform fixtures"
fi

legacy_catalog_root_regex='tests/(benchmark/''corpus|artifacts/''real)/'
if has_pattern "$legacy_catalog_root_regex" "$CATALOG"; then
  fail "acceptance catalog still points at pre-unification artifact roots"
fi

required_catalog_paths=(
  "tests/artifacts/fixtures/k8s/kubescape-hostpath-mount-fail.yaml"
  "tests/artifacts/fixtures/k8s/kubescape-non-root-deployment-pass.yaml"
  "tests/artifacts/fixtures/terraform/checkov-s3-public-access-fail.tfplan.json"
  "tests/artifacts/fixtures/terraform/checkov-iam-wildcard-fail.tfplan.json"
)

for path in "${required_catalog_paths[@]}"; do
  has_fixed_pattern "$path" "$CATALOG" || fail "catalog missing promoted corpus artifact $path"
done

if has_pattern 'k8s_app_stack.yaml|tf_infra_plan.json' "$E2E_TEST"; then
  fail "real_world_test.go still references curated k8s/terraform fixtures"
fi

if has_pattern 'realFixture\(|corpusFixture\(' "$E2E_TEST"; then
  fail "real_world_test.go still uses split fixture helpers"
fi

for needle in \
  'fixturePath("k8s", "kubescape-hostpath-mount-fail.yaml")' \
  'fixturePath("k8s", "kubescape-non-root-deployment-pass.yaml")' \
  'fixturePath("terraform", "checkov-s3-public-access-fail.tfplan.json")' \
  'fixturePath("terraform", "checkov-iam-wildcard-fail.tfplan.json")'
do
  has_fixed_pattern "$needle" "$E2E_TEST" || fail "real_world_test.go missing promoted corpus fixture $needle"
done

echo "PASS: test_acceptance_corpus_promotion"
