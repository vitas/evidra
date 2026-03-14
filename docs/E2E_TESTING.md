# E2E Testing

This document explains the higher-level test structure in Evidra: what suites
exist, what each suite proves, which data they use, and where duplication is
intentionally forbidden.

For the short directory-by-directory test map, start with
[tests-index.md](tests-index.md).

## Test Taxonomy

Real-world artifact-backed acceptance is the authoritative top-level e2e layer.

| Suite | Role | Primary Data | Authoritative For |
| --- | --- | --- | --- |
| `tests/e2e` | Real-world product acceptance | Curated acceptance artifacts plus promoted OSS corpus fixtures referenced through `tests/artifacts/catalog.yaml` | Canonicalization, classification, findings, and noise handling on realistic artifacts |
| `tests/contracts` | Synthetic contract and integration validation | Small handcrafted fixtures in `tests/contracts/fixtures/` | CLI workflow contracts, output shape, signing, explain/compare, session filtering, scanner ingest |
| `tests/inspector` | MCP Inspector and transport integration | Curated JSON cases + transport fixtures | Inspector runner behavior, stdio/REST/hosted transport coverage |
| `tests/benchmark` | Dataset and benchmark contract validation | Benchmark cases, contract snapshots, and corpus metadata | Dataset integrity, benchmark contract drift, coverage reporting |
| `tests/signal-validation` | Scripted behavioral signal calibration | Local evidence sequences, no external infra | Signal differentiation and scoring sanity |
| Package tests under `cmd/`, `internal/`, `pkg/` | Narrow local behavior | Temp files, unit fixtures | Parser behavior, detector logic, scoring math, command-specific contracts |

## Current Suite Inventory

### `tests/e2e`

| File | Type | Purpose | Data Source |
| --- | --- | --- | --- |
| `tests/e2e/real_world_test.go` | real-world acceptance | Validates adapter behavior and classification on shared Kubernetes/Terraform fixtures plus Helm, Kustomize, OpenShift, and Argo CD artifacts | `tests/artifacts/catalog.yaml`, `tests/artifacts/fixtures/` |
| `tests/e2e/noop_test.go` | guard | Ensures the suite only runs when the `e2e` build tag is enabled | none |

### `tests/contracts`

| File | Type | Purpose | Data Source |
| --- | --- | --- | --- |
| `tests/contracts/explain_compare_test.go` | synthetic contract | Validates `explain` and `compare` output semantics across multiple actors | `tests/contracts/fixtures/k8s_deployment.yaml` |
| `tests/contracts/findings_test.go` | synthetic contract | Validates findings ingest before and after lifecycle events and evidence-chain integrity | `tests/contracts/fixtures/*.sarif`, `k8s_deployment.yaml` |
| `tests/contracts/risk_escalation_test.go` | synthetic contract | Validates scorecard-level `risk_escalation` emission from staged risk changes | `tests/contracts/fixtures/k8s_deployment.yaml` |
| `tests/contracts/run_record_parity_test.go` | synthetic contract | Validates that `run` and `record` emit equivalent signal summaries for the same logical operation | generated configmap + record payload |
| `tests/contracts/scanner_prescribe_test.go` | synthetic contract | Validates `prescribe --findings` bundling behavior | `tests/contracts/fixtures/trivy.sarif` |
| `tests/contracts/session_scoring_test.go` | synthetic contract | Validates `scorecard --session-id` filtering and mixed-session scoring | `tests/contracts/fixtures/k8s_deployment.yaml` |
| `tests/contracts/signing_test.go` | synthetic contract | Validates signed evidence, validation, and tamper detection | `tests/contracts/fixtures/k8s_deployment.yaml` |

### `tests/inspector`

The authoritative inspector integration suite is shell-driven:

- `tests/inspector/run_inspector_tests.sh`
- `tests/inspector/cases/*.json`
- `tests/inspector/special/t_*.sh`

This suite covers:

- local MCP stdio
- local REST backend integration
- hosted MCP and hosted REST modes when network tests are explicitly enabled

### `tests/benchmark`

The benchmark layer is not product e2e. It validates the benchmark dataset and
its contract surfaces:

- dataset schema and metadata
- shared vendored artifact fixtures under `tests/artifacts/fixtures/`
- importer availability for the first reviewed upstream sources
- contract drift for promoted cases
- coverage reporting for the limited benchmark dataset

### `tests/signal-validation`

This suite is not product e2e either. It validates the behavioral signal engine
through scripted evidence sequences and score relationships.

## Package-Level Tests That Still Matter

Top-level suites are not a replacement for package-level tests. The packages
below intentionally keep narrow tests for fast local feedback:

