# Acceptance Fixture Status

This document explains the current state of the real-world acceptance fixtures
used by [`tests/e2e/real_world_test.go`](../../tests/e2e/real_world_test.go).

The acceptance layer now mixes two kinds of inputs:

- promoted OSS corpus fixtures with exact upstream provenance
- curated acceptance-only artifacts that still need stronger upstream capture

The acceptance catalog is the source of truth:
- [`tests/artifacts/catalog.yaml`](../../tests/artifacts/catalog.yaml)

The guard that prevents drift back to low-provenance Kubernetes and Terraform
fixtures lives in:
- [`tests/test_acceptance_corpus_promotion.sh`](../../tests/test_acceptance_corpus_promotion.sh)

## Current Promoted OSS Fixtures

These fixtures are first-class acceptance inputs with exact upstream provenance.

| Fixture | Path | Upstream Source | Why It Is Promoted |
| --- | --- | --- | --- |
| Kubescape hostPath fail | `tests/artifacts/fixtures/k8s/kubescape-hostpath-mount-fail.yaml` | `kubescape/regolibrary` `e7639f6653b4a4b274bb8de5aa6a0db3a4c85926` | Real Kubernetes hostPath fixture for companion detector expectations |
| Kubescape non-root pass | `tests/artifacts/fixtures/k8s/kubescape-non-root-deployment-pass.yaml` | `kubescape/regolibrary` `e7639f6653b4a4b274bb8de5aa6a0db3a4c85926` | Real Kubernetes baseline fixture for non-root acceptance |
| Checkov S3 public access fail | `tests/artifacts/fixtures/terraform/checkov-s3-public-access-fail.tfplan.json` | `bridgecrewio/checkov` `8bd89be03d239ff1f118a79a821f989fb119c16c` | Real Terraform plan fixture for public S3 exposure detection |
| Checkov IAM wildcard fail | `tests/artifacts/fixtures/terraform/checkov-iam-wildcard-fail.tfplan.json` | `bridgecrewio/checkov` `8bd89be03d239ff1f118a79a821f989fb119c16c` | Real Terraform plan fixture for wildcard IAM policy detection |

These promoted fixtures currently cover:

- companion Kubernetes canonicalization on exact OSS manifests
- companion Terraform plan canonicalization on exact OSS-derived plan JSON
- external/companion detector classification on realistic inputs

## Remaining Curated Acceptance Fixtures

These artifacts still exist because they provide useful acceptance breadth that
the first OSS corpus wave does not yet replace.

| Fixture | Path | Coverage Role | Current Status |
| --- | --- | --- | --- |
| Helm Redis | `tests/artifacts/fixtures/helm/helm_rendered.yaml` | Rendered Helm manifest normalization | Derived open source, partial provenance |
| Helm ingress-nginx | `tests/artifacts/fixtures/helm/helm_ingress_nginx.yaml` | Helm chart handling with cluster-scoped resources | Derived open source, partial provenance |
| Argo CD sync output | `tests/artifacts/fixtures/argocd/argocd_app_sync.yaml` | Tracking annotation noise filtering | Curated local, partial provenance |
| Kustomize monitoring | `tests/artifacts/fixtures/kustomize/kustomize_monitoring.yaml` | Kustomize rendered manifest handling | Curated local, partial provenance |
| OpenShift app | `tests/artifacts/fixtures/openshift/openshift_app.yaml` | `oc` and OpenShift-specific resource handling | Curated local, partial provenance |

These are still valid acceptance fixtures, and they should stay until equivalent
or better OSS-backed captures exist.

## What Was Replaced

The old low-provenance acceptance-only Kubernetes and Terraform fixtures were
removed:

- `k8s_app_stack.yaml`
- `tf_infra_plan.json`

Those paths were replaced by promoted OSS corpus fixtures to reduce duplication
and improve provenance quality.

## Next Promotion Wave

The next acceptance-fixture promotion wave should focus on replacing the
remaining curated local breadth fixtures where practical:

- Argo CD rendered outputs with pinned upstream examples
- Kustomize monitoring overlays with pinned upstream examples
- OpenShift acceptance fixtures with better source capture
- additional Terraform and Kubernetes pass/fail pairs from Kyverno, Polaris,
  and selected `terraform-provider-aws` examples

This does **not** require converting every acceptance artifact into another test
surface. The rule is simpler:

- prefer exact upstream provenance for new real-world fixtures
- keep curated artifacts when they cover behavior the OSS fixture set does not
  yet represent well

## Related Docs

- [`tests/E2E_TESTING.md`](../../tests/E2E_TESTING.md)
- [`tests/artifacts/fixtures/README.md`](../../tests/artifacts/fixtures/README.md)
