#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

required_corpus_files=(
  tests/artifacts/fixtures/k8s/kubescape-privileged-container-fail.yaml
  tests/artifacts/fixtures/k8s/kubescape-hostpath-mount-fail.yaml
  tests/artifacts/fixtures/k8s/kubescape-non-root-deployment-fail.yaml
  tests/artifacts/fixtures/k8s/kubescape-non-root-deployment-pass.yaml
  tests/artifacts/fixtures/k8s/k8s-website-nginx-deployment.yaml
  tests/artifacts/fixtures/k8s/k8s-website-security-context-pod.yaml
  tests/artifacts/fixtures/terraform/checkov-s3-public-access-fail.tf
  tests/artifacts/fixtures/terraform/checkov-s3-public-access-fail.tfplan.json
  tests/artifacts/fixtures/terraform/checkov-s3-public-access-pass.tf
  tests/artifacts/fixtures/terraform/checkov-s3-public-access-pass.tfplan.json
  tests/artifacts/fixtures/terraform/checkov-iam-wildcard-fail.tf
  tests/artifacts/fixtures/terraform/checkov-iam-wildcard-fail.tfplan.json
  tests/artifacts/fixtures/terraform/checkov-iam-wildcard-pass.tf
  tests/artifacts/fixtures/terraform/checkov-iam-wildcard-pass.tfplan.json
)

for file in "${required_corpus_files[@]}"; do
  [[ -f "$file" ]] || fail "missing shared fixture $file"
done

echo "PASS: test_oss_dataset_corpus"
