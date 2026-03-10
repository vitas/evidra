# Unified Artifact Fixtures Design

## Goal

Make `tests/artifacts/` the single physical home and inventory root for all
vendored real-world test artifacts used across acceptance and benchmark flows.

The repo should no longer require readers to understand two separate artifact
roots just to answer a simple question like "which real fixtures do we own?"

## Problem

The repository currently mixes two concepts:

1. artifact ownership and provenance
2. benchmark suite organization

Today those concepts leak into the directory structure:

- `tests/artifacts/catalog.yaml` is the logical acceptance inventory
- `tests/artifacts/real/` stores some acceptance fixtures
- `tests/benchmark/corpus/` stores shared OSS-backed fixtures that are used by
  both benchmark validation and acceptance tests

This works mechanically, but it is hard to track. A reader inspecting
`tests/artifacts/real/` sees only part of the real-world fixture inventory,
while another part lives under a benchmark-specific path. The path split makes
it look like benchmark owns artifacts that are now also part of acceptance.

## Desired End State

### One inventory

- `tests/artifacts/catalog.yaml` remains the single source of truth for the
  active acceptance artifact inventory.

### One physical fixture root

- all vendored real-world fixtures live under `tests/artifacts/fixtures/`
- benchmark code and acceptance code both reference that root
- active references to `tests/benchmark/corpus/` disappear from code, tests, and
  public docs
- active references to `tests/artifacts/real/` also disappear in favor of the
  unified root

### Metadata, not folders, expresses provenance

The following distinctions remain important, but they should live in metadata
and not in competing directory roots:

- OSS-derived vs curated local
- exact provenance vs partial provenance
- promoted acceptance fixture vs benchmark-only artifact

The catalog already has the right fields for this:

- `source_type`
- `provenance_status`
- `upstream_project`
- `upstream_ref`

## Proposed Layout

The new live fixture tree should be:

```text
tests/artifacts/
  catalog.yaml
  fixtures/
    k8s/
    terraform/
    helm/
    argocd/
    kustomize/
    openshift/
    sarif/
```

Rules:

- organize by artifact family, not by owning test suite
- keep benchmark case metadata under `tests/benchmark/cases/`
- keep benchmark scripts under `tests/benchmark/scripts/`
- do not reintroduce a second active fixture root

## Migration Rules

### Rule 1: Move the entire shared benchmark corpus

Everything currently under `tests/benchmark/corpus/` moves into the
corresponding family directory under `tests/artifacts/fixtures/`.

Examples:

- `tests/benchmark/corpus/k8s/...` ->
  `tests/artifacts/fixtures/k8s/...`
- `tests/benchmark/corpus/terraform/...` ->
  `tests/artifacts/fixtures/terraform/...`

### Rule 2: Move the existing curated acceptance fixtures too

Everything currently under `tests/artifacts/real/` also moves into the unified
family-based fixture tree.

Examples:

- `tests/artifacts/real/helm_rendered.yaml` ->
  `tests/artifacts/fixtures/helm/helm_rendered.yaml`
- `tests/artifacts/real/argocd_app_sync.yaml` ->
  `tests/artifacts/fixtures/argocd/argocd_app_sync.yaml`

### Rule 3: Keep benchmark cases, not benchmark-owned artifacts

`tests/benchmark/cases/` continues to define benchmark cases and case metadata.
It should reference shared artifacts, not act as a second storage system for the
same real fixture inventory.

### Rule 4: Prefer direct cutover over long compatibility shims

Short-lived transition notes are acceptable:

- a compatibility README in the old location
- temporary guard scripts to block regressions

Long-lived compatibility paths are not desirable. The repo should settle on one
active path model quickly.

## Implementation Scope

### Included

- move all live corpus files into `tests/artifacts/fixtures/`
- move current `tests/artifacts/real/*` files into the same unified root
- update code, tests, scripts, and docs to the new paths
- add guards that fail if active references drift back to the old roots
- keep catalog metadata authoritative for provenance and source classification

### Excluded

- changing detector behavior
- changing benchmark case semantics
- changing acceptance assertions beyond path updates
- redefining provenance fields

## Risks

### Risk: path churn breaks scripts

Shell scripts and docs currently hardcode both old roots.

Mitigation:

- do a repo-wide reference audit
- update all active scripts in the same change
- add path guards that fail if old roots remain in live references

### Risk: benchmark scripts still assume the corpus root is benchmark-owned

Several benchmark validation and importer scripts refer directly to
`tests/benchmark/corpus`.

Mitigation:

- update those scripts to the unified root
- make the new artifact root explicit in script variables
- keep benchmark logic focused on case metadata and artifact validation rather
  than benchmark-owned storage

### Risk: doc drift leaves the old mental model behind

Readers may continue to see `tests/benchmark/corpus/` in public docs and infer
that benchmark owns the shared artifacts.

Mitigation:

- update the active public docs in this wave
- leave only clearly historical references in archived plans if necessary

## Verification Strategy

Required verification after implementation:

- guard scripts fail if active references remain under `tests/benchmark/corpus`
  or `tests/artifacts/real`
- acceptance catalog paths resolve under `tests/artifacts/fixtures/`
- benchmark validation scripts run successfully with the new root
- targeted real-world e2e tests still pass
- public docs describe one artifact home and one inventory model

Suggested commands:

```bash
bash tests/test_acceptance_corpus_promotion.sh
make benchmark-validate
go test -tags e2e ./tests/e2e -count=1
```

## Output

At the end of the refactor:

- `tests/artifacts/catalog.yaml` is the obvious inventory front door
- `tests/artifacts/fixtures/` is the only active physical home for vendored
  real-world fixtures
- provenance is expressed by metadata, not by folder ownership
- benchmark and acceptance suites share artifacts without sharing confusing path
  semantics