| File | Purpose |
| --- | --- |
| `cmd/evidra/run_test.go` | `run` command output contract and lifecycle write behavior |
| `cmd/evidra/record_test.go` | `record` payload validation and lifecycle write behavior |
| `cmd/evidra/main_test.go` | scorecard/explain JSON contract, signing modes, evidence-write behavior |
| `pkg/mcpserver/e2e_test.go` | MCP server lifecycle behavior and structured output contract |
| `pkg/mcpserver/integration_test.go` | MCP prescribe/report contract integrity |
| `internal/signal/*_test.go` | detector semantics and edge cases |
| `internal/score/*_test.go` | scoring math, bands, confidence, and profile behavior |

These tests should stay narrow. If a package test grows into a full user
workflow already covered by a top-level suite, it should be reduced or removed.

## Duplication Policy

These rules are enforced going forward:

- Real-world acceptance in `tests/e2e` is authoritative for product behavior.
- Synthetic workflow tests do not belong in `tests/e2e`.
- If a package test and a top-level test prove the same workflow contract, keep
  the more user-visible test and reduce the narrower duplicate.
- Synthetic tests are allowed only for:
  - CLI or MCP contract shape
  - signing and serialization behavior
  - hard-to-trigger error paths
  - deterministic unit-level signal and scoring logic

Recent reduction applied:

- removed `cmd/evidra/run_record_parity_test.go`
- removed `tests/e2e/inspector_smoke_test.go`
- moved synthetic top-level workflow tests out of `tests/e2e` into `tests/contracts`

## Fixture And Provenance Policy

Primary acceptance artifacts are vendored under git:

- real-world acceptance catalog: `tests/artifacts/catalog.yaml`
- shared vendored fixtures for acceptance and benchmark flows: `tests/artifacts/fixtures/`
- benchmark case metadata: `tests/benchmark/cases/`
- synthetic contract fixtures: `tests/contracts/fixtures/`

Rules:

- no runtime downloading for primary CI acceptance coverage
- no mirroring of full upstream repositories when a curated artifact slice is enough
- every real-world artifact should have provenance metadata and intended coverage
- benchmark cases reference the shared corpus directly instead of copying
  case-local duplicates

Current reality:

- the limited benchmark dataset now references reviewed first-wave fixtures from
  Kubescape, Checkov, and Kubernetes docs under `tests/artifacts/fixtures/`
- the real-world acceptance suite now consumes shared Kubernetes and Terraform
  fixtures directly from that root through the acceptance catalog
- the exact split between promoted OSS fixtures and remaining curated
  acceptance-only artifacts is documented in
  [Acceptance Fixture Status](guides/acceptance-fixture-status.md)
- benchmark source manifests must carry exact upstream refs instead of local
  snapshot placeholders
- some real fixtures are still curated local slices with partial provenance
- the next artifact-acquisition wave should replace those with better documented
  open-source fixture captures

## Coverage Matrix

| Behavior Area | Authoritative Tests |
| --- | --- |
| Kubernetes canonicalization and resource identity extraction | `tests/e2e/real_world_test.go` |
| Terraform plan canonicalization and risk classification | `tests/e2e/real_world_test.go` |
| Helm/Kustomize/OpenShift/Argo CD rendered-manifest handling | `tests/e2e/real_world_test.go` |
| Explain and compare output contract | `tests/contracts/explain_compare_test.go` |
| Findings ingest and scanner bundling | `tests/contracts/findings_test.go`, `tests/contracts/scanner_prescribe_test.go` |
| Session-filtered scoring | `tests/contracts/session_scoring_test.go` |
| Risk-escalation scorecard surfacing | `tests/contracts/risk_escalation_test.go` |
| Record vs. import parity | `tests/contracts/run_record_parity_test.go` |
| Signed evidence and tamper detection | `tests/contracts/signing_test.go` |
| MCP transport and inspector modes | `tests/inspector/run_inspector_tests.sh`, `pkg/mcpserver/e2e_test.go` |
| Benchmark dataset integrity | `tests/benchmark/scripts/*.sh` and benchmark metadata |
| Signal differentiation | `tests/signal-validation/validate-signals-engine.sh` |

## Current Gaps

These areas still rely on synthetic top-level contract coverage and should gain
real-world artifact coverage over time:

- explain/compare on vendored multi-actor real datasets
- findings ingest on vendored real SARIF outputs from open-source scanners
- risk-escalation scenarios expressed with realistic workload histories
- record/import parity on vendored real artifacts instead of handwritten configmaps

## CI Mapping

| Command | Purpose |
| --- | --- |
| `make e2e` | real-world acceptance (`tests/e2e`) |
| `make test-contracts` | synthetic contract suite (`tests/contracts`) |
| `make test-mcp-inspector-ci` | inspector/transport suite |
| `make test-signals` | signal validation |
| `make benchmark-validate` | benchmark dataset validation |
| `make benchmark-check-contracts` | benchmark contract drift checks |
| `make benchmark-coverage` | benchmark coverage report |

The CI workflow should keep these layers visible as separate steps so repo
readers can map suite names directly to pipeline execution.
